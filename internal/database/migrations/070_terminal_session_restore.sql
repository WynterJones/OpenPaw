-- Keep enough information to recreate open terminal tabs after OpenPaw exits.
-- The PTY process itself is stopped with the app; startup opens a fresh shell
-- in the same directory while retaining the session id and saved panel layout.
ALTER TABLE terminal_sessions ADD COLUMN cwd TEXT NOT NULL DEFAULT '';
