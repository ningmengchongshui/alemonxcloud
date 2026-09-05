-- Existing sellable versions predate verified distribution and remain readable.
-- New versions start as drafts and can become sellable only after Agent pull
-- reports one consistent immutable digest across all healthy target nodes.
ALTER TABLE xcloud_image_versions ADD COLUMN version_status VARCHAR(24) NOT NULL DEFAULT 'ready';
ALTER TABLE xcloud_image_versions ADD COLUMN published_at DATETIME NULL;
ALTER TABLE xcloud_image_versions ADD COLUMN last_error VARCHAR(512) NULL;
ALTER TABLE xcloud_image_version_pulls ADD COLUMN resolved_digest VARCHAR(255) NULL;
ALTER TABLE xcloud_image_version_pulls ADD COLUMN local_image_id VARCHAR(128) NULL;
UPDATE xcloud_image_versions SET version_status=CASE WHEN enabled=TRUE THEN 'ready' ELSE 'disabled' END WHERE version_status='' OR version_status IS NULL;
