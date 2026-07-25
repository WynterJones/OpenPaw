-- Pinned (archived) chats.
--
-- Pinning ends a conversation: the thread becomes read-only and keeps an
-- AI-written long-form summary so it stays useful as a reference without having
-- to re-read the transcript. The transcript itself is NOT deleted — unlike
-- compaction, pinning is lossless; the summary sits alongside the messages.
ALTER TABLE chat_threads ADD COLUMN pinned INTEGER NOT NULL DEFAULT 0;
ALTER TABLE chat_threads ADD COLUMN pinned_at DATETIME;
ALTER TABLE chat_threads ADD COLUMN pin_summary TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_chat_threads_pinned ON chat_threads(pinned, updated_at DESC);
