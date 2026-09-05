-- The product has not launched: replace the retired promotion/coupon model.
DROP TABLE IF EXISTS xcloud_coupon_issuance_items;
DROP TABLE IF EXISTS xcloud_coupon_issuance_runs;
DROP TABLE IF EXISTS xcloud_user_coupons;
DROP TABLE IF EXISTS xcloud_coupon_batches;
DROP TABLE IF EXISTS xcloud_coupon_claims;
DROP TABLE IF EXISTS xcloud_coupon_redemptions;
DROP TABLE IF EXISTS xcloud_coupons;
DROP TABLE IF EXISTS xcloud_promotions;

ALTER TABLE xcloud_orders DROP COLUMN promotion_snapshot;
ALTER TABLE xcloud_orders DROP COLUMN coupon_redemption_id;
ALTER TABLE xcloud_orders ADD COLUMN benefit_program_id VARCHAR(64) NULL;
ALTER TABLE xcloud_orders ADD COLUMN benefit_snapshot JSON NULL;
ALTER TABLE xcloud_orders ADD COLUMN bonus_days INT NOT NULL DEFAULT 0;
ALTER TABLE xcloud_orders ADD COLUMN promo_code_mask VARCHAR(32) NULL;

CREATE TABLE IF NOT EXISTS xcloud_plan_price_tiers (
  id VARCHAR(64) PRIMARY KEY,
  plan_id VARCHAR(64) NOT NULL,
  months INT NOT NULL,
  price_fen INT NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  UNIQUE KEY uq_xcloud_plan_price_tier (plan_id, months),
  INDEX idx_xcloud_plan_price_tier_plan (plan_id, enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS xcloud_benefit_programs (
  id VARCHAR(64) PRIMARY KEY,
  name VARCHAR(128) NOT NULL,
  goal VARCHAR(32) NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'draft',
  trigger_type VARCHAR(16) NOT NULL,
  order_scope VARCHAR(16) NOT NULL DEFAULT 'both',
  benefit_type VARCHAR(24) NOT NULL,
  benefit_value INT NOT NULL,
  min_amount_fen INT NOT NULL DEFAULT 0,
  plan_ids JSON NULL,
  month_values JSON NULL,
  audience_type VARCHAR(16) NOT NULL DEFAULT 'all',
  starts_at DATETIME NULL,
  ends_at DATETIME NULL,
  per_user_limit INT NOT NULL DEFAULT 0,
  total_limit INT NOT NULL DEFAULT 0,
  used_count INT NOT NULL DEFAULT 0,
  cash_budget_fen INT NOT NULL DEFAULT 0,
  cash_spent_fen INT NOT NULL DEFAULT 0,
  grant_days_limit INT NOT NULL DEFAULT 0,
  grant_days_used INT NOT NULL DEFAULT 0,
  priority INT NOT NULL DEFAULT 0,
  channel_label VARCHAR(64) NULL,
  created_by VARCHAR(191) NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  INDEX idx_xcloud_benefit_program_active (status, starts_at, ends_at),
  INDEX idx_xcloud_benefit_program_goal (goal, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS xcloud_benefit_codes (
  id VARCHAR(64) PRIMARY KEY,
  program_id VARCHAR(64) NOT NULL,
  code_hash CHAR(64) NOT NULL,
  code_mask VARCHAR(32) NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  total_limit INT NOT NULL DEFAULT 0,
  per_user_limit INT NOT NULL DEFAULT 0,
  used_count INT NOT NULL DEFAULT 0,
  starts_at DATETIME NULL,
  ends_at DATETIME NULL,
  channel_label VARCHAR(64) NULL,
  created_at DATETIME NOT NULL,
  UNIQUE KEY uq_xcloud_benefit_code_hash (code_hash),
  INDEX idx_xcloud_benefit_code_program (program_id, enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS xcloud_benefit_grants (
  id VARCHAR(64) PRIMARY KEY,
  program_id VARCHAR(64) NOT NULL,
  owner_id VARCHAR(191) NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'available',
  expires_at DATETIME NULL,
  used_order_id VARCHAR(64) NULL,
  used_at DATETIME NULL,
  issued_by VARCHAR(191) NOT NULL,
  created_at DATETIME NOT NULL,
  UNIQUE KEY uq_xcloud_benefit_grant (program_id, owner_id),
  INDEX idx_xcloud_benefit_grants_owner (owner_id, status, expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS xcloud_benefit_redemptions (
  id VARCHAR(64) PRIMARY KEY,
  program_id VARCHAR(64) NOT NULL,
  code_id VARCHAR(64) NULL,
  grant_id VARCHAR(64) NULL,
  owner_id VARCHAR(191) NOT NULL,
  order_id VARCHAR(64) NOT NULL,
  discount_amount_fen INT NOT NULL DEFAULT 0,
  bonus_days INT NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  UNIQUE KEY uq_xcloud_benefit_redemption_order (order_id),
  INDEX idx_xcloud_benefit_redemptions_owner (program_id, owner_id, created_at),
  INDEX idx_xcloud_benefit_redemptions_program (program_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
