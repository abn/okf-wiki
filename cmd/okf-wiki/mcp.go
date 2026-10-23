package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/abn/okf-wiki/internal/wiki"
)

// mcpServer exposes the rendered corpus over the Model Context Protocol stdio
// transport. It is deliberately read-only: search, page and section reads, and
// budgeted context packs all return citations, but nothing writes to the wiki.
type mcpServer struct {
	siteDir   string
	base      string
	corpus    *wiki.SearchCorpus
	graph     *wiki.GraphCorpus
	changes   wiki.AgentChanges
	health    wiki.GraphHealth
	hasChange bool
	hasHealth bool
	changeErr string
	healthErr string
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

func runMCP(args []string) {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	out := fs.String("out", env("OKF_WIKI_OUT", ".scratch/wiki"), "render output directory")
	base := fs.String("base", env("OKF_WIKI_BASE", wiki.DefaultBase), "URL prefix the wiki was rendered under")
	fs.Parse(reorderArgs(args, fs))
	siteDir, err := (wiki.Config{Out: *out, Base: *base}).SiteDir(*out)
	if err != nil {
		fatal(err)
	}
	s := &mcpServer{siteDir: siteDir, base: *base}
	if err := s.refresh(); err != nil {
		fatal(err)
	}
	if err := s.serve(os.Stdin, os.Stdout); err != nil {
		fatal(err)
	}
}

// refresh reloads the rendered files before each tool call. A long-running
// agent therefore sees a re-render without restarting the MCP process. The
// search and graph files are required; changes and health are optional so a
// site rendered before those files existed still answers search and reads.
func (s *mcpServer) refresh() error {
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if err := s.refreshOnce(); err == nil {
			return nil
		} else {
			last = err
		}
		if attempt < 2 {
			time.Sleep(10 * time.Millisecond)
		}
	}
	return last
}

func (s *mcpServer) refreshOnce() error {
	corpus, err := wiki.LoadSearchCorpus(s.siteDir, s.base)
	if err != nil {
		return fmt.Errorf("load rendered search data: %w", err)
	}
	graph, err := wiki.LoadGraphCorpus(s.siteDir)
	if err != nil {
		return fmt.Errorf("load rendered graph data: %w", err)
	}
	if corpus.RenderID != "" && graph.RenderID != "" && corpus.RenderID != graph.RenderID {
		return fmt.Errorf("search and graph files are from different renders")
	}
	s.corpus = corpus
	s.graph = graph
	s.hasChange = false
	s.hasHealth = false
	s.changeErr = ""
	s.healthErr = ""
	if changes, err := wiki.LoadAgentChanges(s.siteDir); err == nil {
		s.changes = changes
		s.hasChange = true
	} else {
		s.changeErr = err.Error()
	}
	if health, err := wiki.LoadGraphHealth(s.siteDir); err == nil {
		s.health = health
		s.hasHealth = true
	} else {
		s.healthErr = err.Error()
	}
	if s.hasChange && s.changes.RenderID != "" && corpus.RenderID != "" && s.changes.RenderID != corpus.RenderID {
		s.hasChange = false
		s.changeErr = "changes.json is from another render; render the site again"
	}
	if s.hasHealth && s.health.RenderID != "" && corpus.RenderID != "" && s.health.RenderID != corpus.RenderID {
		s.hasHealth = false
		s.healthErr = "health.json is from another render; render the site again"
	}
	return nil
}

