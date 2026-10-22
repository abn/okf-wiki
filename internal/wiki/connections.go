package wiki

import (
	"html/template"
	"net/url"
	"sort"
	"strings"
)

// backlinkRef is one link from another page into this one. SourceTitle and
// Section name where it came from, Heading is the heading the link sat under,
// Text is the link's own text, and URL opens the source at that heading.
type backlinkRef struct {
	SourceTitle string
	Section     string
	Heading     string
	URL         string
	Text        string
}

// buildBacklinks indexes every recorded page link by its target slug. The
// catalogue is built once per renderer, after all pages are loaded, and is used
// only for the page's Connections panel.
func (r *Renderer) buildBacklinks() {
	if r.backlinksReady {
		return
	}
	r.backlinks = make(map[string][]backlinkRef)
	seen := make(map[string]bool)
	for _, s := range r.sections {
		section := sectionHeading(s)
		for _, p := range s.Pages {
			title := pageTitle(p)
			source := r.urlBase() + p.Slug + ".html"
			for _, l := range p.links {
				if l.To == "" || l.To == p.Slug {
					continue
				}
				key := p.Slug + "\x00" + l.To + "\x00" + l.Anchor + "\x00" + l.Text
				if seen[key] {
					continue
				}
				seen[key] = true
				href := source
				if l.Anchor != "" {
					href += "#" + l.Anchor
				}
				r.backlinks[l.To] = append(r.backlinks[l.To], backlinkRef{
					SourceTitle: title,
					Section:     section,
					Heading:     l.Heading,
					URL:         href,
					Text:        strings.TrimSpace(l.Text),
				})
			}
		}
	}
	for target := range r.backlinks {
		sort.SliceStable(r.backlinks[target], func(i, j int) bool {
			a, b := r.backlinks[target][i], r.backlinks[target][j]
			if a.SourceTitle != b.SourceTitle {
				return a.SourceTitle < b.SourceTitle
			}
			if a.Section != b.Section {
				return a.Section < b.Section
			}
			return a.Text < b.Text
		})
	}
	r.backlinksReady = true
}

// connectionsHTML is the page's static Connections panel. It lists the pages
// that link here and offers the focused graph around this page. A page no one
// links to still gets the panel, because the empty state is useful.
func (r *Renderer) connectionsHTML(p Page) string {
	if p.Synthetic {
		return ""
	}
	r.buildBacklinks()
	links := r.backlinks[p.Slug]
	title := pageTitle(p)

	var b strings.Builder
	b.WriteString(`<section class="connections" aria-label="Connections">`)
	b.WriteString(`<div class="connections-head"><h2>Connections</h2>`)
	b.WriteString(`<a class="connections-graph" href="`)
	b.WriteString(template.HTMLEscapeString(r.localGraphHref(p)))
	b.WriteString(`" aria-label="Open the local graph around `)
	b.WriteString(template.HTMLEscapeString(title))
	b.WriteString(`">Local graph</a></div>`)
	if len(links) == 0 {
		b.WriteString(`<p class="connections-empty">No pages link here yet.</p>`)
	} else {
		b.WriteString(`<h3 class="connections-subtitle">Linked from</h3>`)
		b.WriteString(`<ul class="connections-list">`)
		for _, l := range links {
			b.WriteString(`<li><a href="`)
			b.WriteString(template.HTMLEscapeString(l.URL))
			b.WriteString(`">`)
			b.WriteString(template.HTMLEscapeString(l.SourceTitle))
			b.WriteString(`</a>`)
			if l.Section != "" {
				b.WriteString(`<span class="connections-section">`)
				b.WriteString(template.HTMLEscapeString(l.Section))
				b.WriteString(`</span>`)
			}
			if l.Heading != "" && l.Heading != l.Section {
				b.WriteString(`<span class="connections-heading">`)
				b.WriteString(template.HTMLEscapeString(l.Heading))
				b.WriteString(`</span>`)
			}
			if l.Text != "" {
				b.WriteString(`<span class="connections-quote">&ldquo;`)
				b.WriteString(template.HTMLEscapeString(l.Text))
				b.WriteString(`&rdquo;</span>`)
			}
			b.WriteString(`</li>`)
		}
		b.WriteString(`</ul>`)
	}
	b.WriteString(`</section>`)
	return b.String()
}

// localGraphHref names the aggregate node for this page, namespaced when the
// page belongs to a wiki below a tree root, and opens the graph focused on it.
func (r *Renderer) localGraphHref(p Page) string {
	id := pageNodeID(p.Slug)
	if r.nested && r.siteRel != "" {
		id = r.siteRel + ":" + id
	}
	return r.graphBase() + "graph.html?focus=" + url.QueryEscape(id) + "&depth=1"
}
