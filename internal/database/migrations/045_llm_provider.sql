-- CLI provider session continuity: maps a chat thread + agent to the native
-- session ID of a CLI provider (claude-code, codex) so multi-turn threads
-- resume instead of replaying full history.
CREATE TABLE IF NOT EXISTS thread_provider_sessions (
    thread_id  TEXT NOT NULL,
    agent_slug TEXT NOT NULL,
    provider   TEXT NOT NULL,
    session_id TEXT NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (thread_id, agent_slug, provider)
);
