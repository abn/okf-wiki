package wiki

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark/ast"
)

// SearchPage is one page's metadata, held once in the index's page table. Its
// entries refer to it by index, so a page's url and titles are not repeated on
// every section it contributed.
type SearchPage struct {
	ID    string `json:"id,omitempty"` // graph node ID
	URL   string `json:"u"`            // page url
	Title string `json:"t"`            // page title
	// Site is the wiki's display title, set only on a tree's aggregate index so
	// the client can head a group of results with the wiki they came from.
	Site string `json:"w,omitempty"`
	// SiteKey is the wiki's path under the tree root: a stable grouping key,
	// since the display title can change without the path moving.
	SiteKey string `json:"k,omitempty"`
}

// SearchEntry is one search hit: a heading-anchored chunk of a page, or the
// page itself when it produced no chunks. Page indexes the page table, and the
// client rebuilds the deep link as the page url plus "#" plus Anchor, an empty
// Anchor meaning the page url alone. Title is the heading and the page's title
// is on its row, which differ whenever a hit is anchored mid-page.
type SearchEntry struct {
	Page      int    `json:"g"`                   // index into the page table
	Anchor    string `json:"a"`                   // heading anchor, empty for the page itself
	Title     string `json:"t"`                   // heading title or page title
	Section   string `json:"s"`                   // section category (e.g. Architecture)
	Truncated bool   `json:"truncated,omitempty"` // the indexed text was capped
	// Content is the extracted plain text. It is held in memory so the entry is
	// one object throughout a render, but it is not written into the index: it
	// is most of the index's bytes, so it travels in the companion
	// search-content.json, which the client joins onto the entries in the
	// background.
	Content string `json:"-"`
}

// SearchIndex is the on-disk catalogue: a page table, and the entries that
// refer to it. Entry text is not part of it; SearchContent holds that.
type SearchIndex struct {
	RenderID string        `json:"renderId,omitempty"`
	Base     string        `json:"base,omitempty"`
	Pages    []SearchPage  `json:"pages"`
	Entries  []SearchEntry `json:"entries"`
}

// SearchContent is the companion file to SearchIndex: every entry's text, in
// entry order. It is separate because the text is the largest part of the
// catalogue and a reader can search titles, documents and sections before it
// arrives.
type SearchContent struct {
	RenderID string   `json:"renderId,omitempty"`
	Count    int      `json:"n"` // number of entries the texts align with
	Texts    []string `json:"c"` // text of entry 0, entry 1, ...
}

// rawSearchChunk is an unresolved text section extracted from a page's AST.
type rawSearchChunk struct {
	Heading   string
	Anchor    string
	Text      string
	Truncated bool
}

// maxSearchChunkBytes bounds one chunk's indexed text. Content dominates the
// index's bytes, and a page can hold a list or a log long enough that indexing
// its tail buys no ranking the head has not already won. The cut falls on a
// word boundary where one lies in the prefix and on a rune boundary otherwise,
// so the rest is simply not indexed.
const maxSearchChunkBytes = 12000

// boundSearchText caps s at maxSearchChunkBytes bytes, dropping a partial rune
// at the cut and then backing up to the last space, so a cut lands between
// words when the prefix has one.
func boundSearchText(s string) string {
	text, _ := boundSearchTextInfo(s)
	return text
}

