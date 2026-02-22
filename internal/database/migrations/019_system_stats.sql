CREATE TABLE IF NOT EXISTS system_stats (
    key TEXT PRIMARY KEY,
    value REAL NOT NULL DEFAULT 0
);

INSERT OR IGNORE INTO system_stats (key, value) VALUES ('archived_cost_usd', 0);
INSERT OR IGNORE INTO system_stats (key, value) VALUES ('archived_input_tokens', 0);
INSERT OR IGNORE INTO system_stats (key, value) VALUES ('archived_output_tokens', 0);
INSERT OR IGNORE INTO system_stats (key, value) VALUES ('archived_activity_count', 0);
