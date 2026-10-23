package wiki

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// graphLinksFixture is a bundle with a page at the root, a section page, a
// nested page, a broken link, a self link, an external link, an asset link, a
// link that leaves the bundle, and repeated links under different headings. It
// is small enough to reason about and covers every way a Markdown link can or
// cannot become a graph edge.
func graphLinksFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "guide"))
	mustWrite(t, filepath.Join(dir, "index.md"), strings.Join([]string{
		"---", "title: Home", "tags: [Alpha, Home, \"!!!\"]", "---", "# Home",
		"[Alpha](alpha.md)",
		"[Alpha again](alpha.md)",
		"[Beta](guide/beta.md#usage)",
		"[Missing](guide/missing.md)",
		"[Self](index.md)",
		"[External](https://example.com/page.md)",
		"[Asset](pic.png)",
	}, "\n"))
	mustWrite(t, filepath.Join(dir, "alpha.md"), strings.Join([]string{
		"---", "title: Alpha", "type: Reference", "status: stable", "tags: [alpha]", "---",
		"# Alpha",
		"sentenceonlyinthebody",
		"## Back",
		"[Home](index.md)",
		"[Root beta](/guide/beta.md)",
		"## Again",
		"[Home again](index.md)",
	}, "\n"))
	mustWrite(t, filepath.Join(dir, "guide", "beta.md"), strings.Join([]string{
		"---", "title: Beta", "---", "# Beta",
		"## Usage",
		"[Alpha](../alpha.md)",
		"[Outside](../../outside.md)",
		"`[Fake](alpha.md)`",
	}, "\n"))
	return dir
}

