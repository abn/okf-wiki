package wiki

import (
	"context"
	"fmt"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// DefaultAddr is the address serve binds when none is given: loopback only.
//
// A wiki is a documentation site with no authentication, so the safe default is
// one only this machine can reach. A container, or a deliberate LAN share, sets
// OKF_WIKI_ADDR or --addr; the shipped image already sets it to 0.0.0.0:8080.
const DefaultAddr = "127.0.0.1:8080"

// loopback reports whether addr binds only to this machine, so a non-loopback
// bind can be called out on startup rather than discovered later.
func loopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return true // unparseable, so not worth claiming anything about it
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// ServeOptions configures the wiki server.
type ServeOptions struct {
	OutDir   string // rendered site root (contains wiki/)
	Base     string // URL prefix the wiki is mounted at (default "/wiki/")
	RepoRoot string // repository root, exposed read-only at /repo/ (optional)
	Addr     string // host:port, defaults to DefaultAddr
	Open     bool   // open the default browser on startup
}

// handler builds the route table. It is separate from Serve so the routes can
// be exercised without binding a port.
func handler(opts ServeOptions) http.Handler {
	base := Config{Base: opts.Base}.base()

	mux := http.NewServeMux()
	site := noCache(http.FileServer(http.Dir(opts.OutDir)))
	mux.Handle(base, site)
	// A base of "/" is the whole site, so there is no prefix to redirect and
	// nothing left for a catch-all: TrimSuffix would give "", which ServeMux
	// rejects as a pattern, and "/" is already taken by the site above.
	if trimmed := strings.TrimSuffix(base, "/"); trimmed != "" {
		mux.Handle(trimmed, http.RedirectHandler(base, http.StatusMovedPermanently))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				http.Redirect(w, r, base, http.StatusFound)
				return
			}
			site.ServeHTTP(w, r)
		})
	}
	mux.Handle("/index.html", site)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	if opts.RepoRoot != "" {
		mux.Handle("/repo/", http.StripPrefix("/repo/", repoFileServer(opts.RepoRoot)))
	}
	return mux
}

// Serve serves the rendered wiki until the process is interrupted. It blocks;
// the caller should treat a returned error as fatal.
func Serve(opts ServeOptions) error {
	addr := opts.Addr
	if addr == "" {
		addr = DefaultAddr
	}
	mux := handler(opts)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	display := ln.Addr().String()
	if host, port, err := net.SplitHostPort(display); err == nil && (host == "::" || host == "0.0.0.0") {
		display = "localhost:" + port
	}
	fmt.Print(announce(opts, addr, display))

	if opts.Open {
		url := wikiURL(opts, display)
		go func() {
			time.Sleep(250 * time.Millisecond)
			if err := openBrowser(url); err != nil {
				fmt.Printf("okf-wiki: could not open browser (%v); open %s manually\n", err, url)
			}
		}()
	}

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		// A dev server on loopback serves a handful of large assets, so these
		// are generous. They exist so a stalled or slow client cannot hold a
		// connection open indefinitely.
		WriteTimeout: 5 * time.Minute,
		IdleTimeout:  2 * time.Minute,
	}

	// Drain on a signal rather than dying where we stand. The container's stop
	// path is SIGTERM, and on the default disposition the process is gone
	// immediately, so a response part way through a body is truncated.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return serveUntil(ctx, srv, ln)
}

// serveUntil serves on ln until ctx is done, then drains. Draining rather than
// closing is the point: Close drops in-flight connections, Shutdown waits for
// them. It is separated from Serve so the shutdown path can be tested without a
// process to signal.
func serveUntil(ctx context.Context, srv *http.Server, ln net.Listener) error {
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ln) }()

	select {
	case err := <-done:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	case <-ctx.Done():
		fmt.Println("okf-wiki: shutting down")
		if err := Shutdown(srv); err != nil {
			// A shutdown that times out still has to be bounded, or a wedged
			// connection holds the process open forever.
			_ = srv.Close()
			return fmt.Errorf("shutdown: %w", err)
		}
		// Serve always returns once Shutdown completes.
		<-done
		return nil
	}
}

func init() {
	// Browsers only load a module with a JavaScript MIME type, and the vendored
	// ELK layout ships .mjs chunk files.
	_ = mime.AddExtensionType(".mjs", "text/javascript")
	_ = mime.AddExtensionType(".js", "text/javascript")
}

// noCache forces revalidation on every request. http.FileServer otherwise lets
// browsers heuristically cache scripts/styles, serving a stale diagram runtime
// or stylesheet after a re-render.
func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		next.ServeHTTP(w, r)
	})
}

// repoFileServer serves the repository root read-only, refusing dotfiles so
// local secrets (.vault_pass, .env, .git) are never reachable through the wiki.
func repoFileServer(root string) http.Handler {
	fileServer := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, seg := range strings.Split(r.URL.Path, "/") {
			if strings.HasPrefix(seg, ".") {
				http.NotFound(w, r)
				return
			}
		}
		fileServer.ServeHTTP(w, r)
	})
}

func openBrowser(url string) error {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler"}
	default:
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}

// wikiURL is the address to open, given the resolved listener.
func wikiURL(opts ServeOptions, display string) string {
	return "http://" + display + Config{Base: opts.Base}.base()
}

// announce is what the server says once it is listening. The address line names
// the resolved listener, and a bind that is not loopback is called out, because
// the wiki has no authentication and that is not something to discover later.
func announce(opts ServeOptions, addr, display string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "okf-wiki: serving %s at %s\n", opts.OutDir, wikiURL(opts, display))
	if !loopback(addr) {
		fmt.Fprintf(&b, "okf-wiki: %s is not loopback, so this wiki and any /repo/ "+
			"mount are reachable from other machines; it has no authentication\n", addr)
	}
	b.WriteString("okf-wiki: press Ctrl-C to stop\n")
	return b.String()
}

// Shutdown is a convenience wrapper for callers that manage the server handle.
func Shutdown(srv *http.Server) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}
