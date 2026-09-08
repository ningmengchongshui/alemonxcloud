-- A self-hosted device can maintain its tunnel while Docker or its instance
-- runtime is unavailable. Keep connectivity and deployability distinct.
ALTER TABLE xcloud_nodes ADD COLUMN selfhosted_ready BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE xcloud_nodes ADD COLUMN selfhosted_readiness JSON NULL;

CREATE TABLE IF NOT EXISTS xcloud_task_diagnostics (
  task_id VARCHAR(64) PRIMARY KEY,
  node_id VARCHAR(64) NOT NULL,
  owner_id VARCHAR(191) NOT NULL,
  error_code VARCHAR(64) NOT NULL,
  safe_message VARCHAR(512) NOT NULL,
  diagnostic MEDIUMTEXT NOT NULL,
  expires_at DATETIME NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  KEY idx_xcloud_task_diagnostics_node_owner (node_id,owner_id,created_at),
  KEY idx_xcloud_task_diagnostics_expiry (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
