package wiki

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// flatEntry is the expanded form of a search entry: the page table's fields
// joined back onto the entry, which is what the client builds at load and what
// the pre-page-table index stored per entry.
type flatEntry struct {
	URL     string
	PageURL string
	Title   string
	Doc     string
	Section string
	Content string
	Site    string
	SiteKey string
}

// flattenIndex expands a search index the way the client does, resolving each
// entry's page and rebuilding its deep link, so a test can assert on what a
// reader would get.
func flattenIndex(t *testing.T, index SearchIndex) []flatEntry {
	t.Helper()
	out := make([]flatEntry, len(index.Entries))
	for i, e := range index.Entries {
		if e.Page < 0 || e.Page >= len(index.Pages) {
			t.Fatalf("entry %d refers to page %d, out of range of %d pages", i, e.Page, len(index.Pages))
		}
		p := index.Pages[e.Page]
		url := p.URL
		if e.Anchor != "" {
			url += "#" + e.Anchor
		}
		out[i] = flatEntry{
			URL: url, PageURL: p.URL, Title: e.Title, Doc: p.Title,
			Section: e.Section, Content: e.Content, Site: p.Site, SiteKey: p.SiteKey,
		}
	}
	return out
}

func readSearchIndex(t *testing.T, path string) SearchIndex {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var index SearchIndex
	if err := json.Unmarshal(raw, &index); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return index
}

// readSearchContent reads the companion file that carries the entry texts.
func readSearchContent(t *testing.T, path string) SearchContent {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var content SearchContent
	if err := json.Unmarshal(raw, &content); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return content
}

// readSearchIndexWithContent reads a wiki's light index and the text companion
// beside it, attaching each text to its entry by position, which is what the
// client does when the background fetch arrives.
func readSearchIndexWithContent(t *testing.T, dir string) SearchIndex {
	t.Helper()
	index := readSearchIndex(t, filepath.Join(dir, "search-index.json"))
	content := readSearchContent(t, filepath.Join(dir, "search-content.json"))
	if content.Count != len(index.Entries) || len(content.Texts) != len(index.Entries) {
		t.Fatalf("search content carries %d texts (n=%d) for %d entries",
			len(content.Texts), content.Count, len(index.Entries))
	}
	for i := range index.Entries {
		index.Entries[i].Content = content.Texts[i]
	}
	return index
}

func TestLinkPath(t *testing.T) {
	withRepo := &Renderer{cfg: Config{Repo: "/repo-root"}}
	noRepo := &Renderer{}
	relocated := &Renderer{cfg: Config{Repo: "/repo-root", Base: "/docs/deep/"}}

	cases := []struct {
		name string
		r    *Renderer
		from string
		href string
		want string
	}{
		{"internal top-level", withRepo, "index", "usage/cli.md", "/wiki/usage/cli.html"},
		{"bundle-relative", withRepo, "index", "/usage/cli.md", "/wiki/usage/cli.html"},
		{"bundle-relative from nested", withRepo, "usage/cli", "/design/index.md", "/wiki/design/index.html"},
		{"bundle-relative with fragment", withRepo, "usage/cli", "/design/index.md#x", "/wiki/design/index.html#x"},
		{"internal sibling", withRepo, "usage/cli", "../design/index.md", "/wiki/design/index.html"},
		{"internal with fragment", withRepo, "usage/cli", "../design/index.md#x", "/wiki/design/index.html#x"},
		{"bundle-relative asset", withRepo, "usage/cli", "/assets/diagram.png", "/wiki/assets/diagram.png"},
		{"protocol-relative stays", withRepo, "usage/cli", "//example.com/a", "//example.com/a"},
		{"pure fragment", withRepo, "index", "#top", "#top"},
		{"external http", withRepo, "index", "https://example.com/a", "https://example.com/a"},
		{"mailto", withRepo, "index", "mailto:a@b.c", "mailto:a@b.c"},
		{"repo escape", withRepo, "usage/cli", "../../roles/x/tasks/main.yml", "/repo/roles/x/tasks/main.yml"},
		{"repo root file", withRepo, "contribution/index", "../../AGENTS.md", "/repo/AGENTS.md"},
		{"escape without repo keeps href", noRepo, "usage/cli", "../../AGENTS.md", "../../AGENTS.md"},
		{"relocated base", relocated, "index", "usage/cli.md", "/docs/deep/usage/cli.html"},
		{"relocated repo escape stays put", relocated, "usage/cli", "../../AGENTS.md", "/repo/AGENTS.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.r.linkPath(tc.from, tc.href); got != tc.want {
				t.Fatalf("linkPath(%q, %q) = %q, want %q", tc.from, tc.href, got, tc.want)
			}
		})
	}
}

func TestBaseNormalisation(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "/wiki/"},
		{"/wiki/", "/wiki/"},
		{"wiki", "/wiki/"},
		{"/docs/deep", "/docs/deep/"},
		{"docs/", "/docs/"},
	}
	for _, tc := range cases {
		if got := (Config{Base: tc.in}).base(); got != tc.want {
			t.Errorf("Config{Base: %q}.base() = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// lang mirrors base: an unset or blank Lang resolves to DefaultLang, so a bundle
// that says nothing still declares a language, and a surrounding space never
// reaches the attribute.
func TestLangResolution(t *testing.T) {
	cases := []struct{ lang, want string }{
		{"", "en"},
		{"   ", "en"},
		{"de", "de"},
		{"  de  ", "de"},
		{"pt-BR", "pt-BR"},
	}
	for _, tc := range cases {
		if got := (Config{Lang: tc.lang}).lang(); got != tc.want {
			t.Errorf("Config{Lang: %q}.lang() = %q, want %q", tc.lang, got, tc.want)
		}
	}
}

func TestSiteDirFollowsBase(t *testing.T) {
	got, err := Config{Base: "/docs/deep/"}.SiteDir("/out")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join("/out", "docs", "deep"); got != want {
		t.Fatalf("SiteDir = %q, want %q", got, want)
	}
	if _, err := (Config{Base: "/../etc/"}).SiteDir("/out"); err == nil {
		t.Fatal("a base with a .. segment must be rejected")
	}
}

// writeTheme materialises a theme directory in a temp dir and returns its path.
func writeTheme(t *testing.T, manifest string, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "theme.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	for rel, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestDefaultThemeResolves(t *testing.T) {
	th, err := ResolveTheme("")
	if err != nil {
		t.Fatal(err)
	}
	if th.Source != "embedded" {
		t.Errorf("Source = %q, want embedded", th.Source)
	}
	if th.Name == "" || th.Version == "" {
		t.Errorf("the embedded theme must name and version itself, got %q %q", th.Name, th.Version)
	}
	have := map[string]bool{}
	for _, f := range th.files {
		have[f.Path] = true
	}
	for _, want := range []string{"wiki.css", "tokens.css", "fonts.css", "search.js", "diagrams.js", "brand-mark.svg", "favicon.svg", "fonts/inter/inter-latin.woff2"} {
		if !have[want] {
			t.Errorf("the default theme is missing %s", want)
		}
	}
	if len(th.AssetVersion()) != 10 {
		t.Errorf("AssetVersion = %q, want 10 hex chars", th.AssetVersion())
	}
}

// TestDefaultThemeAvoidsADarkFlash guards the two things that stop a dark page
// from painting white during a cross-document navigation: the ground is set on
// html as well as body, because the canvas the browser paints between documents
// is the root background, and the theme is resolved before the body rather than
// after first paint.
func TestDefaultThemeAvoidsADarkFlash(t *testing.T) {
	th, err := ResolveTheme("")
	if err != nil {
		t.Fatal(err)
	}
	assets := map[string]string{}
	for _, f := range th.files {
		assets[f.Path] = string(f.Data)
	}
	shell := th.templateText
	if !strings.Contains(assets["wiki.css"], "html { scroll-behavior: smooth; background: var(--color-bg); }") {
		t.Error("html must paint the ground, not only body, or the canvas flashes white")
	}
	// The pre-paint script sets color-scheme so the browser's own default canvas
	// is dark before the stylesheet applies, for a reader with nothing pinned.
	if !strings.Contains(shell, "style.colorScheme =") {
		t.Error("the pre-paint script must set color-scheme on the root")
	}
	css := strings.Index(shell, "wiki.css?v=")
	script := strings.Index(shell, "wiki-theme")
	body := strings.Index(shell, "<body>")
	if css < 0 || script < 0 || body < 0 {
		t.Fatal("the shell is missing the stylesheet, the theme script or the body")
	}
	if !(css < script && script < body) {
		t.Error("the theme must be resolved after the stylesheet and before the body")
	}
	// The wordmark's mark is an <img> the browser would discover only after the
	// stylesheets and the body, so it paints a step late. It is preloaded from
	// the head, and the <img> must request the same URL the preload names, or
	// the hint is a second fetch rather than a cache hit.
	pre := strings.Index(shell, `rel="preload"`)
	logo := strings.Index(shell, `class="brand-mark"`)
	if pre < 0 || logo < 0 {
		t.Fatal("the shell is missing the brand-mark preload or the img")
	}
	if pre > logo {
		t.Error("the brand-mark preload must come before the img it warms")
	}
	if !strings.Contains(shell, `href="{{.Base}}brand-mark.svg?v={{.AssetVersion}}"`) {
		t.Error("the preload does not carry the asset version")
	}
	if !strings.Contains(shell, `src="{{.Base}}brand-mark.svg?v={{.AssetVersion}}"`) {
		t.Error("the img does not request the preloaded URL, so the preload is wasted")
	}
}

// TestDefaultThemeSearchInputCanShrink pins the CSS that keeps the search modal
// inside a phone viewport. The header is a flex row, and a flex item's
// min-width defaults to auto, so without min-width: 0 the input's intrinsic
// width floors the row and the close control is laid out past the modal edge.
// Go tests cannot measure browser layout, so this asserts the rule's text.
func TestDefaultThemeSearchInputCanShrink(t *testing.T) {
	th, err := ResolveTheme("")
	if err != nil {
		t.Fatal(err)
	}
	var css string
	for _, f := range th.files {
		if f.Path == "wiki.css" {
			css = string(f.Data)
		}
	}
	if css == "" {
		t.Fatal("the default theme has no wiki.css")
	}
	rule := regexp.MustCompile(`\.search-input\s*\{([^}]*)\}`).FindStringSubmatch(css)
	if rule == nil {
		t.Fatal("wiki.css has no .search-input rule")
	}
	if !regexp.MustCompile(`min-width:\s*0(?:px)?\b`).MatchString(rule[1]) {
		t.Error("the .search-input rule must set min-width: 0 so the flex item can shrink and the modal header fits a phone viewport")
	}
}

// TestDefaultThemeSearchSharesTheInFlightFetch pins the guard that keeps a
// keystroke during the first index load from wedging the tab. Resolving with
// searchIndex while it is still null sends runSearch straight back into
// fetchIndex from its own .then; both run as microtasks, so the chain never
// yields and the request it waits on can never settle. The guard must instead
// hand back the one in-flight request, and a failed load must leave an empty
// catalogue rather than a null one, or a keystroke restarts the loop through the
// failure path. Go tests cannot run the client, so this asserts the guard's
// shape.
func TestDefaultThemeSearchSharesTheInFlightFetch(t *testing.T) {
	th, err := ResolveTheme("")
	if err != nil {
		t.Fatal(err)
	}
	var js string
	for _, f := range th.files {
		if f.Path == "search.js" {
			js = string(f.Data)
		}
	}
	if js == "" {
		t.Fatal("the default theme has no search.js")
	}
	// A boolean loading flag is what made the second caller resolve early, so
	// its return pins the regression.
	if strings.Contains(js, "isLoading") || strings.Contains(js, "tagsLoading") {
		t.Error("search.js still gates a fetch on a loading flag, so a caller during the load is resolved with a null index and spins runSearch on the microtask queue")
	}
	// The in-flight request is memoized under a promise, and the guard returns
	// that promise rather than one resolved with the not-yet-loaded index.
	for _, guard := range []string{
		"if (searchIndexPromise) return searchIndexPromise;",
		"if (tagsPromise) return tagsPromise;",
	} {
		if !strings.Contains(js, guard) {
			t.Errorf("search.js has no %q, so a concurrent caller is not handed the in-flight fetch", guard)
		}
	}
	// The failure path must not leave a catalogue null: runSearch would read
	// that as "still loading" and retry on every keystroke. Both the text index
	// and the tag catalogue take this path.
	for _, empty := range []string{"searchIndex = [];", "tagsIndex = [];"} {
		if !strings.Contains(js, empty) {
			t.Errorf("search.js must leave an empty catalogue on a failed load (%s), not a null one, or the client retries on every keystroke", empty)
		}
	}
}

// TestDefaultThemeSearchLowercasesOnceAndDebounces pins the two performance
// changes to the client. The scoring loop reads a lowercased mirror of the
// index instead of lowercasing four fields of every entry on every keystroke,
// and the input listener runs one search a short pause after the last
// keystroke. Both are behavioural no-ops: the mirror is built at load and the
// same ranking reads it, so the results and their order are unchanged. Go tests
// cannot run the client, so this asserts the shape and that the scoring loop
// carries no lowercasing of its own.
func TestDefaultThemeSearchLowercasesOnceAndDebounces(t *testing.T) {
	th, err := ResolveTheme("")
	if err != nil {
		t.Fatal(err)
	}
	var js string
	for _, f := range th.files {
		if f.Path == "search.js" {
			js = string(f.Data)
		}
	}
	if js == "" {
		t.Fatal("the default theme has no search.js")
	}

	// The mirror is built on the load path that assigns the index, and the
	// failure path leaves an empty mirror beside the empty index.
	for _, want := range []string{
		"function buildLowerIndex(index) {",
		"searchIndexLower = buildLowerIndex(searchIndex);",
		"searchIndexLower = [];",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("search.js has no %q, so the lowercased mirror is not built with the index", want)
		}
	}

	// The scoring loop reads the mirror, content through the lazy per-entry
	// helper, and lowercases nothing itself.
	start := strings.Index(js, "function runSearch()")
	end := strings.Index(js, "function groupMatches")
	if start < 0 || end <= start {
		t.Fatal("search.js does not have the expected runSearch and groupMatches functions")
	}
	runSearch := js[start:end]
	// The query itself is still lowercased once per call. What must be gone is
	// the per-entry lowercasing, which repeats the work for the whole index on
	// every keystroke. buildLowerIndex holds those expressions now.
	for _, gone := range []string{
		"entry.t ? entry.t.toLowerCase()",
		"entry.d ? entry.d.toLowerCase()",
		"entry.s ? entry.s.toLowerCase()",
		"entry.c ? entry.c.toLowerCase()",
	} {
		if strings.Contains(runSearch, gone) {
			t.Errorf("runSearch still lowercases an entry field (%q), so the work repeats on every keystroke instead of once when the index loads", gone)
		}
	}
	for _, want := range []string{
		"var lower = searchIndexLower[i];",
		"var contentLower = lowerContent(i);",
	} {
		if !strings.Contains(runSearch, want) {
			t.Errorf("runSearch has no %q, so it does not read the precomputed mirror", want)
		}
	}

	// The content field is the large one, so it is lowercased once per entry on
	// first scan rather than copied whole at load.
	if !strings.Contains(js, "function lowerContent(i) {") {
		t.Error("search.js has no lowerContent, so the content field is not lowercased once per entry")
	}

	// Typing is debounced. The key listener stays separate, so Escape acts at
	// once, and the arrows and Enter flush a pending search first.
	for _, want := range []string{
		"input.addEventListener('input', scheduleSearch);",
		"inputTimer = setTimeout(function () {",
		"function flushSearch() {",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("search.js has no %q, so the debounce or its flush path is missing", want)
		}
	}
	if strings.Contains(js, "input.addEventListener('input', runSearch);") {
		t.Error("search.js still binds the input listener to runSearch, so every keystroke scores the whole index")
	}
}

// TestDefaultThemeSearchStopsOnClose pins the close paths. A debounced search
// or a fetch continuation can outlive the dialog, and writing results then
// speaks into the live region of a hidden dialog, so runSearch has to run
// nothing while the dialog is closed. The debounce handle is cleared when the
// timer fires, so the arrows and Enter do not re-run a search that has already
// run. Go cannot run the client, so the guarded function bodies are what the
// test asserts.
func TestDefaultThemeSearchStopsOnClose(t *testing.T) {
	th, err := ResolveTheme("")
	if err != nil {
		t.Fatal(err)
	}
	var js string
	for _, f := range th.files {
		if f.Path == "search.js" {
			js = string(f.Data)
		}
	}
	if js == "" {
		t.Fatal("the default theme has no search.js")
	}

	// The guard sits in runSearch itself, the one place every debounced and
	// deferred caller lands, so a continuation cannot write into a closed
	// dialog.
	run := jsFuncBody(t, js, "runSearch")
	if !strings.Contains(run, "if (!backdrop.classList.contains('open')) return;") {
		t.Error("runSearch does not return while the dialog is closed, so a debounced or deferred search writes into the hidden live region")
	}

	// flushSearch acts on inputTimer alone, so the timer has to clear the handle
	// as it fires; otherwise a search that already ran looks pending and runs
	// again on the next arrow or Enter.
	sched := jsFuncBody(t, js, "scheduleSearch")
	if !strings.Contains(sched, "inputTimer = null;") {
		t.Error("scheduleSearch leaves the timer handle set after it fires, so flushSearch re-runs the search")
	}
}

// TestDefaultThemeSearchExpandsPageTable pins the client's read of the index
// format: the on-disk shape is a page table and entries that refer to it, and
// the client expands it at load into the flat entry objects the scoring and
// display code reads. Go cannot run the client, so the shape of the expansion
// is what the test asserts, together with the deep link it rebuilds.
func TestDefaultThemeSearchExpandsPageTable(t *testing.T) {
	th, err := ResolveTheme("")
	if err != nil {
		t.Fatal(err)
	}
	var js string
	for _, f := range th.files {
		if f.Path == "search.js" {
			js = string(f.Data)
		}
	}
	if js == "" {
		t.Fatal("the default theme has no search.js")
	}

	// The expansion is built from the page table and the entries, keyed by the
	// entry's page index, and the load path assigns its result to the index the
	// rest of the file reads.
	for _, want := range []string{
		"function expandIndex(data) {",
		"var pages = (data && data.pages) || [];",
		"var entries = (data && data.entries) || [];",
		"var page = pages[entry.g] || {};",
		"searchIndex = expandIndex(data);",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("search.js has no %q, so the page table is not expanded into entries", want)
		}
	}

	// The deep link is the page url plus the entry's anchor, an empty anchor
	// meaning the page url alone, which is what the old per-entry u held.
	expand := jsFuncBody(t, js, "expandIndex")
	for _, want := range []string{
		"u: entry.a ? url + '#' + entry.a : url,",
		"p: url,",
		"d: page.t,",
	} {
		if !strings.Contains(expand, want) {
			t.Errorf("expandIndex has no %q, so the entry it builds is not the old one", want)
		}
	}
}

