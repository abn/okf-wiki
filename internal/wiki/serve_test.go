package wiki

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The route table and the file server work end to end over a real listener.
// An index.html link must not cost a redirect. A nav link is written as slug
// plus ".html", so the home page of a section is requested as .../index.html,
// which http.FileServer and http.ServeFile both answer with a 301 to "./".
// That is a whole extra round trip before the document on every index link, so
// the handler serves the file in place; the canonical directory URL still works.
func TestServeServesIndexHTMLWithoutARedirect(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "wiki", "guide"))
	mustWrite(t, filepath.Join(dir, "wiki", "index.html"), "HOME")
	mustWrite(t, filepath.Join(dir, "wiki", "guide", "index.html"), "GUIDE")
	mustWrite(t, filepath.Join(dir, "wiki", "guide", "page.html"), "PAGE")

	h := handler(ServeOptions{OutDir: dir, Base: "/wiki/"})
	for _, tc := range []struct{ path, want string }{
		{"/wiki/index.html", "HOME"},
		{"/wiki/guide/index.html", "GUIDE"},
		{"/wiki/", "HOME"},
		{"/wiki/guide/", "GUIDE"},
		{"/wiki/guide/page.html", "PAGE"},
	} {
		req := httptest.NewRequest("GET", tc.path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200 (a redirect here is the extra round trip)", tc.path, w.Code)
			continue
		}
		if got := w.Body.String(); got != tc.want {
			t.Errorf("GET %s body = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// An ETag from the file's content keeps revalidation cheap after a re-render.
// Last-Modified is the file's mtime, and a re-render rewrites every file in the
// tree, so an unchanged asset would take a fresh mtime and be answered with a
// full 200. A content hash does not move when only the time did.
func TestServeEtagFromContent(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "wiki", "wiki.css")
	mustMkdir(t, filepath.Join(dir, "wiki"))
	mustWrite(t, file, "body { color: red }")

	h := handler(ServeOptions{OutDir: dir, Base: "/wiki/"})
	get := func(headers map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/wiki/wiki.css", nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}

	first := get(nil)
	if first.Code != http.StatusOK {
		t.Fatalf("plain GET = %d, want 200", first.Code)
	}
	tag := first.Header().Get("ETag")
	if tag == "" {
		t.Fatal("no ETag on the response")
	}
	if cc := first.Header().Get("Cache-Control"); cc != "no-cache, must-revalidate" {
		t.Errorf("Cache-Control = %q, want the revalidate pair", cc)
	}

	if got := get(map[string]string{"If-None-Match": tag}); got.Code != http.StatusNotModified {
		t.Errorf("If-None-Match with the tag = %d, want 304", got.Code)
	}
	if got := get(map[string]string{"If-None-Match": `"other"`}); got.Code != http.StatusOK {
		t.Errorf("If-None-Match with another tag = %d, want 200", got.Code)
	}
	if got := get(map[string]string{"If-None-Match": "*"}); got.Code != http.StatusNotModified {
		t.Errorf("If-None-Match: * = %d, want 304", got.Code)
	}

	// Rewriting the file with the same bytes but a new mtime must not change the
	// tag: this is the re-render case the header exists for.
	time.Sleep(10 * time.Millisecond)
	mustWrite(t, file, "body { color: red }")
	if got := get(map[string]string{"If-None-Match": tag}); got.Code != http.StatusNotModified {
		t.Errorf("after a same-bytes rewrite = %d, want 304", got.Code)
	}

	// A changed byte is a new tag, and the old one no longer matches.
	mustWrite(t, file, "body { color: blue }")
	changed := get(nil).Header().Get("ETag")
	if changed == tag {
		t.Error("the ETag did not move when the bytes did")
	}
	if got := get(map[string]string{"If-None-Match": tag}); got.Code != http.StatusOK {
		t.Errorf("stale If-None-Match after an edit = %d, want 200", got.Code)
	}
}

func TestServeServesOverARealListener(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.html"), "<h1>hi</h1>")

	addr := freeAddr(t)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: handler(ServeOptions{OutDir: dir, Base: "/"})}
	done := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { done <- serveUntil(ctx, srv, ln) }()
	waitForHealth(t, addr)

	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(got), "<h1>hi</h1>") {
		t.Errorf("GET / = %d, body %q", resp.StatusCode, got)
	}

	// A cancelled context stops the server and returns nil.
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("serveUntil returned %v, want nil", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serveUntil did not return within 10s of cancel")
	}
}

// An in-flight request has to be allowed to finish. This is the case a hard
// exit gets wrong: the client sees a short body and no error at all, which
// looks like a corrupt asset rather than a shutdown.
func TestServeFinishesInFlightRequestOnShutdown(t *testing.T) {
	const size = 512 * 1024

	started := make(chan struct{})
	release := make(chan struct{})
	// The handler stays in it after writing, so the test can observe whether
	// the shutdown waited. Dropping connections instead of draining returns
	// from serveUntil while the handler is still inside this.
	stillRunning := make(chan struct{})
	body := make([]byte, size)
	for i := range body {
		body[i] = 'x'
	}

	mux := handler(ServeOptions{OutDir: t.TempDir(), Base: "/"})
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/slow" {
				mux.ServeHTTP(w, r)
				return
			}
			close(started)
			<-release
			_, _ = w.Write(body)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			<-stillRunning
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ln, err := net.Listen("tcp", freeAddr(t))
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- serveUntil(ctx, srv, ln) }()
	waitForHealth(t, addr)

	type result struct {
		n   int
		err error
	}
	res := make(chan result, 1)
	go func() {
		resp, err := http.Get("http://" + addr + "/slow")
		if err != nil {
			res <- result{err: err}
			return
		}
		defer resp.Body.Close()
		got, err := io.ReadAll(resp.Body)
		res <- result{n: len(got), err: err}
	}()

	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("the slow handler never started")
	}
	cancel()
	close(release)

	// The guarantee: serveUntil is still blocked, because the handler has not
	// returned. A Close instead of a Shutdown returns immediately and truncates
	// the response, so this is the assertion that distinguishes them.
	select {
	case err := <-done:
		t.Fatalf("serveUntil returned (%v) while the handler was still running; "+
			"it must wait rather than drop the connection", err)
	case <-time.After(500 * time.Millisecond):
	}

	close(stillRunning)

	select {
	case r := <-res:
		if r.err != nil {
			t.Fatalf("in-flight request failed during shutdown: %v", r.err)
		}
		if r.n != size {
			t.Errorf("in-flight body was %d bytes, want the full %d", r.n, size)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the in-flight request never completed")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("serveUntil returned %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Error("serveUntil did not return after the handler finished")
	}
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

func waitForHealth(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + addr + "/healthz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("the server did not become healthy")
}

// The default bind is loopback. A wiki has no authentication, so the default
// must not put the bundle and any /repo/ mount on the network.
func TestServeDefaultAddrIsLoopback(t *testing.T) {
	if got := DefaultAddr; !loopback(got) {
		t.Errorf("DefaultAddr = %q, want a loopback address", got)
	}
}

func TestLoopbackDetection(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:8080", true},
		{"127.0.0.5:8080", true},
		{"localhost:8080", true},
		{"[::1]:8080", true},
		{"0.0.0.0:8080", false},
		{"[::]:8080", false},
		{"192.168.1.10:8080", false},
		{"10.0.0.4:9000", false},
		{"example.invalid:8080", false},
		{"", true}, // nothing to claim
		{"not an address", true},
	}
	for _, tc := range tests {
		if got := loopback(tc.addr); got != tc.want {
			t.Errorf("loopback(%q) = %v, want %v", tc.addr, got, tc.want)
		}
	}
}

