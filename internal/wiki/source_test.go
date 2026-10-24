package wiki

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTTPSourceAndFederator(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "# Home\n\n## Install\n\nThe install guide starts here.\n")
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != "Bearer secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		http.FileServer(http.Dir(out)).ServeHTTP(w, req)
	}))
	defer server.Close()

	bad, err := NewHTTPSource(server.URL+"/wiki/", "")
	if err != nil {
		t.Fatal(err)
	}
	src, err := LoadAgentSource("docs", bad.BasePath(), bad.Read)
	if err == nil {
		t.Fatal("a source without the bearer token loaded")
	}
	_ = src

	hs, err := NewHTTPSource(server.URL+"/wiki", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if hs.BaseURL() != server.URL+"/wiki/" {
		t.Errorf("canonical base = %q", hs.BaseURL())
	}
	src, err = LoadAgentSource("docs", hs.BasePath(), hs.Read)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := src.Corpus.Search("install", SearchOptions{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Total != 1 || resp.Results[0].PageID != "page:index" {
		t.Errorf("HTTP search = %+v", resp)
	}
	src.Base = hs.BaseURL()
	fed := NewFederator([]*AgentSource{src})
	merged, err := fed.Search("install", SearchOptions{Limit: 5}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(merged.Results[0].URL, server.URL+"/wiki/") || !strings.HasPrefix(merged.Results[0].MarkdownURL, server.URL+"/wiki/") {
		t.Errorf("remote URLs are not absolute: %+v", merged.Results[0])
	}
	page, err := src.Corpus.ReadPage("page:index", 100)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page.Content, "install guide") {
		t.Errorf("HTTP page = %+v", page)
	}

	if merged.Results[0].Source != "docs" {
		t.Errorf("federated source = %q", merged.Results[0].Source)
	}
}

func TestFederatorNamespacesTwoSources(t *testing.T) {
	var sources []*AgentSource
	for _, name := range []string{"docs", "ops"} {
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "index.md"), "# "+name+"\n\n## Install\n\n"+name+" install guide.\n")
		r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: name}})
		if err != nil {
			t.Fatal(err)
		}
		out := t.TempDir()
		if err := r.RenderAll(out); err != nil {
			t.Fatal(err)
		}
		src, err := LoadAgentSource(name, "/wiki/", func(file string) ([]byte, error) {
			return os.ReadFile(filepath.Join(out, "wiki", file))
		})
		if err != nil {
			t.Fatal(err)
		}
		sources = append(sources, src)
	}
	fed := NewFederator(sources)
	resp, err := fed.Search("install", SearchOptions{Limit: 10}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Total != 2 {
		t.Fatalf("federated total = %d, want 2", resp.Total)
	}
	seen := map[string]bool{}
	for _, result := range resp.Results {
		seen[result.PageID] = true
		seen[result.Source] = true
	}
	for _, want := range []string{"docs:page:index", "ops:page:index", "docs", "ops"} {
		if !seen[want] {
			t.Errorf("federated result is missing %q: %+v", want, resp.Results)
		}
	}
	context, err := fed.ContextFor("install", SearchOptions{Limit: 10}, 120, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(context.Results) == 0 || context.Results[0].Source == "" || context.Results[0].Text == "" {
		t.Errorf("federated context = %+v", context)
	}
	page, err := fed.ReadPage("docs", "docs:page:index", 100)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page.Content, "docs install guide") {
		t.Errorf("federated page = %+v", page)
	}
}

func TestHTTPSourceMountedUnderPathPrefix(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "# Home\n\nMounted page.\n")
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.StripPrefix("/mount", http.FileServer(http.Dir(out))))
	defer server.Close()
	hs, err := NewHTTPSource(server.URL+"/mount/wiki", "")
	if err != nil {
		t.Fatal(err)
	}
	src, err := LoadAgentSource("docs", hs.BasePath(), hs.Read)
	if err != nil {
		t.Fatal(err)
	}
	src.Base = hs.BaseURL()
	fed := NewFederator([]*AgentSource{src})
	resp, err := fed.Search("mounted", SearchOptions{Limit: 5}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("mounted search returned no results")
	}
	want := server.URL + "/mount/wiki/index.html"
	if resp.Results[0].URL != want {
		t.Errorf("mounted URL = %q, want %q", resp.Results[0].URL, want)
	}
	page, err := fed.ReadPage("", "page:index", 100)
	if err != nil {
		t.Fatal(err)
	}
	if page.URL != want || !strings.Contains(page.Content, "Mounted page") {
		t.Errorf("mounted page = %+v", page)
	}
}

func TestFederatorContextWithoutManifest(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "# Home\n\n## Install\n\nThe install guide starts here.\n")
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	site := filepath.Join(out, "wiki")
	if err := os.Remove(filepath.Join(site, "pages.json")); err != nil {
		t.Fatal(err)
	}
	src, err := LoadAgentSource("docs", "/wiki/", func(name string) ([]byte, error) {
		return os.ReadFile(filepath.Join(site, name))
	})
	if err != nil {
		t.Fatal(err)
	}
	fed := NewFederator([]*AgentSource{src})
	resp, err := fed.ContextFor("install", SearchOptions{Limit: 5}, 500, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) == 0 || resp.Results[0].Text == "" {
		t.Errorf("context without pages.json = %+v", resp)
	}
}
