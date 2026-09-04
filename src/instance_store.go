package cloud

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

var instanceDB *sql.DB
var memoryInstances = map[string]instance{}
var memoryInstancesMu sync.RWMutex

func initInstanceStore() error {
	dsn := env("MYSQL_DSN", "")
	if dsn == "" {
		log.Printf("instance store: in-memory (development only)")
		return nil
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("open MySQL: %w", err)
	}
	db.SetConnMaxLifetime(3 * time.Minute)
	// External MySQL/proxy deployments often reclaim idle TCP connections.
	// Retire them proactively so a new purchase does not begin on a stale one.
	db.SetConnMaxIdleTime(30 * time.Second)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("ping MySQL: %w", err)
	}
	const schema = `CREATE TABLE IF NOT EXISTS xcloud_instances (
		id VARCHAR(64) PRIMARY KEY, owner_id VARCHAR(191) NOT NULL, name VARCHAR(64) NOT NULL,
		image VARCHAR(255) NOT NULL, version VARCHAR(64) NOT NULL, spec VARCHAR(64) NOT NULL,
		status VARCHAR(32) NOT NULL, access_address VARCHAR(255) NOT NULL, container_name VARCHAR(64) NOT NULL,
		created_at DATETIME NOT NULL, INDEX idx_xcloud_instances_owner (owner_id, created_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`
	if _, err := db.ExecContext(ctx, schema); err != nil {
		_ = db.Close()
		return fmt.Errorf("create instance schema: %w", err)
	}
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS xcloud_settings (setting_key VARCHAR(64) PRIMARY KEY, setting_value JSON NOT NULL, updated_at DATETIME NOT NULL) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS xcloud_nodes (id VARCHAR(64) PRIMARY KEY, name VARCHAR(64) NOT NULL, agent_url VARCHAR(255) NOT NULL, cpu_total DECIMAL(8,2) NOT NULL DEFAULT 16, memory_total_mb INT NOT NULL DEFAULT 262144, enabled BOOLEAN NOT NULL DEFAULT TRUE, last_heartbeat_at DATETIME NULL, created_at DATETIME NOT NULL) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS xcloud_images (id VARCHAR(64) PRIMARY KEY, name VARCHAR(64) NOT NULL, image_ref VARCHAR(255) NOT NULL, image_digest VARCHAR(255) NULL, version VARCHAR(64) NOT NULL, enabled BOOLEAN NOT NULL DEFAULT TRUE, created_at DATETIME NOT NULL, UNIQUE KEY uq_xcloud_image_digest (image_digest), UNIQUE KEY uq_xcloud_image_ref (image_ref)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS xcloud_plans (id VARCHAR(64) PRIMARY KEY, name VARCHAR(64) NOT NULL, cpu DECIMAL(8,2) NOT NULL, memory_mb INT NOT NULL, monthly_price_fen INT NOT NULL, enabled BOOLEAN NOT NULL DEFAULT TRUE, sort_order INT NOT NULL DEFAULT 0, created_at DATETIME NOT NULL) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS xcloud_orders (id VARCHAR(64) PRIMARY KEY, owner_id VARCHAR(191) NOT NULL, plan_id VARCHAR(64) NOT NULL, image_id VARCHAR(64) NOT NULL, instance_id VARCHAR(64) NULL, amount_fen INT NOT NULL, status VARCHAR(32) NOT NULL, payment_note VARCHAR(255) NULL, expires_at DATETIME NULL, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL, INDEX idx_xcloud_orders_owner (owner_id, created_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS xcloud_payments (id VARCHAR(64) PRIMARY KEY, order_id VARCHAR(64) NOT NULL, payer_id VARCHAR(191) NOT NULL, amount_fen INT NOT NULL, reference_no VARCHAR(128) NOT NULL, status VARCHAR(32) NOT NULL, submitted_at DATETIME NOT NULL, reviewed_at DATETIME NULL, reviewer_id VARCHAR(191) NULL, UNIQUE KEY uq_xcloud_payment_reference (reference_no), INDEX idx_xcloud_payments_order (order_id, submitted_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS xcloud_tasks (id VARCHAR(64) PRIMARY KEY, instance_id VARCHAR(64) NOT NULL, action VARCHAR(32) NOT NULL, idempotency_key VARCHAR(128) NOT NULL, status VARCHAR(32) NOT NULL, attempts INT NOT NULL DEFAULT 0, last_error TEXT NULL, run_after DATETIME NOT NULL, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL, UNIQUE KEY uq_xcloud_task_idempotency (idempotency_key)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS xcloud_audit_logs (id BIGINT AUTO_INCREMENT PRIMARY KEY, actor_id VARCHAR(191) NOT NULL, action VARCHAR(64) NOT NULL, target_type VARCHAR(64) NOT NULL, target_id VARCHAR(64) NOT NULL, detail JSON NULL, created_at DATETIME NOT NULL, INDEX idx_xcloud_audit_actor (actor_id, created_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			_ = db.Close()
			return fmt.Errorf("create control schema: %w", err)
		}
	}
	instanceDB = db
	log.Printf("instance store: MySQL")
	return nil
}

func listStoredInstances(ctx context.Context, ownerID string) ([]instance, error) {
	if instanceDB == nil {
		memoryInstancesMu.RLock()
		defer memoryInstancesMu.RUnlock()
		items := make([]instance, 0, len(memoryInstances))
		for _, item := range memoryInstances {
			if item.OwnerID == ownerID {
				items = append(items, item)
			}
		}
		return items, nil
	}
	rows, err := instanceDB.QueryContext(ctx, `SELECT id, name, image, version, spec, status, access_address, created_at FROM xcloud_instances WHERE owner_id=? ORDER BY created_at DESC`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []instance{}
	for rows.Next() {
		var item instance
		var created time.Time
		if err := rows.Scan(&item.ID, &item.Name, &item.Image, &item.Version, &item.Spec, &item.Status, &item.IP, &created); err != nil {
			return nil, err
		}
		item.CreatedAt = created.Format("2006-01-02 15:04")
		items = append(items, item)
	}
	return items, rows.Err()
}

func saveStoredInstance(ctx context.Context, item instance) error {
	if instanceDB == nil {
		memoryInstancesMu.Lock()
		memoryInstances[item.ID] = item
		memoryInstancesMu.Unlock()
		return nil
	}
	created, err := time.ParseInLocation("2006-01-02 15:04", item.CreatedAt, time.Local)
	if err != nil {
		created = time.Now()
	}
	_, err = instanceDB.ExecContext(ctx, `INSERT INTO xcloud_instances (id,owner_id,name,image,version,spec,status,access_address,container_name,created_at) VALUES (?,?,?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE name=VALUES(name), image=VALUES(image), version=VALUES(version), spec=VALUES(spec), status=VALUES(status), access_address=VALUES(access_address), container_name=VALUES(container_name)`, item.ID, item.OwnerID, item.Name, item.Image, item.Version, item.Spec, item.Status, item.IP, item.ContainerName, created)
	return err
}

func getStoredInstance(ctx context.Context, id, ownerID string) (instance, bool, error) {
	if instanceDB == nil {
		memoryInstancesMu.RLock()
		item, ok := memoryInstances[id]
		memoryInstancesMu.RUnlock()
		return item, ok && item.OwnerID == ownerID, nil
	}
	var item instance
	var created time.Time
	err := instanceDB.QueryRowContext(ctx, `SELECT id, name, image, version, spec, status, access_address, container_name, created_at FROM xcloud_instances WHERE id=? AND owner_id=?`, id, ownerID).Scan(&item.ID, &item.Name, &item.Image, &item.Version, &item.Spec, &item.Status, &item.IP, &item.ContainerName, &created)
	if err == sql.ErrNoRows {
		return instance{}, false, nil
	}
	if err != nil {
		return instance{}, false, err
	}
	item.OwnerID = ownerID
	item.CreatedAt = created.Format("2006-01-02 15:04")
	return item, true, nil
}

func removeStoredInstance(ctx context.Context, id, ownerID string) error {
	if instanceDB == nil {
		memoryInstancesMu.Lock()
		delete(memoryInstances, id)
		memoryInstancesMu.Unlock()
		return nil
	}
	_, err := instanceDB.ExecContext(ctx, `DELETE FROM xcloud_instances WHERE id=? AND owner_id=?`, id, ownerID)
	return err
}