// A non-loopback bind is a deliberate choice, so the startup banner states it
// rather than leaving it to be discovered by someone else on the network.
func TestAnnounceWarnsOnANonLoopbackBind(t *testing.T) {
	opts := ServeOptions{OutDir: "/tmp/out", Base: "/wiki/"}

	loop := announce(opts, "127.0.0.1:8080", "127.0.0.1:8080")
	if !strings.Contains(loop, "http://127.0.0.1:8080/wiki/") {
		t.Errorf("banner does not name the address it is serving: %q", loop)
	}
	if strings.Contains(loop, "not loopback") {
		t.Errorf("loopback bind warned anyway: %q", loop)
	}

	for _, addr := range []string{"0.0.0.0:8080", "192.168.1.10:8080", "[::]:8080"} {
		got := announce(opts, addr, "localhost:8080")
		if !strings.Contains(got, "not loopback") {
			t.Errorf("bind %s did not warn: %q", addr, got)
		}
		if !strings.Contains(got, "no authentication") {
			t.Errorf("bind %s warning does not say the wiki is unauthenticated: %q", addr, got)
		}
	}
}

// A gzip request gets a compressed body that decompresses to the bytes of the
// uncompressed response. The aggregate search index is the response this
// exists for, so it is the one the test names.
func TestServeGzipsCompressibleResponses(t *testing.T) {
	dir, index, page := gzipFixture(t)
	h := handler(ServeOptions{OutDir: dir, Base: "/wiki/"})

	for _, tc := range []struct {
		path string
		want string
	}{
		{"/wiki/search-index.json", index},
		{"/wiki/index.html", page},
	} {
		plain := serveGET(t, h, tc.path, nil)
		if enc := plain.Header().Get("Content-Encoding"); enc != "" {
			t.Errorf("GET %s without the header set Content-Encoding %q", tc.path, enc)
		}
		if got := plain.Body.String(); got != tc.want {
			t.Errorf("GET %s body changed without compression", tc.path)
		}

		gz := serveGET(t, h, tc.path, map[string]string{"Accept-Encoding": "gzip"})
		if enc := gz.Header().Get("Content-Encoding"); enc != "gzip" {
			t.Fatalf("GET %s with Accept-Encoding: gzip Content-Encoding = %q, want gzip", tc.path, enc)
		}
		if cl := gz.Header().Get("Content-Length"); cl != "" {
			t.Errorf("GET %s kept Content-Length %q beside a compressed body", tc.path, cl)
		}
		if got := gunzip(t, gz.Body.Bytes()); string(got) != tc.want {
			t.Errorf("GET %s gzip body does not decompress to the uncompressed bytes", tc.path)
		}
		if gz.Body.Len() >= len(tc.want) {
			t.Errorf("GET %s compressed body is %d bytes, not smaller than %d", tc.path, gz.Body.Len(), len(tc.want))
		}
	}
}

