-- PixelLab pixel-art companions: characters generated via the PixelLab API,
-- animated per chat state (idle/thinking/toolcall/responding) and pinned as
-- movable floating companions in chat. Sprite frames live on disk under
-- {dataDir}/pixellab/{characterId}/{clipId}/frame_N.png; this table stores
-- metadata + the animation manifest (JSON paths).
CREATE TABLE IF NOT EXISTS pixellab_characters (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL DEFAULT '',
    name        TEXT NOT NULL,
    pixellab_id TEXT NOT NULL DEFAULT '',          -- reusable PixelLab character id
    base_path   TEXT NOT NULL DEFAULT '',          -- relative path to base sprite frame
    animations  TEXT NOT NULL DEFAULT '[]',        -- JSON: [{id,name,fps,frames:[relpath]}]
    pinned      INTEGER NOT NULL DEFAULT 0,         -- shown as a chat companion
    agent_slug  TEXT NOT NULL DEFAULT '',          -- optional assigned agent role
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_pixellab_characters_pinned ON pixellab_characters(pinned);
CREATE INDEX IF NOT EXISTS idx_pixellab_characters_agent ON pixellab_characters(agent_slug);
