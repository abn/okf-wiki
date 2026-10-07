package wiki

import (
	"encoding/json"
	"fmt"
	"html/template"
	"strings"
)

// shellData is the template payload for a rendered page. It is the contract
// between the renderer and a theme: a theme's template.html may rely on any of
// these fields, and the renderer promises all of them are populated.
type shellData struct {
	Title string
	// Kicker is an eyebrow label above the title, empty on ordinary pages.
	// Only synthetic pages set it today; a theme renders nothing when unset.
	Kicker string
	// TitleHTML is trusted H1 markup replacing Title when set. Ordinary
	// pages never set it; their title stays escaped text.
	TitleHTML    template.HTML
	Description  string
	Version      string
	GeneratedAt  string
	AssetVersion string
	BrandName    string
	BrandTail    string
	BrandSub     string
	BrandTitle   string
	// VersionTag is a freeform version label beside the wordmark, empty when
	// none is configured.
	VersionTag   string
	ThemeName    string
	ThemeVersion string
	// Base is the URL prefix every asset and page link hangs off, with a
	// leading and trailing slash. A theme never hardcodes /wiki/.
	Base string
	// HomeHref is the brand anchor's target. It is Base for a single bundle and
	// the tree base for a page of a multi-site tree, so the brand means the
	// deployment home and works from any depth. It falls back to Base whenever
	// no tree base is set.
	HomeHref string
	// SearchBase is the URL prefix the search index is fetched from. It equals
	// Base for a single bundle. In a multi-site tree it is the tree base, since
	// one index there spans every wiki. The page carries it as
	// window.__WIKI_SEARCH; the client falls back to Base when it is unset.
	SearchBase string
	Breadcrumb template.HTML
	Nav        template.HTML
	// TopNav is the header's links, in the order they were configured.
	TopNav template.HTML
	// TopNavInternal and TopNavExternal are those same links split by kind, for
	// the drawer, which puts the in-wiki ones first and labels the rest.
	TopNavInternal template.HTML
	TopNavExternal template.HTML
	Meta           template.HTML
	TOC            template.HTML
	// TOCList is the same entries as TOC as a bare list, for the sidebar
	// column and the dropdown panel. TOC keeps the details element for
	// themes that place it in the body.
	TOCList template.HTML
	// PrevNext is the previous/next cards for the section, empty when the
	// page stands alone in it.
	PrevNext  template.HTML
	Body      template.HTML
	QuickJSON template.JS
	// Slots holds the theme's named fragments, reachable as {{.Slot "name"}}.
	Slots map[string]template.HTML
}

// Slot returns a named template slot, or empty when the theme leaves it unset.
// A theme uses this to inject a stylesheet or a script without owning the
// whole shell.
func (d shellData) Slot(name string) template.HTML { return d.Slots[name] }

// RenderPage wraps a page body in the themed site shell. A template that parses
// but then fails against the data, on a field that does not exist or a method
// that panics, is a configuration error like a template that does not parse, so
// it fails the render rather than shipping a page whose body is the error text.
func (r *Renderer) RenderPage(p Page) (string, error) {
	title := p.Title
	if title == "" {
		title = p.Slug
	}

	description := p.Description
	if description == "" {
		description = title
	}

	name, tail := splitWordmark(r.cfg.Brand.Name)
	base := r.cfg.base()
	// A single bundle's index sits at its own base, so that is the default. A
	// wiki or group of a tree is told the tree base instead, where the one
	// aggregate index lives.
	searchBase := r.searchBase
	if searchBase == "" {
		searchBase = base
	}
	// The brand points at the deployment home: the page's own base for a single
	// bundle, the tree base for a wiki or group of a tree.
	homeHref := base
	if r.treeBase != "" {
		homeHref = r.treeBase
	}
	// A synthetic page has no file in the bundle, so there is nothing to
	// download; a real one points at the source the render copied beside it.
	markdown := ""
	if !p.Synthetic {
		markdown = base + p.Slug + ".md"
	}
	data := shellData{
		Title:          title,
		Kicker:         p.Kicker,
		TitleHTML:      template.HTML(p.TitleHTML),
		Description:    description,
		Version:        r.Version(),
		GeneratedAt:    r.GeneratedAt(),
		AssetVersion:   r.AssetVersion(),
		BrandName:      name,
		BrandTail:      tail,
		BrandSub:       r.cfg.Brand.Sub,
		BrandTitle:     r.cfg.Brand.Title,
		VersionTag:     r.cfg.Brand.VersionTag,
		ThemeName:      r.theme.Name,
		ThemeVersion:   r.theme.Version,
		Base:           base,
		HomeHref:       homeHref,
		SearchBase:     searchBase,
		Breadcrumb:     template.HTML(r.breadcrumbHTML(p)),
		Nav:            template.HTML(r.sidebarHTML(p.Section, p.Slug)),
		TopNav:         template.HTML(r.topNavHTML()),
		TopNavInternal: template.HTML(r.topNavMatching(false)),
		TopNavExternal: template.HTML(r.topNavMatching(true)),
		Meta:           template.HTML(r.buildMeta(p, markdown)),
		TOC:            template.HTML(buildTOC(p)),
		TOCList:        template.HTML(buildTOCList(p)),
		PrevNext:       template.HTML(r.prevNextHTML(p)),
		Body:           template.HTML(stripLeadingH1(p.Body, title)),
		QuickJSON:      template.JS(r.quickLinksJSON()),
		Slots:          r.slots(),
	}

	var b strings.Builder
	if err := r.theme.Template().Execute(&b, data); err != nil {
		return "", fmt.Errorf("render %s: theme %s: %w", p.Slug, r.theme.Source, err)
	}
	return b.String(), nil
}

