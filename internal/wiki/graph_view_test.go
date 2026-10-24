package wiki

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// graphViewBundle is a small bundle with a link and a tag, enough for the view
// tests to have a node, an edge, and a page to read the header from.
func graphViewBundle(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"),
		"---\ntitle: Home\ntags: [Guide]\n---\n# Home\n[Install](install.md)\n")
	mustWrite(t, filepath.Join(dir, "install.md"),
		"---\ntitle: Install\n---\n# Install\n[Home](index.md)\n")
	return dir
}

// inlineGraphCatalogue returns the catalogue embedded in a rendered graph page.
func inlineGraphCatalogue(t *testing.T, page string) string {
	t.Helper()
	const open = `<script type="application/json" id="graphData">`
	const close = `</script>`
	i := strings.Index(page, open)
	if i < 0 {
		t.Fatalf("the page has no inline catalogue:\n%s", page)
	}
	rest := page[i+len(open):]
	j := strings.Index(rest, close)
	if j < 0 {
		t.Fatalf("the inline catalogue has no closing script tag")
	}
	return rest[:j]
}

// TestGraphPageIsWritten covers the single bundle's view: one page at the base,
// the client asset, the header control on an ordinary page, and the full-width
// shell the view asks for. The catalogue itself is fetched, not embedded.
func TestGraphPageIsWritten(t *testing.T) {
	r := newGraphRenderer(t, graphViewBundle(t))
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(out, "wiki", "graph.html"))
	if err != nil {
		t.Fatal(err)
	}
	page := string(raw)
	for _, want := range []string{
		`id="graphApp"`,
		`data-graph-base="/wiki/"`,
		`id="graphSvg"`,
		`id="graphDownload"`,
		`id="graphFit"`,
		`id="graphClearFocus"`,
		`id="graphTags"`,
		`id="graphTagsSummary"`,
		`id="graphTagsMenu"`,
		`id="graphSections"`,
		`id="graphTagHubs"`,
		`id="graphSectionLayer"`,
		`id="graphList"`,
		`id="graphListStatus"`,
		`id="graphListNodes"`,
		`id="graphListEdges"`,
		`aria-label="List view"`,
		`href="/wiki/graph.json"`,
		`src="/wiki/graph.js?v=`,
		`content content-graph`,
		`prose prose-graph`,
		`aria-current="page"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("graph.html is missing %q", want)
		}
	}
	if strings.Contains(page, `id="graphData"`) {
		t.Error("graph.html embeds the catalogue without --graph-inline")
	}
	if strings.Contains(page, `id="sideDrawer"`) {
		t.Error("the graph page kept a sidebar, so the drawing is not full width")
	}
	if _, err := os.Stat(filepath.Join(out, "wiki", "graph.js")); err != nil {
		t.Errorf("the graph client was not written: %v", err)
	}

	home, err := os.ReadFile(filepath.Join(out, "wiki", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(home), `href="/wiki/graph.html"`) ||
		!strings.Contains(string(home), `aria-label="Knowledge graph"`) {
		t.Error("an ordinary page has no accessible graph control")
	}
}

// TestGraphViewLocalFocusContract pins the client hooks used by the page's
// Local graph link: a focus query, a neighborhood set, and a way back to the
// whole graph. The Firefox smoke test covers the behavior; this catches a rename in
// the generated page or the client.
func TestGraphViewLocalFocusContract(t *testing.T) {
	th, err := ResolveTheme("")
	if err != nil {
		t.Fatal(err)
	}
	var js string
	for _, f := range th.files {
		if f.Path == "graph.js" {
			js = string(f.Data)
		}
	}
	if js == "" {
		t.Fatal("the default theme has no graph.js")
	}
	for _, want := range []string{
		"URLSearchParams(window.location.search)",
		"function focusOn",
		"function clearFocus",
		"graphClearFocus",
		"Focused on",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("graph.js is missing %q", want)
		}
	}
}

// TestGraphViewStateContract pins the client hooks that keep the shareable
// dimensions in the URL. The Firefox smoke test covers the interaction.
func TestGraphViewStateContract(t *testing.T) {
	th, err := ResolveTheme("")
	if err != nil {
		t.Fatal(err)
	}
	var js string
	for _, f := range th.files {
		if f.Path == "graph.js" {
			js = string(f.Data)
		}
	}
	if js == "" {
		t.Fatal("the default theme has no graph.js")
	}
	for _, want := range []string{
		"function readURLState",
		"function syncURL",
		"window.addEventListener('popstate', applyURLState)",
		"history.pushState",
		"history.replaceState",
		"params.getAll('tag')",
		"params.append('tag'",
		"params.set('q'",
		"params.set('kind'",
		"function initTagFilter",
		"function tagSlug",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("graph.js is missing %q", want)
		}
	}
}

// TestGraphSectionsContract pins the view controls for collapsed tag hubs and
// the section grid. The drawing itself is exercised in the Firefox smoke test.
func TestGraphSectionsContract(t *testing.T) {
	th, err := ResolveTheme("")
	if err != nil {
		t.Fatal(err)
	}
	var js string
	for _, f := range th.files {
		if f.Path == "graph.js" {
			js = string(f.Data)
		}
	}
	if js == "" {
		t.Fatal("the default theme has no graph.js")
	}
	for _, want := range []string{
		"function layoutInitialClustered",
		"function clusterGroups",
		"function renderSections",
		"graph-section-box",
		"function buildVisibleGraph",
		"showTagHubs",
		"clusterSections",
		"graphTagHubs",
		"pageSectionKey",
		"sectionID",
		"\\x00tag-hubs",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("graph.js is missing %q", want)
		}
	}
}

// TestGraphListViewContract pins the client hooks for the accessible list. The
// Firefox smoke test covers selection and filtering.
func TestGraphListViewContract(t *testing.T) {
	th, err := ResolveTheme("")
	if err != nil {
		t.Fatal(err)
	}
	var js string
	for _, f := range th.files {
		if f.Path == "graph.js" {
			js = string(f.Data)
		}
	}
	if js == "" {
		t.Fatal("the default theme has no graph.js")
	}
	for _, want := range []string{
		"function renderList",
		"function updateListSelection",
		"graphListNodes",
		"graphListEdges",
		"aria-current",
		"function listEmpty",
		"match the current filters.",
		"This graph has no ",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("graph.js is missing %q", want)
		}
	}
}

// TestGraphDeterministicLayoutContract pins the seeded layout: the client has
// no ambient randomness, and a catalogue produces the same positions again.
func TestGraphDeterministicLayoutContract(t *testing.T) {
	th, err := ResolveTheme("")
	if err != nil {
		t.Fatal(err)
	}
	var js string
	for _, f := range th.files {
		if f.Path == "graph.js" {
			js = string(f.Data)
		}
	}
	if js == "" {
		t.Fatal("the default theme has no graph.js")
	}
	for _, want := range []string{
		"function layoutSeed",
		"function seededRandom",
		"function seedLayout",
		"layoutRandom()",
		"function layoutInitial() {\n    seedLayout();",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("graph.js is missing %q", want)
		}
	}
	if strings.Contains(js, "Math.random") {
		t.Error("graph.js still uses ambient randomness")
	}
}

// TestGraphSemanticZoomContract pins the label tiers: hubs label the map,
// ordinary pages join at the page-label scale, and a focus or filter asks for
// page names without waiting for the zoom.
func TestGraphSemanticZoomContract(t *testing.T) {
	th, err := ResolveTheme("")
	if err != nil {
		t.Fatal(err)
	}
	var js string
	for _, f := range th.files {
		if f.Path == "graph.js" {
			js = string(f.Data)
		}
	}
	if js == "" {
		t.Fatal("the default theme has no graph.js")
	}
	for _, want := range []string{
		"var PAGE_LABEL_SCALE",
		"function pageHubs",
		"function isLabelHub",
		"function labelTier",
		"function pagesMayLabel",
		"data-tier",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("graph.js is missing %q", want)
		}
	}
}

// TestGraphCrossWikiContract pins the client's cross-wiki treatment: an edge
// whose endpoints name different sites is drawn and listed as cross-wiki.
func TestGraphCrossWikiContract(t *testing.T) {
	th, err := ResolveTheme("")
	if err != nil {
		t.Fatal(err)
	}
	var js, css string
	for _, f := range th.files {
		switch f.Path {
		case "graph.js":
			js = string(f.Data)
		case "wiki.css":
			css = string(f.Data)
		}
	}
	for _, want := range []string{"cross-site", "cross-wiki", "siteKey"} {
		if !strings.Contains(js, want) {
			t.Errorf("graph.js is missing %q", want)
		}
	}
	if !strings.Contains(css, ".graph-edge.cross-site") {
		t.Error("wiki.css has no cross-wiki edge style")
	}
}

// TestGraphPathContract pins the path picker's markup and client hooks.
func TestGraphPathContract(t *testing.T) {
	r := newGraphRenderer(t, graphViewBundle(t))
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(out, "wiki", "graph.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"id=\"graphPath\"", "id=\"graphPathFrom\"", "id=\"graphPathTo\"", "id=\"graphPathFind\"", "id=\"graphPathClear\"", "id=\"graphPathStatus\""} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("graph.html is missing %s", want)
		}
	}
	th, err := ResolveTheme("")
	if err != nil {
		t.Fatal(err)
	}
	var js, css string
	for _, f := range th.files {
		switch f.Path {
		case "graph.js":
			js = string(f.Data)
		case "wiki.css":
			css = string(f.Data)
		}
	}
	for _, want := range []string{"function shortestPath", "function findPath", "has-path"} {
		if !strings.Contains(js, want) {
			t.Errorf("graph.js is missing %q", want)
		}
	}
	if !strings.Contains(css, ".graph-app.has-path") {
		t.Error("wiki.css has no path highlight rule")
	}
}

// TestGraphInlineEmbedsCatalogue covers inline mode: the page carries the same
// bytes the served site would fetch, so the client never calls fetch.
func TestGraphInlineEmbedsCatalogue(t *testing.T) {
	dir := graphViewBundle(t)
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}, InlineGraph: true})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}

	page, err := os.ReadFile(filepath.Join(out, "wiki", "graph.html"))
	if err != nil {
		t.Fatal(err)
	}
	embedded := inlineGraphCatalogue(t, string(page))
	if !json.Valid([]byte(embedded)) {
		t.Fatalf("the inline catalogue is not JSON: %s", embedded)
	}
	file, err := os.ReadFile(filepath.Join(out, "wiki", "graph.json"))
	if err != nil {
		t.Fatal(err)
	}
	if embedded != string(file) {
		t.Error("the inline catalogue differs from graph.json")
	}
}

// TestGraphInlineEscapesTitles covers the inline script boundary: a page title
// is data, so a closing script tag in it must not end the catalogue's element.
func TestGraphInlineEscapesTitles(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"),
		"---\ntitle: \"</script><img src=x onerror=alert(1)>\"\n---\n# Home\n")
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}, InlineGraph: true})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(out, "wiki", "graph.html"))
	if err != nil {
		t.Fatal(err)
	}
	page := string(raw)
	embedded := inlineGraphCatalogue(t, page)
	if strings.Contains(embedded, "<") {
		t.Errorf("the inline catalogue contains a raw angle bracket: %s", embedded)
	}
	if !json.Valid([]byte(embedded)) {
		t.Fatalf("the inline catalogue is not JSON: %s", embedded)
	}
	if strings.Contains(page, "</script><img") {
		t.Error("the page title closed the inline script element")
	}
}

// TestGraphPageCollisionFails covers the reserved path. A page or a static file
// that already owns graph.html is a configuration error, not something to
// overwrite silently.
func TestGraphPageCollisionFails(t *testing.T) {
	t.Run("markdown page", func(t *testing.T) {
		dir := graphViewBundle(t)
		mustWrite(t, filepath.Join(dir, "graph.md"), "---\ntitle: Notes\n---\n# Notes\n")
		r := newGraphRenderer(t, dir)
		err := r.RenderAll(t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "collides with graph.md") {
			t.Fatalf("render error = %v, want a graph.md collision", err)
		}
	})

	t.Run("static file", func(t *testing.T) {
		dir := graphViewBundle(t)
		mustWrite(t, filepath.Join(dir, "graph.html"), "<p>hand-written</p>\n")
		r := newGraphRenderer(t, dir)
		err := r.RenderAll(t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "collides with graph.html") {
			t.Fatalf("render error = %v, want a graph.html collision", err)
		}
	})
}

// TestMultiSiteGraphPageAtTreeBase covers the tree rule: one view at the base,
// linked from a wiki and from a group, fetching the aggregate catalogue. A wiki
// of the tree does not get a second view.
func TestMultiSiteGraphPageAtTreeBase(t *testing.T) {
	dir := t.TempDir()
	for _, rel := range []string{"alpha", "group/beta"} {
		mustMkdir(t, filepath.Join(dir, filepath.FromSlash(rel)))
		mustWrite(t, filepath.Join(dir, filepath.FromSlash(rel), "index.md"),
			"---\ntitle: Home\n---\n# Home\n[Child](child.md)\n")
		mustWrite(t, filepath.Join(dir, filepath.FromSlash(rel), "child.md"),
			"---\ntitle: Child\n---\n# Child\n[Home](index.md)\n")
	}

	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "okf-wiki"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(out); err != nil {
		t.Fatal(err)
	}

	page, err := os.ReadFile(filepath.Join(out, "family", "graph.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`data-graph-base="/family/"`,
		`href="/family/graph.json"`,
		`src="/family/graph.js?v=`,
		`aria-current="page"`,
	} {
		if !strings.Contains(string(page), want) {
			t.Errorf("the tree view is missing %q", want)
		}
	}
	for _, rel := range []string{"alpha", "group/beta"} {
		if _, err := os.Stat(filepath.Join(out, "family", filepath.FromSlash(rel), "graph.html")); err == nil {
			t.Errorf("%s kept a second graph page, want one at the tree base", rel)
		}
	}
	for _, rel := range []string{"alpha/index.html", "alpha/child.html", "group/index.html"} {
		raw, err := os.ReadFile(filepath.Join(out, "family", filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), `href="/family/graph.html"`) {
			t.Errorf("%s does not link to the tree view", rel)
		}
	}
}

