package wiki

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func similarFixture() []AgentPage {
	return []AgentPage{
		{
			ID: "page:methods/llm-wiki/overview", Title: "LLM Wiki",
			Description: "Karpathy's pattern for a wiki an LLM writes and maintains",
			Section:     "Methods", Tags: []string{"knowledge-management", "wiki", "agents"},
			URL: "/wiki/methods/llm-wiki/overview.html",
			Headings: []AgentHeading{
				{ID: "what-it-is", Title: "What it is"},
				{ID: "verdict", Title: "Verdict"},
			},
		},
		{
			ID: "page:methods/llm-wiki/report", Title: "LLM Wiki report",
			Description: "How the ingest, query and lint loop works",
			Section:     "Methods",
			URL:         "/wiki/methods/llm-wiki/report.html",
			Headings: []AgentHeading{
				{ID: "the-loop", Title: "The loop"},
				{ID: "where-the-pattern-breaks", Title: "Where the pattern breaks"},
			},
		},
		{
			ID: "page:methods/llm-wiki/evidence", Title: "LLM Wiki evidence",
			Description: "The claims behind the LLM Wiki assessment",
			Section:     "Methods",
			URL:         "/wiki/methods/llm-wiki/evidence.html",
		},
		{
			ID: "page:methods/claim-grounding/overview", Title: "Claim grounding",
			Description: "Verify a claim against a primary source before it is adopted",
			Section:     "Methods", Tags: []string{"research", "evidence"},
			URL: "/wiki/methods/claim-grounding/overview.html",
		},
		{
			ID: "page:usage/container", Title: "Container",
			Description: "Run the image with a mounted content directory",
			Section:     "Usage", Tags: []string{"usage", "container", "podman"},
			URL:      "/wiki/usage/container.html",
			Headings: []AgentHeading{{ID: "mount-the-content", Title: "Mount the content"}},
		},
		{
			ID: "page:usage/cli", Title: "Command line",
			Description: "Running the binary, with render and serve",
			Section:     "Usage", Tags: []string{"usage", "cli"},
			URL: "/wiki/usage/cli.html",
		},
		{
			ID: "page:design/rendering", Title: "Rendering pipeline",
			Description: "How a bundle of Markdown becomes a served wiki",
			Section:     "Design", Tags: []string{"design", "rendering"},
			URL: "/wiki/design/rendering.html",
		},
	}
}

func TestSimilarRanksSubjectAndCollapsesRoles(t *testing.T) {
	draft := `Karpathy's LLM wiki compiles sources into a persistent Markdown wiki. ` +
		`The loop is ingest, query and lint. Near-duplicate pages are the default failure.`
	resp := SimilarPages(similarFixture(), draft, SimilarOptions{Limit: 5})

	if resp.Verdict != "update" {
		t.Fatalf("verdict = %q, want update", resp.Verdict)
	}
	if len(resp.Candidates) == 0 {
		t.Fatal("no candidates")
	}
	top := resp.Candidates[0]
	if top.Subject != "methods/llm-wiki" {
		t.Fatalf("top subject = %q, want methods/llm-wiki", top.Subject)
	}
	if top.Title != "LLM Wiki" {
		t.Fatalf("top title = %q, want the overview title", top.Title)
	}
	if len(top.Pages) != 3 {
		t.Fatalf("top candidate holds %d pages, want 3 collapsed into one subject", len(top.Pages))
	}
	if top.Pages[0].Role != "overview" {
		t.Fatalf("first page role = %q, want overview first", top.Pages[0].Role)
	}
}

func TestSimilarPicksTheOperationalPage(t *testing.T) {
	draft := `Run the image with podman and mount the content directory at the container path.`
	resp := SimilarPages(similarFixture(), draft, SimilarOptions{})

	if len(resp.Candidates) == 0 {
		t.Fatal("no candidates")
	}
	if got := resp.Candidates[0].Subject; got != "usage/container" {
		t.Fatalf("top subject = %q, want usage/container", got)
	}
}

func TestSimilarUnrelatedDraftIsANewSubject(t *testing.T) {
	draft := `A recipe for sourdough bread with a long cold fermentation and a scored crust.`
	resp := SimilarPages(similarFixture(), draft, SimilarOptions{})

	if resp.Verdict != "new_subject" {
		t.Fatalf("verdict = %q, want new_subject (top score %.3f)", resp.Verdict, topScore(resp))
	}
}

