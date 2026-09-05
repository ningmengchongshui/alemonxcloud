CREATE TABLE IF NOT EXISTS xcloud_coupon_batches (
  id VARCHAR(64) PRIMARY KEY, name VARCHAR(128) NOT NULL, status VARCHAR(16) NOT NULL DEFAULT 'paused',
  distribution_mode VARCHAR(16) NOT NULL, discount_type VARCHAR(16) NOT NULL, discount_value INT NOT NULL,
  min_amount_fen INT NOT NULL DEFAULT 0, max_discount_fen INT NOT NULL DEFAULT 0,
  scope VARCHAR(16) NOT NULL DEFAULT 'both', plan_ids JSON NULL, image_ids JSON NULL, month_values JSON NULL,
  starts_at DATETIME NULL, ends_at DATETIME NULL, issue_limit INT NOT NULL DEFAULT 0, per_user_limit INT NOT NULL DEFAULT 1,
  issued_count INT NOT NULL DEFAULT 0, created_by VARCHAR(191) NOT NULL, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL,
  INDEX idx_xcloud_coupon_batches_public (status, distribution_mode, starts_at, ends_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS xcloud_user_coupons (
  id VARCHAR(64) PRIMARY KEY, batch_id VARCHAR(64) NOT NULL, owner_id VARCHAR(191) NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'available', issued_by VARCHAR(191) NULL, issue_source VARCHAR(16) NOT NULL,
  expires_at DATETIME NULL, used_at DATETIME NULL, voided_at DATETIME NULL, order_id VARCHAR(64) NULL, created_at DATETIME NOT NULL,
  UNIQUE KEY uq_xcloud_user_coupon_batch_owner (batch_id, owner_id),
  INDEX idx_xcloud_user_coupons_owner (owner_id, status, expires_at), INDEX idx_xcloud_user_coupons_batch (batch_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS xcloud_coupon_issuance_runs (
  id VARCHAR(64) PRIMARY KEY, batch_id VARCHAR(64) NOT NULL, mode VARCHAR(16) NOT NULL, actor_id VARCHAR(191) NOT NULL, created_at DATETIME NOT NULL,
  INDEX idx_xcloud_coupon_issuance_runs_batch (batch_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS xcloud_coupon_issuance_items (
  id VARCHAR(64) PRIMARY KEY, run_id VARCHAR(64) NOT NULL, owner_id VARCHAR(191) NOT NULL, user_coupon_id VARCHAR(64) NULL, status VARCHAR(16) NOT NULL, reason VARCHAR(255) NULL, created_at DATETIME NOT NULL,
  INDEX idx_xcloud_coupon_issuance_items_run (run_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
ALTER TABLE xcloud_coupon_redemptions ADD COLUMN user_coupon_id VARCHAR(64) NULL;
CREATE INDEX idx_xcloud_redemptions_user_coupon ON xcloud_coupon_redemptions (user_coupon_id, created_at);
-- Legacy campaign rules which already had coupons become paused public batches.
INSERT IGNORE INTO xcloud_coupon_batches (id,name,status,distribution_mode,discount_type,discount_value,min_amount_fen,max_discount_fen,scope,plan_ids,image_ids,month_values,starts_at,ends_at,issue_limit,per_user_limit,issued_count,created_by,created_at,updated_at)
SELECT CONCAT('legacy_',p.id),p.name,'paused','public',p.discount_type,p.discount_value,p.min_amount_fen,p.max_discount_fen,p.scope,p.plan_ids,p.image_ids,p.month_values,p.starts_at,p.ends_at,p.total_limit,p.per_user_limit,p.used_count,p.created_by,p.created_at,NOW()
FROM xcloud_promotions p WHERE p.kind='campaign' AND EXISTS (SELECT 1 FROM xcloud_coupons c WHERE c.promotion_id=p.id);
INSERT IGNORE INTO xcloud_user_coupons (id,batch_id,owner_id,status,issue_source,expires_at,used_at,order_id,created_at)
SELECT CONCAT('legacy_',cl.id),CONCAT('legacy_',cl.promotion_id),cl.owner_id,CASE WHEN cl.status='used' THEN 'used' ELSE 'available' END,'legacy',p.ends_at,cl.used_at,cl.order_id,cl.claimed_at
FROM xcloud_coupon_claims cl JOIN xcloud_promotions p ON p.id=cl.promotion_id
WHERE EXISTS (SELECT 1 FROM xcloud_coupon_batches b WHERE b.id=CONCAT('legacy_',cl.promotion_id));
UPDATE xcloud_coupon_redemptions r JOIN xcloud_coupon_claims cl ON cl.order_id=r.order_id
SET r.user_coupon_id=CONCAT('legacy_',cl.id) WHERE r.user_coupon_id IS NULL;
