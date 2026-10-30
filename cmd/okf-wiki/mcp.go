package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/abn/okf-wiki/internal/wiki"
)

// mcpServer exposes the rendered corpus over the Model Context Protocol stdio
// transport. It is deliberately read-only: search, page and section reads, and
// budgeted context packs all return citations, but nothing writes to the wiki.
type mcpServer struct {
	specs    []mcpSource
	ttl      time.Duration
	fed      *wiki.Federator
	loadedAt time.Time
	retryAt  time.Time
	localDir string
	mu       sync.Mutex
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcInitializeParams struct {
	ProtocolVersion string `json:"protocolVersion"`
}

// mcpProtocols are the protocol versions this server answers, newest first.
// The tool surface is the same across them; the version only decides which
// revision of the surrounding rules a client applies.
var mcpProtocols = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

// negotiateProtocol echoes a version the client asked for when this server
// knows it, and otherwise answers with the newest one it supports.
func negotiateProtocol(requested string) string {
	for _, v := range mcpProtocols {
		if requested == v {
			return v
		}
	}
	return mcpProtocols[0]
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
	out := fs.String("out", env("OKF_WIKI_OUT", ".scratch/wiki"), "render output directory for a local source")
	base := fs.String("base", env("OKF_WIKI_BASE", wiki.DefaultBase), "URL prefix the local wiki was rendered under")
	var sources sourceFlagList
	var tokens tokenFlagMap = make(map[string]string)
	httpAddr := fs.String("http", "", "serve MCP over HTTP at HOST:PORT instead of stdio")
	fs.Var(&sources, "source", "NAME=BASE_URL, repeated")
	fs.Var(&tokens, "source-token", "NAME=ENV, repeated")
	fs.Parse(reorderArgs(args, fs))
	specs, ttl, err := buildMCPSources(*out, *base, sources, tokens)
	if err != nil {
		fatal(err)
	}
	s, err := newMCPServer(specs, ttl)
	if err != nil {
		fatal(err)
	}
	if *httpAddr != "" {
		host, _, splitErr := net.SplitHostPort(*httpAddr)
		if splitErr != nil || host == "" || host == "0.0.0.0" || host == "::" ||
			(host != "127.0.0.1" && host != "localhost" && host != "::1") {
			fmt.Fprintf(os.Stderr, "okf-wiki: warning: MCP HTTP is exposed without built-in TLS or authentication; put it behind a proxy\n")
		}
		if err := s.serveHTTP(*httpAddr); err != nil {
			fatal(err)
		}
		return
	}
	if err := s.serve(os.Stdin, os.Stdout); err != nil {
		fatal(err)
	}
}

func newMCPServer(specs []mcpSource, ttl time.Duration) (*mcpServer, error) {
	s := &mcpServer{specs: specs, ttl: ttl}
	if err := s.refresh(); err != nil {
		return nil, err
	}
	return s, nil
}

// newLocalMCPServer builds the MCP server for one rendered output. serve uses
// it to mount the endpoint on the wiki listener rather than a second port.
func newLocalMCPServer(out, base string) (*mcpServer, error) {
	specs, ttl, err := buildMCPSources(out, base, nil, nil)
	if err != nil {
		return nil, err
	}
	return newMCPServer(specs, ttl)
}

// refresh reloads the rendered files before each tool call. A long-running
// agent therefore sees a re-render without restarting the MCP process. The
// search and graph files are required; changes and health are optional so a
// site rendered before those files existed still answers search and reads.
func (s *mcpServer) refresh() error {
	if s.fed != nil && s.ttl > 0 && time.Since(s.loadedAt) < s.ttl {
		return nil
	}
	// A running server keeps the last good snapshot when an upstream is
	// temporarily unavailable. Startup still fails loudly when no snapshot
	// exists.
	if s.fed != nil {
		if time.Now().Before(s.retryAt) {
			return nil
		}
		if err := s.loadSources(); err != nil {
			s.retryAt = time.Now().Add(30 * time.Second)
			fmt.Fprintf(os.Stderr, "okf-wiki: source refresh failed: %v (serving cached data)\n", err)
		} else {
			s.retryAt = time.Time{}
		}
		return nil
	}
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if err := s.loadSources(); err == nil {
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

func (s *mcpServer) loadSources() error {
	loaded := make([]*wiki.AgentSource, 0, len(s.specs))
	for _, spec := range s.specs {
		src, err := wiki.LoadAgentSource(spec.name, spec.basePath, spec.read)
		if err != nil {
			return err
		}
		src.Base = spec.base
		loaded = append(loaded, src)
	}
	s.fed = wiki.NewFederator(loaded)
	s.loadedAt = time.Now()
	return nil
}

// serveHTTP exposes the same JSON-RPC handler at one POST endpoint. It is
// intentionally transport-thin: TLS, authentication, quotas and proxying stay
// at the edge, and the process remains stateless apart from its source cache.
func (s *mcpServer) serveHTTP(addr string) error {
	server := &http.Server{
		Addr:              addr,
		Handler:           s.standaloneHandler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return server.ListenAndServe()
}

// standaloneHandler is the route table of `mcp --http`: one endpoint at /mcp.
// It is separate from serveHTTP so the mount can be exercised without a port.
func (s *mcpServer) standaloneHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/mcp", s.httpHandler())
	mux.Handle("/mcp/{$}", s.httpHandler())
	return mux
}

// httpHandler is the endpoint itself. It is path-agnostic so the standalone
// server can mount it at /mcp and serve can mount it under the wiki base.
func (s *mcpServer) httpHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// The wiki listener has no read timeout, so a body drip could otherwise
		// hold this handler open. Size is bounded separately by MaxBytesReader.
		_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(20 * time.Second))
		if ct := req.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
			http.Error(w, "content type must be application/json", http.StatusUnsupportedMediaType)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, req.Body, 4<<20))
		if err != nil {
			http.Error(w, "read request", http.StatusRequestEntityTooLarge)
			return
		}
		resp := s.handle(body)
		if resp == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			return
		}
	})
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
		var params rpcInitializeParams
		_ = json.Unmarshal(req.Params, &params)
		protocol := negotiateProtocol(params.ProtocolVersion)
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": protocol,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "okf-wiki", "version": Version},
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
				mcpSimilarTool(), mcpListSourcesTool(),
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
				"source":    map[string]any{"type": "string", "description": "Source name; omit when one wiki is configured."},
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
				"source":    map[string]any{"type": "string", "description": "Source name; omit when one wiki is configured."},
				"pageId":    map[string]any{"type": "string", "description": "Page ID from search or pages.json."},
				"anchor":    map[string]any{"type": "string", "description": "Heading anchor; empty for the page-level overview."},
				"maxTokens": map[string]any{"type": "integer", "minimum": 1, "maximum": 32000, "default": 2000},
			},
			"required":             []string{"pageId"},
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
				"sources":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional source names; omit for every source."},
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
			"type": "object",
			"properties": map[string]any{
				"source": map[string]any{"type": "string", "description": "Source name; omit when one wiki is configured."},
			},
			"additionalProperties": false,
		},
	}
}

