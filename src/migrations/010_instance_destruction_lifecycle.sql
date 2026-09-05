-- A resource may be removed before its persistent data is physically purged.
ALTER TABLE xcloud_instances ADD COLUMN runtime_status VARCHAR(32) NULL;
ALTER TABLE xcloud_instances ADD COLUMN destroy_at DATETIME NULL;
ALTER TABLE xcloud_instances ADD COLUMN destroyed_at DATETIME NULL;
ALTER TABLE xcloud_instances ADD COLUMN destroy_reason VARCHAR(24) NULL;
ALTER TABLE xcloud_instances ADD COLUMN archived_at DATETIME NULL;
UPDATE xcloud_instances SET runtime_status=status WHERE runtime_status IS NULL AND status IN ('deploying','running','stopped');
UPDATE xcloud_instances SET runtime_status=CASE WHEN status='retention' THEN 'stopped' ELSE COALESCE(runtime_status,'stopped') END,status='destroy_scheduled',destroy_at=NOW(),destroy_reason='legacy',purge_at=NULL WHERE status IN ('expired','retention');
CREATE INDEX idx_xcloud_instances_destroy_at ON xcloud_instances (status,destroy_at);
CREATE INDEX idx_xcloud_instances_purge_at ON xcloud_instances (status,purge_at);
CREATE INDEX idx_xcloud_instances_archived ON xcloud_instances (owner_id,archived_at,created_at);