func newGraphRenderer(t *testing.T, dir string) *Renderer {
	t.Helper()
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func graphEdges(g GraphIndex, from, to, kind string) []GraphEdge {
	var out []GraphEdge
	for _, e := range g.Edges {
		if e.From == from && e.To == to && e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

func graphNode(g GraphIndex, id string) (GraphNode, bool) {
	for _, n := range g.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return GraphNode{}, false
}

// TestBuildGraphRecordsSectionIDs keeps section identity separate from the
// display title, so a real directory named tags cannot collide with the view's
// synthetic tag-hub cell.
func TestBuildGraphRecordsSectionIDs(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "tags"))
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\ntags: [alpha]\n---\n# Home\n")
	mustWrite(t, filepath.Join(dir, "tags", "index.md"), "---\ntitle: Tags\n---\n# Tags\n")
	g := newGraphRenderer(t, dir).BuildGraph()
	root, ok := graphNode(g, "page:index")
	if !ok || root.SectionID != "" {
		t.Errorf("root section ID = %q, want empty", root.SectionID)
	}
	tags, ok := graphNode(g, "page:tags/index")
	if !ok || tags.Section != "Tags" || tags.SectionID != "tags" {
		t.Errorf("tags section = %+v, want title Tags and ID tags", tags)
	}
}

// TestBuildGraphRecordsPageLinks pins the edge set: in-bundle Markdown links
// become edges, a link repeated under one heading collapses to one edge while
// the same target under another heading stays a second edge, and a link to a
// page the bundle does not contain becomes a stub instead of a dead URL.
func TestBuildGraphRecordsPageLinks(t *testing.T) {
	g := newGraphRenderer(t, graphLinksFixture(t)).BuildGraph()
	if g.Version != graphVersion {
		t.Fatalf("graph version = %d, want %d", g.Version, graphVersion)
	}

	pages := map[string]GraphNode{}
	for _, n := range g.Nodes {
		if n.Kind == "page" {
			pages[n.ID] = n
		}
	}
	for _, id := range []string{
		"page:index", "page:alpha", "page:guide/beta", "page:guide/missing",
	} {
		if _, ok := pages[id]; !ok {
			t.Errorf("page %q is missing from the graph", id)
		}
	}
	for _, id := range []string{"pic.png", "https://example.com/page.md", "page:outside"} {
		if _, ok := pages[id]; ok {
			t.Errorf("%q became a node, want only in-bundle Markdown pages", id)
		}
	}

	alpha := pages["page:alpha"]
	if alpha.Title != "Alpha" || alpha.URL != "/wiki/alpha.html" {
		t.Errorf("alpha node = %+v, want its title and page URL", alpha)
	}
	if alpha.Type != "Reference" || alpha.Status != "stable" {
		t.Errorf("alpha node lost frontmatter: %+v", alpha)
	}
	stub := pages["page:guide/missing"]
	if !stub.Stub || stub.URL != "" || stub.Title != "missing" {
		t.Errorf("broken link node = %+v, want a URL-less stub", stub)
	}

	cases := []struct {
		from, to, anchor, text string
	}{
		{"page:index", "page:alpha", "", "Alpha"},
		{"page:index", "page:guide/beta", "", "Beta"},
		{"page:index", "page:guide/missing", "", "Missing"},
		{"page:alpha", "page:guide/beta", "back", "Root beta"},
		{"page:guide/beta", "page:alpha", "usage", "Alpha"},
	}
	for _, tc := range cases {
		edges := graphEdges(g, tc.from, tc.to, "link")
		if len(edges) != 1 {
			t.Errorf("%s -> %s has %d link edges, want 1", tc.from, tc.to, len(edges))
			continue
		}
		if edges[0].Anchor != tc.anchor || edges[0].Text != tc.text {
			t.Errorf("%s -> %s edge = %+v, want anchor %q and text %q",
				tc.from, tc.to, edges[0], tc.anchor, tc.text)
		}
	}
	// The same target under two headings is two edges, so the anchor stays part
	// of the dedupe key.
	again := graphEdges(g, "page:alpha", "page:index", "link")
	if len(again) != 2 || again[0].Anchor != "back" || again[1].Anchor != "again" {
		t.Errorf("alpha -> index edges = %+v, want one per heading", again)
	}
	if edges := graphEdges(g, "page:index", "page:index", "link"); len(edges) != 0 {
		t.Errorf("a self link became %d edges, want none", len(edges))
	}
	if edges := graphEdges(g, "page:guide/beta", "page:outside", "link"); len(edges) != 0 {
		t.Errorf("an out-of-bundle link became %d edges, want none", len(edges))
	}
	var links int
	for _, e := range g.Edges {
		if e.Kind == "link" {
			links++
		}
	}
	if links != len(cases)+2 {
		t.Errorf("graph has %d link edges, want %d", links, len(cases)+2)
	}
}

// TestBuildGraphAddsTagHubs covers the tag namespace: one node per tag slug,
// titled with the first spelling seen, linked from every page that carries it,
// and no node for a tag with no URL form.
func TestBuildGraphAddsTagHubs(t *testing.T) {
	g := newGraphRenderer(t, graphLinksFixture(t)).BuildGraph()

	alpha, ok := graphNode(g, "tag:alpha")
	if !ok {
		t.Fatal("tag:alpha is missing")
	}
	if alpha.Kind != "tag" || alpha.Title != "#Alpha" || alpha.URL != "/wiki/tags/alpha.html" {
		t.Errorf("tag node = %+v, want the first spelling and its tag page", alpha)
	}
	if _, ok := graphNode(g, "tag:home"); !ok {
		t.Error("tag:home is missing")
	}
	if _, ok := graphNode(g, "tag:"); ok {
		t.Error("a tag with no URL form became a node")
	}

	for _, from := range []string{"page:index", "page:alpha"} {
		if edges := graphEdges(g, from, "tag:alpha", "tag"); len(edges) != 1 {
			t.Errorf("%s -> tag:alpha has %d tag edges, want 1", from, len(edges))
		}
	}
	if edges := graphEdges(g, "page:index", "tag:home", "tag"); len(edges) != 1 {
		t.Errorf("index -> tag:home has %d tag edges, want 1", len(edges))
	}
}

// TestGraphSeparatesPageAndTagNamespaces pins the reason both node kinds carry
// a prefix: a file may be named tag:foo.md, and its page must not swallow the
// hub for a tag called foo.
func TestGraphSeparatesPageAndTagNamespaces(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"),
		"---\ntitle: Home\ntags: [foo]\n---\n# Home\n[Collision](tag:foo.md)\n")
	mustWrite(t, filepath.Join(dir, "tag:foo.md"),
		"---\ntitle: Collision Page\n---\n# Collision Page\n")

	g := newGraphRenderer(t, dir).BuildGraph()
	page, ok := graphNode(g, "page:tag:foo")
	if !ok || page.Kind != "page" || page.Title != "Collision Page" {
		t.Fatalf("page node = %+v, want the page named tag:foo", page)
	}
	tag, ok := graphNode(g, "tag:foo")
	if !ok || tag.Kind != "tag" || tag.Title != "#foo" {
		t.Fatalf("tag node = %+v, want the hub for foo", tag)
	}
	if edges := graphEdges(g, "page:index", "tag:foo", "tag"); len(edges) != 1 {
		t.Errorf("tag edge = %v, want one to the tag hub", edges)
	}
	if edges := graphEdges(g, "page:index", "page:tag:foo", "link"); len(edges) != 1 {
		t.Errorf("link edge = %v, want one to the page", edges)
	}
}

