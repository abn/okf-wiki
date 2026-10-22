package wiki

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// offlineBundle has a nested page, a fragment link, a table of contents, a
// footnote and an image: enough to exercise every URL shape the offline mode
// changes.
func offlineBundle(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "guide", "setup"))
	mustMkdir(t, filepath.Join(dir, "assets"))
	mustWrite(t, filepath.Join(dir, "index.md"), `---
title: Home
tags: [guide]
---
# Home

[Topic](guide/topic.md#details)

[Jump](#top)

![Mark](assets/mark.svg)

## Top
`)
	mustWrite(t, filepath.Join(dir, "guide", "topic.md"), `---
title: Topic
---
# Topic

## Details

[Home](../index.md)

[Local](#details)

[Deep](setup/first.md)

Footnote.[^1]

[^1]: The note.
`)
	mustWrite(t, filepath.Join(dir, "guide", "setup", "first.md"), `---
title: First
---
# First

## Step
`)
	mustWrite(t, filepath.Join(dir, "assets", "mark.svg"),
		`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"></svg>`)
	return dir
}

func readRendered(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestOfflineSingleBundle covers the URL layer for a bundle opened from disk:
// every asset and page URL is relative to a per-page <base>, home links name
// index.html, same-page fragments name the page, and the graph is inline.
func TestOfflineSingleBundle(t *testing.T) {
	dir := offlineBundle(t)
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}

	index := readRendered(t, filepath.Join(out, "wiki", "index.html"))
	for _, want := range []string{
		`<base href="./">`,
		`href="./tokens.css`,
		`href="./guide/topic.html#details"`,
		`href="index.html#top"`,
		`href="index.html#main"`,
		`src="./assets/mark.svg"`,
		`href="./index.html"`,
		`href="./graph.html"`,
	} {
		if !strings.Contains(index, want) {
			t.Errorf("offline index is missing %q", want)
		}
	}
	if strings.Contains(index, `href="/wiki/`) {
		t.Error("the offline index still has an absolute base URL")
	}

	topic := readRendered(t, filepath.Join(out, "wiki", "guide", "topic.html"))
	for _, want := range []string{
		`<base href="../">`,
		`href="guide/topic.html#main"`,
		`href="guide/topic.html#details"`,
		`href="guide/topic.html#fn:1"`,
		`href="./index.html"`,
	} {
		if !strings.Contains(topic, want) {
			t.Errorf("offline topic page is missing %q", want)
		}
	}

	nested := readRendered(t, filepath.Join(out, "wiki", "guide", "setup", "first.html"))
	for _, want := range []string{
		`<base href="../../">`,
		`href="./tokens.css`,
	} {
		if !strings.Contains(nested, want) {
			t.Errorf("offline depth-two page is missing %q", want)
		}
	}

	tag := readRendered(t, filepath.Join(out, "wiki", "tags", "guide.html"))
	for _, want := range []string{
		`<base href="../">`,
		`href="tags/guide.html#main"`,
		`href="./guide/topic.html"`,
		`href="./tags/guide.html"`,
	} {
		if !strings.Contains(tag, want) {
			t.Errorf("offline tag page is missing %q", want)
		}
	}

	redirect := readRendered(t, filepath.Join(out, "index.html"))
	if !strings.Contains(redirect, `url=wiki/index.html`) {
		t.Error("the root redirect does not point at the wiki index file")
	}

	graphPage := readRendered(t, filepath.Join(out, "wiki", "graph.html"))
	for _, want := range []string{
		`<base href="./">`,
		`data-graph-base="./"`,
		`href="./graph.json"`,
		`id="graphData"`,
	} {
		if !strings.Contains(graphPage, want) {
			t.Errorf("offline graph page is missing %q", want)
		}
	}

	var graph GraphIndex
	raw := readRendered(t, filepath.Join(out, "wiki", "graph.json"))
	if err := json.Unmarshal([]byte(raw), &graph); err != nil {
		t.Fatal(err)
	}
	for _, n := range graph.Nodes {
		if n.ID == "page:guide/topic" && n.URL != "./guide/topic.html" {
			t.Errorf("offline graph node URL = %q, want ./guide/topic.html", n.URL)
		}
	}

	dataFiles := map[string]string{
		"search-index.js":   "window.__WIKI_SEARCH_INDEX = ",
		"search-content.js": "window.__WIKI_SEARCH_CONTENT = ",
		"tags.js":           "window.__WIKI_TAGS = ",
	}
	for name, prefix := range dataFiles {
		body := readRendered(t, filepath.Join(out, "wiki", name))
		if !strings.HasPrefix(body, prefix) {
			t.Errorf("%s does not start with %q", name, prefix)
		}
	}
	if !strings.Contains(readRendered(t, filepath.Join(out, "wiki", "search-index.js")), "./guide/topic.html") {
		t.Error("the offline search index script has no relative page URL")
	}

	var search SearchIndex
	raw = readRendered(t, filepath.Join(out, "wiki", "search-index.json"))
	if err := json.Unmarshal([]byte(raw), &search); err != nil {
		t.Fatal(err)
	}
	for _, p := range search.Pages {
		if p.Title == "Topic" && p.URL != "./guide/topic.html" {
			t.Errorf("offline search page URL = %q, want ./guide/topic.html", p.URL)
		}
	}
}

