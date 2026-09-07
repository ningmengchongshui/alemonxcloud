-- Self-hosted nodes are owned by a user and are reached through xcloud-control.
-- Platform nodes keep the existing HTTP Agent connection model.
ALTER TABLE xcloud_nodes ADD COLUMN node_kind VARCHAR(24) NOT NULL DEFAULT 'platform';
ALTER TABLE xcloud_nodes ADD COLUMN owner_id VARCHAR(191) NULL;
ALTER TABLE xcloud_nodes ADD COLUMN control_device_id VARCHAR(64) NULL;
ALTER TABLE xcloud_nodes ADD COLUMN cpu_quota DECIMAL(8,2) NOT NULL DEFAULT 0;
ALTER TABLE xcloud_nodes ADD COLUMN memory_quota_mb INT NOT NULL DEFAULT 0;
ALTER TABLE xcloud_nodes ADD UNIQUE KEY uq_xcloud_nodes_control_device (control_device_id);
ALTER TABLE xcloud_nodes ADD KEY idx_xcloud_nodes_owner_kind (owner_id,node_kind);

ALTER TABLE xcloud_instances ADD COLUMN placement_type VARCHAR(24) NOT NULL DEFAULT 'platform';
ALTER TABLE xcloud_instances ADD KEY idx_xcloud_instances_node_placement (node_id,placement_type,status);
