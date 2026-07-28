-- Dreaming: agents review their recent chats on a schedule and turn what
-- happened in them into consolidated long-term memory.
--
-- Memory only ever grew from whatever an agent happened to call `memory_save`
-- on mid-turn, which is rare — an agent busy answering does not stop to file
-- what it just learned. Dreaming does that work offline instead: it reads the
-- chats the agent took part in, extracts durable facts, then reviews those
-- facts against the memories already stored and merges, rewrites or drops them.
--
-- dream_scans is the "already read this" ledger. Without it every run would
-- re-read the entire chat history and pay for it again. last_message_at records
-- how far into the thread the scan got, so a conversation that continues after
-- being scanned is picked up again on the next run while an untouched one is
-- skipped. It is also what puts the brain marker on a chat in the sidebar.
CREATE TABLE IF NOT EXISTS dream_scans (
    id              TEXT PRIMARY KEY,
    agent_slug      TEXT NOT NULL,
    thread_id       TEXT NOT NULL,
    facts_found     INTEGER NOT NULL DEFAULT 0,
    last_message_at TIMESTAMP,
    scanned_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (agent_slug, thread_id)
);

CREATE INDEX IF NOT EXISTS idx_dream_scans_thread ON dream_scans(thread_id);
CREATE INDEX IF NOT EXISTS idx_dream_scans_agent ON dream_scans(agent_slug);

-- One row per agent per dream. Kept so the run is inspectable after the fact:
-- a consolidation pass that quietly deleted the wrong thing is otherwise
-- invisible, since the memories it dropped are gone.
CREATE TABLE IF NOT EXISTS dream_runs (
    id               TEXT PRIMARY KEY,
    agent_slug       TEXT NOT NULL,
    status           TEXT NOT NULL DEFAULT 'running',
    threads_scanned  INTEGER NOT NULL DEFAULT 0,
    facts_found      INTEGER NOT NULL DEFAULT 0,
    memories_added   INTEGER NOT NULL DEFAULT 0,
    memories_updated INTEGER NOT NULL DEFAULT 0,
    memories_pruned  INTEGER NOT NULL DEFAULT 0,
    summary          TEXT NOT NULL DEFAULT '',
    error            TEXT NOT NULL DEFAULT '',
    started_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at      TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_dream_runs_started ON dream_runs(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_dream_runs_agent ON dream_runs(agent_slug);
