-- Add ownership tracking to tools and dashboards
ALTER TABLE tools ADD COLUMN owner_agent_slug TEXT NOT NULL DEFAULT '';
ALTER TABLE dashboards ADD COLUMN owner_agent_slug TEXT NOT NULL DEFAULT '';

-- Agent tool access grants
CREATE TABLE IF NOT EXISTS agent_tool_access (
    id TEXT PRIMARY KEY,
    agent_role_slug TEXT NOT NULL,
    tool_id TEXT NOT NULL,
    granted_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(agent_role_slug, tool_id)
);
