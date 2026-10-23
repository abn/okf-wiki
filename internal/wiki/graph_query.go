package wiki

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GraphNodeInfo is the compact node shape returned by the agent graph tools.
type GraphNodeInfo struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Kind    string `json:"kind"`
	URL     string `json:"url,omitempty"`
	Site    string `json:"site,omitempty"`
	SiteKey string `json:"siteKey,omitempty"`
	Stub    bool   `json:"stub,omitempty"`
}

// GraphEdgeInfo is one typed relation between two nodes.
type GraphEdgeInfo struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Kind   string `json:"kind"`
	Anchor string `json:"anchor,omitempty"`
	Text   string `json:"text,omitempty"`
}

// NeighborsResponse is a local neighbourhood around one page.
type NeighborsResponse struct {
	Center    GraphNodeInfo   `json:"center"`
	Depth     int             `json:"depth"`
	Nodes     []GraphNodeInfo `json:"nodes"`
	Edges     []GraphEdgeInfo `json:"edges"`
	Truncated bool            `json:"truncated,omitempty"`
}

// PathResponse is the shortest undirected path between two pages.
type PathResponse struct {
	From   GraphNodeInfo   `json:"from"`
	To     GraphNodeInfo   `json:"to"`
	Found  bool            `json:"found"`
	Length int             `json:"length"`
	Nodes  []GraphNodeInfo `json:"nodes"`
	Edges  []GraphEdgeInfo `json:"edges"`
}

// GraphCorpus is a rendered graph.json plus the page order needed for typed
// previous and next relations. Previous and next are derived from pages.json,
// not stored in graph.json, so they are available to agents without changing
// the browser drawing.
type GraphCorpus struct {
	Graph GraphIndex
	Pages AgentManifest
	nodes map[string]GraphNode
	adj   map[string][]adjEdge
	edges []GraphEdgeInfo
}

type adjEdge struct {
	peer string
	edge GraphEdgeInfo
	kind string
}

// LoadGraphCorpus reads graph.json and, when present, pages.json from a
// rendered site directory.
func LoadGraphCorpus(dir string) (*GraphCorpus, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "graph.json"))
	if err != nil {
		return nil, fmt.Errorf("read graph.json: %w", err)
	}
	var g GraphIndex
	if err := json.Unmarshal(raw, &g); err != nil {
		return nil, fmt.Errorf("decode graph.json: %w", err)
	}
	c := &GraphCorpus{Graph: g, nodes: make(map[string]GraphNode), adj: make(map[string][]adjEdge)}
	for _, n := range g.Nodes {
		c.nodes[n.ID] = n
	}
	for _, e := range g.Edges {
		if _, ok := c.nodes[e.From]; !ok {
			continue
		}
		if _, ok := c.nodes[e.To]; !ok {
			continue
		}
		info := GraphEdgeInfo{From: e.From, To: e.To, Kind: e.Kind, Anchor: e.Anchor, Text: e.Text}
		c.edges = append(c.edges, info)
		c.adj[e.From] = append(c.adj[e.From], adjEdge{peer: e.To, edge: info, kind: e.Kind})
		c.adj[e.To] = append(c.adj[e.To], adjEdge{peer: e.From, edge: info, kind: e.Kind})
	}
	if manifestRaw, err := os.ReadFile(filepath.Join(dir, "pages.json")); err == nil {
		if err := json.Unmarshal(manifestRaw, &c.Pages); err != nil {
			return nil, fmt.Errorf("decode pages.json: %w", err)
		}
		c.addOrderEdges()
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return c, nil
}

// addOrderEdges adds previous and next relations inside each site and section.
// They are deterministic because pages.json is in render order.
func (c *GraphCorpus) addOrderEdges() {
	last := make(map[string]AgentPage)
	for _, p := range c.Pages.Pages {
		key := p.SiteKey + "\x00" + p.Section
		prev, ok := last[key]
		if ok {
			c.addSynthetic(GraphEdgeInfo{From: prev.ID, To: p.ID, Kind: "next"})
			c.addSynthetic(GraphEdgeInfo{From: p.ID, To: prev.ID, Kind: "previous"})
		}
		last[key] = p
	}
}

func (c *GraphCorpus) addSynthetic(e GraphEdgeInfo) {
	if _, ok := c.nodes[e.From]; !ok {
		return
	}
	if _, ok := c.nodes[e.To]; !ok {
		return
	}
	for _, existing := range c.adj[e.From] {
		if existing.peer == e.To && existing.kind == e.Kind {
			return
		}
	}
	c.edges = append(c.edges, e)
	c.adj[e.From] = append(c.adj[e.From], adjEdge{peer: e.To, edge: e, kind: e.Kind})
}

func nodeInfo(n GraphNode) GraphNodeInfo {
	return GraphNodeInfo{ID: n.ID, Title: n.Title, Kind: n.Kind, URL: n.URL, Site: n.Site, SiteKey: n.SiteKey, Stub: n.Stub}
}

