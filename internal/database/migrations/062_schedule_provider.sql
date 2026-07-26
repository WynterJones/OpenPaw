-- Let a schedule choose which engine runs it.
--
-- Schedules ran on whatever provider happened to be active, so a routine
-- written for a subscription CLI would silently start billing OpenRouter the
-- moment the composer switched engines — and an unattended run is exactly where
-- that goes unnoticed. '' means "whatever is active", which is the existing
-- behaviour, so nothing changes until a schedule opts in.
ALTER TABLE schedules ADD COLUMN provider TEXT NOT NULL DEFAULT '';
