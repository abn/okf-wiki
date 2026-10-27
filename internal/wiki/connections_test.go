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
	mustWrite(t, filepath.Join(dir, "guide", "notes.md"), `---
title: Aardvark
---
# Aardvark

[Topic](topic.md)
`)
	mustWrite(t, filepath.Join(dir, "about.md"), `---
title: About
---
# About

[Topic](guide/topic.md)

## First

[Topic](guide/topic.md)

## Second

[Topic](guide/topic.md) and [the topic](guide/topic.md) again
`)
	mustWrite(t, filepath.Join(dir, "extra.md"), `---
title: Extra
---
# Extra

## Details

[Topic](guide/topic.md)
`)
	mustWrite(t, filepath.Join(dir, "single.md"), `---
title: Single
---
# Single

## Only

[Topic](guide/topic.md) and [again](guide/topic.md)
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

// connectionsList slices a rendered page down to the Connections list, whatever
// nested lists it carries.
func connectionsList(t *testing.T, page string) string {
	t.Helper()
	start := strings.Index(page, `<ul class="connections-list">`)
	if start < 0 {
		t.Fatal("the page has no connections list")
	}
	end := strings.Index(page[start:], `</section>`)
	if end < 0 {
		t.Fatal("the connections list is not closed")
	}
	return page[start : start+end]
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
	list := connectionsList(t, topic)
	if n := strings.Count(list, `<li class="connections-source">`); n != 5 {
		t.Errorf("topic source count = %d, want 5: %s", n, list)
	}
	// About links four times across three headings but must read as one source,
	// with those headings nested under it.
	if n := strings.Count(list, `class="connections-quote"`); n != 9 {
		t.Errorf("topic quote count = %d, want 9: %s", n, list)
	}
	if n := strings.Count(list, `class="connections-count"`); n != 1 {
		t.Errorf("topic count badge appears %d times, want 1: %s", n, list)
	}
	if n := strings.Count(list, `class="connections-places"`); n != 1 {
		t.Errorf("topic nested place lists = %d, want 1: %s", n, list)
	}
	for _, want := range []string{
		`>Home</a>`,
		`href="/wiki/index.html"`,
		`class="connections-source-title" href="/wiki/about.html">About</a>`,
		`class="connections-count">4 links in 3 places</span>`,
		`class="connections-place" href="/wiki/about.html">Top of page</a>`,
		`class="connections-place" href="/wiki/about.html#first">First</a>`,
		`class="connections-place" href="/wiki/about.html#second">Second</a>`,
		`class="connections-source-title" href="/wiki/extra.html#details">Extra</a>`,
		`class="connections-heading">Details</span>`,
		`&ldquo;Topic&rdquo;`,
		`&ldquo;the topic&rdquo;`,
	} {
		if !strings.Contains(list, want) {
			t.Errorf("topic connections list is missing %q", want)
		}
	}
	// A page that links twice from one heading needs no count and no nested
	// list, and keeps both texts on its single row.
	singleAt := strings.Index(list, `class="connections-source-title" href="/wiki/single.html#only">Single</a>`)
	if singleAt < 0 {
		t.Fatalf("the topic connections list has no Single source: %s", list)
	}
	single := list[singleAt:]
	rowEnd := strings.Index(single, `</div>`)
	if rowEnd < 0 {
		t.Fatalf("Single's row is not closed: %s", single)
	}
	single = single[:rowEnd]
	if n := strings.Count(single, `class="connections-quote"`); n != 2 {
		t.Errorf("Single carries %d quotes, want 2: %s", n, single)
	}
	first := strings.Index(list, "Top of page")
	second := strings.Index(list, "#first")
	third := strings.Index(list, "#second")
	if first < 0 || second < 0 || third < 0 || !(first < second && second < third) {
		t.Errorf("About's places are not in document order: %s", list)
	}
	// Sections follow the sidebar, so a guide page sorts after the root pages
	// even when its title comes first alphabetically.
	aardvark := strings.Index(list, `class="connections-source-title" href="/wiki/guide/notes.html">Aardvark</a>`)
	if aardvark < 0 || aardvark < singleAt {
		t.Errorf("the guide source is not after the root sources: %s", list)
	}

	home := readPage(t, filepath.Join(out, "wiki", "index.html"))
	if !strings.Contains(connectionsList(t, home), `href="/wiki/guide/topic.html"`) {
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
