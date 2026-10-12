package wiki

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A rendered diagram has nothing for assistive technology to announce, and the
// name it should carry is decided in the browser: mermaid renders the fence
// client-side, and there is no headless Chromium on the test path for
// make check to stay green without node. Both parts are asserted instead on
// the shipped asset and on the markup the server emits.
func diagramsAsset(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(docsRoot(t), "internal", "wiki",
		"themes", "default", "assets", "diagrams.js"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// fnBody cuts one function out of the asset, so an assertion is about a single
// path. The same strings also appear in the toolbar and the viewer, where they
// would make a file-wide check pass no matter what the naming path did.
func fnBody(t *testing.T, src, signature string) string {
	t.Helper()
	start := strings.Index(src, signature)
	if start < 0 {
		t.Fatalf("diagrams.js does not declare %s", signature)
	}
	rest := src[start+len(signature):]
	if end := strings.Index(rest, "\n  function "); end >= 0 {
		return src[start : start+len(signature)+end]
	}
	return src[start:]
}

// accTitle and accDescr are the authoring surface for an accessible name.
// Mermaid answers them with a <title> and a <desc> as the SVG's first children.
// The SVG then has to be one image, or every node label in it stays exposed on
// its own.
func TestDiagramSVGCarriesAnAccessibleName(t *testing.T) {
	client := diagramsAsset(t)
	// If svgMeta is renamed, or a tag lookup goes, this fails: it is the only
	// place the author's title and description are read.
	for _, want := range []string{"svgMeta(svg, 'title')", "svgMeta(svg, 'desc')"} {
		if !strings.Contains(client, want) {
			t.Errorf("diagrams.js does not read %s, so an accTitle or accDescr "+
				"leaves the drawing with nothing to name it", want)
		}
	}
	// Chromium reads an SVG root's own <title> and <desc> as the accessible
	// description next to the label, so the two elements go once their text is
	// in the label. Without this the description is announced twice.
	if !strings.Contains(fnBody(t, client, "function diagramName("), ".remove()") {
		t.Error("the naming path leaves mermaid's <title> and <desc> in the " +
			"SVG, so the drawing is announced twice")
	}
	// Without the image role the shapes inside the SVG are announced one by one.
	if !strings.Contains(client, "setAttribute('role', 'img')") {
		t.Error("diagrams.js does not announce the diagram as one image")
	}
	// decorate is the path every successfully rendered diagram takes.
	if !strings.Contains(fnBody(t, client, "function decorate(pre)"), "nameDiagram(pre)") {
		t.Error("decorate does not name the drawing, so a rendered diagram " +
			"carries no accessible name")
	}
}

// Without directives the name is the type word plus the nearest preceding
// heading. Both readers already existed for the toolbar and the viewer, so the
// naming path uses them rather than a second copy that drifts.
func TestDiagramNameReusesTheTypeAndHeadingReaders(t *testing.T) {
	client := diagramsAsset(t)
	// The path is two functions: the caller resolves the type, the callee
	// resolves the text and falls back to the heading.
	body := fnBody(t, client, "function nameDiagram(pre)") +
		fnBody(t, client, "function diagramName(")
	if !strings.Contains(body, "diagramType(pre)") {
		t.Error("the naming path does not read the type through diagramType, " +
			"so a type word here can drift from TYPE_NAMES and from the " +
			"toolbar and the viewer")
	}
	if !strings.Contains(body, "titleFor(pre)") {
		t.Error("the naming path does not fall back to titleFor, so it has " +
			"grown its own copy of the heading walk")
	}
}

// The runtime swaps the block's contents in the browser, so the fence body is
// what a reader without the runtime gets and what the copy button puts on the
// clipboard. Escaping or trimming it server-side would change the diagram.
func TestDiagramFenceBodySurvivesTheRender(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\n---\n# Home\n")
	src, err := os.ReadFile(filepath.Join(docsDir(t), "testbed", "diagrams.md"))
	if err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, filepath.Join(dir, "testbed"))
	mustWrite(t, filepath.Join(dir, "testbed", "diagrams.md"), string(src))

	r, err := New(Config{Content: dir, Brand: Brand{Name: "okf-wiki"}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatalf("RenderAll: %v", err)
	}
	page, err := os.ReadFile(filepath.Join(out, "wiki", "testbed", "diagrams.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(page)
	if !strings.Contains(html, "<pre class=\"mermaid\">") {
		t.Fatal("the rendered page has no pre.mermaid block")
	}
	for _, want := range []string{
		"flowchart TD\n    A[\"docs/*.md\"] --> B{\"frontmatter<br/>present?\"}",
		"sequenceDiagram\n    participant U as Browser",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the fence body did not survive the render verbatim: %q", want)
		}
	}
}