// TestDefaultThemeSearchLoadsContentInTheBackground pins the split on the
// client: the light index renders results at once, the entry texts are fetched
// separately and joined onto the entries when they arrive, and the join re-runs
// an open query so content matches appear without a keystroke. Go cannot run the
// client, so this asserts the shape of the background load.
func TestDefaultThemeSearchLoadsContentInTheBackground(t *testing.T) {
	th, err := ResolveTheme("")
	if err != nil {
		t.Fatal(err)
	}
	var js string
	for _, f := range th.files {
		if f.Path == "search.js" {
			js = string(f.Data)
		}
	}
	if js == "" {
		t.Fatal("the default theme has no search.js")
	}

	// The companion file is fetched beside the index, under the same base and
	// cache-busting version the index was fetched from.
	if !strings.Contains(js, "fetch(searchBase + 'search-content.json' + v)") {
		t.Error("search.js does not fetch the entry texts from the content companion")
	}
	// The light index starts the text load once it resolves, so the open, hover
	// and focus triggers that load the index also start the texts.
	indexBody := jsFuncBody(t, js, "fetchIndex")
	if !strings.Contains(indexBody, "fetchContent();") {
		t.Error("the index load does not start the background text load")
	}
	// The in-flight request is memoized, so a second trigger does not start a
	// second fetch.
	if !strings.Contains(js, "if (searchContentPromise) return searchContentPromise;") {
		t.Error("search.js has no guard against a second content fetch")
	}

	// The texts are joined by position, and a count that does not match the
	// index is ignored whole rather than misattached.
	apply := jsFuncBody(t, js, "applyContent")
	for _, want := range []string{
		"n !== searchIndex.length",
		"texts.length !== searchIndex.length",
		"searchIndex[i].c = texts[i] || '';",
		"searchIndexLower = buildLowerIndex(searchIndex);",
		"runSearch();",
	} {
		if !strings.Contains(apply, want) {
			t.Errorf("applyContent has no %q, so the texts are not joined and the search re-run", want)
		}
	}
	// The re-run is marked, so the announcement guard can tell it from a
	// keystroke.
	if !strings.Contains(js, "contentRefresh = true;") || !strings.Contains(js, "contentRefresh = false;") {
		t.Error("the background re-run is not marked for the announcement guard")
	}
	// The guard compares the result signature, not only the count, so a refresh
	// that swaps rows for others of the same size is still announced.
	ann := jsFuncBody(t, js, "announceResults")
	guardAt := strings.Index(ann, "if (contentRefresh && key === lastAnnouncedKey) return;")
	if guardAt < 0 {
		t.Error("the announcement guard does not compare the result set, so a refresh that swaps rows of the same count says nothing")
	}
	keyAt := strings.Index(ann, "key += '|' + currentResults[i].u;")
	if keyAt < 0 || keyAt > guardAt {
		t.Error("the announcement builds the result signature after the guard, so the guard compares a stale key")
	}

	// The selected row survives the background re-run: applyContent records its
	// url before re-running, and renderResults selects it again when it is still
	// in the results, falling back to the first row otherwise.
	if !strings.Contains(apply, "refreshSelection = selectedIndex >= 0 && currentResults[selectedIndex]") {
		t.Error("applyContent does not record the selected row before re-running the search")
	}
	render := jsFuncBody(t, js, "renderResults")
	for _, want := range []string{
		"if (contentRefresh && refreshSelection) {",
		"if (currentResults[s].u === refreshSelection) { selectedIndex = s; break; }",
	} {
		if !strings.Contains(render, want) {
			t.Errorf("renderResults has no %q, so a background refresh resets the keyboard selection", want)
		}
	}
}

// TestDefaultThemeSearchRanksWholeWordsAndRepeats pins the ranking additions:
// a whole word outranks a word start outranks a mid-word hit in the title and
// the document, a repeated content term scores above a single mention, and an
// equal score breaks by the page's own entry, the shorter title and then the
// original index. The matching set and the existing weights must not move, so
// they are pinned here too. Go cannot run the client, so this asserts the shape
// and that the repeat count sits after the term loop, where only a matched
// entry pays for it.
func TestDefaultThemeSearchRanksWholeWordsAndRepeats(t *testing.T) {
	th, err := ResolveTheme("")
	if err != nil {
		t.Fatal(err)
	}
	var js string
	for _, f := range th.files {
		if f.Path == "search.js" {
			js = string(f.Data)
		}
	}
	if js == "" {
		t.Fatal("the default theme has no search.js")
	}

	// The helpers that grade a boundary and a repeat count.
	for _, want := range []string{
		"function isWordChar(ch) {",
		"function wordHitKind(field, term) {",
		"function countOccurrences(field, term, stop, from) {",
		"function repeatBonus(count) {",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("search.js has no %q, so a ranking helper is missing", want)
		}
	}
	// The largest bucket is the maximum, so a separate cap above it would be a
	// branch no count can reach.
	if strings.Contains(js, "REPEAT_CAP") {
		t.Error("repeatBonus carries a cap no bucket can reach")
	}

	run := jsFuncBody(t, js, "runSearch")

	// The boundary grade is taken on the title and the document, and the repeat
	// count on the content.
	for _, want := range []string{
		"wordHitKind(titleLower, term)",
		"wordHitKind(docLower, term)",
		"repeatBonus(countOccurrences(contentLower, terms[r], REPEAT_STOP, contentFrom))",
	} {
		if !strings.Contains(run, want) {
			t.Errorf("runSearch has no %q, so the boundary or repeat bonus is not applied", want)
		}
	}
	// The repeat count is an extra scan of the content, so it must sit after the
	// term loop: an entry that fails a later term pays nothing for it.
	termLoopEnd := strings.Index(run, "if (!termMatched) { allMatch = false; break; }")
	repeatAt := strings.Index(run, "repeatBonus(countOccurrences(")
	if termLoopEnd < 0 || repeatAt < termLoopEnd {
		t.Error("runSearch counts content repeats before the entry is known to match, so a failed entry still pays for the count")
	}

	// The field weights and the phrase bonus are unchanged: a title hit still
	// beats a document, section or content hit by the same margins.
	for _, want := range []string{
		"if (titleLower.indexOf(term) !== -1) {",
		"score += 45;",
		"score += 40;",
		"if (titleKind === 2) score += 25;",
		"else if (titleKind === 1) score += 15;",
		"if (docKind === 2) score += 12;",
		"else if (docKind === 1) score += 8;",
		"if (secLower.indexOf(term) !== -1) { termMatched = true; score += 15; }",
		"if (contentHit[j] !== -1) { termMatched = true; score += 10; }",
		"if (titleLower.indexOf(query) !== -1) score += 80;",
		"else if (docLower.indexOf(query) !== -1) score += 40;",
		"else if (contentLower.indexOf(query) !== -1) score += 30;",
	} {
		if !strings.Contains(run, want) {
			t.Errorf("runSearch has no %q, so a field weight or the phrase bonus moved", want)
		}
	}

	// The comparator breaks a tie with global keys, in order: the overview entry,
	// the title length, the original index. The last key is unique, so the order
	// does not depend on the engine's sort being stable.
	for _, want := range []string{
		"if (b.score !== a.score) return b.score - a.score;",
		"if (a.ov !== b.ov) return b.ov - a.ov;",
		"if (a.tl !== b.tl) return a.tl - b.tl;",
		"return a.i - b.i;",
	} {
		if !strings.Contains(run, want) {
			t.Errorf("runSearch has no %q, so the tie-break is not total", want)
		}
	}
	// Scoping the overview key to two rows that share a page is not transitive:
	// A before B, B before C and C before A at once, so the engine's comparison
	// order decides the result.
	if strings.Contains(run, "a.entry.p === b.entry.p && a.ov !== b.ov") {
		t.Error("the tie-break still scopes the overview key to one page, so the comparator is not a total order")
	}
	// Each match carries the keys the comparator reads.
	for _, want := range []string{
		"i: i,",
		"tl: entry.t ? entry.t.length : 0,",
		"ov: entry.u === entry.p ? 1 : 0",
	} {
		if !strings.Contains(run, want) {
			t.Errorf("runSearch pushes no %q, so the comparator cannot read it", want)
		}
	}
	// The score-only sort left ties to the index order the scan produced. Its
	// return pins the regression.
	if strings.Contains(js, "matches.sort(function (a, b) { return b.score - a.score; });") {
		t.Error("search.js still sorts on the score alone, so equal scores fall to whatever order the engine leaves")
	}
}

func TestThemeOverrideLayersOverTheDefault(t *testing.T) {
	// A recolour-only theme: one replaced asset, everything else inherited.
	dir := writeTheme(t,
		`{"name":"slate","version":"9.9.9","assets":{"tokens.css":{"mode":"replace"},"wiki.css":{"mode":"inherit"}}}`,
		map[string]string{"assets/tokens.css": ":root{--color-bg:#123456}"},
	)
	th, err := ResolveTheme(dir)
	if err != nil {
		t.Fatal(err)
	}
	if th.Name != "slate" || th.Version != "9.9.9" {
		t.Errorf("identity = %q %q, want slate 9.9.9", th.Name, th.Version)
	}
	got := map[string]string{}
	for _, f := range th.files {
		got[f.Path] = string(f.Data)
	}
	if got["tokens.css"] != ":root{--color-bg:#123456}" {
		t.Errorf("tokens.css was not replaced, got %q", got["tokens.css"])
	}
	// The key the manifest did not replace must be byte-identical to the
	// default's, which is the whole point of inheriting rather than copying.
	def, err := ResolveTheme("")
	if err != nil {
		t.Fatal(err)
	}
	defCSS := map[string]string{}
	for _, f := range def.files {
		defCSS[f.Path] = string(f.Data)
	}
	if got["wiki.css"] != defCSS["wiki.css"] {
		t.Error("wiki.css should be inherited unchanged from the default theme")
	}
	if _, ok := got["fonts/inter/inter-latin.woff2"]; !ok {
		t.Error("font files should be inherited even though the manifest never mentions them")
	}
	if th.AssetVersion() == def.AssetVersion() {
		t.Error("a theme that changes bytes must change the asset version, or caches go stale")
	}
}

func TestThemeDropRemovesAnAsset(t *testing.T) {
	dir := writeTheme(t, `{"assets":{"diagrams.js":{"mode":"drop"}}}`, nil)
	th, err := ResolveTheme(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range th.files {
		if f.Path == "diagrams.js" {
			t.Fatal("diagrams.js should have been dropped")
		}
	}
	// A drop must not take its neighbours with it.
	if len(th.files) < 5 {
		t.Errorf("dropping one asset left only %d files", len(th.files))
	}
}

func TestThemeSubtreeReplaceSwapsAWholeDirectory(t *testing.T) {
	dir := writeTheme(t, `{"assets":{"fonts/":{"mode":"replace"}}}`,
		map[string]string{"assets/fonts/mono.woff2": "x"})
	th, err := ResolveTheme(dir)
	if err != nil {
		t.Fatal(err)
	}
	var haveMono bool
	for _, f := range th.files {
		if !strings.HasPrefix(f.Path, "fonts/") {
			continue
		}
		if f.Path != "fonts/mono.woff2" {
			t.Errorf("stale font survived a subtree replace: %s", f.Path)
		}
		haveMono = f.Path == "fonts/mono.woff2"
	}
	if !haveMono {
		t.Error("the replacement font is missing")
	}
}

func TestThemeTemplateAndSlots(t *testing.T) {
	dir := writeTheme(t,
		`{"template":"t.html","slots":{"bodyEnd":"b.html"},"assets":{"wiki.css":{"mode":"drop"}}}`,
		map[string]string{
			"t.html": `<p>{{.Title}}</p><p>{{.Base}}</p><p>{{.Slot "bodyEnd"}}</p><p>{{.Slot "nope"}}</p>`,
			"b.html": "SENTINEL",
		},
	)
	th, err := ResolveTheme(dir)
	if err != nil {
		t.Fatal(err)
	}
	if th.Slot("bodyEnd") != "SENTINEL" {
		t.Errorf("Slot(bodyEnd) = %q", th.Slot("bodyEnd"))
	}
	if th.Slot("nope") != "" {
		t.Errorf("an undeclared slot must be empty, got %q", th.Slot("nope"))
	}
	for _, f := range th.files {
		if f.Path == "wiki.css" {
			t.Error("wiki.css should have been dropped")
		}
	}
	// Rendering must go through the theme's template, not a compiled-in one.
	r := &Renderer{cfg: Config{Brand: Brand{Name: "x"}}, theme: th, assetVersion: th.AssetVersion()}
	out, err := r.RenderPage(Page{Slug: "index", Title: "Hello"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Hello", "SENTINEL", "/wiki/"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered page missing %q:\n%s", want, out)
		}
	}
}

func TestThemeRejectsBadInput(t *testing.T) {
	if _, err := ResolveTheme(t.TempDir()); err == nil {
		t.Error("a theme directory without theme.json must be rejected")
	}
	bad := writeTheme(t, `{"assets":{"tokens.css":{"mode":"sideways"}}}`, nil)
	if _, err := ResolveTheme(bad); err == nil {
		t.Error("an unknown asset mode must be rejected")
	}
	missing := writeTheme(t, `{"assets":{"tokens.css":{"mode":"replace"}}}`, nil)
	if _, err := ResolveTheme(missing); err == nil {
		t.Error("a replace with no matching file must be rejected")
	}
	broken := writeTheme(t, `{"template":"t.html"}`, map[string]string{"t.html": "{{.Title"})
	if _, err := ResolveTheme(broken); err == nil {
		t.Error("an unparseable template must fail the render, not ship a broken page")
	}
	if _, err := ResolveTheme(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("a missing theme directory must be rejected")
	}
}

func TestRenderWritesTheResolvedThemeAndItsManifest(t *testing.T) {
	dir := writeTheme(t, `{"name":"overlay","version":"0.0.1","assets":{"tokens.css":{"mode":"replace"}}}`,
		map[string]string{"assets/tokens.css": "/* overlay */"})
	out := t.TempDir()
	r, err := New(Config{Content: docsDir(t), ThemeDir: dir, Brand: Brand{Name: "okf-wiki", Title: "Wiki"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	site := filepath.Join(out, "wiki")
	if b, err := os.ReadFile(filepath.Join(site, "tokens.css")); err != nil || string(b) != "/* overlay */" {
		t.Errorf("the override's tokens.css was not written: %q", string(b))
	}
	if _, err := os.Stat(filepath.Join(site, "wiki.css")); err != nil {
		t.Errorf("inherited wiki.css missing: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(site, "theme.json"))
	if err != nil {
		t.Fatalf("theme.json was not written: %v", err)
	}
	var m resolvedManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("theme.json is not valid JSON: %v", err)
	}
	if m.Name != "overlay" || m.Version != "0.0.1" {
		t.Errorf("manifest identity = %q %q", m.Name, m.Version)
	}
	if m.Source != "override" {
		t.Errorf("manifest Source = %q, want override", m.Source)
	}
	if m.Base != "/wiki/" {
		t.Errorf("manifest Base = %q, want /wiki/", m.Base)
	}
}

func TestRenderRelocatesToTheBase(t *testing.T) {
	out := t.TempDir()
	r, err := New(Config{Content: docsDir(t), Base: "/docs/deep/", Brand: Brand{Name: "okf-wiki", Title: "Wiki"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	site := filepath.Join(out, "docs", "deep")
	if _, err := os.Stat(filepath.Join(site, "index.html")); err != nil {
		t.Fatalf("pages were not written under the base: %v", err)
	}
	page, err := os.ReadFile(filepath.Join(site, "usage", "cli.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), `href="/docs/deep/wiki.css`) {
		t.Error("asset links still hardcode /wiki/")
	}
	if strings.Contains(string(page), `"/wiki/`) {
		t.Error("a page link still hardcodes /wiki/ under a relocated base")
	}
	redirect, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(redirect), "url=/docs/deep/") {
		t.Errorf("root redirect does not follow the base: %s", redirect)
	}

	// The search index is consumed as ready-made hrefs, so a stale prefix there
	// sends every result to a path that does not exist.
	index := readSearchIndex(t, filepath.Join(site, "search-index.json"))
	if len(index.Pages) == 0 || len(index.Entries) == 0 {
		t.Fatal("search index is empty under a relocated base")
	}
	for _, p := range index.Pages {
		if !strings.HasPrefix(p.URL, "/docs/deep/") {
			t.Fatalf("search page URL does not follow the base: %q", p.URL)
		}
	}
	for _, e := range flattenIndex(t, index) {
		if !strings.HasPrefix(e.URL, "/docs/deep/") {
			t.Fatalf("search entry URL does not follow the base: %q", e.URL)
		}
	}
}

// TestVersionTagRendersWhenSet covers the freeform version label: it appears
// beside the wordmark when configured, is escaped as the text it is, and leaves
// no markup behind when empty.
func TestVersionTagRendersWhenSet(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), strings.Join([]string{
		"---", "title: Home", "---", "# Home", "",
	}, "\n"))

	render := func(tag string) string {
		t.Helper()
		r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "okf-wiki", VersionTag: tag}})
		if err != nil {
			t.Fatal(err)
		}
		out := t.TempDir()
		if err := r.RenderAll(out); err != nil {
			t.Fatal(err)
		}
		page, err := os.ReadFile(filepath.Join(out, "wiki", "index.html"))
		if err != nil {
			t.Fatal(err)
		}
		return string(page)
	}

	withTag := render("v1.2.3")
	if !strings.Contains(withTag, `class="version-tag"`) || !strings.Contains(withTag, "v1.2.3") {
		t.Error("a configured version tag was not rendered")
	}

	without := render("")
	if strings.Contains(without, "version-tag") {
		t.Error("an empty version tag left markup behind")
	}

	escaped := render(`<script>alert(1)</script>`)
	if strings.Contains(escaped, "<script>alert(1)</script>") {
		t.Error("the version tag is injected as markup rather than escaped text")
	}
}

// TestTopNavLinks covers the header's outbound links. An external target opens
// in a new tab and is marked, an in-bundle target resolves to the rendered page
// and stays in this tab, and neither label nor target is injected as markup.
func TestTopNavLinks(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), strings.Join([]string{
		"---", "title: Home", "---", "# Home", "",
	}, "\n"))
	mustMkdir(t, filepath.Join(dir, "guide"))
	mustWrite(t, filepath.Join(dir, "guide", "intro.md"), strings.Join([]string{
		"---", "title: Intro", "---", "# Intro", "",
	}, "\n"))

	render := func(nav string) string {
		t.Helper()
		r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t", NavLinks: nav}})
		if err != nil {
			t.Fatal(err)
		}
		out := t.TempDir()
		if err := r.RenderAll(out); err != nil {
			t.Fatal(err)
		}
		page, err := os.ReadFile(filepath.Join(out, "wiki", "index.html"))
		if err != nil {
			t.Fatal(err)
		}
		return string(page)
	}

	page := render("Intro=guide/intro.md,GitHub=https://github.com/abn/okf-wiki")
	if !strings.Contains(page, `href="/wiki/guide/intro.html">Intro`) {
		t.Error("an in-bundle nav link did not resolve to its rendered page")
	}
	if strings.Contains(page, `aria-label="Intro (opens in a new tab)"`) {
		t.Error("an in-bundle nav link was treated as external")
	}
	if !strings.Contains(page, `target="_blank" rel="noopener noreferrer" aria-label="GitHub (opens in a new tab)"`) {
		t.Error("an external nav link did not open in a new tab and say so")
	}
	if !strings.Contains(page, `class="nav-ext"`) {
		t.Error("an external nav link carries no visible marker")
	}
	if !strings.Contains(page, "On this wiki") || !strings.Contains(page, "Elsewhere") {
		t.Error("the drawer does not separate the in-wiki links from the outbound ones")
	}
	if strings.Index(page, "On this wiki") > strings.Index(page, "Elsewhere") {
		t.Error("the in-wiki block comes after the outbound one in the drawer")
	}

	if onlyExt := render("GitHub=https://github.com/abn/okf-wiki"); strings.Contains(onlyExt, "On this wiki") {
		t.Error("an outbound-only nav shows an empty in-wiki block in the drawer")
	}

	if none := render(""); strings.Contains(none, "top-nav") {
		t.Error("an empty nav left markup behind")
	}

	escaped := render(`A<b>=https://example.com/?q="x"`)
	if strings.Contains(escaped, "<b>") {
		t.Error("the nav label is injected as markup rather than escaped text")
	}
}