func boundSearchTextInfo(s string) (string, bool) {
	if len(s) <= maxSearchChunkBytes {
		return s, false
	}
	cut := s[:maxSearchChunkBytes]
	// At most the rune straddling the cut can be partial, so step back over its
	// continuation bytes to its first byte and drop it. Revalidating the whole
	// prefix would discard everything from an earlier invalid byte onward.
	if r, size := utf8.DecodeLastRuneInString(cut); r == utf8.RuneError && size <= 1 {
		i := len(cut) - 1
		for n := 0; n < utf8.UTFMax-1 && i > 0 && !utf8.RuneStart(cut[i]); n++ {
			i--
		}
		cut = cut[:i]
	}
	if i := strings.LastIndexByte(cut, ' '); i > 0 {
		cut = cut[:i]
	}
	return cut, true
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
			txt, cut := boundSearchTextInfo(cleanSearchText(currentText.String()))
			if txt != "" || currentHeading != "" {
				chunks = append(chunks, rawSearchChunk{Heading: currentHeading, Anchor: currentAnchor, Text: txt, Truncated: cut})
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

	txt, cut := boundSearchTextInfo(cleanSearchText(currentText.String()))
	if txt != "" || currentHeading != "" {
		chunks = append(chunks, rawSearchChunk{Heading: currentHeading, Anchor: currentAnchor, Text: txt, Truncated: cut})
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
		if nn.SoftLineBreak() || nn.HardLineBreak() {
			b.WriteByte(' ')
		}
	case *ast.String:
		b.Write(nn.Value)
	case *ast.Link:
		for c := nn.FirstChild(); c != nil; c = c.NextSibling() {
			extractRawBlockText(c, src, b)
		}
		b.WriteByte(' ')
	case *ast.Paragraph, *ast.ListItem, *ast.List, *ast.Blockquote:
		for c := nn.FirstChild(); c != nil; c = c.NextSibling() {
			extractRawBlockText(c, src, b)
		}
		b.WriteByte(' ')
	case *ast.Image:
		for c := nn.FirstChild(); c != nil; c = c.NextSibling() {
			extractRawBlockText(c, src, b)
		}
		b.WriteByte(' ')
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

// BuildSearchIndex flattens every page into one page-table row and one entry
// per heading-anchored chunk.
func (r *Renderer) BuildSearchIndex() SearchIndex {
	index := SearchIndex{Pages: []SearchPage{}, Entries: []SearchEntry{}}
	for _, s := range r.sections {
		secTitle := sectionHeading(s)
		for _, p := range s.Pages {
			pageURL := r.urlBase() + p.Slug + ".html"
			docTitle := pageTitle(p)
			page := len(index.Pages)
			index.Pages = append(index.Pages, SearchPage{ID: pageNodeID(p.Slug), URL: pageURL, Title: docTitle})

			if len(p.searchChunks) == 0 {
				index.Entries = append(index.Entries, SearchEntry{
					Page: page, Title: docTitle, Section: secTitle,
				})
				continue
			}
			for _, chunk := range p.searchChunks {
				itemTitle := chunk.Heading
				if itemTitle == "" {
					itemTitle = docTitle
				}
				index.Entries = append(index.Entries, SearchEntry{
					Page: page, Anchor: chunk.Anchor, Title: itemTitle, Section: secTitle, Content: chunk.Text, Truncated: chunk.Truncated,
				})
			}
		}
	}
	return index
}

// SearchIndexJSON returns the JSON representation of the search index, without
// entry text.
func (r *Renderer) SearchIndexJSON() ([]byte, error) {
	index := r.BuildSearchIndex()
	index.RenderID = agentRenderID(r.AgentPages())
	index.Base = r.urlBase()
	return json.Marshal(index)
}

// SearchContentJSON returns the JSON representation of the entry texts, in the
// same order as the index's entries.
func (r *Renderer) SearchContentJSON() ([]byte, error) {
	content := contentFromIndex(r.BuildSearchIndex())
	content.RenderID = agentRenderID(r.AgentPages())
	return json.Marshal(content)
}

// contentFromIndex collects an index's entry texts into the companion file
// shape, aligned by position with its entries.
func contentFromIndex(index SearchIndex) SearchContent {
	texts := make([]string, len(index.Entries))
	for i, e := range index.Entries {
		texts[i] = e.Content
	}
	return SearchContent{Count: len(index.Entries), Texts: texts}
}

// mergeSearchIndex appends src to dst, rebasing every entry's page index into
// dst's page table so one tree-wide table can hold the pages of many wikis.
func mergeSearchIndex(dst, src *SearchIndex) {
	base := len(dst.Pages)
	dst.Pages = append(dst.Pages, src.Pages...)
	for _, e := range src.Entries {
		e.Page += base
		dst.Entries = append(dst.Entries, e)
	}
}

// labelSearchIndex tags a wiki's page table with the wiki's display title and
// its path under the tree root, so one aggregate index can group rows by wiki.
// It is deliberately separate from BuildSearchIndex: a single bundle's index
// stays unlabelled, and its JSON is unchanged.
func labelSearchIndex(index SearchIndex, site, key string) SearchIndex {
	pages := make([]SearchPage, len(index.Pages))
	for i, p := range index.Pages {
		p.Site = site
		p.SiteKey = key
		if p.ID != "" {
			p.ID = key + ":" + p.ID
		}
		pages[i] = p
	}
	return SearchIndex{Pages: pages, Entries: index.Entries}
}
