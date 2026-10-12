package wiki

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// A callout tells assistive technology that the block is a note rather than
// ordinary prose, but it is not a landmark: a landmark list holding one entry
// per callout buries the page's own regions. The title, which is visible text
// inside the callout, is what names it, so an attribute repeating that word
// made a screen reader announce it twice.
func TestCalloutIsANoteNotALandmark(t *testing.T) {
	page := renderOnlyPage(t, t.TempDir(), alertDoc("[!NOTE]"))
	tag := calloutTag(t, page, "note")

	if want := ` role="note"`; !strings.Contains(tag, want) {
		t.Errorf("callout opening tag has no note role:\n%s", tag)
	}
	if strings.Contains(tag, "aria-label") {
		t.Errorf("callout opening tag repeats the visible title in an aria-label:\n%s", tag)
	}
	if strings.Contains(page, `role="region"`) {
		t.Errorf("a callout still marks itself a region landmark")
	}
	if !strings.Contains(page, `<span>Note</span>`) {
		t.Errorf("callout lost the title that names it:\n%s", page)
	}
	if !strings.Contains(page, "Body text.") {
		t.Errorf("callout lost its body:\n%s", page)
	}
}

func TestEveryAlertKindIsANote(t *testing.T) {
	for _, kind := range []string{"note", "tip", "important", "warning", "caution"} {
		t.Run(kind, func(t *testing.T) {
			page := renderOnlyPage(t, t.TempDir(), alertDoc("[!"+strings.ToUpper(kind)+"]"))
			tag := calloutTag(t, page, kind)

			if !strings.Contains(tag, ` role="note"`) {
				t.Errorf("%s callout opening tag has no note role:\n%s", kind, tag)
			}
			if strings.Contains(tag, "aria-label") {
				t.Errorf("%s callout opening tag has an aria-label:\n%s", kind, tag)
			}
			if strings.Contains(page, `role="region"`) {
				t.Errorf("%s callout still marks itself a region landmark", kind)
			}
		})
	}
}

// A kind the marker table does not name still renders a card, and the title it
// falls back to is now the only thing that names it.
func TestCalloutOfAnUnnamedKind(t *testing.T) {
	src := []byte("> Body text.\n")
	md := goldmark.New(goldmark.WithRendererOptions(
		html.WithUnsafe(),
		renderer.WithNodeRenderers(util.Prioritized(newCustomRenderer(), 500)),
	))
	doc := md.Parser().Parse(text.NewReader(src))
	doc.FirstChild().SetAttribute([]byte("data-alert"), []byte("mystery"))

	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, src, doc); err != nil {
		t.Fatal(err)
	}
	if want := `<div class="callout callout-mystery" role="note">`; !strings.Contains(buf.String(), want) {
		t.Errorf("unnamed kind did not render a note callout:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), `<span>Mystery</span>`) {
		t.Errorf("unnamed kind renders no visible title:\n%s", buf.String())
	}
}

// Without scope a screen reader reads a header cell as body text, so a cell
// arrives as "Left Useful information" with nothing naming the column.
func TestEveryHeaderCellCarriesScope(t *testing.T) {
	page := renderOnlyPage(t, t.TempDir(), alignedTableDoc())
	th := regexp.MustCompile(`<th(?:\s[^>]*)?>`).FindAllString(page, -1)
	if len(th) == 0 {
		t.Fatalf("no header cell in the rendered page:\n%s", page)
	}
	for _, tag := range th {
		if !strings.Contains(tag, `scope="col"`) {
			t.Errorf("header cell does not say which direction it heads: %s", tag)
		}
	}
}

// scope is obsolete on a td, and goldmark does not mark a leading data cell as a
// row header, so a body cell gets nothing.
func TestBodyCellsCarryNoScope(t *testing.T) {
	page := renderOnlyPage(t, t.TempDir(), alignedTableDoc())
	td := regexp.MustCompile(`<td(?:\s[^>]*)?>`).FindAllString(page, -1)
	if len(td) == 0 {
		t.Fatalf("no body cell in the rendered page:\n%s", page)
	}
	for _, tag := range td {
		if strings.Contains(tag, "scope") {
			t.Errorf("body cell carries scope, which is obsolete on a td: %s", tag)
		}
	}
}