// TestMultiSiteEmptyGraphPage covers a tree with no readable wiki: it still
// writes the view and an empty catalogue rather than a missing file.
func TestMultiSiteEmptyGraphPage(t *testing.T) {
	dir := t.TempDir()
	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "t"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(out); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(out, "family", "graph.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), `id="graphApp"`) {
		t.Error("the empty tree has no graph page")
	}
	raw, err := os.ReadFile(filepath.Join(out, "family", "graph.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g GraphIndex
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatalf("the empty catalogue is not a graph: %v", err)
	}
	if len(g.Nodes) != 0 || len(g.Edges) != 0 {
		t.Errorf("the empty tree catalogue has %d nodes and %d edges, want none", len(g.Nodes), len(g.Edges))
	}
}

// TestMultiSiteInlineGraphEmbedsAggregate covers the larger of the two inline
// cases: the tree page carries the aggregate once, with namespaced IDs, rather
// than a copy per wiki.
func TestMultiSiteInlineGraphEmbedsAggregate(t *testing.T) {
	dir := t.TempDir()
	for _, rel := range []string{"alpha", "beta"} {
		mustMkdir(t, filepath.Join(dir, rel))
		mustWrite(t, filepath.Join(dir, rel, "index.md"),
			"---\ntitle: Home\n---\n# Home\n")
	}
	out := t.TempDir()
	r, err := New(Config{
		Content: dir, Base: "/family/", Brand: Brand{Name: "okf-wiki"},
		MultiSite: true, InlineGraph: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(out); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(out, "family", "graph.html"))
	if err != nil {
		t.Fatal(err)
	}
	embedded := inlineGraphCatalogue(t, string(page))
	for _, want := range []string{"alpha:page:index", "beta:page:index"} {
		if !strings.Contains(embedded, want) {
			t.Errorf("the inline aggregate is missing %q", want)
		}
	}
	var g GraphIndex
	if err := json.Unmarshal([]byte(embedded), &g); err != nil {
		t.Fatalf("the inline aggregate is not a graph: %v", err)
	}
	if len(g.Nodes) != 2 {
		t.Errorf("the inline aggregate has %d nodes, want 2", len(g.Nodes))
	}
}

// TestMultiSiteRootWikiGraphPage covers a tree whose root is itself a wiki. The
// view sits at the base from the root's own render, and a root-level file that
// would take its path fails the render before the tree is built.
func TestMultiSiteRootWikiGraphPage(t *testing.T) {
	t.Run("written at the base", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\n---\n# Home\n")
		out := t.TempDir()
		r, err := New(Config{
			Content: dir, Base: "/family/", Brand: Brand{Name: "okf-wiki"},
			MultiSite: true, InlineGraph: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.RenderSites(out); err != nil {
			t.Fatal(err)
		}
		page, err := os.ReadFile(filepath.Join(out, "family", "graph.html"))
		if err != nil {
			t.Fatal(err)
		}
		embedded := inlineGraphCatalogue(t, string(page))
		if !strings.Contains(embedded, `"page:index"`) {
			t.Errorf("the root wiki's view does not carry its own graph: %s", embedded)
		}
		if strings.Contains(embedded, "alpha:page:index") {
			t.Error("the root wiki's view carries an aggregate it does not have")
		}
	})

	t.Run("collision fails before the tree", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "index.md"), "# Home\n")
		mustWrite(t, filepath.Join(dir, "graph.md"), "# Notes\n")
		r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "okf-wiki"}, MultiSite: true})
		if err != nil {
			t.Fatal(err)
		}
		_, err = r.RenderSites(t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "graph.md") {
			t.Fatalf("RenderSites error = %v, want the graph.md collision", err)
		}
	})
}
