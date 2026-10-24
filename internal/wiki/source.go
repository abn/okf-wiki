package wiki

import (
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"
)

// SourceReader reads a rendered file relative to one wiki base. A directory
// render and an HTTP source both satisfy it, so the search, graph and read
// clients do not need to know where the bytes came from.
type SourceReader func(name string) ([]byte, error)

// HTTPSource reads a published wiki over HTTP or HTTPS. A bearer token is
// optional and is sent only to this source.
type HTTPSource struct {
	base   *url.URL
	token  string
	client *http.Client
}

// NewHTTPSource validates a base URL and prepares a reader. HTTPS is not
// required: private deployments often use HTTP behind a tailnet or proxy.
func NewHTTPSource(base, token string) (*HTTPSource, error) {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil {
		return nil, fmt.Errorf("parse source URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("source URL must be http or https: %q", base)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("source URL has no host: %q", base)
	}
	if u.User != nil {
		return nil, fmt.Errorf("source URL must not contain credentials; use --source-token")
	}
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	u.RawQuery = ""
	u.Fragment = ""
	return &HTTPSource{
		base:  u,
		token: token,
		client: &http.Client{
			Timeout: 20 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return http.ErrUseLastResponse
				}
				if req.URL.Host != u.Host {
					return fmt.Errorf("source redirect left %s", u.Host)
				}
				if u.Scheme == "https" && req.URL.Scheme == "http" {
					return fmt.Errorf("source redirect downgraded HTTPS to HTTP")
				}
				return nil
			},
		},
	}, nil
}

// BaseURL is the canonical source URL, with no query, fragment or credentials.
func (s *HTTPSource) BaseURL() string {
	return s.base.String()
}

// BasePath is the URL path prefix that corresponds to SearchCorpus.Base.
func (s *HTTPSource) BasePath() string {
	return s.base.Path
}

// Read fetches one published file. A 404 is returned as fs.ErrNotExist so
// optional files such as changes.json and health.json can be skipped.
func (s *HTTPSource) Read(name string) ([]byte, error) {
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "..") {
		return nil, fmt.Errorf("unsafe source path %q", name)
	}
	u := *s.base
	u.Path = path.Join(s.base.Path, name)
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: %s", fs.ErrNotExist, name)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("source returned HTTP %d for %s", resp.StatusCode, name)
	}
	const maxSourceBytes = 64 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSourceBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxSourceBytes {
		return nil, fmt.Errorf("source file %s exceeds the 64 MiB limit", name)
	}
	return body, nil
}

// AgentSource is one loaded wiki, local or remote, with its published files.
type AgentSource struct {
	Name       string
	Base       string
	Corpus     *SearchCorpus
	Graph      *GraphCorpus
	Changes    AgentChanges
	HasChanges bool
	ChangesErr string
	Health     GraphHealth
	HasHealth  bool
	HealthErr  string
}

// LoadAgentSource loads the published agent surface from one reader.
func LoadAgentSource(name, base string, read SourceReader) (*AgentSource, error) {
	corpus, err := LoadSearchCorpusFrom(read, base)
	if err != nil {
		return nil, fmt.Errorf("%s: search surface: %w", name, err)
	}
	graph, err := LoadGraphCorpusFrom(read)
	if err != nil {
		return nil, fmt.Errorf("%s: graph surface: %w", name, err)
	}
	if corpus.RenderID != "" && graph.RenderID != "" && corpus.RenderID != graph.RenderID {
		return nil, fmt.Errorf("%s: search and graph are from different renders", name)
	}
	src := &AgentSource{Name: name, Base: base, Corpus: corpus, Graph: graph}
	if changes, err := LoadAgentChangesFrom(read); err == nil {
		src.Changes = changes
		src.HasChanges = true
		if changes.RenderID != "" && corpus.RenderID != "" && changes.RenderID != corpus.RenderID {
			src.HasChanges = false
			src.ChangesErr = "changes.json is from another render"
		}
	} else {
		src.ChangesErr = err.Error()
	}
	if health, err := LoadGraphHealthFrom(read); err == nil {
		src.Health = health
		src.HasHealth = true
		if health.RenderID != "" && corpus.RenderID != "" && health.RenderID != corpus.RenderID {
			src.HasHealth = false
			src.HealthErr = "health.json is from another render"
		}
	} else {
		src.HealthErr = err.Error()
	}
	return src, nil
}