func (r *Renderer) slots() map[string]template.HTML {
	out := map[string]template.HTML{}
	for _, name := range r.theme.SlotNames() {
		out[name] = r.theme.Slot(name)
	}
	return out
}

// splitWordmark splits a trailing "." off the brand name so it can be painted in
// the accent colour.
func splitWordmark(name string) (string, string) {
	if strings.HasSuffix(name, ".") {
		return strings.TrimSuffix(name, "."), "."
	}
	return name, ""
}

// quickLinksJSON builds a small list of section landing pages for the search
// modal's empty state, derived from the bundle rather than hardcoded.
func (r *Renderer) quickLinksJSON() string {
	type link struct {
		Label string `json:"label"`
		Href  string `json:"href"`
	}
	var links []link
	add := func(label, href string) {
		if label == "" || href == "" {
			return
		}
		links = append(links, link{Label: label, Href: href})
	}
	base := r.cfg.base()
	for _, s := range r.sections {
		if len(s.Pages) == 0 {
			continue
		}
		rootHref := ""
		for _, p := range s.Pages {
			if (s.ID == "" && p.Slug == "index") || (s.ID != "" && p.Slug == s.ID+"/index") {
				rootHref = base + p.Slug + ".html"
				break
			}
		}
		if rootHref == "" {
			rootHref = base + s.Pages[0].Slug + ".html"
		}
		add(s.Title, rootHref)
		if len(links) >= 6 {
			break
		}
	}
	b, _ := json.Marshal(links)
	return string(b)
}

func buildTOC(p Page) string {
	list := buildTOCList(p)
	if list == "" {
		return ""
	}
	return `<details class="toc" aria-label="On this page"><summary>On this page</summary>` + list + `</details>`
}

// buildTOCList renders the heading entries as a bare list, shared by the
// sidebar column and the dropdown panel. Both link to the same anchors, so
// rendering it twice introduces no duplicate ids.
func buildTOCList(p Page) string {
	if len(p.TOC) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<ul class="toc-list">`)
	for _, e := range p.TOC {
		fmt.Fprintf(&b, `<li><a href="#%s">%s</a></li>`, e.ID, template.HTMLEscapeString(e.Text))
	}
	b.WriteString(`</ul>`)
	return b.String()
}

// PageLink is one neighbor in the previous/next cards: a title and the URL
// it points at.
type PageLink struct {
	Title string
	URL   string
}

// pageNeighbors finds the pages around slug in its section, in sidebar order.
// Either side is nil at the ends, and both are nil when the page stands
// alone or is not found.
func (r *Renderer) pageNeighbors(section, slug string) (prev, next *PageLink) {
	link := func(p Page) *PageLink {
		title := p.Title
		if title == "" {
			title = p.Slug
		}
		return &PageLink{Title: title, URL: r.cfg.base() + p.Slug + ".html"}
	}
	for _, s := range r.sections {
		if s.ID != section {
			continue
		}
		for i, p := range s.Pages {
			if p.Slug != slug {
				continue
			}
			if i > 0 {
				prev = link(s.Pages[i-1])
			}
			if i+1 < len(s.Pages) {
				next = link(s.Pages[i+1])
			}
			return prev, next
		}
	}
	return nil, nil
}

// prevNextHTML renders the previous/next cards for a page. Empty when the
// page has no neighbor on either side.
func (r *Renderer) prevNextHTML(p Page) string {
	prev, next := r.pageNeighbors(p.Section, p.Slug)
	if prev == nil && next == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<nav class="prevnext" aria-label="More in this section">`)
	if prev != nil {
		fmt.Fprintf(&b, `<a class="prevnext-card prevnext-prev" href="%s"><span class="prevnext-dir">Previous</span><span class="prevnext-title">%s</span></a>`,
			prev.URL, template.HTMLEscapeString(prev.Title))
	}
	if next != nil {
		fmt.Fprintf(&b, `<a class="prevnext-card prevnext-next" href="%s"><span class="prevnext-dir">Next</span><span class="prevnext-title">%s</span></a>`,
			next.URL, template.HTMLEscapeString(next.Title))
	}
	b.WriteString(`</nav>`)
	return b.String()
}

