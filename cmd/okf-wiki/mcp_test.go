package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abn/okf-wiki/internal/wiki"
)

func testMCPServer(t *testing.T) *mcpServer {
	t.Helper()
	dir := t.TempDir()
	if err := writeTestPage(filepath.Join(dir, "index.md"), "# Home\n\n## Install\n\nThe install guide starts here.\n"); err != nil {
		t.Fatal(err)
	}
	r, err := wiki.New(wiki.Config{Content: dir, Base: "/wiki/", Brand: wiki.Brand{Name: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := r.RenderAll(out); err != nil {
		t.Fatal(err)
	}
	siteDir := filepath.Join(out, "wiki")
	s, err := newLocalMCPServer(out, "/wiki/")
	if err != nil {
		t.Fatal(err)
	}
	s.localDir = siteDir
	return s
}

func writeTestPage(path, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(body), 0o644)
}

// TestMCPInitializeAndTools covers the handshake and tool advertisement.
func TestMCPInitializeAndTools(t *testing.T) {
	s := testMCPServer(t)
	resp := s.handle([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	if resp == nil || resp.Error != nil {
		t.Fatalf("initialize = %+v", resp)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok || result["protocolVersion"] == "" {
		t.Errorf("initialize result = %+v", resp.Result)
	}
	resp = s.handle([]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`))
	if resp == nil || resp.Error != nil {
		t.Fatalf("tools/list = %+v", resp)
	}
	result = resp.Result.(map[string]any)
	tools, ok := result["tools"].([]any)
	if !ok || len(tools) != 9 {
		t.Fatalf("tools = %+v", result["tools"])
	}
	names := map[string]bool{}
	for _, item := range tools {
		names[item.(map[string]any)["name"].(string)] = true
	}
	for _, want := range []string{"search_wiki", "read_page", "read_section", "context_for", "neighbors", "path", "changes", "health", "list_sources"} {
		if !names[want] {
			t.Errorf("tools are missing %q: %v", want, names)
		}
	}
}

// TestMCPToolSearch covers the search result payload: the text is the same JSON
// shape the CLI writes, so an agent has one result contract.
func TestMCPToolSearch(t *testing.T) {
	s := testMCPServer(t)
	line := []byte(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"search_wiki","arguments":{"query":"install","limit":2}}}`)
	resp := s.handle(line)
	if resp == nil || resp.Error != nil {
		t.Fatalf("tools/call = %+v", resp)
	}
	result := resp.Result.(map[string]any)
	content, ok := result["content"].([]any)
	if !ok || len(content) != 1 {
		t.Fatalf("content = %+v", result["content"])
	}
	text := content[0].(map[string]any)["text"].(string)
	var search wiki.SearchResponse
	if err := json.Unmarshal([]byte(text), &search); err != nil {
		t.Fatal(err)
	}
	if search.Total == 0 || len(search.Results) == 0 || search.Results[0].PageID != "page:index" {
		t.Errorf("search payload = %+v", search)
	}
	if result["isError"] != false {
		t.Errorf("isError = %v", result["isError"])
	}
}

func TestMCPNotificationsAndErrors(t *testing.T) {
	s := testMCPServer(t)
	if resp := s.handle([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)); resp != nil {
		t.Errorf("notification produced %+v", resp)
	}
	resp := s.handle([]byte(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"missing","arguments":{}}}`))
	if resp == nil {
		t.Fatal("unknown tool produced no response")
	}
	result := resp.Result.(map[string]any)
	if result["isError"] != true {
		t.Errorf("unknown tool result = %+v", result)
	}
	resp = s.handle([]byte(`{"jsonrpc":"2.0","id":5,"method":"missing"}`))
	if resp == nil || resp.Error == nil || resp.Error.Code != -32601 {
		t.Errorf("unknown method = %+v", resp)
	}
	resp = s.handle([]byte(`not json`))
	if resp == nil || resp.Error == nil || resp.Error.Code != -32700 {
		t.Fatalf("parse error = %+v", resp)
	}
	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"id":null`) {
		t.Errorf("parse error has no null id: %s", body)
	}
}

// TestMCPReadAndContext covers the tools that turn a search hit into exact
// source text and a budgeted context pack.
func TestMCPReadAndContext(t *testing.T) {
	s := testMCPServer(t)
	cases := []struct {
		name string
		line string
		want string
	}{
		{
			name: "read_page",
			line: `{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"read_page","arguments":{"pageId":"page:index","maxTokens":100}}}`,
			want: "The install guide",
		},
		{
			name: "read_section",
			line: `{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"read_section","arguments":{"pageId":"page:index","anchor":"install","maxTokens":100}}}`,
			want: "The install guide",
		},
		{
			name: "context_for",
			line: `{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"context_for","arguments":{"query":"install","maxTokens":100}}}`,
			want: "The install guide",
		},
		{
			name: "neighbors",
			line: `{"jsonrpc":"2.0","id":13,"method":"tools/call","params":{"name":"neighbors","arguments":{"pageId":"page:index","depth":1}}}`,
			want: "page:index",
		},
		{
			name: "path",
			line: `{"jsonrpc":"2.0","id":14,"method":"tools/call","params":{"name":"path","arguments":{"from":"page:index","to":"page:index"}}}`,
			want: "\"found\":true",
		},
		{
			name: "changes",
			line: `{"jsonrpc":"2.0","id":15,"method":"tools/call","params":{"name":"changes","arguments":{}}}`,
			want: "\"changes\":[",
		},
		{
			name: "health",
			line: `{"jsonrpc":"2.0","id":16,"method":"tools/call","params":{"name":"health","arguments":{}}}`,
			want: "\"brokenLinks\":[]",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := s.handle([]byte(tc.line))
			if resp == nil || resp.Error != nil {
				t.Fatalf("response = %+v", resp)
			}
			result := resp.Result.(map[string]any)
			content := result["content"].([]any)[0].(map[string]any)["text"].(string)
			if !strings.Contains(content, tc.want) {
				t.Errorf("%s content = %s", tc.name, content)
			}
		})
	}
}

// TestMCPOptionalState keeps search and reads working on an output that predates
// changes.json and health.json, while those tools fail clearly.
func TestMCPOptionalState(t *testing.T) {
	s := testMCPServer(t)
	for _, name := range []string{"changes.json", "health.json"} {
		if err := os.Remove(filepath.Join(s.localDir, name)); err != nil {
			t.Fatal(err)
		}
	}
	resp := s.handle([]byte(`{"jsonrpc":"2.0","id":20,"method":"tools/call","params":{"name":"search_wiki","arguments":{"query":"install"}}}`))
	if resp == nil || resp.Error != nil {
		t.Fatalf("search = %+v", resp)
	}
	result := resp.Result.(map[string]any)
	if result["isError"] != false {
		t.Errorf("search failed after removing optional state: %+v", result)
	}
	resp = s.handle([]byte(`{"jsonrpc":"2.0","id":21,"method":"tools/call","params":{"name":"changes","arguments":{}}}`))
	if resp == nil {
		t.Fatal("changes produced no response")
	}
	if resp.Result.(map[string]any)["isError"] != true {
		t.Error("changes did not report missing changes.json")
	}
}

// TestMCPMalformedOptionalState keeps a corrupt optional file from taking down
// search and reads.
func TestMCPMalformedOptionalState(t *testing.T) {
	s := testMCPServer(t)
	if err := os.WriteFile(filepath.Join(s.localDir, "changes.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	resp := s.handle([]byte(`{"jsonrpc":"2.0","id":30,"method":"tools/call","params":{"name":"search_wiki","arguments":{"query":"install"}}}`))
	if resp == nil || resp.Result.(map[string]any)["isError"] != false {
		t.Fatalf("search after malformed state = %+v", resp)
	}
	resp = s.handle([]byte(`{"jsonrpc":"2.0","id":31,"method":"tools/call","params":{"name":"changes","arguments":{}}}`))
	if resp == nil || resp.Result.(map[string]any)["isError"] != true {
		t.Errorf("changes after malformed state = %+v", resp)
	}
}

// A client gets the protocol version it asked for when the server knows it, and
// the newest supported one otherwise. The tool surface does not change, but the
// answer decides which transport rules the client applies.
func TestMCPProtocolNegotiation(t *testing.T) {
	s := testMCPServer(t)
	tests := []struct {
		requested string
		want      string
	}{
		{"2025-06-18", "2025-06-18"},
		{"2025-03-26", "2025-03-26"},
		{"2024-11-05", "2024-11-05"},
		{"1999-01-01", mcpProtocols[0]},
		{"", mcpProtocols[0]},
	}
	for _, tc := range tests {
		line := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"` + tc.requested + `"}}`)
		resp := s.handle(line)
		if resp == nil || resp.Error != nil {
			t.Fatalf("initialize(%q) = %+v", tc.requested, resp)
		}
		got := resp.Result.(map[string]any)["protocolVersion"]
		if got != tc.want {
			t.Errorf("initialize(%q) protocol = %v, want %q", tc.requested, got, tc.want)
		}
	}
}

// TestMCPHTTPHandler covers the transport wrapper around the same JSON-RPC
// handler used by stdio. The handler is path-agnostic so serve can mount it
// under the wiki base; this test pins the standalone mount at /mcp.
func TestMCPHTTPHandler(t *testing.T) {
	s := testMCPServer(t)
	server := httptest.NewServer(s.standaloneHandler())
	defer server.Close()
	for _, path := range []string{"/mcp", "/mcp/"} {
		body := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
		resp, err := http.Post(server.URL+path, "application/json", body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			t.Fatalf("POST %s status = %d", path, resp.StatusCode)
		}
		var envelope rpcResponse
		err = json.NewDecoder(resp.Body).Decode(&envelope)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if envelope.Result == nil {
			t.Errorf("POST %s response = %+v", path, envelope)
		}
	}
	resp, err := http.Post(server.URL+"/other", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("POST /other status = %d, want 404", resp.StatusCode)
	}
	resp, err = http.Get(server.URL + "/mcp")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET status = %d, want 405", resp.StatusCode)
	}
}

func TestBuildMCPSources(t *testing.T) {
	t.Setenv("DOCS_TOKEN", "secret")
	specs, ttl, err := buildMCPSources("", "/wiki/", []string{"docs=http://127.0.0.1:9/wiki/"}, map[string]string{"docs": "DOCS_TOKEN"})
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 || specs[0].name != "docs" || specs[0].basePath != "/wiki/" || ttl != 30*time.Second {
		t.Fatalf("specs = %+v, ttl = %s", specs, ttl)
	}
	if _, _, err := buildMCPSources("", "/wiki/", []string{"docs=http://example.invalid/wiki/"}, map[string]string{"missing": "DOCS_TOKEN"}); err == nil {
		t.Error("an unknown source token was accepted")
	}
	if specs, ttl, err := buildMCPSources("", "/wiki/", nil, nil); err != nil || len(specs) != 1 || specs[0].name != "local" || ttl != 0 {
		t.Errorf("local source = %+v, ttl %s, err %v", specs, ttl, err)
	}
}
