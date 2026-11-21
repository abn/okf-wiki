package wiki

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTagPageLogKindTags covers the detected log-kind tags: carried only by
// log entries and by at least a tenth of them. They leave the topic cloud for
// a Change log row, sorted by count, and a topic tag's Often with skips them.
func TestTagPageLogKindTags(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"),
		"---\ntitle: Home\ntype: Overview\ntags: [docs, graph]\n---\n# Home\n")
	mustWrite(t, filepath.Join(dir, "guide.md"),
		"---\ntitle: Guide\ntype: Guide\ntags: [docs, graph]\n---\n# Guide\n")
	if err := os.MkdirAll(filepath.Join(dir, "logs", "2026", "11"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Eleven entries: ten typed, one found by its path. #update is on three,
	// #log on all eleven, #docs also sits on a non-log page, and #rare is on
	// one entry, under a tenth of them.
	for i := 1; i <= 10; i++ {
		tags := "log"
		switch {
		case i <= 3:
			tags += ", update"
		case i == 4:
			tags += ", docs"
		case i == 5:
			tags += ", rare"
		}
		mustWrite(t, filepath.Join(dir, "logs", "2026", "11", fmt.Sprintf("%02d-entry.md", i)),
			fmt.Sprintf("---\ntitle: Entry %d\ntype: Log Entry\ntags: [%s]\n---\n# Entry %d\n", i, tags, i))
	}
	mustWrite(t, filepath.Join(dir, "logs", "2026", "11", "20-untyped.md"),
		"---\ntitle: Untyped entry\ntags: [log]\n---\n# Untyped entry\n")

	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	listings := r.tagListings()
	kinds := r.logKindTags(listings)
	if len(kinds) != 2 || !kinds["log"] || !kinds["update"] {
		t.Fatalf("log-kind tags = %v, want log and update", kinds)
	}

	page := func(slug string) string {
		t.Helper()
		for _, l := range listings {
			if l.Slug == slug {
				body, err := r.RenderTagPage(l, listings)
				if err != nil {
					t.Fatal(err)
				}
				return body
			}
		}
		t.Fatalf("no listing for %q", slug)
		return ""
	}
	block := func(body, open string) string {
		t.Helper()
		i := strings.Index(body, open)
		if i < 0 {
			t.Fatalf("tag page has no %q", open)
		}
		j := strings.Index(body[i:], "</div>")
		return body[i : i+j]
	}

	docs := page("docs")
	topics := block(docs, `<div class="tag-all">`)
	for _, want := range []string{"tags/docs.html", "tags/graph.html", "tags/rare.html"} {
		if !strings.Contains(topics, want) {
			t.Errorf("the topic cloud is missing %s", want)
		}
	}
	for _, unwanted := range []string{"tags/log.html", "tags/update.html"} {
		if strings.Contains(topics, unwanted) {
			t.Errorf("the topic cloud still carries %s", unwanted)
		}
	}
	logRow := block(docs, `<div class="tag-all tag-all-log">`)
	if !strings.Contains(logRow, `<span class="tag-all-label">Change log</span>`) {
		t.Error("the Change log row has no label")
	}
	logAt, updateAt := strings.Index(logRow, "tags/log.html"), strings.Index(logRow, "tags/update.html")
	if logAt < 0 || updateAt < 0 || logAt > updateAt {
		t.Errorf("the Change log row is not sorted by count:\n%s", logRow)
	}
	if often := block(docs, `<div class="tag-often">`); strings.Contains(often, "tags/log.html") {
		t.Errorf("a topic tag's Often with names a log-kind tag:\n%s", often)
	}

	update := page("update")
	if often := block(update, `<div class="tag-often">`); !strings.Contains(often, "tags/log.html") {
		t.Errorf("a log-kind tag's Often with dropped the other kinds:\n%s", often)
	}
}

// TestTagPageWithoutLogs keeps a bundle with no log entries on one cloud.
func TestTagPageWithoutLogs(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\ntags: [alpha]\n---\n# Home\n")
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	listings := r.tagListings()
	body, err := r.RenderTagPage(listings[0], listings)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body, "tag-all-log") {
		t.Error("a bundle with no log entries rendered a Change log row")
	}
}
