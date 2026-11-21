package wiki

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestSearchTextSkipsDiagramAndChartSource covers a snippet that showed raw
// Mermaid source. Diagram and chart fences are drawn rather than read, so their
// source stays out of the indexed text, while an ordinary code fence and the
// prose around both stay in.
func TestSearchTextSkipsDiagramAndChartSource(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\n---\n# Home\n\n"+
		"Rendering is a single pass.\n\n"+
		"```mermaid\nflowchart LR\n  A[\"content dir\"] --> B[parse]\n```\n\n"+
		"- a list item\n\n  ```mermaid\n  sequenceDiagram\n  ```\n\n"+
		"```vega-lite\n{\"mark\": \"bar\", \"data\": {\"values\": [{\"a\": 1}]}}\n```\n\n"+
		"```sh\nokf-wiki render --content docs\n```\n\n"+
		"After the drawing.\n")
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	for _, e := range r.BuildSearchIndex().Entries {
		text.WriteString(e.Content)
		text.WriteByte(' ')
	}
	got := text.String()
	for _, absent := range []string{"flowchart", "content dir", "sequenceDiagram", `"mark"`, "bar"} {
		if strings.Contains(got, absent) {
			t.Errorf("the indexed text carries fence source %q: %s", absent, got)
		}
	}
	for _, present := range []string{"Rendering is a single pass.", "a list item", "okf-wiki render --content docs", "After the drawing."} {
		if !strings.Contains(got, present) {
			t.Errorf("the indexed text lost %q: %s", present, got)
		}
	}
}
