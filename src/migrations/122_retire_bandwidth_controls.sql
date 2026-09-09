-- Historical monetary and instance snapshots retain their old columns, but
-- traffic shaping is no longer a product capability.  Fence any task that
-- survived a rolling deployment before the old worker can call an Agent.
UPDATE xcloud_tasks
SET status='cancelled',last_error=CONCAT(COALESCE(last_error,''),'\n带宽控制已下线，任务已作废'),finished_at=NOW(),claimed_at=NULL,heartbeat_at=NULL,claim_expires_at=NULL,worker_id=NULL,execution_token=NULL,updated_at=NOW()
WHERE action='bandwidth' AND status IN ('pending','running','needs_review');
