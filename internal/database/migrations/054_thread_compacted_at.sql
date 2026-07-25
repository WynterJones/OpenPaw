-- Track when a thread was last compacted.
--
-- Context usage is measured as MAX(input_tokens) over the thread's assistant
-- messages. Compaction now retains the most recent messages verbatim instead of
-- deleting everything, so those retained messages still carry the large
-- input_tokens from before compaction. Without a watermark the usage ratio would
-- stay above the threshold and the thread would re-compact on every single turn.
-- Only messages created after compacted_at count toward the live context window.
ALTER TABLE chat_threads ADD COLUMN compacted_at DATETIME;