func TestServeDoesNotCompressWithoutAcceptEncoding(t *testing.T) {
	dir, index, _ := gzipFixture(t)
	h := handler(ServeOptions{OutDir: dir, Base: "/wiki/"})

	w := serveGET(t, h, "/wiki/search-index.json", nil)
	if enc := w.Header().Get("Content-Encoding"); enc != "" {
		t.Errorf("Content-Encoding = %q without Accept-Encoding, want none", enc)
	}
	if w.Body.String() != index {
		t.Error("the body is not the raw file")
	}

	// An explicit gzip;q=0 turns compression off even though the header names
	// gzip.
	q0 := serveGET(t, h, "/wiki/search-index.json", map[string]string{"Accept-Encoding": "gzip;q=0"})
	if enc := q0.Header().Get("Content-Encoding"); enc != "" {
		t.Errorf("Content-Encoding = %q for gzip;q=0, want none", enc)
	}
}

// A binary body is already compressed, and a range request would have to
// decompress a whole representation to answer a slice, so neither is gzipped.
func TestServeDoesNotCompressBinaryOrRange(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "wiki"))
	png := "\x89PNG\r\n\x1a\n" + strings.Repeat("binary data ", 200)
	mustWrite(t, filepath.Join(dir, "wiki", "logo.png"), png)
	mustWrite(t, filepath.Join(dir, "wiki", "search-index.json"), strings.Repeat(`{"a":1}`, 200))
	h := handler(ServeOptions{OutDir: dir, Base: "/wiki/"})

	img := serveGET(t, h, "/wiki/logo.png", map[string]string{"Accept-Encoding": "gzip"})
	if enc := img.Header().Get("Content-Encoding"); enc != "" {
		t.Errorf("image Content-Encoding = %q, want none", enc)
	}
	if img.Body.String() != png {
		t.Error("the image body was altered")
	}

	rng := serveGET(t, h, "/wiki/search-index.json", map[string]string{
		"Accept-Encoding": "gzip",
		"Range":           "bytes=0-9",
	})
	if enc := rng.Header().Get("Content-Encoding"); enc != "" {
		t.Errorf("range request Content-Encoding = %q, want none", enc)
	}
	if rng.Code != http.StatusPartialContent {
		t.Errorf("range request status = %d, want 206", rng.Code)
	}
}

