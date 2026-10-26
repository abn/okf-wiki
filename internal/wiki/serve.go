package wiki

import (
	"compress/gzip"
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
	"strconv"
	"strings"
	"sync"
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
	OutDir    string       // rendered site root (contains wiki/)
	Base      string       // URL prefix the wiki is mounted at (default "/wiki/")
	RepoRoot  string       // repository root, exposed read-only at /repo/ (optional)
	Addr      string       // host:port, defaults to DefaultAddr
	Open      bool         // open the default browser on startup
	MCP       http.Handler // read-only MCP endpoint mounted under the base (optional)
	MCPPrefix string       // segment before mcp; empty mounts at <base>/mcp
}

// MCPPath is the URL path the MCP endpoint is served at: the base, the optional
// prefix segment, then mcp. An empty prefix places it directly under the base,
// in the page namespace.
func MCPPath(base, prefix string) string {
	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	if prefix != "" {
		prefix += "/"
	}
	return Config{Base: base}.base() + prefix + "mcp"
}

// NormalizeMCPPrefix accepts the segment between the base and mcp. Surrounding
// slashes are trimmed so "_", "/_" and "_/" are the same, and an empty value
// means no segment. Anything that is not one plain path segment is refused,
// because the value is concatenated into a route pattern.
func NormalizeMCPPrefix(prefix string) (string, error) {
	trimmed := strings.TrimSpace(prefix)
	p := strings.Trim(trimmed, "/")
	if p == "" {
		if prefix == "" {
			return "", nil
		}
		return "", fmt.Errorf("--mcp-prefix %q is empty after trimming; use \"\" for no prefix", prefix)
	}
	for _, r := range p {
		plain := r == '-' || r == '_' || r == '.' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if !plain {
			return "", fmt.Errorf("--mcp-prefix %q must be one path segment of letters, digits, dot, dash or underscore", prefix)
		}
	}
	if p == "." || p == ".." {
		return "", fmt.Errorf("--mcp-prefix %q must be one path segment of letters, digits, dot, dash or underscore", prefix)
	}
	return p, nil
}

