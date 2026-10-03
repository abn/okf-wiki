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
func (r *Renderer) RenderSites(out string) (int, error) {
	root, err := scanSites(r.cfg.Content, "", map[string]bool{})
	if err != nil {
		return 0, err
	}
	// A group index has no bundle of its own, so it renders through the theme
	// with the theme's assets and an empty search catalogue at the tree root. A
	// root that is itself a wiki writes its own.
	if !root.wiki {
		if err := r.writeSiteRoot(out); err != nil {
			return 0, err
		}
	}
	wikis, err := r.renderSites(out, root)
	if err != nil {
		return 0, err
	}
	if err := r.writeRootRedirect(out); err != nil {
		return 0, err
	}
	return wikis, nil
}

func (r *Renderer) renderSites(out string, n *siteNode) (int, error) {
	if n.wiki {
		return 1, r.renderSite(out, n)
	}
	total := 0
	for _, c := range n.children {
		got, err := r.renderSites(out, c)
		if err != nil {
			return total, err
		}
		total += got
	}
	if err := r.writeCatalog(out, n); err != nil {
		return total, err
	}
	return total, nil
}

// renderSite renders one wiki of the tree. It shares the resolved theme with the
// tree renderer, and its own base is the tree base plus its path, so its links
// and assets sit under it.
func (r *Renderer) renderSite(out string, n *siteNode) error {
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
	}
	if err := sub.load(cfg.Content); err != nil {
		return err
	}
	return sub.RenderAll(out)
}

// writeSiteRoot places the theme's assets and an empty search catalogue at the
// tree root, where a group index looks for them. A group has no pages of its
// own, so the catalogue is empty rather than absent, and the search control on a
// group index answers instead of failing.
func (r *Renderer) writeSiteRoot(out string) error {
	site, err := r.cfg.SiteDir(out)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(site, 0o755); err != nil {
		return err
	}
	if err := r.theme.WriteTo(site); err != nil {
		return err
	}
	for name, body := range map[string]string{"search-index.json": "[]\n", "tags.json": "[]\n"} {
		if err := os.WriteFile(filepath.Join(site, name), []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// writeCatalog writes the index for a group directory: its direct children, each
// a card linking to the child's path and badged a wiki or a folder.
func (r *Renderer) writeCatalog(out string, n *siteNode) error {
	base := r.cfg.base()
	var b strings.Builder
	b.WriteString(`<ul class="site-list">`)
	for _, c := range n.children {
		title, desc := r.siteMeta(c)
		badge, cls := "Folder", "site-badge-folder"
		if c.wiki {
			badge, cls = "Wiki", "site-badge-wiki"
		}
		fmt.Fprintf(&b, `<li class="site-item"><a class="site-link" href="%s">`+
			`<span class="site-name">%s</span><span class="site-badge %s">%s</span></a>`+
			`<p class="site-desc">%s</p></li>`,
			template.HTMLEscapeString(siteBase(base, c.rel)),
			template.HTMLEscapeString(title), cls, badge,
			template.HTMLEscapeString(desc))
	}
	b.WriteString(`</ul>`)

	title, desc := r.siteMeta(n)
	html, err := r.catalogHTML(n, title, desc, b.String())
	if err != nil {
		return err
	}
	site, err := Config{Base: siteBase(base, n.rel)}.SiteDir(out)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(site, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(site, "index.html"), []byte(html), 0o644)
}

// catalogHTML renders a group index through the theme, with no sections of its
// own: an empty sidebar, no contents, no previous or next. The base stays the
// tree base so the page's assets and search resolve at the tree root.
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
