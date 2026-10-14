package wiki

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// alertInfo holds metadata for a GitHub-style alert callout. The kind is all
// it carries: alertTitle derives the label from the kind once the kind is on
// the node, so a second copy of the label here would only be able to drift.
type alertInfo struct {
	kind string // note, tip, important, warning, caution
}

var alertPrefixes = map[string]alertInfo{
	"[!NOTE]":      {kind: "note"},
	"[!TIP]":       {kind: "tip"},
	"[!IMPORTANT]": {kind: "important"},
	"[!WARNING]":   {kind: "warning"},
	"[!CAUTION]":   {kind: "caution"},
}

// detectAlert inspects a blockquote node. If it starts with [!NOTE] or similar,
// in any letter case, it strips the marker and tags the node with data-alert.
func detectAlert(bq *ast.Blockquote, source []byte) {
	p, ok := bq.FirstChild().(*ast.Paragraph)
	if !ok {
		return
	}
	// The probe stops at the first node that is not text. Walking past one
	// concatenates across it, so a marker sitting after a node that contributes
	// nothing reads as though it opened the line, and the whitespace skip after
	// it would then delete a space that belongs to that node.
	var b strings.Builder
	for c := p.FirstChild(); c != nil; c = c.NextSibling() {
		textNode, ok := c.(*ast.Text)
		if !ok {
			break
		}
		b.Write(textNode.Segment.Value(source))
	}
	val := b.String()
	for prefix, info := range alertPrefixes {
		// GitHub matches the marker without regard to case, so `[!warning]`
		// renders the way `[!WARNING]` does. EqualFold over a window of the
		// prefix's own length keeps the byte offsets below valid, which
		// uppercasing the whole value would not.
		if len(val) < len(prefix) || !strings.EqualFold(val[:len(prefix)], prefix) {
			continue
		}
		bq.SetAttribute([]byte("data-alert"), []byte(info.kind))
		rem := len(prefix)
		for rem < len(val) && (val[rem] == ' ' || val[rem] == '\t' || val[rem] == '\n' || val[rem] == '\r') {
			rem++
		}
		toRemove := rem
		for c := p.FirstChild(); c != nil && toRemove > 0; {
			next := c.NextSibling()
			if textNode, ok := c.(*ast.Text); ok {
				segLen := textNode.Segment.Len()
				if segLen <= toRemove {
					p.RemoveChild(p, c)
					toRemove -= segLen
				} else {
					textNode.Segment = text.NewSegment(textNode.Segment.Start+toRemove, textNode.Segment.Stop)
					toRemove = 0
				}
			}
			c = next
		}
		return
	}
}

// customRenderer implements callout blockquotes, table cells, and
// chroma-highlighted fenced code blocks (with a mermaid passthrough).
//
// The config is GFM's own TableConfig so that a table cell renders through the
// same settings the GFM extension carries, alignment method included.
type customRenderer struct {
	extension.TableConfig
}

func newCustomRenderer() renderer.NodeRenderer {
	return &customRenderer{TableConfig: extension.NewTableConfig()}
}

// RegisterFuncs implements renderer.NodeRenderer.
//
// GFM registers a renderer for table cells at priority 500, the same priority as
// this one, so registration order decides which of the two renders a cell.
// Goldmark registers renderers from the end of the sorted list backwards, so
// among equals the one appended first wins, and this renderer is appended before
// the GFM extension extends the markdown.
func (r *customRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindBlockquote, r.renderBlockquote)
	reg.Register(ast.KindFencedCodeBlock, r.renderFencedCodeBlock)
	reg.Register(extast.KindTableCell, r.renderTableCell)
}

func (r *customRenderer) renderBlockquote(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	attr, ok := n.Attribute([]byte("data-alert"))
	if !ok {
		if entering {
			_, _ = w.WriteString("<blockquote>\n")
		} else {
			_, _ = w.WriteString("</blockquote>\n")
		}
		return ast.WalkContinue, nil
	}

	kind := string(attr.([]byte))
	title := alertTitle(kind)

	if entering {
		_, _ = fmt.Fprintf(w, "<div class=\"callout callout-%s\" role=\"note\">\n", kind)
		_, _ = fmt.Fprintf(w, "<div class=\"callout-title\">%s<span>%s</span></div>\n", alertIconSVG(kind), title)
		_, _ = w.WriteString("<div class=\"callout-body\">\n")
	} else {
		_, _ = w.WriteString("</div>\n</div>\n")
	}
	return ast.WalkContinue, nil
}

