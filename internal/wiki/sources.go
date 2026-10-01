package wiki

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/yuin/goldmark/ast"
	footnoteast "github.com/yuin/goldmark/extension/ast"
)

// Source records one material a concept derives from: a followable artifact
// or a scope descriptor, plus optional credibility signals. It mirrors the
// OKF sources family; per-claim attribution joins body footnotes to entries
// through id.
type Source struct {
	ID           string `yaml:"id"`
	Resource     string `yaml:"resource"`
	Title        string `yaml:"title"`
	Author       string `yaml:"author"`
	UsageCount   int    `yaml:"usage_count"`
	LastModified string `yaml:"last_modified"`
}

// applySources rewrites footnote references whose label matches a sources[]
// id into numbered links to the generated Sources section, and drops their
// definitions from the footnote list so they do not render dangling.
// Unmatched footnotes render exactly as goldmark would. A repeated citation
// reuses its number.
//
// Numbering counts matched citations only, while goldmark numbers every
// footnote in parse order, so a page mixing both can show one numeral twice.
// Both links still resolve; only the numerals collide.
func (r *Renderer) applySources(doc ast.Node, p *Page, usedIDs map[string]bool) {
	if len(p.Sources) == 0 {
		return
	}
	// Every entry is listed, in frontmatter order, and numbering follows that
	// order: a reference list is numbered by position, not by where a claim
	// happens to cite it. An entry with no id cannot be cited but is still
	// listed, since it is a source the page declares.
	byID := map[string]int{}
	for i := range p.Sources {
		if id := strings.TrimSpace(p.Sources[i].ID); id != "" {
			if _, dup := byID[id]; !dup {
				byID[id] = i + 1
			}
		}
	}
	// Footnote links carry an index, not a label; the definitions map index
	// back to label. Collection precedes mutation throughout, so no walk
	// ever sees a shifting tree.
	indexLabel := map[int]string{}
	var defs []*footnoteast.Footnote
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if fn, ok := n.(*footnoteast.Footnote); ok {
			indexLabel[fn.Index] = string(fn.Ref)
			defs = append(defs, fn)
		}
		return ast.WalkContinue, nil
	})
	type cite struct {
		link *footnoteast.FootnoteLink
		id   string
	}
	var cites []cite
	cited := map[string]bool{}
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		fl, ok := n.(*footnoteast.FootnoteLink)
		if !ok {
			return ast.WalkContinue, nil
		}
		label, ok := indexLabel[fl.Index]
		if !ok {
			return ast.WalkContinue, nil
		}
		id := strings.TrimSpace(label)
		if _, ok := byID[id]; !ok {
			return ast.WalkContinue, nil
		}
		cites = append(cites, cite{link: fl, id: id})
		cited[id] = true
		return ast.WalkContinue, nil
	})
	if len(cites) == 0 {
		return
	}
	// The section id is fixed before the refs so they can point at it. It is
	// uniquified against the page, since a page may already link somewhere
	// called sources.
	sid := uniqueID("sources", usedIDs)
	for _, c := range cites {
		link := ast.NewLink()
		link.Destination = []byte("#" + sid)
		link.SetAttribute([]byte("class"), []byte("source-ref"))
		link.AppendChild(link, ast.NewString([]byte(fmt.Sprintf("[%d]", byID[c.id]))))
		if parent := c.link.Parent(); parent != nil {
			parent.ReplaceChild(parent, c.link, link)
		}
	}
	for _, fn := range defs {
		if !cited[strings.TrimSpace(string(fn.Ref))] {
			continue
		}
		if parent := fn.Parent(); parent != nil {
			parent.RemoveChild(parent, fn)
		}
	}
	// An emptied footnote list would render a stray rule and an empty list,
	// so it goes too.
	var lists []*footnoteast.FootnoteList
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if list, ok := n.(*footnoteast.FootnoteList); ok && list.FirstChild() == nil {
			lists = append(lists, list)
		}
		return ast.WalkContinue, nil
	})
	for _, list := range lists {
		if parent := list.Parent(); parent != nil {
			parent.RemoveChild(parent, list)
		}
	}
	r.appendSources(doc, p, sid)
}

// appendSources adds the Sources section to the document and registers it in
// the table of contents, so the generated heading keeps the same contract
// as an authored one.
func (r *Renderer) appendSources(doc ast.Node, p *Page, sid string) {
	h := &ast.Heading{Level: 2}
	h.SetAttribute([]byte("id"), []byte(sid))
	h.AppendChild(h, ast.NewString([]byte("Sources")))
	doc.AppendChild(doc, h)
	// Start at 1: goldmark's default start is 0, which renders start="0" and
	// numbers the entries from zero.
	list := &ast.List{Marker: '.', IsTight: true, Start: 1}
	for i := range p.Sources {
		s := &p.Sources[i]
		item := &ast.ListItem{}
		para := ast.NewParagraph()
		text, href := r.sourceLink(p.Slug, s)
		if href != "" {
			link := ast.NewLink()
			link.Destination = []byte(href)
			link.AppendChild(link, ast.NewString([]byte(text)))
			para.AppendChild(para, link)
		} else {
			para.AppendChild(para, ast.NewString([]byte(text)))
		}
		for _, m := range sourceMeta(s) {
			para.AppendChild(para, ast.NewString([]byte(" · "+m)))
		}
		item.AppendChild(item, para)
		list.AppendChild(list, item)
	}
	doc.AppendChild(doc, list)
	p.TOC = append(p.TOC, TOCEntry{ID: sid, Text: "Sources"})
}

// sourceLink resolves a source entry to display text and an href. Absolute
// URLs link as-is; paths resolve like any bundle link; a scope descriptor,
// which is free text rather than a destination, renders as text. Text falls
// back from title to resource to id, so an entry is never an empty link.
func (r *Renderer) sourceLink(fromSlug string, s *Source) (string, string) {
	res := strings.TrimSpace(s.Resource)
	text := strings.TrimSpace(s.Title)
	if text == "" {
		text = res
	}
	if text == "" {
		text = strings.TrimSpace(s.ID)
	}
	if res == "" || strings.ContainsAny(res, " \t\n") {
		return text, ""
	}
	return text, r.linkPath(fromSlug, res)
}

// sourceMeta renders the available credibility signals: the host for
// absolute URLs, then author, then the last-modified date. Absent signals
// are omitted, never blank placeholders. An unparsable date is shown raw
// rather than dropped, since a wrong-looking date the author can see beats
// a silently missing one.
func sourceMeta(s *Source) []string {
	var out []string
	if u, err := url.Parse(strings.TrimSpace(s.Resource)); err == nil && u.Hostname() != "" {
		out = append(out, u.Hostname())
	}
	if a := strings.TrimSpace(s.Author); a != "" {
		out = append(out, a)
	}
	if d := strings.TrimSpace(s.LastModified); d != "" {
		if t, err := time.Parse(time.RFC3339, d); err == nil {
			d = t.Format("2 Jan 2006")
		}
		out = append(out, d)
	}
	return out
}
