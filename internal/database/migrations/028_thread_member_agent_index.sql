-- Index on thread_members.agent_role_slug for efficient heartbeat thread lookups
CREATE INDEX IF NOT EXISTS idx_thread_members_agent ON thread_members(agent_role_slug);