// TestMarkdownDownload covers the per-page source: the render copies each page's
// Markdown beside its HTML, the page links to it, and a synthetic page with no
// file in the bundle offers nothing.
func TestMarkdownDownload(t *testing.T) {
	out := t.TempDir()
	r, err := New(Config{Content: docsDir(t), Base: "/wiki/", Brand: Brand{Name: "okf-wiki"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	site := filepath.Join(out, "wiki")

	if _, err := os.Stat(filepath.Join(site, "design", "rendering.md")); err != nil {
		t.Fatalf("the page source was not copied beside its HTML: %v", err)
	}
	page, err := os.ReadFile(filepath.Join(site, "design", "rendering.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), `class="page-download" href="/wiki/design/rendering.md" download`) {
		t.Error("a page does not offer its own Markdown for download")
	}

	tag, err := os.ReadFile(filepath.Join(site, "tags", "design.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(tag), "page-download") {
		t.Error("a synthetic tag page offered a download it has no source for")
	}
}

// TestRenderAtRootBaseKeepsTheHomePage guards the case where the site is served
// from the output root. There is no room for a redirect there, so writing one
// would overwrite the wiki's own index.html with a self-redirect.
func TestRenderAtRootBaseKeepsTheHomePage(t *testing.T) {
	out := t.TempDir()
	r, err := New(Config{Content: docsDir(t), Base: "/", Brand: Brand{Name: "okf-wiki", Title: "Wiki"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	home, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(home), "http-equiv") {
		t.Errorf("the home page is a redirect at base \"/\":\n%s", home)
	}
	if !strings.Contains(string(home), `href="/wiki.css`) {
		t.Errorf("the home page is not a rendered wiki page:\n%s", home)
	}
}

// TestRenderAtNestedBaseWritesARootRedirect is the other half: when the site is
// nested, the output root gets a redirect so serving it lands on the wiki.
func TestRenderAtNestedBaseWritesARootRedirect(t *testing.T) {
	out := t.TempDir()
	r, err := New(Config{Content: docsDir(t), Base: "/handbook/", Brand: Brand{Name: "okf-wiki", Title: "Wiki"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	redirect, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(redirect), "url=/handbook/") {
		t.Errorf("no root redirect at a nested base:\n%s", redirect)
	}
}

// TestPageLanguageIsDeclared covers the language the renderer writes. An
// unconfigured bundle declares English, and the root redirect the output root
// gets carries the language that was configured: the redirect is a page the
// reader is sent to, so it declares the same language as the page it redirects
// to rather than making a screen reader switch voice on arrival.
func TestPageLanguageIsDeclared(t *testing.T) {
	render := func(lang string) (page, redirect string) {
		t.Helper()
		content := t.TempDir()
		mustWrite(t, filepath.Join(content, "index.md"), "---\ntitle: Home\n---\n# Home\n")
		out := t.TempDir()
		r, err := New(Config{
			Content: content, Base: "/wiki/", Lang: lang,
			Brand: Brand{Name: "t", Title: "Wiki"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := r.RenderAll(out); err != nil {
			t.Fatal(err)
		}
		written, err := os.ReadFile(filepath.Join(out, "wiki", "index.html"))
		if err != nil {
			t.Fatal(err)
		}
		rooted, err := os.ReadFile(filepath.Join(out, "index.html"))
		if err != nil {
			t.Fatal(err)
		}
		return string(written), string(rooted)
	}

	page, redirect := render("")
	if !strings.Contains(page, `<html lang="en">`) {
		t.Errorf("an unconfigured bundle does not declare English:\n%s", page)
	}
	if !strings.Contains(redirect, `<html lang="en">`) {
		t.Errorf("the root redirect does not declare English:\n%s", redirect)
	}

	page, redirect = render("de")
	if !strings.Contains(page, `<html lang="de">`) {
		t.Errorf("the configured language did not reach the page:\n%s", page)
	}
	if !strings.Contains(redirect, `<html lang="de">`) {
		t.Errorf("the root redirect does not carry the configured language:\n%s", redirect)
	}
}

// A declared language reaches an attribute, so its shape is decided before the
// render rather than left for the browser to ignore.
func TestLangValidation(t *testing.T) {
	tests := []struct {
		lang string
		want bool // want reports whether the tag is one the renderer will emit
	}{
		{"", true},
		{"en", true},
		{"de", true},
		{"pt-BR", true},
		{"zh-Hans-CN", true},
		{"  de  ", true},
		{"my wiki", false},
		{"de AT", false},
		{"de;", false},
		{`de"`, false},
		{`de<`, false},
		{"-de", false},
		{"3d", false},
		{"de_AT", false},
	}
	for _, tc := range tests {
		err := validateLang(tc.lang)
		if tc.want && err != nil {
			t.Errorf("validateLang(%q) = %v, want it accepted", tc.lang, err)
		}
		if !tc.want && err == nil {
			t.Errorf("validateLang(%q) = nil, want it rejected", tc.lang)
		}
	}
	// The check has to be the one a render goes through, not a helper nothing
	// calls.
	if _, err := New(Config{Content: t.TempDir(), Lang: "my wiki", Brand: Brand{Name: "t"}}); err == nil {
		t.Error("New accepted a language that is not a language tag")
	}
}

// docsDir returns this project's own docs bundle. The suite runs from
// internal/wiki, so the bundle is two levels up.
func docsDir(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	content := filepath.Join(root, "docs")
	if _, err := os.Stat(content); err != nil {
		t.Skipf("docs/ not found at %s: %v", content, err)
	}
	return content
}

func TestFrontmatterParsing(t *testing.T) {
	data := []byte("---\ntype: Guide\ntitle: Cli\ndescription: Run it.\ntags: [a, b]\ngenerated:\n  by: human:abn\n  at: 2026-01-02T03:04:05Z\n---\n\n# Cli\n\nBody.\n")
	fm, body := splitFrontmatter(data)
	if fm.Type != "Guide" || fm.Title != "Cli" || fm.Description != "Run it." {
		t.Fatalf("unexpected frontmatter: %+v", fm)
	}
	if len(fm.Tags) != 2 || fm.Tags[0] != "a" || fm.Tags[1] != "b" {
		t.Fatalf("unexpected tags: %+v", fm.Tags)
	}
	if fm.Generated.At != "2026-01-02T03:04:05Z" {
		t.Fatalf("unexpected generated.at: %q", fm.Generated.At)
	}
	if !strings.Contains(string(body), "# Cli") {
		t.Fatalf("body lost its heading: %q", body)
	}
}

func TestParseSectionsAndOrder(t *testing.T) {
	specs := parseSections("usage:Guides,adr")
	if len(specs) != 2 || specs[0].ID != "usage" || specs[0].Title != "Guides" || specs[1].Title != "Adr" {
		t.Fatalf("unexpected specs: %+v", specs)
	}
	ordered := orderedSectionIDs(parseSections("usage:Guides"), []string{"design", "usage", "reference"})
	got := []string{}
	for _, s := range ordered {
		got = append(got, s.ID)
	}
	want := []string{"", "usage", "design", "reference"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestDetectContent(t *testing.T) {
	dir := t.TempDir()
	if isOKFBundle(dir) {
		t.Fatal("empty dir should not be an OKF bundle")
	}
	if err := os.WriteFile(filepath.Join(dir, "index.md"), []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !isOKFBundle(dir) {
		t.Fatalf("dir with index.md should be an OKF bundle")
	}
	got, err := DetectContent([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("DetectContent = %q, want %q", got, dir)
	}
}

// TestRenderBundledDocs renders this project's own docs/ bundle, the widest
// content the renderer will see in practice.
func TestRenderBundledDocs(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	content := filepath.Join(root, "docs")
	if _, err := os.Stat(content); err != nil {
		t.Skipf("docs/ not found at %s: %v", content, err)
	}

	r, err := New(Config{Content: content, Brand: Brand{Name: "okf-wiki", Sub: "docs", Title: "Wiki"}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatalf("RenderAll: %v", err)
	}

	for _, rel := range []string{
		"wiki/index.html", "wiki/overview.html", "wiki/usage/container.html",
		"wiki/wiki.css", "wiki/tokens.css", "wiki/diagrams.js", "wiki/brand-mark.svg",
	} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Errorf("expected %s: %v", rel, err)
		}
	}

	index := readSearchIndex(t, filepath.Join(out, "wiki", "search-index.json"))
	if len(index.Entries) < 10 {
		t.Fatalf("search index has only %d entries", len(index.Entries))
	}
	for _, e := range flattenIndex(t, index) {
		if e.URL == "" || e.Doc == "" {
			t.Fatalf("search entry missing URL/Doc: %+v", e)
		}
	}
}

func TestRepoFileServerBlocksDotfiles(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("ok.txt", "ok")
	write(".env", "SECRET=1")
	write(".git/config", "[core]")

	srv := httptest.NewServer(repoFileServer(dir))
	defer srv.Close()

	get := func(p string) int {
		resp, err := http.Get(srv.URL + p)
		if err != nil {
			t.Fatalf("GET %s: %v", p, err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}
	if code := get("/ok.txt"); code != http.StatusOK {
		t.Errorf("GET /ok.txt = %d, want 200", code)
	}
	if code := get("/.env"); code != http.StatusNotFound {
		t.Errorf("GET /.env = %d, want 404", code)
	}
	if code := get("/.git/config"); code != http.StatusNotFound {
		t.Errorf("GET /.git/config = %d, want 404", code)
	}
}

// TestServeRoutesAcrossBases builds the real route table for each base. A base
// of "/" used to panic: TrimSuffix gave "", which ServeMux rejects as a
// pattern, and the catch-all then tried to claim "/" a second time.
func TestServeRoutesAcrossBases(t *testing.T) {
	dir := t.TempDir()
	// The rendered layout: the file server is rooted at the output directory and
	// the base is mounted inside it, so the page sits at <out>/<base>/index.html.
	for _, site := range []string{"wiki", "a/b", "c", ""} {
		if err := os.MkdirAll(filepath.Join(dir, site), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, site, "index.html"), []byte("<h1>hi</h1>"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name string
		base string
		want []string // paths that must be served from the site
	}{
		{name: "default", base: "", want: []string{"/wiki/", "/wiki/index.html"}},
		{name: "root", base: "/", want: []string{"/", "/index.html"}},
		{name: "nested", base: "/a/b", want: []string{"/a/b/", "/a/b/index.html"}},
		{name: "no-leading-slash", base: "c", want: []string{"/c/", "/c/index.html"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := handler(ServeOptions{OutDir: dir, Base: tc.base})
			srv := httptest.NewServer(h)
			defer srv.Close()

			for _, p := range tc.want {
				resp, err := http.Get(srv.URL + p)
				if err != nil {
					t.Fatalf("GET %s: %v", p, err)
				}
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					t.Errorf("GET %s = %d, want 200", p, resp.StatusCode)
				}
				if !strings.Contains(string(body), "<h1>hi</h1>") {
					t.Errorf("GET %s did not serve the page", p)
				}
				if got := resp.Header.Get("Cache-Control"); !strings.Contains(got, "no-cache") {
					t.Errorf("GET %s Cache-Control = %q, want no-cache", p, got)
				}
			}

			// The prefix without its trailing slash redirects onto the base.
			prefix := Config{Base: tc.base}.base()
			trimmed := strings.TrimSuffix(prefix, "/")
			if trimmed != "" {
				client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
					return http.ErrUseLastResponse
				}}
				resp, err := client.Get(srv.URL + trimmed)
				if err != nil {
					t.Fatalf("GET %s: %v", trimmed, err)
				}
				resp.Body.Close()
				if resp.StatusCode != http.StatusMovedPermanently {
					t.Errorf("GET %s = %d, want 301", trimmed, resp.StatusCode)
				}
				if loc := resp.Header.Get("Location"); loc != prefix {
					t.Errorf("GET %s Location = %q, want %q", trimmed, loc, prefix)
				}
			}

			// The root lands on the base for a prefixed site, and is the site
			// itself when the base is "/".
			client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			}}
			resp, err := client.Get(srv.URL + "/")
			if err != nil {
				t.Fatalf("GET /: %v", err)
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if trimmed == "" {
				if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "<h1>hi</h1>") {
					t.Errorf("base %q: GET / = %d, want the site itself", tc.base, resp.StatusCode)
				}
			} else if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != prefix {
				t.Errorf("base %q: GET / = %d Location %q, want 302 to %q",
					tc.base, resp.StatusCode, resp.Header.Get("Location"), prefix)
			}

			resp, err = http.Get(srv.URL + "/healthz")
			if err != nil {
				t.Fatalf("GET /healthz: %v", err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("GET /healthz = %d, want 200", resp.StatusCode)
			}
		})
	}
}

// A base of "/" is the one a user or org Pages site is served under, so the
// container's default configuration has to build a mux without panicking.
func TestServeAtRootBaseDoesNotPanic(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>root</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Must not panic. A panic in a handler test fails the test rather than
	// crashing the run, so recover is not needed here.
	handler(ServeOptions{OutDir: dir, Base: "/"})
}

// TestHeadingIDsAreUnique covers the collision that counting repeats of one base
// slug cannot see. "Setup", "Setup" and "Setup 2" all interact: the second
// repeat is handed setup-2, which is exactly what the literal "Setup 2"
// slugifies to.
func TestHeadingIDsAreUnique(t *testing.T) {
	dir := t.TempDir()
	body := strings.Join([]string{
		"---",
		"title: Duplicates",
		"---",
		"# Duplicates",
		"## Setup",
		"## Setup",
		"## Setup",
		"## Setup 2",
		"## Setup 2",
		"## Setup",
		"### Setup",
		"## !!!",
		"## ???",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "index.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	var page Page
	for _, s := range r.Sections() {
		for _, p := range s.Pages {
			page = p
		}
	}
	html, err := r.RenderPage(page)
	if err != nil {
		t.Fatal(err)
	}

	ids := regexp.MustCompile(`<h[1-6] id="([^"]*)"`).FindAllStringSubmatch(html, -1)
	// Seven headings slugify to setup-ish bases and two to nothing, so the H1
	// is the only heading that does not appear here.
	if len(ids) != 9 {
		t.Fatalf("found %d heading ids, want 9: %v", len(ids), ids)
	}
	seen := map[string]bool{}
	for _, m := range ids {
		if seen[m[1]] {
			t.Errorf("duplicate heading id %q", m[1])
		}
		seen[m[1]] = true
	}
	// Every id is distinct, and the numbering is the one uniqueID produces:
	// a suffixed candidate is itself uniquified, so "Setup 2" after two repeats
	// of "Setup" becomes setup-2-2 rather than colliding with setup-2.
	want := []string{
		"setup", "setup-2", "setup-3", "setup-2-2", "setup-2-3",
		"setup-4", "setup-5", "section", "section-2",
	}
	for _, w := range want {
		if !seen[w] {
			t.Errorf("missing expected heading id %q; got %v", w, seen)
		}
	}

	// Every table of contents entry must point at an id that exists.
	for _, m := range regexp.MustCompile(`<a href="#([^"]*)"`).FindAllStringSubmatch(html, -1) {
		if !seen[m[1]] {
			t.Errorf("table of contents links to #%s, which is not a heading id", m[1])
		}
	}
}

// GitHub matches an alert marker without regard to case, so a lowercase or
// mixed-case marker has to become the same card as its uppercase spelling.
func TestAlertMarkersMatchAnyCase(t *testing.T) {
	cases := []struct {
		marker string
		kind   string
		body   string
	}{
		{"[!NOTE]", "note", "Body text."},
		{"[!note]", "note", "Body text."},
		{"[!Note]", "note", "Body text."},
		{"[!TIP]", "tip", "Body text."},
		{"[!tip]", "tip", "Body text."},
		{"[!tIp]", "tip", "Body text."},
		{"[!IMPORTANT]", "important", "Body text."},
		{"[!important]", "important", "Body text."},
		{"[!WARNING]", "warning", "Body text."},
		{"[!warning]", "warning", "Body text."},
		{"[!Warning]", "warning", "Body text."},
		{"[!CAUTION]", "caution", "Body text."},
		{"[!caution]", "caution", "Body text."},
		{"[!CaUtIoN]", "caution", "Body text."},
		// The marker shares the line with its body rather than standing alone.
		{"[!note]", "note", ""},
		// Multi-byte runes either side of the marker, since the marker is
		// matched over a fixed-length byte window and the body is cut at byte
		// offsets. A rune that folded to an ASCII letter would break both.
		{"[!warning]", "warning", "Größe, naïve, 日本語, 🚀 emoji."},
		{"[!note]", "note", "⚠️ Ünïcode straight after the marker."},
	}

	for _, tc := range cases {
		t.Run(tc.marker+" "+tc.body, func(t *testing.T) {
			lines := []string{
				"---",
				"title: Alert",
				"---",
				"# Alert",
			}
			if tc.body == "" {
				lines = append(lines, "> "+tc.marker+" Body on the same line.")
			} else {
				lines = append(lines, "> "+tc.marker, "> "+tc.body)
			}
			html := renderOnlyPage(t, t.TempDir(), strings.Join(lines, "\n"))

			if want := `class="callout callout-` + tc.kind + `"`; !strings.Contains(html, want) {
				t.Errorf("%s did not render a %s callout", tc.marker, tc.kind)
			}
			if strings.Contains(html, "[!") {
				t.Errorf("%s: marker left in the rendered page", tc.marker)
			}
			wantBody := tc.body
			if wantBody == "" {
				wantBody = "Body on the same line."
			}
			if !strings.Contains(html, wantBody) {
				t.Errorf("%s: body %q missing or truncated in the rendered page", tc.marker, wantBody)
			}
		})
	}
}

func TestAlertMarkerNotRecognisedStaysABlockquote(t *testing.T) {
	// The near misses matter as much as the obvious ones: each is a marker
	// whose window is the right length and wrong in one character.
	for _, marker := range []string{"[!UNKNOWN]", "[!NOTES]", "[!nota]", "[!NOTE", "![!NOTE]", "NOTE", "[ NOTE ]"} {
		t.Run(marker, func(t *testing.T) {
			html := renderOnlyPage(t, t.TempDir(), alertDoc(marker))

			if strings.Contains(html, `class="callout`) {
				t.Errorf("%s rendered as a callout", marker)
			}
			if !strings.Contains(html, "<blockquote>") {
				t.Errorf("%s: expected a plain blockquote", marker)
			}
		})
	}
}

// A multi-byte rune inside the marker window must not fold onto an ASCII
// letter and make the window match, since the body is then cut at a byte
// offset that no longer lines up with the rune boundary.
func TestAlertMarkerWithMultiByteRuneIsNotAMarker(t *testing.T) {
	for _, marker := range []string{"[!wärning]", "[!nöte]", "[!ᴡarning]", "[!noteﬁ]"} {
		t.Run(marker, func(t *testing.T) {
			html := renderOnlyPage(t, t.TempDir(), alertDoc(marker))

			if strings.Contains(html, `class="callout`) {
				t.Errorf("%s rendered as a callout", marker)
			}
			if !strings.Contains(html, "<blockquote>") {
				t.Errorf("%s: expected a plain blockquote", marker)
			}
		})
	}
}

// alertDoc is the Markdown of a one-page bundle whose only blockquote carries
// marker.
func alertDoc(marker string) string {
	return alertDocBody("> " + marker + "\n> Body text.")
}

// renderOnlyPage writes body as the one page of a bundle in dir and returns
// that page's HTML. It fails the test unless the bundle holds exactly one
// page, so a caller reaching for the wrong page hears about it.
func renderOnlyPage(t *testing.T, dir, body string) string {
	t.Helper()
	mustWrite(t, filepath.Join(dir, "index.md"), body)
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	var pages []Page
	for _, s := range r.Sections() {
		pages = append(pages, s.Pages...)
	}
	if len(pages) != 1 {
		t.Fatalf("bundle has %d pages, want exactly 1", len(pages))
	}
	html, err := r.RenderPage(pages[0])
	if err != nil {
		t.Fatal(err)
	}
	return html
}

// A marker only opens a callout when it is the first thing in the paragraph.
// The probe reads text nodes only, so a marker that follows a link, an image or
// a code span has to stay a blockquote: the marker is not at the start of the
// line, and the marker text has to survive in the output rather than being
// stripped out from under an inline node.
func TestAlertMarkerAfterInlineNodeStaysABlockquote(t *testing.T) {
	cases := []struct{ name, body string }{
		{"after a link", "> [a](b)[!NOTE]\n> Body text.\n"},
		{"after an image", "> ![alt](a.png)[!NOTE]\n> Body text.\n"},
		{"after a code span", "> `x`[!NOTE]\n> Body text.\n"},
		{"after emphasis", "> **bold**[!NOTE]\n> Body text.\n"},
		// Raw HTML splits the paragraph into siblings, so the marker sits after
		// a node the probe has to stop at. An empty tag and a bare break carry
		// no text of their own, which is what makes them discriminate: the
		// marker's own text is the whole window either way, and a probe that
		// walked past the node would still see it.
		{"after raw html", "> <b></b>[!NOTE]\n> Body text.\n"},
		{"after a hard break", "> <br>[!NOTE]\n> Body text.\n"},
		// An HTML comment produces no goldmark node to stop at, so it
		// discriminates nothing: a marker after one was a callout before this
		// change and still is.
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			html := renderOnlyPage(t, t.TempDir(), alertDocBody(tc.body))

			if strings.Contains(html, `class="callout`) {
				t.Errorf("%s: became a callout, want a plain blockquote", tc.name)
			}
			if !strings.Contains(html, "<blockquote>") {
				t.Errorf("%s: expected a plain blockquote", tc.name)
			}
			if !strings.Contains(html, "[!NOTE]") {
				t.Errorf("%s: marker was stripped from the output", tc.name)
			}
		})
	}
}

// The whitespace after the marker is skipped within the text that holds it.
// A space that belongs to a later node, one separated by an inline node, has to
// survive: the probe is bounded by the text node, so it cannot reach across the
// link to eat it.
func TestAlertMarkerLeavesASpaceAfterAnInlineNode(t *testing.T) {
	html := renderOnlyPage(t, t.TempDir(), alertDocBody("> [!NOTE] [a link](x) tail\n"))

	if !strings.Contains(html, `class="callout callout-note"`) {
		t.Error("expected a note callout")
	}
	// The body is the link, then the word that followed it, one space apart.
	if !strings.Contains(html, "</a> tail") {
		t.Errorf("the space after the link was consumed; rendered body is %q", renderedCalloutBody(html))
	}
	if strings.Contains(html, "</a>tail") {
		t.Error("the space between the link and the following text was removed")
	}
}

// alertDocBody is the one-page bundle skeleton alertDoc builds on, for a test
// that writes its own blockquote.
func alertDocBody(body string) string {
	return strings.Join([]string{
		"---",
		"title: Alert",
		"---",
		"# Alert",
		body,
	}, "\n")
}

// renderedCalloutBody is the first callout's body, for a failure message that
// shows what actually rendered.
func renderedCalloutBody(html string) string {
	m := regexp.MustCompile(`(?s)class="callout[^"]*".*?<div class="callout-body">(.*?)</div>`).FindStringSubmatch(html)
	if m == nil {
		return "(no callout body found)"
	}
	return strings.TrimSpace(m[1])
}

// uniqueID is the unit of behaviour behind the above: the candidate has to be
// checked against every id already taken, not against a count of one base slug.
func TestUniqueID(t *testing.T) {
	taken := map[string]bool{}
	// Bases as slugify produces them: "Setup" -> setup, "Setup 2" -> setup-2.
	seq := []string{"setup", "setup", "setup-2", "setup-2", "setup", ""}
	var got []string
	for _, base := range seq {
		got = append(got, uniqueID(base, taken))
	}
	// setup-2 is taken by the second call, so the literal setup-2 base has to
	// step past it rather than land on it.
	want := []string{"setup", "setup-2", "setup-2-2", "setup-2-3", "setup-3", "section"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("call %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestBundleAssetsAreCopied covers the files a page can point at without being
// one. Before this, a bundle asset was neither copied into the site nor
// rewritten in the page, so the image was a link to a file that was never
// written and the render still reported success.
func TestBundleAssetsAreCopied(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "assets", "nested"))
	mustWrite(t, filepath.Join(dir, "assets", "diagram.png"), "PNG")
	mustWrite(t, filepath.Join(dir, "assets", "nested", "deep.svg"), "<svg/>")
	mustWrite(t, filepath.Join(dir, "notes.txt"), "notes")
	mustWrite(t, filepath.Join(dir, ".env"), "SECRET=1")
	mustMkdir(t, filepath.Join(dir, ".git"))
	mustWrite(t, filepath.Join(dir, ".git", "config"), "[core]")

	mustWrite(t, filepath.Join(dir, "index.md"), strings.Join([]string{
		"---", "title: Home", "---", "# Home",
		"![shot](assets/diagram.png)",
		"",
		"[notes](notes.txt)",
		"",
		"![deep](assets/nested/deep.svg)",
	}, "\n"))

	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	site := filepath.Join(out, "wiki")

	for _, rel := range []string{"assets/diagram.png", "assets/nested/deep.svg", "notes.txt"} {
		if _, err := os.Stat(filepath.Join(site, filepath.FromSlash(rel))); err != nil {
			t.Errorf("bundle asset %s was not copied: %v", rel, err)
		}
	}
	// A dot-directory must not be publishable through the bundle.
	if _, err := os.Stat(filepath.Join(site, ".env")); err == nil {
		t.Error(".env was copied into the site")
	}
	if _, err := os.Stat(filepath.Join(site, ".git")); err == nil {
		t.Error(".git was copied into the site")
	}

	body, err := os.ReadFile(filepath.Join(site, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(body)
	for _, want := range []string{
		`src="/wiki/assets/diagram.png"`,
		`src="/wiki/assets/nested/deep.svg"`,
		`href="/wiki/notes.txt"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("page does not contain %s", want)
		}
	}
	// The relative form is what breaks: it resolves against /wiki/index.html.
	if strings.Contains(html, `src="assets/diagram.png"`) {
		t.Error("image src was left relative to the page")
	}
	// An in-bundle asset is not a repository file.
	if strings.Contains(html, "/repo/assets/") {
		t.Error("in-bundle asset was pointed at /repo/")
	}
}

// Without a repository root there is nothing to serve an escaping target, so the
// href must survive as written rather than become a dead /repo/ URL.
func TestLinkPathWithoutRepoKeepsEscapingHref(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), strings.Join([]string{
		"---", "title: Home", "---", "# Home",
		"[outside](../../README.md)",
		"[page](other.md)",
		"[asset](data.json)",
	}, "\n"))
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	var page Page
	for _, s := range r.Sections() {
		for _, p := range s.Pages {
			page = p
		}
	}
	got := r.linkPath(page.Slug, "../../README.md")
	if strings.HasPrefix(got, "/repo/") {
		t.Errorf("escaping link = %q, want it left as written", got)
	}
	if got := r.linkPath(page.Slug, "data.json"); got != "/wiki/data.json" {
		t.Errorf("in-bundle asset = %q, want /wiki/data.json", got)
	}
	if got := r.linkPath(page.Slug, "other.md"); got != "/wiki/other.html" {
		t.Errorf("in-bundle page = %q, want /wiki/other.html", got)
	}
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestNestedContentPagesRender walks a bundle with Markdown at three depths.
// A one-level glob dropped the nested pages while still rewriting links to
// them, so the site carried dead links and the render reported success.
func TestNestedContentPagesRender(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "guide", "setup"))
	mustMkdir(t, filepath.Join(dir, "guide", "deep", "deeper"))
	mustWrite(t, filepath.Join(dir, "index.md"), strings.Join([]string{
		"---", "title: Home", "---", "# Home",
		"[setup](guide/setup/install.md)",
		"[deeper](guide/deep/deeper/x.md)",
		"[sibling](guide/intro.md)",
	}, "\n"))
	mustWrite(t, filepath.Join(dir, "guide", "intro.md"), "---\ntitle: Intro\n---\n# Intro\n")
	mustWrite(t, filepath.Join(dir, "guide", "setup", "install.md"),
		"---\ntitle: Install\n---\n# Install\n\n[up](../intro.md)\n")
	mustWrite(t, filepath.Join(dir, "guide", "deep", "deeper", "x.md"),
		"---\ntitle: X\n---\n# X\n")

	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	site := filepath.Join(out, "wiki")

	want := []string{
		"index.html",
		"guide/intro.html",
		"guide/setup/install.html",
		"guide/deep/deeper/x.html",
	}
	for _, rel := range want {
		if _, err := os.Stat(filepath.Join(site, filepath.FromSlash(rel))); err != nil {
			t.Errorf("page %s was not written: %v", rel, err)
		}
	}

	// Every internal link on the home page must name a file that exists. The
	// asset links carry a ?v= cache buster, which is not part of the path.
	home, err := os.ReadFile(filepath.Join(site, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range regexp.MustCompile(`href="(/wiki/[^"#?]*)[^"]*"`).FindAllStringSubmatch(string(home), -1) {
		if strings.HasSuffix(m[1], "/") {
			continue
		}
		p := filepath.Join(out, filepath.FromSlash(strings.TrimPrefix(m[1], "/")))
		if _, err := os.Stat(p); err != nil {
			t.Errorf("home page links to %s, which was not written", m[1])
		}
	}

	// A relative link from a nested page back up a level.
	install, err := os.ReadFile(filepath.Join(site, "guide", "setup", "install.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(install), `href="/wiki/guide/intro.html"`) {
		t.Errorf("nested page did not resolve the link up a level: %s", install)
	}

	// The nested pages belong to the guide section and are indexed.
	var guide Section
	for _, s := range r.Sections() {
		if s.ID == "guide" {
			guide = s
		}
	}
	if len(guide.Pages) != 3 {
		t.Errorf("guide section has %d pages, want 3", len(guide.Pages))
	}
	found := false
	for _, p := range r.BuildSearchIndex().Pages {
		if strings.Contains(p.URL, "guide/setup/install.html") {
			found = true
		}
	}
	if !found {
		t.Error("the nested install page is missing from the search index")
	}

	// The sidebar respects one level of the folder structure: a sub-directory of
	// a section nests its pages under its own disclosure rather than running them
	// flat. guide/intro.md has no sub-directory, so it stays a loose item; the
	// pages under guide/setup and guide/deep/deeper collapse into their top
	// group, and the group is labelled from the directory when it has no index.
	nav := r.sidebarHTML("guide", "guide/setup/install")
	if !strings.Contains(nav, `<details class="side-sub" name="wiki-sub-guide" open><summary class="side-subsum"><span>Setup</span>`) {
		t.Errorf("the setup sub-directory is not a nested group:\n%s", nav)
	}
	if !strings.Contains(nav, `<li class="active"><a href="/wiki/guide/setup/install.html" aria-current="true">Install</a></li>`) {
		t.Errorf("the active page is not inside the setup group:\n%s", nav)
	}
	if !strings.Contains(nav, `<summary class="side-subsum"><span>Deep</span>`) {
		t.Errorf("a three-deep group did not collapse to its top directory:\n%s", nav)
	}
	if !strings.Contains(nav, `<li><a href="/wiki/guide/intro.html">Intro</a></li>`) {
		t.Errorf("a page directly under the section is not a loose item:\n%s", nav)
	}
	if strings.Contains(nav, `<li><a href="/wiki/guide/setup/install.html">`) &&
		!strings.Contains(nav, `<details class="side-sub"`) {
		t.Error("a nested page was rendered flat outside its group")
	}
}

// TestSidebarLabelShortensGroupPrefix covers the rail's label shortening: a page
// whose title repeats its group heading drops the shared prefix, and a prefix
// that does not end on a word boundary is left alone so a short heading cannot
// slice a longer word.
func TestSidebarLabelShortensGroupPrefix(t *testing.T) {
	tests := []struct{ title, prefix, want string }{
		{"Tape in Agent Hub evidence", "Tape in Agent Hub", "Evidence"},
		{"Tape in Agent Hub in Simplified Technical English", "Tape in Agent Hub", "In Simplified Technical English"},
		{"Guide: installation", "Guide", "Installation"},
		{"AXIOM", "AX", "AXIOM"},
		{"Tape in Agent Hub", "Tape in Agent Hub", "Tape in Agent Hub"},
		{"Unrelated page", "Tape in Agent Hub", "Unrelated page"},
		{"Tape in Agent Hub evidence", "", "Tape in Agent Hub evidence"},
	}
	for _, tc := range tests {
		if got := sidebarLabel(tc.title, tc.prefix); got != tc.want {
			t.Errorf("sidebarLabel(%q, %q) = %q, want %q", tc.title, tc.prefix, got, tc.want)
		}
	}
}

// TestSidebarNavLabelOverride covers frontmatter nav: a page keeps a long body
// title while the rail shows the short one the author set.
func TestSidebarNavLabelOverride(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "guide", "setup"))
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\n---\n# Home\n")
	mustWrite(t, filepath.Join(dir, "guide", "index.md"), "---\ntitle: Guide\n---\n# Guide\n")
	mustWrite(t, filepath.Join(dir, "guide", "setup", "install.md"),
		"---\ntitle: Installing the whole thing on a clean machine\nnav: Install\n---\n# Install\n")

	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	nav := r.sidebarHTML("guide", "guide/setup/install")
	if !strings.Contains(nav, `<a href="/wiki/guide/setup/install.html" aria-current="true">Install</a>`) {
		t.Errorf("the nav label did not replace the page title:\n%s", nav)
	}
	if strings.Contains(nav, "Installing the whole thing") {
		t.Errorf("the rail shows the body title instead of nav:\n%s", nav)
	}
}

// TestSidebarGroupPrefixFold covers the render path for shortening: a child of a
// group whose title it repeats loses the shared prefix in the rail.
func TestSidebarGroupPrefixFold(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "agents", "tape"))
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\n---\n# Home\n")
	mustWrite(t, filepath.Join(dir, "agents", "index.md"), "---\ntitle: Agents\n---\n# Agents\n")
	mustWrite(t, filepath.Join(dir, "agents", "tape", "index.md"), "---\ntitle: Tape in Agent Hub\n---\n# Tape in Agent Hub\n")
	mustWrite(t, filepath.Join(dir, "agents", "tape", "report.md"), "---\ntitle: Tape in Agent Hub report\n---\n# Report\n")

	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	nav := r.sidebarHTML("agents", "agents/tape/report")
	if !strings.Contains(nav, `<li class="active"><a href="/wiki/agents/tape/report.html" aria-current="true">Report</a></li>`) {
		t.Errorf("the repeating prefix was not shortened in the rail:\n%s", nav)
	}
}

// A dot-directory is not content, and must not become a page or an asset.
func TestDotDirectoriesAreNotContent(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, ".hidden"))
	mustWrite(t, filepath.Join(dir, ".hidden", "notes.md"), "---\ntitle: Secret\n---\n# Secret\n")
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\n---\n# Home\n")

	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, s := range r.Sections() {
		total += len(s.Pages)
	}
	if total != 1 {
		t.Errorf("rendered %d pages, want 1: a .hidden directory is not content", total)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "wiki", ".hidden")); err == nil {
		t.Error(".hidden was copied into the site")
	}
}

// TestTitleCaseHandlesNonASCII covers a section whose directory name starts
// with a multi-byte character. Upper-casing the first byte of one produces
// U+FFFD, so the section used to be titled "ber" with a replacement character
// in front of it.
func TestTitleCaseHandlesNonASCII(t *testing.T) {
	tests := []struct{ in, want string }{
		{"design", "Design"},
		{"multi-word-name", "Multi Word Name"},
		{"snake_case_name", "Snake Case Name"},
		{"über", "Über"},
		{"über-notes", "Über Notes"},
		{"中文文档", "中文文档"},
		{"русский", "Русский"},
		{"Ελληνικά", "Ελληνικά"},
		{"2024-release", "2024 Release"},
		{"a", "A"},
	}
	for _, tc := range tests {
		if got := titleCase(tc.in); got != tc.want {
			t.Errorf("titleCase(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if strings.ContainsRune(titleCase(tc.in), 0xFFFD) {
			t.Errorf("titleCase(%q) produced a replacement character", tc.in)
		}
	}
}

// A non-ASCII section name reaches the sidebar, the breadcrumb and the URL, so
// the whole path has to survive rather than just the helper.
func TestNonASCIISectionNameRenders(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "über"))
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\n---\n# Home\n\n[Zurück](über/index.md)\n")
	mustWrite(t, filepath.Join(dir, "über", "index.md"), "---\ntitle: Über\n---\n# Über\n")

	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	var title string
	for _, s := range r.Sections() {
		if s.ID == "über" {
			title = s.Title
		}
	}
	if title != "Über" {
		t.Errorf("section title = %q, want %q", title, "Über")
	}

	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "wiki", "über", "index.html")); err != nil {
		t.Errorf("the page was not written under its own name: %v", err)
	}
	home, err := os.ReadFile(filepath.Join(out, "wiki", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(home)
	if strings.ContainsRune(html, 0xFFFD) {
		t.Error("the rendered page contains a replacement character")
	}
	if !strings.Contains(html, `href="/wiki/über/index.html"`) {
		t.Errorf("the link to the section did not keep the character: %s", html)
	}
}

// A theme that parses but fails against the data used to write a page whose
// body was the error text, and the render still reported success. A broken
// theme shipped a site of error pages, and the Pages workflow published it.
func TestThemeExecuteFailureFailsTheRender(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\n---\n# Home\n")

	// Parses cleanly, but Nope is not a field of shellData.
	dir2 := writeTheme(t,
		`{"name":"broken","version":"1","template":"t.html"}`,
		map[string]string{"t.html": "<html><body>{{ .Nope.Missing }}</body></html>"})

	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}, ThemeDir: dir2})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	err = r.RenderAll(out)
	if err == nil {
		t.Fatal("RenderAll succeeded with a theme that cannot execute")
	}
	if !strings.Contains(err.Error(), "Nope") {
		t.Errorf("error does not name the offending field: %v", err)
	}
	// The page must not exist: a failed render leaves no page to be served.
	page := filepath.Join(out, "wiki", "index.html")
	if _, statErr := os.Stat(page); statErr == nil {
		body, _ := os.ReadFile(page)
		if strings.Contains(string(body), "render error") {
			t.Error("the error was written into the page instead of returned")
		}
	}
}

// The error names the page and the theme, so a failure in a large bundle points
// at the thing to fix.
func TestRenderPageErrorNamesPageAndTheme(t *testing.T) {
	over := writeTheme(t,
		`{"name":"bad","version":"1","template":"t.html"}`,
		map[string]string{"t.html": "{{ .Nope.Missing }}"})
	th, err := ResolveTheme(over)
	if err != nil {
		t.Fatal(err)
	}
	r := &Renderer{cfg: Config{Brand: Brand{Name: "x"}}, theme: th, assetVersion: th.AssetVersion()}

	_, err = r.RenderPage(Page{Slug: "guide/setup", Title: "Hello"})
	if err == nil {
		t.Fatal("RenderPage succeeded with a template that cannot execute")
	}
	for _, want := range []string{"guide/setup", th.Source, "Nope"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// The CDN fallback is only reached when the vendor bundle is missing, which is
// exactly when a build was skipped and the reader is the one paying for it. Its
// versions have to be the ones the vendored bundle is built from, or the same
// page can draw a diagram one way offline and another way online.
func TestDiagramCDNFallbackMatchesTheVendoredVersions(t *testing.T) {
	root := docsRoot(t)
	pkg, err := os.ReadFile(filepath.Join(root, "mermaid", "package.json"))
	if err != nil {
		t.Skipf("no vendored bundle in this checkout: %v", err)
	}
	var m struct {
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(pkg, &m); err != nil {
		t.Fatal(err)
	}
	js, err := os.ReadFile(filepath.Join(root, "internal", "wiki", "themes", "default",
		"assets", "diagrams.js"))
	if err != nil {
		t.Fatal(err)
	}
	client := string(js)

	for _, dep := range []string{"mermaid", "@mermaid-js/layout-elk"} {
		version, ok := m.DevDependencies[dep]
		if !ok {
			t.Errorf("%s is not pinned in package.json", dep)
			continue
		}
		if !strings.Contains(client, "@"+version+"/") {
			t.Errorf("diagrams.js does not reference %s@%s; the CDN fallback and the "+
				"vendored bundle have drifted apart", dep, version)
		}
	}
	// A major range is exactly what this guards against.
	if strings.Contains(client, "mermaid@11/") || strings.Contains(client, "layout-elk/dist") {
		t.Error("diagrams.js still uses a moving version range")
	}
}

func docsRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// A bundle with a diagram and no vendor directory produced a site that looked
// fine and was not offline: the runtime fell back to a public CDN, so the
// reader's browser reached the internet. The failure was silent, because a
// missing vendor directory was not an error.
func TestMissingVendorRuntimeIsAnError(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"),
		"---\ntitle: Home\n---\n# Home\n\n```mermaid\nflowchart LR\n  A --> B\n```\n")

	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"},
		VendorDir: filepath.Join(t.TempDir(), "absent")})
	if err != nil {
		t.Fatal(err)
	}
	err = r.RenderAll(t.TempDir())
	if err == nil {
		t.Fatal("RenderAll succeeded with a diagram and no vendor runtime")
	}
	msg := err.Error()
	for _, want := range []string{"vendor", "CDN", "offline"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error does not mention %q: %v", want, err)
		}
	}
}

// A bundle with no diagram needs no runtime, so a missing vendor directory is
// not its problem and must not fail the render.
func TestMissingVendorRuntimeIsFineWithoutDiagrams(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\n---\n# Home\n\nNo diagram here.\n")

	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"},
		VendorDir: filepath.Join(t.TempDir(), "absent")})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatalf("a bundle with no diagram should not need the vendor runtime: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "wiki", "index.html")); err != nil {
		t.Errorf("the page was not written: %v", err)
	}
}

// A vendor path that is a file rather than a directory is the same mistake, and
// has to be reported the same way.
func TestVendorPathThatIsAFileIsAnError(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"),
		"---\ntitle: Home\n---\n# Home\n\n```mermaid\nflowchart LR\n  A --> B\n```\n")
	vendor := filepath.Join(t.TempDir(), "notadir")
	mustWrite(t, vendor, "oops")

	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}, VendorDir: vendor})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RenderAll(t.TempDir()); err == nil {
		t.Fatal("RenderAll succeeded with a vendor path that is a file")
	}
}

