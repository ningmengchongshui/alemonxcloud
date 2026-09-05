-- The migration runner treats MySQL 1091 (index already absent) as an
-- idempotent result, which also keeps this syntax compatible with MariaDB.
ALTER TABLE xcloud_user_coupons DROP INDEX uq_xcloud_user_coupon_batch_owner;
CREATE INDEX idx_xcloud_user_coupons_batch_owner ON xcloud_user_coupons (batch_id, owner_id, status);
