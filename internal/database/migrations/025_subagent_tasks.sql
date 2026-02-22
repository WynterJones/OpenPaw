CREATE TABLE IF NOT EXISTS subagent_tasks (
    id TEXT PRIMARY KEY,
    thread_id TEXT NOT NULL,
    parent_agent_slug TEXT NOT NULL,
    agent_slug TEXT NOT NULL,
    task_description TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'running',
    result_text TEXT DEFAULT '',
    error TEXT DEFAULT '',
    cost_usd REAL DEFAULT 0,
    input_tokens INTEGER DEFAULT 0,
    output_tokens INTEGER DEFAULT 0,
    started_at DATETIME NOT NULL,
    completed_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_subagent_tasks_thread ON subagent_tasks(thread_id);
CREATE INDEX IF NOT EXISTS idx_subagent_tasks_parent ON subagent_tasks(parent_agent_slug);
