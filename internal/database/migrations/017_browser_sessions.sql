CREATE TABLE IF NOT EXISTS browser_sessions (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'stopped',
    headless INTEGER NOT NULL DEFAULT 1,
    user_data_dir TEXT NOT NULL DEFAULT '',
    current_url TEXT NOT NULL DEFAULT '',
    current_title TEXT NOT NULL DEFAULT '',
    owner_agent_slug TEXT NOT NULL DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS browser_tasks (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    thread_id TEXT NOT NULL DEFAULT '',
    agent_role_slug TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    instructions TEXT NOT NULL DEFAULT '',
    result TEXT NOT NULL DEFAULT '',
    extracted_data TEXT NOT NULL DEFAULT '{}',
    dashboard_id TEXT NOT NULL DEFAULT '',
    widget_id TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    started_at DATETIME,
    completed_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (session_id) REFERENCES browser_sessions(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS browser_action_log (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL DEFAULT '',
    session_id TEXT NOT NULL,
    action TEXT NOT NULL,
    selector TEXT NOT NULL DEFAULT '',
    value TEXT NOT NULL DEFAULT '',
    success INTEGER NOT NULL DEFAULT 1,
    error TEXT NOT NULL DEFAULT '',
    url_before TEXT NOT NULL DEFAULT '',
    url_after TEXT NOT NULL DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (session_id) REFERENCES browser_sessions(id) ON DELETE CASCADE
);
