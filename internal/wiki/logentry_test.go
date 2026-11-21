package wiki

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLogNeighborsByDate covers the change log walk: Older and Newer follow the
// entry dates across a month and a year boundary, skip the month indexes, order
// two entries of one day by file name, and let a frontmatter date override the
// path.
func TestLogNeighborsByDate(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\n---\n# Home\n")
	entry := func(rel, title, extra string) {
		full := filepath.Join(dir, "logs", filepath.FromSlash(rel))
		mustMkdir(t, filepath.Dir(full))
		mustWrite(t, full, "---\ntype: Log Entry\ntitle: "+title+"\n"+extra+"---\n# "+title+"\n")
	}
	mustMkdir(t, filepath.Join(dir, "logs", "2025", "12"))
	mustWrite(t, filepath.Join(dir, "logs", "2025", "12", "index.md"), "# December\n")
	entry("2025/12/30-end-of-year.md", "End of year", "")
	entry("2026/01/04-early.md", "Early", "")
	entry("2026/01/28-late.md", "Late", "")
	entry("2026/02/01-b-second.md", "Second", "")
	entry("2026/02/01-a-first.md", "First", "")
	// The path says February, the frontmatter moves it before Late.
	entry("2026/02/09-moved.md", "Moved", "date: 2026-01-10\n")

	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range r.logEntries() {
		got = append(got, e.page.Title)
	}
	want := []string{"End of year", "Early", "Moved", "Late", "First", "Second"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("log order = %v, want %v", got, want)
	}

	page := func(slug string) Page {
		for _, s := range r.Sections() {
			for _, p := range s.Pages {
				if p.Slug == slug {
					return p
				}
			}
		}
		t.Fatalf("no page %s", slug)
		return Page{}
	}

	html := r.prevNextHTML(page("logs/2026/01/04-early"))
	for _, want := range []string{
		`<nav class="prevnext prevnext-log" aria-label="More in the change log">`,
		`href="/wiki/logs/2025/12/30-end-of-year.html" rel="prev"`,
		`Older<span class="prevnext-sep" aria-hidden="true">·</span><time datetime="2025-12-30">30 Dec 2025</time>`,
		`href="/wiki/logs/2026/02/09-moved.html" rel="next"`,
		`Newer<span class="prevnext-sep" aria-hidden="true">·</span><time datetime="2026-01-10">10 Jan 2026</time>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("log prevnext does not contain %s\n%s", want, html)
		}
	}

	newest := r.prevNextHTML(page("logs/2026/02/01-b-second"))
	if !strings.Contains(newest, "prevnext-prev") || strings.Contains(newest, "prevnext-next") {
		t.Errorf("newest entry should show only Older: %s", newest)
	}
	oldest := r.prevNextHTML(page("logs/2025/12/30-end-of-year"))
	if strings.Contains(oldest, "prevnext-prev") || !strings.Contains(oldest, "prevnext-next") {
		t.Errorf("oldest entry should show only Newer: %s", oldest)
	}

	meta := r.buildMeta(page("logs/2026/01/28-late"), "")
	if !strings.Contains(meta, `<span class="chip chip-type">Log Entry</span><time class="page-date" datetime="2026-01-28">28 January 2026</time>`) {
		t.Errorf("meta row should carry the date after the type chip: %s", meta)
	}
	if strings.Contains(r.buildMeta(page("index"), ""), "page-date") {
		t.Error("an ordinary page must not carry a date")
	}
}

// TestLogEntryDetection covers isLogEntry and logDate on their own: the type
// wins, an untyped page counts only at a dated path, and a month index is not
// an entry.
func TestLogEntryDetection(t *testing.T) {
	cases := []struct {
		page  Page
		entry bool
		date  string
	}{
		{Page{Type: "Log Entry", Slug: "notes/x"}, true, ""},
		{Page{Slug: "logs/2026/11/21-x"}, true, "2026-11-21"},
		{Page{Slug: "logs/2026/11/index"}, false, ""},
		{Page{Type: "Concept", Slug: "logs/2026/11/21-x"}, false, "2026-11-21"},
		{Page{Type: "log entry", Slug: "logs/2026/11/21-x", Date: "2026-11-20T09:30:00Z"}, true, "2026-11-20"},
		{Page{Type: "Log Entry", Slug: "logs/2026/11/21-x", Date: "soon"}, true, "2026-11-21"},
	}
	for _, c := range cases {
		if got := isLogEntry(c.page); got != c.entry {
			t.Errorf("isLogEntry(%+v) = %v, want %v", c.page, got, c.entry)
		}
		d, ok := logDate(c.page)
		got := ""
		if ok {
			got = d.Format("2006-01-02")
		}
		if got != c.date {
			t.Errorf("logDate(%+v) = %q, want %q", c.page, got, c.date)
		}
	}
	if got := formatLogDate(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), "de", false); got != "2026-11-01" {
		t.Errorf("a non-English page should get the ISO date, got %q", got)
	}
}
