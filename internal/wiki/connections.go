package wiki

import (
	"html/template"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// backlinkPlace is one place in a source page that links to the current page.
// Heading is the heading the links sit under, empty for links above the first
// heading, URL opens the source there, and Texts holds the link texts found in
// that place. One place may link more than once, so its texts stay together.
type backlinkPlace struct {
	Heading string
	URL     string
	Texts   []string
}

// backlinkSource is one page that links to the current page. Section names the
// wiki section it belongs to, SectionID orders sources by the sidebar's
// sections, Links counts the distinct links from it, and Places groups those
// links by the place they sit in, in document order.
type backlinkSource struct {
	Title     string
	Section   string
	SectionID string
	URL       string
	Links     int
	Places    []backlinkPlace
}

// add records one link from this source. Links in the same place share a row,
// so a page that mentions the target twice under one heading reads as one row
// rather than two.
func (s *backlinkSource) add(heading, href, text string) {
	s.Links++
	for i := range s.Places {
		if s.Places[i].URL != href {
			continue
		}
		if text != "" {
			s.Places[i].Texts = append(s.Places[i].Texts, text)
		}
		return
	}
	place := backlinkPlace{Heading: heading, URL: href}
	if text != "" {
		place.Texts = []string{text}
	}
	s.Places = append(s.Places, place)
}

// buildBacklinks indexes every recorded page link by its target slug, grouped
// by the source page it came from. The catalogue is built once per renderer,
// after all pages are loaded, and is used only for the page's Connections
// panel.
func (r *Renderer) buildBacklinks() {
	if r.backlinksReady {
		return
	}
	r.backlinks = make(map[string][]backlinkSource)
	index := make(map[string]map[string]*backlinkSource)
	seen := make(map[string]bool)
	sectionRank := make(map[string]int, len(r.sections))
	for i, s := range r.sections {
		sectionRank[s.ID] = i
	}
	for _, s := range r.sections {
		section := sectionHeading(s)
		for _, p := range s.Pages {
			source := r.urlBase() + p.Slug + ".html"
			for _, l := range p.links {
				if l.To == "" || l.To == p.Slug {
					continue
				}
				text := strings.TrimSpace(l.Text)
				key := p.Slug + "\x00" + l.To + "\x00" + l.Anchor + "\x00" + text
				if seen[key] {
					continue
				}
				seen[key] = true
				href := source
				if l.Anchor != "" {
					href += "#" + l.Anchor
				}
				targets, ok := index[l.To]
				if !ok {
					targets = make(map[string]*backlinkSource)
					index[l.To] = targets
				}
				ref, ok := targets[p.Slug]
				if !ok {
					ref = &backlinkSource{
						Title:     pageTitle(p),
						Section:   section,
						SectionID: s.ID,
						URL:       source,
					}
					targets[p.Slug] = ref
				}
				ref.add(l.Heading, href, text)
			}
		}
	}
	for target, targets := range index {
		sources := make([]backlinkSource, 0, len(targets))
		for _, ref := range targets {
			sources = append(sources, *ref)
		}
		// Sidebar order first, so the panel reads in the same order as the
		// navigation; titles break ties inside a section.
		sort.SliceStable(sources, func(i, j int) bool {
			a, b := sources[i], sources[j]
			if sectionRank[a.SectionID] != sectionRank[b.SectionID] {
				return sectionRank[a.SectionID] < sectionRank[b.SectionID]
			}
			if a.Title != b.Title {
				return a.Title < b.Title
			}
			return a.URL < b.URL
		})
		r.backlinks[target] = sources
	}
	r.backlinksReady = true
}

// connectionsHTML is the page's static Connections panel. It lists the pages
// that link here, once per page, and offers the focused graph around this page.
// A page no one links to still gets the panel, because the empty state is
// useful.
func (r *Renderer) connectionsHTML(p Page) string {
	if p.Synthetic {
		return ""
	}
	r.buildBacklinks()
	sources := r.backlinks[p.Slug]
	title := pageTitle(p)

	var b strings.Builder
	b.WriteString(`<section class="connections" aria-label="Connections">`)
	b.WriteString(`<div class="connections-head"><h2>Connections</h2>`)
	b.WriteString(`<a class="connections-graph" href="`)
	b.WriteString(template.HTMLEscapeString(r.localGraphHref(p)))
	b.WriteString(`" aria-label="Open the local graph around `)
	b.WriteString(template.HTMLEscapeString(title))
	b.WriteString(`">Local graph</a></div>`)
	if len(sources) == 0 {
		b.WriteString(`<p class="connections-empty">No pages link here yet.</p>`)
	} else {
		b.WriteString(`<h3 class="connections-subtitle">Linked from</h3>`)
		b.WriteString(`<ul class="connections-list">`)
		for _, s := range sources {
			b.WriteString(backlinkSourceHTML(s))
		}
		b.WriteString(`</ul>`)
	}
	b.WriteString(`</section>`)
	return b.String()
}

// backlinkSourceHTML renders one source page. A source with a single place,
// one heading or the page top, stays on one row with every link text on it. A
// source that links from several places carries a link count and one nested row
// per place, each one opening the source there.
func backlinkSourceHTML(s backlinkSource) string {
	esc := template.HTMLEscapeString
	var b strings.Builder
	b.WriteString(`<li class="connections-source">`)
	b.WriteString(`<div class="connections-row">`)
	href := s.URL
	if len(s.Places) == 1 {
		href = s.Places[0].URL
	}
	b.WriteString(`<a class="connections-source-title" href="`)
	b.WriteString(esc(href))
	b.WriteString(`">`)
	b.WriteString(esc(s.Title))
	b.WriteString(`</a>`)
	if s.Section != "" {
		b.WriteString(`<span class="connections-section">`)
		b.WriteString(esc(s.Section))
		b.WriteString(`</span>`)
	}
	if len(s.Places) == 1 {
		place := s.Places[0]
		if place.Heading != "" && place.Heading != s.Section {
			b.WriteString(`<span class="connections-heading">`)
			b.WriteString(esc(place.Heading))
			b.WriteString(`</span>`)
		}
		b.WriteString(backlinkQuotesHTML(place.Texts))
	} else {
		b.WriteString(`<span class="connections-count">`)
		b.WriteString(strconv.Itoa(s.Links))
		b.WriteString(` links`)
		if s.Links != len(s.Places) {
			b.WriteString(` in `)
			b.WriteString(strconv.Itoa(len(s.Places)))
			b.WriteString(` places`)
		}
		b.WriteString(`</span>`)
	}
	b.WriteString(`</div>`)
	if len(s.Places) > 1 {
		b.WriteString(`<ul class="connections-places">`)
		for _, place := range s.Places {
			label := place.Heading
			if label == "" {
				label = "Top of page"
			}
			b.WriteString(`<li><a class="connections-place" href="`)
			b.WriteString(esc(place.URL))
			b.WriteString(`">`)
			b.WriteString(esc(label))
			b.WriteString(`</a>`)
			b.WriteString(backlinkQuotesHTML(place.Texts))
			b.WriteString(`</li>`)
		}
		b.WriteString(`</ul>`)
	}
	b.WriteString(`</li>`)
	return b.String()
}

// backlinkQuotesHTML renders the link texts found in one place, each quoted.
func backlinkQuotesHTML(texts []string) string {
	var b strings.Builder
	for _, text := range texts {
		b.WriteString(`<span class="connections-quote">&ldquo;`)
		b.WriteString(template.HTMLEscapeString(text))
		b.WriteString(`&rdquo;</span>`)
	}
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
