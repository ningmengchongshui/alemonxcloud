CREATE TABLE IF NOT EXISTS xcloud_coupon_claims (
  id VARCHAR(64) PRIMARY KEY, owner_id VARCHAR(191) NOT NULL, promotion_id VARCHAR(64) NOT NULL,
  coupon_id VARCHAR(64) NOT NULL, status VARCHAR(16) NOT NULL DEFAULT 'active', claimed_at DATETIME NOT NULL,
  used_at DATETIME NULL, order_id VARCHAR(64) NULL,
  UNIQUE KEY uq_xcloud_coupon_claim_owner_coupon (owner_id, coupon_id),
  INDEX idx_xcloud_coupon_claims_owner (owner_id, status, claimed_at),
  INDEX idx_xcloud_coupon_claims_promotion (promotion_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
