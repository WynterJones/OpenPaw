-- Add category column to audit_logs for grouping/filtering
ALTER TABLE audit_logs ADD COLUMN category TEXT NOT NULL DEFAULT 'system';

-- Backfill existing rows based on action prefix
UPDATE audit_logs SET category = 'auth' WHERE action IN ('login', 'logout', 'password_changed', 'account_deleted', 'setup_init');
UPDATE audit_logs SET category = 'tool' WHERE action LIKE 'tool_%';
UPDATE audit_logs SET category = 'schedule' WHERE action LIKE 'schedule_%';
UPDATE audit_logs SET category = 'secret' WHERE action LIKE 'secret_%';
UPDATE audit_logs SET category = 'dashboard' WHERE action LIKE 'dashboard_%';
UPDATE audit_logs SET category = 'settings' WHERE action LIKE 'settings_%' OR action LIKE 'design_%' OR action LIKE 'model_%' OR action LIKE 'api_key_%';
UPDATE audit_logs SET category = 'chat' WHERE action LIKE 'chat_%';
UPDATE audit_logs SET category = 'agent' WHERE action LIKE 'agent_%';
UPDATE audit_logs SET category = 'context' WHERE action LIKE 'context_%';

-- Index for efficient category filtering
CREATE INDEX idx_audit_logs_category ON audit_logs(category);
