-- Device names are chosen by the account owner at enrollment time.  Keeping
-- them on the one-time token lets the Gateway create the node atomically.
ALTER TABLE xcloud_control_enrollment_tokens ADD COLUMN node_name VARCHAR(96) NULL;

CREATE TABLE IF NOT EXISTS xcloud_selfhosted_readiness_events (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  node_id VARCHAR(64) NOT NULL,
  ready BOOLEAN NOT NULL,
  issue_code VARCHAR(64) NOT NULL DEFAULT '',
  message VARCHAR(512) NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  KEY idx_xcloud_selfhosted_readiness_events_node_time (node_id,created_at),
  KEY idx_xcloud_selfhosted_readiness_events_expiry (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
