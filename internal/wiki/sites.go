package wiki

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// siteNode is one directory in a multi-site tree: a wiki, or a group holding
// wikis and other groups.
type siteNode struct {
	rel      string // path under the content root, "" for the root itself
	abs      string
	wiki     bool
	children []*siteNode
}

// treeCrumb is one folder segment of a multi-site breadcrumb trail: the title
// shown and the group index it links to.
type treeCrumb struct {
	Title string
	Href  string
}

// siteCrumb names a node for a trail: the title a catalog card shows and the
// group index that lists what is below it.
func (r *Renderer) siteCrumb(n *siteNode) treeCrumb {
	title, _ := r.siteMeta(n)
	return treeCrumb{Title: title, Href: r.siteHomeURL(n.rel)}
}

// isWikiRoot reports whether a directory is a wiki root. The marker is an
// index.md, a bundle's home page. A group directory holds only sub-directories
// and optional metadata (.meta.json, README.md), so it has none.
func isWikiRoot(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, "index.md"))
	return err == nil && !st.IsDir()
}

// scanSites builds the tree under dir. A wiki is not descended into: its
// sub-directories are the wiki's own sections, not other sites.
//
// os.Stat decides whether an entry is a directory, rather than the type the
// directory read reported, so a symlink to a directory counts as a directory:
// a tree is assembled by linking bundles in from elsewhere. seen holds the
// resolved path of every directory already entered, so a link back into the
// tree is walked once instead of expanding until the operating system refuses
// the path length. An entry already seen yields no node.
func scanSites(dir, rel string, seen map[string]bool) (*siteNode, error) {
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		if seen[real] {
			return nil, nil
		}
		seen[real] = true
	}
	n := &siteNode{rel: rel, abs: dir, wiki: isWikiRoot(dir)}
	if n.wiki {
		return n, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		childAbs := filepath.Join(dir, e.Name())
		fi, err := os.Stat(childAbs)
		if err != nil || !fi.IsDir() {
			continue
		}
		child, err := scanSites(childAbs, path.Join(rel, e.Name()), seen)
		if err != nil {
			return nil, err
		}
		if child != nil {
			n.children = append(n.children, child)
		}
	}
	sort.Slice(n.children, func(i, j int) bool { return n.children[i].rel < n.children[j].rel })
	return n, nil
}

// collectWikiKeys records every wiki path below a tree root. The root itself
// is left out: a root wiki has no aggregate, so it cannot carry cross-wiki
// edges.
func collectWikiKeys(n *siteNode, keys map[string]bool) {
	if n.wiki {
		if n.rel != "" {
			keys[n.rel] = true
		}
		return
	}
	for _, c := range n.children {
		collectWikiKeys(c, keys)
	}
}

// siteBase puts a wiki or group's path under the configured base. The base
// already ends in a slash; the relative path has none.
func siteBase(base, rel string) string {
	if rel == "" {
		return base
	}
	return base + rel + "/"
}

