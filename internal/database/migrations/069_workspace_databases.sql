-- Airtable-style workspace databases.
--
-- Schema metadata is normalized so columns can be added, renamed, reordered,
-- and removed without rebuilding a SQLite table. Row values stay flexible JSON
-- keyed by stable column UUIDs, which keeps renames cheap and preserves types.

CREATE TABLE user_databases (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    name         TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id)
);

CREATE UNIQUE INDEX idx_user_databases_workspace_name
    ON user_databases(workspace_id, name COLLATE NOCASE);
CREATE INDEX idx_user_databases_workspace
    ON user_databases(workspace_id, updated_at DESC);

CREATE TABLE user_database_tables (
    id          TEXT PRIMARY KEY,
    database_id TEXT NOT NULL,
    name        TEXT NOT NULL,
    sort_order  INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (database_id) REFERENCES user_databases(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX idx_user_database_tables_name
    ON user_database_tables(database_id, name COLLATE NOCASE);
CREATE INDEX idx_user_database_tables_database
    ON user_database_tables(database_id, sort_order, created_at);

CREATE TABLE user_database_columns (
    id          TEXT PRIMARY KEY,
    table_id    TEXT NOT NULL,
    name        TEXT NOT NULL,
    type        TEXT NOT NULL DEFAULT 'text',
    options     TEXT NOT NULL DEFAULT '{}',
    sort_order  INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (table_id) REFERENCES user_database_tables(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX idx_user_database_columns_name
    ON user_database_columns(table_id, name COLLATE NOCASE);
CREATE INDEX idx_user_database_columns_table
    ON user_database_columns(table_id, sort_order, created_at);

CREATE TABLE user_database_rows (
    id          TEXT PRIMARY KEY,
    table_id    TEXT NOT NULL,
    data        TEXT NOT NULL DEFAULT '{}',
    sort_order  INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (table_id) REFERENCES user_database_tables(id) ON DELETE CASCADE
);

CREATE INDEX idx_user_database_rows_table
    ON user_database_rows(table_id, sort_order, created_at);
