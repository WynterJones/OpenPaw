-- Tool library system: catalog metadata and integrity tracking

ALTER TABLE tools ADD COLUMN library_slug TEXT NOT NULL DEFAULT '';
ALTER TABLE tools ADD COLUMN library_version TEXT NOT NULL DEFAULT '';
ALTER TABLE tools ADD COLUMN source_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE tools ADD COLUMN binary_hash TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS tool_integrity (
    id TEXT PRIMARY KEY,
    tool_id TEXT NOT NULL,
    filename TEXT NOT NULL,
    file_hash TEXT NOT NULL,
    file_size INTEGER NOT NULL DEFAULT 0,
    recorded_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (tool_id) REFERENCES tools(id) ON DELETE CASCADE,
    UNIQUE(tool_id, filename)
);