func mcpHealthTool() map[string]any {
	return map[string]any{
		"name":        "health",
		"description": "Return broken links, orphans, tag-only pages, link cycles, stale pages and the most connected pages.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"source": map[string]any{"type": "string", "description": "Source name; omit when one wiki is configured."},
			},
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
				"source": map[string]any{"type": "string", "description": "Source name; omit when one wiki is configured."},
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
				"source":   map[string]any{"type": "string", "description": "Source name; omit when one wiki is configured."},
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

func mcpListSourcesTool() map[string]any {
	return map[string]any{
		"name":        "list_sources",
		"description": "List the wikis this federator can read, with their render IDs and optional change and health surfaces.",
		"inputSchema": map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
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
				"sources": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional source names; omit for every source."},
				"site":    map[string]any{"type": "string", "description": "Wiki title or site key."},
				"tag":     map[string]any{"type": "string", "description": "Page tag."},
				"section": map[string]any{"type": "string", "description": "Section title."},
			},
			"required":             []string{"query"},
			"additionalProperties": false,
		},
	}
}

func mcpSimilarTool() map[string]any {
	return map[string]any{
		"name":        "suggest_pages",
		"description": "Plan a capture against the wiki. Ranks existing subjects a draft should update, with the pages to read first, or answers that it is a new subject. Read-only: it plans and never writes.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text":     map[string]any{"type": "string", "description": "The draft, summary or new source to plan."},
				"limit":    map[string]any{"type": "integer", "minimum": 1, "maximum": 20, "default": 5},
				"sources":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional source names; omit for every source."},
				"minScore": map[string]any{"type": "number", "minimum": 0, "maximum": 1, "description": "Update threshold; omit for the default."},
			},
			"required":             []string{"text"},
			"additionalProperties": false,
		},
	}
}

type mcpCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type mcpSearchArgs struct {
	Query   string   `json:"query"`
	Sources []string `json:"sources"`
	Limit   int      `json:"limit"`
	Site    string   `json:"site"`
	Tag     string   `json:"tag"`
	Section string   `json:"section"`
}

