package wiki

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCodeCopyAffordanceAssetContract guards the client-side copy affordance on code
// and text blocks, the way TestDiagramCardContract guards the diagram card.
func TestCodeCopyAffordanceAssetContract(t *testing.T) {
	js, err := os.ReadFile(filepath.Join(docsRoot(t), "internal", "wiki",
		"themes", "default", "assets", "diagrams.js"))
	if err != nil {
		t.Fatal(err)
	}
	client := string(js)
	for _, want := range []string{
		"code-copy", "copyCode", "enhanceCodeBlocks",
		`setAttribute('aria-label', 'Copy code')`,
		"<rect x=\"9\" y=\"9\" width=\"12\" height=\"12\" rx=\"2\"/>",
	} {
		if !strings.Contains(client, want) {
			t.Errorf("diagrams.js does not contain %q", want)
		}
	}

	css, err := os.ReadFile(filepath.Join(docsRoot(t), "internal", "wiki",
		"themes", "default", "assets", "wiki.css"))
	if err != nil {
		t.Fatal(err)
	}
	sheet := string(css)
	for _, want := range []string{
		".code-copy {",
		"position: absolute",
		".code-copy[data-copy=\"ok\"]",
		".code-copy[data-copy=\"fail\"]",
	} {
		if !strings.Contains(sheet, want) {
			t.Errorf("wiki.css does not contain %q", want)
		}
	}
	if !strings.Contains(sheet, ".mermaid-actions, .code-copy, .heading-anchor") {
		t.Error("the print stylesheet does not hide the code copy button")
	}
}
