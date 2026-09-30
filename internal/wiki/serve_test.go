package wiki

import (
	"context"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The route table and the file server work end to end over a real listener.
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