// TestGraphJSONCarriesNoPageText guards the split between the structural graph
// and any semantic layer: the catalogue names pages and relations, never their
// prose.
func TestGraphJSONCarriesNoPageText(t *testing.T) {
	raw, err := newGraphRenderer(t, graphLinksFixture(t)).GraphJSON()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "sentenceonlyinthebody") {
		t.Error("graph.json carries page text")
	}
	var g GraphIndex
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatalf("decode graph.json: %v", err)
	}
	if g.Version != graphVersion {
		t.Errorf("version = %d, want %d", g.Version, graphVersion)
	}
}

// TestRenderWritesGraph covers the single-bundle write point, beside the search
// catalogue, and the byte stability two renders of one bundle promise.
func TestRenderWritesGraph(t *testing.T) {
	r := newGraphRenderer(t, graphLinksFixture(t))
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(out, "wiki", "graph.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g GraphIndex
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	alpha, ok := graphNode(g, "page:alpha")
	if !ok || alpha.URL != "/wiki/alpha.html" {
		t.Errorf("rendered graph has no usable alpha node: %+v", alpha)
	}

	second := t.TempDir()
	if err := r.RenderAll(second); err != nil {
		t.Fatal(err)
	}
	again, err := os.ReadFile(filepath.Join(second, "wiki", "graph.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, again) {
		t.Error("two renders of one bundle produced different graph.json files")
	}
}

// TestMergeGraphNamespacesIDs pins the tree rule: identical slugs in two wikis
// stay distinct, edges move with their nodes, and a tag hub belongs to the wiki
// that uses it.
func TestMergeGraphNamespacesIDs(t *testing.T) {
	mk := func(t *testing.T, tag string) GraphIndex {
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "index.md"),
			"---\ntitle: Home\ntags: ["+tag+"]\n---\n# Home\n[Child](child.md)\n")
		mustWrite(t, filepath.Join(dir, "child.md"),
			"---\ntitle: Child\n---\n# Child\n[Home](index.md)\n")
		r := newGraphRenderer(t, dir)
		return r.BuildGraph()
	}
	a := mk(t, "One")
	b := mk(t, "Two")

	var merged GraphIndex
	mergeGraph(&merged, &a, "One", "one")
	mergeGraph(&merged, &b, "Two", "two")
	if merged.Version != graphVersion {
		t.Errorf("merged version = %d, want %d", merged.Version, graphVersion)
	}
	for _, id := range []string{
		"one:page:index", "one:page:child", "one:tag:one",
		"two:page:index", "two:page:child", "two:tag:two",
	} {
		if _, ok := graphNode(merged, id); !ok {
			t.Errorf("merged graph is missing %q", id)
		}
	}
	if edges := graphEdges(merged, "one:page:child", "one:page:index", "link"); len(edges) != 1 {
		t.Errorf("namespaced edge = %v, want one inside wiki one", edges)
	}
	if node, ok := graphNode(merged, "one:page:index"); ok && (node.Site != "One" || node.SiteKey != "one") {
		t.Errorf("merged node lost its wiki label: %+v", node)
	}
	var unlabelled GraphIndex
	mergeGraph(&unlabelled, &a, "One", "")
	if len(unlabelled.Nodes) != 0 {
		t.Error("a merge with no wiki key produced nodes")
	}
}

