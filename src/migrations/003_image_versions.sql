CREATE TABLE IF NOT EXISTS xcloud_image_versions (
  id VARCHAR(64) PRIMARY KEY, image_id VARCHAR(64) NOT NULL, version_tag VARCHAR(64) NOT NULL,
  image_digest VARCHAR(255) NULL, enabled BOOLEAN NOT NULL DEFAULT TRUE, created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL, UNIQUE KEY uq_xcloud_image_version (image_id, version_tag),
  INDEX idx_xcloud_image_versions_image (image_id, enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS xcloud_image_version_pulls (
  image_version_id VARCHAR(64) NOT NULL, node_id VARCHAR(64) NOT NULL, status VARCHAR(32) NOT NULL,
  last_error VARCHAR(512) NULL, pulled_at DATETIME NULL, updated_at DATETIME NOT NULL,
  PRIMARY KEY (image_version_id, node_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
ALTER TABLE xcloud_orders ADD COLUMN selected_image_digest VARCHAR(255) NULL;
INSERT IGNORE INTO xcloud_image_versions (id,image_id,version_tag,image_digest,enabled,created_at,updated_at)
  SELECT CONCAT('ver_', id),id,COALESCE(NULLIF(version,''),'latest'),image_digest,enabled,created_at,created_at FROM xcloud_images;