// Our own bundle has diagrams, so its render must carry the runtime. This is the
// check CI would otherwise only notice by reading a screenshot.
func TestBundledDocsRenderCarriesTheVendorRuntime(t *testing.T) {
	if !hasMermaidBundle(t) {
		t.Skip("no vendored bundle built in this checkout")
	}
	r, err := New(Config{Content: docsDir(t), Base: "/wiki/", Brand: Brand{Name: "okf-wiki"},
		VendorDir: filepath.Join(docsRoot(t), "mermaid")})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(out, "wiki", "vendor", "mermaid-bundle.min.mjs")
	st, err := os.Stat(bundle)
	if err != nil {
		t.Fatalf("the diagram runtime was not copied: %v", err)
	}
	if st.Size() == 0 {
		t.Error("the copied diagram runtime is empty")
	}
}

func hasMermaidBundle(t *testing.T) bool {
	t.Helper()
	st, err := os.Stat(filepath.Join(docsRoot(t), "mermaid", "mermaid-bundle.min.mjs"))
	return err == nil && st.Size() > 0
}

// TestTagPagesRender covers the tag namespace end to end. Tag chips link to a
// page per tag, the page lists every page carrying it, and the catalogue the
// search modal reads names the same pages.
func TestTagPagesRender(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), strings.Join([]string{
		"---", "title: Home", "type: Overview", "tags: [alpha, shared]", "---",
		"# Home", "",
	}, "\n"))
	mustMkdir(t, filepath.Join(dir, "guide"))
	mustWrite(t, filepath.Join(dir, "guide", "intro.md"), strings.Join([]string{
		"---", "title: Intro", "type: Guide", "tags: [Shared, beta]", "---",
		"# Intro", "",
	}, "\n"))
	mustWrite(t, filepath.Join(dir, "guide", "plain.md"), strings.Join([]string{
		"---", "title: Plain", "type: Guide", "---",
		"# Plain", "",
	}, "\n"))

	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	site := filepath.Join(out, "wiki")

	// "Shared" and "shared" slugify alike, so they share one page under the
	// first spelling seen.
	shared, err := os.ReadFile(filepath.Join(site, "tags", "shared.html"))
	if err != nil {
		t.Fatalf("shared tag page was not written: %v", err)
	}
	html := string(shared)
	for _, want := range []string{`href="/wiki/index.html"`, `href="/wiki/guide/intro.html"`, ">Home<", ">Intro<"} {
		if !strings.Contains(html, want) {
			t.Errorf("shared tag page does not contain %s", want)
		}
	}
	// Membership is asserted against the listing itself: the rendered shell
	// carries the sidebar, which links every page on every page.
	listings := r.tagListings()
	var sharedListing tagListing
	for _, l := range listings {
		if l.Slug == "shared" {
			sharedListing = l
		}
	}
	listBody := r.tagPageBody(sharedListing, listings, 0, time.Now().UTC())
	for _, want := range []string{
		`data-tag-group="Overview"`,
		`data-tag-group="Guide"`,
		`data-title="Home"`,
		`data-title="Intro"`,
		`href="/wiki/guide/intro.html"`,
		`<span class="chip">Overview</span>`,
		`<span class="chip">Guide</span>`,
		`data-tag-setmode="section"`,
		`data-tag-setmode="all"`,
		`id="all-tags"`,
		`aria-current="true"`,
	} {
		if !strings.Contains(listBody, want) {
			t.Errorf("shared listing does not contain %s", want)
		}
	}
	if strings.Contains(listBody, "plain.html") {
		t.Error("shared listing links an untagged page")
	}
	// The hook names live in the inline script regardless; what must be
	// absent is the rendered toggle and count line. The trailing > keeps the
	// match on markup, since the script mentions the same hooks.
	if strings.Contains(listBody, `<label class="tag-check">`) ||
		strings.Contains(listBody, `data-tag-dep-note>`) {
		t.Error("shared listing has deprecated controls with nothing deprecated")
	}
	// Often-with names the co-occurring tags with counts, excluding the tag
	// itself, and points at the full catalogue. The hash is its own span so
	// themes can mute it without muting the name.
	for _, want := range []string{
		`href="/wiki/tags/alpha.html"><span class="tag-hash">#</span>alpha 1`,
		`href="/wiki/tags/beta.html"><span class="tag-hash">#</span>beta 1`,
		`href="#all-tags"`,
	} {
		if !strings.Contains(listBody, want) {
			t.Errorf("shared listing does not contain %s", want)
		}
	}

	// Chips link to the tag page; the type chip stays a span.
	home, err := os.ReadFile(filepath.Join(site, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(home), `<a class="tag-link" href="/wiki/tags/alpha.html"><span class="tag-hash">#</span>alpha</a>`) {
		t.Error("home page has no link chip for tag alpha")
	}
	if !strings.Contains(string(home), `<a class="tag-link" href="/wiki/tags/shared.html"><span class="tag-hash">#</span>shared</a>`) {
		t.Error("home page has no link chip for tag shared")
	}
	if !strings.Contains(string(home), `<span class="chip chip-type">Overview</span>`) {
		t.Error("type chip should stay a span")
	}

	// The catalogue names the same pages the tag page lists.
	raw, err := os.ReadFile(filepath.Join(site, "tags.json"))
	if err != nil {
		t.Fatal(err)
	}
	var tags []struct {
		Display string `json:"t"`
		URL     string `json:"u"`
		Pages   int    `json:"n"`
	}
	if err := json.Unmarshal(raw, &tags); err != nil {
		t.Fatal(err)
	}
	byURL := map[string]int{}
	for _, tg := range tags {
		byURL[tg.URL] = tg.Pages
	}
	want := map[string]int{
		"/wiki/tags/alpha.html":  1,
		"/wiki/tags/shared.html": 2,
		"/wiki/tags/beta.html":   1,
	}
	for u, n := range want {
		if byURL[u] != n {
			t.Errorf("tags.json: %s has %d pages, want %d (got %v)", u, byURL[u], n, byURL)
		}
	}
	if len(tags) != len(want) {
		t.Errorf("tags.json has %d rows, want %d", len(tags), len(want))
	}
}

