CREATE TABLE IF NOT EXISTS xcloud_instance_refund_settlements (
  id VARCHAR(64) PRIMARY KEY,
  instance_id VARCHAR(64) NOT NULL,
  owner_id VARCHAR(191) NOT NULL,
  requested_order_id VARCHAR(64) NOT NULL,
  total_refund_fen INT NOT NULL DEFAULT 0,
  service_ends_at DATETIME NOT NULL,
  data_purge_at DATETIME NOT NULL,
  idempotency_key VARCHAR(128) NOT NULL,
  created_at DATETIME NOT NULL,
  UNIQUE KEY uq_xcloud_instance_refund_settlement_key (idempotency_key),
  INDEX idx_xcloud_instance_refund_settlements_instance (instance_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS xcloud_instance_refund_settlement_items (
  id VARCHAR(64) PRIMARY KEY,
  settlement_id VARCHAR(64) NOT NULL,
  order_id VARCHAR(64) NOT NULL,
  refund_fen INT NOT NULL DEFAULT 0,
  wallet_entry_id VARCHAR(64) NULL,
  created_at DATETIME NOT NULL,
  UNIQUE KEY uq_xcloud_instance_refund_settlement_order (settlement_id, order_id),
  UNIQUE KEY uq_xcloud_instance_refund_settlement_wallet (wallet_entry_id),
  INDEX idx_xcloud_instance_refund_settlement_items_order (order_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

ALTER TABLE xcloud_instance_plan_changes ADD COLUMN reservation_fen INT NOT NULL DEFAULT 0;
ALTER TABLE xcloud_order_exchange_items ADD COLUMN tier_months INT NOT NULL DEFAULT 1;
ALTER TABLE xcloud_order_exchange_items ADD COLUMN benefit_program_id VARCHAR(64) NULL;
ALTER TABLE xcloud_order_exchange_items ADD COLUMN benefit_discount_fen INT NOT NULL DEFAULT 0;
