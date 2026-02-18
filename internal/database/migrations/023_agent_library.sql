ALTER TABLE agent_roles ADD COLUMN library_slug TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_roles ADD COLUMN library_version TEXT NOT NULL DEFAULT '';