// TestTagPageHasNoActiveNav asserts the tag page is an orphan view: the
// sidebar marks nothing active, and the breadcrumb reads Docs plus the tag.
func TestTagPageHasNoActiveNav(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\ntype: Overview\ntags: [alpha]\n---\n# Home\n")
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	listings := r.tagListings()
	if len(listings) != 1 {
		t.Fatalf("want 1 tag listing, got %d", len(listings))
	}
	body, err := r.RenderTagPage(listings[0], listings)
	if err != nil {
		t.Fatal(err)
	}
	// The header Docs link is hardcoded active on every page, so scope the
	// check to the sidebar's own active markers.
	if strings.Contains(body, `side-title-link active`) || strings.Contains(body, `<li class="active"`) {
		t.Error("tag page marks a sidebar entry active")
	}
	// A single bundle is unchanged by the tree work: Docs / Tags / #tag, byte
	// for byte.
	got := breadcrumbHTMLOf(t, body)
	want := `<a href="/wiki/">Docs</a><span class="crumb-sep">/</span>` +
		`<span>Tags</span><span class="crumb-sep">/</span>` +
		`<span class="crumb-cur"><span class="tag-hash">#</span>alpha</span>`
	if got != want {
		t.Errorf("single bundle tag trail =\n%s\nwant\n%s", got, want)
	}
	if !strings.Contains(body, "Docs</a>") || !strings.Contains(body, "<span>Tags</span>") ||
		!strings.Contains(body, `<span class="tag-hash">#</span>alpha</span>`) {
		t.Error("tag page breadcrumb does not read Docs / Tags / #tag")
	}
	if !strings.Contains(body, `<h1><span class="tag-hash">#</span>alpha</h1>`) {
		t.Error("tag page heading does not mute exactly the hash")
	}
}

// TestTagPageLifecycle covers the status-driven parts of a tag page: a
// deprecated page is hidden behind the toggle and counted, draft and stale
// pages carry badges. Trust tiers and dates are not rendered because no page
// is required to carry verified or generated frontmatter.
func TestTagPageLifecycle(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), strings.Join([]string{
		"---", "title: Home", "type: Overview", "tags: [alpha]", "---", "# Home", "",
	}, "\n"))
	mustWrite(t, filepath.Join(dir, "old.md"), strings.Join([]string{
		"---", "title: Old", "type: Guide", "status: deprecated", "tags: [alpha]", "---",
		"# Old", "",
	}, "\n"))
	mustWrite(t, filepath.Join(dir, "wip.md"), strings.Join([]string{
		"---", "title: Wip", "type: Guide", "status: draft", "tags: [alpha]", "---",
		"# Wip", "",
	}, "\n"))
	mustWrite(t, filepath.Join(dir, "rotten.md"), strings.Join([]string{
		"---", "title: Rotten", "type: Guide", "stale_after: 2000-01-01T00:00:00Z",
		"tags: [alpha]", "---", "# Rotten", "",
	}, "\n"))
	mustWrite(t, filepath.Join(dir, "fresh.md"), strings.Join([]string{
		"---", "title: Fresh", "type: Guide", "stale_after: 2999-01-01T00:00:00Z",
		"tags: [alpha]", "---", "# Fresh", "",
	}, "\n"))

	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	listings := r.tagListings()
	if len(listings) != 1 {
		t.Fatalf("want 1 tag listing, got %d", len(listings))
	}
	body, err := r.RenderTagPage(listings[0], listings)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(body, `<input type="checkbox" data-tag-dep>`) {
		t.Error("tag page with a deprecated page has no toggle")
	}
	if !strings.Contains(body, "1 deprecated hidden") {
		t.Error("tag page does not count the hidden deprecated page")
	}
	if !strings.Contains(body, `data-deprecated="true" hidden`) {
		t.Error("deprecated row is not hidden server-side")
	}
	for _, want := range []string{
		`<span class="chip">Draft</span>`,
		`<span class="chip">Stale</span>`,
		`<span class="chip">Deprecated</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("tag page does not contain %s", want)
		}
	}
	if strings.Count(body, `<span class="chip">Stale</span>`) != 1 {
		t.Error("the fresh page must not carry a Stale badge")
	}
}

// TestTagSlugWithoutURLForm covers tags that slugify to nothing. They have no
// page to point at, so the chip stays a span rather than a dead link.
func TestTagSlugWithoutURLForm(t *testing.T) {
	dir := t.TempDir()
	// Quoted: an unquoted !!! is a YAML tag directive and never reaches the list.
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\ntype: Overview\ntags: [\"!!!\", ok]\n---\n# Home\n")
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.tagListings()) != 1 {
		t.Errorf("want only the linkable tag listed, got %v", r.tagListings())
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	home, err := os.ReadFile(filepath.Join(out, "wiki", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(home), `<span class="tag-link">#!!!</span>`) {
		t.Error("unsluggable tag should render as a plain span")
	}
	if !strings.Contains(string(home), `<a class="tag-link" href="/wiki/tags/ok.html"><span class="tag-hash">#</span>ok</a>`) {
		t.Error("sluggable tag should render as a link")
	}
}

// TestTagPageCollisionFailsTheRender covers a bundle with a tags/ section and
// a tag claiming the same path. Overwriting either way loses a page silently,
// so the render fails and names both.
func TestTagPageCollisionFailsTheRender(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "tags"))
	mustWrite(t, filepath.Join(dir, "tags", "foo.md"), "---\ntitle: Foo\ntype: Guide\n---\n# Foo\n")
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\ntype: Overview\ntags: [foo]\n---\n# Home\n")
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	err = r.RenderAll(t.TempDir())
	if err == nil {
		t.Fatal("RenderAll succeeded with a tag colliding with a section page")
	}
	if !strings.Contains(err.Error(), "foo") {
		t.Errorf("error does not name the collision: %v", err)
	}
}

// TestTagsJSONEmptyBundle asserts an untagged bundle writes an empty array,
// so the search modal never has to distinguish null from missing.
func TestTagsJSONEmptyBundle(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\ntype: Overview\n---\n# Home\n")
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(out, "wiki", "tags.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(raw)) != "[]" {
		t.Errorf("tags.json = %s, want []", raw)
	}
	if _, err := os.Stat(filepath.Join(out, "wiki", "tags")); !os.IsNotExist(err) {
		t.Error("untagged bundle should not grow a tags directory")
	}
}

// TestBundledDocsTags wires the tag namespace into the suite's own-bundle
// render, so a regression in our own wiki is caught rather than only covered
// by fixtures.
func TestBundledDocsTags(t *testing.T) {
	r, err := New(Config{Content: docsDir(t), Base: "/wiki/", Brand: Brand{Name: "okf-wiki"}})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	site := filepath.Join(out, "wiki")
	raw, err := os.ReadFile(filepath.Join(site, "tags.json"))
	if err != nil {
		t.Fatal(err)
	}
	var tags []struct {
		Display string `json:"t"`
		URL     string `json:"u"`
		Pages   int    `json:"n"`
	}
	if err := json.Unmarshal(raw, &tags); err != nil {
		t.Fatal(err)
	}
	if len(tags) == 0 {
		t.Fatal("our own bundle produced no tags")
	}
	found := false
	for _, tg := range tags {
		p := filepath.Join(site, strings.TrimPrefix(tg.URL, "/wiki/"))
		if _, err := os.Stat(p); err != nil {
			t.Errorf("tag page %s missing: %v", tg.URL, err)
		}
		if tg.Display == "testbed" {
			found = true
			if tg.Pages < 5 {
				t.Errorf("tag testbed has %d pages, want at least 5", tg.Pages)
			}
		}
	}
	if !found {
		t.Error("tag testbed missing from our own catalogue")
	}
}

// TestPageNeighbors covers the previous/next cards: middle pages link both
// ways, ends link one way, a lone page links nowhere, and an unknown slug
// links nowhere rather than somewhere wrong.
func TestPageNeighbors(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "guide"))
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\n---\n# Home\n")
	mustWrite(t, filepath.Join(dir, "guide", "a.md"), "---\ntitle: Aye\n---\n# Aye\n")
	mustWrite(t, filepath.Join(dir, "guide", "b.md"), "---\ntitle: Bee\n---\n# Bee\n")
	mustWrite(t, filepath.Join(dir, "guide", "c.md"), "---\ntitle: Cee\n---\n# Cee\n")

	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}

	prev, next := r.pageNeighbors("guide", "guide/b")
	if prev == nil || prev.Title != "Aye" || prev.URL != "/wiki/guide/a.html" {
		t.Errorf("prev of guide/b = %+v, want Aye at /wiki/guide/a.html", prev)
	}
	if next == nil || next.Title != "Cee" || next.URL != "/wiki/guide/c.html" {
		t.Errorf("next of guide/b = %+v, want Cee at /wiki/guide/c.html", next)
	}

	prev, next = r.pageNeighbors("guide", "guide/a")
	if prev != nil {
		t.Errorf("first page should have no prev, got %+v", prev)
	}
	if next == nil || next.Title != "Bee" {
		t.Errorf("first page should link next Bee, got %+v", next)
	}

	prev, next = r.pageNeighbors("guide", "guide/c")
	if next != nil {
		t.Errorf("last page should have no next, got %+v", next)
	}

	prev, next = r.pageNeighbors("", "index")
	if prev != nil || next != nil {
		t.Errorf("lone top-level page should link nowhere, got %+v / %+v", prev, next)
	}

	prev, next = r.pageNeighbors("guide", "guide/missing")
	if prev != nil || next != nil {
		t.Errorf("unknown slug should link nowhere, got %+v / %+v", prev, next)
	}
}

// TestPrevNextHTML covers the card markup: both cards in order, a lone next
// holding the right column, and nothing at all for a lone page.
func TestPrevNextHTML(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "guide"))
	mustWrite(t, filepath.Join(dir, "guide", "a.md"), "---\ntitle: Aye\n---\n# Aye\n")
	mustWrite(t, filepath.Join(dir, "guide", "b.md"), "---\ntitle: Bee\n---\n# Bee\n")
	mustWrite(t, filepath.Join(dir, "guide", "c.md"), "---\ntitle: Cee\n---\n# Cee\n")
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}

	var mid Page
	for _, s := range r.Sections() {
		for _, p := range s.Pages {
			if p.Slug == "guide/b" {
				mid = p
			}
		}
	}
	html := r.prevNextHTML(mid)
	for _, want := range []string{
		`<nav class="prevnext"`,
		`class="prevnext-card prevnext-prev" href="/wiki/guide/a.html"`,
		`class="prevnext-card prevnext-next" href="/wiki/guide/c.html"`,
		`<span class="prevnext-dir">Previous</span>`,
		`<span class="prevnext-title">Aye</span>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("prevnext markup does not contain %s", want)
		}
	}
	if strings.Index(html, "prevnext-prev") > strings.Index(html, "prevnext-next") {
		t.Error("previous card must come before the next card")
	}

	// A lone next keeps the right column; the grid rule places it there.
	var first Page
	for _, s := range r.Sections() {
		for _, p := range s.Pages {
			if p.Slug == "guide/a" {
				first = p
			}
		}
	}
	single := r.prevNextHTML(first)
	if !strings.Contains(single, "prevnext-next") || strings.Contains(single, "prevnext-prev") {
		t.Errorf("first page should render only a next card: %s", single)
	}
}

// TestPrevNextAbsentAlone covers a section of one: no neighbors, no nav.
func TestPrevNextAbsentAlone(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\n---\n# Home\n")
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	var home Page
	for _, s := range r.Sections() {
		for _, p := range s.Pages {
			home = p
		}
	}
	if got := r.prevNextHTML(home); got != "" {
		t.Errorf("lone page should render no nav, got %s", got)
	}
}

// TestTOCAsidePlacement covers the relocated table of contents: the sidebar
// column and the dropdown panel carry the list, the body carries no
// collapsible box, and pages without headings carry none of it.
func TestTOCAsidePlacement(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), strings.Join([]string{
		"---", "title: Home", "---", "# Home", "## Second", "### Third", "",
	}, "\n"))
	mustWrite(t, filepath.Join(dir, "flat.md"), "---\ntitle: Flat\n---\n# Flat\n\nNo headings here.\n")
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	home, err := os.ReadFile(filepath.Join(out, "wiki", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(home)
	for _, want := range []string{
		`<aside class="toc-aside"`,
		`<ul class="toc-list">`,
		`id="tocBtn"`,
		`id="tocPanel" hidden`,
		`href="#second"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered page does not contain %s", want)
		}
	}
	// Level three is a link target but never a navigation entry.
	if strings.Contains(html, `href="#third"`) {
		t.Error("table of contents lists a level-three heading")
	}
	if strings.Contains(html, `<details class="toc"`) {
		t.Error("rendered page still carries the in-body collapsible box")
	}

	flat, err := os.ReadFile(filepath.Join(out, "wiki", "flat.html"))
	if err != nil {
		t.Fatal(err)
	}
	// Element markup, not hook names: the template's inline script mentions
	// the ids on every page regardless.
	for _, want := range []string{
		`<aside class="toc-aside"`,
		`id="tocBtn"`,
		`id="tocPanel"`,
		`<ul class="toc-list">`,
	} {
		if strings.Contains(string(flat), want) {
			t.Errorf("headingless page should carry no TOC markup, found %s", want)
		}
	}
}

// TestSourceFootnotes covers the sources family: a footnote whose label
// matches a sources[].id becomes a numbered link to the generated section,
// the section lists title, host, author and date, and an unmatched footnote
// keeps goldmark's own rendering.
func TestSourceFootnotes(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), strings.Join([]string{
		"---",
		"title: Home",
		"sources:",
		"  - id: spec",
		"    resource: https://example.invalid/okf/spec",
		"    title: The OKF specification",
		"    author: team:docs",
		"    last_modified: 2026-05-30T00:00:00Z",
		"  - id: scope",
		"    resource: all queries in project X",
		"    title: A scope descriptor",
		"  - id: uncited",
		"    resource: https://example.invalid/never",
		"    title: Declared but never cited",
		"---",
		"# Home",
		"",
		"A claim attributed to the spec.[^spec]",
		"",
		"Cited twice.[^spec] And a plain footnote.[^plain]",
		"",
		"[^spec]: The OKF specification",
		"[^plain]: An ordinary footnote.",
		"",
	}, "\n"))
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(out, "wiki", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(body)

	// Matched citation becomes a numbered ref to the section, reused on repeat.
	if !strings.Contains(html, `<a href="#sources" class="source-ref">[1]</a>`) {
		t.Error("matched footnote did not become a source ref")
	}
	if strings.Count(html, `class="source-ref"`) != 2 {
		t.Errorf("want 2 source refs, got %d", strings.Count(html, `class="source-ref"`))
	}
	// The section, its heading id, and the table of contents entry. The list
	// starts at 1: goldmark's default start is 0.
	if !strings.Contains(html, `<h2 id="sources">Sources`) {
		t.Error("Sources section missing")
	}
	if strings.Contains(html, `<ol start="0">`) {
		t.Error("Sources list is numbered from zero")
	}
	if !strings.Contains(html, `href="#sources"`) {
		t.Error("table of contents does not list Sources")
	}
	// Entry: title linked to the resource, then host, author and date.
	if !strings.Contains(html, `href="https://example.invalid/okf/spec"`) ||
		!strings.Contains(html, "The OKF specification") {
		t.Error("source entry does not link its title to the resource")
	}
	for _, want := range []string{"example.invalid", "team:docs", "30 May 2026"} {
		if !strings.Contains(html, want) {
			t.Errorf("source entry missing %s", want)
		}
	}
	// A scope descriptor has no href, so it renders as text.
	if !strings.Contains(html, "A scope descriptor") {
		t.Error("scope descriptor entry missing")
	}
	if strings.Contains(html, `href="all queries`) {
		t.Error("scope descriptor should not be linked")
	}
	// Every declared entry is listed, not only the cited ones, so a reader
	// sees the whole reference list the page declares.
	if !strings.Contains(html, "Declared but never cited") {
		t.Error("uncited source entry is missing from the list")
	}
	if strings.Contains(html, "Declared but never cited</a>") == false {
		t.Error("uncited entry should still link its resource")
	}
	// The unmatched footnote keeps goldmark's rendering and its definition.
	if !strings.Contains(html, `class="footnote-ref"`) || !strings.Contains(html, "An ordinary footnote.") {
		t.Error("unmatched footnote should render as a footnote")
	}
	// The matched definition is gone, so no dangling footnote text.
	if strings.Contains(html, "The OKF specification</a>:") {
		t.Error("matched footnote definition still renders")
	}
}

