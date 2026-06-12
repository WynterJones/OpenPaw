package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/logger"
)

// OpenClaw integration: OpenClaw (openclaw.ai) is a self-hosted personal AI
// assistant gateway users talk to via Telegram/WhatsApp/etc. OpenPaw imports
// its agents as "remote" agent_roles and routes their chat turns through the
// `openclaw` CLI, so the same agents (with their own memory and sessions on
// the OpenClaw side) are reachable from OpenPaw chat.

const openClawProvider = "openclaw"

var openClawProbe struct {
	mu        sync.Mutex
	checkedAt time.Time
	available bool
	version   string
}

// OpenClawStatus reports whether the openclaw CLI is installed (cached 60s).
func OpenClawStatus() (available bool, version string) {
	openClawProbe.mu.Lock()
	defer openClawProbe.mu.Unlock()
	if time.Since(openClawProbe.checkedAt) < 60*time.Second {
		return openClawProbe.available, openClawProbe.version
	}
	openClawProbe.checkedAt = time.Now()
	openClawProbe.available = false
	openClawProbe.version = ""

	path, err := exec.LookPath("openclaw")
	if err != nil {
		return false, ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return false, ""
	}
	openClawProbe.available = true
	openClawProbe.version = strings.TrimSpace(string(out))
	return true, openClawProbe.version
}

// OpenClawAgent is one agent from `openclaw agents list --json`.
type OpenClawAgent struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	IdentityName  string `json:"identityName"`
	IdentityEmoji string `json:"identityEmoji"`
	Model         string `json:"model"`
	Workspace     string `json:"workspace"`
}

// DisplayName returns the best human-facing name for the agent.
func (a OpenClawAgent) DisplayName() string {
	if a.Name != "" {
		return a.Name
	}
	if a.IdentityName != "" {
		return a.IdentityName
	}
	return a.ID
}

// OpenClawListAgents fetches the agent roster from the local OpenClaw install.
func OpenClawListAgents(ctx context.Context) ([]OpenClawAgent, error) {
	if ok, _ := OpenClawStatus(); !ok {
		return nil, fmt.Errorf("openclaw CLI not found in PATH")
	}

	cmd := exec.CommandContext(ctx, "openclaw", "agents", "list", "--json")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("openclaw agents list failed: %w: %s", err, truncateStr(stderr.String(), 300, true))
	}

	var agents []OpenClawAgent
	if err := json.Unmarshal(extractCLIJSON(stdout.Bytes()), &agents); err != nil {
		return nil, fmt.Errorf("failed to parse openclaw agent list: %w", err)
	}
	return agents, nil
}