// Both codings of one URL revalidate to 304, and each answers with its own tag
// so a cache cannot apply a compressed body's validator to a plain one.
func TestServeConditionalGzipVariants(t *testing.T) {
	dir, _, _ := gzipFixture(t)
	h := handler(ServeOptions{OutDir: dir, Base: "/wiki/"})
	const path = "/wiki/search-index.json"

	plainTag := serveGET(t, h, path, nil).Header().Get("ETag")
	gzTag := serveGET(t, h, path, map[string]string{"Accept-Encoding": "gzip"}).Header().Get("ETag")
	if plainTag == "" || gzTag == "" {
		t.Fatal("missing ETag on one of the variants")
	}
	if plainTag == gzTag {
		t.Errorf("the two variants share the ETag %q", plainTag)
	}

	got := serveGET(t, h, path, map[string]string{"If-None-Match": plainTag})
	if got.Code != http.StatusNotModified {
		t.Errorf("plain If-None-Match %q = %d, want 304", plainTag, got.Code)
	}
	if got.Header().Get("ETag") != plainTag {
		t.Errorf("plain 304 ETag = %q, want %q", got.Header().Get("ETag"), plainTag)
	}

	got = serveGET(t, h, path, map[string]string{
		"If-None-Match":   gzTag,
		"Accept-Encoding": "gzip",
	})
	if got.Code != http.StatusNotModified {
		t.Fatalf("gzip If-None-Match %q = %d, want 304", gzTag, got.Code)
	}
	if got.Header().Get("ETag") != gzTag {
		t.Errorf("gzip 304 ETag = %q, want %q", got.Header().Get("ETag"), gzTag)
	}
	if got.Body.Len() != 0 {
		t.Errorf("304 carried a %d byte body", got.Body.Len())
	}

	weak := serveGET(t, h, path, map[string]string{
		"If-None-Match":   "W/" + gzTag,
		"Accept-Encoding": "gzip",
	})
	if weak.Code != http.StatusNotModified {
		t.Errorf("weak gzip If-None-Match = %d, want 304", weak.Code)
	}
}