// TestSourceFootnotesAbsent covers pages without sources: no section, no
// refs, and footnotes exactly as goldmark renders them.
func TestSourceFootnotesAbsent(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), strings.Join([]string{
		"---", "title: Home", "---", "# Home", "",
		"A note.[^n]", "", "[^n]: The note body.", "",
	}, "\n"))
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(out, "wiki", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(body)
	if strings.Contains(html, `id="sources"`) {
		t.Error("page without sources grew a Sources section")
	}
	if strings.Contains(html, "source-ref") {
		t.Error("page without sources grew a source ref")
	}
	if !strings.Contains(html, "The note body.") {
		t.Error("ordinary footnote should still render")
	}
}

// TestSourceSectionIDCollision covers a page that already uses the id
// sources, so the generated section and its refs must move together.
func TestSourceSectionIDCollision(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), strings.Join([]string{
		"---", "title: Home", "sources:",
		"  - id: s", "    resource: https://example.invalid/x",
		"---",
		"# Home", "",
		"## Sources", "",
		"An authored section with the same name.",
		"",
		"Cited.[^s]",
		"",
		"[^s]: The OKF specification",
		"",
	}, "\n"))
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(out, "wiki", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(body)
	if !strings.Contains(html, `<h2 id="sources-2">Sources`) {
		t.Error("generated Sources section did not step past the authored id")
	}
	if !strings.Contains(html, `<a href="#sources-2" class="source-ref">[1]</a>`) {
		t.Error("source ref does not point at the uniquified section")
	}
}

// TestSourceMetaRendering covers the signals: host only for absolute URLs,
// absent signals omitted rather than blank, an unparsable date shown raw.
func TestSourceMetaRendering(t *testing.T) {
	tests := []struct {
		name string
		src  Source
		want []string
		not  []string
	}{
		{
			name: "absolute url with every signal",
			src: Source{Resource: "https://wiki.example/p", Author: "team:a",
				LastModified: "2026-04-02T00:00:00Z"},
			want: []string{"wiki.example", "team:a", "2 Apr 2026"},
		},
		{
			name: "bundle path has no host",
			src:  Source{Resource: "../reference/themes.md", Title: "Themes"},
			not:  []string{"·"},
		},
		{
			name: "unparsable date shown raw",
			src:  Source{Resource: "https://e.invalid", LastModified: "sometime last year"},
			want: []string{"e.invalid", "sometime last year"},
		},
		{
			name: "scope descriptor has no host",
			src:  Source{Resource: "all queries in project X", Title: "Scope"},
			not:  []string{"·"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sourceMeta(&tc.src)
			joined := strings.Join(got, " · ")
			for _, w := range tc.want {
				if !strings.Contains(joined, w) {
					t.Errorf("meta %q missing %q", joined, w)
				}
			}
			for _, n := range tc.not {
				if strings.Contains(joined, n) {
					t.Errorf("meta %q should carry no separators, got %q", joined, n)
				}
			}
		})
	}
}

// TestDiagramCardContract guards the pieces the diagram card needs, since the
// behaviour itself is client-side and cannot be asserted from Go. Each of
// these was a real defect: diagrams shrank to fit, sequence text rendered a
// step larger, and a failed diagram left a blank block.
func TestDiagramCardContract(t *testing.T) {
	js, err := os.ReadFile(filepath.Join(docsRoot(t), "internal", "wiki",
		"themes", "default", "assets", "diagrams.js"))
	if err != nil {
		t.Fatal(err)
	}
	client := string(js)
	// Every renderer reads useMaxWidth from its own config block, so a type
	// missing from the list silently shrinks to the column.
	for _, want := range []string{"TYPES.forEach", "useMaxWidth: false", "'flowchart'", "'sequence'", "'state'"} {
		if !strings.Contains(client, want) {
			t.Errorf("diagrams.js does not contain %q", want)
		}
	}
	// The failure card, and the frame that gives it the card shape.
	for _, want := range []string{"mermaid-error", "mermaid-error-detail", "mermaid-error-source", "wrapFrame"} {
		if !strings.Contains(client, want) {
			t.Errorf("diagrams.js does not contain %q", want)
		}
	}
	// The toolbar carries a type label, a zoom group and two glyph buttons, no text.
	for _, want := range []string{
		"mermaid-toolbar", "mermaid-type", "mermaid-zoom-out", "mermaid-zoom-in",
		"mermaid-zoom-pct", "mermaid-copy", "mermaid-expand",
	} {
		if !strings.Contains(client, want) {
			t.Errorf("diagrams.js does not contain %q", want)
		}
	}
	// The viewer opens on the whole drawing: one larger than the stage is
	// scaled down to fit, so an edge is not already off screen.
	if !strings.Contains(client, "Math.min(fit, 1.25)") {
		t.Error("the lightbox does not fit a diagram larger than the stage")
	}
	// The card opens fitted to its box, and steps the scale from there.
	if !strings.Contains(client, "cardFitScale") {
		t.Error("the card has no fit-to-box scale")
	}
	// Clicking outside must close, and a drag must not.
	if !strings.Contains(client, "downAt") {
		t.Error("lightbox has no drag guard on the outside-click close")
	}

	css, err := os.ReadFile(filepath.Join(docsRoot(t), "internal", "wiki",
		"themes", "default", "assets", "wiki.css"))
	if err != nil {
		t.Fatal(err)
	}
	sheet := string(css)
	for _, want := range []string{
		".mermaid-frame {",
		".mermaid-toolbar {",
		".mermaid-error {",
		"max-height: 560px",
	} {
		if !strings.Contains(sheet, want) {
			t.Errorf("wiki.css does not contain %q", want)
		}
	}
	// Mermaid writes the sequence font size inline from a snapshot of its own
	// defaults, so only an important stylesheet rule can pin it.
	if !strings.Contains(sheet, ".mermaid-frame .messageText") ||
		!strings.Contains(sheet, "font-size: 14px !important") {
		t.Error("wiki.css does not pin the sequence diagram font size")
	}
	// The viewer's clone lays out at the card's text size, or mermaid's labels
	// outgrow the boxes they were measured for.
	if !strings.Contains(sheet, "will-change: transform; font-size: var(--text-sm);") {
		t.Error("the lightbox clone does not pin the diagram text size")
	}
	// The failure card must read as a card, not a stray block.
	if !strings.Contains(sheet, "var(--status-danger-tint)") {
		t.Error("failure card carries no danger tint")
	}
}

// TestWatchSignatureTracksChanges covers the reload trigger: an edit to the
// bundle changes the signature, a dotfile does not, and a missing theme root is
// not a permanent difference.
func TestWatchSignatureTracksChanges(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "# Home\n")
	base := watchSignature([]string{dir})

	if again := watchSignature([]string{dir}); again != base {
		t.Error("the signature changed with no change on disk")
	}
	mustWrite(t, filepath.Join(dir, ".hidden"), "secret")
	if withDot := watchSignature([]string{dir}); withDot != base {
		t.Error("a dotfile changed the signature, so a .git write would re-render")
	}
	mustWrite(t, filepath.Join(dir, "index.md"), "# Home, edited\n")
	if after := watchSignature([]string{dir}); after == base {
		t.Error("an edit did not change the signature")
	}
	if missing := watchSignature([]string{filepath.Join(dir, "nope")}); missing == "" {
		t.Error("a missing root produced no signature")
	}
}

// TestWatchRendersOnChange covers the reload loop: it renders when a file
// changes and returns when its context is done. That a dotfile is not a change
// is asserted on the signature itself, in the test above, because asserting it
// through the live loop makes the test race the poll interval.
// TestSwapDirReplacesAtomically covers the swap: a built tree becomes dest, a
// build that fails leaves the previous tree in place, and no temporary or set-
// aside directory is left behind either way.
func TestSwapDirReplacesAtomically(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "site")
	mustMkdir(t, dest)
	mustWrite(t, filepath.Join(dest, "old.html"), "old\n")

	if err := swapDir(dest, func(tmp string) error {
		mustWrite(t, filepath.Join(tmp, "new.html"), "new\n")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "new.html")); err != nil {
		t.Errorf("the built tree did not reach dest: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "old.html")); err == nil {
		t.Error("the previous tree survived the swap, so it was updated rather than replaced")
	}

	// A build that fails must not disturb what is being served.
	before, err := os.ReadFile(filepath.Join(dest, "new.html"))
	if err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("render failed")
	if err := swapDir(dest, func(tmp string) error { return wantErr }); !errors.Is(err, wantErr) {
		t.Fatalf("swapDir error = %v, want %v", err, wantErr)
	}
	after, err := os.ReadFile(filepath.Join(dest, "new.html"))
	if err != nil || string(after) != string(before) {
		t.Errorf("a failed build changed dest: %v, %q", err, after)
	}

	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "site" {
			t.Errorf("swap left %q behind in the output parent", e.Name())
		}
	}
}

func TestWatchRendersOnChange(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "# Home\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	renders := make(chan struct{}, 8)
	done := make(chan struct{})
	go func() {
		defer close(done)
		Watch(ctx, WatchOptions{
			Content:  dir,
			Interval: 20 * time.Millisecond,
			Render:   func() error { renders <- struct{}{}; return nil },
			Logf:     func(string, ...any) {},
		})
	}()

	// The watcher takes its baseline before the first tick, so a change after a
	// short pause is seen as a change.
	time.Sleep(100 * time.Millisecond)
	mustWrite(t, filepath.Join(dir, "index.md"), "# Home, edited\n")
	select {
	case <-renders:
	case <-time.After(3 * time.Second):
		t.Fatal("a change did not trigger a render")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Watch did not return when its context was cancelled")
	}
}

// TestMultiSiteRendersTree covers the tree mode: each directory holding an
// index.md is a wiki rendered at its own path under the base, a directory
// without one is a group given an index of its children, and the output root
// still points at the tree.
func TestMultiSiteRendersTree(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "clients"))
	mustWrite(t, filepath.Join(dir, "clients", ".meta.json"),
		`{"title": "Clients", "description": "Engagement wikis."}`)
	mustMkdir(t, filepath.Join(dir, "clients", "example", "operations"))
	mustWrite(t, filepath.Join(dir, "clients", "example", "operations", "index.md"),
		"---\ntype: Overview\ntitle: Operations\n---\n# Operations\n")
	mustMkdir(t, filepath.Join(dir, "homelab"))
	mustWrite(t, filepath.Join(dir, "homelab", "index.md"),
		"---\ntype: Overview\ntitle: Homelab\n---\n# Homelab\n")

	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "okf-wiki"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	wikis, err := r.RenderSites(out)
	if err != nil {
		t.Fatal(err)
	}
	if wikis != 2 {
		t.Fatalf("rendered %d wikis, want 2", wikis)
	}
	site := filepath.Join(out, "family")

	// A group is given an index of its children: the wikis lead as cards and
	// the folders follow in their own section.
	root, err := os.ReadFile(filepath.Join(site, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	rootHTML := string(root)
	if !strings.Contains(rootHTML, `href="/family/clients/">`) ||
		!strings.Contains(rootHTML, `<span class="folder-name">Clients</span>`) {
		t.Error("the root index does not list the clients group as a folder")
	}
	if !strings.Contains(rootHTML, `site-badge-wiki">Wiki</span>`) {
		t.Error("the root index does not badge a wiki")
	}
	// A folder is a compact row under its own heading, not a wiki card.
	if !strings.Contains(rootHTML, `<div class="site-group"><h2 class="site-group-title">Folders</h2><ul class="folder-list">`) {
		t.Error("the folders are not grouped under their own heading")
	}
	if strings.Contains(rootHTML, `site-badge-folder`) {
		t.Error("a folder still renders as a badge-carrying card rather than a row")
	}
	// The wiki card is clickable across its whole face through the link's
	// overlay, not by widening the link text: the anchor ends after the badge, so
	// a screen reader names the card by its title.
	if !strings.Contains(rootHTML, `site-badge-wiki">Wiki</span></a>`) {
		t.Error("the card link wraps more than the title and badge")
	}
	// A group names itself from its own metadata.
	clients, err := os.ReadFile(filepath.Join(site, "clients", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(clients), "<h1>Clients</h1>") ||
		!strings.Contains(string(clients), "Engagement wikis.") {
		t.Error("the group index does not use its .meta.json name and description")
	}
	// A wiki is rendered at its own path under the base, with its own assets.
	ops, err := os.ReadFile(filepath.Join(site, "clients", "example", "operations", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ops), `href="/family/clients/example/operations/wiki.css`) {
		t.Error("a tree wiki's assets are not under its own path")
	}
	if !strings.Contains(string(ops), "<h1>Operations</h1>") {
		t.Error("a tree wiki does not render its own home page")
	}
	// The output root still points at the tree.
	redirect, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(redirect), "url=/family/") {
		t.Error("the output root does not redirect to the base")
	}
}

// localAssetRefs returns the file references (href/src) in a rendered page that
// point at a local file rather than a directory or an anchor. Catalog links to
// wikis and groups end in a slash, so the two are distinguished by shape.
func localAssetRefs(html string) []string {
	var refs []string
	for _, attr := range []string{`href="`, `src="`} {
		rest := html
		for {
			i := strings.Index(rest, attr)
			if i < 0 {
				break
			}
			rest = rest[i+len(attr):]
			j := strings.Index(rest, `"`)
			if j < 0 {
				break
			}
			ref := rest[:j]
			rest = rest[j:]
			if ref == "" || strings.HasPrefix(ref, "#") ||
				strings.Contains(ref, "://") || strings.HasSuffix(ref, "/") {
				continue
			}
			refs = append(refs, ref)
		}
	}
	return refs
}

// TestMultiSiteGroupAssetsSitAtTheBase covers a tree published under a non-root
// base. A group index renders with the tree renderer, so every asset and
// catalogue it points at carries the base and must sit under the base
// directory, not at the output root a base of "/" happens to share with it. Get
// this wrong and every group index is unstyled and its search silently empty
// while the wikis inside it still render, which is easy to miss.
func TestMultiSiteGroupAssetsSitAtTheBase(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "clients"))
	mustWrite(t, filepath.Join(dir, "clients", ".meta.json"),
		`{"title": "Clients", "description": "Engagement wikis."}`)
	mustMkdir(t, filepath.Join(dir, "clients", "example", "operations"))
	mustWrite(t, filepath.Join(dir, "clients", "example", "operations", "index.md"),
		"---\ntype: Overview\ntitle: Operations\n---\n# Operations\n")
	mustMkdir(t, filepath.Join(dir, "homelab"))
	mustWrite(t, filepath.Join(dir, "homelab", "index.md"),
		"---\ntype: Overview\ntitle: Homelab\n---\n# Homelab\n")

	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "okf-wiki"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(out); err != nil {
		t.Fatal(err)
	}

	// The theme's assets and the empty catalogue belong beside the group
	// indexes that reference them, under the base.
	baseDir := filepath.Join(out, "family")
	for _, rel := range []string{
		"wiki.css", "tokens.css", "fonts.css", "favicon.svg", "search.js",
		"graph.js", "diagrams.js", "brand-mark.svg", "search-index.json", "graph.json",
		"graph.html", "tags.json",
	} {
		if _, err := os.Stat(filepath.Join(baseDir, rel)); err != nil {
			t.Errorf("group asset %s is not under the base: %v", rel, err)
		}
	}
	// The output root belongs to the redirect, not to the theme.
	if _, err := os.Stat(filepath.Join(out, "wiki.css")); err == nil {
		t.Error("the theme was written to the output root instead of the base")
	}

	// Every local file the root group index and a nested one references
	// resolves on disk, and each page carries the tree base it was rendered
	// with.
	for _, page := range []string{
		filepath.Join(baseDir, "index.html"),
		filepath.Join(baseDir, "clients", "index.html"),
	} {
		body, err := os.ReadFile(page)
		if err != nil {
			t.Fatal(err)
		}
		html := string(body)
		if !strings.Contains(html, `window.__WIKI_BASE = '\/family\/'`) {
			t.Errorf("%s does not carry the tree base", page)
		}
		if !strings.Contains(html, `href="/family/wiki.css`) {
			t.Errorf("%s does not reference its stylesheet under the base", page)
		}
		for _, ref := range localAssetRefs(html) {
			rel := strings.TrimPrefix(ref, "/family/")
			rel = strings.SplitN(rel, "?", 2)[0]
			if _, err := os.Stat(filepath.Join(out, "family", filepath.FromSlash(rel))); err != nil {
				t.Errorf("%s references %s but it is not on disk: %v", page, ref, err)
			}
		}
	}

	// The output root still redirects into the base, so a host at / lands on
	// the tree.
	redirect, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(redirect), "url=/family/") {
		t.Error("the output root does not redirect to the base")
	}
}

// TestMultiSiteRootBaseKeepsAssetsAtTheRoot pins the base "/" case, where the
// output root and the site directory are the same: the tree-base assets stay at
// the output root, and the root group index is the home page rather than a
// redirect.
func TestMultiSiteRootBaseKeepsAssetsAtTheRoot(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "homelab"))
	mustWrite(t, filepath.Join(dir, "homelab", "index.md"),
		"---\ntype: Overview\ntitle: Homelab\n---\n# Homelab\n")

	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/", Brand: Brand{Name: "okf-wiki"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(out); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"wiki.css", "search.js", "search-index.json"} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Errorf("asset %s is not at the output root for base /: %v", rel, err)
		}
	}
	home, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(home), `href="/homelab/"`) {
		t.Error("the output root is not the root group index")
	}
}

// TestMultiSiteSkipsABrokenWiki covers a tree where one wiki cannot be read: a
// stale link in a linked checkout, a directory that vanished. The tree still
// renders, the bad wiki is reported through SkippedSites, its partial output is
// removed so nothing serves it, and no catalog card links to it.
func TestMultiSiteSkipsABrokenWiki(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "good"))
	mustWrite(t, filepath.Join(dir, "good", "index.md"),
		"---\ntype: Overview\ntitle: Good\n---\n# Good\n")
	mustMkdir(t, filepath.Join(dir, "broken"))
	mustWrite(t, filepath.Join(dir, "broken", "index.md"),
		"---\ntype: Overview\ntitle: Broken\n---\n# Broken\n")
	// A link that resolves to nothing, which is what a moved checkout leaves.
	if err := os.Symlink("/nonexistent/elsewhere", filepath.Join(dir, "broken", "docs")); err != nil {
		t.Skipf("symlinks are unavailable here: %v", err)
	}

	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/", Brand: Brand{Name: "t"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	wikis, err := r.RenderSites(out)
	if err != nil {
		t.Fatalf("a broken wiki failed the whole tree: %v", err)
	}
	if wikis != 1 {
		t.Errorf("rendered %d wikis, want 1 for the readable one", wikis)
	}
	if _, err := os.Stat(filepath.Join(out, "good", "index.html")); err != nil {
		t.Errorf("the readable wiki was not rendered: %v", err)
	}
	// The skipped wiki must leave nothing behind: a partial directory would be
	// served as though it had rendered.
	if _, err := os.Stat(filepath.Join(out, "broken")); err == nil {
		t.Error("the skipped wiki left output that the server would serve")
	}

	skipped := r.SkippedSites()
	if len(skipped) != 1 || skipped[0].Rel != "broken" {
		t.Fatalf("SkippedSites = %+v, want one entry for broken", skipped)
	}
	if skipped[0].Err == nil {
		t.Error("the skipped wiki carries no reason")
	}

	root, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(root), `href="/broken/"`) {
		t.Error("the catalog links to a wiki that was skipped")
	}
	if !strings.Contains(string(root), `href="/good/"`) {
		t.Error("the catalog is missing the wiki that rendered")
	}
}

