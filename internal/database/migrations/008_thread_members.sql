CREATE TABLE IF NOT EXISTS thread_members (
    thread_id TEXT NOT NULL,
    agent_role_slug TEXT NOT NULL,
    joined_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (thread_id, agent_role_slug),
    FOREIGN KEY (thread_id) REFERENCES chat_threads(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_thread_members_thread ON thread_members(thread_id);
