package wiki

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// AgentChange is one page added, changed or removed since the previous render.
type AgentChange struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Kind         string `json:"kind"` // added, changed, removed
	ContentHash  string `json:"contentHash,omitempty"`
	PreviousHash string `json:"previousHash,omitempty"`
}

// AgentChanges is changes.json: the difference between the previous and current
// page manifests.
type AgentChanges struct {
	Version          int           `json:"version"`
	GeneratedAt      string        `json:"generatedAt,omitempty"`
	RenderID         string        `json:"renderId"`
	PreviousRenderID string        `json:"previousRenderId,omitempty"`
	Changes          []AgentChange `json:"changes"`
}

// HealthPage is a page named by the graph health report.
type HealthPage struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// HealthLink is a broken link edge.
type HealthLink struct {
	From string `json:"from"`
	To   string `json:"to"`
	Text string `json:"text,omitempty"`
}

// HealthNode is a page with its total graph degree.
type HealthNode struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Degree int    `json:"degree"`
}

// GraphHealth is health.json: the structural problems an agent should avoid and
// the pages worth starting from.
type GraphHealth struct {
	Version       int          `json:"version"`
	GeneratedAt   string       `json:"generatedAt,omitempty"`
	RenderID      string       `json:"renderId"`
	BrokenLinks   []HealthLink `json:"brokenLinks"`
	Orphans       []HealthPage `json:"orphans"`
	TagOnly       []HealthPage `json:"tagOnlyPages"`
	Cycles        [][]string   `json:"cycles"`
	MostConnected []HealthNode `json:"mostConnected"`
	Stale         []HealthPage `json:"stale"`
}

const agentStateVersion = 1

// readAgentPages reads the previous pages.json, if one exists. A missing file
// means this is the first render, not an error.
func readAgentPages(dir string) ([]AgentPage, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "pages.json"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var m AgentManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("decode pages.json: %w", err)
	}
	return m.Pages, nil
}

// agentRenderID is a stable digest of page IDs and source hashes. Page order is
// ignored so a section reorder alone does not look like a content change.
func agentRenderID(pages []AgentPage) string {
	items := make([]string, 0, len(pages))
	for _, p := range pages {
		items = append(items, p.ID+"\x00"+p.ContentHash)
	}
	sort.Strings(items)
	sum := sha256.Sum256([]byte(strings.Join(items, "\n")))
	return hex.EncodeToString(sum[:])
}

func buildAgentChanges(previous, current []AgentPage, generatedAt string) AgentChanges {
	prev := make(map[string]AgentPage, len(previous))
	for _, p := range previous {
		prev[p.ID] = p
	}
	now := make(map[string]AgentPage, len(current))
	changes := []AgentChange{}
	for _, p := range current {
		now[p.ID] = p
		old, ok := prev[p.ID]
		switch {
		case !ok:
			changes = append(changes, AgentChange{ID: p.ID, Title: p.Title, Kind: "added", ContentHash: p.ContentHash})
		case old.ContentHash != p.ContentHash:
			changes = append(changes, AgentChange{ID: p.ID, Title: p.Title, Kind: "changed", ContentHash: p.ContentHash, PreviousHash: old.ContentHash})
		}
	}
	for _, p := range previous {
		if _, ok := now[p.ID]; !ok {
			changes = append(changes, AgentChange{ID: p.ID, Title: p.Title, Kind: "removed", PreviousHash: p.ContentHash})
		}
	}
	changesOut := AgentChanges{
		Version:     agentStateVersion,
		GeneratedAt: generatedAt,
		RenderID:    agentRenderID(current),
		Changes:     changes,
	}
	if len(previous) > 0 {
		changesOut.PreviousRenderID = agentRenderID(previous)
	}
	return changesOut
}

