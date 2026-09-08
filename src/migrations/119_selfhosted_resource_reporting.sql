-- Keep a user-selected self-hosted capacity fixed, while allowing the default
-- 80% capacity to follow corrected Agent resource reports.
ALTER TABLE xcloud_nodes ADD COLUMN selfhosted_quota_mode VARCHAR(16) NOT NULL DEFAULT 'auto';

-- Existing nodes whose values exactly match the old automatic 80% default can
-- safely continue following hardware/Agent detection. Any other value is an
-- operator choice and must never be overwritten by a heartbeat.
UPDATE xcloud_nodes
SET selfhosted_quota_mode=CASE
  WHEN node_kind='selfhosted'
   AND cpu_quota=ROUND(cpu_detected*0.8,2)
   AND memory_quota_mb=FLOOR(memory_detected_mb*0.8)
  THEN 'auto'
  ELSE 'manual'
END
WHERE node_kind='selfhosted';
