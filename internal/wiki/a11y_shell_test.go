package wiki

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// a11yBundle writes a bundle of three pages in one section and two header
// links, and renders it. It returns the middle page's HTML, which is the page
// with both a previous and a next card, and the stylesheet the theme wrote
// beside it.
func a11yBundle(t *testing.T) (page, sheet string) {
	t.Helper()
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\n---\n# Home\n")
	for _, p := range []struct{ slug, title string }{
		{"guide/a", "Aye"}, {"guide/b", "Bee"}, {"guide/c", "Cee"},
	} {
		mustMkdir(t, filepath.Join(dir, filepath.Dir(p.slug)))
		mustWrite(t, filepath.Join(dir, p.slug+".md"),
			"---\ntitle: "+p.title+"\n---\n# "+p.title+"\n")
	}
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{
		Name: "t", Title: "Wiki",
		NavLinks: "Guide=guide/a.md,Docs=https://example.invalid/xyz",
	}})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(out, "wiki", "guide", "b.html"))
	if err != nil {
		t.Fatalf("guide/b.html was not written: %v", err)
	}
	css, err := os.ReadFile(filepath.Join(out, "wiki", "wiki.css"))
	if err != nil {
		t.Fatalf("wiki.css was not written: %v", err)
	}
	return string(body), string(css)
}

// TestCurrentPageIsAnnounced covers the rail's position mark: the class paints
// it for sighted readers and the attribute announces it to everyone else.
func TestCurrentPageIsAnnounced(t *testing.T) {
	page, _ := a11yBundle(t)

	if n := strings.Count(page, ariaCurrentMark); n != 1 {
		t.Errorf("page carries %d aria-current marks, want exactly 1:\n%s", n, page)
	}
	want := `<li class="active"><a href="/wiki/guide/b.html"` + ariaCurrentMark + `>Bee</a></li>`
	if !strings.Contains(page, want) {
		t.Errorf("the active rail row does not mark itself current, want\n%s", want)
	}
	if n := strings.Count(page, `class="active"`); n != 1 {
		t.Errorf("page marks %d rail rows active, want exactly 1", n)
	}
}

// TestCurrentMarkStaysOutOfTheRestOfTheShell covers the rest of the shell: the
// header links, the drawer's copy of them, the breadcrumb and the
// previous/next cards. Only the rail marks position, so anything else carrying
// it would announce a position that is not the current page.
func TestCurrentMarkStaysOutOfTheRestOfTheShell(t *testing.T) {
	page, _ := a11yBundle(t)

	for _, region := range []struct{ name, open, end string }{
		{"breadcrumb", `<nav class="breadcrumb" aria-label="Breadcrumb">`, "</nav>"},
		{"top nav", `<nav class="top-nav" aria-label="Site">`, "</nav>"},
		{"drawer links", `class="side-links">`, "</aside>"},
		{"previous and next", `<nav class="prevnext" aria-label="More in this section">`, "</nav>"},
	} {
		got := regionOf(t, page, region.open, region.end)
		if strings.Contains(got, "aria-current") {
			t.Errorf("the %s carries an aria-current mark:\n%s", region.name, got)
		}
	}

	// The current crumb stays a plain span. A mark on it would say nothing a
	// screen reader listens to, since the crumb is text rather than a control.
	crumb := regionOf(t, page, `<nav class="breadcrumb" aria-label="Breadcrumb">`, "</nav>")
	if !strings.Contains(crumb, `<span class="crumb-cur">Bee</span>`) {
		t.Errorf("the current crumb is not the plain span it was:\n%s", crumb)
	}
}

