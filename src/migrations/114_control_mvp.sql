CREATE TABLE IF NOT EXISTS xcloud_control_devices (
  id VARCHAR(64) PRIMARY KEY,
  owner_id VARCHAR(191) NOT NULL,
  name VARCHAR(96) NOT NULL,
  public_key TEXT NOT NULL,
  credential_hash CHAR(64) NOT NULL,
  status VARCHAR(24) NOT NULL DEFAULT 'enabled',
  client_version VARCHAR(64) NOT NULL DEFAULT '',
  last_heartbeat_at DATETIME NULL,
  last_error VARCHAR(255) NULL,
  revoked_at DATETIME NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  INDEX idx_xcloud_control_devices_owner (owner_id, status),
  UNIQUE KEY uq_xcloud_control_credential (credential_hash)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS xcloud_control_authorizations (
  id VARCHAR(64) PRIMARY KEY,
  name VARCHAR(96) NOT NULL,
  public_key TEXT NOT NULL,
  client_version VARCHAR(64) NOT NULL DEFAULT '',
  poll_token_hash CHAR(64) NOT NULL,
  status VARCHAR(24) NOT NULL DEFAULT 'pending',
  owner_id VARCHAR(191) NULL,
  device_id VARCHAR(64) NULL,
  issued_credential TEXT NULL,
  expires_at DATETIME NOT NULL,
  delivered_at DATETIME NULL,
  created_at DATETIME NOT NULL,
  INDEX idx_xcloud_control_authorizations_expiry (status, expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS xcloud_control_routes (
  id VARCHAR(64) PRIMARY KEY,
  owner_id VARCHAR(191) NOT NULL,
  device_id VARCHAR(64) NOT NULL,
  route_key VARCHAR(32) NOT NULL,
  target_url VARCHAR(255) NOT NULL DEFAULT '',
  status VARCHAR(24) NOT NULL DEFAULT 'paused',
  access_address VARCHAR(255) NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  UNIQUE KEY uq_xcloud_control_route_key (route_key),
  UNIQUE KEY uq_xcloud_control_device_route (device_id),
  INDEX idx_xcloud_control_routes_owner (owner_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