func TestSimilarEmptyDraft(t *testing.T) {
	resp := SimilarPages(similarFixture(), "   ", SimilarOptions{})
	if resp.Verdict != "new_subject" || len(resp.Candidates) != 0 || resp.Tokens != 0 {
		t.Fatalf("empty draft = %+v, want an empty new_subject", resp)
	}
}

func TestSimilarRespectsLimit(t *testing.T) {
	resp := SimilarPages(similarFixture(), "wiki page design usage container render", SimilarOptions{Limit: 2})
	if len(resp.Candidates) > 2 {
		t.Fatalf("returned %d candidates, want at most 2", len(resp.Candidates))
	}
	if resp.Total < len(resp.Candidates) {
		t.Fatalf("total %d is below the returned count %d", resp.Total, len(resp.Candidates))
	}
}

func TestSimilarSubjectStripsSourcePrefix(t *testing.T) {
	site, subject, role := similarPageKey(AgentPage{ID: "docs:page:methods/llm-wiki/report"})
	if site != "" || subject != "methods/llm-wiki" || role != "report" {
		t.Fatalf("site, subject, role = %q, %q, %q", site, subject, role)
	}
	site, subject, role = similarPageKey(AgentPage{ID: "page:usage/container"})
	if site != "" || subject != "usage/container" || role != "" {
		t.Fatalf("site, subject, role = %q, %q, %q", site, subject, role)
	}
	site, subject, role = similarPageKey(AgentPage{ID: "one:page:index", SiteKey: "one"})
	if site != "one" || subject != "index" || role != "" {
		t.Fatalf("tree site, subject, role = %q, %q, %q", site, subject, role)
	}
}

func TestSimilarWordsKeepsTechnicalTokens(t *testing.T) {
	got := similarWords("FTS5 and Q4_K_M, the a an")
	want := []string{"fts5", "q4"}
	if len(got) != len(want) {
		t.Fatalf("words = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("words = %v, want %v", got, want)
		}
	}
}

func TestSimilarWithoutManifestIsAnError(t *testing.T) {
	c := &SearchCorpus{}
	if _, err := c.Similar("anything", SimilarOptions{}); err == nil {
		t.Fatal("a corpus without pages.json returned no error")
	}
}

func topScore(r SimilarResponse) float64 {
	if len(r.Candidates) == 0 {
		return 0
	}
	return r.Candidates[0].Score
}

func TestSimilarKeepsTreeSitesApart(t *testing.T) {
	pages := []AgentPage{
		{ID: "one:page:index", SiteKey: "one", Site: "One", Title: "Home", URL: "/one/index.html"},
		{ID: "two:page:index", SiteKey: "two", Site: "Two", Title: "Home", URL: "/two/index.html"},
	}
	resp := SimilarPages(pages, "home", SimilarOptions{})
	if len(resp.Candidates) != 2 {
		t.Fatalf("candidates = %d, want 2 (one per wiki)", len(resp.Candidates))
	}
	sites := map[string]bool{}
	for _, c := range resp.Candidates {
		sites[c.Site] = true
	}
	if !sites["one"] || !sites["two"] {
		t.Fatalf("sites = %v, want one and two", sites)
	}
}

func TestSimilarUnknownWordsCannotCarryAMatch(t *testing.T) {
	draft := "sourdough starter hydration crust bake render"
	resp := SimilarPages(similarFixture(), draft, SimilarOptions{})
	if resp.Verdict != "new_subject" {
		t.Fatalf("verdict = %q at %.3f for a draft that shares one word", resp.Verdict, topScore(resp))
	}
}

func TestSimilarThresholdAndOverride(t *testing.T) {
	draft := "llm wiki ingest loop"
	if resp := SimilarPages(similarFixture(), draft, SimilarOptions{MinScore: 0.99}); resp.Verdict != "new_subject" {
		t.Fatalf("verdict = %q with a 0.99 threshold", resp.Verdict)
	}
	if resp := SimilarPages(similarFixture(), draft, SimilarOptions{MinScore: 0.01}); resp.Verdict != "update" {
		t.Fatalf("verdict = %q with a 0.01 threshold", resp.Verdict)
	}
}

