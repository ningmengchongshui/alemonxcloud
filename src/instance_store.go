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
	return initInstanceStoreWithDSN(dsn)
}

func initInstanceStoreWithDSN(dsn string) error {
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
	rows, err := instanceDB.QueryContext(ctx, `SELECT id,name,image,version,spec,status,COALESCE(runtime_status,''),access_address,created_at,destroy_at,destroyed_at,purge_at,COALESCE(destroy_reason,''),archived_at FROM xcloud_instances WHERE owner_id=? AND archived_at IS NULL ORDER BY created_at DESC`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []instance{}
	for rows.Next() {
		var item instance
		var created time.Time
		if err := rows.Scan(&item.ID, &item.Name, &item.Image, &item.Version, &item.Spec, &item.Status, &item.RuntimeStatus, &item.IP, &created, &item.DestroyAt, &item.DestroyedAt, &item.PurgeAt, &item.DestroyReason, &item.ArchivedAt); err != nil {
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
	err := instanceDB.QueryRowContext(ctx, `SELECT id,name,image,version,spec,status,COALESCE(runtime_status,''),access_address,container_name,created_at,destroy_at,destroyed_at,purge_at,COALESCE(destroy_reason,''),archived_at FROM xcloud_instances WHERE id=? AND owner_id=?`, id, ownerID).Scan(&item.ID, &item.Name, &item.Image, &item.Version, &item.Spec, &item.Status, &item.RuntimeStatus, &item.IP, &item.ContainerName, &created, &item.DestroyAt, &item.DestroyedAt, &item.PurgeAt, &item.DestroyReason, &item.ArchivedAt)
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