// RenderSites renders the content directory as a tree of wikis rather than as a
// single bundle, and returns how many wikis it rendered. Each directory holding
// an index.md is a wiki, rendered at the path it sits at under the base; every
// other directory is a group, given a generated index listing the wikis and
// groups directly inside it. The output mirrors the tree, so the same layout
// serves from a static host or from `serve`.
//
// The tree is built beside the output root and renamed into place, so a render
// never exposes a tree with wikis missing. A tree owns the output root, so the
// swap replaces it whole.
func (r *Renderer) RenderSites(out string) (int, error) {
	root, err := scanSites(r.cfg.Content, "", map[string]bool{})
	if err != nil {
		return 0, err
	}
	r.treeWikis = make(map[string]bool)
	collectWikiKeys(root, r.treeWikis)
	// A root that is itself a wiki renders its own graph page at the tree base,
	// where the view reserves graph.html. A Markdown page or a static file of
	// that name would be overwritten, so name the collision before any output is
	// built. A wiki below the root keeps its graph.md: the view is not there.
	if err := agentCollision(root.abs); err != nil {
		return 0, err
	}
	previous, err := readAgentPages(r.cfg.sitePath(out))
	if err != nil {
		return 0, err
	}
	r.previousPages = previous
	if root.wiki {
		if err := graphCollision(root.abs); err != nil {
			return 0, err
		}
	}
	wikis := 0
	err = swapDir(out, func(tmp string) error {
		// A group index has no bundle of its own, so it renders through the
		// theme with the theme's assets at the tree base. A root that is itself
		// a wiki writes its own.
		if !root.wiki {
			if err := r.writeSiteRoot(tmp); err != nil {
				return err
			}
		}
		var err error
		wikis, err = r.renderSites(tmp, root, nil)
		if err != nil {
			return err
		}
		// The aggregate index is written after the wikis, because it is built
		// from what each of them produced. It sits at the tree base, where every
		// wiki of the tree looks for it. A root that is itself a wiki has no tree
		// beneath it: its own index already sits at the base, and an aggregate
		// there would replace it with labelled rows that make the client head a
		// list of one.
		if !root.wiki {
			if err := r.writeAggregateSearchIndex(tmp); err != nil {
				return err
			}
			r.appendCrossWikiEdges()
			if err := r.writeAggregateGraph(tmp); err != nil {
				return err
			}
			if err := r.writeTreeGraphPage(tmp, root); err != nil {
				return err
			}
			if err := r.writeTreeAgentFiles(tmp, root); err != nil {
				return err
			}
		}
		return r.writeRootRedirect(tmp)
	})
	if err != nil {
		return 0, err
	}
	return wikis, nil
}

// renderSites walks the tree, rendering each wiki and writing a group index for
// each group. trail is the folder crumbs from the tree root down to n, so a
// page inside n can name the groups above it.
func (r *Renderer) renderSites(root string, n *siteNode, trail []treeCrumb) (int, error) {
	if n.wiki {
		return r.renderSite(root, n, trail)
	}
	// The children sit one level below n, so their trail is n's plus n itself.
	// The tree root is left out: the breadcrumb names it from the configured
	// title, not from a folder crumb.
	childTrail := trail
	if n.rel != "" {
		childTrail = append(append([]treeCrumb{}, trail...), r.siteCrumb(n))
	}
	total := 0
	for _, c := range n.children {
		got, err := r.renderSites(root, c, childTrail)
		if err != nil {
			return total, err
		}
		total += got
	}
	if err := r.writeCatalog(root, n, trail); err != nil {
		return total, err
	}
	return total, nil
}