// serve reads one JSON-RPC message per line, as the MCP stdio transport does,
// and writes one response per request. Notifications produce no response.
func (s *mcpServer) serve(in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	encoder := json.NewEncoder(out)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		resp := s.handle([]byte(line))
		if resp == nil {
			continue
		}
		if err := encoder.Encode(resp); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (s *mcpServer) handle(line []byte) *rpcResponse {
	var req rpcRequest
	if err := json.Unmarshal(line, &req); err != nil {
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      json.RawMessage("null"),
			Error:   &rpcError{Code: -32700, Message: "parse error"},
		}
	}
	if strings.HasPrefix(req.Method, "notifications/") {
		return nil
	}
	if len(req.ID) == 0 {
		// A JSON-RPC notification has no response, even if its method is
		// unknown or malformed.
		return nil
	}
	switch req.Method {
	case "initialize":
		// The server implements the core tool surface, which is compatible with
		// this protocol version. It does not echo an unknown requested version.
		protocol := "2024-11-05"
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": protocol,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "okf-wiki", "version": version},
			},
		}
	case "ping":
		return &rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}
	case "tools/list":
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{"tools": []any{
				mcpSearchTool(), mcpReadPageTool(), mcpReadSectionTool(), mcpContextTool(),
				mcpNeighborsTool(), mcpPathTool(), mcpChangesTool(), mcpHealthTool(),
			}},
		}
	case "tools/call":
		result := s.callTool(req.Params)
		return &rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result}
	default:
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32601, Message: "method not found"},
		}
	}
}

func mcpReadPageTool() map[string]any {
	return map[string]any{
		"name":        "read_page",
		"description": "Read one page's Markdown source, capped at a token budget. Returns the page ID, HTML URL, Markdown URL and content hash.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pageId":    map[string]any{"type": "string", "description": "Page ID from search, pages.json or the graph."},
				"maxTokens": map[string]any{"type": "integer", "minimum": 1, "maximum": 32000, "default": 4000},
			},
			"required":             []string{"pageId"},
			"additionalProperties": false,
		},
	}
}

func mcpReadSectionTool() map[string]any {
	return map[string]any{
		"name":        "read_section",
		"description": "Read one heading-anchored section as plain text, with page ID, anchor, URLs and content hash.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pageId":    map[string]any{"type": "string", "description": "Page ID from search or pages.json."},
				"anchor":    map[string]any{"type": "string", "description": "Heading anchor from a search result or pages.json."},
				"maxTokens": map[string]any{"type": "integer", "minimum": 1, "maximum": 32000, "default": 2000},
			},
			"required":             []string{"pageId", "anchor"},
			"additionalProperties": false,
		},
	}
}

func mcpContextTool() map[string]any {
	return map[string]any{
		"name":        "context_for",
		"description": "Return the best-ranked sections for a query within a token budget, with page IDs, anchors, URLs and content hashes.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":     map[string]any{"type": "string", "description": "Words or a phrase to search for."},
				"maxTokens": map[string]any{"type": "integer", "minimum": 1, "maximum": 32000, "default": 3000},
				"limit":     map[string]any{"type": "integer", "minimum": 1, "maximum": 20, "default": 10},
				"site":      map[string]any{"type": "string", "description": "Wiki title or site key."},
				"tag":       map[string]any{"type": "string", "description": "Page tag."},
				"section":   map[string]any{"type": "string", "description": "Section title."},
			},
			"required":             []string{"query"},
			"additionalProperties": false,
		},
	}
}

func mcpChangesTool() map[string]any {
	return map[string]any{
		"name":        "changes",
		"description": "Return the pages added, changed or removed in the latest render, with page IDs and source hashes.",
		"inputSchema": map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": false,
		},
	}
}

func mcpHealthTool() map[string]any {
	return map[string]any{
		"name":        "health",
		"description": "Return broken links, orphans, tag-only pages, link cycles, stale pages and the most connected pages.",
		"inputSchema": map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": false,
		},
	}
}

func mcpNeighborsTool() map[string]any {
	return map[string]any{
		"name":        "neighbors",
		"description": "Return the local graph around a page up to three hops, with typed edges and node citations.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pageId": map[string]any{"type": "string", "description": "Page ID from search, pages.json or the graph."},
				"depth":  map[string]any{"type": "integer", "minimum": 1, "maximum": 3, "default": 1},
				"kinds":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional edge kinds: link, tag, previous, next."},
			},
			"required":             []string{"pageId"},
			"additionalProperties": false,
		},
	}
}