func (r *customRenderer) renderFencedCodeBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.FencedCodeBlock)
	lang := string(n.Language(source))

	var buf bytes.Buffer
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		buf.Write(line.Value(source))
	}
	codeText := buf.String()

	if lang == "mermaid" {
		_, _ = w.WriteString("<pre class=\"mermaid\">\n")
		_, _ = w.WriteString(codeText)
		_, _ = w.WriteString("</pre>\n")
		return ast.WalkSkipChildren, nil
	}

	if lang == "" {
		_, _ = w.WriteString("<pre class=\"code\"><code>")
		_, _ = w.WriteString(template.HTMLEscapeString(codeText))
		_, _ = w.WriteString("</code></pre>\n")
		return ast.WalkSkipChildren, nil
	}

	lexer := lexers.Get(lang)
	if lexer == nil {
		lexer = lexers.Match(lang)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	style := styles.Get("github-dark")
	formatter := chromahtml.New(chromahtml.WithClasses(true), chromahtml.PreventSurroundingPre(true))
	iterator, err := lexer.Tokenise(nil, codeText)
	if err != nil {
		_, _ = fmt.Fprintf(w, "<pre class=\"code\"><code class=\"language-%s\">", template.HTMLEscapeString(lang))
		_, _ = w.WriteString(template.HTMLEscapeString(codeText))
		_, _ = w.WriteString("</code></pre>\n")
		return ast.WalkSkipChildren, nil
	}

	_, _ = fmt.Fprintf(w, "<pre class=\"code chroma\"><code class=\"language-%s\">", template.HTMLEscapeString(lang))
	_ = formatter.Format(w, style, iterator)
	_, _ = w.WriteString("</code></pre>\n")
	return ast.WalkSkipChildren, nil
}

// renderTableCell is a copy of the alignment logic in
// extension.(*TableHTMLRenderer).renderTableCell, with one addition: a header
// cell says which direction it heads. Keep the copied half in step with
// goldmark, since a change to how it emits alignments has to land here too.
func (r *customRenderer) renderTableCell(
	w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*extast.TableCell)
	tag := "td"
	if n.Parent().Kind() == extast.KindTableHeader {
		tag = "th"
	}
	if entering {
		_, _ = fmt.Fprintf(w, "<%s", tag)
		if n.Alignment != extast.AlignNone {
			amethod := r.TableConfig.TableCellAlignMethod
			if amethod == extension.TableCellAlignDefault {
				if r.Config.XHTML {
					amethod = extension.TableCellAlignAttribute
				} else {
					amethod = extension.TableCellAlignStyle
				}
			}
			switch amethod {
			case extension.TableCellAlignAttribute:
				if _, ok := n.AttributeString("align"); !ok { // Skip align render if overridden
					_, _ = fmt.Fprintf(w, ` align="%s"`, n.Alignment.String())
				}
			case extension.TableCellAlignStyle:
				v, ok := n.AttributeString("style")
				var cob util.CopyOnWriteBuffer
				if ok {
					switch v := v.(type) {
					case []byte:
						cob = util.NewCopyOnWriteBuffer(v)
					case string:
						cob = util.NewCopyOnWriteBuffer([]byte(v))
					}
					cob.AppendByte(';')
				}
				style := fmt.Sprintf("text-align:%s", n.Alignment.String())
				cob.AppendString(style)
				n.SetAttributeString("style", cob.Bytes())
			}
		}
		if n.Attributes() != nil {
			if tag == "td" {
				html.RenderAttributes(w, n, extension.TableTdCellAttributeFilter) // <td>
			} else {
				html.RenderAttributes(w, n, extension.TableThCellAttributeFilter) // <th>
			}
		}
		// scope is obsolete on a <td>, and goldmark marks no cell as a row
		// header, so only a column header gets it. Without it a screen reader
		// reads a cell as "Left Useful information", with nothing saying that
		// Left is the column it sits under.
		if tag == "th" {
			_, _ = w.WriteString(` scope="col"`)
		}
		_ = w.WriteByte('>')
	} else {
		_, _ = fmt.Fprintf(w, "</%s>\n", tag)
	}
	return ast.WalkContinue, nil
}

func alertTitle(kind string) string {
	switch kind {
	case "note":
		return "Note"
	case "tip":
		return "Tip"
	case "important":
		return "Important"
	case "warning":
		return "Warning"
	case "caution":
		return "Caution"
	default:
		return titleCase(kind)
	}
}

