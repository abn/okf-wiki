package wiki

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestReadPageSectionAndContext(t *testing.T) {
	site := searchFixture(t)
	c, err := LoadSearchCorpus(site, "/wiki/")
	if err != nil {
		t.Fatal(err)
	}
	page, err := c.ReadPage("page:guide/install", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page.Content, "Run the installer") || page.ContentHash == "" {
		t.Errorf("page = %+v", page)
	}
	if page.Truncated {
		t.Error("full page was truncated at the default budget")
	}
	small, err := c.ReadPage("page:guide/install", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !small.Truncated || small.EstimatedTokens > 1 {
		t.Errorf("small page = %+v, want a one-token truncation", small)
	}

	section, err := c.ReadSection("page:guide/install", "install", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(section.Text), "installer") || section.URL != "/wiki/guide/install.html#install" {
		t.Errorf("section = %+v", section)
	}
	if section.ContentHash == "" || section.MarkdownURL != "/wiki/guide/install.md" {
		t.Errorf("section citation = %+v", section)
	}

	context, err := c.ContextFor("install", SearchOptions{Limit: 10}, 20)
	if err != nil {
		t.Fatal(err)
	}
	if context.Total == 0 || len(context.Results) == 0 {
		t.Fatalf("context = %+v", context)
	}
	if context.EstimatedTokens > 20 || !context.Truncated {
		t.Errorf("context budget = %+v, want a truncated pack within 20 tokens", context)
	}
	if context.Results[0].PageID == "" || context.Results[0].Text == "" {
		t.Errorf("context citation = %+v", context.Results[0])
	}
}

func TestReadPageOffline(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "# Home\n\nHello.\n")
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	c, err := LoadSearchCorpus(filepath.Join(out, "wiki"), "/wiki/")
	if err != nil {
		t.Fatal(err)
	}
	page, err := c.ReadPage("page:index", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page.Content, "Hello") {
		t.Errorf("offline page = %+v", page)
	}
}