// buildMeta renders the row under the title: the type and tag chips, and the
// page's own download link. The type stays a span, since it names the page
// rather than pointing anywhere. Each tag links to its tag page, which lists
// every page carrying it. A tag with no URL form stays a span, so it never
// points at a page that was never written. A page with neither chips nor a
// source still renders no row.
func (r *Renderer) buildMeta(p Page, markdown string) string {
	var chips []string
	if p.Type != "" {
		chips = append(chips, fmt.Sprintf(`<span class="chip chip-type">%s</span>`, template.HTMLEscapeString(p.Type)))
	}
	for _, t := range p.Tags {
		name := strings.TrimSpace(t)
		if name == "" {
			continue
		}
		slug := slugify(name)
		if slug == "" {
			chips = append(chips, fmt.Sprintf(`<span class="tag-link">#%s</span>`, template.HTMLEscapeString(name)))
			continue
		}
		chips = append(chips, tagLink(r.cfg.base(), slug, name))
	}
	if len(chips) == 0 && markdown == "" {
		return ""
	}
	row := `<div class="page-meta">` + strings.Join(chips, "")
	if markdown != "" {
		row += fmt.Sprintf(`<a class="page-download" href="%s" download>%s<span>Download Markdown</span></a>`,
			template.HTMLEscapeString(markdown), downloadArrow)
	}
	return row + `</div>`
}

// downloadArrow is the download glyph on the per-page Markdown link. It is
// decorative; the link's own text names the action.
const downloadArrow = `<svg class="page-download-icon" width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M8 2v8m0 0 3-3m-3 3-3-3M3 13h10"/></svg>`

func (r *Renderer) sidebarHTML(activeSection, activeSlug string) string {
	// A renderer with no sections, which is what a multi-site group index uses,
	// has no sidebar to show. Empty rather than an empty nav, so the template
	// can drop the column instead of reserving a blank rail.
	hasPages := false
	for _, sec := range r.sections {
		if len(sec.Pages) > 0 {
			hasPages = true
			break
		}
	}
	if !hasPages {
		return ""
	}
	var b strings.Builder
	base := r.cfg.base()
	b.WriteString(`<nav class="side" aria-label="Documentation navigation">`)
	for _, sec := range r.sections {
		if len(sec.Pages) == 0 {
			continue
		}

		var rootPage *Page
		var childPages []Page
		for i := range sec.Pages {
			p := &sec.Pages[i]
			if (sec.ID == "" && (p.Slug == "index" || p.Slug == "")) || (sec.ID != "" && p.Slug == sec.ID+"/index") {
				rootPage = p
			} else {
				childPages = append(childPages, *p)
			}
		}

		secHref := ""
		secActive := ""
		if rootPage != nil {
			secHref = base + rootPage.Slug + ".html"
			if rootPage.Slug == activeSlug {
				secActive = " active"
			}
		}

		if len(childPages) == 0 {
			b.WriteString(`<div class="side-sec side-leaf"><div class="side-summary">`)
			if secHref != "" {
				fmt.Fprintf(&b, `<a href="%s" class="side-title-link%s">%s</a>`, secHref, secActive, template.HTMLEscapeString(sec.Title))
			} else {
				fmt.Fprintf(&b, `<span class="side-title-text">%s</span>`, template.HTMLEscapeString(sec.Title))
			}
			b.WriteString(`</div></div>`)
			continue
		}

		isOpen := ""
		if sec.ID != "" && sec.ID == activeSection {
			isOpen = " open"
		} else if sec.ID == "" && activeSection == "" && activeSlug != "index" && activeSlug != "" {
			isOpen = " open"
		}

		b.WriteString(`<details class="side-sec" name="wiki-sections"` + isOpen + `>`)
		b.WriteString(`<summary class="side-summary">`)
		if secHref != "" {
			fmt.Fprintf(&b, `<a href="%s" class="side-title-link%s">%s</a>`, secHref, secActive, template.HTMLEscapeString(sec.Title))
		} else {
			fmt.Fprintf(&b, `<span class="side-title-text">%s</span>`, template.HTMLEscapeString(sec.Title))
		}
		b.WriteString(`<span class="side-chevron" aria-hidden="true"><svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M6 9l6 6 6-6"/></svg></span>`)
		b.WriteString(`</summary>`)
		b.WriteString(`<ul>`)
		r.writeSidebarEntries(&b, base, sec, childPages, activeSlug)
		b.WriteString(`</ul></details>`)
	}
	b.WriteString(`</nav>`)
	return b.String()
}