func TestSimilarSkipsLogEntries(t *testing.T) {
	pages := append(similarFixture(), AgentPage{
		ID: "page:logs/2026/10/01-x", Type: "Log Entry",
		Title: "LLM wiki ingest loop", Description: "Karpathy wiki ingest",
		URL: "/wiki/logs/2026/10/01-x.html",
	})
	resp := SimilarPages(pages, "llm wiki ingest loop", SimilarOptions{})
	for _, c := range resp.Candidates {
		if c.Subject == "logs/2026/10/01-x" {
			t.Fatal("a log entry was offered as a subject")
		}
	}
}

func TestSimilarStemFoldsBothSides(t *testing.T) {
	pairs := [][2]string{
		{"cache", "caches"}, {"page", "pages"}, {"file", "files"},
		{"building", "buildings"}, {"box", "boxes"}, {"wish", "wishes"},
		{"render", "renders"}, {"status", "statuses"},
		{"policy", "policies"}, {"cookie", "cookies"},
		{"body", "bodies"}, {"city", "cities"}, {"copy", "copies"},
		{"try", "tries"}, {"deny", "denies"}, {"vary", "varies"},
	}
	for _, pair := range pairs {
		left := similarStem(pair[0])
		right := similarStem(pair[1])
		if left != right {
			t.Errorf("stem(%q)=%q but stem(%q)=%q", pair[0], left, pair[1], right)
		}
	}
}

func renderTestSource(t *testing.T, title, body string) (*AgentSource, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.md"),
		[]byte("# "+title+"\n\n"+body+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	site := filepath.Join(out, "wiki")
	c, err := LoadSearchCorpus(site)
	if err != nil {
		t.Fatal(err)
	}
	return &AgentSource{
		Name: title, Base: "http://example.test/" + title + "/",
		Corpus: c, Graph: &GraphCorpus{},
	}, site
}

func TestSimilarPageBodiesMatchTheCorpus(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.md"),
		[]byte("# Home\n\n## Install\n\nThe install guide starts here.\n\n## Run\n\nStart the binary.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := New(Config{Content: dir, Base: "/wiki/", Brand: Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	c, err := LoadSearchCorpus(filepath.Join(out, "wiki"))
	if err != nil {
		t.Fatal(err)
	}
	fromBundle := r.PageBodies()
	fromRender := c.pageBodies()
	if len(fromBundle) != len(fromRender) {
		t.Fatalf("bundle has %d bodies, render has %d", len(fromBundle), len(fromRender))
	}
	for id, text := range fromBundle {
		if fromRender[id] != text {
			t.Fatalf("body for %s differs between bundle and render", id)
		}
	}
}

func TestFederatorSimilar(t *testing.T) {
	good, goodSite := renderTestSource(t, "good", "The install guide starts here.")
	f := NewFederator([]*AgentSource{good})
	resp, err := f.Similar("install guide", SimilarOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Candidates) == 0 || resp.Candidates[0].Source != "good" {
		t.Fatalf("single-source plan = %+v", resp)
	}
	if !strings.HasPrefix(resp.Candidates[0].Pages[0].URL, "http://example.test/good/") {
		t.Fatalf("page URL not resolved against the source: %q", resp.Candidates[0].Pages[0].URL)
	}

	// A source that rendered no manifest cannot be planned, and must not turn
	// the whole answer into an error while another source reads.
	if err := os.Remove(filepath.Join(goodSite, "pages.json")); err != nil {
		t.Fatal(err)
	}
	broken, err := LoadSearchCorpus(goodSite)
	if err != nil {
		t.Fatal(err)
	}
	bad := &AgentSource{
		Name: "bad", Base: "http://example.test/bad/",
		Corpus: broken, Graph: &GraphCorpus{},
	}
	f = NewFederator([]*AgentSource{good, bad})
	resp, err = f.Similar("install guide", SimilarOptions{}, nil)
	if err != nil {
		t.Fatalf("a partial failure returned an error: %v", err)
	}
	if len(resp.Errors) != 1 || len(resp.Candidates) == 0 {
		t.Fatalf("partial plan = %+v", resp)
	}

	f = NewFederator([]*AgentSource{bad})
	if _, err := f.Similar("install guide", SimilarOptions{}, nil); err == nil {
		t.Fatal("a plan where every source failed returned no error")
	}
}
