-- A consumer may disappear after claiming a task. The lease lets the control
-- loop safely return that task to the durable pending queue.
ALTER TABLE xcloud_tasks ADD COLUMN claimed_at DATETIME NULL;
ALTER TABLE xcloud_tasks ADD COLUMN claim_expires_at DATETIME NULL;
ALTER TABLE xcloud_tasks ADD COLUMN worker_id VARCHAR(128) NULL;
CREATE INDEX idx_xcloud_tasks_lease ON xcloud_tasks (status,claim_expires_at);