// renderSite renders one wiki of the tree. It shares the resolved theme with the
// tree renderer, and its own base is the tree base plus its path, so its links
// and assets sit under it. root is the tree root being written into.
//
// A wiki that cannot be read is recorded and skipped rather than returned as an
// error: one stale link in a tree of linked checkouts should not take the whole
// wiki offline. The failure is kept on the tree renderer so the caller can
// report it; the wiki is simply absent from the output and the count.
func (r *Renderer) renderSite(root string, n *siteNode, trail []treeCrumb) (int, error) {
	cfg := r.cfg
	cfg.Content = n.abs
	cfg.Base = siteBase(r.cfg.base(), n.rel)
	title, description := r.siteMeta(n)
	sub := &Renderer{
		md:           r.md,
		cfg:          cfg,
		theme:        r.theme,
		generatedAt:  r.generatedAt,
		assetVersion: r.assetVersion,
		nested:       true,
		// A wiki of a tree searches the tree, not itself: the page carries the
		// tree base so the client fetches the aggregate index written there.
		searchBase: r.cfg.base(),
		treeWikis:  r.treeWikis,
		// The page belongs to the tree, so its brand and breadcrumb point at the
		// tree root and name the folders and wiki it sits under.
		treeBase:        r.cfg.base(),
		siteRel:         n.rel,
		siteTitle:       title,
		siteDescription: description,
		siteTrail:       trail,
		previousPages:   r.previousPages,
	}
	if err := sub.load(cfg.Content); err != nil {
		r.skipped = append(r.skipped, SkippedSite{Rel: n.rel, Err: err})
		return 0, nil
	}
	// A wiki of the tree writes into the tree root at its own path, not into a
	// directory of its own to swap: the group indexes and the tree-base assets
	// share the tree, so only the whole tree turns over.
	site := cfg.sitePath(root)
	if err := sub.renderInto(root, site); err != nil {
		// A missing or incomplete diagram runtime is a deployment-wide
		// configuration error. Skipping the wiki would publish a tree that is
		// silently missing a wiki, and the next wiki would fail the same way.
		if errors.Is(err, errVendorRuntime) {
			return 0, err
		}
		// The bundle loaded but a page or an asset failed. The partial output is
		// removed, so a skipped wiki leaves nothing behind for the server to
		// serve as though it had rendered.
		if rmErr := os.RemoveAll(site); rmErr != nil {
			return 0, fmt.Errorf("skipping %s: %w (and removing its partial output: %v)", n.rel, err, rmErr)
		}
		r.skipped = append(r.skipped, SkippedSite{Rel: n.rel, Err: err})
		return 0, nil
	}
	// The wiki rendered, so its pages and entries merge into the tree's
	// aggregate index, labelled with the wiki they came from. renderSites
	// visits the wikis in sorted rel order, and this preserves that order. A
	// root that is itself a wiki (rel "") has no separate aggregate to feed.
	if n.rel != "" {
		index := labelSearchIndex(sub.BuildSearchIndex(), title, n.rel)
		mergeSearchIndex(&r.aggregated, &index)
		graph := sub.BuildGraph()
		mergeGraph(&r.aggregatedGraph, &graph, title, n.rel)
		r.crossEdges = append(r.crossEdges, sub.CrossWikiEdges(r.treeWikis)...)
		r.crossBroken = append(r.crossBroken, sub.crossBroken...)
		pages := sub.AgentPages()
		for i := range pages {
			pages[i].ID = n.rel + ":" + pages[i].ID
			pages[i].Site = title
			pages[i].SiteKey = n.rel
		}
		r.aggregatedPages = append(r.aggregatedPages, pages...)
	}
	return 1, nil
}

// SkippedSites returns the wikis a tree render left out, in the order they were
// met. Empty for a single-bundle render, and empty for a tree where every wiki
// read.
func (r *Renderer) SkippedSites() []SkippedSite { return r.skipped }

// wasSkipped reports whether a node was left out of the tree render, so its
// catalog card is not written and nothing links to a page that is not there.
func (r *Renderer) wasSkipped(n *siteNode) bool {
	for _, s := range r.skipped {
		if s.Rel == n.rel {
			return true
		}
	}
	return false
}

// writeSiteRoot places the theme's assets and an empty tag catalogue at the
// tree base, where a group index looks for them: a group page renders with the
// tree renderer, so its asset URLs carry the base, not the output root. The
// search catalogue is not written here: the aggregate one is written once every
// wiki has rendered, so a group page finds it. root is the output root the tree
// is being written into.
func (r *Renderer) writeSiteRoot(root string) error {
	dir := r.cfg.sitePath(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := r.theme.WriteTo(dir); err != nil {
		return err
	}
	tags := []byte("[]\n")
	if err := os.WriteFile(filepath.Join(dir, "tags.json"), tags, 0o644); err != nil {
		return err
	}
	if r.cfg.Offline {
		return writeJSData(dir, "tags.js", "__WIKI_TAGS", []byte("[]"))
	}
	return nil
}

// writeAggregateSearchIndex writes the tree's one search index at the tree base:
// the pages and entries of every wiki that rendered, in the order renderSites
// visited them and with each entry's page index rebased onto the merged page
// table. A search from any wiki of the tree therefore reaches all of them, and
// each page says which wiki it came from. A tree with no readable wiki still
// gets an empty catalogue, so a group page's search control answers instead of
// fetching a 404. The entries' texts are written beside it as
// search-content.json, in the same order as the aggregate's entries. root is
// the output root the tree was written into.
func (r *Renderer) writeAggregateSearchIndex(root string) error {
	index := r.aggregated
	index.RenderID = agentRenderID(r.aggregatedPages)
	if r.cfg.Offline {
		index.Base = ""
	} else {
		index.Base = r.cfg.base()
	}
	// Marshal empty slices, not nil ones, so the catalogue is an empty table
	// rather than null: the client parses either, but a group page expecting
	// pages and entries should find both.
	if index.Pages == nil {
		index.Pages = []SearchPage{}
	}
	if index.Entries == nil {
		index.Entries = []SearchEntry{}
	}
	b, err := json.Marshal(index)
	if err != nil {
		return err
	}
	dir := r.cfg.sitePath(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "search-index.json"), b, 0o644); err != nil {
		return err
	}
	contentValue := contentFromIndex(index)
	contentValue.RenderID = index.RenderID
	content, err := json.Marshal(contentValue)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "search-content.json"), content, 0o644); err != nil {
		return err
	}
	if r.cfg.Offline {
		if err := writeJSData(dir, "search-index.js", "__WIKI_SEARCH_INDEX", b); err != nil {
			return err
		}
		return writeJSData(dir, "search-content.js", "__WIKI_SEARCH_CONTENT", content)
	}
	return nil
}