func buildGraphHealth(g GraphIndex, pages []AgentPage, generatedAt string, extraBroken ...HealthLink) GraphHealth {
	pageByID := make(map[string]AgentPage, len(pages))
	for _, p := range pages {
		pageByID[p.ID] = p
	}
	nodes := make(map[string]GraphNode, len(g.Nodes))
	for _, n := range g.Nodes {
		nodes[n.ID] = n
	}
	degree := make(map[string]int)
	linkIn := make(map[string]int)
	linkOut := make(map[string]int)
	adj := make(map[string][]string)
	broken := append([]HealthLink{}, extraBroken...)
	for _, e := range g.Edges {
		from, ok := nodes[e.From]
		if !ok {
			continue
		}
		to, ok := nodes[e.To]
		if !ok {
			continue
		}
		if from.Kind == "page" {
			degree[from.ID]++
		}
		if to.Kind == "page" {
			degree[to.ID]++
		}
		if e.Kind != "link" {
			continue
		}
		if to.Stub {
			broken = append(broken, HealthLink{From: e.From, To: e.To, Text: e.Text})
			continue
		}
		if from.Kind != "page" || to.Kind != "page" {
			continue
		}
		linkOut[from.ID]++
		linkIn[to.ID]++
		adj[from.ID] = append(adj[from.ID], to.ID)
	}

	ids := make([]string, 0, len(pageByID))
	for id := range pageByID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	orphans := []HealthPage{}
	tagOnly := []HealthPage{}
	stalePages := []HealthPage{}
	now, _ := time.Parse("2006-01-02", generatedAt)
	for _, id := range ids {
		p := pageByID[id]
		page := HealthPage{ID: id, Title: p.Title}
		if linkIn[id] == 0 && !isIndexPage(id) {
			orphans = append(orphans, page)
		}
		if len(p.Tags) > 0 && linkIn[id] == 0 && linkOut[id] == 0 && !isIndexPage(id) {
			tagOnly = append(tagOnly, page)
		}
		if !now.IsZero() && stale(p.StaleAfter, now) {
			stalePages = append(stalePages, page)
		}
	}
	sort.Slice(broken, func(i, j int) bool {
		if broken[i].From != broken[j].From {
			return broken[i].From < broken[j].From
		}
		if broken[i].To != broken[j].To {
			return broken[i].To < broken[j].To
		}
		return broken[i].Text < broken[j].Text
	})
	most := make([]HealthNode, 0, len(ids))
	for _, id := range ids {
		if degree[id] == 0 {
			continue
		}
		most = append(most, HealthNode{ID: id, Title: pageByID[id].Title, Degree: degree[id]})
	}
	sort.Slice(most, func(i, j int) bool {
		if most[i].Degree != most[j].Degree {
			return most[i].Degree > most[j].Degree
		}
		return most[i].ID < most[j].ID
	})
	if len(most) > 10 {
		most = most[:10]
	}
	return GraphHealth{
		Version:       agentStateVersion,
		GeneratedAt:   generatedAt,
		RenderID:      agentRenderID(pages),
		BrokenLinks:   broken,
		Orphans:       orphans,
		TagOnly:       tagOnly,
		Cycles:        linkCycles(ids, adj),
		MostConnected: most,
		Stale:         stalePages,
	}
}

func isIndexPage(id string) bool {
	slug := id
	if i := strings.LastIndex(slug, ":page:"); i >= 0 {
		slug = slug[i+len(":page:"):]
	} else {
		slug = strings.TrimPrefix(slug, "page:")
	}
	return slug == "index" || strings.HasSuffix(slug, "/index")
}

// linkCycles returns strongly connected components with more than one page.
// The graph builder excludes self links, so a singleton is never a cycle.
func linkCycles(ids []string, adj map[string][]string) [][]string {
	index := make(map[string]int)
	low := make(map[string]int)
	onStack := make(map[string]bool)
	var stack []string
	comps := [][]string{}
	next := 0
	var visit func(string)
	visit = func(v string) {
		index[v] = next
		low[v] = next
		next++
		stack = append(stack, v)
		onStack[v] = true
		for _, w := range adj[v] {
			if _, ok := index[w]; !ok {
				visit(w)
				if low[w] < low[v] {
					low[v] = low[w]
				}
			} else if onStack[w] && index[w] < low[v] {
				low[v] = index[w]
			}
		}
		if low[v] == index[v] {
			var comp []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				comp = append(comp, w)
				if w == v {
					break
				}
			}
			if len(comp) > 1 {
				sort.Strings(comp)
				comps = append(comps, comp)
			}
		}
	}
	for _, id := range ids {
		if _, ok := index[id]; !ok {
			visit(id)
		}
	}
	sort.Slice(comps, func(i, j int) bool { return comps[i][0] < comps[j][0] })
	return comps
}

func writeAgentState(dir string, previous, current []AgentPage, graph GraphIndex, generatedAt string, extraBroken ...HealthLink) error {
	changes := buildAgentChanges(previous, current, generatedAt)
	health := buildGraphHealth(graph, current, generatedAt, extraBroken...)
	changeJSON, err := json.Marshal(changes)
	if err != nil {
		return err
	}
	healthJSON, err := json.Marshal(health)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "changes.json"), changeJSON, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "health.json"), healthJSON, 0o644)
}

// LoadAgentChanges reads changes.json from a rendered site.
func LoadAgentChanges(dir string) (AgentChanges, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "changes.json"))
	if err != nil {
		return AgentChanges{}, fmt.Errorf("read changes.json: %w", err)
	}
	var c AgentChanges
	if err := json.Unmarshal(raw, &c); err != nil {
		return AgentChanges{}, fmt.Errorf("decode changes.json: %w", err)
	}
	return c, nil
}

// LoadGraphHealth reads health.json from a rendered site.
func LoadGraphHealth(dir string) (GraphHealth, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "health.json"))
	if err != nil {
		return GraphHealth{}, fmt.Errorf("read health.json: %w", err)
	}
	var h GraphHealth
	if err := json.Unmarshal(raw, &h); err != nil {
		return GraphHealth{}, fmt.Errorf("decode health.json: %w", err)
	}
	return h, nil
}
