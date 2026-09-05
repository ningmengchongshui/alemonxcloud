ALTER TABLE xcloud_tasks ADD COLUMN execution_token VARCHAR(64) NULL;
ALTER TABLE xcloud_tasks ADD COLUMN heartbeat_at DATETIME NULL;
ALTER TABLE xcloud_tasks ADD COLUMN recovery_count INT NOT NULL DEFAULT 0;
ALTER TABLE xcloud_instances ADD COLUMN active_task_id VARCHAR(64) NULL;
ALTER TABLE xcloud_instances ADD COLUMN active_task_token VARCHAR(64) NULL;
ALTER TABLE xcloud_instances ADD COLUMN active_task_expires_at DATETIME NULL;
CREATE INDEX idx_xcloud_instances_active_task ON xcloud_instances (active_task_expires_at);
