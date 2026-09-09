ALTER TABLE xcloud_tasks ADD COLUMN agent_operation_id VARCHAR(96) NULL;
ALTER TABLE xcloud_tasks ADD COLUMN agent_operation_state VARCHAR(24) NULL;

CREATE TABLE IF NOT EXISTS xcloud_selfhosted_agent_operations (
  id VARCHAR(96) NOT NULL PRIMARY KEY,
  task_id VARCHAR(64) NOT NULL,
  instance_id VARCHAR(64) NOT NULL,
  node_id VARCHAR(64) NOT NULL,
  action VARCHAR(32) NOT NULL,
  execution_token VARCHAR(96) NOT NULL,
  desired_state VARCHAR(24) NOT NULL,
  config_summary VARCHAR(128) NOT NULL DEFAULT '',
  status VARCHAR(24) NOT NULL,
  observed_state VARCHAR(24) NULL,
  safe_error VARCHAR(512) NULL,
  started_at DATETIME NOT NULL,
  finished_at DATETIME NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  UNIQUE KEY uq_xcloud_selfhosted_agent_operations_task_token (task_id,execution_token),
  KEY idx_xcloud_selfhosted_agent_operations_instance_time (instance_id,created_at),
  KEY idx_xcloud_selfhosted_agent_operations_node_time (node_id,created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
