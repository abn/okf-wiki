package wiki

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func connectionsBundle(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "guide"))
	mustWrite(t, filepath.Join(dir, "index.md"), `---
title: Home
tags: [guide]
---
# Home

[Topic](guide/topic.md)
`)
	mustWrite(t, filepath.Join(dir, "guide", "topic.md"), `---
title: Topic
tags: [guide]
---
# Topic

[Home](../index.md)

[About](../about.md)
`)
	mustWrite(t, filepath.Join(dir, "about.md"), `---
title: About
---
# About

## First

[Topic](guide/topic.md)

## Second

[Topic](guide/topic.md)
`)
	mustWrite(t, filepath.Join(dir, "orphan.md"), `---
title: Orphan
---
# Orphan
`)
	return dir
}

func readPage(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestConnectionsAndBacklinks covers the static panel: a page lists the pages
// that link to it, keeps the link context, and offers a focused graph.
func TestConnectionsAndBacklinks(t *testing.T) {
	r, err := New(Config{Content: connectionsBundle(t), Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}

	topic := readPage(t, filepath.Join(out, "wiki", "guide", "topic.html"))
	for _, want := range []string{
		`class="connections"`,
		`Linked from`,
		`class="connections-graph"`,
		`graph.html?focus=page`,
		`depth=1`,
	} {
		if !strings.Contains(topic, want) {
			t.Errorf("topic page is missing %q", want)
		}
	}
	start := strings.Index(topic, `<ul class="connections-list">`)
	end := -1
	if start >= 0 {
		if n := strings.Index(topic[start:], `</ul>`); n >= 0 {
			end = start + n
		}
	}
	if start < 0 || end < 0 {
		t.Fatal("the topic page has no connections list")
	}
	list := topic[start:end]
	if strings.Count(list, "<li>") != 3 {
		t.Errorf("topic backlink count = %d, want 3: %s", strings.Count(list, "<li>"), list)
	}
	for _, want := range []string{
		`href="/wiki/index.html"`,
		`>Home</a>`,
		`>About</a>`,
		`&ldquo;Topic&rdquo;`,
		`class="connections-heading">First</span>`,
		`class="connections-heading">Second</span>`,
		`href="/wiki/about.html#first"`,
		`href="/wiki/about.html#second"`,
	} {
		if !strings.Contains(list, want) {
			t.Errorf("topic connections list is missing %q", want)
		}
	}

	home := readPage(t, filepath.Join(out, "wiki", "index.html"))
	start = strings.Index(home, `<ul class="connections-list">`)
	end = -1
	if start >= 0 {
		if n := strings.Index(home[start:], `</ul>`); n >= 0 {
			end = start + n
		}
	}
	if start < 0 || end < 0 {
		t.Fatal("Home has no connections list")
	}
	if !strings.Contains(home[start:end], `href="/wiki/guide/topic.html"`) {
		t.Error("Home has no backlink to Topic")
	}
	if strings.Contains(home, "No pages link here yet.") {
		t.Error("Home is wrongly reported as an orphan")
	}

	orphan := readPage(t, filepath.Join(out, "wiki", "orphan.html"))
	if !strings.Contains(orphan, "No pages link here yet.") {
		t.Error("the orphan page has no empty-state message")
	}

	tag := readPage(t, filepath.Join(out, "wiki", "tags", "guide.html"))
	if strings.Contains(tag, `class="connections"`) {
		t.Error("a synthetic tag page should not carry a Connections panel")
	}
	graph := readPage(t, filepath.Join(out, "wiki", "graph.html"))
	if strings.Contains(graph, `class="connections"`) {
		t.Error("the graph page should not carry a Connections panel")
	}
}

// TestOfflineConnections keeps the panel's URLs relative and keeps the local
// graph link usable from a file:// page.
func TestOfflineConnections(t *testing.T) {
	r, err := New(Config{Content: connectionsBundle(t), Base: "/wiki/", Brand: Brand{Name: "t"}, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	topic := readPage(t, filepath.Join(out, "wiki", "guide", "topic.html"))
	for _, want := range []string{
		`href="./index.html"`,
		`href="./about.html"`,
		`href="./graph.html?focus=page`,
		`depth=1`,
	} {
		if !strings.Contains(topic, want) {
			t.Errorf("offline topic page is missing %q", want)
		}
	}
	if strings.Contains(topic, `href="/wiki/`) {
		t.Error("the offline connections panel kept an absolute link")
	}
}

// TestTreeConnectionsNamespacesTheLocalGraph covers the aggregate ID prefix for
// a wiki below the tree root and a root wiki that keeps unnamespaced IDs.
func TestTreeConnectionsNamespacesTheLocalGraph(t *testing.T) {
	dir := t.TempDir()
	for _, rel := range []string{"alpha", "beta"} {
		mustMkdir(t, filepath.Join(dir, rel))
		mustWrite(t, filepath.Join(dir, rel, "index.md"),
			"---\ntitle: "+rel+"\n---\n# "+rel+"\n[Topic](topic.md)\n")
		mustWrite(t, filepath.Join(dir, rel, "topic.md"),
			"---\ntitle: Topic\n---\n# Topic\n[Home](index.md)\n")
	}
	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "t"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(out); err != nil {
		t.Fatal(err)
	}
	var aggregate GraphIndex
	if err := json.Unmarshal([]byte(readPage(t, filepath.Join(out, "family", "graph.json"))), &aggregate); err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, n := range aggregate.Nodes {
		ids[n.ID] = true
	}
	for _, rel := range []string{"alpha", "beta"} {
		page := readPage(t, filepath.Join(out, "family", rel, "index.html"))
		id := rel + ":page:index"
		if !ids[id] {
			t.Errorf("the aggregate graph has no %s node", id)
		}
		if !strings.Contains(page, "focus="+url.QueryEscape(id)) {
			t.Errorf("%s local graph link does not name %s", rel, id)
		}
		if !strings.Contains(page, `href="/family/`+rel+`/topic.html"`) {
			t.Errorf("%s backlink lost its wiki path", rel)
		}
	}

	rootDir := t.TempDir()
	mustWrite(t, filepath.Join(rootDir, "index.md"), "---\ntitle: Home\n---\n# Home\n[Topic](topic.md)\n")
	mustWrite(t, filepath.Join(rootDir, "topic.md"), "---\ntitle: Topic\n---\n# Topic\n[Home](index.md)\n")
	rootOut := t.TempDir()
	root, err := New(Config{Content: rootDir, Base: "/family/", Brand: Brand{Name: "t"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := root.RenderSites(rootOut); err != nil {
		t.Fatal(err)
	}
	page := readPage(t, filepath.Join(rootOut, "family", "index.html"))
	if !strings.Contains(page, "focus="+url.QueryEscape("page:index")) {
		t.Error("the root wiki's local graph link should not be namespaced")
	}
	if !strings.Contains(page, `href="/family/topic.html"`) {
		t.Error("the root wiki backlink lost its path")
	}
}
