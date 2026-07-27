package agents

import (
	"context"
	"encoding/json"
	"strings"

	llm "github.com/openpaw/openpaw/internal/llm"
)

// BuildRequester hands a specialist agent's build request to the same machinery
// the gateway uses — work order, confirmation card, builder agent.
//
// Injected rather than called directly because starting a build has to post
// into a chat thread, which lives in handlers, and handlers already imports this
// package. Same reason as TmuxWatchFn.
//
// kind is "service" or "dashboard". Returns the sentence to show the agent.
type BuildRequester func(ctx context.Context, threadID, kind, title, description, requirements string) (string, error)

// BuildRequestToolDefs returns the request_build tool.
func BuildRequestToolDefs() []llm.ToolDef {
	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"kind": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"service", "dashboard"},
				"description": "\"service\" for a running HTTP service other agents can call; \"dashboard\" for a visual page in the Dashboards tab.",
			},
			"title": map[string]interface{}{
				"type":        "string",
				"description": "Short name for the thing being built (e.g. \"Feedback Lookup\"). For an update, the exact existing name.",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "One or two sentences on what it does, for the user to read before approving.",
			},
			"requirements": map[string]interface{}{
				"type":        "string",
				"description": "The full brief for the builder: endpoints, request/response shapes, auth, data sources, behaviour, edge cases. The builder cannot see this conversation — everything it needs must be in here.",
			},
		},
		"required": []string{"kind", "title", "requirements"},
	})
	return []llm.ToolDef{{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "request_build",
			Description: "Hand a service or dashboard to the builder to be created or updated. Use this the moment the user asks you to build one — you do not build it yourself, and you must not tell the user to go and ask someone else.",
			Parameters:  params,
		},
	}}
}

// MakeBuildRequestHandler returns the request_build handler for one thread.
func (m *Manager) MakeBuildRequestHandler(threadID string) map[string]llm.ToolHandler {
	return map[string]llm.ToolHandler{
		"request_build": func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
			var params struct {
				Kind         string `json:"kind"`
				Title        string `json:"title"`
				Description  string `json:"description"`
				Requirements string `json:"requirements"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
			}
			if m.BuildRequestFn == nil {
				return llm.ToolResult{Output: "Builds are not available in this context.", IsError: true}
			}

			kind := strings.ToLower(strings.TrimSpace(params.Kind))
			// Models reach for the vocabulary in the conversation ("tool", "API",
			// "page"), not the two enum values.
			switch kind {
			case "service", "tool", "api", "microservice", "integration":
				kind = "service"
			case "dashboard", "page", "view", "report":
				kind = "dashboard"
			default:
				return llm.ToolResult{Output: "kind must be \"service\" or \"dashboard\".", IsError: true}
			}
			if strings.TrimSpace(params.Title) == "" {
				return llm.ToolResult{Output: "title is required.", IsError: true}
			}
			if strings.TrimSpace(params.Requirements) == "" {
				return llm.ToolResult{Output: "requirements is required — the builder cannot see this conversation.", IsError: true}
			}

			out, err := m.BuildRequestFn(ctx, threadID, kind, params.Title, params.Description, params.Requirements)
			if err != nil {
				return llm.ToolResult{Output: "Could not start the build: " + err.Error(), IsError: true}
			}
			return llm.ToolResult{Output: out}
		},
	}
}

// buildRequestPromptSection tells an agent that building is something it can
// start, not something to redirect the user about.
func buildRequestPromptSection() string {
	return `## BUILDING SERVICES AND DASHBOARDS

You can have a service or dashboard built without leaving this conversation: call ` + "`request_build`" + `. It hands the work to the builder, which writes and compiles the code and reports back in this thread.

- Asked to build, create, set up, or update a service, API, integration, or dashboard? Call ` + "`request_build`" + `. Do not answer that you are unable to, and never tell the user to ask the Gateway or anyone else — you are the one holding the context, so you are the one who files it.
- Put everything the builder needs in ` + "`requirements`" + `. It does not see this conversation: spell out endpoints, parameters, response shapes, auth, data sources and error behaviour. A brief that assumes context produces the wrong thing.
- Gather what you are missing first. Ask the user about credentials or unclear behaviour before filing, not after.
- The user may be asked to approve the build. Once you have called the tool your part is done — say what you asked for and stop; do not call it again for the same thing.`
}
