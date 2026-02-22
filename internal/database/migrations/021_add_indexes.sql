-- MED-09: notifications table
CREATE INDEX IF NOT EXISTS idx_notifications_active ON notifications(dismissed, read, created_at DESC);

-- MED-10: heartbeat_executions table
CREATE INDEX IF NOT EXISTS idx_heartbeat_exec_started ON heartbeat_executions(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_heartbeat_exec_agent ON heartbeat_executions(agent_role_slug, started_at DESC);

-- MED-11: browser_tasks and browser_action_log tables
CREATE INDEX IF NOT EXISTS idx_browser_tasks_session ON browser_tasks(session_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_browser_action_log_task ON browser_action_log(task_id, created_at);

-- MED-12: running counters for LogStats (avoid full table scans)
INSERT OR IGNORE INTO system_stats (key, value) VALUES ('live_cost_usd', 0);
INSERT OR IGNORE INTO system_stats (key, value) VALUES ('live_input_tokens', 0);
INSERT OR IGNORE INTO system_stats (key, value) VALUES ('live_output_tokens', 0);
