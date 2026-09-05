ALTER TABLE xcloud_plan_price_tiers ADD COLUMN discount_bps INT NOT NULL DEFAULT 10000;
ALTER TABLE xcloud_plan_price_tiers DROP COLUMN price_fen;
