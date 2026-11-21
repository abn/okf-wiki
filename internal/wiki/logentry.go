package wiki

import (
	"fmt"
	"html/template"
	"regexp"
	"sort"
	"strings"
	"time"
)

// logEntryPath matches the dated file name docs/log.md prescribes for an entry
// page, anywhere in the slug so a wiki nested in a tree still matches.
var logEntryPath = regexp.MustCompile(`(?:^|/)logs/(\d{4})/(\d{2})/(\d{2})-[^/]+$`)

// logDateLayouts are the frontmatter date forms an entry may carry, most
// specific first. yaml.v3 hands a timestamp to a string field verbatim.
var logDateLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02",
}

// isLogEntry reports whether p is a change log entry: typed "Log Entry", or
// filed at logs/YYYY/MM/DD-<slug>.md when it carries no type. A month or year
// index under logs/ is not an entry.
func isLogEntry(p Page) bool {
	if strings.EqualFold(strings.TrimSpace(p.Type), "Log Entry") {
		return true
	}
	return p.Type == "" && logEntryPath.MatchString(p.Slug)
}

// logDate is the date a log entry records: its frontmatter date when that
// parses, otherwise the date in its logs/YYYY/MM/DD- path. The second result is
// false for a page with neither, which then keeps its section order.
func logDate(p Page) (time.Time, bool) {
	if raw := strings.TrimSpace(p.Date); raw != "" {
		for _, layout := range logDateLayouts {
			if t, err := time.Parse(layout, raw); err == nil {
				return t, true
			}
		}
	}
	m := logEntryPath.FindStringSubmatch(p.Slug)
	if m == nil {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", m[1]+"-"+m[2]+"-"+m[3])
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// datedLogEntry is a log entry with the date it sorts by.
type datedLogEntry struct {
	page Page
	date time.Time
}

// logEntries is every dated log entry of the bundle, oldest first. Entries on
// one day order by their frontmatter time, then by slug, which is the file
// name. It is built once per renderer.
func (r *Renderer) logEntries() []datedLogEntry {
	if r.logEntriesReady {
		return r.logEntriesCache
	}
	var out []datedLogEntry
	for _, s := range r.sections {
		for _, p := range s.Pages {
			if !isLogEntry(p) {
				continue
			}
			if d, ok := logDate(p); ok {
				out = append(out, datedLogEntry{page: p, date: d})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].date.Equal(out[j].date) {
			return out[i].date.Before(out[j].date)
		}
		return out[i].page.Slug < out[j].page.Slug
	})
	r.logEntriesCache = out
	r.logEntriesReady = true
	return out
}

// logNeighbors finds the entries either side of p in date order. ok is false
// when p is not a dated log entry, so the caller falls back to section order.
func (r *Renderer) logNeighbors(p Page) (older, newer *datedLogEntry, ok bool) {
	if !isLogEntry(p) {
		return nil, nil, false
	}
	entries := r.logEntries()
	for i := range entries {
		if entries[i].page.Slug != p.Slug {
			continue
		}
		if i > 0 {
			older = &entries[i-1]
		}
		if i+1 < len(entries) {
			newer = &entries[i+1]
		}
		return older, newer, true
	}
	return nil, nil, false
}

// formatLogDate writes a date for the page language. The renderer carries no
// locale data, so only English spells the month out; any other language gets
// the ISO date, which reads the same everywhere.
func formatLogDate(t time.Time, lang string, short bool) string {
	if !strings.EqualFold(lang, "en") && !strings.HasPrefix(strings.ToLower(lang), "en-") {
		return t.Format("2006-01-02")
	}
	if short {
		return t.Format("2 Jan 2006")
	}
	return t.Format("2 January 2006")
}

// logTimeHTML is a <time> element for a log date.
func logTimeHTML(class string, t time.Time, lang string, short bool) string {
	attr := ""
	if class != "" {
		attr = fmt.Sprintf(` class="%s"`, class)
	}
	return fmt.Sprintf(`<time%s datetime="%s">%s</time>`, attr, t.Format("2006-01-02"),
		template.HTMLEscapeString(formatLogDate(t, lang, short)))
}

const (
	prevnextArrowLeft  = `<svg class="prevnext-arrow" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M19 12H5m6-6-6 6 6 6"/></svg>`
	prevnextArrowRight = `<svg class="prevnext-arrow" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 12h14m-6-6 6 6-6 6"/></svg>`
)

// logPrevNextHTML renders the Older and Newer cards of a log entry. Older is
// the previous card and keeps the left column, so the newest entry shows only
// Older on the left and the oldest only Newer on the right.
func (r *Renderer) logPrevNextHTML(older, newer *datedLogEntry) string {
	if older == nil && newer == nil {
		return ""
	}
	lang := r.cfg.lang()
	title := func(p Page) string {
		if p.Title != "" {
			return p.Title
		}
		return p.Slug
	}
	sep := `<span class="prevnext-sep" aria-hidden="true">·</span>`
	var b strings.Builder
	b.WriteString(`<nav class="prevnext prevnext-log" aria-label="More in the change log">`)
	if older != nil {
		fmt.Fprintf(&b, `<a class="prevnext-card prevnext-prev" href="%s" rel="prev"><span class="prevnext-dir">%sOlder%s%s</span><span class="prevnext-title">%s</span></a>`,
			r.urlBase()+older.page.Slug+".html", prevnextArrowLeft, sep,
			logTimeHTML("", older.date, lang, true), template.HTMLEscapeString(title(older.page)))
	}
	if newer != nil {
		fmt.Fprintf(&b, `<a class="prevnext-card prevnext-next" href="%s" rel="next"><span class="prevnext-dir">Newer%s%s%s</span><span class="prevnext-title">%s</span></a>`,
			r.urlBase()+newer.page.Slug+".html", sep,
			logTimeHTML("", newer.date, lang, true), prevnextArrowRight, template.HTMLEscapeString(title(newer.page)))
	}
	b.WriteString(`</nav>`)
	return b.String()
}
