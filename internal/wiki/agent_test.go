package wiki

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readAgentManifest(t *testing.T, path string) AgentManifest {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m AgentManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return m
}

// TestAgentManifestSingle covers the discovery surface for one bundle: the
// page metadata, source hash, headings and Markdown link all come from the
// rendered page rather than a second copy of its text.
func TestAgentManifestSingle(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), `---
title: Home
description: Start here.
status: stable
tags: [guide]
stale_after: "2026-06-01"
generated:
  at: "2026-01-02"
---
# Home

## Setup

Install it.
`)
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t", Title: "Wiki"}})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	m := readAgentManifest(t, filepath.Join(out, "wiki", "pages.json"))
	if m.Version != agentVersion || len(m.Pages) != 1 {
		t.Fatalf("manifest = %+v, want version %d and one page", m, agentVersion)
	}
	p := m.Pages[0]
	if p.ID != "page:index" || p.Title != "Home" || p.Description != "Start here." {
		t.Errorf("page identity = %+v", p)
	}
	if p.URL != "/wiki/index.html" || p.MarkdownURL != "/wiki/index.md" {
		t.Errorf("page links = %q / %q", p.URL, p.MarkdownURL)
	}
	if p.Section != "Overview" || p.Status != "stable" || p.StaleAfter != "2026-06-01" || p.GeneratedAt != "2026-01-02" {
		t.Errorf("page metadata = %+v", p)
	}
	if len(p.Tags) != 1 || p.Tags[0] != "guide" {
		t.Errorf("page tags = %v", p.Tags)
	}
	if len(p.ContentHash) != 64 {
		t.Errorf("content hash = %q, want 64 hex characters", p.ContentHash)
	}
	if len(p.Headings) != 1 || p.Headings[0].ID != "setup" || p.Headings[0].Title != "Setup" {
		t.Errorf("headings = %+v", p.Headings)
	}
	llms, err := os.ReadFile(filepath.Join(out, "wiki", "llms.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# Wiki",
		"Machine-readable page list: [pages.json](pages.json).",
		"- [Home](/wiki/index.md): Start here.",
	} {
		if !strings.Contains(string(llms), want) {
			t.Errorf("llms.txt is missing %q:\n%s", want, llms)
		}
	}
}

// TestAgentManifestOffline keeps the links usable from a file:// render.
func TestAgentManifestOffline(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "# Home\n\nHello.\n")
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	m := readAgentManifest(t, filepath.Join(out, "wiki", "pages.json"))
	if len(m.Pages) != 1 || m.Pages[0].URL != "./index.html" || m.Pages[0].MarkdownURL != "./index.md" {
		t.Fatalf("offline page links = %+v", m.Pages)
	}
	llms, err := os.ReadFile(filepath.Join(out, "wiki", "llms.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(llms), "](./index.md)") {
		t.Errorf("offline llms.txt has no relative Markdown link:\n%s", llms)
	}
}

// TestAgentManifestMultiSite covers the one tree-level manifest: page IDs are
// namespaced and each page names the wiki it came from.
func TestAgentManifestMultiSite(t *testing.T) {
	dir := t.TempDir()
	for _, wiki := range []string{"alpha", "beta"} {
		mustMkdir(t, filepath.Join(dir, wiki))
		mustWrite(t, filepath.Join(dir, wiki, "index.md"),
			"---\ntitle: "+strings.ToUpper(wiki[:1])+wiki[1:]+"\n---\n# Home\n")
	}
	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "t"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(out); err != nil {
		t.Fatal(err)
	}
	m := readAgentManifest(t, filepath.Join(out, "family", "pages.json"))
	if len(m.Pages) != 2 {
		t.Fatalf("tree manifest has %d pages, want 2", len(m.Pages))
	}
	for _, p := range m.Pages {
		if !strings.Contains(p.ID, ":page:index") {
			t.Errorf("tree page ID is not namespaced: %q", p.ID)
		}
		if p.Site == "" || p.SiteKey == "" {
			t.Errorf("tree page has no site label: %+v", p)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "family", "alpha", "pages.json")); !os.IsNotExist(err) {
		t.Error("a wiki below the tree root wrote its own agent manifest")
	}
	llms, err := os.ReadFile(filepath.Join(out, "family", "llms.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(llms), "/family/alpha/index.md") || !strings.Contains(string(llms), "/family/beta/index.md") {
		t.Errorf("tree llms.txt does not link both wikis:\n%s", llms)
	}
}

// TestAgentCollision covers the reserved paths. A reader's file is never
// overwritten by the generated surface.
func TestAgentCollision(t *testing.T) {
	for _, name := range []string{"llms.txt", "pages.json", "changes.json", "health.json"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			mustWrite(t, filepath.Join(dir, "index.md"), "# Home\n")
			mustWrite(t, filepath.Join(dir, name), "owned by the bundle\n")
			r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
			if err != nil {
				t.Fatal(err)
			}
			err = r.RenderAll(t.TempDir())
			if err == nil || !strings.Contains(err.Error(), name) {
				t.Errorf("render error = %v, want a collision naming %s", err, name)
			}
		})
	}
}

