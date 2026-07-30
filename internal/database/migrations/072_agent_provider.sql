-- Agents can pin their own inference engine. Blank preserves today's behavior:
-- use whichever provider is active for the app.
ALTER TABLE agent_roles ADD COLUMN provider TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_agent_roles_provider
    ON agent_roles(provider);
