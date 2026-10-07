package wiki

import (
	"encoding/json"
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
		wikis, err = r.renderSites(tmp, root)
		if err != nil {
			return err
		}
		// The aggregate index is written after the wikis, because it is built
		// from what each of them produced. It sits at the tree base, where every
		// wiki of the tree looks for it.
		if err := r.writeAggregateSearchIndex(tmp); err != nil {
			return err
		}
		return r.writeRootRedirect(tmp)
	})
	if err != nil {
		return 0, err
	}
	return wikis, nil
}

func (r *Renderer) renderSites(root string, n *siteNode) (int, error) {
	if n.wiki {
		return r.renderSite(root, n)
	}
	total := 0
	for _, c := range n.children {
		got, err := r.renderSites(root, c)
		if err != nil {
			return total, err
		}
		total += got
	}
	if err := r.writeCatalog(root, n); err != nil {
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
func (r *Renderer) renderSite(root string, n *siteNode) (int, error) {
	cfg := r.cfg
	cfg.Content = n.abs
	cfg.Base = siteBase(r.cfg.base(), n.rel)
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
		// The bundle loaded but a page or an asset failed. The partial output is
		// removed, so a skipped wiki leaves nothing behind for the server to
		// serve as though it had rendered.
		if rmErr := os.RemoveAll(site); rmErr != nil {
			return 0, fmt.Errorf("skipping %s: %w (and removing its partial output: %v)", n.rel, err, rmErr)
		}
		r.skipped = append(r.skipped, SkippedSite{Rel: n.rel, Err: err})
		return 0, nil
	}
	// The wiki rendered, so its entries join the tree's aggregate index, labelled
	// with the wiki they came from. renderSites visits the wikis in sorted rel
	// order, and this preserves that order.
	title, _ := r.siteMeta(n)
	r.aggregated = append(r.aggregated, labelSearchIndex(sub.BuildSearchIndex(), title, n.rel)...)
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
	return os.WriteFile(filepath.Join(dir, "tags.json"), []byte("[]\n"), 0o644)
}

// writeAggregateSearchIndex writes the tree's one search index at the tree base:
// the labelled entries of every wiki that rendered, in the order renderSites
// visited them. A search from any wiki of the tree therefore reaches all of
// them, and each row says which wiki it came from. A tree with no readable wiki
// still gets an empty catalogue, so a group page's search control answers
// instead of fetching a 404. root is the output root the tree was written into.
func (r *Renderer) writeAggregateSearchIndex(root string) error {
	entries := r.aggregated
	if entries == nil {
		// Marshal an empty slice, not a nil one, so the catalogue is [] rather
		// than null: the client parses it as a list either way, but a group page
		// expecting an array should find one.
		entries = []SearchEntry{}
	}
	b, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	dir := r.cfg.sitePath(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "search-index.json"), b, 0o644)
}

// writeCatalog writes the index for a group directory: its direct children,
// grouped so the wikis read first and the folders that hold them follow.
// root is the tree root the tree is being written into.
//
// A child that was skipped is left off the cards: it has no page to link to, and
// a card that 404s is worse than the wiki being absent. The failure is reported
// by the caller through SkippedSites, not silently dropped here.
func (r *Renderer) writeCatalog(root string, n *siteNode) error {
	base := r.cfg.base()
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
				template.HTMLEscapeString(siteBase(base, c.rel)),
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
				template.HTMLEscapeString(siteBase(base, c.rel)),
				template.HTMLEscapeString(title),
				template.HTMLEscapeString(desc))
		}
		b.WriteString(`</ul></div>`)
	}

	title, desc := r.siteMeta(n)
	html, err := r.catalogHTML(n, title, desc, b.String())
	if err != nil {
		return err
	}
	dir := Config{Base: siteBase(base, n.rel)}.sitePath(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "index.html"), []byte(html), 0o644)
}

// catalogHTML renders a group index through the theme, with no sections of its
// own: an empty sidebar, no contents, no previous or next. The base stays the
// tree base so the page's assets and search resolve at the tree base.
func (r *Renderer) catalogHTML(n *siteNode, title, desc, body string) (string, error) {
	shell := &Renderer{
		md:           r.md,
		cfg:          r.cfg,
		theme:        r.theme,
		generatedAt:  r.generatedAt,
		assetVersion: r.assetVersion,
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
