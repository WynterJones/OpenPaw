-- Remote agents: agent_roles rows that proxy to an external assistant
-- (e.g. OpenClaw) instead of running OpenPaw's own LLM loop.
ALTER TABLE agent_roles ADD COLUMN remote_provider TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_roles ADD COLUMN remote_agent_id TEXT NOT NULL DEFAULT '';
