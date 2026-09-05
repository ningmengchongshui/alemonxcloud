CREATE TABLE IF NOT EXISTS xcloud_instances (
  id VARCHAR(64) PRIMARY KEY, owner_id VARCHAR(191) NOT NULL, name VARCHAR(64) NOT NULL,
  image VARCHAR(255) NOT NULL, version VARCHAR(64) NOT NULL, spec VARCHAR(64) NOT NULL,
  status VARCHAR(32) NOT NULL, access_address VARCHAR(255) NOT NULL, container_name VARCHAR(64) NOT NULL,
  created_at DATETIME NOT NULL, cpu DECIMAL(8,2) NOT NULL DEFAULT 0, memory_mb INT NOT NULL DEFAULT 0,
  node_id VARCHAR(64) NULL, order_id VARCHAR(64) NULL, route_key VARCHAR(32) NULL,
  expires_at DATETIME NULL, purge_at DATETIME NULL, retention_days SMALLINT NOT NULL DEFAULT 7,
  INDEX idx_xcloud_instances_owner (owner_id, created_at), UNIQUE KEY uq_xcloud_route_key (route_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS xcloud_settings (setting_key VARCHAR(64) PRIMARY KEY, setting_value JSON NOT NULL, updated_at DATETIME NOT NULL) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS xcloud_nodes (
  id VARCHAR(64) PRIMARY KEY, name VARCHAR(64) NOT NULL, agent_url VARCHAR(255) NOT NULL,
  cpu_total DECIMAL(8,2) NOT NULL DEFAULT 16, memory_total_mb INT NOT NULL DEFAULT 262144,
  enabled BOOLEAN NOT NULL DEFAULT TRUE, last_heartbeat_at DATETIME NULL, created_at DATETIME NOT NULL,
  updated_at DATETIME NULL, cpu_detected DECIMAL(8,2) NOT NULL DEFAULT 0, memory_detected_mb INT NOT NULL DEFAULT 0,
  agent_token_ciphertext TEXT NULL, docker_version VARCHAR(64) NULL, disk_available_bytes BIGINT NULL,
  managed_container_count INT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS xcloud_images (
  id VARCHAR(64) PRIMARY KEY, name VARCHAR(64) NOT NULL, image_ref VARCHAR(255) NOT NULL,
  image_digest VARCHAR(255) NULL, version VARCHAR(64) NOT NULL, enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at DATETIME NOT NULL, UNIQUE KEY uq_xcloud_image_digest (image_digest), UNIQUE KEY uq_xcloud_image_ref (image_ref)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS xcloud_plans (
  id VARCHAR(64) PRIMARY KEY, name VARCHAR(64) NOT NULL, cpu DECIMAL(8,2) NOT NULL,
  memory_mb INT NOT NULL, monthly_price_fen INT NOT NULL, enabled BOOLEAN NOT NULL DEFAULT TRUE,
  sort_order INT NOT NULL DEFAULT 0, created_at DATETIME NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS xcloud_orders (
  id VARCHAR(64) PRIMARY KEY, owner_id VARCHAR(191) NOT NULL, plan_id VARCHAR(64) NOT NULL,
  image_id VARCHAR(64) NOT NULL, instance_id VARCHAR(64) NULL, amount_fen INT NOT NULL,
  status VARCHAR(32) NOT NULL, payment_note VARCHAR(255) NULL, expires_at DATETIME NULL,
  created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL, renewal_instance_id VARCHAR(64) NULL,
  payment_source VARCHAR(32) NULL, wallet_entry_id VARCHAR(64) NULL, scheduled_node_id VARCHAR(64) NULL,
  selected_image_version VARCHAR(64) NULL, service_starts_at DATETIME NULL, refunded_at DATETIME NULL,
  refund_amount_fen INT NULL, refund_wallet_entry_id VARCHAR(64) NULL,
  INDEX idx_xcloud_orders_owner (owner_id, created_at), UNIQUE KEY uq_xcloud_orders_refund_entry (refund_wallet_entry_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS xcloud_payments (
  id VARCHAR(64) PRIMARY KEY, order_id VARCHAR(64) NOT NULL, payer_id VARCHAR(191) NOT NULL,
  amount_fen INT NOT NULL, reference_no VARCHAR(128) NOT NULL, status VARCHAR(32) NOT NULL,
  submitted_at DATETIME NOT NULL, reviewed_at DATETIME NULL, reviewer_id VARCHAR(191) NULL,
  UNIQUE KEY uq_xcloud_payment_reference (reference_no), INDEX idx_xcloud_payments_order (order_id, submitted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS xcloud_tasks (
  id VARCHAR(64) PRIMARY KEY, instance_id VARCHAR(64) NOT NULL, action VARCHAR(32) NOT NULL,
  idempotency_key VARCHAR(128) NOT NULL, status VARCHAR(32) NOT NULL, attempts INT NOT NULL DEFAULT 0,
  last_error TEXT NULL, run_after DATETIME NOT NULL, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL,
  payload JSON NULL, finished_at DATETIME NULL, UNIQUE KEY uq_xcloud_task_idempotency (idempotency_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS xcloud_audit_logs (
  id BIGINT AUTO_INCREMENT PRIMARY KEY, actor_id VARCHAR(191) NOT NULL, action VARCHAR(64) NOT NULL,
  target_type VARCHAR(64) NOT NULL, target_id VARCHAR(64) NOT NULL, detail JSON NULL, created_at DATETIME NOT NULL,
  INDEX idx_xcloud_audit_actor (actor_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS xcloud_users (
  id VARCHAR(191) PRIMARY KEY, username VARCHAR(191) NOT NULL, email VARCHAR(255) NOT NULL,
  last_login_at DATETIME NOT NULL, created_at DATETIME NOT NULL, INDEX idx_xcloud_users_username (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS xcloud_wallets (user_id VARCHAR(191) PRIMARY KEY, balance_fen BIGINT NOT NULL DEFAULT 0, updated_at DATETIME NOT NULL) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS xcloud_wallet_entries (
  id VARCHAR(64) PRIMARY KEY, user_id VARCHAR(191) NOT NULL, amount_fen BIGINT NOT NULL,
  balance_after_fen BIGINT NOT NULL, entry_type VARCHAR(32) NOT NULL, note VARCHAR(255) NOT NULL,
  actor_id VARCHAR(191) NOT NULL, order_id VARCHAR(64) NULL, created_at DATETIME NOT NULL,
  INDEX idx_xcloud_wallet_entries_user (user_id, created_at), INDEX idx_xcloud_wallet_entries_order (order_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS xcloud_notifications (
  id VARCHAR(64) PRIMARY KEY, user_id VARCHAR(191) NOT NULL, notification_type VARCHAR(48) NOT NULL,
  title VARCHAR(128) NOT NULL, body VARCHAR(512) NOT NULL, data JSON NULL, read_at DATETIME NULL,
  created_at DATETIME NOT NULL, INDEX idx_xcloud_notifications_user (user_id, read_at, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS xcloud_instance_notification_events (
  instance_id VARCHAR(64) NOT NULL, event_type VARCHAR(32) NOT NULL, created_at DATETIME NOT NULL,
  PRIMARY KEY (instance_id, event_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS xcloud_task_events (
  id BIGINT AUTO_INCREMENT PRIMARY KEY, task_id VARCHAR(64) NOT NULL, event_type VARCHAR(32) NOT NULL,
  detail VARCHAR(1024) NOT NULL DEFAULT '', created_at DATETIME NOT NULL,
  INDEX idx_xcloud_task_events_task (task_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS xcloud_tickets (
  id VARCHAR(64) PRIMARY KEY, owner_id VARCHAR(191) NOT NULL, category VARCHAR(32) NOT NULL,
  priority VARCHAR(16) NOT NULL DEFAULT 'normal', subject VARCHAR(160) NOT NULL, instance_id VARCHAR(64) NULL,
  order_id VARCHAR(64) NULL, status VARCHAR(16) NOT NULL DEFAULT 'open', last_admin_id VARCHAR(191) NULL,
  closed_at DATETIME NULL, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL,
  INDEX idx_xcloud_tickets_owner (owner_id, updated_at), INDEX idx_xcloud_tickets_queue (status, priority, updated_at),
  INDEX idx_xcloud_tickets_instance (instance_id), INDEX idx_xcloud_tickets_order (order_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS xcloud_ticket_messages (
  id VARCHAR(64) PRIMARY KEY, ticket_id VARCHAR(64) NOT NULL, sender_id VARCHAR(191) NOT NULL,
  sender_role VARCHAR(16) NOT NULL, body TEXT NOT NULL, created_at DATETIME NOT NULL,
  INDEX idx_xcloud_ticket_messages_ticket (ticket_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