// sidebarGroup is the pages of a section that share a first sub-directory under
// the section. dir is that sub-directory's name, root its index.md if it has
// one, and pages the rest.
type sidebarGroup struct {
	dir   string
	root  *Page
	pages []Page
}

// childBase is the path the first segment of a slug is measured against: the
// section's own path for a named section, and nothing for the bundle root,
// whose pages carry no section prefix.
func childBase(secID string) string {
	if secID == "" {
		return ""
	}
	return secID + "/"
}

// groupChildren splits a section's child pages by the first path segment below
// the section, so a page at mods/acp-agents/architecture belongs to the
// acp-agents group. A page directly under the section has no second segment and
// is returned as a loose page. The group's own index.md is pulled out as the
// group root rather than left among its pages.
//
// One extra level is deliberate: a docs bundle is usually two deep, and a rail
// that nests every directory arbitrarily is tall and busy. Deeper paths collapse
// into their top group.
func groupChildren(secID string, pages []Page) (loose []Page, groups []sidebarGroup) {
	base := childBase(secID)
	byDir := map[string]*sidebarGroup{}
	var order []string
	for _, p := range pages {
		rest := strings.TrimPrefix(p.Slug, base)
		dir, tail, hasDir := strings.Cut(rest, "/")
		if !hasDir {
			loose = append(loose, p)
			continue
		}
		g, ok := byDir[dir]
		if !ok {
			g = &sidebarGroup{dir: dir}
			byDir[dir] = g
			order = append(order, dir)
		}
		if tail == "index" {
			p := p
			g.root = &p
			continue
		}
		g.pages = append(g.pages, p)
	}
	for _, dir := range order {
		groups = append(groups, *byDir[dir])
	}
	return loose, groups
}

// writeSidebarEntries writes the items of one section: its loose pages first,
// then a nested disclosure per sub-directory group. A group with no pages of its
// own beyond its index is a leaf, like a section with only a home page.
func (r *Renderer) writeSidebarEntries(b *strings.Builder, base string, sec Section, pages []Page, activeSlug string) {
	loose, groups := groupChildren(sec.ID, pages)
	for _, page := range loose {
		r.writeSidebarItem(b, base, page, activeSlug)
	}
	for _, g := range groups {
		r.writeSidebarGroup(b, base, g, activeSlug)
	}
}

// writeSidebarItem writes one page as a list item.
func (r *Renderer) writeSidebarItem(b *strings.Builder, base string, page Page, activeSlug string) {
	active := ""
	if page.Slug == activeSlug {
		active = ` class="active"`
	}
	title := page.Title
	if title == "" {
		title = page.Slug
	}
	fmt.Fprintf(b, `<li%s><a href="%s%s.html">%s</a></li>`, active, base, page.Slug, template.HTMLEscapeString(title))
}

