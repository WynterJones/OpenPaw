-- Workspaces: project containers that scope chats, dashboards, context
-- (files + folders), tasks (todo lists), and a real on-disk files directory.
-- Agents, tools, skills, secrets and settings remain shared (unscoped).
-- Schedules and heartbeat may optionally target a workspace (nullable = global).
--
-- Option A design: exactly ONE active workspace at a time, persisted server-side
-- in the settings key/value table under 'active_workspace_id'.

CREATE TABLE IF NOT EXISTS workspaces (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    is_default INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Seed the single default workspace with a fixed, well-known uuid so both the
-- backfill below and application code can reference it deterministically.
INSERT OR IGNORE INTO workspaces (id, name, sort_order, is_default)
VALUES ('00000000-0000-0000-0000-000000000001', 'Default', 0, 1);

-- Per-workspace scoping columns. Nullable with a default of the Default
-- workspace id so pre-existing rows and any un-scoped inserts land in Default.
ALTER TABLE chat_threads    ADD COLUMN workspace_id TEXT DEFAULT '00000000-0000-0000-0000-000000000001';
ALTER TABLE dashboards      ADD COLUMN workspace_id TEXT DEFAULT '00000000-0000-0000-0000-000000000001';
ALTER TABLE context_files   ADD COLUMN workspace_id TEXT DEFAULT '00000000-0000-0000-0000-000000000001';
ALTER TABLE context_folders ADD COLUMN workspace_id TEXT DEFAULT '00000000-0000-0000-0000-000000000001';
ALTER TABLE todo_lists      ADD COLUMN workspace_id TEXT DEFAULT '00000000-0000-0000-0000-000000000001';

-- Backfill any rows that predate the column (defensive; the DEFAULT already
-- covers existing rows on SQLite, but this makes the intent explicit).
UPDATE chat_threads    SET workspace_id = '00000000-0000-0000-0000-000000000001' WHERE workspace_id IS NULL;
UPDATE dashboards      SET workspace_id = '00000000-0000-0000-0000-000000000001' WHERE workspace_id IS NULL;
UPDATE context_files   SET workspace_id = '00000000-0000-0000-0000-000000000001' WHERE workspace_id IS NULL;
UPDATE context_folders SET workspace_id = '00000000-0000-0000-0000-000000000001' WHERE workspace_id IS NULL;
UPDATE todo_lists      SET workspace_id = '00000000-0000-0000-0000-000000000001' WHERE workspace_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_chat_threads_workspace    ON chat_threads(workspace_id);
CREATE INDEX IF NOT EXISTS idx_dashboards_workspace      ON dashboards(workspace_id);
CREATE INDEX IF NOT EXISTS idx_context_files_workspace   ON context_files(workspace_id);
CREATE INDEX IF NOT EXISTS idx_context_folders_workspace ON context_folders(workspace_id);
CREATE INDEX IF NOT EXISTS idx_todo_lists_workspace      ON todo_lists(workspace_id);

-- Optional workspace target for schedules (nullable = all/global).
ALTER TABLE schedules ADD COLUMN workspace_id TEXT;

-- Heartbeat has no dedicated config table; its config lives in the settings
-- key/value table (keys 'heartbeat_*'). Its optional workspace target is
-- therefore represented as the settings key 'heartbeat_workspace_id'
-- (unset/empty = global). Not seeded here so the default stays "global".

-- Active workspace pointer, persisted server-side in settings.
INSERT OR IGNORE INTO settings (id, key, value)
VALUES ('00000000-0000-0000-0000-0000000000a1', 'active_workspace_id', '00000000-0000-0000-0000-000000000001');
