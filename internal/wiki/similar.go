package wiki

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"
)

// SimilarDefaultMinScore is the coverage below which a draft is treated as a
// new subject rather than an update. It is a starting point, not a fact: it was
// set from a run over one deep-dive bundle, and a bundle with a different page
// convention should re-derive it.
const SimilarDefaultMinScore = 0.16

// SimilarMaxBytes bounds the draft that is read. A page draft is a few thousand
// words; pasted source is cut before tokenising, so a large paste costs the same
// as a page.
const SimilarMaxBytes = 256 << 10

// SimilarMaxTokens caps the distinct words that are ranked, after the byte cut.
const SimilarMaxTokens = 2000

// subjectRoleWords are file names that name a role inside a subject directory
// rather than a subject of their own. They are how a deep dive's overview,
// report and evidence pages collapse to one candidate.
var subjectRoleWords = map[string]bool{
	"index": true, "overview": true, "report": true, "evidence": true,
	"abstract": true, "method": true, "implications": true,
	"implementations": true, "benchmark": true, "claims": true,
	"limits": true, "risks": true, "setup": true, "usage": true,
	"comparison": true, "discovery": true, "history": true,
	"quirks": true, "integration": true, "capabilities": true,
	"architecture": true,
}

var similarStopWords = map[string]bool{
	"a": true, "about": true, "after": true, "against": true, "all": true,
	"also": true, "an": true, "and": true, "any": true, "are": true,
	"as": true, "at": true, "be": true, "because": true, "before": true,
	"between": true, "both": true, "but": true, "by": true, "can": true,
	"during": true, "each": true, "every": true, "for": true, "from": true,
	"has": true, "have": true, "how": true, "in": true, "into": true,
	"is": true, "it": true, "its": true, "may": true, "more": true,
	"most": true, "not": true, "of": true, "on": true, "one": true,
	"only": true, "or": true, "other": true, "over": true, "rather": true,
	"so": true, "some": true, "such": true, "than": true, "that": true,
	"the": true, "their": true, "then": true, "there": true, "these": true,
	"they": true, "this": true, "three": true, "through": true, "to": true,
	"two": true, "under": true, "was": true, "were": true, "what": true,
	"when": true, "where": true, "which": true, "while": true, "who": true,
	"why": true, "will": true, "with": true, "within": true, "without": true,
	"would": true, "you": true, "your": true,
}

// Field weights for one page's terms. A title match means more than a heading
// match, and a tag is an explicit statement of subject. The weights matter less
// than their order; the coverage keeps the result comparable across drafts.
const (
	similarWeightTitle       = 3.0
	similarWeightDescription = 2.0
	similarWeightTag         = 2.0
	similarWeightHeading     = 1.5
	similarWeightBody        = 1.0
	similarWeightSlug        = 1.0
)

// SimilarOptions shapes a page-planning query.
type SimilarOptions struct {
	// Limit is the number of subject candidates to return, 1 through 20.
	Limit int
	// MinScore overrides SimilarDefaultMinScore when it is above zero.
	MinScore float64
}

// SimilarPage is one page of a candidate subject. A caller that decides to
// update the subject reads these rather than searching again.
type SimilarPage struct {
	PageID      string  `json:"pageId"`
	Role        string  `json:"role,omitempty"`
	Title       string  `json:"title"`
	URL         string  `json:"url"`
	MarkdownURL string  `json:"markdownUrl,omitempty"`
	ContentHash string  `json:"contentHash,omitempty"`
	Score       float64 `json:"score"`
}

// SimilarCandidate is one subject the draft may belong to. Score is the best
// page's coverage of the draft's words, so a subject with one strong match
// outranks a subject with several weak ones. Site is set in a tree or a
// federation, where the same subject path can exist in two wikis.
type SimilarCandidate struct {
	Source  string        `json:"source,omitempty"`
	Site    string        `json:"site,omitempty"`
	Subject string        `json:"subject"`
	Title   string        `json:"title"`
	Section string        `json:"section,omitempty"`
	Score   float64       `json:"score"`
	Matched []string      `json:"matched,omitempty"`
	Pages   []SimilarPage `json:"pages"`
}

