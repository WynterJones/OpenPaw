package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/openpaw/openpaw/internal/agents"
	llm "github.com/openpaw/openpaw/internal/llm"
	"github.com/openpaw/openpaw/internal/middleware"
	"github.com/openpaw/openpaw/internal/models"
)

func (h *ChatHandler) addThreadMember(threadID, agentRoleSlug string) {
	if agentRoleSlug == "" || agentRoleSlug == "gateway" {
		return
	}
	_, err := h.db.Exec(
		"INSERT OR IGNORE INTO thread_members (thread_id, agent_role_slug) VALUES (?, ?)",
		threadID, agentRoleSlug,
	)
	if err != nil {
		return
	}
	// Look up agent info for the broadcast
	var name, avatarPath string
	h.db.QueryRow("SELECT name, avatar_path FROM agent_roles WHERE slug = ?", agentRoleSlug).Scan(&name, &avatarPath)
	h.db.LogAudit("system", "thread_member_joined", "chat", "chat_thread", threadID, agentRoleSlug)
	h.agentManager.Broadcast("thread_member_joined", map[string]interface{}{
		"thread_id":       threadID,
		"agent_role_slug": agentRoleSlug,
		"name":            name,
		"avatar_path":     avatarPath,
	})
}

func (h *ChatHandler) getLastResponder(threadID string) string {
	var slug string
	err := h.db.QueryRow(
		"SELECT agent_role_slug FROM chat_messages WHERE thread_id = ? AND role = 'assistant' AND agent_role_slug != '' ORDER BY created_at DESC LIMIT 1",
		threadID,
	).Scan(&slug)
	if err != nil {
		return ""
	}
	return slug
}

func (h *ChatHandler) getThreadMemberSlugs(threadID string) []string {
	rows, err := h.db.Query(
		"SELECT agent_role_slug FROM thread_members WHERE thread_id = ?",
		threadID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var slugs []string
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err == nil {
			slugs = append(slugs, slug)
		}
	}
	return slugs
}

func (h *ChatHandler) buildThreadMemberContext(threadID, currentAgentSlug string) string {
	rows, err := h.db.Query(
		`SELECT tm.agent_role_slug, ar.name, ar.description
		 FROM thread_members tm
		 JOIN agent_roles ar ON ar.slug = tm.agent_role_slug
		 WHERE tm.thread_id = ? AND tm.agent_role_slug != ?`,
		threadID, currentAgentSlug,
	)
	if err != nil {
		return ""
	}
	defer rows.Close()

	var members []string
	for rows.Next() {
		var slug, name, desc string
		if err := rows.Scan(&slug, &name, &desc); err == nil {
			members = append(members, fmt.Sprintf("- @%s (%s): %s", slug, name, desc))
		}
	}

	if len(members) == 0 {
		return ""
	}

	return "## OTHER AGENTS IN THIS THREAD\n" +
		strings.Join(members, "\n") +
		"\nYou can @mention another agent if they should help with this conversation."
}

func (h *ChatHandler) evaluateAgentMention(ctx context.Context, threadID, currentAgentSlug, mentionedSlug, agentResponse string, depth int) {
	// Build a compact evaluation prompt for the gateway
	evalMessage := fmt.Sprintf("[AGENT_MENTION_EVALUATION]\nAgent @%s mentioned @%s in their response.\n\nAgent response (excerpt):\n%s",
		currentAgentSlug, mentionedSlug, truncateStr(agentResponse, 500, true))

	hints := &agents.GatewayRoutingHints{
		LastResponder: currentAgentSlug,
		MentionSlug:   mentionedSlug,
		ThreadMembers: h.getThreadMemberSlugs(threadID),
	}

	history := h.fetchThreadHistory(threadID)
	resp, usage, err := h.agentManager.GatewayAnalyze(ctx, evalMessage, threadID, history, hints)
	if err != nil {
		return
	}

	var gatewayCostUSD float64
	if usage != nil {
		gatewayCostUSD = usage.CostUSD
	}

	// If gateway decided the mentioned agent should respond, hand off to them
	if resp.AssignedAgent == mentionedSlug {
		h.addThreadMember(threadID, mentionedSlug)
		h.broadcastRoutingIndicator(threadID, mentionedSlug)
		h.broadcastStatus(threadID, "thinking", "Thinking...")
		h.handleRoleChatWithDepth(ctx, threadID, agentResponse, mentionedSlug, depth+1, gatewayCostUSD)
	}
}

func (h *ChatHandler) generateThreadTitle(threadID, content string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	prompt := fmt.Sprintf("Generate a 2-4 word title for this chat message. Reply with ONLY the title, nothing else.\n\nMessage: %s", content)
	title, _, err := h.agentManager.Client().RunOneShot(ctx, llm.ResolveModel(h.agentManager.GatewayModel, llm.ModelHaiku), "", prompt)
	if err != nil || title == "" {
		return
	}

	title = strings.Trim(strings.TrimSpace(title), `"'`)
	h.setThreadTitle(threadID, title)
}

func (h *ChatHandler) ListThreadMembers(w http.ResponseWriter, r *http.Request) {
	threadID := chi.URLParam(r, "id")

	rows, err := h.db.Query(
		`SELECT tm.agent_role_slug, ar.name, ar.description, ar.avatar_path, tm.joined_at
		 FROM thread_members tm
		 JOIN agent_roles ar ON ar.slug = tm.agent_role_slug
		 WHERE tm.thread_id = ?
		 ORDER BY tm.joined_at ASC`,
		threadID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list thread members")
		return
	}
	defer rows.Close()

	members := []models.ThreadMember{}
	for rows.Next() {
		var m models.ThreadMember
		m.ThreadID = threadID
		if err := rows.Scan(&m.AgentRoleSlug, &m.Name, &m.Description, &m.AvatarPath, &m.JoinedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan member")
			return
		}
		members = append(members, m)
	}
	writeJSON(w, http.StatusOK, members)
}

func (h *ChatHandler) RemoveThreadMember(w http.ResponseWriter, r *http.Request) {
	threadID := chi.URLParam(r, "id")
	slug := chi.URLParam(r, "slug")

	result, err := h.db.Exec(
		"DELETE FROM thread_members WHERE thread_id = ? AND agent_role_slug = ?",
		threadID, slug,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove member")
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		writeError(w, http.StatusNotFound, "member not found")
		return
	}

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "thread_member_removed", "chat", "chat_thread", threadID, slug)
	h.agentManager.Broadcast("thread_member_removed", map[string]interface{}{
		"thread_id":       threadID,
		"agent_role_slug": slug,
	})

	w.WriteHeader(http.StatusNoContent)
}
