package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
)

// Custom dashboards run in an iframe with an opaque origin, where localStorage
// and cookies are unavailable — anything a dashboard collects from the user has
// to live server-side. These endpoints back OpenPaw.storage in the dashboard
// SDK: a small per-dashboard key/value store holding arbitrary JSON.
const (
	// maxStorageValueBytes bounds a single value. Dashboards keep lists and
	// settings here, not datasets; anything larger belongs in a service.
	maxStorageValueBytes = 512 * 1024
	// maxStorageKeys bounds how many keys one dashboard may hold, so a runaway
	// loop in dashboard.js cannot grow the database without limit.
	maxStorageKeys = 500
	// maxStorageKeyLen bounds a key name.
	maxStorageKeyLen = 200
)

// storageKey reads the {key} path param. Keys are user-facing strings that may
// contain spaces or slashes, so the client percent-encodes them; chi hands back
// the still-escaped segment whenever it did. Decoding here keeps what is stored
// (and what ListStorage returns) identical to the key the dashboard passed.
func storageKey(r *http.Request) string {
	raw := chi.URLParam(r, "key")
	if decoded, err := url.PathUnescape(raw); err == nil {
		return decoded
	}
	return raw
}

// dashboardExists reports whether the dashboard is real, so storage rows can
// never be created for an ID the caller invented.
func (h *DashboardsHandler) dashboardExists(id string) bool {
	var found string
	err := h.db.QueryRow("SELECT id FROM dashboards WHERE id = ?", id).Scan(&found)
	return err == nil && found != ""
}

// ListStorage returns every stored key for a dashboard as a JSON object.
func (h *DashboardsHandler) ListStorage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.dashboardExists(id) {
		writeError(w, http.StatusNotFound, "dashboard not found")
		return
	}

	rows, err := h.db.Query(
		"SELECT key, value FROM dashboard_storage WHERE dashboard_id = ? ORDER BY key ASC", id,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read dashboard storage")
		return
	}
	defer rows.Close()

	items := map[string]json.RawMessage{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		raw := json.RawMessage(value)
		if !json.Valid(raw) {
			raw = json.RawMessage("null")
		}
		items[key] = raw
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}

// GetStorageItem returns one stored value. A missing key is not an error — the
// dashboard gets {"value": null} so first-run code needs no special casing.
func (h *DashboardsHandler) GetStorageItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	key := storageKey(r)
	if !h.dashboardExists(id) {
		writeError(w, http.StatusNotFound, "dashboard not found")
		return
	}

	var value string
	err := h.db.QueryRow(
		"SELECT value FROM dashboard_storage WHERE dashboard_id = ? AND key = ?", id, key,
	).Scan(&value)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusOK, map[string]interface{}{"key": key, "value": nil, "found": false})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read dashboard storage")
		return
	}

	raw := json.RawMessage(value)
	if !json.Valid(raw) {
		raw = json.RawMessage("null")
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"key": key, "value": raw, "found": true})
}

// SetStorageItem writes one value, replacing any previous value for the key.
func (h *DashboardsHandler) SetStorageItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	key := storageKey(r)
	if !h.dashboardExists(id) {
		writeError(w, http.StatusNotFound, "dashboard not found")
		return
	}
	if key == "" || len(key) > maxStorageKeyLen {
		writeError(w, http.StatusBadRequest, "key is required and must be under 200 characters")
		return
	}

	var req struct {
		Value json.RawMessage `json:"value"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Value) == 0 {
		req.Value = json.RawMessage("null")
	}
	if !json.Valid(req.Value) {
		writeError(w, http.StatusBadRequest, "value must be valid JSON")
		return
	}
	if len(req.Value) > maxStorageValueBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "value is too large (max 512KB)")
		return
	}

	// Only new keys count against the quota — overwriting an existing one is
	// always allowed, so a dashboard can never be locked out of its own data.
	var existing string
	h.db.QueryRow("SELECT key FROM dashboard_storage WHERE dashboard_id = ? AND key = ?", id, key).Scan(&existing)
	if existing == "" {
		var count int
		h.db.QueryRow("SELECT COUNT(*) FROM dashboard_storage WHERE dashboard_id = ?", id).Scan(&count)
		if count >= maxStorageKeys {
			writeError(w, http.StatusInsufficientStorage, "dashboard storage is full (max 500 keys)")
			return
		}
	}

	now := time.Now().UTC()
	if _, err := h.db.Exec(
		`INSERT INTO dashboard_storage (dashboard_id, key, value, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(dashboard_id, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		id, key, string(req.Value), now, now,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save dashboard storage")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"key": key, "value": req.Value, "saved": true})
}

// DeleteStorageItem removes one key. Deleting a key that was never set is a
// success — the caller's intent (the key is gone) already holds.
func (h *DashboardsHandler) DeleteStorageItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	key := storageKey(r)
	if !h.dashboardExists(id) {
		writeError(w, http.StatusNotFound, "dashboard not found")
		return
	}

	if _, err := h.db.Exec(
		"DELETE FROM dashboard_storage WHERE dashboard_id = ? AND key = ?", id, key,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete from dashboard storage")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"key": key, "deleted": true})
}

// ClearStorage removes every key for a dashboard.
func (h *DashboardsHandler) ClearStorage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !h.dashboardExists(id) {
		writeError(w, http.StatusNotFound, "dashboard not found")
		return
	}

	if _, err := h.db.Exec("DELETE FROM dashboard_storage WHERE dashboard_id = ?", id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to clear dashboard storage")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"cleared": true})
}
