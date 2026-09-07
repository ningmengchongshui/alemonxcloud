ALTER TABLE xcloud_control_devices ADD COLUMN key_version SMALLINT NOT NULL DEFAULT 1;
ALTER TABLE xcloud_control_devices ADD COLUMN credential_version INT NOT NULL DEFAULT 1;
ALTER TABLE xcloud_control_devices ADD COLUMN rebind_required BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE xcloud_control_devices ADD COLUMN last_connected_at DATETIME NULL;
ALTER TABLE xcloud_control_devices ADD COLUMN gateway_id VARCHAR(96) NULL;
ALTER TABLE xcloud_control_devices ADD INDEX idx_xcloud_control_devices_gateway (gateway_id,last_connected_at);
UPDATE xcloud_control_devices SET rebind_required=TRUE WHERE key_version=1;

CREATE TABLE IF NOT EXISTS xcloud_tunnel_gateways (
  id VARCHAR(96) PRIMARY KEY,
  internal_url VARCHAR(255) NOT NULL,
  region VARCHAR(48) NOT NULL DEFAULT 'default',
  status VARCHAR(24) NOT NULL DEFAULT 'online',
  last_heartbeat_at DATETIME NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS xcloud_control_events (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  device_id VARCHAR(64) NOT NULL,
  event_type VARCHAR(48) NOT NULL,
  detail VARCHAR(512) NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  INDEX idx_xcloud_control_events_device (device_id,created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
