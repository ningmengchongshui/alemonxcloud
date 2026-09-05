ALTER TABLE xcloud_benefit_codes ADD UNIQUE KEY uq_xcloud_benefit_code_program (program_id);
ALTER TABLE xcloud_orders ADD COLUMN price_tier_snapshot JSON NULL;
ALTER TABLE xcloud_orders ADD COLUMN benefit_trigger VARCHAR(16) NULL;
ALTER TABLE xcloud_orders ADD COLUMN benefit_priority INT NULL;
ALTER TABLE xcloud_orders ADD COLUMN benefit_channel_label VARCHAR(64) NULL;
ALTER TABLE xcloud_benefit_grants ADD COLUMN voided_at DATETIME NULL;
ALTER TABLE xcloud_benefit_grants ADD COLUMN voided_by VARCHAR(191) NULL;
