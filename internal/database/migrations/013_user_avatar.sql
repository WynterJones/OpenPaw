-- Add avatar_path to users table
ALTER TABLE users ADD COLUMN avatar_path TEXT NOT NULL DEFAULT '';