func (s *mcpServer) callTool(raw json.RawMessage) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
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
		return s.changesTool(params.Arguments)
	case "health":
		return s.healthTool(params.Arguments)
	case "suggest_pages":
		return s.similarTool(params.Arguments)
	case "list_sources":
		return mcpToolJSON(s.fed.Info())
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
	resp, err := s.fed.Search(args.Query, wiki.SearchOptions{
		Limit: args.Limit, Site: args.Site, Tag: args.Tag, Section: args.Section,
	}, args.Sources)
	if err != nil {
		return mcpToolError(err.Error())
	}
	return mcpToolJSON(resp)
}

type mcpSimilarArgs struct {
	Text     string   `json:"text"`
	Sources  []string `json:"sources"`
	Limit    int      `json:"limit"`
	MinScore float64  `json:"minScore"`
}

func (s *mcpServer) similarTool(raw json.RawMessage) map[string]any {
	var args mcpSimilarArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return mcpToolError("invalid suggest_pages arguments")
	}
	if strings.TrimSpace(args.Text) == "" {
		return mcpToolError("text is required")
	}
	if args.Limit == 0 {
		args.Limit = 5
	}
	if args.Limit < 1 || args.Limit > 20 {
		return mcpToolError("limit must be between 1 and 20")
	}
	if args.MinScore < 0 || args.MinScore > 1 {
		return mcpToolError("minScore must be between 0 and 1")
	}
	resp, err := s.fed.Similar(args.Text, wiki.SimilarOptions{
		Limit: args.Limit, MinScore: args.MinScore,
	}, args.Sources)
	if err != nil {
		return mcpToolError(err.Error())
	}
	return mcpToolJSON(resp)
}

type mcpReadPageArgs struct {
	Source    string `json:"source"`
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
	page, err := s.fed.ReadPage(args.Source, args.PageID, args.MaxTokens)
	if err != nil {
		return mcpToolError(err.Error())
	}
	return mcpToolJSON(page)
}

type mcpReadSectionArgs struct {
	Source    string `json:"source"`
	PageID    string `json:"pageId"`
	Anchor    string `json:"anchor"`
	MaxTokens int    `json:"maxTokens"`
}

func (s *mcpServer) readSectionTool(raw json.RawMessage) map[string]any {
	var args mcpReadSectionArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return mcpToolError("invalid read_section arguments")
	}
	if args.PageID == "" {
		return mcpToolError("pageId is required")
	}
	if args.MaxTokens < 0 || args.MaxTokens > 32000 {
		return mcpToolError("maxTokens must be between 1 and 32000")
	}
	section, err := s.fed.ReadSection(args.Source, args.PageID, args.Anchor, args.MaxTokens)
	if err != nil {
		return mcpToolError(err.Error())
	}
	return mcpToolJSON(section)
}

type mcpContextArgs struct {
	Query     string   `json:"query"`
	Sources   []string `json:"sources"`
	MaxTokens int      `json:"maxTokens"`
	Limit     int      `json:"limit"`
	Site      string   `json:"site"`
	Tag       string   `json:"tag"`
	Section   string   `json:"section"`
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
	resp, err := s.fed.ContextFor(args.Query, wiki.SearchOptions{
		Limit: args.Limit, Site: args.Site, Tag: args.Tag, Section: args.Section,
	}, args.MaxTokens, args.Sources)
	if err != nil {
		return mcpToolError(err.Error())
	}
	return mcpToolJSON(resp)
}

type mcpNeighborsArgs struct {
	Source string   `json:"source"`
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
	resp, err := s.fed.Neighbors(args.Source, args.PageID, args.Depth, args.Kinds)
	if err != nil {
		return mcpToolError(err.Error())
	}
	return mcpToolJSON(resp)
}

type mcpPathArgs struct {
	Source   string   `json:"source"`
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
	resp, err := s.fed.Path(args.Source, args.From, args.To, args.MaxDepth, args.Kinds)
	if err != nil {
		return mcpToolError(err.Error())
	}
	return mcpToolJSON(resp)
}

type mcpSourceArg struct {
	Source string `json:"source"`
}

func (s *mcpServer) changesTool(raw json.RawMessage) map[string]any {
	var args mcpSourceArg
	if err := json.Unmarshal(raw, &args); err != nil {
		return mcpToolError("invalid changes arguments")
	}
	resp, err := s.fed.Changes(args.Source)
	if err != nil {
		return mcpToolError(err.Error())
	}
	return mcpToolJSON(resp)
}

func (s *mcpServer) healthTool(raw json.RawMessage) map[string]any {
	var args mcpSourceArg
	if err := json.Unmarshal(raw, &args); err != nil {
		return mcpToolError("invalid health arguments")
	}
	resp, err := s.fed.Health(args.Source)
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
