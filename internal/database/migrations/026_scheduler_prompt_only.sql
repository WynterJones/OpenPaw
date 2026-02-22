-- Add thread_id to schedules for chat thread integration
ALTER TABLE schedules ADD COLUMN thread_id TEXT NOT NULL DEFAULT '';

-- Convert any existing non-prompt schedules to prompt type
UPDATE schedules SET type = 'prompt' WHERE type != 'prompt';