// TestGraphTarget pins the guard that keeps a rewritten link from outside this
// render's base out of the catalogue. The escaping-link fixture reaches this
// through the suffix check too, so this table isolates the prefix.
func TestGraphTarget(t *testing.T) {
	base := &Renderer{cfg: Config{Base: "/wiki/"}}
	cases := []struct {
		dest string
		slug string
		ok   bool
	}{
		{"/wiki/guide/install.html", "guide/install", true},
		{"/wiki/guide/install.html#setup", "guide/install", true},
		{"/repo/outside.html", "", false},
		{"https://example.com/page.html", "", false},
		{"/elsewhere/page.html", "", false},
		{"/wiki/", "", false},
		{"/wiki/../outside.html", "", false},
	}
	for _, tc := range cases {
		slug, ok := base.graphTarget(tc.dest)
		if slug != tc.slug || ok != tc.ok {
			t.Errorf("graphTarget(%q) = %q/%v, want %q/%v", tc.dest, slug, ok, tc.slug, tc.ok)
		}
	}

	root := &Renderer{cfg: Config{Base: "/"}}
	if slug, ok := root.graphTarget("/page.html"); !ok || slug != "page" {
		t.Errorf("root base graphTarget = %q/%v, want page/true", slug, ok)
	}
}

// TestMultiSiteWritesAggregateGraph covers the tree write points: each wiki
// keeps its own catalogue with unnamespaced IDs, and the tree base holds one
// aggregate whose IDs carry the wiki's path.
func TestMultiSiteWritesAggregateGraph(t *testing.T) {
	dir := t.TempDir()
	for _, wiki := range []string{"alpha", "beta"} {
		mustMkdir(t, filepath.Join(dir, wiki))
		mustWrite(t, filepath.Join(dir, wiki, ".meta.json"),
			`{"title": "`+strings.ToUpper(wiki[:1])+wiki[1:]+`"}`)
		mustWrite(t, filepath.Join(dir, wiki, "index.md"),
			"---\ntitle: Home\ntags: [Shared]\n---\n# Home\n[Child](child.md)\n")
		mustWrite(t, filepath.Join(dir, wiki, "child.md"),
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

	read := func(path string) GraphIndex {
		t.Helper()
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var g GraphIndex
		if err := json.Unmarshal(raw, &g); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		return g
	}

	own := read(filepath.Join(out, "family", "alpha", "graph.json"))
	if node, ok := graphNode(own, "page:child"); !ok || node.URL != "/family/alpha/child.html" {
		t.Errorf("wiki catalogue has no usable child node: %+v", node)
	}

	agg := read(filepath.Join(out, "family", "graph.json"))
	for _, id := range []string{
		"alpha:page:index", "alpha:page:child", "beta:page:index", "beta:page:child",
		"alpha:tag:shared", "beta:tag:shared",
	} {
		if _, ok := graphNode(agg, id); !ok {
			t.Errorf("aggregate is missing %q", id)
		}
	}
	if edges := graphEdges(agg, "alpha:page:child", "alpha:page:index", "link"); len(edges) != 1 {
		t.Errorf("aggregate edge = %v, want one inside alpha", edges)
	}
	if _, ok := graphNode(agg, "page:index"); ok {
		t.Error("the aggregate kept an unnamespaced node id")
	}
	node, _ := graphNode(agg, "alpha:page:index")
	if node.Site != "Alpha" || node.SiteKey != "alpha" {
		t.Errorf("aggregate node label = %q/%q, want Alpha/alpha", node.Site, node.SiteKey)
	}
}

// TestCrossWikiTarget pins which links leave a wiki: a relative Markdown link
// that climbs to a sibling is a cross-wiki target, while an in-wiki link, a
// root-relative link and a path above the tree are not.
func TestCrossWikiTarget(t *testing.T) {
	r := &Renderer{nested: true, siteRel: "alpha", treeWikis: map[string]bool{"alpha": true, "beta": true}}
	cases := []struct {
		from, href, want string
		ok               bool
	}{
		{"index", "../beta/index.md", "beta/index", true},
		{"guide/x", "../../beta/page.md", "beta/page", true},
		{"guide/x", "../../beta/page.md#setup", "beta/page", true},
		{"guide/x", "../../beta/page.md?view=1", "beta/page", true},
		{"guide/x", "../index.md", "", false},
		{"index", "../../outside.md", "", false},
		{"index", "/beta/page.md", "", false},
		{"index", "https://example.com/page.md", "", false},
		{"index", "../beta/image.png", "", false},
	}
	for _, tc := range cases {
		got, ok := r.crossWikiTarget(tc.from, tc.href)
		if got != tc.want || ok != tc.ok {
			t.Errorf("crossWikiTarget(%q, %q) = %q/%v, want %q/%v",
				tc.from, tc.href, got, ok, tc.want, tc.ok)
		}
	}
}

// TestSplitTreeTarget pins the wiki a tree-relative target belongs to. A
// nested wiki key wins over a group path that merely shares its prefix.
func TestSplitTreeTarget(t *testing.T) {
	keys := map[string]bool{
		"clients/example/ops":      true,
		"clients/example/strategy": true,
		"homelab":                  true,
	}
	key, slug, ok := splitTreeTarget("clients/example/strategy/page", keys)
	if !ok || key != "clients/example/strategy" || slug != "page" {
		t.Errorf("nested split = %q/%q/%v, want clients/example/strategy/page/true",
			key, slug, ok)
	}
	if _, _, ok := splitTreeTarget("clients/example/group/page", keys); ok {
		t.Error("a group path became a wiki target")
	}
	if _, _, ok := splitTreeTarget("homelab", keys); ok {
		t.Error("a bare wiki key became a page target")
	}
}

// TestMultiSiteCrossWikiLinks covers the aggregate-only edge: a sibling link
// becomes a namespaced edge in the tree graph, stays out of the wiki's own
// graph, and is rewritten to the sibling page in both online and offline
// output.
func TestMultiSiteCrossWikiLinks(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "alpha"))
	mustMkdir(t, filepath.Join(dir, "beta"))
	mustWrite(t, filepath.Join(dir, "alpha", "index.md"),
		"---\ntitle: Alpha\n---\n# Alpha\n[Beta](../beta/index.md)\n")
	mustWrite(t, filepath.Join(dir, "alpha", "topic.md"),
		"---\ntitle: Alpha topic\n---\n# Alpha topic\n")
	mustWrite(t, filepath.Join(dir, "beta", "index.md"),
		"---\ntitle: Beta\n---\n# Beta\n[Alpha](../alpha/index.md)\n")
	mustWrite(t, filepath.Join(dir, "beta", "topic.md"),
		"---\ntitle: Beta topic\n---\n# Beta topic\n")

	for _, offline := range []bool{false, true} {
		name := "online"
		if offline {
			name = "offline"
		}
		t.Run(name, func(t *testing.T) {
			out := t.TempDir()
			r, err := New(Config{
				Content: dir, Base: "/family/", Brand: Brand{Name: "t"},
				MultiSite: true, Offline: offline,
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.RenderSites(out); err != nil {
				t.Fatal(err)
			}

			read := func(path string) GraphIndex {
				t.Helper()
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var g GraphIndex
				if err := json.Unmarshal(raw, &g); err != nil {
					t.Fatalf("decode %s: %v", path, err)
				}
				return g
			}
			agg := read(filepath.Join(out, "family", "graph.json"))
			if edges := graphEdges(agg, "alpha:page:index", "beta:page:index", "link"); len(edges) != 1 {
				t.Errorf("cross edge = %+v, want one from alpha to beta", edges)
			}
			if edges := graphEdges(agg, "beta:page:index", "alpha:page:index", "link"); len(edges) != 1 {
				t.Errorf("reverse cross edge = %+v, want one from beta to alpha", edges)
			}
			own := read(filepath.Join(out, "family", "alpha", "graph.json"))
			for _, id := range []string{"page:beta/index", "page:beta/topic"} {
				if _, ok := graphNode(own, id); ok {
					t.Errorf("the per-wiki graph contains sibling node %q", id)
				}
			}
			for _, e := range own.Edges {
				if _, ok := graphNode(own, e.From); !ok {
					t.Errorf("the per-wiki graph has an edge from absent node %q", e.From)
				}
				if _, ok := graphNode(own, e.To); !ok {
					t.Errorf("the per-wiki graph has an edge to absent node %q", e.To)
				}
			}

			page, err := os.ReadFile(filepath.Join(out, "family", "alpha", "index.html"))
			if err != nil {
				t.Fatal(err)
			}
			want := `href="/family/beta/index.html"`
			if offline {
				want = `href="beta/index.html"`
			}
			if !strings.Contains(string(page), want) {
				t.Errorf("alpha page does not contain %q", want)
			}
		})
	}
}

// TestMultiSiteCrossWikiDoesNotHijackRepoLinks keeps a relative Markdown link
// to a non-wiki file inside the tree on the repo path. Only a path under a wiki
// key is a cross-wiki page.
func TestMultiSiteCrossWikiDoesNotHijackRepoLinks(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "alpha"))
	mustWrite(t, filepath.Join(dir, "alpha", "index.md"),
		"---\ntitle: Alpha\n---\n# Alpha\n[Docs](../repo-docs/page.md)\n")
	out := t.TempDir()
	r, err := New(Config{
		Content: dir, Base: "/family/", Brand: Brand{Name: "t"},
		MultiSite: true, Repo: "/tmp/repo",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(out); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(out, "family", "alpha", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), `href="/repo/repo-docs/page.md"`) {
		t.Error("a non-wiki tree link did not stay on the repo path")
	}
	raw, err := os.ReadFile(filepath.Join(out, "family", "graph.json"))
	if err != nil {
		t.Fatal(err)
	}
	var agg GraphIndex
	if err := json.Unmarshal(raw, &agg); err != nil {
		t.Fatal(err)
	}
	for _, e := range agg.Edges {
		if e.From == "alpha:page:index" {
			t.Errorf("a non-wiki tree link became a cross-wiki edge: %+v", e)
		}
	}
}

// TestMultiSiteEmptyGraphs covers the two cases with no aggregate to write over
// a wiki: a tree with no readable wiki still gets empty arrays at its base, and
// a root that is itself a wiki keeps its own unnamespaced catalogue there.
func TestMultiSiteEmptyGraphs(t *testing.T) {
	t.Run("empty tree", func(t *testing.T) {
		dir := t.TempDir()
		mustMkdir(t, filepath.Join(dir, "group"))
		out := t.TempDir()
		r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "okf-wiki"}, MultiSite: true})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.RenderSites(out); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(filepath.Join(out, "family", "graph.json"))
		if err != nil {
			t.Fatal(err)
		}
		var g GraphIndex
		if err := json.Unmarshal(raw, &g); err != nil {
			t.Fatal(err)
		}
		if g.Version != graphVersion || len(g.Nodes) != 0 || len(g.Edges) != 0 {
			t.Errorf("empty tree graph = %+v, want version %d and empty arrays", g, graphVersion)
		}
		if !strings.Contains(string(raw), `"nodes":[]`) || !strings.Contains(string(raw), `"edges":[]`) {
			t.Errorf("empty tree graph has null arrays: %s", raw)
		}
	})

	t.Run("root wiki", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\n---\n# Home\n")
		out := t.TempDir()
		r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "okf-wiki"}, MultiSite: true})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.RenderSites(out); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(filepath.Join(out, "family", "graph.json"))
		if err != nil {
			t.Fatal(err)
		}
		var g GraphIndex
		if err := json.Unmarshal(raw, &g); err != nil {
			t.Fatal(err)
		}
		node, ok := graphNode(g, "page:index")
		if !ok {
			t.Fatalf("root wiki graph = %+v, want its own page node", g)
		}
		if node.Site != "" || node.SiteKey != "" {
			t.Errorf("root wiki node gained a wiki label: %+v", node)
		}
	})
}
