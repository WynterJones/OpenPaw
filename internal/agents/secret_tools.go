package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openpaw/openpaw/internal/database"
	llm "github.com/openpaw/openpaw/internal/llm"
)

// Secret tools for agents.
//
// Agents constantly need to answer "is FOO_API_KEY set yet?" while wiring up a
// service, and without this they had to guess or ask the user to go read the
// Secrets page and report back. list_secrets and check_secrets close that loop
// using names alone.
//
// get_secret goes further and hands over the decrypted value. Services normally
// receive their secrets as environment variables injected by the process
// manager, so the model rarely needs one — but "rarely" is not "never": calling
// an API from the shell, or writing a config file the user asked for, both need
// the actual string, and without this tool the agent's only move was to ask the
// user to paste it into chat, which is strictly worse. The value still lands in
// the context window and therefore in the provider's logs, so the tool is
// deliberately separate from the name tools, requires a stated reason, and
// every call is written to the audit log.

// SecretDecryptor decrypts stored secret values. Satisfied by secrets.Manager.
type SecretDecryptor interface {
	Decrypt(encrypted string) (string, error)
}

// BuildSecretToolDefs returns the secret tools. get_secret is only offered when
// a decryptor is wired in — a run that cannot decrypt must not advertise a tool
// it would only ever fail at.
func BuildSecretToolDefs(dec SecretDecryptor) []llm.ToolDef {
	listParams, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"filter": map[string]interface{}{
				"type":        "string",
				"description": "Optional case-insensitive substring to match against secret names, e.g. \"STRIPE\".",
			},
		},
	})

	checkParams, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"names": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Secret names to check, e.g. [\"STRIPE_SECRET_KEY\", \"RAILWAY_TOKEN\"].",
			},
		},
		"required": []string{"names"},
	})

	getParams, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Name of the secret to reveal, e.g. \"STRIPE_SECRET_KEY\".",
			},
			"reason": map[string]interface{}{
				"type":        "string",
				"description": "Short reason you need the value, e.g. \"calling the Stripe API to look up the webhook signing secret\". Recorded in the audit log.",
			},
		},
		"required": []string{"name", "reason"},
	})

	defs := []llm.ToolDef{
		{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        "list_secrets",
				Description: "List the NAMES of all secrets stored in OpenPaw, with their descriptions and which service owns them. Values are not returned — use get_secret for those. Use this to see what credentials are already configured.",
				Parameters:  listParams,
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        "check_secrets",
				Description: "Check which of the given secret names are already set and which are missing. Values are not returned — use get_secret for those. Use this when telling the user what they still need to add.",
				Parameters:  checkParams,
			},
		},
	}

	if dec != nil {
		defs = append(defs, llm.ToolDef{
			Type: "function",
			Function: llm.FunctionDef{
				Name: "get_secret",
				Description: "Reveal the decrypted VALUE of a stored secret. Use it when you actually need the credential to do the work — calling an API from the shell, or writing a config file the user asked for. " +
					"Prefer list_secrets/check_secrets when you only need to know whether something is set. Never echo the value back into chat, into a file the user did not ask for, or into a commit. Every call is audit-logged.",
				Parameters: getParams,
			},
		})
	}
	return defs
}

// MakeSecretToolHandlers returns handlers for the secret tools. A nil decryptor
// omits get_secret, matching BuildSecretToolDefs.
func MakeSecretToolHandlers(db *database.DB, dec SecretDecryptor, agentSlug string) map[string]llm.ToolHandler {
	handlers := map[string]llm.ToolHandler{
		"list_secrets":  handleListSecrets(db),
		"check_secrets": handleCheckSecrets(db),
	}
	if dec != nil {
		handlers["get_secret"] = handleGetSecret(db, dec, agentSlug)
	}
	return handlers
}

type secretInfo struct {
	name, description, toolName string
}

