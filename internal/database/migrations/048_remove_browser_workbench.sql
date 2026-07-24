-- Remove the Browser use (go-rod automation) and Workbench/terminal subsystems.
-- Both features were deleted from the application; this migration drops their tables
-- and the schedule columns that referenced browser sessions.

-- Browser tables (browser_tasks / browser_action_log FK-cascade off browser_sessions,
-- but drop children first to be explicit). Indexes are dropped automatically with the tables.
DROP TABLE IF EXISTS browser_action_log;
DROP TABLE IF EXISTS browser_tasks;
DROP TABLE IF EXISTS browser_sessions;

-- Browser columns on schedules (added in 018, carried through the 027 rebuild).
-- Neither column is indexed or referenced by a view/trigger, so DROP COLUMN is safe
-- (SQLite >= 3.35, bundled with mattn/go-sqlite3 v1.14.34).
ALTER TABLE schedules DROP COLUMN browser_instructions;
ALTER TABLE schedules DROP COLUMN browser_session_id;

-- Workbench + terminal subsystem (Workbench was the only UI consumer of terminals).
-- Dropping terminal_sessions also removes its workbench_id column.
DROP TABLE IF EXISTS workbenches;
DROP TABLE IF EXISTS terminal_sessions;
