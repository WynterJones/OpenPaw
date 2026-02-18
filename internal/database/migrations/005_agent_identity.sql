-- Agent identity file system
ALTER TABLE agent_roles ADD COLUMN identity_initialized INTEGER NOT NULL DEFAULT 0;
