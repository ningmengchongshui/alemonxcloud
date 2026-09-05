-- `new_user` used to conflate account-level newcomer eligibility and a
-- product's first-purchase eligibility. Existing rules retain the former
-- account-level meaning after this migration.
UPDATE xcloud_promotions
SET kind='newcomer', updated_at=NOW()
WHERE kind='new_user';

CREATE INDEX idx_xcloud_orders_owner_plan_wallet
  ON xcloud_orders (owner_id, plan_id, payment_source, created_at);
