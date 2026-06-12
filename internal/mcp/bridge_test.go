package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	llm "github.com/openpaw/openpaw/internal/llm"
)

func newTestServer(t *testing.T) (*httptest.Server, *Registry, *Session) {
	t.Helper()
	registry := NewRegistry()

	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"note": map[string]interface{}{"type": "string"},
		},
	})
	session := registry.Create(&Session{
		AgentSlug: "tester",
		ThreadID:  "thread-1",
		Tools: []llm.ToolDef{{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        "save_note",
				Description: "Save a note",
				Parameters:  params,
			},
		}},
		Handlers: map[string]llm.ToolHandler{
			"save_note": func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
				var req struct {
					Note string `json:"note"`
				}
				json.Unmarshal(input, &req)
				return llm.ToolResult{Output: "saved: " + req.Note, ImageURL: "/media/img-1"}
			},
		},
	})

	r := chi.NewRouter()
	r.HandleFunc("/mcp/{token}", registry.Handler())
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, registry, session
}

func rpcCall(t *testing.T, url string, body string) map[string]interface{} {
	t.Helper()
	resp, err := http.Post(url, "application/json", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestBridgeRoundTrip(t *testing.T) {
	srv, _, session := newTestServer(t)
	url := srv.URL + "/mcp/" + session.Token

	// initialize
	init := rpcCall(t, url, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`)
	result := init["result"].(map[string]interface{})
	if result["protocolVersion"] != "2025-06-18" {
		t.Errorf("protocolVersion = %v", result["protocolVersion"])
	}

	// tools/list
	list := rpcCall(t, url, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	tools := list["result"].(map[string]interface{})["tools"].([]interface{})
	if len(tools) != 1 {
		t.Fatalf("tools = %d, want 1", len(tools))
	}
	if tools[0].(map[string]interface{})["name"] != "save_note" {
		t.Errorf("tool name = %v", tools[0])
	}

	// tools/call
	call := rpcCall(t, url, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"save_note","arguments":{"note":"remember this"}}}`)
	callResult := call["result"].(map[string]interface{})
	content := callResult["content"].([]interface{})[0].(map[string]interface{})
	if content["text"] != "saved: remember this" {
		t.Errorf("tool output = %v", content["text"])
	}
	if callResult["isError"] != false {
		t.Errorf("isError = %v", callResult["isError"])
	}

	// ImageURL capture for AgentResult parity
	if session.ImageURL() != "/media/img-1" {
		t.Errorf("ImageURL = %q", session.ImageURL())
	}

	// unknown tool
	bad := rpcCall(t, url, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"nope"}}`)
	if bad["error"] == nil {
		t.Error("unknown tool should return an error")
	}
}

func TestBridgeAuth(t *testing.T) {
	srv, registry, session := newTestServer(t)

	// Wrong token → 401
	resp, _ := http.Post(srv.URL+"/mcp/wrong-token", "application/json", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong token status = %d, want 401", resp.StatusCode)
	}

	// Released token → 401
	registry.Release(session.Token)
	resp2, _ := http.Post(srv.URL+"/mcp/"+session.Token, "application/json", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Errorf("released token status = %d, want 401", resp2.StatusCode)
	}
}

func TestBridgeNotification(t *testing.T) {
	srv, _, session := newTestServer(t)
	resp, err := http.Post(srv.URL+"/mcp/"+session.Token, "application/json",
		strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("notification status = %d, want 202", resp.StatusCode)
	}
}