// OpenClawChat sends one message to an OpenClaw agent through the gateway and
// returns its reply. The session ID is stable per OpenPaw thread so multi-turn
// context lives on the OpenClaw side.
func (m *Manager) OpenClawChat(ctx context.Context, remoteAgentID, threadID, message string) (string, error) {
	if ok, _ := OpenClawStatus(); !ok {
		return "", fmt.Errorf("the OpenClaw CLI is not available — is OpenClaw installed and in PATH?")
	}

	args := []string{"agent", "--agent", remoteAgentID, "--json", "-m", message}
	if threadID != "" {
		args = append(args, "--session-id", "openpaw-"+threadID)
	}

	cmd := exec.CommandContext(ctx, "openclaw", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	logger.Info("openclaw agent turn: agent=%s thread=%s took=%s err=%v", remoteAgentID, threadID, time.Since(start).Round(time.Millisecond), err)

	reply := parseOpenClawReply(stdout.Bytes())
	if err != nil {
		detail := truncateStr(strings.TrimSpace(stderr.String()), 400, true)
		if strings.Contains(detail, "pairing required") || strings.Contains(detail, "scope") {
			return "", fmt.Errorf("OpenClaw rejected the request: device pairing/scope approval needed. Approve this device in OpenClaw (`openclaw devices`) and try again")
		}
		if reply != "" {
			// CLI exited non-zero but still produced a reply (e.g. delivery warnings)
			return reply, nil
		}
		return "", fmt.Errorf("openclaw agent turn failed: %s", detail)
	}
	if reply == "" {
		return "", fmt.Errorf("OpenClaw returned no reply")
	}
	return reply, nil
}

// parseOpenClawReply extracts the agent's reply text from `openclaw agent
// --json` output. The shape is parsed tolerantly: known payload fields first,
// then a generic deep scan for text-bearing fields.
func parseOpenClawReply(out []byte) string {
	raw := extractCLIJSON(out)
	if len(raw) == 0 {
		return ""
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		return ""
	}

	// Common shapes: {result:{payloads:[{text}]}}, {payloads:[{text}]},
	// {reply:...}, {text:...}, {message:...}
	if res, ok := doc["result"]; ok {
		var inner map[string]json.RawMessage
		if json.Unmarshal(res, &inner) == nil {
			if text := payloadsText(inner["payloads"]); text != "" {
				return text
			}
			for _, key := range []string{"reply", "text", "message"} {
				var s string
				if json.Unmarshal(inner[key], &s) == nil && strings.TrimSpace(s) != "" {
					return strings.TrimSpace(s)
				}
			}
		}
	}
	if text := payloadsText(doc["payloads"]); text != "" {
		return text
	}
	for _, key := range []string{"reply", "text", "message", "response"} {
		var s string
		if json.Unmarshal(doc[key], &s) == nil && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func payloadsText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var payloads []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &payloads) != nil {
		return ""
	}
	var parts []string
	for _, p := range payloads {
		if strings.TrimSpace(p.Text) != "" {
			parts = append(parts, strings.TrimSpace(p.Text))
		}
	}
	return strings.Join(parts, "\n\n")
}

// extractJSON returns the JSON document in CLI output, tolerating log lines
// before/after it (finds the first '{' or '[' and the matching last bracket).
func extractCLIJSON(out []byte) []byte {
	s := bytes.TrimSpace(out)
	if len(s) == 0 {
		return nil
	}
	if s[0] == '{' || s[0] == '[' {
		return s
	}
	objStart := bytes.IndexByte(s, '{')
	arrStart := bytes.IndexByte(s, '[')
	start := objStart
	end := bytes.LastIndexByte(s, '}')
	if arrStart >= 0 && (objStart < 0 || arrStart < objStart) {
		start = arrStart
		end = bytes.LastIndexByte(s, ']')
	}
	if start < 0 || end <= start {
		return nil
	}
	return s[start : end+1]
}

// SyncOpenClawAgents imports/updates OpenClaw agents as remote agent_roles.
// Agents that disappeared from OpenClaw are removed. Returns imported count.
func (m *Manager) SyncOpenClawAgents(ctx context.Context) (int, error) {
	remote, err := OpenClawListAgents(ctx)
	if err != nil {
		return 0, err
	}

	var maxSort int
	m.db.QueryRow("SELECT COALESCE(MAX(sort_order), 0) FROM agent_roles").Scan(&maxSort)

	now := time.Now().UTC()
	seen := make([]string, 0, len(remote))
	for _, agent := range remote {
		slug := m.openClawSlugFor(agent.ID)
		if slug == "" {
			logger.Warn("openclaw sync: no free slug for agent %q, skipping", agent.ID)
			continue
		}
		seen = append(seen, agent.ID)

		desc := fmt.Sprintf("Remote OpenClaw agent (model: %s). Replies come from your OpenClaw assistant — same brain you talk to on Telegram.", agent.Model)
		maxSort++
		_, err := m.db.Exec(
			`INSERT INTO agent_roles (id, slug, name, description, system_prompt, model, avatar_path, enabled, sort_order, is_preset, identity_initialized, remote_provider, remote_agent_id, created_at, updated_at)
			 VALUES (?, ?, ?, ?, '', ?, ?, 1, ?, 0, 0, ?, ?, ?, ?)
			 ON CONFLICT(slug) DO UPDATE SET name = excluded.name, description = excluded.description, model = excluded.model, updated_at = excluded.updated_at,
			   avatar_path = CASE WHEN agent_roles.avatar_path = '' THEN excluded.avatar_path ELSE agent_roles.avatar_path END`,
			uuid.New().String(), slug, agent.DisplayName(), desc, agent.Model, presetAvatarFor(agent.ID), maxSort, openClawProvider, agent.ID, now, now,
		)
		if err != nil {
			logger.Error("openclaw sync: upsert %s failed: %v", slug, err)
		}
	}

	// Remove imported agents that no longer exist in OpenClaw
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(seen)), ",")
	query := "DELETE FROM agent_roles WHERE remote_provider = ?"
	args := []interface{}{openClawProvider}
	if len(seen) > 0 {
		query += " AND remote_agent_id NOT IN (" + placeholders + ")"
		for _, id := range seen {
			args = append(args, id)
		}
	}
	m.db.Exec(query, args...)

	return len(seen), nil
}

// presetAvatarFor deterministically assigns one of the 45 bundled preset
// avatars so imported agents render properly (and keep the same avatar
// across re-syncs). Users can still customize it afterwards.
func presetAvatarFor(remoteID string) string {
	h := fnv.New32a()
	h.Write([]byte(remoteID))
	return fmt.Sprintf("/avatars/avatar-%d.webp", (h.Sum32()%45)+1)
}

// openClawSlugFor picks a chat slug for a remote agent: the OpenClaw id
// itself when free (or already ours), else prefixed with "oc-".
func (m *Manager) openClawSlugFor(remoteID string) string {
	for _, candidate := range []string{remoteID, "oc-" + remoteID} {
		var existingProvider string
		err := m.db.QueryRow("SELECT remote_provider FROM agent_roles WHERE slug = ?", candidate).Scan(&existingProvider)
		if err != nil || existingProvider == openClawProvider {
			return candidate
		}
	}
	return ""
}

// RemoveOpenClawAgents deletes all imported OpenClaw agent roles.
func (m *Manager) RemoveOpenClawAgents() (int, error) {
	res, err := m.db.Exec("DELETE FROM agent_roles WHERE remote_provider = ?", openClawProvider)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// CountOpenClawAgents returns how many OpenClaw agents are imported.
func (m *Manager) CountOpenClawAgents() int {
	var n int
	m.db.QueryRow("SELECT COUNT(*) FROM agent_roles WHERE remote_provider = ?", openClawProvider).Scan(&n)
	return n
}
