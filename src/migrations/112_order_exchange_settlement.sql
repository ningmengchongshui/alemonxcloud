CREATE TABLE IF NOT EXISTS xcloud_order_exchanges (
  id VARCHAR(64) PRIMARY KEY,
  instance_id VARCHAR(64) NOT NULL,
  owner_id VARCHAR(191) NOT NULL,
  target_plan_id VARCHAR(64) NOT NULL,
  effective_at DATETIME NOT NULL,
  cutoff_at DATETIME NOT NULL,
  task_id VARCHAR(64) NOT NULL,
  status VARCHAR(24) NOT NULL,
  refund_total_fen INT NOT NULL DEFAULT 0,
  charge_total_fen INT NOT NULL DEFAULT 0,
  net_settlement_fen INT NOT NULL DEFAULT 0,
  price_snapshot JSON NOT NULL,
  idempotency_key VARCHAR(128) NOT NULL,
  created_at DATETIME NOT NULL,
  completed_at DATETIME NULL,
  UNIQUE KEY uq_xcloud_order_exchanges_idempotency (idempotency_key),
  INDEX idx_xcloud_order_exchanges_instance (instance_id, created_at),
  INDEX idx_xcloud_order_exchanges_task (task_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS xcloud_order_exchange_items (
  id VARCHAR(64) PRIMARY KEY,
  exchange_id VARCHAR(64) NOT NULL,
  source_order_id VARCHAR(64) NOT NULL,
  replacement_order_id VARCHAR(64) NULL,
  source_plan_id VARCHAR(64) NOT NULL,
  source_starts_at DATETIME NOT NULL,
  source_expires_at DATETIME NOT NULL,
  replacement_starts_at DATETIME NOT NULL,
  replacement_expires_at DATETIME NOT NULL,
  source_refund_fen INT NOT NULL,
  replacement_charge_fen INT NOT NULL,
  tier_discount_bps INT NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  UNIQUE KEY uq_xcloud_order_exchange_item_source (exchange_id, source_order_id),
  INDEX idx_xcloud_order_exchange_items_replacement (replacement_order_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

ALTER TABLE xcloud_orders ADD COLUMN exchange_id VARCHAR(64) NULL;
ALTER TABLE xcloud_orders ADD COLUMN replaces_order_id VARCHAR(64) NULL;
ALTER TABLE xcloud_orders ADD COLUMN order_role VARCHAR(24) NOT NULL DEFAULT 'standard';
ALTER TABLE xcloud_instance_plan_changes ADD COLUMN exchange_id VARCHAR(64) NULL;
CREATE INDEX idx_xcloud_orders_instance_exchange ON xcloud_orders (instance_id, exchange_id, status);
