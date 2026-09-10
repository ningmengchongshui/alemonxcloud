-- Declarative resource envelope. Existing lifecycle/task records are retained
-- as the compatibility implementation of an InstanceOperation.
ALTER TABLE xcloud_instances ADD COLUMN resource_version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE xcloud_instances ADD COLUMN desired_power_state VARCHAR(16) NULL;
ALTER TABLE xcloud_instances ADD COLUMN desired_image_revision JSON NULL;
ALTER TABLE xcloud_instances ADD COLUMN desired_recreate_nonce VARCHAR(64) NULL;
ALTER TABLE xcloud_instances ADD COLUMN deletion_intent VARCHAR(24) NULL;
ALTER TABLE xcloud_instances ADD COLUMN spec_updated_at DATETIME NULL;
CREATE INDEX idx_xcloud_instances_resource_version ON xcloud_instances (id,resource_version);

CREATE TABLE IF NOT EXISTS xcloud_instance_conditions (
  instance_id VARCHAR(64) NOT NULL,
  condition_type VARCHAR(40) NOT NULL,
  condition_status VARCHAR(16) NOT NULL,
  reason VARCHAR(64) NOT NULL DEFAULT '',
  message VARCHAR(512) NOT NULL DEFAULT '',
  operation_id VARCHAR(64) NULL,
  observed_generation BIGINT NOT NULL DEFAULT 0,
  updated_at DATETIME NOT NULL,
  PRIMARY KEY (instance_id,condition_type),
  KEY idx_xcloud_instance_conditions_operation (operation_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

UPDATE xcloud_instances
SET desired_power_state=CASE
  WHEN status='running' THEN 'running'
  WHEN status IN ('stopped','destroy_scheduled','destroyed','purged') THEN 'stopped'
  ELSE desired_power_state END,
  deletion_intent=CASE WHEN status IN ('destroy_scheduled','destroyed','purged') THEN 'absent' ELSE deletion_intent END,
  spec_updated_at=COALESCE(spec_updated_at,created_at);