// TestMultiSiteFollowsSymlinkedBundles covers a tree assembled by linking
// bundles in from elsewhere: a directory symlink counts as a directory, a change
// inside a linked bundle is a change to the site, and a link back into the tree
// is walked once rather than expanding.
func TestMultiSiteFollowsSymlinkedBundles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "content")

	mustMkdir(t, filepath.Join(dir, "home"))
	mustWrite(t, filepath.Join(dir, "home", "index.md"),
		"---\ntype: Overview\ntitle: Home\n---\n# Home\n")

	linked := filepath.Join(root, "elsewhere")
	mustMkdir(t, linked)
	mustWrite(t, filepath.Join(linked, "index.md"),
		"---\ntype: Overview\ntitle: Linked\n---\n# Linked\n")
	// A non-Markdown file under the linked bundle, so the copy step is covered:
	// a linked bundle's assets must reach the output, or its pages render with
	// every image broken.
	mustMkdir(t, filepath.Join(linked, "assets", "demos"))
	mustWrite(t, filepath.Join(linked, "assets", "demos", "shot.png"), "png")

	if err := os.Symlink(linked, filepath.Join(dir, "linked")); err != nil {
		t.Skipf("symlinks are unavailable here: %v", err)
	}
	// A link back to the tree itself would expand until the operating system
	// refused the path length without the visited guard.
	if err := os.Symlink(dir, filepath.Join(dir, "loop")); err != nil {
		t.Skipf("symlinks are unavailable here: %v", err)
	}

	// The watcher has to see a change inside a linked bundle, or a tree renders
	// once and never updates.
	before := watchSignature([]string{dir})
	mustWrite(t, filepath.Join(linked, "extra.md"),
		"---\ntype: Guide\ntitle: Extra\n---\n# Extra\n")
	if after := watchSignature([]string{dir}); after == before {
		t.Error("a change inside a linked bundle did not change the watch signature")
	}

	// The content root itself may be a symlink. WalkDir does not follow its
	// own root, so the signature would never change unless it is resolved.
	rootLink := filepath.Join(root, "content-link")
	if err := os.Symlink(dir, rootLink); err != nil {
		t.Skipf("symlinks are unavailable here: %v", err)
	}
	beforeRoot := watchSignature([]string{rootLink})
	mustWrite(t, filepath.Join(dir, "home", "index.md"),
		"---\ntype: Overview\ntitle: Home\n---\n# Home, edited\n")
	if afterRoot := watchSignature([]string{rootLink}); afterRoot == beforeRoot {
		t.Error("a change under a symlinked content root did not change the signature")
	}

	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/", Brand: Brand{Name: "t"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	wikis, err := r.RenderSites(out)
	if err != nil {
		t.Fatal(err)
	}
	if wikis != 2 {
		t.Fatalf("rendered %d wikis, want 2 for a local bundle and a linked one", wikis)
	}
	if _, err := os.Stat(filepath.Join(out, "linked", "index.html")); err != nil {
		t.Errorf("a linked bundle was not rendered: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "linked", "assets", "demos", "shot.png")); err != nil {
		t.Errorf("a linked bundle's asset was not copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "loop")); err == nil {
		t.Error("a link back into the tree was walked again instead of stopping")
	}
}

// treeSearchFixture writes a three-wiki tree plus one wiki that cannot be read,
// and returns its content root. Every wiki carries one distinctive phrase so a
// test can tell which wiki a hit came from.
func treeSearchFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "alpha"))
	mustWrite(t, filepath.Join(dir, "alpha", "index.md"),
		"---\ntype: Overview\ntitle: Alpha\n---\n# Alpha\n\nzephyrquartz lives here.\n")
	mustMkdir(t, filepath.Join(dir, "clients", "example", "operations"))
	mustWrite(t, filepath.Join(dir, "clients", "example", "operations", "index.md"),
		"---\ntype: Overview\ntitle: Operations\n---\n# Operations\n\nexampleneedle sits in a sibling wiki.\n")
	mustMkdir(t, filepath.Join(dir, "clients", "example", "strategy"))
	mustWrite(t, filepath.Join(dir, "clients", "example", "strategy", "index.md"),
		"---\ntype: Overview\ntitle: Strategy\n---\n# Strategy\n")
	mustMkdir(t, filepath.Join(dir, "homelab"))
	mustWrite(t, filepath.Join(dir, "homelab", "index.md"),
		"---\ntype: Overview\ntitle: Homelab\n---\n# Homelab\n\nhomelabneedle is here.\n")
	// A wiki whose bundle cannot be walked: a link that resolves to nothing,
	// which is what a moved checkout leaves.
	mustMkdir(t, filepath.Join(dir, "broken"))
	mustWrite(t, filepath.Join(dir, "broken", "index.md"),
		"---\ntype: Overview\ntitle: Broken\n---\n# Broken\n")
	if err := os.Symlink("/nonexistent/elsewhere", filepath.Join(dir, "broken", "docs")); err != nil {
		t.Skipf("symlinks are unavailable here: %v", err)
	}
	return dir
}

// TestMultiSiteAggregatesSearchIndex covers the tree's one search index: it sits
// at the tree base, it holds the labelled entries of every wiki that rendered in
// visit order, every URL is root-relative under the right wiki, and a wiki that
// was skipped contributes nothing.
func TestMultiSiteAggregatesSearchIndex(t *testing.T) {
	dir := treeSearchFixture(t)

	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "okf-wiki"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	wikis, err := r.RenderSites(out)
	if err != nil {
		t.Fatal(err)
	}
	if wikis != 4 {
		t.Fatalf("rendered %d wikis, want 4 readable ones", wikis)
	}

	index := readSearchIndexWithContent(t, filepath.Join(out, "family"))
	if len(index.Entries) == 0 || len(index.Pages) == 0 {
		t.Fatal("aggregate search index is empty")
	}
	flat := flattenIndex(t, index)

	// Every entry names its wiki, and its URL is the root-relative page under
	// that wiki's own base.
	var order []string
	for _, e := range flat {
		if e.Site == "" || e.SiteKey == "" {
			t.Fatalf("aggregate entry is not labelled: %+v", e)
		}
		if !strings.HasPrefix(e.URL, "/family/"+e.SiteKey+"/") {
			t.Errorf("entry URL %q is not under its wiki base /family/%s/", e.URL, e.SiteKey)
		}
		if len(order) == 0 || order[len(order)-1] != e.SiteKey {
			order = append(order, e.SiteKey)
		}
	}

	// renderSites visits in sorted rel order, and the aggregate preserves it.
	wantOrder := []string{"alpha", "clients/example/operations", "clients/example/strategy", "homelab"}
	if strings.Join(order, ",") != strings.Join(wantOrder, ",") {
		t.Errorf("aggregate order = %v, want %v", order, wantOrder)
	}

	// A phrase that exists only in the operations wiki is found under that wiki.
	found := false
	for _, e := range flat {
		if !strings.Contains(e.Content, "exampleneedle") {
			continue
		}
		found = true
		if e.Site != "Operations" || e.SiteKey != "clients/example/operations" {
			t.Errorf("sibling hit labelled %q/%q, want Operations/clients/example/operations", e.Site, e.SiteKey)
		}
		if e.URL != "/family/clients/example/operations/index.html" {
			t.Errorf("sibling hit URL = %q, want /family/clients/example/operations/index.html", e.URL)
		}
	}
	if !found {
		t.Error("the operations wiki's distinctive phrase is missing from the aggregate index")
	}

	// The skipped wiki contributes nothing.
	for _, e := range flat {
		if e.SiteKey == "broken" || strings.Contains(e.URL, "/broken/") {
			t.Errorf("a skipped wiki is in the aggregate index: %+v", e)
		}
	}

	// Each wiki's own index is still written at its own base, unlabelled, so a
	// single wiki remains a site of its own.
	own := readSearchIndex(t, filepath.Join(out, "family", "alpha", "search-index.json"))
	if len(own.Pages) == 0 {
		t.Fatal("the wiki's own index is empty")
	}
	for _, p := range own.Pages {
		if p.Site != "" || p.SiteKey != "" {
			t.Errorf("the wiki's own index gained a site label: %+v", p)
		}
	}
}

// TestSingleBundleSearchIndexOmitsSiteFields pins the single-bundle shape: the
// page table holds only the page url and title, so no site field is written and
// the file's bytes are the pre-label shape. A page that contributed a chunk
// appears once in the table however many entries refer to it.
func TestSingleBundleSearchIndexOmitsSiteFields(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"),
		"---\ntitle: Home\n---\n# Home\n\nA stable sentence lives here.\n")
	mustMkdir(t, filepath.Join(dir, "guide"))
	mustWrite(t, filepath.Join(dir, "guide", "intro.md"),
		"---\ntitle: Intro\n---\n# Intro\n\n## Setup\n\nInstall steps.\n")

	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(out, "wiki", "search-index.json"))
	if err != nil {
		t.Fatal(err)
	}

	// Unmarshal into a shape without the site fields and re-marshal. If any new
	// key leaked into the file it would be dropped here and the two would differ.
	// The entry text is not part of the index: it travels in search-content.json.
	type barePage struct {
		URL   string `json:"u"`
		Title string `json:"t"`
	}
	type bareEntry struct {
		Page    int    `json:"g"`
		Anchor  string `json:"a"`
		Title   string `json:"t"`
		Section string `json:"s"`
	}
	type bareIndex struct {
		Pages   []barePage  `json:"pages"`
		Entries []bareEntry `json:"entries"`
	}
	var bare bareIndex
	if err := json.Unmarshal(raw, &bare); err != nil {
		t.Fatal(err)
	}
	again, err := json.Marshal(bare)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != string(raw) {
		t.Errorf("single-bundle index bytes changed:\n got: %s\nwant: %s", raw, again)
	}
	if strings.Contains(string(raw), `"w"`) || strings.Contains(string(raw), `"k"`) {
		t.Errorf("single-bundle index carries site fields: %s", raw)
	}
}

// TestSearchContentFileAlignsWithPerWikiIndex covers the companion file: it sits
// beside the light index, its count matches the entry count, and each text
// belongs to the entry at the same position.
func TestSearchContentFileAlignsWithPerWikiIndex(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"),
		"---\ntitle: Home\n---\n# Home\n\n## Alpha\n\nalphaonly phrase here.\n\n## Beta\n\nbetaonly phrase here.\n")

	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}

	site := filepath.Join(out, "wiki")
	index := readSearchIndex(t, filepath.Join(site, "search-index.json"))
	content := readSearchContent(t, filepath.Join(site, "search-content.json"))
	if content.Count != len(index.Entries) {
		t.Fatalf("search content n = %d, index has %d entries", content.Count, len(index.Entries))
	}
	if len(content.Texts) != len(index.Entries) {
		t.Fatalf("search content carries %d texts, index has %d entries", len(content.Texts), len(index.Entries))
	}
	if len(index.Entries) != 2 {
		t.Fatalf("the page contributed %d entries, want 2", len(index.Entries))
	}
	// Positional alignment: the text at each row is the text of the entry whose
	// heading is there.
	for i, e := range index.Entries {
		want := strings.ToLower(e.Title) + "only"
		if !strings.Contains(content.Texts[i], want) {
			t.Errorf("content[%d] for entry %q = %q, want it to carry %q", i, e.Title, content.Texts[i], want)
		}
	}
}