func alertIconSVG(kind string) string {
	switch kind {
	case "tip":
		return `<svg class="callout-icon" viewBox="0 0 16 16" width="16" height="16" fill="currentColor" aria-hidden="true"><path d="M8 1.5c-2.363 0-4 1.69-4 3.75 0 .984.424 1.625.984 2.304l.214.253c.223.264.47.556.673.848.284.411.537.896.621 1.49a.75.75 0 0 1-1.484.21c-.04-.282-.163-.537-.367-.833-.232-.335-.522-.677-.775-.977l-.219-.26C3.005 7.519 2.5 6.612 2.5 5.25c0-2.879 2.288-5.25 5.5-5.25s5.5 2.371 5.5 5.25c0 1.362-.505 2.269-1.143 3.03l-.219.26c-.253.3-.543.642-.775.977-.204.296-.328.551-.367.833a.75.75 0 0 1-1.484-.21c.084-.594.337-1.079.621-1.49.203-.292.45-.584.673-.848l.214-.253c.56-.679.984-1.32.984-2.304 0-2.06-1.637-3.75-4-3.75ZM6 12a1 1 0 0 1 1-1h2a1 1 0 0 1 1 1v.5a.5.5 0 0 1-.5.5h-3a.5.5 0 0 1-.5-.5V12Zm1.5 3a.5.5 0 0 0 0 1h1a.5.5 0 0 0 0-1h-1Z"/></svg>`
	case "important":
		return `<svg class="callout-icon" viewBox="0 0 16 16" width="16" height="16" fill="currentColor" aria-hidden="true"><path d="M0 1.75C0 .784.784 0 1.75 0h12.5C15.216 0 16 .784 16 1.75v9.5A1.75 1.75 0 0 1 14.25 13H8.06l-2.573 2.573A1.458 1.458 0 0 1 3 14.543V13H1.75A1.75 1.75 0 0 1 0 11.25Zm1.75-.25a.25.25 0 0 0-.25.25v9.5c0 .138.112.25.25.25h2a.75.75 0 0 1 .75.75v2.19l2.72-2.72a.749.749 0 0 1 .53-.22h6.5a.25.25 0 0 0 .25-.25v-9.5a.25.25 0 0 0-.25-.25Zm7 2.25v2.5a.75.75 0 0 1-1.5 0v-2.5a.75.75 0 0 1 1.5 0ZM9 9a1 1 0 1 1-2 0 1 1 0 0 1 2 0Z"/></svg>`
	case "warning":
		return `<svg class="callout-icon" viewBox="0 0 16 16" width="16" height="16" fill="currentColor" aria-hidden="true"><path d="M6.457 1.047c.659-1.234 2.427-1.234 3.086 0l6.082 11.378A1.75 1.75 0 0 1 14.082 15H1.918a1.75 1.75 0 0 1-1.543-2.575Zm1.763.707a.25.25 0 0 0-.44 0L1.698 13.132a.25.25 0 0 0 .22.368h12.164a.25.25 0 0 0 .22-.368Zm.53 3.996v2.5a.75.75 0 0 1-1.5 0v-2.5a.75.75 0 0 1 1.5 0ZM9 11a1 1 0 1 1-2 0 1 1 0 0 1 2 0Z"/></svg>`
	case "caution":
		return `<svg class="callout-icon" viewBox="0 0 16 16" width="16" height="16" fill="currentColor" aria-hidden="true"><path d="M4.47.04c.186-.04.379-.04.565 0l7 1.5c.218.046.408.169.533.345.125.175.182.39.16.605l-.5 5c-.09 1.137-.48 2.227-1.135 3.176l-2.5 3.5a1.75 1.75 0 0 1-2.186.588l-.134-.068a1.75 1.75 0 0 1-.746-.788l-2.5-3.5A6.25 6.25 0 0 1 2.09 7.49l-.5-5a1 1 0 0 1 .693-.95ZM8 4.25a.75.75 0 0 0-.75.75v2.5a.75.75 0 0 0 1.5 0V5A.75.75 0 0 0 8 4.25Zm0 6a1 1 0 1 0 0-2 1 1 0 0 0 0 2Z"/></svg>`
	default: // note
		return `<svg class="callout-icon" viewBox="0 0 16 16" width="16" height="16" fill="currentColor" aria-hidden="true"><path d="M0 8a8 8 0 1 1 16 0A8 8 0 0 1 0 8Zm8-6.5a6.5 6.5 0 1 0 0 13 6.5 6.5 0 0 0 0-13ZM6.5 7.75A.75.75 0 0 1 7.25 7h1a.75.75 0 0 1 .75.75v2.75h.25a.75.75 0 0 1 0 1.5h-2a.75.75 0 0 1 0-1.5h.25v-2h-.25a.75.75 0 0 1-.75-.75ZM8 6a1 1 0 1 1 0-2 1 1 0 0 1 0 2Z"/></svg>`
	}
}
