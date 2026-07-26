-- Todo items get a middle state: started but not finished.
--
-- Without it "work on the next task" picks the same item every run, because
-- nothing an agent does mid-task is visible until the box is ticked — two
-- scheduled runs an hour apart would both start the same thing. in_progress is
-- the claim; completed still means done, so nothing that reads `completed`
-- needs to change.
ALTER TABLE todo_items ADD COLUMN in_progress INTEGER NOT NULL DEFAULT 0;
ALTER TABLE todo_items ADD COLUMN started_at DATETIME;
ALTER TABLE todo_items ADD COLUMN started_by_agent_slug TEXT;
