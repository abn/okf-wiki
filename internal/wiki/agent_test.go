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

// TestAgentManifestMultiSite covers the per-wiki agent surface of a tree and the
// aggregate that sits beside it: each wiki writes its own page table and page
// list with unprefixed IDs, and the tree base routes to them.
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
	for _, site := range []string{"alpha", "beta"} {
		own := readAgentManifest(t, filepath.Join(out, "family", site, "pages.json"))
		if len(own.Pages) != 1 || own.Pages[0].ID != "page:index" {
			t.Errorf("%s manifest = %+v, want one unprefixed page", site, own.Pages)
		}
		if want := "/family/" + site + "/index.html"; own.Pages[0].URL != want {
			t.Errorf("%s page URL = %q, want %q", site, own.Pages[0].URL, want)
		}
		llms, err := os.ReadFile(filepath.Join(out, "family", site, "llms.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(llms), "/family/"+site+"/index.md") {
			t.Errorf("%s llms.txt does not link its own page:\n%s", site, llms)
		}
		for _, name := range []string{"changes.json", "health.json"} {
			if _, err := os.Stat(filepath.Join(out, "family", site, name)); err != nil {
				t.Errorf("%s is missing %s: %v", site, name, err)
			}
		}
	}
	llms, err := os.ReadFile(filepath.Join(out, "family", "llms.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"(alpha/llms.txt)", "(beta/llms.txt)", "1 page"} {
		if !strings.Contains(string(llms), want) {
			t.Errorf("tree llms.txt is missing %q:\n%s", want, llms)
		}
	}
	// The tree file is a router: it names wikis, not their pages.
	if strings.Contains(string(llms), "index.md") {
		t.Errorf("tree llms.txt lists pages instead of wikis:\n%s", llms)
	}
}

// TestAgentPerSiteChanges covers changes.json for one wiki of a tree: a page
// that moves in alpha is reported against alpha, its sibling reports nothing,
// and the aggregate names the namespaced page.
func TestAgentPerSiteChanges(t *testing.T) {
	dir := t.TempDir()
	for _, site := range []string{"alpha", "beta"} {
		mustMkdir(t, filepath.Join(dir, site))
		mustWrite(t, filepath.Join(dir, site, "index.md"),
			"---\ntitle: "+site+"\n---\n# Home\n\nOne.\n")
	}
	out := t.TempDir()
	render := func() {
		t.Helper()
		r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "t"}, MultiSite: true})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.RenderSites(out); err != nil {
			t.Fatal(err)
		}
	}
	render()
	mustWrite(t, filepath.Join(dir, "alpha", "index.md"),
		"---\ntitle: alpha\n---\n# Home\n\nTwo.\n")
	render()

	alpha, err := LoadAgentChanges(filepath.Join(out, "family", "alpha"))
	if err != nil {
		t.Fatal(err)
	}
	if len(alpha.Changes) != 1 || alpha.Changes[0].ID != "page:index" || alpha.Changes[0].Kind != "changed" {
		t.Errorf("alpha changes = %+v, want one changed page:index", alpha.Changes)
	}
	beta, err := LoadAgentChanges(filepath.Join(out, "family", "beta"))
	if err != nil {
		t.Fatal(err)
	}
	if len(beta.Changes) != 0 {
		t.Errorf("beta changes = %+v, want none", beta.Changes)
	}
	tree, err := LoadAgentChanges(filepath.Join(out, "family"))
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Changes) != 1 || tree.Changes[0].ID != "alpha:page:index" {
		t.Errorf("tree changes = %+v, want one namespaced page", tree.Changes)
	}
}

// TestAgentNestedWikiFiles covers a wiki below a group directory: its own
// surface lands at its path, and the router names that path.
func TestAgentNestedWikiFiles(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "group", "gamma"))
	mustWrite(t, filepath.Join(dir, "group", "gamma", "index.md"), "---\ntitle: Gamma\n---\n# Gamma\n")
	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "t"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(out); err != nil {
		t.Fatal(err)
	}
	site := filepath.Join(out, "family", "group", "gamma")
	for _, name := range []string{"llms.txt", "pages.json", "changes.json", "health.json"} {
		if _, err := os.Stat(filepath.Join(site, name)); err != nil {
			t.Errorf("the nested wiki is missing %s: %v", name, err)
		}
	}
	m := readAgentManifest(t, filepath.Join(site, "pages.json"))
	if len(m.Pages) != 1 || m.Pages[0].URL != "/family/group/gamma/index.html" {
		t.Errorf("nested wiki manifest = %+v", m.Pages)
	}
	llms, err := os.ReadFile(filepath.Join(site, "llms.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(llms), "(/family/group/gamma/index.md)") {
		t.Errorf("nested wiki llms.txt does not link its own page:\n%s", llms)
	}
	tree, err := os.ReadFile(filepath.Join(out, "family", "llms.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(tree), "(group/gamma/llms.txt)") {
		t.Errorf("tree router does not name the nested wiki's path:\n%s", tree)
	}
}

// TestAgentCollisionInTree covers the reserved paths in a wiki of a tree: the
// per-wiki surface must not overwrite a bundle's own file either.
func TestAgentCollisionInTree(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "alpha"))
	mustWrite(t, filepath.Join(dir, "alpha", "index.md"), "# Alpha\n")
	mustWrite(t, filepath.Join(dir, "alpha", "llms.txt"), "owned by the bundle\n")
	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "t"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(t.TempDir()); err == nil || !strings.Contains(err.Error(), "llms.txt") {
		t.Errorf("render error = %v, want a collision naming llms.txt", err)
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

// TestAgentTreeLLMSRoutesToEachWiki covers the tree router: one line per wiki,
// a link to that wiki's own file, and the page count.
func TestAgentTreeLLMSRoutesToEachWiki(t *testing.T) {
	llms := agentTreeLLMS("Tree", "All the wikis.", []agentSiteLink{
		{Rel: "alpha", Title: "Alpha", Description: "The alpha wiki.", Pages: 1},
		{Rel: "my group/beta", Title: "Beta", Pages: 3},
	})
	for _, want := range []string{
		"# Tree",
		"> All the wikis.",
		"## Wikis",
		"- [Alpha](alpha/llms.txt): The alpha wiki. (1 page)",
		"- [Beta](my%20group/beta/llms.txt): 3 pages",
	} {
		if !strings.Contains(llms, want) {
			t.Errorf("tree llms.txt is missing %q:\n%s", want, llms)
		}
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
	// A wiki's own pair is resolved against the wiki directory, where a client
	// fetched it, not against the render root the aggregate uses.
	own := readAgentManifest(t, filepath.Join(out, "family", "alpha", "pages.json"))
	if len(own.Pages) != 1 || own.Pages[0].MarkdownURL != "./index.md" || own.Pages[0].URL != "./index.html" {
		t.Errorf("offline per-wiki links = %+v, want links from the wiki base", own.Pages)
	}
	llms, err := os.ReadFile(filepath.Join(out, "family", "alpha", "llms.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(llms), "](./index.md)") {
		t.Errorf("offline per-wiki llms.txt is not relative to its own base:\n%s", llms)
	}
}
