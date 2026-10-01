ALTER TABLE xcloud_users ADD COLUMN host_access_allowed BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE xcloud_instances ADD COLUMN host_access_enabled BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE xcloud_instances ADD COLUMN host_access_applied BOOLEAN NOT NULL DEFAULT FALSE;
