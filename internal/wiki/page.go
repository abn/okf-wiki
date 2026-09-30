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
	ThemeName    string
	ThemeVersion string
	// Base is the URL prefix every asset and page link hangs off, with a
	// leading and trailing slash. A theme never hardcodes /wiki/.
	Base       string
	Breadcrumb template.HTML
	Nav        template.HTML
	Meta       template.HTML
	TOC        template.HTML
	Body       template.HTML
	QuickJSON  template.JS
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
	data := shellData{
		Title:        title,
		Kicker:       p.Kicker,
		TitleHTML:    template.HTML(p.TitleHTML),
		Description:  description,
		Version:      r.Version(),
		GeneratedAt:  r.GeneratedAt(),
		AssetVersion: r.AssetVersion(),
		BrandName:    name,
		BrandTail:    tail,
		BrandSub:     r.cfg.Brand.Sub,
		BrandTitle:   r.cfg.Brand.Title,
		ThemeName:    r.theme.Name,
		ThemeVersion: r.theme.Version,
		Base:         base,
		Breadcrumb:   template.HTML(r.breadcrumbHTML(p)),
		Nav:          template.HTML(r.sidebarHTML(p.Section, p.Slug)),
		Meta:         template.HTML(r.buildMeta(p)),
		TOC:          template.HTML(buildTOC(p)),
		Body:         template.HTML(stripLeadingH1(p.Body, title)),
		QuickJSON:    template.JS(r.quickLinksJSON()),
		Slots:        r.slots(),
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
	if len(p.TOC) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<details class="toc" aria-label="On this page"><summary>On this page</summary><ul>`)
	for _, e := range p.TOC {
		cls := ""
		if e.Level == 3 {
			cls = ` class="l3"`
		}
		fmt.Fprintf(&b, `<li%s><a href="#%s">%s</a></li>`, cls, e.ID, template.HTMLEscapeString(e.Text))
	}
	b.WriteString(`</ul></details>`)
	return b.String()
}

// buildMeta renders the type and tag chips under the title. The type stays a
// span, since it names the page rather than pointing anywhere. Each tag links
// to its tag page, which lists every page carrying it. A tag with no URL form
// stays a span, so it never points at a page that was never written.
func (r *Renderer) buildMeta(p Page) string {
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
	if len(chips) == 0 {
		return ""
	}
	return `<div class="page-meta">` + strings.Join(chips, "") + `</div>`
}

func (r *Renderer) sidebarHTML(activeSection, activeSlug string) string {
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
		for _, page := range childPages {
			active := ""
			if page.Slug == activeSlug {
				active = ` class="active"`
			}
			title := page.Title
			if title == "" {
				title = page.Slug
			}
			fmt.Fprintf(&b, `<li%s><a href="%s%s.html">%s</a></li>`, active, base, page.Slug, template.HTMLEscapeString(title))
		}
		b.WriteString(`</ul></details>`)
	}
	b.WriteString(`</nav>`)
	return b.String()
}

func (r *Renderer) breadcrumbHTML(p Page) string {
	base := r.cfg.base()
	parts := []string{`<a href="` + base + `">Docs</a>`}
	// Tag pages belong to no section. They read Docs / Tags / #tag, with
	// Tags as unclassed text since there is no tag index page to point it
	// at and no theme class for a non-current, non-link crumb. The slug
	// prefix alone is not enough: a tags/ section holds real pages, so the
	// empty section marks the synthetic ones. The title already carries
	// the hash.
	if p.Section == "" && strings.HasPrefix(p.Slug, "tags/") {
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
