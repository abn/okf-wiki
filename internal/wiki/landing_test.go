package wiki

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A bundle root holding landing.md introduces the deployment at the output
// root instead of redirecting to the wiki. The wiki itself renders unchanged
// beside it, and the landing source is never published as a page.
func TestRenderLandingReplacesRootRedirect(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\ndescription: The wiki home.\n---\n\n# Home\n")
	mustWrite(t, filepath.Join(dir, "landing.md"), "---\ntitle: Example\ndescription: A friendlier front end.\n---\n\n# Example\n\nWelcome to the deployment.\n")

	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t", Title: "T"}})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}

	root, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(root), landingMarker) {
		t.Error("output root index has no landing marker, so serve would redirect it to the wiki")
	}
	if strings.Contains(string(root), "http-equiv=\"refresh\"") {
		t.Error("output root index still redirects to the wiki")
	}
	if !strings.Contains(string(root), "Welcome to the deployment.") {
		t.Error("output root index does not carry the landing body")
	}
	if !strings.Contains(string(root), "content-landing") {
		t.Error("landing page does not take the wide content class")
	}
	if _, err := os.Stat(filepath.Join(out, "wiki", "landing.html")); !os.IsNotExist(err) {
		t.Error("landing.md was published as a wiki page")
	}
	for _, s := range r.Sections() {
		for _, p := range s.Pages {
			if strings.Contains(p.Slug, "landing") {
				t.Errorf("landing page listed as %q in the sections", p.Slug)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(out, "wiki", "index.html")); err != nil {
		t.Errorf("wiki home was not rendered beside the landing: %v", err)
	}
}

// Without a landing page the output root keeps redirecting to the wiki base,
// byte for byte the old behavior.
func TestRenderWithoutLandingKeepsRedirect(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "# Home\n")

	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t", Title: "T"}})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	root, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(root), landingMarker) {
		t.Error("output root index carries a landing marker with no landing.md")
	}
	if !strings.Contains(string(root), "http-equiv=\"refresh\"") {
		t.Error("output root index no longer redirects to the wiki")
	}
}

// A base of "/" leaves no room for both a wiki home and a landing page at the
// output root. The render names the clash rather than replacing the home.
func TestLandingWithRootBaseFails(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "# Home\n")
	mustWrite(t, filepath.Join(dir, "landing.md"), "# Example\n")

	r, err := New(Config{Content: dir, Base: "/", Brand: Brand{Name: "t", Title: "T"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RenderAll(t.TempDir()); err == nil || !strings.Contains(err.Error(), "landing") {
		t.Errorf("render with base / and landing.md = %v, want an error naming the landing", err)
	}
}

// A group holding landing.md shows the authored body above its generated
// catalog, so the group index reads as one page rather than two.
func TestGroupLandingPrependsCatalog(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "landing.md"), "---\ntitle: Clients\ndescription: Client work.\n---\n\n# Clients\n\nStart here.\n")
	mustWrite(t, filepath.Join(dir, "hero.png"), "fake-png")
	mustMkdir(t, filepath.Join(dir, "ops"))
	mustWrite(t, filepath.Join(dir, "ops", "index.md"), "---\ntitle: Operations\ndescription: Runbooks.\n---\n\n# Operations\n")

	r, err := New(Config{Content: dir, Base: "/", Brand: Brand{Name: "t", Title: "T"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if _, err := r.RenderSites(out); err != nil {
		t.Fatal(err)
	}
	index, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	hero := strings.Index(string(index), "Start here.")
	cards := strings.Index(string(index), "site-list")
	if hero == -1 {
		t.Fatal("group index does not carry the landing body")
	}
	if cards == -1 {
		t.Fatal("group index lost its generated catalog")
	}
	if hero > cards {
		t.Error("landing body renders below the catalog; it must introduce the group first")
	}
	if _, err := os.Stat(filepath.Join(out, "hero.png")); err != nil {
		t.Errorf("group landing asset did not travel with the catalog: %v", err)
	}
}

// Inside a wiki directory the landing name stays reserved and inert: no hero
// is prepended anywhere, no page is published, and the tree root keeps its
// redirect.
func TestLandingInsideAWikiDirStaysInert(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "ops"))
	mustWrite(t, filepath.Join(dir, "ops", "index.md"), "---\ntitle: Operations\n---\n\n# Operations\n")
	mustWrite(t, filepath.Join(dir, "ops", "landing.md"), "# Misplaced\n\nNowhere.\n")

	r, err := New(Config{Content: dir, Base: "/", Brand: Brand{Name: "t", Title: "T"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if _, err := r.RenderSites(out); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"index.html", filepath.Join("ops", "index.html")} {
		b, err := os.ReadFile(filepath.Join(out, f))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "Nowhere.") {
			t.Errorf("%s carries an inert landing body", f)
		}
		if strings.Contains(string(b), landingMarker) {
			t.Errorf("%s carries a landing marker", f)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "ops", "landing.html")); !os.IsNotExist(err) {
		t.Error("landing.md inside a wiki dir was published as a page")
	}
}

// The introduction sells the deployment; it is not wiki content, so the
// search catalogue leaves it out.
func TestLandingExcludedFromSearchIndex(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "# Home\n")
	mustWrite(t, filepath.Join(dir, "landing.md"), "# Example\n\nZanzibarConsulting.\n")

	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t", Title: "T"}})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	index, err := os.ReadFile(filepath.Join(out, "wiki", "search-index.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(index), "ZanzibarConsulting") {
		t.Error("search index carries the landing body")
	}
	if _, err := os.Stat(filepath.Join(out, "wiki", "landing.md")); !os.IsNotExist(err) {
		t.Error("landing source is copied into the site with no page to download it from")
	}
}

// The landing stylesheet ships with every render, since group catalogs reuse
// its blocks for their heroes.
func TestLandingCSSShipped(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "# Home\n")

	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t", Title: "T"}})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "wiki", "landing.css")); err != nil {
		t.Errorf("landing.css not in the output: %v", err)
	}
}
func TestServeRootServesLandingOrRedirects(t *testing.T) {
	landing := t.TempDir()
	mustWrite(t, filepath.Join(landing, "index.html"), "<html>hi"+landingMarker+"</html>")
	h := handler(ServeOptions{OutDir: landing, Base: "/wiki/"})
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET / with a landing = %d, want 200", w.Code)
	}

	plain := t.TempDir()
	mustWrite(t, filepath.Join(plain, "index.html"), "<html>redirect</html>")
	h = handler(ServeOptions{OutDir: plain, Base: "/wiki/"})
	req = httptest.NewRequest("GET", "/", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusFound {
		t.Errorf("GET / without a landing = %d, want 302", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/wiki/" {
		t.Errorf("GET / without a landing redirects to %q, want the wiki base", loc)
	}
}