// TestOfflineMultiSite keeps the same URL rules over a tree: a group page's
// assets sit at the tree root, a wiki's assets sit under the wiki, every node
// link names index.html, and the one graph view at the tree base carries the
// aggregate.
func TestOfflineMultiSite(t *testing.T) {
	dir := t.TempDir()
	for _, rel := range []string{"alpha", "beta", "group/gamma"} {
		mustMkdir(t, filepath.Join(dir, filepath.FromSlash(rel)))
		mustWrite(t, filepath.Join(dir, filepath.FromSlash(rel), "index.md"),
			"---\ntitle: "+rel+"\n---\n# "+rel+"\n")
	}
	mustWrite(t, filepath.Join(dir, "alpha", "index.md"),
		"---\ntitle: Alpha\ntags: [guide]\n---\n# Alpha\n")
	mustWrite(t, filepath.Join(dir, "alpha", "topic.md"), "---\ntitle: Topic\n---\n# Topic\n")

	out := t.TempDir()
	r, err := New(Config{
		Content: dir, Base: "/family/", Brand: Brand{Name: "t"},
		MultiSite: true, Offline: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(out); err != nil {
		t.Fatal(err)
	}

	root := readRendered(t, filepath.Join(out, "family", "index.html"))
	for _, want := range []string{
		`<base href="./">`,
		`href="alpha/index.html"`,
		`href="group/index.html"`,
		`href="./index.html"`,
		`href="./graph.html"`,
	} {
		if !strings.Contains(root, want) {
			t.Errorf("offline tree root is missing %q", want)
		}
	}

	group := readRendered(t, filepath.Join(out, "family", "group", "index.html"))
	for _, want := range []string{
		`<base href="../">`,
		`href="./tokens.css`,
		`href="group/gamma/index.html"`,
	} {
		if !strings.Contains(group, want) {
			t.Errorf("offline group page is missing %q", want)
		}
	}

	wiki := readRendered(t, filepath.Join(out, "family", "alpha", "index.html"))
	for _, want := range []string{
		`<base href="../">`,
		`href="alpha/tokens.css`,
		`href="alpha/topic.html"`,
		`href="./index.html"`,
		`href="./graph.html"`,
	} {
		if !strings.Contains(wiki, want) {
			t.Errorf("offline wiki page is missing %q", want)
		}
	}
	if strings.Contains(wiki, `href="/family/`) {
		t.Error("the offline wiki page still has an absolute base URL")
	}

	nested := readRendered(t, filepath.Join(out, "family", "group", "gamma", "index.html"))
	for _, want := range []string{
		`<base href="../../">`,
		`href="group/gamma/tokens.css`,
	} {
		if !strings.Contains(nested, want) {
			t.Errorf("offline nested wiki is missing %q", want)
		}
	}

	graphPage := readRendered(t, filepath.Join(out, "family", "graph.html"))
	if !strings.Contains(graphPage, `data-graph-base="./"`) ||
		!strings.Contains(graphPage, `id="graphData"`) {
		t.Error("the offline tree graph page is not an inline root view")
	}
	if _, err := os.Stat(filepath.Join(out, "family", "alpha", "graph.html")); err == nil {
		t.Error("an offline tree wiki kept its own graph page")
	}

	redirect := readRendered(t, filepath.Join(out, "index.html"))
	if !strings.Contains(redirect, `url=family/index.html`) {
		t.Error("the offline tree redirect does not point at the tree index file")
	}

	var graph GraphIndex
	raw := readRendered(t, filepath.Join(out, "family", "graph.json"))
	if err := json.Unmarshal([]byte(raw), &graph); err != nil {
		t.Fatal(err)
	}
	for _, n := range graph.Nodes {
		if n.ID == "alpha:page:index" && n.URL != "alpha/index.html" {
			t.Errorf("tree graph node URL = %q, want alpha/index.html", n.URL)
		}
	}

	var search SearchIndex
	raw = readRendered(t, filepath.Join(out, "family", "search-index.json"))
	if err := json.Unmarshal([]byte(raw), &search); err != nil {
		t.Fatal(err)
	}
	for _, p := range search.Pages {
		if p.Title == "Alpha" && p.URL != "alpha/index.html" {
			t.Errorf("tree search page URL = %q, want alpha/index.html", p.URL)
		}
	}
	tags := readRendered(t, filepath.Join(out, "family", "alpha", "tags.json"))
	if !strings.Contains(tags, `alpha/tags/guide.html`) {
		t.Errorf("offline wiki tags URL is not tree-relative: %s", tags)
	}
	for name, prefix := range map[string]string{
		"search-index.js":   "window.__WIKI_SEARCH_INDEX = ",
		"search-content.js": "window.__WIKI_SEARCH_CONTENT = ",
	} {
		body := readRendered(t, filepath.Join(out, "family", name))
		if !strings.HasPrefix(body, prefix) {
			t.Errorf("%s does not start with %q", name, prefix)
		}
		if name == "search-index.js" && !strings.Contains(body, "alpha/index.html") {
			t.Errorf("%s does not carry the tree-relative search data", name)
		}
	}
	if !strings.Contains(readRendered(t, filepath.Join(out, "family", "alpha", "tags.js")), "alpha/tags/guide.html") {
		t.Error("the offline wiki tags script is not tree-relative")
	}
	if _, err := os.Stat(filepath.Join(out, "family", "tags.js")); err != nil {
		t.Error("the offline tree base has no empty tags script")
	}
	err = filepath.WalkDir(out, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".html") && !strings.HasSuffix(path, ".json") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(body), `"/family/`) || strings.Contains(string(body), `'/family/`) {
			t.Errorf("%s still contains an absolute /family/ URL", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestOfflineNoAbsoluteURLs sweeps the generated HTML and JSON of a bundle for
// an absolute /wiki/ URL. A single missed src or link would otherwise pass the
// page-level assertions and fail only when opened from disk.
func TestOfflineNoAbsoluteURLs(t *testing.T) {
	dir := offlineBundle(t)
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	err = filepath.WalkDir(out, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".html") && !strings.HasSuffix(path, ".json") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(body), `"/wiki/`) || strings.Contains(string(body), `'/wiki/`) {
			t.Errorf("%s still contains an absolute /wiki/ URL", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestOnlinePageHasNoOfflineBase keeps the served site's URLs unchanged: no
// <base>, root-absolute assets, and a fragment-only skip link.
func TestOnlinePageHasNoOfflineBase(t *testing.T) {
	r := newGraphRenderer(t, offlineBundle(t))
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	page := readRendered(t, filepath.Join(out, "wiki", "index.html"))
	if strings.Contains(page, `<base href=`) {
		t.Error("an online page carries an offline <base>")
	}
	if !strings.Contains(page, `href="/wiki/tokens.css`) {
		t.Error("an online page lost its absolute asset base")
	}
	if !strings.Contains(page, `href="#main"`) {
		t.Error("an online page's skip link should stay fragment-only")
	}
	for _, name := range []string{"search-index.js", "search-content.js", "tags.js"} {
		if _, err := os.Stat(filepath.Join(out, "wiki", name)); err == nil {
			t.Errorf("an online render wrote %s", name)
		}
	}
}

func TestRewriteFragments(t *testing.T) {
	got := rewriteFragments(
		`<A title="1 > 0" HREF="#part"> <a href=#bare> <use href="#icon"> <area href = '#map'> <span data-href="#data"> <script>var s = '<a href="#script">';</script> <a href='#single'>`,
		"guide/topic.html")
	want := `<A title="1 > 0" HREF="guide/topic.html#part"> <a href=guide/topic.html#bare> <use href="#icon"> <area href = 'guide/topic.html#map'> <span data-href="#data"> <script>var s = '<a href="#script">';</script> <a href='guide/topic.html#single'>`
	if got != want {
		t.Errorf("rewriteFragments = %q, want %q", got, want)
	}
	if got := rewriteFragments(`<a href="#part">`, ""); got != `<a href="#part">` {
		t.Errorf("rewriteFragments online = %q", got)
	}
}

// TestOfflineBaseRoot covers a bundle rendered at the output root. There is no
// root redirect, and the home page's <base> is the same directory.
func TestOfflineBaseRoot(t *testing.T) {
	dir := offlineBundle(t)
	r, err := New(Config{Content: dir, Base: "/", Brand: Brand{Name: "t"}, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	index := readRendered(t, filepath.Join(out, "index.html"))
	for _, want := range []string{
		`<base href="./">`,
		`href="./tokens.css`,
		`href="./graph.html"`,
	} {
		if !strings.Contains(index, want) {
			t.Errorf("offline root page is missing %q", want)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "graph.html")); err != nil {
		t.Error("the graph page is not at the offline output root")
	}
}

func TestOfflineRejectsRepo(t *testing.T) {
	_, err := New(Config{
		Content: offlineBundle(t), Base: "/wiki/", Brand: Brand{Name: "t"},
		Offline: true, Repo: "/tmp/repo",
	})
	if err == nil || !strings.Contains(err.Error(), "repository mount") {
		t.Fatalf("New error = %v, want a repository mount rejection", err)
	}
}

// TestOfflineSearchDataContract pins the names shared by the Go writer and the
// browser client. A rename on one side would otherwise leave offline search
// silently empty.
func TestOfflineSearchDataContract(t *testing.T) {
	th, err := ResolveTheme("")
	if err != nil {
		t.Fatal(err)
	}
	var js string
	for _, f := range th.files {
		if f.Path == "search.js" {
			js = string(f.Data)
		}
	}
	if js == "" {
		t.Fatal("the default theme has no search.js")
	}
	for _, want := range []string{
		"window.__WIKI_OFFLINE",
		"__WIKI_SEARCH_INDEX",
		"'search-index.js'",
		"__WIKI_SEARCH_CONTENT",
		"'search-content.js'",
		"__WIKI_TAGS",
		"'tags.js'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("search.js is missing %q", want)
		}
	}

	dir := t.TempDir()
	if err := writeJSData(dir, "search-index.js", "__WIKI_SEARCH_INDEX", []byte(`{"pages":[]}`)); err != nil {
		t.Fatal(err)
	}
	body := readRendered(t, filepath.Join(dir, "search-index.js"))
	if body != "window.__WIKI_SEARCH_INDEX = {\"pages\":[]};\n" {
		t.Errorf("writeJSData = %q", body)
	}
}

// TestOfflinePageCarriesClassicMermaid pins the file:// diagram path: the page
// loads the classic bundle before diagrams.js, and does not ask for the ESM
// bundle, which cannot be imported from disk.
func TestOfflinePageCarriesClassicMermaid(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\n---\n# Home\n\n```mermaid\nflowchart LR\n  A --> B\n```\n")
	vendor := t.TempDir()
	mustWrite(t, filepath.Join(vendor, "mermaid-bundle.min.mjs"), "export default {};\n")
	mustWrite(t, filepath.Join(vendor, "mermaid-bundle.min.js"), "window.__WIKI_MERMAID = {};\n")
	mustWrite(t, filepath.Join(vendor, "entry.js"), "source\n")

	r, err := New(Config{
		Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"},
		Offline: true, VendorDir: vendor,
	})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	page := readRendered(t, filepath.Join(out, "wiki", "index.html"))
	for _, want := range []string{
		`src="./vendor/mermaid-bundle.min.js?v=`,
		`src="./diagrams.js?v=`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("offline diagram page is missing %q", want)
		}
	}
	if strings.Contains(page, "vendor/mermaid-bundle.min.mjs") {
		t.Error("the offline page asks for the ESM bundle")
	}
	if _, err := os.Stat(filepath.Join(out, "wiki", "vendor", "mermaid-bundle.min.js")); err != nil {
		t.Error("the classic vendor bundle was not copied")
	}
	if _, err := os.Stat(filepath.Join(out, "wiki", "vendor", "mermaid-bundle.min.mjs")); err == nil {
		t.Error("the offline render copied the unused ESM bundle")
	}
	if _, err := os.Stat(filepath.Join(out, "wiki", "vendor", "entry.js")); err == nil {
		t.Error("the vendor build entry was published")
	}
}

// TestOfflineMissingClassicMermaidIsAnError covers an older vendor directory
// with only the ESM build: an offline page would otherwise get a 404 where its
// diagrams should be.
func TestOfflineMissingClassicMermaidIsAnError(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\n---\n# Home\n\n```mermaid\nflowchart LR\n  A --> B\n```\n")
	vendor := t.TempDir()
	mustWrite(t, filepath.Join(vendor, "mermaid-bundle.min.mjs"), "export default {};\n")

	r, err := New(Config{
		Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"},
		Offline: true, VendorDir: vendor,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = r.RenderAll(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "mermaid-bundle.min.js") {
		t.Fatalf("RenderAll error = %v, want the missing classic bundle", err)
	}
}

// TestOfflineTreeMissingClassicMermaidIsFatal keeps a vendor configuration
// error from becoming a silently skipped wiki in a tree render.
func TestOfflineTreeMissingClassicMermaidIsFatal(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "alpha"))
	mustWrite(t, filepath.Join(dir, "alpha", "index.md"), "---\ntitle: Alpha\n---\n# Alpha\n\n```mermaid\nflowchart LR\n  A --> B\n```\n")
	vendor := t.TempDir()
	mustWrite(t, filepath.Join(vendor, "mermaid-bundle.min.mjs"), "export default {};\n")

	r, err := New(Config{
		Content: dir, Base: "/family/", Brand: Brand{Name: "t"},
		MultiSite: true, Offline: true, VendorDir: vendor,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.RenderSites(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "mermaid-bundle.min.js") {
		t.Fatalf("RenderSites error = %v, want the missing classic bundle", err)
	}
	if len(r.SkippedSites()) != 0 {
		t.Errorf("SkippedSites = %+v, want the runtime error to be fatal", r.SkippedSites())
	}
}