// handler builds the route table. It is separate from Serve so the routes can
// be exercised without binding a port.
func handler(opts ServeOptions) http.Handler {
	base := Config{Base: opts.Base}.base()

	mux := http.NewServeMux()
	site := noCache(gzipResponses(etag(opts.OutDir)(indexDirect(http.FileServer(http.Dir(opts.OutDir)), opts.OutDir))))
	mux.Handle(base, site)
	if opts.MCP != nil {
		// An exact pattern beats the base subtree, and {$} accepts the
		// trailing slash a client may add without opening the whole subtree.
		path := MCPPath(opts.Base, opts.MCPPrefix)
		mux.Handle(path, opts.MCP)
		mux.Handle(path+"/{$}", opts.MCP)
	}
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
	if opts.MCP != nil {
		prefix, err := NormalizeMCPPrefix(opts.MCPPrefix)
		if err != nil {
			return err
		}
		opts.MCPPrefix = prefix
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

// gzipResponses compresses a compressible static response when the client
// accepts gzip. The rendered site is mostly text, and the aggregate search
// index is the large JSON response this exists for, so the transfer is
// dominated by exactly these bodies.
//
// The choice is per response, not per route: the file server sets the content
// type from the file's extension when it writes, and images, fonts and other
// already-compressed bytes must pass through untouched. Vary is set on every
// response, including the uncompressed one, so a cache keys both variants of a
// URL on Accept-Encoding. The variant's ETag is set by etag(), which knows the
// file, so a conditional request and its 200 agree on one validator.
func gzipResponses(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Accept-Encoding")
		if !acceptsGzip(r) || r.Header.Get("Range") != "" {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipResponseWriter{ResponseWriter: w}
		defer gw.close()
		next.ServeHTTP(gw, r)
	})
}

// gzipResponseWriter decides at header time whether the body is compressible,
// then routes writes through a pooled gzip writer. A response that does not
// qualify is written through unchanged.
type gzipResponseWriter struct {
	http.ResponseWriter
	gz      *gzip.Writer
	decided bool
}

func (w *gzipResponseWriter) WriteHeader(code int) {
	if w.decided {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	w.decided = true
	h := w.Header()
	if code >= 200 && code != http.StatusNoContent && h.Get("Content-Encoding") == "" &&
		compressibleContentType(h.Get("Content-Type")) && compressibleLength(h.Get("Content-Length")) {
		h.Set("Content-Encoding", "gzip")
		// The file server's length describes the bytes on disk, not the
		// compressed ones, and a wrong length would truncate the response.
		h.Del("Content-Length")
		w.gz = newGzipWriter(w.ResponseWriter)
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *gzipResponseWriter) Write(p []byte) (int, error) {
	if !w.decided {
		w.WriteHeader(http.StatusOK)
	}
	if w.gz == nil {
		return w.ResponseWriter.Write(p)
	}
	return w.gz.Write(p)
}

// Flush preserves http.Flusher across the wrapper, which would otherwise hide
// it: the gzip buffer is flushed before the underlying writer's.
func (w *gzipResponseWriter) Flush() {
	if !w.decided {
		w.WriteHeader(http.StatusOK)
	}
	if w.gz != nil {
		_ = w.gz.Flush()
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *gzipResponseWriter) close() {
	if w.gz == nil {
		return
	}
	_ = w.gz.Close()
	gzipPool.Put(w.gz)
	w.gz = nil
}

// A pooled writer is reused across requests: the search index is large enough
// that allocating a fresh one per request and letting its buffers churn is
// worth avoiding. BestSpeed keeps the render-serve loop responsive; the index
// is read once and answered many times.
var gzipPool = sync.Pool{
	New: func() any {
		gz, _ := gzip.NewWriterLevel(io.Discard, gzip.BestSpeed)
		return gz
	},
}

func newGzipWriter(w io.Writer) *gzip.Writer {
	gz := gzipPool.Get().(*gzip.Writer)
	gz.Reset(w)
	return gz
}

// acceptsGzip reports whether the request's Accept-Encoding allows gzip. The
// choice is per RFC 9110: an explicit gzip entry overrides the wildcard, so
// `gzip;q=0, *` refuses gzip rather than accepting it through `*`.
func acceptsGzip(r *http.Request) bool {
	header := r.Header.Get("Accept-Encoding")
	if header == "" {
		return false
	}
	explicit := -1.0
	wildcard := -1.0
	for _, part := range strings.Split(header, ",") {
		name, params, _ := strings.Cut(part, ";")
		name = strings.TrimSpace(name)
		switch {
		case strings.EqualFold(name, "gzip"):
			explicit = qValue(params)
		case name == "*":
			wildcard = qValue(params)
		}
	}
	if explicit >= 0 {
		return explicit > 0
	}
	if wildcard >= 0 {
		return wildcard > 0
	}
	return false
}

func qValue(params string) float64 {
	for _, p := range strings.Split(params, ";") {
		p = strings.TrimSpace(p)
		if v, ok := strings.CutPrefix(p, "q="); ok {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return f
			}
		}
	}
	return 1
}

// compressibleContentType reports whether a response of this content type is
// worth gzipping. It is the text-like set gzip helps; an image or a font is
// already compressed, and recompressing it spends CPU to grow the body.
func compressibleContentType(ct string) bool {
	if ct == "" {
		return false
	}
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		mt = ct
	}
	switch mt {
	case "application/json", "application/javascript", "text/javascript", "image/svg+xml":
		return true
	}
	return strings.HasPrefix(mt, "text/")
}

// minGzipBytes is where compression stops paying: below this the framing and
// the compression cost more than the bytes they save, and a tiny body grows.
const minGzipBytes = 1024

// compressibleLength reports whether a body of this size is worth compressing.
// An absent or unparsable length counts as compressible, since a streaming
// response has no known size to weigh.
func compressibleLength(contentLength string) bool {
	if contentLength == "" {
		return true
	}
	n, err := strconv.ParseInt(contentLength, 10, 64)
	if err != nil {
		return true
	}
	return n >= minGzipBytes
}

// gzipCoded reports whether a response for this request would be gzip-coded,
// which decides the validator its 200 and its 304 must carry. It mirrors the
// gzip middleware: gzip accepted, no range, a compressible type and a body
// large enough to be worth compressing.
func gzipCoded(r *http.Request, file string, size int64) bool {
	if !acceptsGzip(r) || r.Header.Get("Range") != "" {
		return false
	}
	return size >= minGzipBytes && compressibleContentType(contentTypeForFile(file))
}

// contentTypeForFile mirrors the rule http.ServeContent uses: the extension
// when it names a type, and a sniff of the leading bytes otherwise. The gzip
// wrapper reads the same content type off the response header, so the tag this
// chooses agrees with the coding the wrapper applies.
func contentTypeForFile(file string) string {
	if ct := mime.TypeByExtension(filepath.Ext(file)); ct != "" {
		return ct
	}
	f, err := os.Open(file)
	if err != nil {
		return ""
	}
	defer f.Close()
	var buf [512]byte
	n, _ := io.ReadFull(f, buf[:])
	return http.DetectContentType(buf[:n])
}

// gzipETag marks a tag as belonging to the gzip variant of an entity, so the
// compressed and uncompressed responses of one URL cannot share a validator.
func gzipETag(tag string) string {
	if strings.HasSuffix(tag, `"`) {
		return tag[:len(tag)-1] + `-gzip"`
	}
	return tag + "-gzip"
}

// stripGzipSuffix reduces a gzip-variant tag to the content tag its coding
// shares with the plain one.
func stripGzipSuffix(tag string) string {
	if strings.HasSuffix(tag, `-gzip"`) {
		return tag[:len(tag)-len(`-gzip"`)] + `"`
	}
	return tag
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
			sum, err := cachedFileSHA256(file)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			tag := `"` + sum + `"`
			// The response carries the tag of the coding the server would use,
			// not of the coding the request named. A conditional request and
			// its 200 therefore always agree: a 304 for a gzip-coded exchange
			// names the gzip variant, and a plain-coded one the plain tag.
			variant := tag
			if gzipCoded(r, file, st.Size()) {
				variant = gzipETag(tag)
			}
			w.Header().Set("ETag", variant)
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

// An etag is derived from file contents, but the contents of a rendered file do
// not change between re-renders: serve sees the same file until the watcher
// writes a new one and mtime advances. Caching on size and mtime avoids
// re-hashing every request.
var etagCacheMu sync.Mutex
var etagCache = map[string]etagCacheEntry{}

type etagCacheEntry struct {
	size  int64
	mtime time.Time
	sum   string
}

func cachedFileSHA256(name string) (string, error) {
	st, err := os.Stat(name)
	if err != nil {
		return "", err
	}
	etagCacheMu.Lock()
	if e, ok := etagCache[name]; ok && e.size == st.Size() && e.mtime.Equal(st.ModTime()) {
		sum := e.sum
		etagCacheMu.Unlock()
		return sum, nil
	}
	etagCacheMu.Unlock()

	sum, err := fileSHA256(name)
	if err != nil {
		return "", err
	}
	etagCacheMu.Lock()
	etagCache[name] = etagCacheEntry{size: st.Size(), mtime: st.ModTime(), sum: sum}
	etagCacheMu.Unlock()
	return sum, nil
}

// etagMatches reports whether an If-None-Match header names the given tag.
// The header is a comma-separated list, "*" matches anything, and a weak tag
// ("W/...") is compared without its prefix, which is what makes a weak
// validator usable for a conditional GET. The gzip variant of a tag also
// matches: the two codings share one underlying file, so a client holding
// either revalidates the resource.
func etagMatches(header, tag string) bool {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "*" {
			return true
		}
		part = strings.TrimPrefix(part, "W/")
		if part == tag || stripGzipSuffix(part) == tag {
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
	if opts.MCP != nil {
		fmt.Fprintf(&b, "okf-wiki: MCP at http://%s%s\n", display, MCPPath(opts.Base, opts.MCPPrefix))
	}
	if !loopback(addr) {
		reachable := "this wiki and any /repo/ mount"
		if opts.MCP != nil {
			reachable = "this wiki, any /repo/ mount and the MCP endpoint"
		}
		fmt.Fprintf(&b, "okf-wiki: %s is not loopback, so %s are reachable from "+
			"other machines; it has no authentication\n", addr, reachable)
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
