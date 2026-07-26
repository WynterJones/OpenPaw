-- Marks an assistant message the user interrupted mid-stream.
--
-- Stopping used to throw the half-written reply away and save the literal text
-- "Stopped." in its place, while the cancelled agent goroutine saved a second
-- "I encountered an error" message behind it. The partial answer is usually
-- worth keeping — it is what the user was reading when they decided to stop —
-- so it is saved as-is and flagged here, and the UI badges it rather than
-- pretending it was a complete reply.
ALTER TABLE chat_messages ADD COLUMN stopped INTEGER NOT NULL DEFAULT 0;
