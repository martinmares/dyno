package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mares/dyno/internal/mcpengine"
)

func TestMCPInitializeAndToolsList(t *testing.T) {
	engine, err := mcpengine.Load([]string{filepath.Join("..", "..", "site")}, "https://docs.example.test")
	if err != nil {
		t.Fatal(err)
	}
	server := New(Config{
		Name:    "dyno-mcp",
		Version: "test",
		Engine:  engine,
	})

	responses, notificationsOnly, err := serverTestHandle(server, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}`)
	if err != nil {
		t.Fatal(err)
	}
	if notificationsOnly || len(responses) != 1 {
		t.Fatalf("unexpected initialize response count: %d", len(responses))
	}
	if !strings.Contains(responses[0], `"protocolVersion":"2025-03-26"`) {
		t.Fatalf("initialize response missing protocol version: %s", responses[0])
	}

	responses, notificationsOnly, err = serverTestHandle(server, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if err != nil {
		t.Fatal(err)
	}
	if notificationsOnly || len(responses) != 1 {
		t.Fatalf("unexpected tools/list response count: %d", len(responses))
	}
	if !strings.Contains(responses[0], `"search_docs"`) || !strings.Contains(responses[0], `"get_page"`) {
		t.Fatalf("tools/list response missing expected tools: %s", responses[0])
	}
	if !strings.Contains(responses[0], `"list_pages"`) || !strings.Contains(responses[0], `"get_page_section"`) {
		t.Fatalf("tools/list response missing new tools: %s", responses[0])
	}
}

func TestMCPSearchDocsToolCall(t *testing.T) {
	engine, err := mcpengine.Load([]string{filepath.Join("..", "..", "site")}, "https://docs.example.test")
	if err != nil {
		t.Fatal(err)
	}
	server := New(Config{
		Name:    "dyno-mcp",
		Version: "test",
		Engine:  engine,
	})

	responses, notificationsOnly, err := serverTestHandle(server, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"search_docs","arguments":{"query":"D2","limit":2}}}`)
	if err != nil {
		t.Fatal(err)
	}
	if notificationsOnly || len(responses) != 1 {
		t.Fatalf("unexpected tools/call response count: %d", len(responses))
	}
	if !strings.Contains(responses[0], `"structuredContent"`) || !strings.Contains(responses[0], `https://docs.example.test/guides/d2-diagrams`) {
		t.Fatalf("tool result missing structured content or URL: %s", responses[0])
	}
}

func TestMCPResourcesAndHTTPValidation(t *testing.T) {
	engine, err := mcpengine.Load([]string{filepath.Join("..", "..", "site")}, "https://docs.example.test")
	if err != nil {
		t.Fatal(err)
	}
	server := New(Config{
		Name:         "dyno-mcp",
		Version:      "test",
		Engine:       engine,
		AuthToken:    "secret",
		AllowOrigins: []string{"https://chat.example.test"},
	})

	responses, notificationsOnly, err := serverTestHandle(server, `{"jsonrpc":"2.0","id":4,"method":"resources/list","params":{"cursor":""}}`)
	if err != nil {
		t.Fatal(err)
	}
	if notificationsOnly || len(responses) != 1 || !strings.Contains(responses[0], `"dyno://book/_default"`) {
		t.Fatalf("unexpected resources/list response: %v %v", notificationsOnly, responses)
	}

	responses, notificationsOnly, err = serverTestHandle(server, `{"jsonrpc":"2.0","id":5,"method":"resources/read","params":{"uri":"dyno://book/_default/page?path=%2Fguides%2Fd2-diagrams"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if notificationsOnly || len(responses) != 1 || !strings.Contains(responses[0], `"text/markdown"`) {
		t.Fatalf("unexpected resources/read response: %v %v", notificationsOnly, responses)
	}

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	req.Header.Set("Origin", "https://chat.example.test")
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("MCP-Protocol-Version", ProtocolVersion)
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("MCP-Protocol-Version") != ProtocolVersion {
		t.Fatalf("missing MCP-Protocol-Version response header")
	}

	badReq := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	badReq.Header.Set("Origin", "https://chat.example.test")
	badReq.Header.Set("Authorization", "Bearer secret")
	badReq.Header.Set("MCP-Protocol-Version", "2099-01-01")
	badRR := httptest.NewRecorder()
	server.ServeHTTP(badRR, badReq)
	if badRR.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unsupported MCP-Protocol-Version, got %d", badRR.Code)
	}

	unauthReq := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	unauthReq.Header.Set("Origin", "https://chat.example.test")
	unauthRR := httptest.NewRecorder()
	server.ServeHTTP(unauthRR, unauthReq)
	if unauthRR.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", unauthRR.Code)
	}
	if !strings.Contains(unauthRR.Header().Get("WWW-Authenticate"), "Bearer") {
		t.Fatal("expected WWW-Authenticate Bearer header")
	}
}

func serverTestHandle(server *Server, payload string) ([]string, bool, error) {
	responses, notificationsOnly, err := server.handlePayload(strings.NewReader(payload))
	if err != nil {
		return nil, false, err
	}
	encoded := make([]string, 0, len(responses))
	for _, response := range responses {
		data, err := json.Marshal(response)
		if err != nil {
			return nil, false, err
		}
		encoded = append(encoded, string(data))
	}
	return encoded, notificationsOnly, nil
}