// The alignment a cell already carries has to be appended to rather than
// replaced, and the branch a renderer is built with picks attribute over style,
// so both are checked against goldmark's own output over the same table.
func TestCellRendererKeepsAlignmentPerMethod(t *testing.T) {
	src := []byte(alignedTableBody + "\n")

	// render builds goldmark with GFM plus, when ours is set, this package's
	// cell renderer standing in for GFM's own, so a table can be rendered both
	// ways and compared.
	render := func(xhtml, ours bool, prepare func(ast.Node)) string {
		opts := []renderer.Option{html.WithXHTML()}
		if !xhtml {
			opts[0] = html.WithUnsafe()
		}
		if ours {
			opts = append(opts, renderer.WithNodeRenderers(util.Prioritized(newCustomRenderer(), 500)))
		}
		md := goldmark.New(goldmark.WithExtensions(extension.GFM), goldmark.WithRendererOptions(opts...))
		doc := md.Parser().Parse(text.NewReader(src))
		if prepare != nil {
			prepare(doc)
		}
		var buf bytes.Buffer
		if err := md.Renderer().Render(&buf, src, doc); err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(buf.String())
	}

	for _, tc := range []struct {
		name    string
		xhtml   bool
		prepare func(ast.Node)
	}{
		{"style method", false, nil},
		{"attribute method", true, nil},
		{"style appended to an existing style", false, setCellStyle},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := render(tc.xhtml, false, tc.prepare) // GFM alone
			got := strings.ReplaceAll(render(tc.xhtml, true, tc.prepare), ` scope="col"`, "")
			if diff := diffLines(got, want); diff != "" {
				t.Errorf("table cells differ from goldmark's own rendering%s\nours:\n%s\nwant:\n%s", diff, got, want)
			}
		})
	}
}

// setCellStyle writes a style onto every cell of a parsed table, which puts the
// cells into the state goldmark's style branch appends to.
func setCellStyle(doc ast.Node) {
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if c, ok := n.(*extast.TableCell); ok {
				c.SetAttributeString("style", []byte("color:red"))
			}
		}
		return ast.WalkContinue, nil
	})
}

// The repo's own bundle is the widest content the renderer sees in practice, so
// a callout that stayed a landmark here, or a header cell that lost its scope,
// would ship.
func TestDocsBundleHasNoLandmarkCallouts(t *testing.T) {
	r, err := New(Config{Content: docsDir(t), Brand: Brand{Name: "okf-wiki", Sub: "docs", Title: "Wiki"}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatalf("RenderAll: %v", err)
	}

	th := regexp.MustCompile(`<th(?:\s[^>]*)?>`)
	td := regexp.MustCompile(`<td(?:\s[^>]*)?>`)
	err = filepath.WalkDir(filepath.Join(out, "wiki"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".html" {
			return err
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		name, page := filepath.Base(p), string(raw)
		if strings.Contains(page, `role="region"`) {
			t.Errorf("%s: a callout is still a region landmark", name)
		}
		for _, tag := range th.FindAllString(page, -1) {
			if !strings.Contains(tag, `scope="col"`) {
				t.Errorf("%s: header cell without scope: %s", name, tag)
			}
		}
		for _, tag := range td.FindAllString(page, -1) {
			if strings.Contains(tag, "scope") {
				t.Errorf("%s: body cell with scope: %s", name, tag)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// calloutTag returns the opening tag of the first callout of kind, which is
// where the role and any name-carrying attribute live.
func calloutTag(t *testing.T, page, kind string) string {
	t.Helper()
	re := regexp.MustCompile(`<div class="callout callout-` + regexp.QuoteMeta(kind) + `"[^>]*>`)
	tag := re.FindString(page)
	if tag == "" {
		t.Fatalf("no %s callout in the rendered page:\n%s", kind, page)
	}
	return tag
}

// diffLines reports the lines where two renders disagree, so a failure names the
// cells that moved instead of dumping two tables.
func diffLines(got, want string) string {
	if got == want {
		return ""
	}
	g, w := strings.Split(got, "\n"), strings.Split(want, "\n")
	var b strings.Builder
	for i := 0; i < len(g) || i < len(w); i++ {
		gl, wl := "", ""
		if i < len(g) {
			gl = g[i]
		}
		if i < len(w) {
			wl = w[i]
		}
		if gl != wl {
			fmt.Fprintf(&b, "\nline %d: ours %q, goldmark %q", i+1, gl, wl)
		}
	}
	return b.String()
}

var alignedTableBody = strings.Join([]string{
	"| Left | Centre | Right | Default |",
	"| :--- | :----: | ----: | -------- |",
	"| a | bb | ccc | dddd |",
	"| longer cell | x | yyyy | z |",
}, "\n")

func alignedTableDoc() string {
	return strings.Join([]string{
		"---",
		"title: Table",
		"---",
		"# Table",
		alignedTableBody,
	}, "\n")
}
