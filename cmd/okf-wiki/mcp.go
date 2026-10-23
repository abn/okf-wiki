package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/abn/okf-wiki/internal/wiki"
)

// mcpServer exposes the rendered corpus over the Model Context Protocol stdio
// transport. It is deliberately read-only: search, page and section reads, and
// budgeted context packs all return citations, but nothing writes to the wiki.
type mcpServer struct {
	corpus *wiki.SearchCorpus
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
	corpus, err := wiki.LoadSearchCorpus(siteDir, *base)
	if err != nil {
		fatal(fmt.Errorf("load rendered search data: %w", err))
	}
	s := &mcpServer{corpus: corpus}
	if err := s.serve(os.Stdin, os.Stdout); err != nil {
		fatal(err)
	}
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