// loadSecretNames reads every secret's name and metadata. The encrypted_value
// column is not selected — not decrypted-and-discarded, simply never read, so
// there is no path by which a value could reach the model.
func loadSecretNames(db *database.DB) ([]secretInfo, error) {
	rows, err := db.Query(
		`SELECT s.name, s.description, COALESCE(t.name, '')
		 FROM secrets s
		 LEFT JOIN tools t ON s.tool_id != '' AND s.tool_id = t.id
		 ORDER BY s.name ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []secretInfo
	for rows.Next() {
		var s secretInfo
		if rows.Scan(&s.name, &s.description, &s.toolName) != nil {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

func handleListSecrets(db *database.DB) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var params struct {
			Filter string `json:"filter"`
		}
		json.Unmarshal(input, &params)

		secrets, err := loadSecretNames(db)
		if err != nil {
			return llm.ToolResult{Output: "Failed to list secrets: " + err.Error(), IsError: true}
		}

		filter := strings.ToLower(strings.TrimSpace(params.Filter))
		var lines []string
		for _, s := range secrets {
			if filter != "" && !strings.Contains(strings.ToLower(s.name), filter) {
				continue
			}
			line := "- " + s.name
			if s.description != "" {
				line += " — " + s.description
			}
			if s.toolName != "" {
				line += fmt.Sprintf(" (owned by service %q)", s.toolName)
			}
			lines = append(lines, line)
		}

		if len(lines) == 0 {
			if filter != "" {
				return llm.ToolResult{Output: fmt.Sprintf("No secrets match %q. %d secrets are stored in total.", params.Filter, len(secrets))}
			}
			return llm.ToolResult{Output: "No secrets are stored yet."}
		}

		return llm.ToolResult{Output: fmt.Sprintf(
			"%d secret(s). Names only — values are never exposed; services receive them as environment variables at runtime.\n\n%s",
			len(lines), strings.Join(lines, "\n"),
		)}
	}
}

func handleCheckSecrets(db *database.DB) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var params struct {
			Names []string `json:"names"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
		}
		if len(params.Names) == 0 {
			return llm.ToolResult{Output: "names is required", IsError: true}
		}

		secrets, err := loadSecretNames(db)
		if err != nil {
			return llm.ToolResult{Output: "Failed to check secrets: " + err.Error(), IsError: true}
		}

		// Compared case-insensitively: secret names are conventionally
		// SCREAMING_SNAKE_CASE and a model asking for "stripe_secret_key"
		// means the same secret, so a case-sensitive miss would report a
		// configured credential as missing.
		have := make(map[string]string, len(secrets))
		for _, s := range secrets {
			have[strings.ToLower(s.name)] = s.name
		}

		var set, missing []string
		for _, n := range params.Names {
			n = strings.TrimSpace(n)
			if n == "" {
				continue
			}
			if actual, ok := have[strings.ToLower(n)]; ok {
				set = append(set, actual)
			} else {
				missing = append(missing, n)
			}
		}

		var out strings.Builder
		if len(set) > 0 {
			fmt.Fprintf(&out, "SET (%d): %s\n", len(set), strings.Join(set, ", "))
		}
		if len(missing) > 0 {
			fmt.Fprintf(&out, "MISSING (%d): %s\n", len(missing), strings.Join(missing, ", "))
		} else {
			out.WriteString("MISSING (0): none — all requested secrets are configured.\n")
		}
		return llm.ToolResult{Output: strings.TrimSpace(out.String())}
	}
}