// Namespace prefixes every ID with the source name. Federated tools use this
// when more than one source is configured; the single-source surface keeps its
// historical unprefixed IDs.
func (s *AgentSource) Namespace(prefix string) {
	if prefix == "" {
		return
	}
	c := s.Corpus
	for i := range c.Manifest.Pages {
		c.Manifest.Pages[i].ID = prefix + ":" + c.Manifest.Pages[i].ID
	}
	for i := range c.Index.Pages {
		if c.Index.Pages[i].ID != "" {
			c.Index.Pages[i].ID = prefix + ":" + c.Index.Pages[i].ID
		}
	}
	c.pageByID = make(map[string]AgentPage, len(c.Manifest.Pages))
	c.pageByURL = make(map[string]AgentPage, len(c.Manifest.Pages))
	for _, p := range c.Manifest.Pages {
		c.pageByID[p.ID] = p
		c.pageByURL[p.URL] = p
	}
	s.Graph.Namespace(prefix)
	for i := range s.Changes.Changes {
		s.Changes.Changes[i].ID = prefix + ":" + s.Changes.Changes[i].ID
	}
	for i := range s.Health.BrokenLinks {
		s.Health.BrokenLinks[i].From = prefix + ":" + s.Health.BrokenLinks[i].From
		s.Health.BrokenLinks[i].To = prefix + ":" + s.Health.BrokenLinks[i].To
	}
	for i := range s.Health.Orphans {
		s.Health.Orphans[i].ID = prefix + ":" + s.Health.Orphans[i].ID
	}
	for i := range s.Health.TagOnly {
		s.Health.TagOnly[i].ID = prefix + ":" + s.Health.TagOnly[i].ID
	}
	for i := range s.Health.Stale {
		s.Health.Stale[i].ID = prefix + ":" + s.Health.Stale[i].ID
	}
	for i := range s.Health.MostConnected {
		s.Health.MostConnected[i].ID = prefix + ":" + s.Health.MostConnected[i].ID
	}
	for i := range s.Health.Cycles {
		for j := range s.Health.Cycles[i] {
			s.Health.Cycles[i][j] = prefix + ":" + s.Health.Cycles[i][j]
		}
	}
}

// Federator queries a set of loaded sources.
type Federator struct {
	Sources []*AgentSource
	byName  map[string]*AgentSource
}

func NewFederator(sources []*AgentSource) *Federator {
	if len(sources) > 1 {
		for _, s := range sources {
			s.Namespace(s.Name)
		}
	}
	f := &Federator{Sources: sources, byName: make(map[string]*AgentSource, len(sources))}
	for _, s := range sources {
		f.byName[s.Name] = s
	}
	return f
}

// inferPageBase finds the URL prefix shared by the page table. A wiki mounted
// under /b/wiki/ can still have page URLs rooted at /wiki/, so the source URL
// path and the render base must not be assumed to be the same.
func inferPageBase(pages []SearchPage) string {
	base := ""
	for _, page := range pages {
		u := page.URL
		if i := strings.IndexAny(u, "?#"); i >= 0 {
			u = u[:i]
		}
		dir := path.Dir(u)
		prefix := dir + "/"
		if dir == "." {
			prefix = "./"
		} else if dir == "/" {
			prefix = "/"
		}
		if base == "" {
			base = prefix
			continue
		}
		i := 0
		for i < len(base) && i < len(prefix) && base[i] == prefix[i] {
			i++
		}
		base = base[:i]
		if cut := strings.LastIndexByte(base, '/'); cut >= 0 {
			base = base[:cut+1]
		}
	}
	return base
}

func resolveSourceURL(base, renderBase, raw string) string {
	if raw == "" || strings.Contains(raw, "://") {
		return raw
	}
	// Local sources already carry the URL mode the renderer chose, including
	// relative URLs for an offline folder.
	if !strings.Contains(base, "://") {
		return raw
	}
	if renderBase != "" && strings.HasPrefix(raw, renderBase) {
		raw = strings.TrimPrefix(raw, renderBase)
	}
	raw = strings.TrimPrefix(raw, "/")
	b, err := url.Parse(base)
	if err != nil {
		return raw
	}
	r, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return b.ResolveReference(r).String()
}

