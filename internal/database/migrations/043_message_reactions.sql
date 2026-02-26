-- Emoji reactions on chat messages (user + agent)
CREATE TABLE IF NOT EXISTS chat_message_reactions (
    id TEXT PRIMARY KEY,
    message_id TEXT NOT NULL,
    emoji TEXT NOT NULL,
    source TEXT NOT NULL DEFAULT 'user',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(message_id, emoji, source)
);
CREATE INDEX IF NOT EXISTS idx_reactions_message ON chat_message_reactions(message_id);
