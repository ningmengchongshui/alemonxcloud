-- Separate desired instance intent, observed runtime facts, and plan-change
-- settlement. Existing rows retain generation 1 and are safe to reconcile.
ALTER TABLE xcloud_instances ADD COLUMN desired_generation BIGINT NOT NULL DEFAULT 1;
ALTER TABLE xcloud_instances ADD COLUMN observed_cpu DECIMAL(8,2) NULL;
ALTER TABLE xcloud_instances ADD COLUMN observed_memory_mb INT NULL;
ALTER TABLE xcloud_instances ADD COLUMN observed_runtime_status VARCHAR(32) NULL;
ALTER TABLE xcloud_instances ADD COLUMN observed_at DATETIME NULL;

ALTER TABLE xcloud_tasks ADD COLUMN desired_generation BIGINT NOT NULL DEFAULT 0;
ALTER TABLE xcloud_tasks ADD COLUMN cancel_requested_at DATETIME NULL;
ALTER TABLE xcloud_tasks ADD COLUMN cancel_reason VARCHAR(128) NULL;
CREATE INDEX idx_xcloud_tasks_generation ON xcloud_tasks (instance_id,desired_generation,status);

ALTER TABLE xcloud_instance_plan_changes ADD COLUMN saga_status VARCHAR(24) NOT NULL DEFAULT 'applying';
ALTER TABLE xcloud_instance_plan_changes ADD COLUMN compensation_task_id VARCHAR(64) NULL;
ALTER TABLE xcloud_instance_plan_changes ADD COLUMN cancel_requested_at DATETIME NULL;
ALTER TABLE xcloud_instance_plan_changes ADD COLUMN cancel_reason VARCHAR(128) NULL;
ALTER TABLE xcloud_instance_plan_changes ADD COLUMN observed_cpu DECIMAL(8,2) NULL;
ALTER TABLE xcloud_instance_plan_changes ADD COLUMN observed_memory_mb INT NULL;
ALTER TABLE xcloud_instance_plan_changes ADD COLUMN observed_at DATETIME NULL;
CREATE INDEX idx_xcloud_plan_changes_saga ON xcloud_instance_plan_changes (saga_status,updated_at);

UPDATE xcloud_instance_plan_changes
SET saga_status=CASE status
  WHEN 'succeeded' THEN 'settled'
  WHEN 'failed' THEN 'cancelled'
  WHEN 'needs_review' THEN 'exception'
  ELSE 'applying'
END;

-- Existing in-flight resizes gain the instance's current fence. Historical
-- review rows are put back on the observation path; this is not a replay of
-- the resize command and therefore cannot mutate resources.
UPDATE xcloud_tasks t
JOIN xcloud_instances i ON i.id=t.instance_id
SET t.desired_generation=i.desired_generation
WHERE t.action='resize' AND t.status IN ('pending','running')
  AND t.desired_generation=0;

UPDATE xcloud_tasks t
JOIN xcloud_instance_plan_changes p ON p.task_id=t.id
SET t.status='needs_review',t.finished_at=NULL,t.run_after=NOW(),
    t.last_error=CONCAT(COALESCE(t.last_error,''),'\n迁移后将自动核实实际资源'),
    t.updated_at=NOW()
WHERE p.status='needs_review' AND t.action='resize'
  AND t.status IN ('failed','cancelled');
