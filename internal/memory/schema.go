package memory

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

const schemaSQL = `
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 3000;

CREATE TABLE IF NOT EXISTS memories (
    id TEXT PRIMARY KEY,
    content TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    category TEXT NOT NULL DEFAULT 'general',
    importance INTEGER NOT NULL DEFAULT 5,
    source TEXT NOT NULL DEFAULT 'agent',
    tags TEXT NOT NULL DEFAULT '',
    access_count INTEGER NOT NULL DEFAULT 0,
    last_accessed_at DATETIME,
    expires_at DATETIME,
    archived INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_memories_category ON memories(category);
CREATE INDEX IF NOT EXISTS idx_memories_importance ON memories(importance DESC);
CREATE INDEX IF NOT EXISTS idx_memories_created ON memories(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_memories_archived ON memories(archived);

CREATE TABLE IF NOT EXISTS collections (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS memory_collections (
    memory_id TEXT NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    collection_id TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    PRIMARY KEY (memory_id, collection_id)
);
`

const ftsSQL = `
CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
    content, summary, category, tags,
    content=memories, content_rowid=rowid,
    tokenize='porter unicode61'
);
`

const triggersSQL = `
CREATE TRIGGER IF NOT EXISTS memories_ai AFTER INSERT ON memories BEGIN
    INSERT INTO memories_fts(rowid, content, summary, category, tags)
    VALUES (new.rowid, new.content, new.summary, new.category, new.tags);
END;

CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN
    INSERT INTO memories_fts(memories_fts, rowid, content, summary, category, tags)
    VALUES ('delete', old.rowid, old.content, old.summary, old.category, old.tags);
END;

CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE ON memories BEGIN
    INSERT INTO memories_fts(memories_fts, rowid, content, summary, category, tags)
    VALUES ('delete', old.rowid, old.content, old.summary, old.category, old.tags);
    INSERT INTO memories_fts(rowid, content, summary, category, tags)
    VALUES (new.rowid, new.content, new.summary, new.category, new.tags);
END;
`

const schemaVersion = 1

func applySchema(db *sql.DB) error {
	if _, err := db.Exec(schemaSQL); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	if _, err := db.Exec(ftsSQL); err != nil {
		return fmt.Errorf("apply FTS: %w", err)
	}
	if _, err := db.Exec(triggersSQL); err != nil {
		return fmt.Errorf("apply triggers: %w", err)
	}

	// Track schema version
	db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`)
	var v int
	err := db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&v)
	if err != nil {
		db.Exec("INSERT INTO schema_version (version) VALUES (?)", schemaVersion)
	}

	return nil
}