// SimilarResponse answers the question the planner asks: does this draft belong
// to a page that already exists, and if so which one.
//
// Verdict is "update" when the best candidate meets the threshold and
// "new_subject" otherwise. Total is the number of subjects with any match,
// before Limit; a federated answer sums it across the sources that read.
type SimilarResponse struct {
	Verdict    string             `json:"verdict"`
	Threshold  float64            `json:"threshold"`
	Total      int                `json:"total"`
	Tokens     int                `json:"tokens"`
	Truncated  bool               `json:"truncated,omitempty"`
	Candidates []SimilarCandidate `json:"candidates"`
	Errors     []string           `json:"errors,omitempty"`
}

// Similar plans a capture against an already rendered wiki. The corpus is the
// same one search and context packs read, so a suggestion names the exact page
// an agent would otherwise have found by reading the index by hand.
func (c *SearchCorpus) Similar(text string, opts SimilarOptions) (SimilarResponse, error) {
	if !c.hasManifest || len(c.Manifest.Pages) == 0 {
		return SimilarResponse{}, fmt.Errorf("no pages.json at this source; render it before planning against it")
	}
	return SimilarPagesWithText(c.Manifest.Pages, c.pageBodies(), text, opts), nil
}

// pageBodies joins the search catalogue's chunks back into one text per page.
// The CLI builds the same map from the renderer's chunks, so a bundle and its
// render rank the same words.
func (c *SearchCorpus) pageBodies() map[string]string {
	out := make(map[string]string, len(c.Index.Pages))
	for i, entry := range c.Index.Entries {
		if i >= len(c.Content.Texts) {
			break
		}
		if entry.Page < 0 || entry.Page >= len(c.Index.Pages) {
			continue
		}
		id := c.Index.Pages[entry.Page].ID
		if id == "" {
			continue
		}
		if out[id] != "" {
			out[id] += " "
		}
		out[id] += c.Content.Texts[i]
	}
	return out
}

// PageBodies returns each page's indexed text, keyed by page ID, from the chunks
// the search catalogue is built from. A planner on the bundle and a planner on
// the render therefore see the same text, including a page's truncated tail.
func (r *Renderer) PageBodies() map[string]string {
	out := make(map[string]string, len(r.sections))
	for _, section := range r.sections {
		for _, p := range section.Pages {
			if p.Synthetic {
				continue
			}
			var b strings.Builder
			for _, chunk := range p.searchChunks {
				if chunk.Text == "" {
					continue
				}
				if b.Len() > 0 {
					b.WriteByte(' ')
				}
				b.WriteString(chunk.Text)
			}
			out[pageNodeID(p.Slug)] = b.String()
		}
	}
	return out
}

// SimilarPages ranks existing pages against a draft and collapses the result by
// subject.
func SimilarPages(pages []AgentPage, text string, opts SimilarOptions) SimilarResponse {
	return SimilarPagesWithText(pages, nil, text, opts)
}

// SimilarPagesWithText is SimilarPages with each page's body text, keyed by page
// ID. A page missing from bodies is ranked on its metadata alone.
func SimilarPagesWithText(pages []AgentPage, bodies map[string]string, text string, opts SimilarOptions) SimilarResponse {
	limit := opts.Limit
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}
	threshold := opts.MinScore
	if threshold <= 0 {
		threshold = SimilarDefaultMinScore
	}
	terms, truncated := similarTokens(text)
	resp := SimilarResponse{
		Verdict:    "new_subject",
		Threshold:  threshold,
		Tokens:     len(terms),
		Truncated:  truncated,
		Candidates: []SimilarCandidate{},
	}
	if len(terms) == 0 || len(pages) == 0 {
		return resp
	}

	docs := make([]similarDoc, 0, len(pages))
	df := make(map[string]int, len(terms))
	for _, p := range pages {
		if !similarCandidatePage(p) {
			continue
		}
		d := newSimilarDoc(p, bodies[p.ID])
		docs = append(docs, d)
		for _, t := range terms {
			if d.terms[t] > 0 {
				df[t]++
			}
		}
	}
	if len(docs) == 0 {
		return resp
	}

	// A word the bundle has never seen is not free: it takes the highest idf and
	// contributes nothing, so a draft that shares one word out of ten scores low
	// instead of perfect.
	unseen := math.Log(1 + (float64(len(docs))+0.5)/0.5)
	idf := make(map[string]float64, len(terms))
	denom := 0.0
	for _, t := range terms {
		v := unseen
		if df[t] > 0 {
			v = math.Log(1 + (float64(len(docs))-float64(df[t])+0.5)/(float64(df[t])+0.5))
		}
		idf[t] = v
		denom += v * similarWeightTitle
	}

	groups := map[string]*similarGroup{}
	for _, d := range docs {
		score := 0.0
		for _, t := range terms {
			score += idf[t] * d.terms[t]
		}
		if score == 0 {
			continue
		}
		score /= denom
		key := d.site + "\x00" + d.subject
		g := groups[key]
		if g == nil {
			g = &similarGroup{site: d.site, subject: d.subject, terms: map[string]bool{}}
			groups[key] = g
		}
		g.pages = append(g.pages, similarScored{page: d.page, role: d.role, score: score})
		if score > g.score {
			g.score = score
		}
		for _, t := range terms {
			if d.terms[t] > 0 {
				g.terms[t] = true
			}
		}
	}

	out := make([]SimilarCandidate, 0, len(groups))
	for _, g := range groups {
		if g.score <= 0 {
			continue
		}
		out = append(out, g.candidate(idf))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].Site != out[j].Site {
			return out[i].Site < out[j].Site
		}
		if out[i].Subject != out[j].Subject {
			return out[i].Subject < out[j].Subject
		}
		return out[i].Title < out[j].Title
	})
	resp.Total = len(out)
	if len(out) > limit {
		out = out[:limit]
	}
	resp.Candidates = out
	if len(out) > 0 && out[0].Score >= threshold {
		resp.Verdict = "update"
	}
	return resp
}

