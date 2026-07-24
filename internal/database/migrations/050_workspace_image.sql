-- Optional per-workspace image, shown in the workspace switcher in place of the
-- generated letter badge. Stored as a URL served from the uploads endpoint
-- (data/avatars/<uuid>.ext), same as agent avatars. Empty string = no image.
ALTER TABLE workspaces ADD COLUMN image_url TEXT NOT NULL DEFAULT '';
