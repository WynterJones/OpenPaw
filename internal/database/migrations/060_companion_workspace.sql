-- Scope companions to a workspace.
--
-- Companions are decoration tied to how you work, and workspaces are how work
-- is separated here — a companion made for a client project has no business
-- floating over an unrelated one. NULL means "all workspaces", matching agents,
-- skills and services, so every existing companion keeps showing everywhere.
ALTER TABLE pixellab_characters ADD COLUMN workspace_id TEXT;

CREATE INDEX IF NOT EXISTS idx_pixellab_characters_workspace
    ON pixellab_characters(workspace_id);