// Neighbors returns the undirected local graph around pageID up to depth. The
// edge kinds filter is optional; an empty filter includes every kind.
func (c *GraphCorpus) Neighbors(pageID string, depth int, kinds []string) (NeighborsResponse, error) {
	center, ok := c.nodes[pageID]
	if !ok {
		return NeighborsResponse{}, fmt.Errorf("unknown page %q", pageID)
	}
	if depth <= 0 {
		depth = 1
	}
	if depth > 3 {
		depth = 3
	}
	allowed := edgeKindFilter(kinds)
	visited := map[string]bool{pageID: true}
	nodes := []GraphNodeInfo{nodeInfo(center)}
	edges := []GraphEdgeInfo{}
	seenEdges := map[string]bool{}
	type item struct {
		id    string
		depth int
	}
	queue := []item{{pageID, 0}}
	truncated := false
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current.depth >= depth {
			continue
		}
		for _, e := range c.adj[current.id] {
			if !allowed(e.kind) {
				continue
			}
			if !visited[e.peer] {
				if len(nodes) >= 200 {
					truncated = true
					continue
				}
				visited[e.peer] = true
				nodes = append(nodes, nodeInfo(c.nodes[e.peer]))
				queue = append(queue, item{e.peer, current.depth + 1})
			}
			key := e.edge.From + "\x00" + e.edge.To + "\x00" + e.kind + "\x00" + e.edge.Anchor
			if !seenEdges[key] {
				seenEdges[key] = true
				edges = append(edges, e.edge)
			}
		}
	}
	return NeighborsResponse{Center: nodeInfo(center), Depth: depth, Nodes: nodes, Edges: edges, Truncated: truncated}, nil
}

// Path returns the shortest undirected path between two pages, with the typed
// edge that connects each pair.
func (c *GraphCorpus) Path(fromID, toID string, maxDepth int, kinds []string) (PathResponse, error) {
	from, ok := c.nodes[fromID]
	if !ok {
		return PathResponse{}, fmt.Errorf("unknown page %q", fromID)
	}
	to, ok := c.nodes[toID]
	if !ok {
		return PathResponse{}, fmt.Errorf("unknown page %q", toID)
	}
	if maxDepth <= 0 {
		maxDepth = 6
	}
	if maxDepth > 10 {
		maxDepth = 10
	}
	if fromID == toID {
		return PathResponse{From: nodeInfo(from), To: nodeInfo(to), Found: true, Nodes: []GraphNodeInfo{nodeInfo(from)}, Edges: []GraphEdgeInfo{}}, nil
	}
	allowed := edgeKindFilter(kinds)
	type step struct {
		node GraphNodeInfo
		edge GraphEdgeInfo
		from string
	}
	prev := make(map[string]step)
	visited := map[string]bool{fromID: true}
	type item struct {
		id    string
		depth int
	}
	queue := []item{{fromID, 0}}
	found := false
	for len(queue) > 0 && !found {
		current := queue[0]
		queue = queue[1:]
		if current.depth >= maxDepth {
			continue
		}
		for _, e := range c.adj[current.id] {
			if !allowed(e.kind) || visited[e.peer] {
				continue
			}
			visited[e.peer] = true
			prev[e.peer] = step{node: nodeInfo(c.nodes[e.peer]), edge: e.edge, from: current.id}
			if e.peer == toID {
				found = true
				break
			}
			queue = append(queue, item{e.peer, current.depth + 1})
		}
	}
	resp := PathResponse{From: nodeInfo(from), To: nodeInfo(to), Found: found, Nodes: []GraphNodeInfo{}, Edges: []GraphEdgeInfo{}}
	if !found {
		return resp, nil
	}
	var nodes []GraphNodeInfo
	var edges []GraphEdgeInfo
	for id := toID; ; {
		nodes = append(nodes, nodeInfo(c.nodes[id]))
		if id == fromID {
			break
		}
		s := prev[id]
		edges = append(edges, s.edge)
		id = s.from
	}
	for i, j := 0, len(nodes)-1; i < j; i, j = i+1, j-1 {
		nodes[i], nodes[j] = nodes[j], nodes[i]
	}
	for i, j := 0, len(edges)-1; i < j; i, j = i+1, j-1 {
		edges[i], edges[j] = edges[j], edges[i]
	}
	resp.Nodes = nodes
	resp.Edges = edges
	resp.Length = len(edges)
	return resp, nil
}

func edgeKindFilter(kinds []string) func(string) bool {
	if len(kinds) == 0 {
		return func(string) bool { return true }
	}
	allowed := make(map[string]bool, len(kinds))
	for _, kind := range kinds {
		allowed[strings.TrimSpace(kind)] = true
	}
	return func(kind string) bool { return allowed[kind] }
}