// TestServeNotModifiedCarriesTheCodingTag pins the 304's validator: it is the
// tag of the coding the server would use, not the one the request happened to
// name, so a client revalidating with the plain tag against a gzip request is
// answered with the gzip variant's tag and never a tag its own 200 would not
// carry.
func TestServeNotModifiedCarriesTheCodingTag(t *testing.T) {
	dir, _, _ := gzipFixture(t)
	h := handler(ServeOptions{OutDir: dir, Base: "/wiki/"})
	const path = "/wiki/search-index.json"

	plainTag := serveGET(t, h, path, nil).Header().Get("ETag")
	gzTag := serveGET(t, h, path, map[string]string{"Accept-Encoding": "gzip"}).Header().Get("ETag")
	if plainTag == "" || gzTag == "" || plainTag == gzTag {
		t.Fatalf("variant tags = %q, %q, want two distinct tags", plainTag, gzTag)
	}

	// The If-None-Match form: a gzip request holding the plain tag still
	// revalidates, and the 304 names the gzip variant, which is what its 200
	// carries.
	got := serveGET(t, h, path, map[string]string{
		"If-None-Match":   plainTag,
		"Accept-Encoding": "gzip",
	})
	if got.Code != http.StatusNotModified {
		t.Fatalf("gzip If-None-Match with the plain tag = %d, want 304", got.Code)
	}
	if tag := got.Header().Get("ETag"); tag != gzTag {
		t.Errorf("304 ETag = %q, want the gzip variant %q", tag, gzTag)
	}

	// The If-Modified-Since form takes the same path through the file server,
	// and its validator is still the coding's rather than the file's plain tag.
	lm := serveGET(t, h, path, nil).Header().Get("Last-Modified")
	if lm == "" {
		t.Fatal("no Last-Modified on the plain 200")
	}
	ims := serveGET(t, h, path, map[string]string{
		"If-Modified-Since": lm,
		"Accept-Encoding":   "gzip",
	})
	if ims.Code != http.StatusNotModified {
		t.Fatalf("If-Modified-Since gzip = %d, want 304", ims.Code)
	}
	if tag := ims.Header().Get("ETag"); tag != gzTag {
		t.Errorf("If-Modified-Since 304 ETag = %q, want the gzip variant %q", tag, gzTag)
	}

	// A plain-coded exchange keeps the plain tag, so the two codings never
	// share a validator.
	plain := serveGET(t, h, path, map[string]string{"If-None-Match": plainTag})
	if plain.Code != http.StatusNotModified || plain.Header().Get("ETag") != plainTag {
		t.Errorf("plain 304 = %d ETag %q, want 304 with %q", plain.Code, plain.Header().Get("ETag"), plainTag)
	}

	// A non-compressible body is a plain exchange even when gzip is accepted,
	// so its 304 keeps the plain tag too.
	mustWrite(t, filepath.Join(dir, "wiki", "logo.png"), "\x89PNG\r\n\x1a\n"+strings.Repeat("bits ", 400))
	pngTag := serveGET(t, h, "/wiki/logo.png", nil).Header().Get("ETag")
	png := serveGET(t, h, "/wiki/logo.png", map[string]string{
		"If-None-Match":   pngTag,
		"Accept-Encoding": "gzip",
	})
	if png.Code != http.StatusNotModified || png.Header().Get("ETag") != pngTag {
		t.Errorf("image 304 = %d ETag %q, want 304 with the plain tag %q", png.Code, png.Header().Get("ETag"), pngTag)
	}
}

// TestServeGzipQualitySelection pins the content-coding choice: the two
// codings that matter are gzip and identity, and an explicit gzip entry wins
// over the wildcard, including a q=0 that refuses gzip.
func TestServeGzipQualitySelection(t *testing.T) {
	dir, _, _ := gzipFixture(t)
	h := handler(ServeOptions{OutDir: dir, Base: "/wiki/"})

	for _, tc := range []struct {
		header string
		want   bool
	}{
		{"gzip", true},
		{"", false},
		{"identity", false},
		{"br", false},
		{"gzip, deflate, br", true},
		{"gzip;q=0", false},
		{"gzip;q=0, *", false}, // the explicit refusal beats the wildcard
		{"gzip;q=0, *;q=1", false},
		{"*", true},
		{"*;q=0", false},
		{"*;q=0, gzip;q=1", true}, // the explicit entry beats a wildcard refusal
		{"gzip;q=0.5, identity;q=0", true},
	} {
		w := serveGET(t, h, "/wiki/search-index.json", map[string]string{"Accept-Encoding": tc.header})
		got := w.Header().Get("Content-Encoding") == "gzip"
		if got != tc.want {
			t.Errorf("Accept-Encoding %q gzipped = %v, want %v", tc.header, got, tc.want)
		}
	}
}

