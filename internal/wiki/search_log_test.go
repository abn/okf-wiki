package wiki

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestSearchClientGroupsTheChangeLog covers a single bundle whose log entries
// outranked its pages. Go cannot drive the client, so each assertion names the
// part of the grouping it holds in place.
func TestSearchClientGroupsTheChangeLog(t *testing.T) {
	js := searchClient(t)
	run := jsFuncBody(t, js, "runSearch")
	for _, want := range []string{
		// Pages and log entries are split before the cap, so a log hit never
		// takes a page's row.
		"var docs = matches.filter(function (m) { return !m.entry.l; });",
		"var logs = matches.filter(function (m) { return !!m.entry.l; });",
		// The log group is capped only when pages match too.
		"docs.length ? MAX_LOGS : MAX_FLAT",
		// Either kind alone stays flat, with no headings.
		"if (docTake && logTake) {",
		"label: 'Pages'",
		"label: 'Change log'",
		"more: logs.length - logTake",
	} {
		if !strings.Contains(run, want) {
			t.Errorf("runSearch has no %q", want)
		}
	}
	if !strings.Contains(js, "var MAX_LOGS = 3;") {
		t.Error("the change log group is no longer capped at three rows")
	}
	// In a tree, a wiki's log entries sort after its pages before the per-wiki
	// cap is taken.
	group := jsFuncBody(t, js, "groupMatches")
	if !strings.Contains(group, ".concat(grp.rows.filter(function (e) { return !!e.l; }))") {
		t.Error("a wiki group no longer puts its log entries after its pages")
	}
	// The live region says how many rows came from the log, since the headings
	// are not read with the count.
	if !strings.Contains(jsFuncBody(t, js, "announceResults"), "' from the change log'") {
		t.Error("the announcement does not count the change log rows")
	}
	// The group is named by its label only, so the overflow count is not read
	// as part of its name.
	render := jsFuncBody(t, js, "renderResults")
	for _, want := range []string{
		`'<div class="search-group-heading"><span id="search-group-' + g + '">' + escapeHTML(grp.label) + '</span>' + more + '</div>'`,
		"Try fewer words, a page title, or # to search tags.",
		"quickLinksHTML()",
	} {
		if !strings.Contains(render, want) {
			t.Errorf("renderResults has no %q", want)
		}
	}
	if strings.Contains(js, "Try a host, a command, or a topic.") {
		t.Error("the empty state still suggests a host or a command")
	}
	// The title leads the row; the context line follows it.
	row := jsFuncBody(t, js, "resultItemHTML")
	if strings.Index(row, `search-result-title`) > strings.Index(row, `search-result-top`) {
		t.Error("a result row no longer leads with its title")
	}
}

// TestSearchIndexFlagsLogEntries covers the page table fields the client groups
// by: a log entry carries l and its ISO date, a page carries neither, and the
// index names the log tag page when an entry carries the tag.
func TestSearchIndexFlagsLogEntries(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\n---\n# Home\n\nRendering.\n")
	mustMkdir(t, filepath.Join(dir, "logs", "2026", "11"))
	mustWrite(t, filepath.Join(dir, "logs", "2026", "11", "01-render.md"),
		"---\ntype: Log Entry\ntitle: Render\ntags: [log]\n---\n# Render\n\nRendering.\n")
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	index := r.BuildSearchIndex()
	for _, p := range index.Pages {
		switch p.Title {
		case "Home":
			if p.Log != 0 || p.Date != "" {
				t.Errorf("a page is flagged as a log entry: %+v", p)
			}
		case "Render":
			if p.Log != 1 || p.Date != "2026-11-01" {
				t.Errorf("the log entry carries l=%d dt=%q, want 1 and 2026-11-01", p.Log, p.Date)
			}
		}
	}
	if index.LogTag != "tags/log.html" {
		t.Errorf("logTag = %q, want tags/log.html", index.LogTag)
	}
	raw, err := r.SearchIndexJSON()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"l":1`, `"dt":"2026-11-01"`, `"logTag":"tags/log.html"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("search-index.json has no %s", want)
		}
	}
}
