-- Tool process management columns
ALTER TABLE tools ADD COLUMN port INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tools ADD COLUMN pid INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tools ADD COLUMN capabilities TEXT NOT NULL DEFAULT '';

-- Schedule type support (tool_action = existing behavior, prompt = AI agent prompt)
ALTER TABLE schedules ADD COLUMN type TEXT NOT NULL DEFAULT 'tool_action';
ALTER TABLE schedules ADD COLUMN agent_role_slug TEXT NOT NULL DEFAULT '';
ALTER TABLE schedules ADD COLUMN prompt_content TEXT NOT NULL DEFAULT '';

-- Execution history for schedules
CREATE TABLE IF NOT EXISTS schedule_executions (
    id TEXT PRIMARY KEY,
    schedule_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'running',
    output TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at DATETIME,
    FOREIGN KEY (schedule_id) REFERENCES schedules(id) ON DELETE CASCADE
);
