-- Restore the Workbench + terminal subsystem that migration 048 dropped.
--
-- This recreates the schema as it stood after migrations 034-037 (terminal
-- sessions, tab colour, workbenches, workbench colour), collapsed into one
-- CREATE per table since the intermediate ALTERs have nothing to alter on a
-- database that never had these tables, or that had them dropped by 048.
--
-- Workspace scoping is included from the start (workspace_id, matching the
-- convention in 052) so a workbench belongs to one workspace, with the shared
-- Default workspace as the fallback for pre-existing rows.

CREATE TABLE IF NOT EXISTS workbenches (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL DEFAULT 'Default',
    sort_order INTEGER NOT NULL DEFAULT 0,
    color TEXT NOT NULL DEFAULT '',
    workspace_id TEXT DEFAULT '00000000-0000-0000-0000-000000000001',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS terminal_sessions (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL DEFAULT 'Terminal',
    shell TEXT NOT NULL,
    cols INTEGER NOT NULL DEFAULT 120,
    rows INTEGER NOT NULL DEFAULT 30,
    color TEXT NOT NULL DEFAULT '',
    workbench_id TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_terminal_sessions_workbench ON terminal_sessions(workbench_id);
CREATE INDEX IF NOT EXISTS idx_workbenches_workspace ON workbenches(workspace_id);