func mcpPathTool() map[string]any {
	return map[string]any{
		"name":        "path",
		"description": "Find the shortest undirected path between two pages, with the typed edge that connects each pair.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"from":     map[string]any{"type": "string", "description": "Starting page ID."},
				"to":       map[string]any{"type": "string", "description": "Target page ID."},
				"maxDepth": map[string]any{"type": "integer", "minimum": 1, "maximum": 10, "default": 6},
				"kinds":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional edge kinds: link, tag, previous, next."},
			},
			"required":             []string{"from", "to"},
			"additionalProperties": false,
		},
	}
}

func mcpSearchTool() map[string]any {
	return map[string]any{
		"name":        "search_wiki",
		"description": "Search the rendered OKF wiki. Returns ranked snippets with page ID, heading anchor, HTML URL, Markdown URL and source hash.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":   map[string]any{"type": "string", "description": "Words or a phrase to search for."},
				"limit":   map[string]any{"type": "integer", "minimum": 1, "maximum": 20, "default": 5},
				"site":    map[string]any{"type": "string", "description": "Wiki title or site key."},
				"tag":     map[string]any{"type": "string", "description": "Page tag."},
				"section": map[string]any{"type": "string", "description": "Section title."},
			},
			"required":             []string{"query"},
			"additionalProperties": false,
		},
	}
}

type mcpCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type mcpSearchArgs struct {
	Query   string `json:"query"`
	Limit   int    `json:"limit"`
	Site    string `json:"site"`
	Tag     string `json:"tag"`
	Section string `json:"section"`
}

func (s *mcpServer) callTool(raw json.RawMessage) map[string]any {
	if err := s.refresh(); err != nil {
		return mcpToolError(err.Error())
	}
	var params mcpCallParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return mcpToolError("invalid tool call")
	}
	switch params.Name {
	case "search_wiki":
		return s.searchTool(params.Arguments)
	case "read_page":
		return s.readPageTool(params.Arguments)
	case "read_section":
		return s.readSectionTool(params.Arguments)
	case "context_for":
		return s.contextTool(params.Arguments)
	case "neighbors":
		return s.neighborsTool(params.Arguments)
	case "path":
		return s.pathTool(params.Arguments)
	case "changes":
		if !s.hasChange {
			return mcpToolError("changes unavailable: " + s.changeErr + "; render the site again")
		}
		return mcpToolJSON(s.changes)
	case "health":
		if !s.hasHealth {
			return mcpToolError("health unavailable: " + s.healthErr + "; render the site again")
		}
		return mcpToolJSON(s.health)
	default:
		return mcpToolError("unknown tool " + params.Name)
	}
}

func (s *mcpServer) searchTool(raw json.RawMessage) map[string]any {
	var args mcpSearchArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return mcpToolError("invalid search arguments")
	}
	if strings.TrimSpace(args.Query) == "" {
		return mcpToolError("query is required")
	}
	if args.Limit == 0 {
		args.Limit = 5
	}
	if args.Limit < 1 || args.Limit > 20 {
		return mcpToolError("limit must be between 1 and 20")
	}
	resp, err := s.corpus.Search(args.Query, wiki.SearchOptions{
		Limit: args.Limit, Site: args.Site, Tag: args.Tag, Section: args.Section,
	})
	if err != nil {
		return mcpToolError(err.Error())
	}
	return mcpToolJSON(resp)
}

type mcpReadPageArgs struct {
	PageID    string `json:"pageId"`
	MaxTokens int    `json:"maxTokens"`
}

func (s *mcpServer) readPageTool(raw json.RawMessage) map[string]any {
	var args mcpReadPageArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return mcpToolError("invalid read_page arguments")
	}
	if args.PageID == "" {
		return mcpToolError("pageId is required")
	}
	if args.MaxTokens < 0 || args.MaxTokens > 32000 {
		return mcpToolError("maxTokens must be between 1 and 32000")
	}
	page, err := s.corpus.ReadPage(args.PageID, args.MaxTokens)
	if err != nil {
		return mcpToolError(err.Error())
	}
	return mcpToolJSON(page)
}

