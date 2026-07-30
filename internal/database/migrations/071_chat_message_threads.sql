-- Focused, Slack-style message threads. Child chats are intentionally stored
-- in chat_threads so they reuse the existing routing, streaming, members,
-- provider sessions, cost tracking, and context compaction machinery.
ALTER TABLE chat_threads ADD COLUMN parent_thread_id TEXT NOT NULL DEFAULT '';
ALTER TABLE chat_threads ADD COLUMN root_message_id TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_chat_threads_parent
    ON chat_threads(parent_thread_id);

-- A message has at most one focused thread. The partial index leaves every
-- top-level chat (whose root is blank) outside the uniqueness constraint.
CREATE UNIQUE INDEX IF NOT EXISTS idx_chat_threads_root_message
    ON chat_threads(root_message_id)
    WHERE root_message_id != '';
