CREATE TABLE IF NOT EXISTS media (
    id TEXT PRIMARY KEY,
    thread_id TEXT DEFAULT '',
    message_id TEXT DEFAULT '',
    source TEXT NOT NULL DEFAULT 'fal',
    source_model TEXT DEFAULT '',
    media_type TEXT NOT NULL DEFAULT 'image',
    url TEXT NOT NULL DEFAULT '',
    filename TEXT NOT NULL DEFAULT '',
    mime_type TEXT NOT NULL DEFAULT 'image/jpeg',
    width INTEGER DEFAULT 0,
    height INTEGER DEFAULT 0,
    size_bytes INTEGER DEFAULT 0,
    prompt TEXT DEFAULT '',
    metadata TEXT DEFAULT '{}',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_media_thread ON media(thread_id);
CREATE INDEX IF NOT EXISTS idx_media_type ON media(media_type);
CREATE INDEX IF NOT EXISTS idx_media_created ON media(created_at DESC);
