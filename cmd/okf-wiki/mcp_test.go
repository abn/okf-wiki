package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abn/okf-wiki/internal/wiki"
)

func testMCPServer(t *testing.T) *mcpServer {
	t.Helper()
	dir := t.TempDir()
	if err := writeTestPage(filepath.Join(dir, "index.md"), "# Home\n\nThe install guide starts here.\n"); err != nil {
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
	corpus, err := wiki.LoadSearchCorpus(filepath.Join(out, "wiki"))
	if err != nil {
		t.Fatal(err)
	}
	return &mcpServer{corpus: corpus}
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
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %+v", result["tools"])
	}
	tool := tools[0].(map[string]any)
	if tool["name"] != "search_wiki" {
		t.Errorf("tool name = %v", tool["name"])
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
