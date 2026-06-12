package database

import "time"

// GetProviderSession returns the stored CLI provider session ID for a
// thread+agent pair, or "" if none exists.
func (db *DB) GetProviderSession(threadID, agentSlug, provider string) string {
	var sessionID string
	db.QueryRow(
		"SELECT session_id FROM thread_provider_sessions WHERE thread_id = ? AND agent_slug = ? AND provider = ?",
		threadID, agentSlug, provider,
	).Scan(&sessionID)
	return sessionID
}

func (db *DB) PutProviderSession(threadID, agentSlug, provider, sessionID string) {
	db.Exec(
		`INSERT INTO thread_provider_sessions (thread_id, agent_slug, provider, session_id, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(thread_id, agent_slug, provider) DO UPDATE SET session_id = excluded.session_id, updated_at = excluded.updated_at`,
		threadID, agentSlug, provider, sessionID, time.Now().UTC(),
	)
}

func (db *DB) DeleteProviderSession(threadID, agentSlug, provider string) {
	db.Exec(
		"DELETE FROM thread_provider_sessions WHERE thread_id = ? AND agent_slug = ? AND provider = ?",
		threadID, agentSlug, provider,
	)
}

// DeleteThreadProviderSessions removes all provider sessions for a thread
// (called when the thread itself is deleted).
func (db *DB) DeleteThreadProviderSessions(threadID string) {
	db.Exec("DELETE FROM thread_provider_sessions WHERE thread_id = ?", threadID)
}