// TestSearchIndexOmitsContentKey covers the split: the light index is written
// without the entry text, and the text lives in the companion file. A "c" key
// left in the index would carry the bytes the split exists to defer.
func TestSearchIndexOmitsContentKey(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"),
		"---\ntitle: Home\n---\n# Home\n\n## Alpha\n\nalphaonly phrase here.\n")

	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	assertNoContentKey(t, filepath.Join(out, "wiki", "search-index.json"))

	// The same holds for a tree's aggregate at its base.
	tree := treeSearchFixture(t)
	tout := t.TempDir()
	tr, err := New(Config{Content: tree, Base: "/family/", Brand: Brand{Name: "t"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.RenderSites(tout); err != nil {
		t.Fatal(err)
	}
	assertNoContentKey(t, filepath.Join(tout, "family", "search-index.json"))
}

// assertNoContentKey fails when any entry in the index at path carries a c key.
func assertNoContentKey(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var probe struct {
		Entries []map[string]json.RawMessage `json:"entries"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	if len(probe.Entries) == 0 {
		t.Fatalf("%s has no entries to check", path)
	}
	for i, e := range probe.Entries {
		if _, ok := e["c"]; ok {
			t.Errorf("%s entry %d still carries a c key", path, i)
		}
	}
}

// TestSearchIndexPageTableDedup covers the page table: a page with two sections
// contributes one row, however many entries point at it, so the page's url and
// titles are stored once.
func TestSearchIndexPageTableDedup(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), strings.Join([]string{
		"---", "title: Home", "---", "# Home", "",
		"## First", "", "alpha content", "",
		"## Second", "", "beta content", "",
	}, "\n"))

	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	index := r.BuildSearchIndex()
	if len(index.Pages) != 1 {
		t.Fatalf("the page table has %d rows for one page, want 1", len(index.Pages))
	}
	if len(index.Entries) != 2 {
		t.Fatalf("the page contributed %d entries, want 2", len(index.Entries))
	}
	for i, e := range index.Entries {
		if e.Page != 0 {
			t.Errorf("entry %d refers to page %d, want 0", i, e.Page)
		}
	}
	if index.Pages[0].URL != "/wiki/index.html" || index.Pages[0].Title != "Home" {
		t.Errorf("page row = %+v, want the home page url and title", index.Pages[0])
	}
}

// TestAggregateSearchIndexRebasesPageTable covers the tree merge: two wikis'
// page tables concatenate into one, every entry's page index is rebased into
// the merged table, and each page carries the wiki it came from.
func TestAggregateSearchIndexRebasesPageTable(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "alpha"))
	mustWrite(t, filepath.Join(dir, "alpha", "index.md"),
		"---\ntype: Overview\ntitle: Alpha\n---\n# Alpha\n\n## First\n\nalphaneedle here.\n\n## Second\n\nalpha lore.\n")
	mustMkdir(t, filepath.Join(dir, "beta"))
	mustWrite(t, filepath.Join(dir, "beta", "index.md"),
		"---\ntype: Overview\ntitle: Beta\n---\n# Beta\n\n## First\n\nbetaneedle here.\n\n## Second\n\nbeta lore.\n")

	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "t"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(out); err != nil {
		t.Fatal(err)
	}

	index := readSearchIndexWithContent(t, filepath.Join(out, "family"))
	if len(index.Pages) != 2 {
		t.Fatalf("aggregate page table has %d rows, want 2", len(index.Pages))
	}
	if len(index.Entries) != 4 {
		t.Fatalf("aggregate has %d entries, want 4", len(index.Entries))
	}
	// The page table is the concatenation of the wikis' tables in visit order.
	if index.Pages[0].SiteKey != "alpha" || index.Pages[1].SiteKey != "beta" {
		t.Fatalf("page table order = %q, %q, want alpha then beta", index.Pages[0].SiteKey, index.Pages[1].SiteKey)
	}
	for _, p := range index.Pages {
		if p.Site != "Alpha" && p.Site != "Beta" {
			t.Errorf("aggregate page has the wrong site title: %+v", p)
		}
		if !strings.Contains(p.URL, "/"+p.SiteKey+"/") {
			t.Errorf("page url %q does not sit under its wiki %q", p.URL, p.SiteKey)
		}
	}
	// A merge that forgot to rebase would leave beta's entries pointing at
	// alpha's row, so each entry's page must name the wiki its content came from.
	for i, e := range index.Entries {
		if e.Page < 0 || e.Page >= len(index.Pages) {
			t.Fatalf("entry %d points outside the page table: %d", i, e.Page)
		}
		want := "alpha"
		if strings.Contains(e.Content, "beta") {
			want = "beta"
		}
		if got := index.Pages[e.Page].SiteKey; got != want {
			t.Errorf("entry %d (%q) points at page %d of %q, want %q", i, e.Content, e.Page, got, want)
		}
	}
}

// TestAggregateSearchContentAlignsAcrossWikis covers the tree's companion file:
// the texts of every wiki's entries are concatenated in the aggregate's entry
// order, so the text at each position belongs to that entry's wiki.
func TestAggregateSearchContentAlignsAcrossWikis(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "alpha"))
	mustWrite(t, filepath.Join(dir, "alpha", "index.md"),
		"---\ntype: Overview\ntitle: Alpha\n---\n# Alpha\n\n## First\n\nalphaneedle here.\n")
	mustMkdir(t, filepath.Join(dir, "beta"))
	mustWrite(t, filepath.Join(dir, "beta", "index.md"),
		"---\ntype: Overview\ntitle: Beta\n---\n# Beta\n\n## First\n\nbetaneedle here.\n")

	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "t"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(out); err != nil {
		t.Fatal(err)
	}

	base := filepath.Join(out, "family")
	index := readSearchIndex(t, filepath.Join(base, "search-index.json"))
	content := readSearchContent(t, filepath.Join(base, "search-content.json"))
	if content.Count != len(index.Entries) || len(content.Texts) != len(index.Entries) {
		t.Fatalf("aggregate content has %d texts (n=%d) for %d entries",
			len(content.Texts), content.Count, len(index.Entries))
	}
	for i, e := range index.Entries {
		site := index.Pages[e.Page].SiteKey
		if !strings.Contains(content.Texts[i], site+"needle") {
			t.Errorf("content[%d] = %q, want the text of entry %d under %s", i, content.Texts[i], i, site)
		}
	}
}

// TestSearchIndexBoundsLongChunk covers the chunk bound: a section longer than
// the cap is indexed up to a word boundary and no further, so no word is cut
// in half and an outlier section cannot dominate the index.
func TestSearchIndexBoundsLongChunk(t *testing.T) {
	word := "lorem"
	var b strings.Builder
	for b.Len() < maxSearchChunkBytes*2 {
		b.WriteString(word)
		b.WriteByte(' ')
	}
	body := b.String()

	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"),
		"---\ntitle: Home\n---\n# Home\n\n## Big\n\n"+body+"\n")

	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	index := r.BuildSearchIndex()
	if len(index.Entries) != 1 {
		t.Fatalf("the page contributed %d entries, want 1", len(index.Entries))
	}
	got := index.Entries[0].Content
	if len(got) > maxSearchChunkBytes {
		t.Fatalf("chunk is %d bytes, over the %d cap", len(got), maxSearchChunkBytes)
	}
	if !strings.HasPrefix(body, got) {
		t.Error("the bounded chunk is not a prefix of the original text")
	}
	// The cut is a whole-word one: the original's next character after the cut
	// is a space, so the chunk stops between words rather than inside one.
	if next := body[len(got)]; next != ' ' {
		t.Errorf("the cut landed mid-word: chunk ends %q, next byte %q", got[len(got)-8:], next)
	}
}

// TestSearchIndexLongChunkShortStaysWhole is the other side of the bound: text
// at or under the cap is indexed in full, so the common chunk is untouched.
func TestSearchIndexLongChunkShortStaysWhole(t *testing.T) {
	text := strings.Repeat("short clause ", 10)
	if got := boundSearchText(text); got != text {
		t.Errorf("a chunk under the cap was altered: %q", got)
	}
}

// TestSearchIndexBoundsLongChunkWithEarlyInvalidByte covers a chunk whose
// source carries a stray non-UTF-8 byte before the cut. Only the rune at the
// cut may be partial, so the cut has to step back over that rune alone; a
// validity check over the whole prefix would throw the section away from the
// first invalid byte onward and index nothing.
func TestSearchIndexBoundsLongChunkWithEarlyInvalidByte(t *testing.T) {
	body := "\xff" + strings.Repeat("lorem ", maxSearchChunkBytes)
	got := boundSearchText(body)
	if got == "" {
		t.Fatal("a long section with one early invalid byte was reduced to nothing")
	}
	if len(got) > maxSearchChunkBytes {
		t.Errorf("chunk is %d bytes, over the %d cap", len(got), maxSearchChunkBytes)
	}
	if !strings.HasPrefix(body, got) {
		t.Error("the bounded chunk is not a prefix of the original text")
	}
	if !strings.Contains(got, "lorem") {
		t.Errorf("the chunk dropped the text around the invalid byte: %q", got[:min(len(got), 40)])
	}
}

// TestSearchIndexBoundsLongChunkRuneBoundary covers a long run with no spaces
// to cut at. The cut then lands on a rune boundary rather than a word one, and
// the indexed text must still be valid UTF-8.
func TestSearchIndexBoundsLongChunkRuneBoundary(t *testing.T) {
	// One ASCII byte ahead of the runes puts the byte cap inside a multi-byte
	// rune, which is the partial rune the cut has to step back over.
	body := "x" + strings.Repeat("\u6f22", maxSearchChunkBytes)
	got := boundSearchText(body)
	if len(got) > maxSearchChunkBytes {
		t.Errorf("chunk is %d bytes, over the %d cap", len(got), maxSearchChunkBytes)
	}
	if !utf8.ValidString(got) {
		t.Errorf("the rune-boundary cut left invalid UTF-8 at %q", got[len(got)-8:])
	}
	if !strings.HasPrefix(body, got) {
		t.Error("the bounded chunk is not a prefix of the original text")
	}
	if r, size := utf8.DecodeLastRuneInString(got); r == utf8.RuneError && size <= 1 {
		t.Error("the chunk ends in a partial rune")
	}
}

// TestMultiSiteZeroWikisWritesEmptyAggregate covers a tree whose directories
// hold no wiki at all: the tree base still gets an empty catalogue, so a group
// page's search control answers instead of fetching a 404.
func TestMultiSiteZeroWikisWritesEmptyAggregate(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "empty"))
	mustWrite(t, filepath.Join(dir, "empty", ".meta.json"), `{"title": "Empty"}`)

	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "t"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	wikis, err := r.RenderSites(out)
	if err != nil {
		t.Fatal(err)
	}
	if wikis != 0 {
		t.Fatalf("rendered %d wikis, want 0", wikis)
	}
	raw, err := os.ReadFile(filepath.Join(out, "family", "search-index.json"))
	if err != nil {
		t.Fatalf("the tree base has no catalogue: %v", err)
	}
	if strings.Contains(string(raw), "null") {
		t.Errorf("empty tree catalogue has a null table: %s", raw)
	}
	var index SearchIndex
	if err := json.Unmarshal(raw, &index); err != nil {
		t.Fatalf("empty tree catalogue is not a search index: %v", err)
	}
	if len(index.Pages) != 0 || len(index.Entries) != 0 {
		t.Errorf("empty tree catalogue = %s, want empty pages and entries", raw)
	}
}

// TestMultiSiteRootWikiKeepsItsOwnIndex covers a tree whose content root is
// itself a wiki: scanSites stops there, so it is a tree of one whose base equals
// the wiki's own base. The wiki's own index stays put rather than being replaced
// by a labelled aggregate, and the base serves the wiki's home page, not a
// redirect, so its search reads as a single bundle's.
func TestMultiSiteRootWikiKeepsItsOwnIndex(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"),
		"---\ntype: Overview\ntitle: Home\n---\n# Home\n\nrootwikitoken lives here.\n")

	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "okf-wiki"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	wikis, err := r.RenderSites(out)
	if err != nil {
		t.Fatal(err)
	}
	if wikis != 1 {
		t.Fatalf("rendered %d wikis, want 1", wikis)
	}
	base := filepath.Join(out, "family")

	// The base index is the wiki's own: every page is unlabelled, so the client
	// renders the flat list with no per-wiki headings.
	own := readSearchIndex(t, filepath.Join(base, "search-index.json"))
	if len(own.Pages) == 0 || len(own.Entries) == 0 {
		t.Fatal("the root wiki's own index is empty")
	}
	for _, p := range own.Pages {
		if p.Site != "" || p.SiteKey != "" {
			t.Errorf("the root wiki's own index gained a site label: %+v", p)
		}
	}

	// The base is the wiki's home page, and the only index.html there: an
	// aggregate or a root redirect at the base would be a second one.
	home, err := os.ReadFile(filepath.Join(base, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(home), "<h1>Home</h1>") {
		t.Error("the base index is not the wiki's home page")
	}
	if strings.Contains(string(home), `http-equiv="refresh"`) {
		t.Error("the base index is a redirect rather than the wiki's home page")
	}
	if !strings.Contains(string(home), `window.__WIKI_SEARCH = '\/family\/'`) {
		t.Error("the root wiki does not search its own base")
	}
	dirEntries, err := os.ReadDir(base)
	if err != nil {
		t.Fatal(err)
	}
	var indexFiles int
	for _, e := range dirEntries {
		if !e.IsDir() && e.Name() == "index.html" {
			indexFiles++
		}
	}
	if indexFiles != 1 {
		t.Errorf("the base holds %d index.html files, want 1", indexFiles)
	}
}

// TestSearchGlobalsPerRenderKind covers the difference the client keys on: a
// tree wiki points its search at the tree base, while its own base stays the
// wiki's, and a group index and a single bundle point search at their own base.
func TestSearchGlobalsPerRenderKind(t *testing.T) {
	dir := treeSearchFixture(t)

	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "okf-wiki"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(out); err != nil {
		t.Fatal(err)
	}

	// A nested wiki: its assets and links follow its own base, but the search
	// index it fetches is the tree's.
	page, err := os.ReadFile(filepath.Join(out, "family", "clients", "example", "operations", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(page)
	if !strings.Contains(html, `window.__WIKI_BASE = '\/family\/clients\/example\/operations\/'`) {
		t.Errorf("tree wiki does not carry its own base:\n%s", html)
	}
	if !strings.Contains(html, `window.__WIKI_SEARCH = '\/family\/'`) {
		t.Errorf("tree wiki does not point search at the tree base:\n%s", html)
	}

	// A group index is rendered with the tree renderer, so both are the tree.
	group, err := os.ReadFile(filepath.Join(out, "family", "clients", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(group), `window.__WIKI_BASE = '\/family\/'`) ||
		!strings.Contains(string(group), `window.__WIKI_SEARCH = '\/family\/'`) {
		t.Errorf("group index bases are not both the tree base:\n%s", group)
	}

	// A single bundle searches its own base.
	single := t.TempDir()
	mustWrite(t, filepath.Join(single, "index.md"), "---\ntitle: Home\n---\n# Home\n")
	sout := t.TempDir()
	sr, err := New(Config{Content: single, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := sr.RenderAll(sout); err != nil {
		t.Fatal(err)
	}
	singleHTML, err := os.ReadFile(filepath.Join(sout, "wiki", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(singleHTML), `window.__WIKI_BASE = '\/wiki\/'`) ||
		!strings.Contains(string(singleHTML), `window.__WIKI_SEARCH = '\/wiki\/'`) {
		t.Errorf("single bundle bases are not both its base:\n%s", singleHTML)
	}
}

// breadcrumbHTMLOf extracts the breadcrumb nav's inner HTML from a rendered
// page, so a test can pin the trail without matching a link elsewhere.
func breadcrumbHTMLOf(t *testing.T, html string) string {
	t.Helper()
	const open = `<nav class="breadcrumb" aria-label="Breadcrumb">`
	i := strings.Index(html, open)
	if i < 0 {
		t.Fatalf("no breadcrumb nav in:\n%s", html)
	}
	rest := html[i+len(open):]
	j := strings.Index(rest, `</nav>`)
	if j < 0 {
		t.Fatalf("unterminated breadcrumb nav in:\n%s", html)
	}
	return rest[:j]
}

// multiSiteHeaderFixture writes a tree with two nested groups above one wiki
// and a section page inside it, plus a second wiki at the top level. It is the
// shape the header and breadcrumb tests share.
func multiSiteHeaderFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "clients"))
	mustWrite(t, filepath.Join(dir, "clients", ".meta.json"),
		`{"title": "Clients", "description": "Engagement wikis."}`)
	mustMkdir(t, filepath.Join(dir, "clients", "example"))
	mustWrite(t, filepath.Join(dir, "clients", "example", ".meta.json"),
		`{"title": "Example", "description": "The Example engagement."}`)
	mustMkdir(t, filepath.Join(dir, "clients", "example", "operations", "guides"))
	mustWrite(t, filepath.Join(dir, "clients", "example", "operations", "index.md"),
		"---\ntype: Overview\ntitle: Operations\n---\n# Operations\n")
	mustWrite(t, filepath.Join(dir, "clients", "example", "operations", "guides", "setup.md"),
		"---\ntype: Guide\ntitle: Setup\n---\n# Setup\n")
	mustMkdir(t, filepath.Join(dir, "homelab"))
	mustWrite(t, filepath.Join(dir, "homelab", "index.md"),
		"---\ntype: Overview\ntitle: Homelab\n---\n# Homelab\n")
	return dir
}

// TestMultiSiteBrandPointsAtTree covers the brand of a page inside a tree: the
// anchor targets the tree base so the mark and the wordmark are the deployment
// home from any depth, while the page's assets and preload keep hanging off its
// own wiki base.
func TestMultiSiteBrandPointsAtTree(t *testing.T) {
	dir := multiSiteHeaderFixture(t)

	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "okf-wiki", Title: "Wiki"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(out); err != nil {
		t.Fatal(err)
	}

	page, err := os.ReadFile(filepath.Join(out, "family", "clients", "example", "operations", "guides", "setup.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(page)
	if !strings.Contains(html, `<a class="brand" href="/family/">`) {
		t.Errorf("the brand does not point at the tree base:\n%s", html)
	}
	if strings.Contains(html, `<a class="brand" href="/family/clients/example/operations/">`) {
		t.Errorf("the brand still points at the wiki's own base:\n%s", html)
	}
	// The wordmark link is the only place HomeHref may change: every asset path
	// stays on the wiki's own base.
	if !strings.Contains(html, `href="/family/clients/example/operations/wiki.css`) ||
		!strings.Contains(html, `href="/family/clients/example/operations/brand-mark.svg`) {
		t.Errorf("an asset stopped using the wiki's own base:\n%s", html)
	}
}

// TestMultiSiteBreadcrumbTrail covers the trail on a deep wiki page: the tree
// title, each folder in order, the wiki title, then the section and the page,
// with each link pointing at the right index and the current page last.
func TestMultiSiteBreadcrumbTrail(t *testing.T) {
	dir := multiSiteHeaderFixture(t)

	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "okf-wiki", Title: "Wiki"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(out); err != nil {
		t.Fatal(err)
	}

	page, err := os.ReadFile(filepath.Join(out, "family", "clients", "example", "operations", "guides", "setup.html"))
	if err != nil {
		t.Fatal(err)
	}
	got := breadcrumbHTMLOf(t, string(page))
	want := `<a href="/family/">Wiki</a><span class="crumb-sep">/</span>` +
		`<a href="/family/clients/">Clients</a><span class="crumb-sep">/</span>` +
		`<a href="/family/clients/example/">Example</a><span class="crumb-sep">/</span>` +
		`<a href="/family/clients/example/operations/">Operations</a><span class="crumb-sep">/</span>` +
		`<a href="/family/clients/example/operations/guides/index.html">Guides</a><span class="crumb-sep">/</span>` +
		`<span class="crumb-cur">Setup</span>`
	if got != want {
		t.Errorf("breadcrumb trail =\n%s\nwant\n%s", got, want)
	}
}

// TestMultiSiteBreadcrumbWikiHome covers a wiki's own home page: the wiki crumb
// is the current one rather than a link to the page the reader is on, so the
// trail ends at the wiki and does not repeat its title.
func TestMultiSiteBreadcrumbWikiHome(t *testing.T) {
	dir := multiSiteHeaderFixture(t)

	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "okf-wiki", Title: "Wiki"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(out); err != nil {
		t.Fatal(err)
	}

	page, err := os.ReadFile(filepath.Join(out, "family", "clients", "example", "operations", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	got := breadcrumbHTMLOf(t, string(page))
	want := `<a href="/family/">Wiki</a><span class="crumb-sep">/</span>` +
		`<a href="/family/clients/">Clients</a><span class="crumb-sep">/</span>` +
		`<a href="/family/clients/example/">Example</a><span class="crumb-sep">/</span>` +
		`<span class="crumb-cur">Operations</span>`
	if got != want {
		t.Errorf("wiki home trail =\n%s\nwant\n%s", got, want)
	}
}

// TestMultiSiteTagPageBreadcrumb covers a tag page inside a tree: the page
// belongs to its wiki, so it keeps the trail down to that wiki and then appends
// Tags / #tag. The wiki title links to the wiki home, Tags stays unclassed text
// since there is no tag index page, and the tag is the current crumb with its
// hash muted, the same shape every other page of the tree shows.
func TestMultiSiteTagPageBreadcrumb(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "clients"))
	mustWrite(t, filepath.Join(dir, "clients", ".meta.json"),
		`{"title": "Clients", "description": "Engagement wikis."}`)
	mustMkdir(t, filepath.Join(dir, "clients", "example"))
	mustWrite(t, filepath.Join(dir, "clients", "example", ".meta.json"),
		`{"title": "Example", "description": "The Example engagement."}`)
	mustMkdir(t, filepath.Join(dir, "clients", "example", "operations"))
	mustWrite(t, filepath.Join(dir, "clients", "example", "operations", "index.md"),
		"---\ntype: Overview\ntitle: Operations\ntags: [beta]\n---\n# Operations\n")

	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "okf-wiki", Title: "Wiki"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(out); err != nil {
		t.Fatal(err)
	}

	page, err := os.ReadFile(filepath.Join(out, "family", "clients", "example", "operations", "tags", "beta.html"))
	if err != nil {
		t.Fatal(err)
	}
	got := breadcrumbHTMLOf(t, string(page))
	want := `<a href="/family/">Wiki</a><span class="crumb-sep">/</span>` +
		`<a href="/family/clients/">Clients</a><span class="crumb-sep">/</span>` +
		`<a href="/family/clients/example/">Example</a><span class="crumb-sep">/</span>` +
		`<a href="/family/clients/example/operations/">Operations</a><span class="crumb-sep">/</span>` +
		`<span>Tags</span><span class="crumb-sep">/</span>` +
		`<span class="crumb-cur"><span class="tag-hash">#</span>beta</span>`
	if got != want {
		t.Errorf("tag page trail =\n%s\nwant\n%s", got, want)
	}
	if strings.Contains(got, `href="/family/clients/example/operations/tags/beta.html"`) {
		t.Errorf("the tag page links to itself:\n%s", got)
	}
}

// TestMultiSiteRootWikiTagPageBreadcrumb covers a tag page in a tree whose
// content root is itself a wiki: the tree title links to the tree base, which is
// the wiki's own home, and then Tags / #tag follows, with no duplicated wiki
// title above itself.
func TestMultiSiteRootWikiTagPageBreadcrumb(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"),
		"---\ntype: Overview\ntitle: Home\ntags: [alpha]\n---\n# Home\n")

	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "okf-wiki", Title: "Wiki"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(out); err != nil {
		t.Fatal(err)
	}

	page, err := os.ReadFile(filepath.Join(out, "family", "tags", "alpha.html"))
	if err != nil {
		t.Fatal(err)
	}
	got := breadcrumbHTMLOf(t, string(page))
	want := `<a href="/family/">Wiki</a><span class="crumb-sep">/</span>` +
		`<span>Tags</span><span class="crumb-sep">/</span>` +
		`<span class="crumb-cur"><span class="tag-hash">#</span>alpha</span>`
	if got != want {
		t.Errorf("root wiki tag trail =\n%s\nwant\n%s", got, want)
	}
	if n := strings.Count(got, "Wiki"); n != 1 {
		t.Errorf("the trail names Wiki %d times, want 1:\n%s", n, got)
	}
}

// TestMultiSiteGroupIndexBreadcrumb covers a group index: the trail runs from
// the tree root to the group, whose own name is current rather than a link to
// the page the reader is on, and is not duplicated.
func TestMultiSiteGroupIndexBreadcrumb(t *testing.T) {
	dir := multiSiteHeaderFixture(t)

	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "okf-wiki", Title: "Wiki"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(out); err != nil {
		t.Fatal(err)
	}

	page, err := os.ReadFile(filepath.Join(out, "family", "clients", "example", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	got := breadcrumbHTMLOf(t, string(page))
	want := `<a href="/family/">Wiki</a><span class="crumb-sep">/</span>` +
		`<a href="/family/clients/">Clients</a><span class="crumb-sep">/</span>` +
		`<span class="crumb-cur">Example</span>`
	if got != want {
		t.Errorf("group index trail =\n%s\nwant\n%s", got, want)
	}
	if strings.Contains(got, `href="/family/clients/example/">`) {
		t.Errorf("the group index links to itself:\n%s", got)
	}
	if n := strings.Count(got, "Example"); n != 1 {
		t.Errorf("the group index trail names Example %d times, want 1:\n%s", n, got)
	}
}

// TestMultiSiteRootGroupBreadcrumb covers the tree root group index: the tree
// title shows once as the current crumb, with no link to itself and no repeat.
func TestMultiSiteRootGroupBreadcrumb(t *testing.T) {
	dir := multiSiteHeaderFixture(t)

	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "okf-wiki", Title: "Wiki"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(out); err != nil {
		t.Fatal(err)
	}

	page, err := os.ReadFile(filepath.Join(out, "family", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	got := breadcrumbHTMLOf(t, string(page))
	if got != `<span class="crumb-cur">Wiki</span>` {
		t.Errorf("root group trail = %q, want a single current Wiki crumb", got)
	}
	if strings.Contains(got, `href="/family/">`) {
		t.Errorf("the root group links to itself:\n%s", got)
	}
	if n := strings.Count(got, "Wiki"); n != 1 {
		t.Errorf("the root group trail names Wiki %d times, want 1:\n%s", n, got)
	}
}

// TestMultiSiteRootWikiHeaderAndTrail covers a tree whose content root is
// itself a wiki: the brand reaches the tree base, which is the wiki's own home,
// and the trail shows the tree title once rather than duplicating the wiki's
// own title above itself.
func TestMultiSiteRootWikiHeaderAndTrail(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "guide"))
	mustWrite(t, filepath.Join(dir, "index.md"),
		"---\ntype: Overview\ntitle: Home\n---\n# Home\n")
	mustWrite(t, filepath.Join(dir, "guide", "setup.md"),
		"---\ntype: Guide\ntitle: Setup\n---\n# Setup\n")

	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "okf-wiki", Title: "Wiki"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(out); err != nil {
		t.Fatal(err)
	}

	home, err := os.ReadFile(filepath.Join(out, "family", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(home), `<a class="brand" href="/family/">`) {
		t.Errorf("the root wiki's brand does not reach the tree base:\n%s", home)
	}
	if got := breadcrumbHTMLOf(t, string(home)); got != `<span class="crumb-cur">Wiki</span>` {
		t.Errorf("root wiki home trail = %q, want a single current Wiki crumb", got)
	} else if strings.Contains(got, "Home") {
		t.Errorf("root wiki home trail duplicates the wiki title: %s", got)
	}

	deep, err := os.ReadFile(filepath.Join(out, "family", "guide", "setup.html"))
	if err != nil {
		t.Fatal(err)
	}
	got := breadcrumbHTMLOf(t, string(deep))
	want := `<a href="/family/">Wiki</a><span class="crumb-sep">/</span>` +
		`<a href="/family/guide/index.html">Guide</a><span class="crumb-sep">/</span>` +
		`<span class="crumb-cur">Setup</span>`
	if got != want {
		t.Errorf("root wiki deep trail =\n%s\nwant\n%s", got, want)
	}
}

// TestSingleBundleHeaderUnchanged pins a single bundle's header: the brand and
// the breadcrumb are exactly what they were before a tree existed, with no tree
// title and no folder crumbs.
func TestSingleBundleHeaderUnchanged(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "guide"))
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\n---\n# Home\n")
	mustWrite(t, filepath.Join(dir, "guide", "setup.md"),
		"---\ntype: Guide\ntitle: Setup\n---\n# Setup\n")

	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t", Title: "Wiki"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}

	page, err := os.ReadFile(filepath.Join(out, "wiki", "guide", "setup.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(page)
	if !strings.Contains(html, `<a class="brand" href="/wiki/">`) {
		t.Errorf("single bundle brand is not its own base:\n%s", html)
	}
	got := breadcrumbHTMLOf(t, html)
	want := `<a href="/wiki/">Docs</a><span class="crumb-sep">/</span>` +
		`<a href="/wiki/guide/index.html">Guide</a><span class="crumb-sep">/</span>` +
		`<span class="crumb-cur">Setup</span>`
	if got != want {
		t.Errorf("single bundle trail =\n%s\nwant\n%s", got, want)
	}
}
