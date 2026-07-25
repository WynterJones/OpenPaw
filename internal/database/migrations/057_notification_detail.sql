-- Notifications become self-contained reports, so the Inbox can show them like
-- email rather than as a pointer to somewhere else.
--
-- Until now `body` held a 100-character truncation of the *prompt* that was
-- scheduled, so a notification told you a task ran but never what came of it,
-- and the only way to read the result was to open the chat thread the run had
-- created. A scheduled run no longer creates a chat thread at all; the report
-- lands here, and a thread is created on demand if the user opens it as a chat.
--
-- Deliberately denormalized: a report keeps its own copy of the prompt and
-- response so it still reads (and still opens as a chat) after the schedule
-- that produced it has been edited or deleted.
ALTER TABLE notifications ADD COLUMN detail TEXT NOT NULL DEFAULT '';
ALTER TABLE notifications ADD COLUMN prompt TEXT NOT NULL DEFAULT '';
ALTER TABLE notifications ADD COLUMN workspace_id TEXT NOT NULL DEFAULT '';
ALTER TABLE notifications ADD COLUMN source_id TEXT NOT NULL DEFAULT '';

-- The Inbox lists newest-first, optionally filtered to one source.
CREATE INDEX IF NOT EXISTS idx_notifications_created ON notifications(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notifications_source ON notifications(source_type, created_at DESC);
