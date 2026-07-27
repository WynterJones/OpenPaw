-- Give every workbench a definite workspace before terminals become
-- workspace-scoped.
--
-- 055 added workspace_id with a default, but the column is nullable and rows
-- written by CreateWorkbench never named it, so a workbench could carry NULL.
-- Once listing filters on workspace_id those rows would match no workspace at
-- all and their terminals would vanish from the UI — the sessions would still
-- be running, just unreachable. Pin them to the seeded Default workspace, which
-- is where they have effectively been living all along.
UPDATE workbenches
SET workspace_id = '00000000-0000-0000-0000-000000000001'
WHERE workspace_id IS NULL OR workspace_id = '';
