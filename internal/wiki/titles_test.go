package wiki

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestTitleWarnings covers the pages a render names as having a title that
// cannot work as a name: one longer than a line, and one that carries a line
// break. A title at the limit still fits, so it is not named.
func TestTitleWarnings(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\n---\n# Home\n")
	mustWrite(t, filepath.Join(dir, "limit.md"),
		"---\ntitle: "+strings.Repeat("n", TitleWarningMax)+"\n---\n# Limit\n")
	mustWrite(t, filepath.Join(dir, "long.md"),
		"---\ntitle: "+strings.Repeat("n", TitleWarningMax+1)+"\n---\n# Long\n")
	mustWrite(t, filepath.Join(dir, "broken.md"),
		"---\ntitle: \"first\\nsecond\"\n---\n# Broken\n")

	r := newGraphRenderer(t, dir)
	got := r.TitleWarnings()
	if len(got) != 2 {
		t.Fatalf("TitleWarnings() = %+v, want two warnings", got)
	}
	byPage := map[string]TitleWarning{}
	for _, warning := range got {
		byPage[warning.Page] = warning
	}
	if warning, ok := byPage["long"]; !ok {
		t.Errorf("warnings = %+v, want the long page", got)
	} else if warning.Length != TitleWarningMax+1 || warning.LineBreak {
		t.Errorf("long warning = %+v, want %d characters and no line break", warning, TitleWarningMax+1)
	}
	if warning, ok := byPage["broken"]; !ok {
		t.Errorf("warnings = %+v, want the page with a line break", got)
	} else if !warning.LineBreak {
		t.Errorf("broken warning = %+v, want a line break", warning)
	}
	if _, ok := byPage["limit"]; ok {
		t.Errorf("a title at the limit was reported: %+v", got)
	}
}

// TestTitleWarningsAcrossTree covers the tree's collection: each rendered wiki
// contributes its own pages under its path, and a page is reported once.
func TestTitleWarningsAcrossTree(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "alpha"))
	mustWrite(t, filepath.Join(dir, "alpha", "index.md"),
		"---\ntitle: "+strings.Repeat("a", TitleWarningMax+1)+"\n---\n# Alpha\n")
	mustMkdir(t, filepath.Join(dir, "beta"))
	mustWrite(t, filepath.Join(dir, "beta", "index.md"), "---\ntitle: Beta\n---\n# Beta\n")

	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "okf-wiki"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	got := r.TitleWarnings()
	if len(got) != 1 {
		t.Fatalf("TitleWarnings() = %+v, want one warning for alpha", got)
	}
	if got[0].Page != "alpha/index" || got[0].Length != TitleWarningMax+1 {
		t.Errorf("warning = %+v, want alpha's index page and its length", got[0])
	}
}
