-- Backup execution history
CREATE TABLE IF NOT EXISTS backup_executions (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL DEFAULT 'running',
    files_count INTEGER NOT NULL DEFAULT 0,
    commit_sha TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    started_at DATETIME NOT NULL,
    finished_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_backup_executions_started ON backup_executions(started_at);