type similarDoc struct {
	page    AgentPage
	site    string
	subject string
	role    string
	terms   map[string]float64
}

type similarScored struct {
	page  AgentPage
	role  string
	score float64
}

type similarGroup struct {
	site    string
	subject string
	score   float64
	terms   map[string]bool
	pages   []similarScored
}

// candidate collapses one subject's pages into the answer a caller acts on.
// The page list is ordered overview first, then by score, so the first entry is
// the page to read before editing.
func (g *similarGroup) candidate(idf map[string]float64) SimilarCandidate {
	sort.SliceStable(g.pages, func(i, j int) bool {
		left, right := g.pages[i].role == "overview", g.pages[j].role == "overview"
		if left != right {
			return left
		}
		if g.pages[i].score != g.pages[j].score {
			return g.pages[i].score > g.pages[j].score
		}
		return g.pages[i].page.ID < g.pages[j].page.ID
	})
	matched := make([]string, 0, len(g.terms))
	for t := range g.terms {
		matched = append(matched, t)
	}
	sort.SliceStable(matched, func(i, j int) bool {
		if idf[matched[i]] != idf[matched[j]] {
			return idf[matched[i]] > idf[matched[j]]
		}
		return matched[i] < matched[j]
	})
	if len(matched) > 8 {
		matched = matched[:8]
	}
	title := g.pages[0].page.Title
	for _, p := range g.pages {
		if p.role == "overview" {
			title = p.page.Title
			break
		}
	}
	pages := make([]SimilarPage, 0, len(g.pages))
	for _, p := range g.pages {
		pages = append(pages, SimilarPage{
			PageID:      p.page.ID,
			Role:        p.role,
			Title:       p.page.Title,
			URL:         p.page.URL,
			MarkdownURL: p.page.MarkdownURL,
			ContentHash: p.page.ContentHash,
			Score:       round3(p.score),
		})
	}
	return SimilarCandidate{
		Site:    g.site,
		Subject: g.subject,
		Title:   title,
		Section: g.pages[0].page.Section,
		Score:   round3(g.score),
		Matched: matched,
		Pages:   pages,
	}
}

func newSimilarDoc(p AgentPage, body string) similarDoc {
	doc := similarDoc{page: p, terms: map[string]float64{}}
	doc.site, doc.subject, doc.role = similarPageKey(p)
	add := func(words []string, weight float64) {
		for _, t := range words {
			if doc.terms[t] < weight {
				doc.terms[t] = weight
			}
		}
	}
	add(similarWords(p.Title), similarWeightTitle)
	add(similarWords(p.Description), similarWeightDescription)
	add(similarWords(strings.Join(p.Tags, " ")), similarWeightTag)
	add(similarWords(doc.subject), similarWeightSlug)
	for _, h := range p.Headings {
		add(similarWords(h.Title), similarWeightHeading)
	}
	add(similarWords(body), similarWeightBody)
	return doc
}

