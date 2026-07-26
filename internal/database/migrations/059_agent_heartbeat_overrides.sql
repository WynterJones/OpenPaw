-- Per-agent heartbeat tuning.
--
-- The heartbeat had exactly one interval for every agent, so a research agent
-- that only needs a nightly sweep and a task-runner that should pick up work
-- every half hour could not coexist. These columns let an agent override the
-- global settings; 0 means "inherit the global value", which is what every
-- existing agent gets, so behaviour is unchanged until something is set.
--
-- max_turns and timeout are here for the same reason as the interval: the
-- global caps (5 turns / 2 minutes) suit a quick check-in but cannot finish a
-- real task, and which agents do real work is a per-agent fact.
ALTER TABLE agent_roles ADD COLUMN heartbeat_interval_sec INTEGER NOT NULL DEFAULT 0;
ALTER TABLE agent_roles ADD COLUMN heartbeat_max_turns INTEGER NOT NULL DEFAULT 0;
ALTER TABLE agent_roles ADD COLUMN heartbeat_timeout_sec INTEGER NOT NULL DEFAULT 0;
