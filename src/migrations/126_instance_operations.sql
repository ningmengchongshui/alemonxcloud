ALTER TABLE xcloud_instances ADD COLUMN spec_hash CHAR(64) NULL;
ALTER TABLE xcloud_instances ADD COLUMN reconcile_requested_at DATETIME NULL;
ALTER TABLE xcloud_instances ADD COLUMN reconciled_at DATETIME NULL;

CREATE TABLE IF NOT EXISTS xcloud_instance_operations (
  id VARCHAR(64) PRIMARY KEY,
  instance_id VARCHAR(64) NOT NULL,
  generation BIGINT NOT NULL,
  operation_kind VARCHAR(32) NOT NULL,
  status VARCHAR(24) NOT NULL,
  idempotency_key VARCHAR(160) NOT NULL,
  spec_hash CHAR(64) NULL,
  task_id VARCHAR(64) NULL,
  saga_id VARCHAR(64) NULL,
  requested_at DATETIME NOT NULL,
  started_at DATETIME NULL,
  finished_at DATETIME NULL,
  next_retry_at DATETIME NULL,
  error_message VARCHAR(512) NULL,
  UNIQUE KEY uq_xcloud_instance_operation_idempotency (instance_id,idempotency_key),
  KEY idx_xcloud_instance_operations_reconcile (instance_id,status,generation),
  KEY idx_xcloud_instance_operations_task (task_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT IGNORE INTO xcloud_instance_operations (id,instance_id,generation,operation_kind,status,idempotency_key,task_id,requested_at,started_at,finished_at,next_retry_at,error_message)
SELECT CONCAT('legacy-',t.id),t.instance_id,COALESCE(t.desired_generation,0),t.action,
  CASE WHEN t.status='pending' THEN 'pending' WHEN t.status='running' THEN 'running' WHEN t.status='succeeded' THEN 'done' WHEN t.status='cancelled' THEN 'cancel_requested' ELSE 'error' END,
  CONCAT('legacy:',t.id),t.id,t.created_at,t.claimed_at,t.finished_at,t.run_after,NULLIF(t.last_error,'')
FROM xcloud_tasks t;
