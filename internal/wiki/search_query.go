package wiki

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// SearchOptions narrows an agent search. Empty strings mean no filter.
type SearchOptions struct {
	Limit   int
	Site    string
	Tag     string
	Section string
}

// SearchResult is one ranked chunk. The page fields come from pages.json when
// it is present, so a result carries the same ID, Markdown URL and source hash
// an agent would find in the manifest.
type SearchResult struct {
	PageID      string   `json:"pageId,omitempty"`
	Title       string   `json:"title"`
	PageTitle   string   `json:"pageTitle"`
	Section     string   `json:"section,omitempty"`
	Anchor      string   `json:"anchor,omitempty"`
	URL         string   `json:"url"`
	MarkdownURL string   `json:"markdownUrl,omitempty"`
	Site        string   `json:"site,omitempty"`
	SiteKey     string   `json:"siteKey,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Snippet     string   `json:"snippet,omitempty"`
	Score       int      `json:"score"`
	ContentHash string   `json:"contentHash,omitempty"`
}

// SearchResponse is the JSON returned by the CLI and the MCP tool. Total is the
// number of matches after filters, before Limit.
type SearchResponse struct {
	Query   string         `json:"query"`
	Total   int            `json:"total"`
	Results []SearchResult `json:"results"`
}

// SearchCorpus is a rendered site's search files plus its optional page
// manifest. It is loaded once and can answer many queries.
type SearchCorpus struct {
	Index       SearchIndex
	Content     SearchContent
	Manifest    AgentManifest
	SiteDir     string
	Base        string
	RenderID    string
	pageByURL   map[string]AgentPage
	pageByID    map[string]AgentPage
	hasManifest bool
}

// LoadSearchCorpus reads search-index.json, search-content.json and, when
// present, pages.json from a rendered site directory. A content file whose
// count does not match the index is refused rather than attached by position:
// a mismatch would put another render's text on the wrong result.
func LoadSearchCorpus(dir string, bases ...string) (*SearchCorpus, error) {
	read := func(name string) ([]byte, error) {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		return b, nil
	}
	indexRaw, err := read("search-index.json")
	if err != nil {
		return nil, err
	}
	contentRaw, err := read("search-content.json")
	if err != nil {
		return nil, err
	}
	c := &SearchCorpus{SiteDir: dir}
	if len(bases) > 0 {
		c.Base = normalizeCorpusBase(bases[0])
	}
	if err := json.Unmarshal(indexRaw, &c.Index); err != nil {
		return nil, fmt.Errorf("decode search-index.json: %w", err)
	}
	if err := json.Unmarshal(contentRaw, &c.Content); err != nil {
		return nil, fmt.Errorf("decode search-content.json: %w", err)
	}
	if c.Content.Count != len(c.Index.Entries) || len(c.Content.Texts) != len(c.Index.Entries) {
		return nil, fmt.Errorf("search content has %d entries, index has %d", c.Content.Count, len(c.Index.Entries))
	}
	if c.Index.RenderID != "" && c.Content.RenderID != "" && c.Index.RenderID != c.Content.RenderID {
		return nil, fmt.Errorf("search index and content are from different renders")
	}
	c.RenderID = c.Index.RenderID
	c.pageByURL = make(map[string]AgentPage)
	c.pageByID = make(map[string]AgentPage)
	if manifestRaw, err := read("pages.json"); err == nil {
		if err := json.Unmarshal(manifestRaw, &c.Manifest); err != nil {
			return nil, fmt.Errorf("decode pages.json: %w", err)
		}
		c.hasManifest = true
		manifestID := agentRenderID(c.Manifest.Pages)
		if c.RenderID != "" && c.RenderID != manifestID {
			return nil, fmt.Errorf("search index and pages.json are from different renders")
		}
		c.RenderID = manifestID
		for _, p := range c.Manifest.Pages {
			c.pageByURL[p.URL] = p
			c.pageByID[p.ID] = p
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return c, nil
}

// normalizeCorpusBase keeps a URL prefix usable for stripping a rendered page
// URL back to a path under SiteDir.
func normalizeCorpusBase(base string) string {
	if base == "" {
		return ""
	}
	if !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	return base
}

// Search scores the corpus with the same field weights and tie breaks as the
// browser search. It returns exact snippets and a stable page/anchor citation
// for each hit.
func (c *SearchCorpus) Search(query string, opts SearchOptions) (SearchResponse, error) {
	query = strings.TrimSpace(strings.ToLower(query))
	resp := SearchResponse{Query: query, Results: []SearchResult{}}
	if query == "" {
		return resp, nil
	}
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return resp, nil
	}
	if opts.Limit <= 0 {
		opts.Limit = 5
	}
	if opts.Limit > 20 {
		opts.Limit = 20
	}
	if opts.Tag != "" && !c.hasManifest {
		return resp, fmt.Errorf("tag filtering needs pages.json; render the site again")
	}
	if opts.Site != "" && !c.hasSite() {
		return resp, fmt.Errorf("site filtering needs a multi-site aggregate")
	}

	type scored struct {
		entry    SearchEntry
		page     SearchPage
		agent    AgentPage
		score    int
		index    int
		titleLen int
		overview bool
	}
	var matches []scored
	for i, entry := range c.Index.Entries {
		if entry.Page < 0 || entry.Page >= len(c.Index.Pages) {
			continue
		}
		page := c.Index.Pages[entry.Page]
		agent, hasAgent := c.pageByURL[page.URL]
		if !matchesFilters(page, entry, agent, hasAgent, opts) {
			continue
		}
		title := entry.Title
		if title == "" {
			title = page.Title
		}
		titleLower := strings.ToLower(title)
		docLower := strings.ToLower(page.Title)
		sectionLower := strings.ToLower(entry.Section)
		contentLower := strings.ToLower(c.Content.Texts[i])

		score := 0
		all := true
		contentHit := make([]int, len(terms))
		for j, term := range terms {
			matched := false
			if pos := strings.Index(titleLower, term); pos != -1 {
				matched = true
				score += 45
				if titleLower == term || strings.HasPrefix(titleLower, term+" ") {
					score += 40
				}
				switch wordHitKind(titleLower, term) {
				case 2:
					score += 25
				case 1:
					score += 15
				}
			}
			if strings.Contains(docLower, term) {
				matched = true
				score += 25
				switch wordHitKind(docLower, term) {
				case 2:
					score += 12
				case 1:
					score += 8
				}
			}
			if strings.Contains(sectionLower, term) {
				matched = true
				score += 15
			}
			contentHit[j] = strings.Index(contentLower, term)
			if contentHit[j] != -1 {
				matched = true
				score += 10
			}
			if !matched {
				all = false
				break
			}
		}
		if !all {
			continue
		}
		if strings.Contains(titleLower, query) {
			score += 80
		} else if strings.Contains(docLower, query) {
			score += 40
		} else if strings.Contains(contentLower, query) {
			score += 30
		}
		for j, term := range terms {
			if contentHit[j] == -1 {
				continue
			}
			from := contentHit[j] + len(term)
			score += repeatBonus(countOccurrences(contentLower, term, repeatStop, from))
		}
		matches = append(matches, scored{
			entry: entry, page: page, agent: agent, score: score, index: i,
			titleLen: len([]rune(title)), overview: entry.Anchor == "",
		})
	}
	sort.SliceStable(matches, func(i, j int) bool {
		a, b := matches[i], matches[j]
		if a.score != b.score {
			return a.score > b.score
		}
		if a.overview != b.overview {
			return a.overview
		}
		if a.titleLen != b.titleLen {
			return a.titleLen < b.titleLen
		}
		return a.index < b.index
	})
	resp.Total = len(matches)
	if len(matches) > opts.Limit {
		matches = matches[:opts.Limit]
	}
	for _, m := range matches {
		title := m.entry.Title
		if title == "" {
			title = m.page.Title
		}
		result := SearchResult{
			Title:     title,
			PageTitle: m.page.Title,
			Section:   m.entry.Section,
			Anchor:    m.entry.Anchor,
			URL:       m.page.URL,
			Site:      m.page.Site,
			SiteKey:   m.page.SiteKey,
			Snippet:   searchSnippet(c.Content.Texts[m.index], terms),
			Score:     m.score,
		}
		if m.entry.Anchor != "" {
			result.URL += "#" + m.entry.Anchor
		}
		if m.agent.ID != "" {
			result.PageID = m.agent.ID
			result.MarkdownURL = m.agent.MarkdownURL
			result.ContentHash = m.agent.ContentHash
			result.Tags = m.agent.Tags
			if result.Site == "" {
				result.Site = m.agent.Site
			}
			if result.SiteKey == "" {
				result.SiteKey = m.agent.SiteKey
			}
		} else {
			result.MarkdownURL = strings.TrimSuffix(m.page.URL, ".html") + ".md"
		}
		resp.Results = append(resp.Results, result)
	}
	return resp, nil
}

func (c *SearchCorpus) hasSite() bool {
	for _, page := range c.Index.Pages {
		if page.Site != "" || page.SiteKey != "" {
			return true
		}
	}
	return false
}

func matchesFilters(page SearchPage, entry SearchEntry, agent AgentPage, hasAgent bool, opts SearchOptions) bool {
	if opts.Site != "" {
		site := page.Site
		key := page.SiteKey
		if hasAgent {
			site = agent.Site
			key = agent.SiteKey
		}
		if !strings.EqualFold(site, opts.Site) && !strings.EqualFold(key, opts.Site) {
			return false
		}
	}
	if opts.Section != "" && !strings.EqualFold(entry.Section, opts.Section) {
		return false
	}
	if opts.Tag != "" {
		if !hasAgent {
			return false
		}
		found := false
		for _, tag := range agent.Tags {
			if strings.EqualFold(tag, opts.Tag) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// wordHitKind grades the best occurrence of term in a lowercased field: 2 for a
// whole word, 1 for one that starts a word, 0 for one embedded in one.
func wordHitKind(field, term string) int {
	if term == "" {
		return 0
	}
	from := 0
	starts := false
	for from <= len(field)-len(term) {
		pos := strings.Index(field[from:], term)
		if pos == -1 {
			break
		}
		pos += from
		before := false
		if pos > 0 {
			r, _ := utf8.DecodeLastRuneInString(field[:pos])
			before = isWordRune(r)
		}
		after := false
		if end := pos + len(term); end < len(field) {
			r, _ := utf8.DecodeRuneInString(field[end:])
			after = isWordRune(r)
		}
		if !before && !after {
			return 2
		}
		if !before {
			starts = true
		}
		from = pos + 1
	}
	if starts {
		return 1
	}
	return 0
}

const repeatStop = 8

func repeatBonus(count int) int {
	switch {
	case count <= 1:
		return 0
	case count <= 3:
		return 4
	case count <= 7:
		return 7
	default:
		return 9
	}
}

func countOccurrences(field, term string, stop, from int) int {
	count := 1
	for count < stop {
		pos := strings.Index(field[from:], term)
		if pos == -1 {
			break
		}
		count++
		from += pos + len(term)
	}
	return count
}

// searchSnippet returns a plain-text window around the first matching term. It
// is deliberately extractive: no summary or model output is added to a result.
func searchSnippet(content string, terms []string) string {
	if content == "" {
		return ""
	}
	lower := strings.ToLower(content)
	first := -1
	for _, term := range terms {
		if pos := strings.Index(lower, term); pos != -1 && (first == -1 || pos < first) {
			first = pos
		}
	}
	if first == -1 {
		end := len(content)
		if end > 110 {
			end = 110
		}
		for end < len(content) && !utf8.RuneStart(content[end]) {
			end++
		}
		s := content[:end]
		if end < len(content) {
			s += "..."
		}
		return s
	}
	start := first - 35
	if start < 0 {
		start = 0
	}
	for start > 0 && !utf8.RuneStart(content[start]) {
		start--
	}
	end := first + 95
	if end > len(content) {
		end = len(content)
	}
	for end < len(content) && !utf8.RuneStart(content[end]) {
		end++
	}
	s := content[start:end]
	if start > 0 {
		s = "..." + s
	}
	if end < len(content) {
		s += "..."
	}
	return s
}