// similarCandidatePage keeps chronological records out of the rank. A log entry
// repeats the words of every subject it touched, so it matches almost any draft
// and would sit above the page the draft actually belongs to.
func similarCandidatePage(p AgentPage) bool {
	if strings.EqualFold(p.Type, "Log Entry") {
		return false
	}
	slug := similarSlug(p.ID)
	return slug != "log" && !strings.HasPrefix(slug, "logs/") && !strings.HasPrefix(slug, "log/")
}

// similarPageKey separates a page's site, subject and role. Site comes from the
// tree namespace the aggregate records, so two wikis with a page at the same path
// stay two candidates. A federated source is identified by the candidate's
// Source field instead, because each source is ranked on its own.
func similarPageKey(p AgentPage) (site, subject, role string) {
	slug := similarSlug(p.ID)
	site = p.SiteKey
	dir, base := "", slug
	if i := strings.LastIndex(slug, "/"); i >= 0 {
		dir, base = slug[:i], slug[i+1:]
	}
	if dir != "" && subjectRoleWords[base] {
		return site, dir, base
	}
	return site, slug, ""
}

func similarSlug(pageID string) string {
	if i := strings.LastIndex(pageID, "page:"); i >= 0 {
		return pageID[i+len("page:"):]
	}
	return pageID
}

func similarTokens(text string) ([]string, bool) {
	truncated := len(text) > SimilarMaxBytes
	if truncated {
		text = text[:SimilarMaxBytes]
	}
	all := similarWords(text)
	seen := make(map[string]bool, len(all))
	out := make([]string, 0, len(all))
	for _, t := range all {
		if seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	if len(out) > SimilarMaxTokens {
		return out[:SimilarMaxTokens], true
	}
	return out, truncated
}

// similarWords lowercases on rune boundaries and keeps letters and digits
// together, so "FTS5" survives as one token. A single letter is dropped, which is
// why "Q4_K_M" becomes q4.
func similarWords(text string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		w := strings.ToLower(b.String())
		b.Reset()
		if len([]rune(w)) < 2 || similarStopWords[w] || similarNumber(w) {
			return
		}
		out = append(out, similarStem(w))
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

func round3(v float64) float64 {
	return math.Round(v*1000) / 1000
}

// similarNumber reports whether a token carries no letter. A year or a count is
// a fact about one page, not a subject, and it should not decide a match.
func similarNumber(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

// similarStem folds plurals, the -ing and -ed endings, and a trailing e that
// separates "cache" from "caches". It is deliberately not a full stemmer: an
// over-eager stem turns two subjects into one, and a wrong merge is worse than a
// missed match. Both sides of a comparison run the same rules, so consistency
// matters more than the dictionary form.
//
// Irregular plurals are not folded: "bus"/"buses" and "gas"/"gases" add -es
// where "case" adds -s, and no suffix rule separates them without a dictionary.
// The sibilant cases that do fold are -ches, -shes, -xes and -zes.
func similarStem(w string) string {
	w = similarPlural(w)
	if len(w) > 5 && strings.HasSuffix(w, "ing") {
		w = w[:len(w)-3]
	}
	if len(w) > 4 && strings.HasSuffix(w, "ed") {
		w = w[:len(w)-2]
	}
	if len(w) > 4 && strings.HasSuffix(w, "e") {
		w = w[:len(w)-1]
	}
	if len(w) > 3 && strings.HasSuffix(w, "y") {
		w = w[:len(w)-1] + "i"
	}
	return w
}

func similarPlural(w string) string {
	switch {
	case len(w) > 4 && strings.HasSuffix(w, "ies"):
		return w[:len(w)-3] + "y"
	case len(w) > 4 && strings.HasSuffix(w, "xes"),
		len(w) > 4 && strings.HasSuffix(w, "zes"),
		len(w) > 4 && strings.HasSuffix(w, "ches"),
		len(w) > 4 && strings.HasSuffix(w, "shes"):
		return w[:len(w)-2]
	case len(w) > 3 && strings.HasSuffix(w, "s") &&
		!strings.HasSuffix(w, "ss") && !strings.HasSuffix(w, "us") && !strings.HasSuffix(w, "is"):
		return w[:len(w)-1]
	}
	return w
}
