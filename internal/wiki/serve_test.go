package wiki

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
