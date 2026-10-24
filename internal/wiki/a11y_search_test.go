package wiki

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// searchClient returns the shipped search modal client. Everything it pins is
// client-side, so the asset's own text is what a Go test can reach.
func searchClient(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(docsRoot(t), "internal", "wiki",
		"themes", "default", "assets", "search.js"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// jsFuncBody returns one function's body out of a flat client script. A guard is
// only worth asserting if it sits in the function that needs it: the same words
// anywhere else in the file would satisfy a plain substring check while the code
// that should call it stayed broken.
func jsFuncBody(t *testing.T, js, name string) string {
	t.Helper()
	start := strings.Index(js, "function "+name+"(")
	if start < 0 {
		t.Fatalf("search.js declares no function %s", name)
	}
	rest := js[start:]
	end := strings.Index(rest, "\n  }\n")
	if end < 0 {
		t.Fatalf("search.js has no end of body for %s", name)
	}
	return rest[:end]
}

// TestSearchClientInitialisesOnTheDocumentedIds is the regression guard for the
// modal's entry points. A renamed id leaves the client returning before it binds
// anything, and the theme still draws a dialog nothing can open.
func TestSearchClientInitialisesOnTheDocumentedIds(t *testing.T) {
	js := searchClient(t)
	for _, id := range []string{
		"searchBtn", "searchInput", "searchResults", "searchBackdrop",
		"searchCloseBtn", "searchScope", "searchGraph",
	} {
		if !strings.Contains(js, "getElementById('"+id+"')") {
			t.Errorf("search.js no longer looks up #%s, which the DOM contract requires", id)
		}
	}
	// Without the bail, an unknown theme throws on the first addEventListener
	// below and nothing on the page that follows it runs either.
	if !strings.Contains(js, "if (!backdrop || !input || !resultsContainer) {") {
		t.Error("search.js no longer bails out when the modal markup is absent")
	}
	for _, want := range []string{
		"backdrop.addEventListener('click'",
		"input.addEventListener('input'",
		"input.addEventListener('keydown'",
		"resultsContainer.addEventListener('mousemove'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("search.js binds no %q", want)
		}
	}
}

// TestSearchClientNamesTheInput covers a search box whose only name was its
// placeholder. A placeholder is a hint rather than a name: it disappears as soon
// as a reader types, and assistive tech announces it as a hint. The dialog
// around the input is already named by the template, so the input is named the
// same way.
func TestSearchClientNamesTheInput(t *testing.T) {
	js := searchClient(t)
	for _, want := range []string{
		// A name the markup already carries, by any of the three routes, wins.
		"input.getAttribute('aria-label')",
		"input.getAttribute('aria-labelledby')",
		"input.labels",
		"input.setAttribute('aria-label', 'Search documentation')",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("search.js has no %q, so a theme that replaces the template leaves the input unnamed", want)
		}
	}
	// A placeholder is a hint rather than a name, so reading it to decide whether
	// a real name exists would make the hint answer its own question.
	if strings.Contains(js, "getAttribute('placeholder')") {
		t.Error("search.js reads the placeholder as the input's name")
	}
}

