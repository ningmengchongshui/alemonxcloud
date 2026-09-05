-- Existing percentage rows used an ambiguous reduction semantic. Require an
-- explicit operator review before the new payable-rate interpretation applies.
UPDATE xcloud_promotions SET enabled=FALSE, updated_at=NOW() WHERE discount_type='percent';
CREATE INDEX idx_xcloud_redemptions_coupon_owner ON xcloud_coupon_redemptions (coupon_id, owner_id, created_at);
CREATE INDEX idx_xcloud_redemptions_coupon_created ON xcloud_coupon_redemptions (coupon_id, created_at);
