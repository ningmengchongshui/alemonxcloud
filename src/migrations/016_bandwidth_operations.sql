ALTER TABLE xcloud_instances ADD COLUMN bandwidth_status VARCHAR(24) NOT NULL DEFAULT 'pending';
ALTER TABLE xcloud_instances ADD COLUMN bandwidth_applied_at DATETIME NULL;
ALTER TABLE xcloud_instances ADD COLUMN bandwidth_last_error VARCHAR(512) NULL;