// TestActiveLinkIsMarkedInEveryRailShape covers every shape the rail can write
// a link in: a section title, a sub-directory group heading, a group that is
// only its own index, and an ordinary page row. Each is the reader's position
// when they are on that page, so each carries the mark.
func TestActiveLinkIsMarkedInEveryRailShape(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\n---\n# Home\n")
	mustMkdir(t, filepath.Join(dir, "guide"))
	mustWrite(t, filepath.Join(dir, "guide", "index.md"), "---\ntitle: Guide\n---\n# Guide\n")
	mustMkdir(t, filepath.Join(dir, "guide", "setup"))
	mustWrite(t, filepath.Join(dir, "guide", "setup", "index.md"), "---\ntitle: Setup\n---\n# Setup\n")
	mustWrite(t, filepath.Join(dir, "guide", "setup", "install.md"), "---\ntitle: Install\n---\n# Install\n")
	mustMkdir(t, filepath.Join(dir, "guide", "solo"))
	mustWrite(t, filepath.Join(dir, "guide", "solo", "index.md"), "---\ntitle: Solo\n---\n# Solo\n")

	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	render := func(t *testing.T, rel string) string {
		t.Helper()
		body, err := os.ReadFile(filepath.Join(out, "wiki", filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("%s was not written: %v", rel, err)
		}
		return string(body)
	}

	// The section title is the current page's link.
	section := render(t, "guide/index.html")
	sectionWant := `<a href="/wiki/guide/index.html" class="side-title-link active"` +
		ariaCurrentMark + `>Guide</a>`
	if !strings.Contains(section, sectionWant) {
		t.Errorf("the section title does not mark itself current, want\n%s", sectionWant)
	}
	if n := strings.Count(section, ariaCurrentMark); n != 1 {
		t.Errorf("section home carries %d marks, want exactly 1", n)
	}

	// A section with no child pages is a leaf row, and its title is the link.
	leafSection := render(t, "index.html")
	leafSectionWant := `<a href="/wiki/index.html" class="side-title-link active"` +
		ariaCurrentMark + `>Overview</a>`
	if !strings.Contains(leafSection, leafSectionWant) {
		t.Errorf("the leaf section title does not mark itself current, want\n%s", leafSectionWant)
	}

	// The group heading is the current page's link, and takes no active class,
	// so the mark is the only thing that says so.
	group := render(t, "guide/setup/index.html")
	groupWant := `<a href="/wiki/guide/setup/index.html"` + ariaCurrentMark + `>Setup</a>`
	if !strings.Contains(group, groupWant) {
		t.Errorf("the group heading does not mark itself current, want\n%s", groupWant)
	}
	if n := strings.Count(group, ariaCurrentMark); n != 1 {
		t.Errorf("group home carries %d marks, want exactly 1", n)
	}

	// A group with nothing but its own index is a leaf row inside the list.
	leaf := render(t, "guide/solo/index.html")
	leafWant := `<a class="active"` + ariaCurrentMark + ` href="/wiki/guide/solo/index.html">Solo</a>`
	if !strings.Contains(leaf, leafWant) {
		t.Errorf("the leaf group row does not mark itself current, want\n%s", leafWant)
	}

	// A page of a group is an ordinary rail row.
	inside := render(t, "guide/setup/install.html")
	insideWant := `<li class="active"><a href="/wiki/guide/setup/install.html"` +
		ariaCurrentMark + `>Install</a></li>`
	if !strings.Contains(inside, insideWant) {
		t.Errorf("the rail row does not mark itself current, want\n%s", insideWant)
	}
	if n := strings.Count(inside, ariaCurrentMark); n != 1 {
		t.Errorf("group page carries %d marks, want exactly 1", n)
	}
}

// TestRenderedLabelsSpeakInAuthoredCase covers the shouted labels: a
// stylesheet that transforms text to uppercase reaches a screen reader as
// uppercase text, so the look is drawn with small caps instead and the markup
// keeps the label as it was authored.
//
// The two labels that set their own width, the wiki badge and the lightbox
// pill, keep the transform. Small caps cost them about a third of their text
// run, and they already render their labels in full caps, so the transform is
// a no-op on them and dropping it buys nothing.
func TestRenderedLabelsSpeakInAuthoredCase(t *testing.T) {
	page, sheet := a11yBundle(t)

	// Every label that is transformed must be one of the two that set their own
	// width, so the assertion names the selectors rather than counting.
	transformed := strings.Count(sheet, "text-transform: uppercase")
	if transformed > 2 {
		t.Errorf("the stylesheet shouts %d labels, want only the badge and the pill", transformed)
	}
	for _, want := range []string{".site-badge", ".wiki-lbx-pill"} {
		rule := ruleOf(t, sheet, want)
		if !strings.Contains(rule, "text-transform: uppercase") {
			t.Errorf("%s no longer carries the transform, so a badge drawn small caps is a third narrower", want)
		}
	}
	if n := strings.Count(sheet, "font-variant-caps"); n != 5 {
		t.Errorf("the stylesheet carries %d font-variant-caps rules, want the five block labels", n)
	}
	if !strings.Contains(page, `>Previous<`) {
		t.Errorf("the previous card lost its authored label:\n%s", page)
	}
	if strings.Contains(page, "PREVIOUS") {
		t.Error("the label was uppercased in the markup rather than drawn by the stylesheet")
	}
}

// ruleOf returns the declaration block of one selector, so an assertion is
// about that rule rather than about any line in the file.
func ruleOf(t *testing.T, sheet, selector string) string {
	t.Helper()
	i := strings.Index(sheet, selector+" {")
	if i < 0 {
		t.Fatalf("no rule for %s", selector)
	}
	rest := sheet[i+len(selector)+2:]
	end := strings.Index(rest, "\n}")
	if end < 0 {
		t.Fatalf("unterminated rule for %s", selector)
	}
	return rest[:end]
}

// regionOf returns the inner HTML of the element opening with openTag, so a test
// can assert about one part of the shell without matching the same markup
// elsewhere on the page.
func regionOf(t *testing.T, html, openTag, closeTag string) string {
	t.Helper()
	i := strings.Index(html, openTag)
	if i < 0 {
		t.Fatalf("no %s in:\n%s", openTag, html)
	}
	rest := html[i+len(openTag):]
	j := strings.Index(rest, closeTag)
	if j < 0 {
		t.Fatalf("unterminated %s in:\n%s", openTag, html)
	}
	return rest[:j]
}
