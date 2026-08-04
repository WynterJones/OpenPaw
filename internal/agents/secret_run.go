package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	llm "github.com/openpaw/openpaw/internal/llm"
	"github.com/openpaw/openpaw/internal/tmux"
)

// run_with_secrets: use a credential without reading it.
//
// get_secret hands over a decrypted value, which then has to go somewhere — and
// the only place an agent can put it is a command it composes, which means the
// value passes through the context window on the way. From there it is in the
// provider's logs, in the chat transcript, and one careless echo from the
// screen. That is a real cost paid every time a deploy needs a token.
//
// This closes the gap: name the secrets, and OpenPaw resolves them into the
// environment of the command itself. The agent writes `railway up`, not
// `RAILWAY_TOKEN=… railway up`, and never sees the token at all.

// BuildSecretRunToolDefs returns run_with_secrets, which needs a decryptor for
// the same reason get_secret does — without one there is nothing to inject.
func BuildSecretRunToolDefs(dec SecretDecryptor) []llm.ToolDef {
	if dec == nil {
		return nil
	}
	params, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type": "string",
				"description": "The shell command to run, written as if the credentials were already in the " +
					"environment — e.g. \"railway up\" or \"gh release create v1.2.0\". " +
					"Do not interpolate any values yourself.",
			},
			"secrets": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
				"description": "Names of the secrets to put in the environment, e.g. " +
					"[\"RAILWAY_TOKEN\"]. Each is set under its own name.",
			},
			"session": map[string]interface{}{
				"type":        "string",
				"description": "Optional name for the tmux session, so you can find it again.",
			},
			"reason": map[string]interface{}{
				"type":        "string",
				"description": "Short reason these credentials are needed. Recorded in the audit log.",
			},
			"watch": map[string]interface{}{
				"type":        "boolean",
				"description": "Report back into this chat when the command finishes. Defaults to true.",
				"default":     true,
			},
		},
		"required": []string{"command", "secrets", "reason"},
	})

	return []llm.ToolDef{{
		Type: "function",
		Function: llm.FunctionDef{
			Name: "run_with_secrets",
			Description: "Run a command with stored secrets set as environment variables, WITHOUT the values " +
				"ever being shown to you. PREFER THIS over get_secret whenever the credential is only needed " +
				"to run something — deploys, API calls from the shell, CLI logins. The command runs detached " +
				"in tmux like tmux_run. Use get_secret only when you genuinely need to read the value itself, " +
				"such as writing it into a config file the user asked for.",
			Parameters: params,
		},
	}}
}

// MakeSecretRunHandler builds the run_with_secrets handler. Returns nil when
// there is no decryptor, matching BuildSecretRunToolDefs.
func (m *Manager) MakeSecretRunHandler(threadID, agentSlug string) map[string]llm.ToolHandler {
	if m.SecretsMgr == nil {
		return nil
	}
	return map[string]llm.ToolHandler{
		"run_with_secrets": m.handleRunWithSecrets(threadID, agentSlug),
	}
}

func (m *Manager) handleRunWithSecrets(threadID, agentSlug string) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var req struct {
			Command string   `json:"command"`
			Secrets []string `json:"secrets"`
			Session string   `json:"session"`
			Reason  string   `json:"reason"`
			Watch   *bool    `json:"watch"`
		}
		json.Unmarshal(input, &req)

		if strings.TrimSpace(req.Command) == "" {
			return llm.ToolResult{Output: "command is required", IsError: true}
		}
		if len(req.Secrets) == 0 {
			return llm.ToolResult{Output: "secrets is required — name at least one, or use tmux_run instead", IsError: true}
		}
		if !tmux.Available() {
			return llm.ToolResult{Output: "tmux is not installed on this machine, so this cannot be run detached.", IsError: true}
		}

		env, missing, err := m.resolveSecretEnv(req.Secrets, req.Reason, agentSlug)
		if err != nil {
			return llm.ToolResult{Output: err.Error(), IsError: true}
		}
		// Nothing is run with a credential missing: a deploy that starts without
		// its token fails halfway through having already done something, which is
		// worse than not starting.
		if len(missing) > 0 {
			return llm.ToolResult{Output: fmt.Sprintf(
				"Not running it — these secrets are not configured: %s. "+
					"Ask the user to add them in Settings → Secrets.", strings.Join(missing, ", ")), IsError: true}
		}

		label := req.Session
		if strings.TrimSpace(label) == "" {
			label = firstWords(req.Command, 4)
		}
		name := uniqueSessionName(ctx, tmux.SessionName(label))

		if err := tmux.StartWithEnv(ctx, name, workDir, req.Command, env); err != nil {
			return llm.ToolResult{Output: "Failed to start the session: " + err.Error(), IsError: true}
		}

		names := make([]string, 0, len(env))
		for n := range env {
			names = append(names, n)
		}
		out := fmt.Sprintf("Started %q in tmux with %s set in its environment, running: %s",
			name, strings.Join(names, ", "), req.Command)

		if req.Watch != nil && !*req.Watch {
			return llm.ToolResult{Output: out + "\nNot watching it — check it with tmux_status."}
		}
		if threadID == "" || m.TmuxWatchFn == nil {
			return llm.ToolResult{Output: out + "\nReporting back is not available here, so check it with tmux_status."}
		}
		if err := m.TmuxWatchFn(threadID, name, 0); err != nil {
			return llm.ToolResult{Output: out + "\nCould not start watching it: " + err.Error()}
		}
		return llm.ToolResult{Output: out + "\nWatching it — I'll post here when it finishes."}
	}
}

// resolveSecretEnv decrypts the named secrets into an environment map. The
// values are returned to the caller for injection and never placed in a tool
// result; only the names come back to the model.
func (m *Manager) resolveSecretEnv(names []string, reason, agentSlug string) (map[string]string, []string, error) {
	env := make(map[string]string, len(names))
	var missing []string

	actor := "system"
	if agentSlug != "" {
		actor = "agent:" + agentSlug
	}

	for _, requested := range names {
		requested = strings.TrimSpace(requested)
		if requested == "" {
			continue
		}

		var id, actualName, encrypted string
		err := m.db.QueryRow(
			"SELECT id, name, encrypted_value FROM secrets WHERE LOWER(name) = LOWER(?)",
			requested,
		).Scan(&id, &actualName, &encrypted)
		if err != nil {
			missing = append(missing, requested)
			continue
		}

		value, err := m.SecretsMgr.Decrypt(encrypted)
		if err != nil {
			return nil, nil, fmt.Errorf(
				"secret %q exists but could not be decrypted (the encryption key may have changed). "+
					"Ask the user to re-enter it in Settings → Secrets", actualName)
		}
		// A placeholder is not a credential, and letting one through produces a
		// confusing auth failure several minutes into a deploy.
		if value == "REPLACE_ME" {
			missing = append(missing, actualName+" (still a REPLACE_ME placeholder)")
			continue
		}

		// Set under the name asked for as well as the stored one when they differ
		// only in case, so a command written against either spelling works.
		env[actualName] = value
		if requested != actualName {
			env[requested] = value
		}
		m.db.LogAudit(actor, "secret_injected", "secret", "secret", id, actualName+" — "+reason)
	}
	return env, missing, nil
}
