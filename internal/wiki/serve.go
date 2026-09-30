package wiki

import (
	"context"
	"fmt"
	"mime"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// ServeOptions configures the wiki server.
type ServeOptions struct {
	OutDir   string // rendered site root (contains wiki/)
	Base     string // URL prefix the wiki is mounted at (default "/wiki/")
	RepoRoot string // repository root, exposed read-only at /repo/ (optional)
	Addr     string // host:port, defaults to 0.0.0.0:8080
	Open     bool   // open the default browser on startup
}

// Serve serves the rendered wiki until the process is interrupted. It blocks;
// the caller should treat a returned error as fatal. Registering a pattern of
// "" panics inside http.ServeMux, so a base of "/" must not take the
// TrimSuffix branch below.
func Serve(opts ServeOptions) error {
	addr := opts.Addr
	if addr == "" {
		addr = "0.0.0.0:8080"
	}
	base := Config{Base: opts.Base}.base()

	mux := http.NewServeMux()
	site := noCache(http.FileServer(http.Dir(opts.OutDir)))
	mux.Handle(base, site)
	mux.Handle(strings.TrimSuffix(base, "/"), http.RedirectHandler(base, http.StatusMovedPermanently))
	mux.Handle("/index.html", site)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	if opts.RepoRoot != "" {
		mux.Handle("/repo/", http.StripPrefix("/repo/", repoFileServer(opts.RepoRoot)))
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, base, http.StatusFound)
			return
		}
		site.ServeHTTP(w, r)
	})

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	display := ln.Addr().String()
	if host, port, err := net.SplitHostPort(display); err == nil && (host == "::" || host == "0.0.0.0") {
		display = "localhost:" + port
	}
	url := "http://" + display + base
	fmt.Printf("okf-wiki: serving %s at %s\n", opts.OutDir, url)
	fmt.Println("okf-wiki: press Ctrl-C to stop")

	if opts.Open {
		go func() {
			time.Sleep(250 * time.Millisecond)
			if err := openBrowser(url); err != nil {
				fmt.Printf("okf-wiki: could not open browser (%v); open %s manually\n", err, url)
			}
		}()
	}

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	err = srv.Serve(ln)
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
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

// Shutdown is a convenience wrapper for callers that manage the server handle.
// Nothing in this repository calls it: Serve blocks on srv.Serve and the
// process ends on a signal, so there is no in-flight drain.
func Shutdown(srv *http.Server) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}