// TestServeSmallBodyStaysUncompressed pins the size floor: a body smaller than
// the floor is served as it is, since gzip's framing would grow a tiny
// response, while a larger one still compresses.
func TestServeSmallBodyStaysUncompressed(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "wiki"))
	small := `{"a":"tiny"}`
	big := strings.Repeat(`{"key":"value"}`, 200)
	if len(small) >= minGzipBytes || len(big) < minGzipBytes {
		t.Fatalf("fixture sizes small=%d big=%d do not straddle the %d floor", len(small), len(big), minGzipBytes)
	}
	mustWrite(t, filepath.Join(dir, "wiki", "small.json"), small)
	mustWrite(t, filepath.Join(dir, "wiki", "big.json"), big)
	h := handler(ServeOptions{OutDir: dir, Base: "/wiki/"})

	sw := serveGET(t, h, "/wiki/small.json", map[string]string{"Accept-Encoding": "gzip"})
	if enc := sw.Header().Get("Content-Encoding"); enc != "" {
		t.Errorf("a %d byte body was gzip-coded (%q), which grows it", len(small), enc)
	}
	if cl := sw.Header().Get("Content-Length"); cl != "" && cl != strconv.Itoa(len(small)) {
		t.Errorf("small body Content-Length = %q, want %d or none", cl, len(small))
	}
	if sw.Body.String() != small {
		t.Error("the small body was altered")
	}
	if !strings.Contains(sw.Header().Get("Vary"), "Accept-Encoding") {
		t.Errorf("small body Vary = %q, want Accept-Encoding", sw.Header().Get("Vary"))
	}

	bw := serveGET(t, h, "/wiki/big.json", map[string]string{"Accept-Encoding": "gzip"})
	if enc := bw.Header().Get("Content-Encoding"); enc != "gzip" {
		t.Errorf("a %d byte body is not gzip-coded: %q", len(big), enc)
	}
	if got := gunzip(t, bw.Body.Bytes()); string(got) != big {
		t.Error("the compressed large body does not decompress to the file")
	}
	if !strings.Contains(bw.Header().Get("Vary"), "Accept-Encoding") {
		t.Errorf("large body Vary = %q, want Accept-Encoding", bw.Header().Get("Vary"))
	}
}

func TestServeVaryHeaderOnBothVariants(t *testing.T) {
	dir, _, _ := gzipFixture(t)
	h := handler(ServeOptions{OutDir: dir, Base: "/wiki/"})
	for _, headers := range []map[string]string{nil, {"Accept-Encoding": "gzip"}} {
		w := serveGET(t, h, "/wiki/search-index.json", headers)
		if !strings.Contains(w.Header().Get("Vary"), "Accept-Encoding") {
			t.Errorf("Vary = %q for headers %v, want Accept-Encoding", w.Header().Get("Vary"), headers)
		}
	}
}

// gzipFixture writes a repetitive index and page, so a compressed body is
// reliably smaller than the file and the decompressed bytes can be compared.
func gzipFixture(t *testing.T) (dir, index, page string) {
	t.Helper()
	dir = t.TempDir()
	mustMkdir(t, filepath.Join(dir, "wiki"))
	index = strings.Repeat(`{"title":"a page","text":"lorem ipsum dolor sit amet"}`, 80)
	page = "<html><body><h1>A page</h1><p>" + strings.Repeat("lorem ipsum ", 200) + "</p></body></html>"
	mustWrite(t, filepath.Join(dir, "wiki", "search-index.json"), index)
	mustWrite(t, filepath.Join(dir, "wiki", "index.html"), page)
	return dir, index, page
}

func serveGET(t *testing.T, h http.Handler, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func gunzip(t *testing.T, b []byte) []byte {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("gzip body does not decode: %v", err)
	}
	defer zr.Close()
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("reading gzip body: %v", err)
	}
	return out
}
