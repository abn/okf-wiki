package wiki

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func searchFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), `---
title: Home
description: Start here.
tags: [guide]
---
# Home

The install guide starts here.
`)
	mustMkdir(t, filepath.Join(dir, "guide"))
	mustWrite(t, filepath.Join(dir, "guide", "install.md"), `---
title: Install
description: Install the tool.
tags: [guide]
---
# Install

## Install

Run the installer. Install it again when the version changes.
`)
	mustWrite(t, filepath.Join(dir, "linebreak.md"), `---
title: Line break
---
# Line break

The first line
continues here.

[First](one.md)[Second](two.md)

- First item
- Second item
`)
	mustWrite(t, filepath.Join(dir, "reference.md"), `---
title: Reference
description: Flags and options.
tags: [reference]
---
# Reference

The install flags are listed here.
`)
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(out, "wiki")
}

// TestSearchQuery covers ranking, snippets, manifest joins and filters against
// a real rendered site rather than a hand-built index.
func TestSearchQuery(t *testing.T) {
	site := searchFixture(t)
	c, err := LoadSearchCorpus(site)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Search("install", SearchOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Total < 3 || len(resp.Results) < 3 {
		t.Fatalf("results = %+v, want at least three install hits", resp)
	}
	first := resp.Results[0]
	if first.Title != "Install" || first.PageID != "page:guide/install" {
		t.Errorf("top result = %+v, want the Install page", first)
	}
	if first.MarkdownURL != "/wiki/guide/install.md" || first.ContentHash == "" {
		t.Errorf("top result manifest fields = %+v", first)
	}
	if !strings.Contains(strings.ToLower(first.Snippet), "install") {
		t.Errorf("snippet = %q, want an install hit", first.Snippet)
	}

	limited, err := c.Search("install", SearchOptions{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if limited.Total != resp.Total || len(limited.Results) != 1 {
		t.Errorf("limited response = %+v, want total %d and one row", limited, resp.Total)
	}

	tagged, err := c.Search("install", SearchOptions{Limit: 10, Tag: "guide"})
	if err != nil {
		t.Fatal(err)
	}
	if tagged.Total != 2 {
		t.Errorf("tagged total = %d, want 2", tagged.Total)
	}
	for _, result := range tagged.Results {
		if result.PageID == "page:reference" {
			t.Errorf("tag filter kept the reference page: %+v", result)
		}
	}

	sectioned, err := c.Search("install", SearchOptions{Limit: 10, Section: "Guide"})
	if err != nil {
		t.Fatal(err)
	}
	if sectioned.Total != 1 || sectioned.Results[0].PageID != "page:guide/install" {
		t.Errorf("section filter = %+v, want only the guide page", sectioned)
	}

	soft, err := c.Search("first line continues", SearchOptions{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if soft.Total != 1 || soft.Results[0].PageID != "page:linebreak" {
		t.Errorf("soft line break search = %+v, want the line break page", soft)
	}
	links, err := c.Search("first second", SearchOptions{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if links.Total != 1 || links.Results[0].PageID != "page:linebreak" {
		t.Errorf("adjacent link search = %+v, want the line break page", links)
	}
	items, err := c.Search("first item second item", SearchOptions{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if items.Total != 1 || items.Results[0].PageID != "page:linebreak" {
		t.Errorf("list item search = %+v, want the line break page", items)
	}
}

// TestSearchCorpusRejectsMismatchedContent pins the safety rule: a content
// file from another render must not be attached by position.
func TestSearchCorpusRejectsMismatchedContent(t *testing.T) {
	site := searchFixture(t)
	raw, err := os.ReadFile(filepath.Join(site, "search-content.json"))
	if err != nil {
		t.Fatal(err)
	}
	var content SearchContent
	if err := json.Unmarshal(raw, &content); err != nil {
		t.Fatal(err)
	}
	content.Count++
	broken, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(site, "search-content.json"), broken, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSearchCorpus(site); err == nil {
		t.Error("mismatched search content was accepted")
	}
}

// TestSearchCorpusWithoutManifest keeps search usable on a site rendered before
// pages.json existed. Page citations fall back to the HTML URL, while tag and
// site filters fail with an explanation rather than returning silent zeros.
func TestSearchCorpusWithoutManifest(t *testing.T) {
	site := searchFixture(t)
	if err := os.Remove(filepath.Join(site, "pages.json")); err != nil {
		t.Fatal(err)
	}
	c, err := LoadSearchCorpus(site)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Search("install", SearchOptions{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Total == 0 || resp.Results[0].PageID != "" {
		t.Errorf("fallback search = %+v, want results without manifest IDs", resp)
	}
	if !strings.HasSuffix(resp.Results[0].MarkdownURL, ".md") {
		t.Errorf("fallback Markdown URL = %q", resp.Results[0].MarkdownURL)
	}
	if _, err := c.Search("install", SearchOptions{Tag: "guide"}); err == nil {
		t.Error("tag filtering succeeded without pages.json")
	}
	if _, err := c.Search("install", SearchOptions{Site: "wiki"}); err == nil {
		t.Error("site filtering succeeded on a single bundle")
	}
}

// TestSearchTreeAggregate covers namespaced page IDs and site filters against a
// real tree render, not a synthetic index.
func TestSearchTreeAggregate(t *testing.T) {
	dir := t.TempDir()
	for _, wiki := range []string{"alpha", "beta"} {
		mustMkdir(t, filepath.Join(dir, wiki))
		mustWrite(t, filepath.Join(dir, wiki, "index.md"),
			"---\ntitle: "+strings.ToUpper(wiki[:1])+wiki[1:]+"\n---\n# Home\n\nThe install guide.\n")
	}
	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "t"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(out); err != nil {
		t.Fatal(err)
	}
	c, err := LoadSearchCorpus(filepath.Join(out, "family"))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Search("install", SearchOptions{Limit: 5, Site: "beta"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Total != 1 || resp.Results[0].PageID != "beta:page:index" || resp.Results[0].SiteKey != "beta" {
		t.Errorf("tree search = %+v, want the beta page", resp)
	}
	if resp.Results[0].MarkdownURL != "/family/beta/index.md" {
		t.Errorf("tree Markdown URL = %q", resp.Results[0].MarkdownURL)
	}
}