type mcpReadSectionArgs struct {
	PageID    string `json:"pageId"`
	Anchor    string `json:"anchor"`
	MaxTokens int    `json:"maxTokens"`
}

func (s *mcpServer) readSectionTool(raw json.RawMessage) map[string]any {
	var args mcpReadSectionArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return mcpToolError("invalid read_section arguments")
	}
	if args.PageID == "" || args.Anchor == "" {
		return mcpToolError("pageId and anchor are required")
	}
	if args.MaxTokens < 0 || args.MaxTokens > 32000 {
		return mcpToolError("maxTokens must be between 1 and 32000")
	}
	section, err := s.corpus.ReadSection(args.PageID, args.Anchor, args.MaxTokens)
	if err != nil {
		return mcpToolError(err.Error())
	}
	return mcpToolJSON(section)
}

type mcpContextArgs struct {
	Query     string `json:"query"`
	MaxTokens int    `json:"maxTokens"`
	Limit     int    `json:"limit"`
	Site      string `json:"site"`
	Tag       string `json:"tag"`
	Section   string `json:"section"`
}

func (s *mcpServer) contextTool(raw json.RawMessage) map[string]any {
	var args mcpContextArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return mcpToolError("invalid context_for arguments")
	}
	if strings.TrimSpace(args.Query) == "" {
		return mcpToolError("query is required")
	}
	if args.MaxTokens < 0 || args.MaxTokens > 32000 {
		return mcpToolError("maxTokens must be between 1 and 32000")
	}
	if args.Limit == 0 {
		args.Limit = 10
	}
	if args.Limit < 1 || args.Limit > 20 {
		return mcpToolError("limit must be between 1 and 20")
	}
	resp, err := s.corpus.ContextFor(args.Query, wiki.SearchOptions{
		Limit: args.Limit, Site: args.Site, Tag: args.Tag, Section: args.Section,
	}, args.MaxTokens)
	if err != nil {
		return mcpToolError(err.Error())
	}
	return mcpToolJSON(resp)
}

type mcpNeighborsArgs struct {
	PageID string   `json:"pageId"`
	Depth  int      `json:"depth"`
	Kinds  []string `json:"kinds"`
}

func (s *mcpServer) neighborsTool(raw json.RawMessage) map[string]any {
	var args mcpNeighborsArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return mcpToolError("invalid neighbors arguments")
	}
	if args.PageID == "" {
		return mcpToolError("pageId is required")
	}
	resp, err := s.graph.Neighbors(args.PageID, args.Depth, args.Kinds)
	if err != nil {
		return mcpToolError(err.Error())
	}
	return mcpToolJSON(resp)
}

type mcpPathArgs struct {
	From     string   `json:"from"`
	To       string   `json:"to"`
	MaxDepth int      `json:"maxDepth"`
	Kinds    []string `json:"kinds"`
}

func (s *mcpServer) pathTool(raw json.RawMessage) map[string]any {
	var args mcpPathArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return mcpToolError("invalid path arguments")
	}
	if args.From == "" || args.To == "" {
		return mcpToolError("from and to are required")
	}
	resp, err := s.graph.Path(args.From, args.To, args.MaxDepth, args.Kinds)
	if err != nil {
		return mcpToolError(err.Error())
	}
	return mcpToolJSON(resp)
}

func mcpToolJSON(value any) map[string]any {
	body, err := json.Marshal(value)
	if err != nil {
		return mcpToolError(err.Error())
	}
	return map[string]any{
		"content": []any{map[string]any{"type": "text", "text": string(body)}},
		"isError": false,
	}
}

func mcpToolError(message string) map[string]any {
	return map[string]any{
		"content": []any{map[string]any{"type": "text", "text": message}},
		"isError": true,
	}
}
