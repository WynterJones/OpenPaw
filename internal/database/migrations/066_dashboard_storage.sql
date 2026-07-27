-- Per-dashboard key/value store.
--
-- Custom dashboards render in an iframe sandboxed with `allow-scripts` but not
-- `allow-same-origin`, so the document has an opaque origin: localStorage,
-- sessionStorage, IndexedDB and cookies all throw or silently no-op there. A
-- dashboard that collects anything from the user (a budget, a list of products,
-- a checklist) therefore appeared to save and lost everything on reload.
--
-- `value` holds arbitrary JSON supplied by the dashboard — a string, number,
-- object or array — so the dashboard can round-trip whatever shape it uses.
CREATE TABLE IF NOT EXISTS dashboard_storage (
    dashboard_id TEXT NOT NULL,
    key          TEXT NOT NULL,
    value        TEXT NOT NULL,
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (dashboard_id, key),
    FOREIGN KEY (dashboard_id) REFERENCES dashboards(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_dashboard_storage_dashboard ON dashboard_storage(dashboard_id);
