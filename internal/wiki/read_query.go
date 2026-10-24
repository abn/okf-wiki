package wiki

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// PageContent is the Markdown source of one page, capped at a token budget.
type PageContent struct {
	PageID          string `json:"pageId"`
	Title           string `json:"title"`
	URL             string `json:"url"`
	MarkdownURL     string `json:"markdownUrl"`
	ContentHash     string `json:"contentHash,omitempty"`
	Content         string `json:"content"`
	EstimatedTokens int    `json:"estimatedTokens"`
	Truncated       bool   `json:"truncated"`
}

// SectionContent is one heading-anchored chunk from the search index. The text
// is plain text, matching what search scored and what the browser can cite.
type SectionContent struct {
	PageID          string `json:"pageId"`
	Title           string `json:"title"`
	PageTitle       string `json:"pageTitle"`
	Section         string `json:"section,omitempty"`
	Anchor          string `json:"anchor"`
	URL             string `json:"url"`
	MarkdownURL     string `json:"markdownUrl,omitempty"`
	ContentHash     string `json:"contentHash,omitempty"`
	Text            string `json:"text"`
	EstimatedTokens int    `json:"estimatedTokens"`
	Truncated       bool   `json:"truncated"`
}

// ContextResult is one section selected for a task-sized context pack.
type ContextResult struct {
	Source          string `json:"source,omitempty"`
	PageID          string `json:"pageId,omitempty"`
	Title           string `json:"title"`
	PageTitle       string `json:"pageTitle"`
	Section         string `json:"section,omitempty"`
	Anchor          string `json:"anchor,omitempty"`
	URL             string `json:"url"`
	MarkdownURL     string `json:"markdownUrl,omitempty"`
	ContentHash     string `json:"contentHash,omitempty"`
	Text            string `json:"text"`
	EstimatedTokens int    `json:"estimatedTokens"`
	Truncated       bool   `json:"truncated"`
}

// ContextResponse is a budgeted set of sections for one query.
type ContextResponse struct {
	Query           string          `json:"query"`
	Total           int             `json:"total"`
	EstimatedTokens int             `json:"estimatedTokens"`
	Truncated       bool            `json:"truncated"`
	Results         []ContextResult `json:"results"`
}

const (
	defaultPageTokens    = 4000
	defaultSectionTokens = 2000
	defaultContextTokens = 3000
	maxReadTokens        = 32000
)

// ReadPage returns the Markdown source of one page. maxTokens is an estimate,
// not an exact tokenizer: four bytes per token is deliberately conservative
// enough for routing and budgeting.
func (c *SearchCorpus) ReadPage(pageID string, maxTokens int) (PageContent, error) {
	page, err := c.page(pageID)
	if err != nil {
		return PageContent{}, err
	}
	rel, err := c.markdownRel(page)
	if err != nil {
		return PageContent{}, err
	}
	raw, err := c.readFile(rel)
	if err != nil {
		return PageContent{}, fmt.Errorf("read page %s: %w", pageID, err)
	}
	if maxTokens <= 0 {
		maxTokens = defaultPageTokens
	}
	if maxTokens > maxReadTokens {
		maxTokens = maxReadTokens
	}
	content, truncated := truncateToTokens(string(raw), maxTokens)
	return PageContent{
		PageID:          page.ID,
		Title:           page.Title,
		URL:             page.URL,
		MarkdownURL:     page.MarkdownURL,
		ContentHash:     page.ContentHash,
		Content:         content,
		EstimatedTokens: estimateTokens(content),
		Truncated:       truncated,
	}, nil
}

// ReadSection returns one heading-anchored text chunk from the search index.
// It does not need to read the Markdown file, so it is the cheapest way to get
// the exact text behind a search result.
func (c *SearchCorpus) ReadSection(pageID, anchor string, maxTokens int) (SectionContent, error) {
	page, err := c.page(pageID)
	if err != nil {
		return SectionContent{}, err
	}
	index, ok := c.entryFor(page.URL, anchor)
	if !ok {
		return SectionContent{}, fmt.Errorf("no section %q on %s", anchor, pageID)
	}
	entry := c.Index.Entries[index]
	if maxTokens <= 0 {
		maxTokens = defaultSectionTokens
	}
	if maxTokens > maxReadTokens {
		maxTokens = maxReadTokens
	}
	text, truncated := truncateToTokens(c.Content.Texts[index], maxTokens)
	title := entry.Title
	if title == "" {
		title = page.Title
	}
	return SectionContent{
		PageID:    page.ID,
		Title:     title,
		PageTitle: page.Title,
		Section:   entry.Section,
		Anchor:    anchor,
		URL: page.URL + func() string {
			if anchor == "" {
				return ""
			}
			return "#" + anchor
		}(),
		MarkdownURL:     page.MarkdownURL,
		ContentHash:     page.ContentHash,
		Text:            text,
		EstimatedTokens: estimateTokens(text),
		Truncated:       truncated || entry.Truncated,
	}, nil
}