// handleGetSecret decrypts one secret by name. Names are matched
// case-insensitively for the same reason check_secrets does it.
func handleGetSecret(db *database.DB, dec SecretDecryptor, agentSlug string) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var params struct {
			Name   string `json:"name"`
			Reason string `json:"reason"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return llm.ToolResult{Output: "Invalid input: " + err.Error(), IsError: true}
		}
		params.Name = strings.TrimSpace(params.Name)
		if params.Name == "" {
			return llm.ToolResult{Output: "name is required", IsError: true}
		}

		var id, actualName, encrypted string
		err := db.QueryRow(
			"SELECT id, name, encrypted_value FROM secrets WHERE LOWER(name) = LOWER(?)",
			params.Name,
		).Scan(&id, &actualName, &encrypted)
		if err != nil {
			return llm.ToolResult{
				Output:  fmt.Sprintf("No secret named %q is stored. Call list_secrets to see what is configured.", params.Name),
				IsError: true,
			}
		}

		value, err := dec.Decrypt(encrypted)
		if err != nil {
			return llm.ToolResult{
				Output:  fmt.Sprintf("Secret %q exists but could not be decrypted (the encryption key may have changed). Ask the user to re-enter it in Settings → Secrets.", actualName),
				IsError: true,
			}
		}

		actor := "system"
		if agentSlug != "" {
			actor = "agent:" + agentSlug
		}
		db.LogAudit(actor, "secret_revealed", "secret", "secret", id, actualName+" — "+params.Reason)

		// A placeholder decrypts fine but is not a usable credential; handing it
		// over unlabelled sends the agent off to authenticate with "REPLACE_ME".
		if value == "REPLACE_ME" {
			return llm.ToolResult{Output: fmt.Sprintf(
				"%s is still a placeholder (REPLACE_ME) — no real value has been set. Ask the user to add it in Settings → Secrets.", actualName)}
		}

		return llm.ToolResult{Output: fmt.Sprintf(
			"%s=%s\n\n(Handle this value carefully: use it for the task at hand and do not repeat it back in chat or write it into files or commits.)",
			actualName, value,
		)}
	}
}

// buildSecretsPromptSection tells the agent the tools exist and how to treat
// values — otherwise a model unsure whether it can read one tends to ask the
// user to paste the secret into chat instead.
func buildSecretsPromptSection(db *database.DB, dec SecretDecryptor) string {
	secrets, err := loadSecretNames(db)
	if err != nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("## SECRETS\n")
	b.WriteString("Use `list_secrets` to see which credentials are configured and `check_secrets` to test specific names.\n")
	if dec != nil {
		b.WriteString("When a command needs a credential — a deploy, a CLI login, an API call from the shell — use `run_with_secrets`: name the secrets and OpenPaw sets them in that command's environment, so the value never passes through you at all. Prefer it over reading a value you were only going to paste into a command anyway.\n")
		b.WriteString("When you genuinely need a credential's value — writing a config file the user asked for, or calling an API from inside a tool — use `get_secret`; it returns the decrypted value and records the call in the audit log. ")
		b.WriteString("Reach for it only when the work needs it, and do not repeat the value back in chat, write it into files the user did not ask for, or commit it. ")
		b.WriteString("Services already receive their secrets as environment variables at runtime, so you usually do not need to read one.\n")
	} else {
		b.WriteString("You can read secret NAMES but not their VALUES — services receive them as environment variables at runtime.\n")
	}
	b.WriteString("Never ask the user to paste a secret value into chat; read it with `get_secret` or direct them to Settings → Secrets.\n")

	if len(secrets) == 0 {
		b.WriteString("\nNo secrets are configured yet.\n")
		return b.String()
	}

	// A short inline list saves a tool call in the common case. Capped because
	// a large vault would crowd out the rest of the system prompt.
	const inlineLimit = 40
	names := make([]string, 0, len(secrets))
	for i, s := range secrets {
		if i >= inlineLimit {
			break
		}
		names = append(names, s.name)
	}
	fmt.Fprintf(&b, "\nCurrently set (%d): %s", len(secrets), strings.Join(names, ", "))
	if len(secrets) > inlineLimit {
		fmt.Fprintf(&b, ", … call list_secrets for the remaining %d", len(secrets)-inlineLimit)
	}
	b.WriteString("\n")
	return b.String()
}
