ALTER TABLE agent_roles ADD COLUMN workspace_id TEXT;
ALTER TABLE tools ADD COLUMN workspace_id TEXT;
CREATE INDEX IF NOT EXISTS idx_agent_roles_workspace ON agent_roles(workspace_id);
CREATE INDEX IF NOT EXISTS idx_tools_workspace ON tools(workspace_id);
