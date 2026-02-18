-- Dashboard data points for time-series storage
ALTER TABLE dashboards ADD COLUMN description TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS dashboard_data_points (
    id TEXT PRIMARY KEY,
    dashboard_id TEXT NOT NULL,
    widget_id TEXT NOT NULL,
    tool_id TEXT NOT NULL DEFAULT '',
    endpoint TEXT NOT NULL DEFAULT '',
    data TEXT NOT NULL DEFAULT '{}',
    collected_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (dashboard_id) REFERENCES dashboards(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_dashboard_data_points_lookup
    ON dashboard_data_points(dashboard_id, widget_id, collected_at);

-- Add dashboard tracking to schedules
ALTER TABLE schedules ADD COLUMN dashboard_id TEXT NOT NULL DEFAULT '';
ALTER TABLE schedules ADD COLUMN widget_id TEXT NOT NULL DEFAULT '';
