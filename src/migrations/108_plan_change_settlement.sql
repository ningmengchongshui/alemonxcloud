ALTER TABLE xcloud_instance_plan_changes ADD COLUMN fund_status VARCHAR(24) NOT NULL DEFAULT 'pending';
ALTER TABLE xcloud_instance_plan_changes ADD COLUMN agent_verify_status VARCHAR(24) NOT NULL DEFAULT 'not_checked';
ALTER TABLE xcloud_instance_plan_changes ADD COLUMN agent_verified_at DATETIME NULL;
ALTER TABLE xcloud_instance_plan_changes ADD COLUMN agent_verify_result JSON NULL;
ALTER TABLE xcloud_instance_plan_changes ADD COLUMN agent_verify_error VARCHAR(512) NULL;
ALTER TABLE xcloud_wallet_entries ADD COLUMN plan_change_id VARCHAR(64) NULL;
ALTER TABLE xcloud_wallet_entries ADD COLUMN business_key VARCHAR(128) NULL;
CREATE UNIQUE INDEX uq_xcloud_wallet_plan_change_business ON xcloud_wallet_entries (business_key);
CREATE INDEX idx_xcloud_plan_changes_funds ON xcloud_instance_plan_changes (fund_status, updated_at);