// TestAgentLLMSEscapesDestinations covers filenames that are valid on disk but
// break Markdown link syntax without escaping.
func TestAgentLLMSEscapesDestinations(t *testing.T) {
	llms := agentLLMS("Wiki", "", []AgentPage{{
		Title:       "Spaced",
		MarkdownURL: "/wiki/my page.md",
	}})
	if !strings.Contains(llms, "(/wiki/my%20page.md)") {
		t.Errorf("llms.txt destination was not escaped:\n%s", llms)
	}
}

// TestAgentLLMSKeepsDuplicateSiteTitlesApart keys site headings by the stable
// path, so two wikis with the same display title still get their own group.
func TestAgentLLMSKeepsDuplicateSiteTitlesApart(t *testing.T) {
	llms := agentLLMS("Tree", "", []AgentPage{
		{Title: "A", Site: "Docs", SiteKey: "a", Section: "One", MarkdownURL: "/a/index.md"},
		{Title: "B", Site: "Docs", SiteKey: "b", Section: "Two", MarkdownURL: "/b/index.md"},
	})
	if strings.Count(llms, "## Docs") != 2 {
		t.Errorf("duplicate site titles were merged:\n%s", llms)
	}
}

// TestAgentManifestRootWiki covers a tree whose root is itself a wiki: it has
// no aggregate, so its own unnamespaced pair sits at the tree base.
func TestAgentManifestRootWiki(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Root\n---\n# Root\n")
	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(out); err != nil {
		t.Fatal(err)
	}
	m := readAgentManifest(t, filepath.Join(out, "wiki", "pages.json"))
	if len(m.Pages) != 1 || m.Pages[0].ID != "page:index" {
		t.Errorf("root wiki manifest = %+v", m.Pages)
	}
}

// TestAgentManifestOfflineTree keeps a tree's aggregate links relative to the
// render root, where the offline <base> points.
func TestAgentManifestOfflineTree(t *testing.T) {
	dir := t.TempDir()
	for _, wiki := range []string{"alpha", "beta"} {
		mustMkdir(t, filepath.Join(dir, wiki))
		mustWrite(t, filepath.Join(dir, wiki, "index.md"),
			"---\ntitle: "+strings.ToUpper(wiki[:1])+wiki[1:]+"\n---\n# Home\n")
	}
	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "t"}, MultiSite: true, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(out); err != nil {
		t.Fatal(err)
	}
	m := readAgentManifest(t, filepath.Join(out, "family", "pages.json"))
	if len(m.Pages) != 2 {
		t.Fatalf("offline tree manifest has %d pages", len(m.Pages))
	}
	for _, p := range m.Pages {
		if !strings.HasPrefix(p.URL, "alpha/") && !strings.HasPrefix(p.URL, "beta/") {
			t.Errorf("offline tree URL = %q", p.URL)
		}
		if !strings.HasSuffix(p.MarkdownURL, ".md") || strings.HasPrefix(p.MarkdownURL, "/") {
			t.Errorf("offline tree Markdown URL = %q", p.MarkdownURL)
		}
	}
}
