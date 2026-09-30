package wiki

import (
	"encoding/json"
	"fmt"
	"html/template"
	"sort"
	"strings"
	"time"
)

// tagListing groups the pages carrying one tag. Slug is the URL form of
// Display, so the page lives at tags/<slug>.html under the base.
type tagListing struct {
	Display string
	Slug    string
	Pages   []Page
}

// tagListings collects every tag used in the bundle. Tags group by slug, so
// two spellings that slugify alike share one page under the first spelling
// seen. A tag with no URL form cannot have a page and is left out here;
// buildMeta renders those as plain spans instead of links.
func (r *Renderer) tagListings() []tagListing {
	type accum struct {
		display string
		pages   []Page
	}
	bySlug := map[string]*accum{}
	var order []string
	for _, s := range r.sections {
		for _, p := range s.Pages {
			for _, t := range p.Tags {
				name := strings.TrimSpace(t)
				if name == "" {
					continue
				}
				slug := slugify(name)
				if slug == "" {
					continue
				}
				a := bySlug[slug]
				if a == nil {
					a = &accum{display: name}
					bySlug[slug] = a
					order = append(order, slug)
				}
				a.pages = append(a.pages, p)
			}
		}
	}
	titles := map[string]string{}
	for _, s := range r.sections {
		titles[s.ID] = s.Title
	}
	var out []tagListing
	for _, slug := range order {
		a := bySlug[slug]
		pages := append([]Page(nil), a.pages...)
		sort.Slice(pages, func(i, j int) bool {
			ti, tj := sectionTitle(titles, pages[i].Section), sectionTitle(titles, pages[j].Section)
			if ti != tj {
				return ti < tj
			}
			return pages[i].Title < pages[j].Title
		})
		out = append(out, tagListing{Display: a.display, Slug: slug, Pages: pages})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

func sectionTitle(titles map[string]string, id string) string {
	if t, ok := titles[id]; ok && t != "" {
		return t
	}
	return "Overview"
}

// RenderTagPage renders the listing page for one tag through the themed
// shell. It is a synthetic page: no section, so the sidebar marks nothing
// active and the breadcrumb reads Docs / Tags / #tag. The title carries the
// hash, matching the search rows and the chips that point here.
func (r *Renderer) RenderTagPage(t tagListing, all []tagListing) (string, error) {
	n := len(t.Pages)
	lead := fmt.Sprintf("%d pages tagged #%s.", n, t.Display)
	if n == 1 {
		lead = fmt.Sprintf("1 page tagged #%s.", t.Display)
	}
	dep := 0
	for _, p := range t.Pages {
		if p.Status == "deprecated" {
			dep++
		}
	}
	return r.RenderPage(Page{
		Section:     "",
		Slug:        "tags/" + t.Slug,
		Title:       "#" + t.Display,
		Kicker:      "Tag",
		TitleHTML:   `<span class="tag-hash">#</span>` + template.HTMLEscapeString(t.Display),
		Description: lead,
		Body:        r.tagPageBody(t, all, dep, time.Now().UTC()),
	})
}

// tagPageBody builds a tag page: co-occurring tags, grouping and deprecated
// controls, the grouped listing, the full tag catalogue, and the script that
// drives the controls. Only information the bundle actually carries is
// rendered: trust tiers, dates and sort orders need frontmatter no page here
// is required to have, so they are absent rather than guessed.
func (r *Renderer) tagPageBody(t tagListing, all []tagListing, dep int, now time.Time) string {
	var b strings.Builder
	base := r.cfg.base()
	titles := map[string]string{}
	for _, s := range r.sections {
		titles[s.ID] = s.Title
	}

	if often := oftenWith(t, all); len(often) > 0 {
		b.WriteString(`<div class="tag-often"><span class="tag-muted">Often with</span> `)
		for _, o := range often {
			fmt.Fprintf(&b, `%s `, tagChip(base, o.Slug, o.Display, o.Pages, false))
		}
		b.WriteString(`<a class="tag-all-link" href="#all-tags">All tags →</a></div>` + "\n")
	}

	b.WriteString("<hr>\n")
	b.WriteString(`<div class="tag-controls">` + "\n")
	b.WriteString(`<div class="seg" role="group" aria-label="Grouping">` +
		`<button type="button" data-tag-setmode="section" aria-pressed="true">By section</button>` +
		`<button type="button" data-tag-setmode="all" aria-pressed="false">All</button></div>` + "\n")
	if dep > 0 {
		b.WriteString(`<label class="tag-check"><input type="checkbox" data-tag-dep> Show deprecated</label>` + "\n")
	}
	b.WriteString(`</div>` + "\n")

	// On its own line the note reads cleaner without the separator the
	// summary line would need, so it is a plain sentence.
	if dep > 0 {
		fmt.Fprintf(&b, `<p class="tag-muted" data-tag-dep-wrap><span data-tag-dep-note>%d deprecated hidden</span></p>`+"\n", dep)
	}

	b.WriteString(`<div data-tag-list data-tag-mode="section">` + "\n")
	lastSection := ""
	for _, p := range t.Pages {
		sec := sectionTitle(titles, p.Section)
		if sec != lastSection {
			lastSection = sec
			n := 0
			for _, q := range t.Pages {
				if sectionTitle(titles, q.Section) == sec {
					n++
				}
			}
			fmt.Fprintf(&b, `<h3 class="tag-group" data-tag-group="%s">%s`,
				template.HTMLEscapeString(sec), template.HTMLEscapeString(sec))
			if p.Section != "" {
				fmt.Fprintf(&b, ` <code>%s/</code>`, template.HTMLEscapeString(p.Section))
			}
			if n == 1 {
				b.WriteString(` <span class="tag-muted">1 page</span>`)
			} else {
				fmt.Fprintf(&b, ` <span class="tag-muted">%d pages</span>`, n)
			}
			b.WriteString(`</h3>` + "\n")
		}
		r.tagRow(&b, p, sec, base, t.Slug, now)
	}
	b.WriteString(`</div>` + "\n")

	sorted := append([]tagListing(nil), all...)
	sort.Slice(sorted, func(i, j int) bool {
		ai, aj := strings.ToLower(sorted[i].Display), strings.ToLower(sorted[j].Display)
		if ai != aj {
			return ai < aj
		}
		return sorted[i].Display < sorted[j].Display
	})
	b.WriteString("<hr>\n")
	b.WriteString(`<h3 id="all-tags">All tags</h3>` + "\n" + `<div class="tag-all">` + "\n")
	for _, o := range sorted {
		b.WriteString(tagChip(base, o.Slug, o.Display, len(o.Pages), o.Slug == t.Slug) + "\n")
	}
	b.WriteString(`</div>` + "\n")
	b.WriteString(tagScript)
	return b.String()
}

// tagRow renders one listed page: title, type and status badges, description,
// and the concept path with the page's other tags. The listed tag itself is
// left out of the row's chips: it names every row here, so repeating it is
// noise. Rows carry data hooks for the grouping and deprecated toggles; the
// script reorders and filters them.
func (r *Renderer) tagRow(b *strings.Builder, p Page, sec, base, self string, now time.Time) {
	title := p.Title
	if title == "" {
		title = p.Slug
	}
	dep := p.Status == "deprecated"
	fmt.Fprintf(b, `<div class="tag-row" data-tag-row data-title="%s" data-section="%s" data-deprecated="%t"`,
		template.HTMLEscapeString(title), template.HTMLEscapeString(sec), dep)
	if dep {
		b.WriteString(` hidden`)
	}
	b.WriteString(">\n")
	fmt.Fprintf(b, `<div class="tag-row-head"><a href="%s%s.html">%s</a>`,
		base, p.Slug, template.HTMLEscapeString(title))
	b.WriteString(`<span class="tag-badges">`)
	if p.Type != "" {
		fmt.Fprintf(b, `<span class="chip">%s</span>`, template.HTMLEscapeString(p.Type))
	}
	switch {
	case dep:
		b.WriteString(`<span class="chip">Deprecated</span>`)
	case p.Status == "draft":
		b.WriteString(`<span class="chip">Draft</span>`)
	case stale(p.StaleAfter, now):
		b.WriteString(`<span class="chip">Stale</span>`)
	}
	b.WriteString(`</span></div>` + "\n")
	if d := strings.TrimSpace(p.Description); d != "" {
		fmt.Fprintf(b, "<p>%s</p>\n", template.HTMLEscapeString(d))
	}
	b.WriteString(`<p class="tag-row-meta">`)
	fmt.Fprintf(b, `<code>%s</code>`, template.HTMLEscapeString(p.Slug))
	for _, tg := range p.Tags {
		name := strings.TrimSpace(tg)
		if name == "" {
			continue
		}
		slug := slugify(name)
		if slug == "" || slug == self {
			continue
		}
		fmt.Fprintf(b, ` %s`, tagChip(base, slug, name, 0, false))
	}
	b.WriteString("</p>\n</div>\n")
}

// tagLink renders a tag link for page heads: raw text, no pill. The page head
// already carries the type pill, so tags read as trailing metadata.
func tagLink(base, slug, display string) string {
	return fmt.Sprintf(`<a class="tag-link" href="%stags/%s.html"><span class="tag-hash">#</span>%s</a>`,
		base, slug, template.HTMLEscapeString(display))
}

// tagChip renders one tag link for tag contexts: often-with, row meta lines,
// and the full catalogue. The hash is its own span so themes can mute it
// without muting the name, and the count shows only when nonzero, which is
// what separates the catalogue chips from the row ones.
func tagChip(base, slug, display string, count int, current bool) string {
	var b strings.Builder
	b.WriteString(`<a class="chip"`)
	if current {
		b.WriteString(` aria-current="true"`)
	}
	fmt.Fprintf(&b, ` href="%stags/%s.html"><span class="tag-hash">#</span>%s`,
		base, slug, template.HTMLEscapeString(display))
	if count > 0 {
		fmt.Fprintf(&b, " %d", count)
	}
	b.WriteString(`</a>`)
	return b.String()
}

// coTag is one tag occurring alongside the listed tag, with its page count.
type coTag struct {
	Display string
	Slug    string
	Pages   int
}

// oftenWith collects the tags co-occurring on the listed tag's pages, minus
// the tag itself, most frequent first. Capped at five: the full catalogue
// sits at the bottom of the page for browsing.
func oftenWith(t tagListing, all []tagListing) []coTag {
	counts := map[string]int{}
	display := map[string]string{}
	for _, p := range t.Pages {
		for _, tg := range p.Tags {
			name := strings.TrimSpace(tg)
			if name == "" {
				continue
			}
			slug := slugify(name)
			if slug == "" || slug == t.Slug {
				continue
			}
			counts[slug]++
			if _, ok := display[slug]; !ok {
				display[slug] = name
			}
		}
	}
	var out []coTag
	for slug, n := range counts {
		out = append(out, coTag{Display: display[slug], Slug: slug, Pages: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Pages != out[j].Pages {
			return out[i].Pages > out[j].Pages
		}
		return strings.ToLower(out[i].Display) < strings.ToLower(out[j].Display)
	})
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}

// stale reports whether a stale_after instant has passed. Absent or
// unparsable means not stale; the badge simply never renders.
func stale(after string, now time.Time) bool {
	if after == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, after)
	if err != nil {
		return false
	}
	return !t.After(now)
}

// tagScript drives the grouping and deprecated toggles by reordering and
// filtering the server-rendered rows. No data is duplicated: rows carry
// everything in data attributes.
const tagScript = `<script>
(function () {
  var root = document.querySelector('[data-tag-list]');
  if (!root) return;
  var groups = [];
  (function () {
    var cur = null;
    Array.prototype.forEach.call(root.children, function (el) {
      if (el.hasAttribute('data-tag-group')) { cur = { head: el, rows: [] }; groups.push(cur); }
      else if (el.hasAttribute('data-tag-row') && cur) { cur.rows.push(el); }
    });
  })();
  var rows = [];
  groups.forEach(function (g) { rows = rows.concat(g.rows); });
  var note = document.querySelector('[data-tag-dep-note]');
  var wrap = document.querySelector('[data-tag-dep-wrap]');
  var box = document.querySelector('[data-tag-dep]');
  function byTitle(a, b) {
    var x = a.getAttribute('data-title').toLowerCase();
    var y = b.getAttribute('data-title').toLowerCase();
    return x < y ? -1 : x > y ? 1 : 0;
  }
  function apply() {
    var showDep = !!(box && box.checked);
    var all = root.getAttribute('data-tag-mode') === 'all';
    var hidden = 0;
    rows.forEach(function (r) { r.removeAttribute('hidden'); });
    while (root.firstChild) root.removeChild(root.firstChild);
    function take(list) {
      return list.filter(function (r) {
        if (r.getAttribute('data-deprecated') !== 'true') return true;
        if (showDep) return true;
        hidden++;
        return false;
      });
    }
    if (all) {
      take(rows).sort(byTitle).forEach(function (r) { root.appendChild(r); });
    } else {
      groups.forEach(function (g) {
        var vis = take(g.rows);
        if (!vis.length) return;
        root.appendChild(g.head);
        vis.forEach(function (r) { root.appendChild(r); });
      });
    }
    if (note) note.textContent = hidden ? hidden + ' deprecated hidden' : '';
    if (wrap) wrap.style.display = hidden ? '' : 'none';
  }
  Array.prototype.forEach.call(document.querySelectorAll('[data-tag-setmode]'), function (btn) {
    btn.addEventListener('click', function () {
      root.setAttribute('data-tag-mode', btn.getAttribute('data-tag-setmode'));
      Array.prototype.forEach.call(document.querySelectorAll('[data-tag-setmode]'), function (b) {
        b.setAttribute('aria-pressed', b === btn ? 'true' : 'false');
      });
      apply();
    });
  });
  if (box) box.addEventListener('change', apply);
  // The whole row navigates to the page. Clicks on inner links keep their
  // own destination, and a text selection is never a navigation: without
  // the guard, selecting description text would leave the page.
  // Keyboard users keep the title link, which stays focused and reachable.
  rows.forEach(function (r) {
    r.addEventListener('click', function (e) {
      if (e.target.closest('a,button')) return;
      var sel = window.getSelection ? window.getSelection().toString() : '';
      if (sel) return;
      var link = r.querySelector('.tag-row-head > a');
      if (link) window.location.href = link.getAttribute('href');
    });
  });
  apply();
})();
</script>
`

// tagJSON is one row of tags.json, the file the search modal reads for
// hash-prefixed queries. Display is the original spelling, URL the tag page,
// Pages the count shown beside it.
type tagJSON struct {
	Display string `json:"t"`
	URL     string `json:"u"`
	Pages   int    `json:"n"`
}

// TagsJSON returns the tag catalogue for the search modal, sorted by display
// name. An empty bundle writes an empty array rather than null, so the client
// never has to distinguish the two.
func (r *Renderer) TagsJSON() ([]byte, error) {
	out := []tagJSON{}
	for _, t := range r.tagListings() {
		out = append(out, tagJSON{
			Display: t.Display,
			URL:     r.cfg.base() + "tags/" + t.Slug + ".html",
			Pages:   len(t.Pages),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		ai, aj := strings.ToLower(out[i].Display), strings.ToLower(out[j].Display)
		if ai != aj {
			return ai < aj
		}
		return out[i].Display < out[j].Display
	})
	return json.Marshal(out)
}
