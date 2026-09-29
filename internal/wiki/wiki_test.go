package wiki

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		{"internal sibling", withRepo, "usage/cli", "../design/index.md", "/wiki/design/index.html"},
		{"internal with fragment", withRepo, "usage/cli", "../design/index.md#x", "/wiki/design/index.html#x"},
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
	out := r.RenderPage(Page{Slug: "index", Title: "Hello"})
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

// docsDir returns this project's own docs bundle, skipping when it is absent.
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
	if !isOKFBundle(dir) {
		// empty dir is not a bundle
	} else {
		t.Fatalf("empty dir should not be an OKF bundle")
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

// TestRenderBundledDocs renders this project's own docs/ bundle and asserts the
// core outputs exist and are well formed.
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
