-- AI-generated UI backgrounds.
--
-- These sit alongside the shipped presets in Settings → Design → Background
-- Image. They are deliberately NOT stored in the `media` table: media is
-- Studio's workspace-scoped gallery, whereas a background is a global piece of
-- app chrome with its own recipe (mascot + style reference + agent avatar +
-- prompt) that we want to keep so a user can see how one was made.
--
-- Files live under <data>/generated-backgrounds/ rather than <data>/backgrounds/
-- because the existing "delete uploaded background" endpoint wipes that whole
-- directory.
CREATE TABLE IF NOT EXISTS generated_backgrounds (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL DEFAULT '',
    prompt TEXT NOT NULL DEFAULT '',
    -- The recipe, kept for display and for "make another like this".
    agent_slug TEXT NOT NULL DEFAULT '',
    style_ref TEXT NOT NULL DEFAULT '',
    provider TEXT NOT NULL DEFAULT 'openrouter',
    model TEXT NOT NULL DEFAULT '',
    filename TEXT NOT NULL,
    mime_type TEXT NOT NULL DEFAULT 'image/png',
    width INTEGER NOT NULL DEFAULT 0,
    height INTEGER NOT NULL DEFAULT 0,
    size_bytes INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_generated_backgrounds_created
    ON generated_backgrounds(created_at DESC);
