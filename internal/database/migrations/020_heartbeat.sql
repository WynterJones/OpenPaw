-- Notifications
CREATE TABLE IF NOT EXISTS notifications (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    priority TEXT NOT NULL DEFAULT 'normal',
    source_agent_slug TEXT NOT NULL DEFAULT '',
    source_type TEXT NOT NULL DEFAULT 'heartbeat',
    link TEXT NOT NULL DEFAULT '',
    read INTEGER NOT NULL DEFAULT 0,
    dismissed INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Heartbeat execution history
CREATE TABLE IF NOT EXISTS heartbeat_executions (
    id TEXT PRIMARY KEY,
    agent_role_slug TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'running',
    actions_taken TEXT NOT NULL DEFAULT '[]',
    output TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    cost_usd REAL NOT NULL DEFAULT 0,
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    started_at DATETIME NOT NULL,
    finished_at DATETIME
);

-- Per-agent heartbeat toggle
ALTER TABLE agent_roles ADD COLUMN heartbeat_enabled INTEGER NOT NULL DEFAULT 0;