// TestSearchClientTrapsTabInsideTheDialog covers a dialog that declares
// aria-modal. A keyboard reader takes that declaration as a promise that Tab
// stays inside, and the page behind the backdrop is the cost of breaking it. Go
// cannot drive the client, so each assertion names the defect it catches.
func TestSearchClientTrapsTabInsideTheDialog(t *testing.T) {
	js := searchClient(t)
	// The trap has to see the key before the browser acts on it, or focus has
	// already left the dialog.
	if !strings.Contains(js, "e.key === 'Tab'") {
		t.Error("search.js has no Tab branch in its keydown handler")
	}
	// Shift+Tab has to be caught as well, or the trap holds in one direction
	// only and a reader walking backwards still lands on the page behind.
	if !strings.Contains(js, "e.shiftKey") {
		t.Error("search.js does not distinguish Shift+Tab")
	}
	// A trap that ran with the dialog closed would steal Tab from every page.
	if !strings.Contains(js, "e.key === 'Tab' && backdrop.classList.contains('open')") {
		t.Error("the Tab trap is not conditioned on the dialog being open")
	}

	body := jsFuncBody(t, js, "trapTab")
	for _, want := range []string{
		// Declared inside the handler, so the list is rebuilt on every keypress.
		"var candidates = Array.prototype.filter.call(",
		// The rows are anchors with role="option" and the input is the only text
		// field, so all three shapes have to be candidates.
		"modal.querySelectorAll('button, a[href], input')",
		"hidden",
		"inert",
		"first.focus()",
		"last.focus()",
		// A control a stylesheet hides is still in the DOM, and focus() on it
		// does nothing: the trap would quietly stop at the edge.
		"offsetParent !== null",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the Tab trap has no %q", want)
		}
	}
	// The direction matters as much as the pair of calls. Each branch is pinned
	// as a whole: a reader walking backwards off the first element has to land on
	// the last, and forward off the last on the first, or the trap holds from one
	// side only while the other side still lets focus out.
	for _, want := range []string{
		"e.shiftKey && (active === first || !inside)) { e.preventDefault(); last.focus(); }",
		"!e.shiftKey && (active === last || !inside)) { e.preventDefault(); first.focus(); }",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the Tab trap does not cycle in both directions: no %q", want)
		}
	}
	// The results list is rebuilt on every keystroke, so a list of candidates
	// hoisted out of the handler would name the rows of an earlier render, which
	// are detached, and focus() on them does nothing: the trap would quietly
	// stop working after the first character typed.
	if strings.Count(js, "querySelectorAll('button, a[href], input')") != 1 {
		t.Error("the candidate query appears more than once, so it is no longer the handler's own")
	}
	// The scope has to be the dialog rather than the document: a query against
	// the document would return the header controls sitting behind the backdrop.
	if !strings.Contains(js, "querySelector('.search-modal')") {
		t.Error("search.js does not scope the dialog by its class, so the trap can reach the page behind it")
	}
	// The dialog closes on Escape and hands focus back, which the trap must not
	// disturb.
	if !strings.Contains(js, "lastActiveElement.focus()") {
		t.Error("closing the dialog no longer restores the element focused before it opened")
	}
}

// TestSearchClientAnnouncesTheResults covers a results area that rendered and
// said nothing. Before the fix the page held no live region at all, so a reader
// typed into the search box and heard no count and no results.
func TestSearchClientAnnouncesTheResults(t *testing.T) {
	js := searchClient(t)
	// Polite, never assertive: the region would speak on every keystroke, and an
	// assertive one cuts the reader off mid-sentence for each of them.
	if !strings.Contains(js, "setAttribute('aria-live', 'polite')") {
		t.Error("the results are not announced politely")
	}
	if strings.Contains(js, "assertive") {
		t.Error("an assertive live region interrupts the reader on every keystroke")
	}
	// The status node is taken out of the layout in the stylesheet rather than in
	// the client: display:none would silence it, and a rule in the asset is one a
	// theme can recolour. The class is what ties the two together.
	if !strings.Contains(js, "status.className = 'search-status'") {
		t.Error("the status node carries no class the stylesheet can reach")
	}
	css, err := os.ReadFile(filepath.Join(docsRoot(t), "internal", "wiki",
		"themes", "default", "assets", "wiki.css"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(css), ".search-status") {
		t.Error("the stylesheet has no .search-status rule, so the announced count is laid out on the page")
	}
	if !strings.Contains(string(css), "clip-path: inset(50%)") {
		t.Error(".search-status is not clipped out of the layout")
	}

	body := jsFuncBody(t, js, "announceResults")
	for _, want := range []string{
		// The count is the part a reader cannot see. It has to come from the
		// capped flat list, which is what a reader can reach, not from the number
		// of matches the index holds.
		"currentResults.length",
		// One result, several results and none are three different sentences.
		"'1 result'",
		"results",
		"No results",
		// In a tree the rows sit under a heading per wiki, which is where a row
		// came from, so the named wikis go with the count.
		"currentGroups",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the announcement does not carry %q", want)
		}
	}

	guard := jsFuncBody(t, js, "announce")
	// A keystroke that leaves the set alone, a change of case, has nothing new
	// to say, and repeating it would talk over the next character. The assertion
	// names the line rather than the identifiers it touches: announcingSet and a
	// bare return both survive elsewhere in this function, so a check for them
	// passes with the guard deleted.
	if !strings.Contains(guard, "if (key === announcedSet) return;") {
		t.Error("the live region is rewritten unconditionally, so an unchanged result set is announced again")
	}
	// The signature is built from the result URLs: a count alone cannot tell one
	// set of pages from another of the same size.
	if !strings.Contains(body, "currentResults[i].u") {
		t.Error("the announcement is keyed on something other than the result set")
	}
}

