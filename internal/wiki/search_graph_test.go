package wiki

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestSearchIndexCarriesGraphIDs pins the join the search modal uses to open a
// hit in the graph. A tree namespaces both sides the same way.
func TestSearchIndexCarriesGraphIDs(t *testing.T) {
	dir := t.TempDir()
	for _, wiki := range []string{"alpha", "beta"} {
		mustMkdir(t, filepath.Join(dir, wiki))
		mustWrite(t, filepath.Join(dir, wiki, "index.md"),
			"---\\ntitle: "+wiki+"\\n---\\n# "+wiki+"\\n")
	}
	out := t.TempDir()
	r, err := New(Config{Content: dir, Base: "/family/", Brand: Brand{Name: "t"}, MultiSite: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RenderSites(out); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(out, "family", "search-index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var index SearchIndex
	if err := json.Unmarshal(raw, &index); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"alpha:page:index", "beta:page:index"} {
		found := false
		for _, page := range index.Pages {
			if page.ID == id {
				found = true
			}
		}
		if !found {
			t.Errorf("search index has no page ID %q", id)
		}
	}
}