func (f *Federator) Source(name string) (*AgentSource, error) {
	if len(f.Sources) == 1 && name == "" {
		return f.Sources[0], nil
	}
	if name == "" {
		return nil, fmt.Errorf("source is required when more than one wiki is configured")
	}
	s, ok := f.byName[name]
	if !ok {
		return nil, fmt.Errorf("unknown source %q", name)
	}
	return s, nil
}

func (f *Federator) selected(names []string) ([]*AgentSource, error) {
	if len(names) == 0 {
		return f.Sources, nil
	}
	out := make([]*AgentSource, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		s, err := f.Source(name)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// Search queries every selected source and returns one globally ranked list.
func (f *Federator) Search(query string, opts SearchOptions, sources []string) (SearchResponse, error) {
	selected, err := f.selected(sources)
	if err != nil {
		return SearchResponse{}, err
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}
	opts.Limit = 20
	if len(selected) == 1 {
		src := selected[0]
		resp, err := src.Corpus.Search(query, opts)
		if err != nil {
			return SearchResponse{}, fmt.Errorf("%s: %w", src.Name, err)
		}
		for i := range resp.Results {
			resp.Results[i].Source = src.Name
			resp.Results[i].rawURL = resp.Results[i].URLWithoutFragment()
			resp.Results[i].URL = resolveSourceURL(src.Base, src.Corpus.Base, resp.Results[i].URL)
			resp.Results[i].MarkdownURL = resolveSourceURL(src.Base, src.Corpus.Base, resp.Results[i].MarkdownURL)
		}
		if len(resp.Results) > limit {
			resp.Results = resp.Results[:limit]
		}
		return resp, nil
	}
	all := []SearchResult{}
	total := 0
	errs := []string{}
	for _, src := range selected {
		resp, err := src.Corpus.Search(query, opts)
		if err != nil {
			errs = append(errs, src.Name+": "+err.Error())
			continue
		}
		total += resp.Total
		for _, result := range resp.Results {
			result.Source = src.Name
			result.rawURL = result.URLWithoutFragment()
			result.URL = resolveSourceURL(src.Base, src.Corpus.Base, result.URL)
			result.MarkdownURL = resolveSourceURL(src.Base, src.Corpus.Base, result.MarkdownURL)
			all = append(all, result)
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Score != all[j].Score {
			return all[i].Score > all[j].Score
		}
		if all[i].Source != all[j].Source {
			return all[i].Source < all[j].Source
		}
		if all[i].PageID != all[j].PageID {
			return all[i].PageID < all[j].PageID
		}
		return all[i].Anchor < all[j].Anchor
	})
	if len(all) > limit {
		all = all[:limit]
	}
	return SearchResponse{Query: strings.TrimSpace(strings.ToLower(query)), Total: total, Results: all, Errors: errs}, nil
}

// ContextFor merges the best sections from the selected sources into one
// budget. The same extractive text and truncation rules apply per source.
func (f *Federator) ContextFor(query string, opts SearchOptions, maxTokens int, sources []string) (ContextResponse, error) {
	resp, err := f.Search(query, opts, sources)
	if err != nil {
		return ContextResponse{}, err
	}
	if maxTokens <= 0 {
		maxTokens = defaultContextTokens
	}
	if maxTokens > maxReadTokens {
		maxTokens = maxReadTokens
	}
	out := ContextResponse{Query: resp.Query, Total: resp.Total, Results: []ContextResult{}}
	remaining := maxTokens
	for _, result := range resp.Results {
		if remaining <= 0 {
			out.Truncated = true
			break
		}
		src, err := f.Source(result.Source)
		if err != nil {
			return out, err
		}
		section, err := src.Corpus.ReadSection(result.PageID, result.Anchor, remaining)
		if err != nil {
			section, err = src.Corpus.ReadSectionByURL(result.rawURL, result.Anchor, remaining)
		}
		if err != nil {
			continue
		}
		out.Results = append(out.Results, ContextResult{
			Source:          result.Source,
			PageID:          section.PageID,
			Title:           section.Title,
			PageTitle:       section.PageTitle,
			Section:         section.Section,
			Anchor:          section.Anchor,
			URL:             resolveSourceURL(src.Base, src.Corpus.Base, section.URL),
			MarkdownURL:     resolveSourceURL(src.Base, src.Corpus.Base, section.MarkdownURL),
			ContentHash:     section.ContentHash,
			Text:            section.Text,
			EstimatedTokens: section.EstimatedTokens,
			Truncated:       section.Truncated,
		})
		remaining -= section.EstimatedTokens
		if section.Truncated {
			out.Truncated = true
		}
		if remaining <= 0 {
			out.Truncated = true
			break
		}
	}
	out.EstimatedTokens = maxTokens - remaining
	return out, nil
}

// SourceInfo is the public description of one configured source.
type SourceInfo struct {
	Name       string `json:"name"`
	Base       string `json:"base"`
	RenderID   string `json:"renderId,omitempty"`
	HasChanges bool   `json:"hasChanges"`
	HasHealth  bool   `json:"hasHealth"`
}

func (f *Federator) Info() []SourceInfo {
	out := make([]SourceInfo, 0, len(f.Sources))
	for _, s := range f.Sources {
		out = append(out, SourceInfo{
			Name: s.Name, Base: s.Base, RenderID: s.Corpus.RenderID,
			HasChanges: s.HasChanges, HasHealth: s.HasHealth,
		})
	}
	return out
}

func (f *Federator) ReadPage(source, pageID string, maxTokens int) (PageContent, error) {
	s, err := f.Source(source)
	if err != nil {
		return PageContent{}, err
	}
	page, err := s.Corpus.ReadPage(pageID, maxTokens)
	if err != nil {
		return PageContent{}, err
	}
	page.URL = resolveSourceURL(s.Base, s.Corpus.Base, page.URL)
	page.MarkdownURL = resolveSourceURL(s.Base, s.Corpus.Base, page.MarkdownURL)
	return page, nil
}

func (f *Federator) ReadSection(source, pageID, anchor string, maxTokens int) (SectionContent, error) {
	s, err := f.Source(source)
	if err != nil {
		return SectionContent{}, err
	}
	section, err := s.Corpus.ReadSection(pageID, anchor, maxTokens)
	if err != nil {
		return SectionContent{}, err
	}
	section.URL = resolveSourceURL(s.Base, s.Corpus.Base, section.URL)
	section.MarkdownURL = resolveSourceURL(s.Base, s.Corpus.Base, section.MarkdownURL)
	return section, nil
}

func (f *Federator) Neighbors(source, pageID string, depth int, kinds []string) (NeighborsResponse, error) {
	s, err := f.Source(source)
	if err != nil {
		return NeighborsResponse{}, err
	}
	resp, err := s.Graph.Neighbors(pageID, depth, kinds)
	if err != nil {
		return NeighborsResponse{}, err
	}
	resp.Center.URL = resolveSourceURL(s.Base, s.Corpus.Base, resp.Center.URL)
	for i := range resp.Nodes {
		resp.Nodes[i].URL = resolveSourceURL(s.Base, s.Corpus.Base, resp.Nodes[i].URL)
	}
	return resp, nil
}

func (f *Federator) Path(source, from, to string, maxDepth int, kinds []string) (PathResponse, error) {
	s, err := f.Source(source)
	if err != nil {
		return PathResponse{}, err
	}
	resp, err := s.Graph.Path(from, to, maxDepth, kinds)
	if err != nil {
		return PathResponse{}, err
	}
	resp.From.URL = resolveSourceURL(s.Base, s.Corpus.Base, resp.From.URL)
	resp.To.URL = resolveSourceURL(s.Base, s.Corpus.Base, resp.To.URL)
	for i := range resp.Nodes {
		resp.Nodes[i].URL = resolveSourceURL(s.Base, s.Corpus.Base, resp.Nodes[i].URL)
	}
	return resp, nil
}

func (f *Federator) Changes(source string) (AgentChanges, error) {
	s, err := f.Source(source)
	if err != nil {
		return AgentChanges{}, err
	}
	if !s.HasChanges {
		if s.ChangesErr != "" {
			return AgentChanges{}, fmt.Errorf("%s: %s", s.Name, s.ChangesErr)
		}
		return AgentChanges{}, fmt.Errorf("%s: changes.json is unavailable", s.Name)
	}
	return s.Changes, nil
}

func (f *Federator) Health(source string) (GraphHealth, error) {
	s, err := f.Source(source)
	if err != nil {
		return GraphHealth{}, err
	}
	if !s.HasHealth {
		if s.HealthErr != "" {
			return GraphHealth{}, fmt.Errorf("%s: %s", s.Name, s.HealthErr)
		}
		return GraphHealth{}, fmt.Errorf("%s: health.json is unavailable", s.Name)
	}
	return s.Health, nil
}