// writeSidebarGroup writes a sub-directory of a section as its own list item: a
// nested disclosure when it holds pages, and a plain link when it holds only its
// index. The disclosure's summary is the group's index page when it has one, so
// the heading is a link the way a section heading is.
func (r *Renderer) writeSidebarGroup(b *strings.Builder, base string, g sidebarGroup, activeSlug string) {
	label := titleCase(g.dir)
	href := ""
	if g.root != nil {
		href = base + g.root.Slug + ".html"
		if g.root.Title != "" {
			label = g.root.Title
		}
	}

	if len(g.pages) == 0 && g.root != nil {
		active := ""
		if g.root.Slug == activeSlug {
			active = ` class="active"`
		}
		fmt.Fprintf(b, `<li class="side-leaf"><a%s href="%s">%s</a></li>`, active, href, template.HTMLEscapeString(label))
		return
	}

	open := ""
	for i := range g.pages {
		if g.pages[i].Slug == activeSlug {
			open = " open"
			break
		}
	}
	if g.root != nil && g.root.Slug == activeSlug {
		open = " open"
	}

	b.WriteString(`<li><details class="side-sub"` + open + `><summary class="side-subsum">`)
	if href != "" {
		fmt.Fprintf(b, `<a href="%s">%s</a>`, href, template.HTMLEscapeString(label))
	} else {
		fmt.Fprintf(b, `<span>%s</span>`, template.HTMLEscapeString(label))
	}
	b.WriteString(`<span class="side-chevron" aria-hidden="true"><svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M6 9l6 6 6-6"/></svg></span>`)
	b.WriteString(`</summary><ul>`)
	for _, page := range g.pages {
		r.writeSidebarItem(b, base, page, activeSlug)
	}
	b.WriteString(`</ul></details></li>`)
}

// navExtArrow marks a link that leaves the wiki. It is decorative: the
// new-tab behaviour is carried by the link's own label for a screen reader.
const navExtArrow = `<svg class="nav-ext" width="10" height="10" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4.2 7.8 7.8 4.2M5 4.2h3.2V7.4"/></svg>`

// external reports whether a target leaves the wiki. Only http and https count:
// a relative target is a page of this site, and a mailto or tel is handed to
// the system rather than opened in a window.
func external(target string) bool {
	t := strings.ToLower(strings.TrimSpace(target))
	return strings.HasPrefix(t, "http://") || strings.HasPrefix(t, "https://")
}

// topNavLink renders one header link. An http or https target leaves the wiki:
// it opens in a new tab and says so, with the arrow as the visible signal and
// the same words in the label for a screen reader. Any other target is a page of
// this bundle and resolves as a body link does, from the bundle root, so
// "usage/cli.md" becomes the rendered page under the base and opens in the same
// tab. Label and href are escaped, since both come from configuration rather
// than the bundle.
func (r *Renderer) topNavLink(l NavLink) string {
	label := template.HTMLEscapeString(l.Label)
	href := r.linkPath("", l.URL)
	attrs := ""
	if external(l.URL) {
		href = l.URL
		attrs = fmt.Sprintf(` target="_blank" rel="noopener noreferrer" aria-label="%s"`,
			template.HTMLEscapeString(l.Label+" (opens in a new tab)"))
		label += navExtArrow
	}
	return fmt.Sprintf(`<a href="%s"%s>%s</a>`, template.HTMLEscapeString(href), attrs, label)
}

// topNavHTML renders the header's links in the order they were configured,
// whether in-wiki or outbound. The header has no headings to group them under,
// and the arrow already marks the ones that leave, so the order stands.
func (r *Renderer) topNavHTML() string {
	var b strings.Builder
	for _, l := range parseNav(r.cfg.Brand.NavLinks) {
		b.WriteString(r.topNavLink(l))
	}
	return b.String()
}

// topNavMatching renders the configured links whose kind matches wantExternal.
// The drawer needs the two kinds apart: an in-wiki link is a page of this site,
// not somewhere else, so it does not belong under the heading the outbound links
// get. Empty when none of that kind are configured.
func (r *Renderer) topNavMatching(wantExternal bool) string {
	var b strings.Builder
	for _, l := range parseNav(r.cfg.Brand.NavLinks) {
		if external(l.URL) == wantExternal {
			b.WriteString(r.topNavLink(l))
		}
	}
	return b.String()
}

