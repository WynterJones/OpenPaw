-- Studio: media generation workspace (images, video, audio).
--
-- Builds on the existing `media` table rather than introducing a parallel
-- store, so anything Studio makes shows up in the media library and in chat
-- exactly like agent-generated images already do.

-- Folders organise generated media. Scoped per workspace so switching
-- workspaces re-scopes Studio the same way it re-scopes everything else.
CREATE TABLE IF NOT EXISTS media_folders (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_media_folders_workspace ON media_folders(workspace_id);

-- Existing media rows predate folders and workspaces; empty string means
-- "unfiled" / "not workspace-scoped" and is handled as such by the queries.
ALTER TABLE media ADD COLUMN folder_id TEXT DEFAULT '';
ALTER TABLE media ADD COLUMN workspace_id TEXT DEFAULT '';
-- Which provider actually produced the asset (openrouter / replicate / fal).
-- `source` already records this for agent images, but it doubles as an
-- attribution field there ("claude-code"), so provider gets its own column.
ALTER TABLE media ADD COLUMN provider TEXT DEFAULT '';
ALTER TABLE media ADD COLUMN duration_ms INTEGER DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_media_folder ON media(folder_id);
CREATE INDEX IF NOT EXISTS idx_media_workspace ON media(workspace_id);

-- Saved editor configurations, surfaced by Studio's "Saved" tab. A row is the
-- whole left-column state, so clicking one restores the exact setup that
-- produced a result the user liked.
CREATE TABLE IF NOT EXISTS studio_presets (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL,
    provider TEXT NOT NULL DEFAULT '',
    media_type TEXT NOT NULL DEFAULT 'image',
    model TEXT NOT NULL DEFAULT '',
    prompt TEXT NOT NULL DEFAULT '',
    count INTEGER NOT NULL DEFAULT 1,
    size TEXT NOT NULL DEFAULT '',
    folder_id TEXT NOT NULL DEFAULT '',
    params TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_studio_presets_workspace ON studio_presets(workspace_id, updated_at DESC);