// writeAggregateGraph writes the tree's one graph catalogue at the tree base,
// with each wiki's node IDs prefixed by its path so equal slugs in two wikis
// stay distinct. Like the aggregate search index it is written after every wiki
// has rendered, and a tree with no readable wiki still gets an empty catalogue
// rather than a missing file. root is the output root the tree was written into.
func (r *Renderer) writeAggregateGraph(root string) error {
	r.aggregatedGraph.RenderID = agentRenderID(r.aggregatedPages)
	b, err := graphJSON(r.aggregatedGraph)
	if err != nil {
		return err
	}
	dir := r.cfg.sitePath(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "graph.json"), b, 0o644)
}

// writeTreeGraphPage writes the tree's one graph view at the tree base, beside
// the aggregate catalogue it fetches. A group root has no wiki of its own to
// render it, so it is built here from the tree renderer; the root's title names
// the view's breadcrumb, and the tree base is where every wiki's header points.
// A root that is itself a wiki writes the page from its own render, where its
// pages are already in place.
func (r *Renderer) writeTreeGraphPage(out string, root *siteNode) error {
	title, _ := r.siteMeta(root)
	shell := &Renderer{
		md:           r.md,
		cfg:          r.cfg,
		theme:        r.theme,
		generatedAt:  r.generatedAt,
		assetVersion: r.assetVersion,
		treeBase:     r.cfg.base(),
		siteRel:      "",
		siteTitle:    title,
		graphPage:    true,
	}
	var inline []byte
	if r.cfg.InlineGraph {
		b, err := graphJSON(r.aggregatedGraph)
		if err != nil {
			return err
		}
		inline = b
	}
	dir := r.cfg.sitePath(out)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return shell.writeGraphPage(dir, inline)
}

// writeCatalog writes the index for a group directory: its direct children,
// grouped so the wikis read first and the folders that hold them follow.
// root is the tree root the tree is being written into.
//
// A child that was skipped is left off the cards: it has no page to link to, and
// a card that 404s is worse than the wiki being absent. The failure is reported
// by the caller through SkippedSites, not silently dropped here.
func (r *Renderer) writeCatalog(root string, n *siteNode, trail []treeCrumb) error {
	var b strings.Builder
	var wikis, folders []*siteNode
	for _, c := range n.children {
		if r.wasSkipped(c) {
			continue
		}
		if c.wiki {
			wikis = append(wikis, c)
			continue
		}
		folders = append(folders, c)
	}

	if len(wikis) > 0 {
		fmt.Fprintf(&b, `<ul class="site-list">`)
		for _, c := range wikis {
			title, desc := r.siteMeta(c)
			fmt.Fprintf(&b, `<li class="site-item"><a class="site-link" href="%s">`+
				`<span class="site-name">%s</span><span class="site-badge site-badge-wiki">Wiki</span></a>`+
				`<p class="site-desc">%s</p></li>`,
				template.HTMLEscapeString(r.siteHomeURL(c.rel)),
				template.HTMLEscapeString(title),
				template.HTMLEscapeString(desc))
		}
		b.WriteString(`</ul>`)
	}

	if len(folders) > 0 {
		b.WriteString(`<div class="site-group">`)
		b.WriteString(`<h2 class="site-group-title">Folders</h2>`)
		b.WriteString(`<ul class="folder-list">`)
		for _, c := range folders {
			title, desc := r.siteMeta(c)
			fmt.Fprintf(&b, `<li class="folder-item"><a class="folder-link" href="%s">`+
				`<span class="folder-name">%s</span>`+
				`<span class="folder-arrow" aria-hidden="true">&#8594;</span>`+
				`<span class="folder-desc">%s</span></a></li>`,
				template.HTMLEscapeString(r.siteHomeURL(c.rel)),
				template.HTMLEscapeString(title),
				template.HTMLEscapeString(desc))
		}
		b.WriteString(`</ul></div>`)
	}

	title, desc := r.siteMeta(n)
	html, err := r.catalogHTML(n, title, desc, b.String(), trail)
	if err != nil {
		return err
	}
	dir := Config{Base: siteBase(r.cfg.base(), n.rel)}.sitePath(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "index.html"), []byte(html), 0o644)
}

