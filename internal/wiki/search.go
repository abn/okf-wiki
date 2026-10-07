package wiki

import (
	"encoding/json"
	"strings"

	"github.com/yuin/goldmark/ast"
)

// SearchEntry is one search hit: a heading-anchored chunk of a page, or the
// page itself when it produced no chunks. Title is the heading and Doc the
// enclosing page, which differ whenever a hit is anchored mid-page.
type SearchEntry struct {
	URL     string `json:"u"` // deep link anchor or page url
	PageURL string `json:"p"` // page url
	Title   string `json:"t"` // heading title or page title
	Doc     string `json:"d"` // enclosing page title
	Section string `json:"s"` // section category (e.g. Architecture)
	Content string `json:"c"` // extracted plain text
	// Site is the wiki's display title, set only on a tree's aggregate index so
	// the client can head a group of results with the wiki they came from.
	Site string `json:"w,omitempty"`
	// SiteKey is the wiki's path under the tree root: a stable grouping key,
	// since the display title can change without the path moving.
	SiteKey string `json:"k,omitempty"`
}

// rawSearchChunk is an unresolved text section extracted from a page's AST.
type rawSearchChunk struct {
	Heading string
	Anchor  string
	Text    string
}

// extractSearchChunks splits a page into one text chunk per heading, anchored
// to that heading's id. The H1 is skipped, so content under it joins the first
// real heading's chunk.
func extractSearchChunks(doc ast.Node, src []byte) []rawSearchChunk {
	var chunks []rawSearchChunk
	currentHeading := ""
	currentAnchor := ""
	var currentText strings.Builder

	for block := doc.FirstChild(); block != nil; block = block.NextSibling() {
		if h, ok := block.(*ast.Heading); ok {
			if h.Level == 1 {
				continue
			}
			txt := cleanSearchText(currentText.String())
			if txt != "" || currentHeading != "" {
				chunks = append(chunks, rawSearchChunk{Heading: currentHeading, Anchor: currentAnchor, Text: txt})
			}
			currentText.Reset()
			currentHeading = headingText(h, src)
			if val, ok := h.Attribute([]byte("id")); ok {
				currentAnchor = string(val.([]byte))
			} else {
				currentAnchor = slugify(currentHeading)
			}
		} else {
			extractRawBlockText(block, src, &currentText)
			currentText.WriteByte(' ')
		}
	}

	txt := cleanSearchText(currentText.String())
	if txt != "" || currentHeading != "" {
		chunks = append(chunks, rawSearchChunk{Heading: currentHeading, Anchor: currentAnchor, Text: txt})
	}
	return chunks
}

func extractRawBlockText(n ast.Node, src []byte, b *strings.Builder) {
	if n == nil {
		return
	}
	switch nn := n.(type) {
	case *ast.Text:
		b.Write(nn.Segment.Value(src))
	case *ast.String:
		b.Write(nn.Value)
	case *ast.CodeSpan:
		for c := nn.FirstChild(); c != nil; c = c.NextSibling() {
			extractRawBlockText(c, src, b)
		}
	case *ast.FencedCodeBlock:
		for i := 0; i < nn.Lines().Len(); i++ {
			line := nn.Lines().At(i)
			b.Write(line.Value(src))
			b.WriteByte(' ')
		}
	case *ast.CodeBlock:
		for i := 0; i < nn.Lines().Len(); i++ {
			line := nn.Lines().At(i)
			b.Write(line.Value(src))
			b.WriteByte(' ')
		}
	default:
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			extractRawBlockText(c, src, b)
		}
	}
}

func cleanSearchText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// BuildSearchIndex flattens every page into one entry per heading-anchored
// chunk.
func (r *Renderer) BuildSearchIndex() []SearchEntry {
	var index []SearchEntry
	for _, s := range r.sections {
		secTitle := s.Title
		if secTitle == "" {
			secTitle = "Overview"
		}
		for _, p := range s.Pages {
			pageURL := r.cfg.base() + p.Slug + ".html"
			docTitle := p.Title
			if docTitle == "" {
				docTitle = p.Slug
			}

			if len(p.searchChunks) == 0 {
				index = append(index, SearchEntry{
					URL: pageURL, PageURL: pageURL, Title: docTitle, Doc: docTitle, Section: secTitle,
				})
				continue
			}
			for _, chunk := range p.searchChunks {
				itemURL := pageURL
				if chunk.Anchor != "" {
					itemURL = pageURL + "#" + chunk.Anchor
				}
				itemTitle := chunk.Heading
				if itemTitle == "" {
					itemTitle = docTitle
				}
				index = append(index, SearchEntry{
					URL: itemURL, PageURL: pageURL, Title: itemTitle, Doc: docTitle, Section: secTitle, Content: chunk.Text,
				})
			}
		}
	}
	return index
}

// SearchIndexJSON returns the JSON representation of the search index.
func (r *Renderer) SearchIndexJSON() ([]byte, error) {
	return json.Marshal(r.BuildSearchIndex())
}

// labelSearchIndex tags a wiki's own index with the wiki's display title and its
// path under the tree root, so one aggregate index can group rows by wiki. It
// is deliberately separate from BuildSearchIndex: a single bundle's index stays
// unlabelled, and its JSON is unchanged.
func labelSearchIndex(index []SearchEntry, site, key string) []SearchEntry {
	out := make([]SearchEntry, len(index))
	for i, e := range index {
		e.Site = site
		e.SiteKey = key
		out[i] = e
	}
	return out
}
