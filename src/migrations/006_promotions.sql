ALTER TABLE xcloud_orders ADD COLUMN list_amount_fen INT NOT NULL DEFAULT 0;
ALTER TABLE xcloud_orders ADD COLUMN discount_amount_fen INT NOT NULL DEFAULT 0;
ALTER TABLE xcloud_orders ADD COLUMN promotion_snapshot JSON NULL;
ALTER TABLE xcloud_orders ADD COLUMN coupon_redemption_id VARCHAR(64) NULL;
UPDATE xcloud_orders SET list_amount_fen=amount_fen WHERE list_amount_fen=0;
CREATE TABLE IF NOT EXISTS xcloud_promotions (
  id VARCHAR(64) PRIMARY KEY, name VARCHAR(128) NOT NULL, kind VARCHAR(24) NOT NULL,
  scope VARCHAR(16) NOT NULL DEFAULT 'both', discount_type VARCHAR(16) NOT NULL,
  discount_value INT NOT NULL, min_amount_fen INT NOT NULL DEFAULT 0, max_discount_fen INT NOT NULL DEFAULT 0,
  plan_ids JSON NULL, image_ids JSON NULL, month_values JSON NULL,
  starts_at DATETIME NULL, ends_at DATETIME NULL, total_limit INT NOT NULL DEFAULT 0,
  per_user_limit INT NOT NULL DEFAULT 1, used_count INT NOT NULL DEFAULT 0,
  enabled BOOLEAN NOT NULL DEFAULT TRUE, created_by VARCHAR(191) NOT NULL, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL,
  INDEX idx_xcloud_promotions_active (enabled, starts_at, ends_at), INDEX idx_xcloud_promotions_kind (kind, enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS xcloud_coupons (
  id VARCHAR(64) PRIMARY KEY, promotion_id VARCHAR(64) NOT NULL, code_hash CHAR(64) NOT NULL,
  code_mask VARCHAR(32) NOT NULL, mode VARCHAR(16) NOT NULL DEFAULT 'single', enabled BOOLEAN NOT NULL DEFAULT TRUE,
  total_limit INT NOT NULL DEFAULT 1, per_user_limit INT NOT NULL DEFAULT 1, used_count INT NOT NULL DEFAULT 0,
  created_by VARCHAR(191) NOT NULL, created_at DATETIME NOT NULL, disabled_at DATETIME NULL,
  UNIQUE KEY uq_xcloud_coupons_hash (code_hash), INDEX idx_xcloud_coupons_promotion (promotion_id, enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS xcloud_coupon_redemptions (
  id VARCHAR(64) PRIMARY KEY, promotion_id VARCHAR(64) NOT NULL, coupon_id VARCHAR(64) NULL,
  owner_id VARCHAR(191) NOT NULL, order_id VARCHAR(64) NOT NULL, discount_amount_fen INT NOT NULL,
  created_at DATETIME NOT NULL, UNIQUE KEY uq_xcloud_redemption_order (order_id),
  INDEX idx_xcloud_redemptions_owner (owner_id, promotion_id, created_at), INDEX idx_xcloud_redemptions_promotion (promotion_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
