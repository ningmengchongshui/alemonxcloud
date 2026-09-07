CREATE TABLE IF NOT EXISTS xcloud_control_enrollment_tokens (
  id VARCHAR(64) PRIMARY KEY,
  owner_id VARCHAR(191) NOT NULL,
  token_hash CHAR(64) NOT NULL,
  expires_at DATETIME NOT NULL,
  used_at DATETIME NULL,
  revoked_at DATETIME NULL,
  device_id VARCHAR(64) NULL,
  created_at DATETIME NOT NULL,
  UNIQUE KEY uq_xcloud_control_enrollment_token (token_hash),
  INDEX idx_xcloud_control_enrollment_owner (owner_id,expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