// catalogHTML renders a group index through the theme, with no sections of its
// own: an empty sidebar, no contents, no previous or next. The base stays the
// tree base so the page's assets and search resolve at the tree base. The tree
// state makes the group's brand and breadcrumb point at the tree root and name
// the folders above it.
func (r *Renderer) catalogHTML(n *siteNode, title, desc, body string, trail []treeCrumb) (string, error) {
	shell := &Renderer{
		md:           r.md,
		cfg:          r.cfg,
		theme:        r.theme,
		generatedAt:  r.generatedAt,
		assetVersion: r.assetVersion,
		treeBase:     r.cfg.base(),
		siteRel:      n.rel,
		siteTitle:    title,
		siteTrail:    trail,
		groupIndex:   true,
	}
	return shell.RenderPage(Page{
		Synthetic:   true,
		Slug:        n.rel,
		Title:       title,
		Description: desc,
		Body:        body,
	})
}

// siteMeta names a node for a catalog card, preferring explicit metadata over a
// guess: .meta.json first, then a wiki's own frontmatter, then a README, then
// the directory name. The tree root takes the configured title, since it has no
// directory name of its own.
func (r *Renderer) siteMeta(n *siteNode) (title, desc string) {
	if m, ok := readMetaJSON(filepath.Join(n.abs, ".meta.json")); ok {
		title, desc = m.Title, m.Description
	}
	if title == "" && n.wiki {
		fm, _ := splitFrontmatter(readFile(filepath.Join(n.abs, "index.md")))
		title, desc = firstNonEmpty(title, fm.Title), firstNonEmpty(desc, fm.Description)
	}
	if title == "" {
		t, d := readREADME(filepath.Join(n.abs, "README.md"))
		title, desc = t, firstNonEmpty(desc, d)
	}
	if title == "" {
		if n.rel == "" {
			title = r.cfg.Brand.Title
		} else {
			title = titleCase(path.Base(n.rel))
		}
	}
	if desc == "" && n.rel != "" {
		desc = "Knowledge base for " + title + "."
	}
	if desc == "" {
		desc = "Wikis published here."
	}
	return title, desc
}

type siteMetaJSON struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

func readMetaJSON(p string) (siteMetaJSON, bool) {
	b, err := os.ReadFile(p)
	if err != nil {
		return siteMetaJSON{}, false
	}
	var m siteMetaJSON
	if json.Unmarshal(b, &m) != nil {
		return siteMetaJSON{}, false
	}
	m.Title, m.Description = strings.TrimSpace(m.Title), strings.TrimSpace(m.Description)
	return m, m.Title != "" || m.Description != ""
}

// readREADME takes a title from the first heading and a summary from the first
// line of prose, which is all a collection README is used for.
func readREADME(p string) (title, desc string) {
	b, err := os.ReadFile(p)
	if err != nil {
		return "", ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "# ") {
			if title == "" {
				title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
			}
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		if title != "" && desc == "" {
			desc = line
		}
	}
	return title, desc
}

func readFile(p string) []byte {
	b, _ := os.ReadFile(p)
	return b
}
