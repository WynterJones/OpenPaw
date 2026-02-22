package database

import (
	"time"

	"github.com/google/uuid"
)

func (db *DB) LogAudit(userID, action, category, target, targetID, details string) {
	if len(details) > 200 {
		details = details[:200]
	}
	id := uuid.New().String()
	now := time.Now().UTC()
	_, _ = db.Exec(
		"INSERT INTO audit_logs (id, user_id, action, category, target, target_id, details, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		id, userID, action, category, target, targetID, details, now,
	)
	if db.OnAudit != nil {
		db.OnAudit(action, category)
	}
}