// TestSearchModalMarkupIsStable pins the rendered modal region. The input's name
// is the one part of it that a theme may restate, so the golden holds either
// side of that attribute; every other byte of the modal is the DOM contract the
// theme's CSS and its documented ids are written against.
func TestSearchModalMarkupIsStable(t *testing.T) {
	page := renderSearchPage(t)
	region := modalRegion(t, page)
	if got := stripInputName(region); got != wantModal {
		t.Errorf("the rendered modal region changed:\ngot:\n%s\nwant:\n%s", got, wantModal)
	}
	// The rows are written by the client, so the markup they carry is a contract
	// of the client and its stylesheet rather than of the template.
	for _, want := range []string{
		"id=\"searchInput\"", "id=\"searchResults\"", "id=\"searchBackdrop\"",
		"id=\"searchCloseBtn\"", "id=\"searchScope\"",
	} {
		if !strings.Contains(region, want) {
			t.Errorf("the modal region has no %s", want)
		}
	}
}

// renderSearchPage renders a one-page bundle and returns its HTML.
func renderSearchPage(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\n---\n# Home\n\nBody.\n")
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "wiki", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// modalRegion is the static markup between the backdrop's opening tag and the
// script tag that loads the client.
func modalRegion(t *testing.T, page string) string {
	t.Helper()
	start := strings.Index(page, `<div class="search-modal-backdrop"`)
	end := strings.Index(page, `<script src="/wiki/search.js`)
	if start < 0 || end < 0 || end < start {
		t.Fatalf("the rendered page has no search modal region")
	}
	return strings.TrimRight(page[start:end], "\n")
}

var inputNameHunk = regexp.MustCompile(`<input\b[^>]*?>`)

// stripInputName removes the input's aria-label, so the golden below is the modal
// region with or without the name the template carries.
func stripInputName(region string) string {
	return inputNameHunk.ReplaceAllStringFunc(region, func(tag string) string {
		return regexp.MustCompile(`\s*aria-label="[^"]*"`).ReplaceAllString(tag, "")
	})
}

// wantModal is the rendered modal region with the reported input hunk removed.
const wantModal = `<div class="search-modal-backdrop" id="searchBackdrop" aria-hidden="true">
  <div class="search-modal" role="dialog" aria-modal="true" aria-label="Search documentation">
    <div class="search-modal-header">
      <svg class="search-modal-icon" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>
      <input type="search" id="searchInput" class="search-input" placeholder="Search..." aria-controls="searchResults" autocomplete="off" autocorrect="off" autocapitalize="off" spellcheck="false">
      <button type="button" class="search-close-btn" id="searchCloseBtn" aria-label="Close search"><kbd class="kbd-esc">ESC</kbd></button>
    </div>
    <div class="search-modal-body" id="searchResults"></div>
    <div class="search-modal-footer">
      <button type="button" class="search-scope" id="searchScope" aria-pressed="true" hidden>All wikis</button>
      <button type="button" class="search-graph" id="searchGraph" hidden>Open in graph</button>
      <div class="search-footer-hint"><kbd class="kbd-key">↑</kbd><kbd class="kbd-key">↓</kbd> <span>Navigate</span></div>
      <div class="search-footer-hint"><kbd class="kbd-key">↵</kbd> <span>Select</span></div>
      <div class="search-footer-hint"><kbd class="kbd-key">ESC</kbd> <span>Close</span></div>
    </div>
  </div>
</div>`
