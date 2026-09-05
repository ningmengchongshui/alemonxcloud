-- A plan advertises an upper bound, not dedicated bandwidth. Existing plans
-- receive a conservative 10 Mbps ceiling until an administrator edits them.
ALTER TABLE xcloud_plans ADD COLUMN bandwidth_mbps INT NOT NULL DEFAULT 10;
ALTER TABLE xcloud_instances ADD COLUMN bandwidth_mbps INT NOT NULL DEFAULT 10;
ALTER TABLE xcloud_orders ADD COLUMN bandwidth_mbps INT NOT NULL DEFAULT 10;