func (r *Renderer) breadcrumbHTML(p Page) string {
	base := r.cfg.base()
	// Tag pages belong to no section. They read Docs / Tags / #tag, with
	// Tags as unclassed text since there is no tag index page to point it
	// at and no theme class for a non-current, non-link crumb. The slug
	// prefix alone is not enough: a tags/ section holds real pages, so the
	// empty section marks the synthetic ones. The title already carries
	// the hash. A tree leaves this untouched: a tag page belongs to its own
	// wiki, and a trail into the tree would name a wiki the page does not
	// list.
	if p.Section == "" && strings.HasPrefix(p.Slug, "tags/") {
		parts := []string{`<a href="` + base + `">Docs</a>`}
		parts = append(parts, `<span>Tags</span>`)
		title := p.Title
		if title == "" {
			title = p.Slug
		}
		// The title carries the hash; split it off so themes can mute it
		// the way tag chips do.
		name := strings.TrimPrefix(title, "#")
		parts = append(parts, fmt.Sprintf(`<span class="crumb-cur"><span class="tag-hash">#</span>%s</span>`,
			template.HTMLEscapeString(name)))
		return strings.Join(parts, `<span class="crumb-sep">/</span>`)
	}
	if r.treeBase != "" {
		return r.treeBreadcrumbHTML(p)
	}
	parts := []string{`<a href="` + base + `">Docs</a>`}
	if p.Section != "" {
		for _, s := range r.sections {
			if s.ID == p.Section {
				parts = append(parts, fmt.Sprintf(`<a href="%s%s/index.html">%s</a>`, base, s.ID, template.HTMLEscapeString(s.Title)))
				break
			}
		}
	}
	title := p.Title
	if title == "" {
		title = p.Slug
	}
	parts = append(parts, fmt.Sprintf(`<span class="crumb-cur">%s</span>`, template.HTMLEscapeString(title)))
	return strings.Join(parts, `<span class="crumb-sep">/</span>`)
}

// treeBreadcrumbHTML builds the trail of a page in a multi-site tree: the tree
// title at the root, then a crumb per folder down to the page's own wiki or
// group, then the same section and page crumbs a single bundle gets. The tree
// title replaces the hardcoded "Docs" of a single bundle, and a folder links to
// the group index that lists what is below it. The current node's own crumb is
// a link on a deeper page and the current crumb on its home, so a wiki home is
// one click away and never linked to itself.
func (r *Renderer) treeBreadcrumbHTML(p Page) string {
	base := r.cfg.base()
	sep := `<span class="crumb-sep">/</span>`
	var parts []string
	// A page that is the tree root itself shows the root once as the current
	// crumb; a deeper page links back to it.
	if r.siteRel == "" {
		if r.nodeIsHome(p) {
			return crumbCur(r.cfg.Brand.Title)
		}
		parts = append(parts, crumbLink(r.treeBase, r.cfg.Brand.Title))
	} else {
		parts = append(parts, crumbLink(r.treeBase, r.cfg.Brand.Title))
		for _, c := range r.siteTrail {
			parts = append(parts, crumbLink(c.Href, c.Title))
		}
		nodeTitle := r.siteTitle
		if nodeTitle == "" {
			nodeTitle = r.cfg.Brand.Title
		}
		if r.nodeIsHome(p) {
			parts = append(parts, crumbCur(nodeTitle))
			return strings.Join(parts, sep)
		}
		parts = append(parts, crumbLink(siteBase(r.treeBase, r.siteRel), nodeTitle))
	}
	if p.Section != "" {
		for _, s := range r.sections {
			if s.ID == p.Section {
				parts = append(parts, crumbLink(base+s.ID+"/index.html", s.Title))
				break
			}
		}
	}
	title := p.Title
	if title == "" {
		title = p.Slug
	}
	parts = append(parts, crumbCur(title))
	return strings.Join(parts, sep)
}

// nodeIsHome reports whether a page is its node's own home: the wiki's
// index.md, or any page of a synthetic group index. It is what keeps the node's
// own crumb from being both a link and the current crumb.
func (r *Renderer) nodeIsHome(p Page) bool {
	if r.groupIndex {
		return true
	}
	return p.Section == "" && p.Slug == "index"
}

func crumbLink(href, title string) string {
	return fmt.Sprintf(`<a href="%s">%s</a>`, template.HTMLEscapeString(href), template.HTMLEscapeString(title))
}

// crumbCur renders the current breadcrumb segment. It carries the crumb-cur
// class the stylesheet and the DOM contract expect.
func crumbCur(title string) string {
	return fmt.Sprintf(`<span class="crumb-cur">%s</span>`, template.HTMLEscapeString(title))
}

func stripLeadingH1(body, title string) string {
	trimmed := strings.TrimSpace(body)
	if strings.HasPrefix(trimmed, "<h1") {
		idx := strings.Index(trimmed, "</h1>")
		if idx != -1 {
			return strings.TrimSpace(trimmed[idx+5:])
		}
	}
	return body
}
