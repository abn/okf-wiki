package wiki

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

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
	raw, err := os.ReadFile(filepath.Join(site, "search-index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var index []SearchEntry
	if err := json.Unmarshal(raw, &index); err != nil {
		t.Fatal(err)
	}
	if len(index) == 0 {
		t.Fatal("search index is empty under a relocated base")
	}
	for _, e := range index {
		if !strings.HasPrefix(e.URL, "/docs/deep/") {
			t.Fatalf("search entry URL does not follow the base: %q", e.URL)
		}
		if !strings.HasPrefix(e.PageURL, "/docs/deep/") {
			t.Fatalf("search entry PageURL does not follow the base: %q", e.PageURL)
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

	raw, err := os.ReadFile(filepath.Join(out, "wiki", "search-index.json"))
	if err != nil {
		t.Fatalf("read search index: %v", err)
	}
	var index []SearchEntry
	if err := json.Unmarshal(raw, &index); err != nil {
		t.Fatalf("unmarshal search index: %v", err)
	}
	if len(index) < 10 {
		t.Fatalf("search index has only %d entries", len(index))
	}
	for _, e := range index {
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
	for _, e := range r.BuildSearchIndex() {
		if strings.Contains(e.PageURL, "guide/setup/install.html") {
			found = true
		}
	}
	if !found {
		t.Error("the nested install page is missing from the search index")
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
	// The toolbar carries a type label and two glyph buttons, no text.
	for _, want := range []string{"mermaid-toolbar", "mermaid-type", "mermaid-copy", "mermaid-expand"} {
		if !strings.Contains(client, want) {
			t.Errorf("diagrams.js does not contain %q", want)
		}
	}
	// The lightbox must not shrink a wide diagram below its natural size.
	if !strings.Contains(client, "fit >= 1 ? Math.min(fit, 1.25) : 1") {
		t.Error("lightbox fit can scale a wide diagram below natural size")
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
	// The failure card must read as a card, not a stray block.
	if !strings.Contains(sheet, "var(--status-danger-tint)") {
		t.Error("failure card carries no danger tint")
	}
}