// ReadSectionByURL reads a section without a page manifest. It is the fallback
// for an older render where search-index.json exists but pages.json does not.
func (c *SearchCorpus) ReadSectionByURL(pageURL, anchor string, maxTokens int) (SectionContent, error) {
	index, ok := c.entryFor(pageURL, anchor)
	if !ok {
		return SectionContent{}, fmt.Errorf("no section %q on %s", anchor, pageURL)
	}
	entry := c.Index.Entries[index]
	page := SearchPage{URL: pageURL, Title: entry.Title}
	for _, candidate := range c.Index.Pages {
		if candidate.URL == pageURL {
			page = candidate
			break
		}
	}
	title := entry.Title
	if title == "" {
		title = page.Title
	}
	if maxTokens <= 0 {
		maxTokens = defaultSectionTokens
	}
	if maxTokens > maxReadTokens {
		maxTokens = maxReadTokens
	}
	text, truncated := truncateToTokens(c.Content.Texts[index], maxTokens)
	url := pageURL
	if anchor != "" {
		url += "#" + anchor
	}
	return SectionContent{
		PageID:          page.ID,
		Title:           title,
		PageTitle:       page.Title,
		Section:         entry.Section,
		Anchor:          anchor,
		URL:             url,
		Text:            text,
		EstimatedTokens: estimateTokens(text),
		Truncated:       truncated || entry.Truncated,
	}, nil
}

// ContextFor returns ranked sections that fit a token budget. It stops at the
// first result that would cross the budget, so the caller never receives a
// silently incomplete set without the truncated flag.
func (c *SearchCorpus) ContextFor(query string, opts SearchOptions, maxTokens int) (ContextResponse, error) {
	resp := ContextResponse{Query: strings.TrimSpace(strings.ToLower(query)), Results: []ContextResult{}}
	if maxTokens <= 0 {
		maxTokens = defaultContextTokens
	}
	if maxTokens > maxReadTokens {
		maxTokens = maxReadTokens
	}
	if opts.Limit <= 0 {
		opts.Limit = 10
	}
	search, err := c.Search(query, opts)
	if err != nil {
		return resp, err
	}
	resp.Total = search.Total
	remaining := maxTokens
	for _, result := range search.Results {
		if remaining <= 0 {
			resp.Truncated = true
			break
		}
		index, ok := c.entryFor(result.URLWithoutFragment(), result.Anchor)
		if !ok {
			continue
		}
		text, cut := truncateToTokens(c.Content.Texts[index], remaining)
		if text == "" {
			continue
		}
		resp.Results = append(resp.Results, ContextResult{
			PageID:          result.PageID,
			Title:           result.Title,
			PageTitle:       result.PageTitle,
			Section:         result.Section,
			Anchor:          result.Anchor,
			URL:             result.URL,
			MarkdownURL:     result.MarkdownURL,
			ContentHash:     result.ContentHash,
			Text:            text,
			EstimatedTokens: estimateTokens(text),
			Truncated:       cut || c.Index.Entries[index].Truncated,
		})
		remaining -= estimateTokens(text)
		if cut || c.Index.Entries[index].Truncated {
			resp.Truncated = true
		}
		if cut {
			break
		}
	}
	resp.EstimatedTokens = maxTokens - remaining
	return resp, nil
}

func (c *SearchCorpus) page(pageID string) (AgentPage, error) {
	if !c.hasManifest {
		return AgentPage{}, fmt.Errorf("page reading needs pages.json; render the site again")
	}
	page, ok := c.pageByID[pageID]
	if !ok {
		return AgentPage{}, fmt.Errorf("unknown page %q", pageID)
	}
	return page, nil
}

func (c *SearchCorpus) entryFor(pageURL, anchor string) (int, bool) {
	for i, entry := range c.Index.Entries {
		if entry.Page < 0 || entry.Page >= len(c.Index.Pages) {
			continue
		}
		if c.Index.Pages[entry.Page].URL == pageURL && entry.Anchor == anchor {
			return i, true
		}
	}
	return 0, false
}

func (c *SearchCorpus) markdownRel(page AgentPage) (string, error) {
	u := page.MarkdownURL
	if u == "" {
		return "", fmt.Errorf("page %s has no Markdown URL", page.ID)
	}
	if strings.Contains(u, "://") || strings.HasPrefix(u, "//") {
		return "", fmt.Errorf("page %s has an external Markdown URL", page.ID)
	}
	if strings.HasPrefix(u, "/") {
		if c.Base == "" {
			return "", fmt.Errorf("page reading needs the render base")
		}
		if !strings.HasPrefix(u, c.Base) {
			return "", fmt.Errorf("Markdown URL %q is outside the render base %q", u, c.Base)
		}
		u = strings.TrimPrefix(u, c.Base)
	}
	u = strings.TrimPrefix(u, "./")
	clean := filepath.Clean(filepath.FromSlash(u))
	if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("unsafe Markdown path %q", page.MarkdownURL)
	}
	return clean, nil
}

// URLWithoutFragment is the page URL of a search result without its anchor.
func (r SearchResult) URLWithoutFragment() string {
	if i := strings.IndexByte(r.URL, '#'); i >= 0 {
		return r.URL[:i]
	}
	return r.URL
}

func estimateTokens(s string) int {
	return (len(s) + 3) / 4
}

func truncateToTokens(s string, maxTokens int) (string, bool) {
	if maxTokens <= 0 || estimateTokens(s) <= maxTokens {
		return s, false
	}
	const marker = "\n\n[truncated]"
	reserve := estimateTokens(marker)
	limit := maxTokens * 4
	if reserve < maxTokens {
		limit = (maxTokens - reserve) * 4
	}
	if limit >= len(s) {
		return s, false
	}
	cut := s[:limit]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	if i := strings.LastIndexByte(cut, ' '); i > 0 {
		cut = cut[:i]
	}
	if reserve < maxTokens {
		return cut + marker, true
	}
	return cut, true
}
