package wiki

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
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
	site := noCache(etag(opts.OutDir)(indexDirect(http.FileServer(http.Dir(opts.OutDir)), opts.OutDir)))
	mux.Handle(base, site)
	// A base of "/" is the whole site, so there is no prefix to redirect and
	// nothing left for a catch-all: TrimSuffix would give "", which ServeMux
	// rejects as a pattern, and "/" is already taken by the site above.
	if trimmed := strings.TrimSuffix(base, "/"); trimmed != "" {
		mux.Handle(trimmed, http.RedirectHandler(base, http.StatusMovedPermanently))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				// A bundle root holding landing.md introduces the deployment
				// at "/", so the root serves the page rather than bouncing to
				// the wiki. The marker tells the two apart per request, which
				// is what keeps --watch correct when the file appears or goes.
				if hasLandingRoot(opts.OutDir) {
					site.ServeHTTP(w, r)
					return
				}
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

// indexDirect serves a request for a page's index.html in place, instead of
// letting http.FileServer answer it with a 301 to "./".
//
// A nav link is written as slug + ".html", so the home page of a section is
// requested as .../index.html. FileServer treats that as a directory and
// redirects, which costs a whole extra round trip before the document on every
// index link a reader clicks. http.ServeFile has the same rule for any path
// ending in /index.html, so the path is rewritten to the file before serving;
// the canonical directory URL still resolves through next unchanged.
func indexDirect(next http.Handler, outDir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/index.html") {
			clean := path.Clean(r.URL.Path)
			// A path that escapes the output dir is left to next, which will
			// clean or refuse it rather than this serving an arbitrary file.
			if !strings.Contains(clean, "..") {
				file := filepath.Join(outDir, filepath.FromSlash(clean))
				if st, err := os.Stat(file); err == nil && !st.IsDir() {
					// Copy the request so the caller's URL is untouched, and
					// drop the trailing /index.html that ServeFile redirects on.
					r2 := r.Clone(r.Context())
					r2.URL.Path = strings.TrimSuffix(clean, "index.html")
					http.ServeFile(w, r2, file)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// noCache forces revalidation on every request. http.FileServer otherwise lets
// browsers heuristically cache scripts/styles, serving a stale diagram runtime
// or stylesheet after a re-render. The ETag beside it is what makes that
// revalidation cheap: Last-Modified is the file's mtime, and a re-render
// rewrites every file in the tree, so an otherwise unchanged asset would take a
// fresh mtime and be answered with a full 200. A content hash does not move
// when only the time did, so an unchanged file revalidates to a 304.
func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		next.ServeHTTP(w, r)
	})
}

// etag sets an ETag from the file's content so a conditional request is answered
// with a 304 even after a re-render moved its modification time.
//
// The hash is computed from the bytes on disk at request time. On the output of
// a dev server that is a handful of small files, and a render already rewrites
// them, so this costs one read of a file the request is about to read anyway.
// A body that is not a whole file, a 304 already, or an error is left alone:
// the header means nothing there and would be wrong to send.
func etag(outDir string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clean := path.Clean(r.URL.Path)
			if strings.Contains(clean, "..") {
				next.ServeHTTP(w, r)
				return
			}
			file := filepath.Join(outDir, filepath.FromSlash(clean))
			st, err := os.Stat(file)
			if err != nil || st.IsDir() {
				next.ServeHTTP(w, r)
				return
			}
			// A range request or a conditional PUT is not a plain GET, so a full
			// content hash says nothing useful and the header is not set.
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				next.ServeHTTP(w, r)
				return
			}
			sum, err := fileSHA256(file)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			tag := `"` + sum + `"`
			w.Header().Set("ETag", tag)
			// If-None-Match wins over If-Modified-Since (RFC 9110), so a match
			// here is answered before the handler can send a 200 body.
			if match := r.Header.Get("If-None-Match"); match != "" && etagMatches(match, tag) {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// fileSHA256 returns the hex SHA-256 of a file's contents.
func fileSHA256(name string) (string, error) {
	f, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// etagMatches reports whether an If-None-Match header names the given tag.
// The header is a comma-separated list, "*" matches anything, and a weak tag
// ("W/...") is compared without its prefix, which is what makes a weak
// validator usable for a conditional GET.
func etagMatches(header, tag string) bool {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "*" {
			return true
		}
		if strings.TrimPrefix(part, "W/") == tag {
			return true
		}
	}
	return false
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
