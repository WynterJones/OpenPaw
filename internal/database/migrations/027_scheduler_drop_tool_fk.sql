-- Disable FK checks for table rebuild
PRAGMA foreign_keys = OFF;

-- Recreate schedules table without tool_id foreign key constraint
CREATE TABLE schedules_new (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    cron_expr TEXT NOT NULL,
    tool_id TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL DEFAULT '',
    payload TEXT NOT NULL DEFAULT '{}',
    enabled INTEGER NOT NULL DEFAULT 1,
    type TEXT NOT NULL DEFAULT 'prompt',
    agent_role_slug TEXT NOT NULL DEFAULT '',
    prompt_content TEXT NOT NULL DEFAULT '',
    thread_id TEXT NOT NULL DEFAULT '',
    dashboard_id TEXT NOT NULL DEFAULT '',
    widget_id TEXT NOT NULL DEFAULT '',
    browser_session_id TEXT NOT NULL DEFAULT '',
    browser_instructions TEXT NOT NULL DEFAULT '',
    last_run_at DATETIME,
    next_run_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO schedules_new
    SELECT id, name, description, cron_expr, tool_id, action, payload, enabled,
           type, agent_role_slug, prompt_content, thread_id, dashboard_id, widget_id,
           browser_session_id, browser_instructions,
           last_run_at, next_run_at, created_at, updated_at
    FROM schedules;

DROP TABLE schedules;
ALTER TABLE schedules_new RENAME TO schedules;

CREATE INDEX IF NOT EXISTS idx_schedules_enabled ON schedules(enabled);

-- Re-enable FK checks
PRAGMA foreign_keys = ON;
