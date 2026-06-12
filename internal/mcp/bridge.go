package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/openpaw/openpaw/internal/logger"
)

// protocolVersion is the MCP revision this bridge speaks. The bridge only
// serves tools (no resources/prompts/sampling), which is identical across
// revisions, so we echo whatever the client requests and fall back to this.
const protocolVersion = "2025-03-26"

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Handler serves the streamable-HTTP MCP endpoint at /mcp/{token}. It
// implements the minimal JSON-RPC surface CLI clients need for tool calling:
// initialize, notifications/initialized, ping, tools/list, tools/call.
func (r *Registry) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		token := chi.URLParam(req, "token")
		session := r.Get(token)
		if session == nil {
			http.Error(w, "unknown or expired MCP session", http.StatusUnauthorized)
			return
		}

		switch req.Method {
		case http.MethodPost:
			r.handlePost(w, req, session)
		case http.MethodDelete:
			w.WriteHeader(http.StatusOK)
		default:
			// No standalone SSE stream — the spec allows 405 here.
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func (r *Registry) handlePost(w http.ResponseWriter, req *http.Request, session *Session) {
	body, err := io.ReadAll(io.LimitReader(req.Body, 10<<20))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	var rpc rpcRequest
	if err := json.Unmarshal(body, &rpc); err != nil {
		writeRPC(w, rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "parse error"}})
		return
	}

	// Notifications (no id) get 202 with no body.
	if len(rpc.ID) == 0 || string(rpc.ID) == "null" {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	resp := rpcResponse{JSONRPC: "2.0", ID: rpc.ID}

	switch rpc.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		json.Unmarshal(rpc.Params, &params)
		version := params.ProtocolVersion
		if version == "" {
			version = protocolVersion
		}
		resp.Result = map[string]interface{}{
			"protocolVersion": version,
			"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
			"serverInfo":      map[string]interface{}{"name": "openpaw", "version": "1.0"},
		}
	case "ping":
		resp.Result = map[string]interface{}{}
	case "tools/list":
		tools := make([]mcpTool, 0, len(session.Tools))
		for _, t := range session.Tools {
			schema := t.Function.Parameters
			if len(schema) == 0 {
				schema = json.RawMessage(`{"type":"object"}`)
			}
			tools = append(tools, mcpTool{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				InputSchema: schema,
			})
		}
		resp.Result = map[string]interface{}{"tools": tools}
	case "tools/call":
		resp.Result, resp.Error = r.callTool(req, session, rpc.Params)
	default:
		resp.Error = &rpcError{Code: -32601, Message: fmt.Sprintf("method %q not supported", rpc.Method)}
	}

	writeRPC(w, resp)
}

func (r *Registry) callTool(req *http.Request, session *Session, params json.RawMessage) (interface{}, *rpcError) {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return nil, &rpcError{Code: -32602, Message: "invalid params"}
	}

	handler, ok := session.Handlers[call.Name]
	if !ok {
		return nil, &rpcError{Code: -32602, Message: fmt.Sprintf("unknown tool %q", call.Name)}
	}

	args := call.Arguments
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}

	logger.Info("MCP tool call: %s (agent=%s)", call.Name, session.AgentSlug)
	result := handler(req.Context(), session.WorkDir, args)

	if result.ImageURL != "" {
		session.setImageURL(result.ImageURL)
	}

	return map[string]interface{}{
		"content": []textContent{{Type: "text", Text: result.Output}},
		"isError": result.IsError,
	}, nil
}

func writeRPC(w http.ResponseWriter, resp rpcResponse) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
