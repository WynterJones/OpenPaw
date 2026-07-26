package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openpaw/openpaw/internal/database"
	llm "github.com/openpaw/openpaw/internal/llm"
)

// Secret *names* for agents — never values.
//
// Agents constantly need to answer "is FOO_API_KEY set yet?" while wiring up a
// service, and without this they had to guess or ask the user to go read the
// Secrets page and report back. Listing names closes that loop.
//
// Values are deliberately unreachable here. A service receives its secrets as
// environment variables at runtime, injected by the process manager — the model
// never needs to see one, so it never gets one. That keeps credentials out of
// chat transcripts, out of the context window, and out of whatever the model
// provider logs.

// BuildSecretToolDefs returns the read-only secret tools.
func BuildSecretToolDefs() []llm.ToolDef {
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

	return []llm.ToolDef{
		{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        "list_secrets",
				Description: "List the NAMES of all secrets stored in OpenPaw, with their descriptions and which service owns them. Values are never returned. Use this to see what credentials are already configured.",
				Parameters:  listParams,
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        "check_secrets",
				Description: "Check which of the given secret names are already set and which are missing. Values are never returned. Use this when telling the user what they still need to add.",
				Parameters:  checkParams,
			},
		},
	}
}

// MakeSecretToolHandlers returns handlers for the read-only secret tools.
func MakeSecretToolHandlers(db *database.DB) map[string]llm.ToolHandler {
	return map[string]llm.ToolHandler{
		"list_secrets":  handleListSecrets(db),
		"check_secrets": handleCheckSecrets(db),
	}
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

// buildSecretsPromptSection tells the agent the tools exist and, crucially,
// that values are off-limits — otherwise a model that cannot find a value tool
// tends to ask the user to paste the secret into chat.
func buildSecretsPromptSection(db *database.DB) string {
	secrets, err := loadSecretNames(db)
	if err != nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("## SECRETS\n")
	b.WriteString("Use `list_secrets` to see which credentials are configured and `check_secrets` to test specific names.\n")
	b.WriteString("You can read secret NAMES but never their VALUES — services receive them as environment variables at runtime. ")
	b.WriteString("Never ask the user to paste a secret value into chat; direct them to Settings → Secrets instead.\n")

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
