package cloud

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
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

// listStoredInstances returns only platform-managed instances for the main
// “我的实例” surface. Self-hosted instances are deliberately listed through
// their owning node's dedicated API instead.
func listStoredInstances(ctx context.Context, ownerID string) ([]instance, error) {
	if instanceDB == nil {
		memoryInstancesMu.RLock()
		defer memoryInstancesMu.RUnlock()
		items := make([]instance, 0, len(memoryInstances))
		for _, item := range memoryInstances {
			if item.OwnerID == ownerID && item.ArchivedAt == nil {
				items = append(items, item)
			}
		}
		return items, nil
	}
	rows, err := instanceDB.QueryContext(ctx, `SELECT i.id,COALESCE(i.resource_version,1),i.name,i.image,i.version,i.spec,i.cpu,i.memory_mb,i.status,COALESCE(i.runtime_status,''),i.access_address,i.container_name,i.created_at,COALESCE((SELECT MIN(o.service_starts_at) FROM xcloud_orders o WHERE o.instance_id=i.id AND o.owner_id=i.owner_id AND o.status IN ('active','expired') AND o.service_starts_at IS NOT NULL),i.created_at),i.expires_at,i.destroy_at,i.destroyed_at,i.purge_at,COALESCE(i.destroy_reason,''),i.archived_at,COALESCE(img.terminal_only,FALSE),COALESCE(active_task.id,''),COALESCE(active_task.action,''),COALESCE(active_task.status,'')
		FROM xcloud_instances i
		LEFT JOIN xcloud_orders source_order ON source_order.id=i.order_id
		LEFT JOIN xcloud_images img ON img.id=source_order.image_id
		LEFT JOIN xcloud_tasks active_task ON active_task.id=(
			SELECT t.id FROM xcloud_tasks t
			WHERE t.instance_id=i.id AND t.status IN ('pending','running')
			AND t.action IN ('create','retry-deploy','start','stop','update','restart','reinstall','destroy','purge','resize','compensate-resize')
			ORDER BY t.created_at DESC LIMIT 1
		)
		WHERE i.owner_id=? AND i.archived_at IS NULL
		AND COALESCE(i.placement_type,'platform')<>'selfhosted'
		ORDER BY i.created_at DESC`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []instance{}
	for rows.Next() {
		var item instance
		var created time.Time
		var serviceStart time.Time
		var serviceEnd sql.NullTime
		var cpu float64
		var memoryMB int
		var activeTask instanceActiveTask
		if err := rows.Scan(&item.ID, &item.ResourceVersion, &item.Name, &item.Image, &item.Version, &item.Spec, &cpu, &memoryMB, &item.Status, &item.RuntimeStatus, &item.IP, &item.ContainerName, &created, &serviceStart, &serviceEnd, &item.DestroyAt, &item.DestroyedAt, &item.PurgeAt, &item.DestroyReason, &item.ArchivedAt, &item.TerminalOnly, &activeTask.ID, &activeTask.Action, &activeTask.Status); err != nil {
			return nil, err
		}
		item.CreatedAt = created.Format("2006-01-02 15:04")
		item.Spec = displayInstanceSpec(cpu, memoryMB, item.Spec)
		item.ServiceStartsAt = &serviceStart
		if serviceEnd.Valid {
			item.ServiceExpiresAt = &serviceEnd.Time
		}
		item.CurrentPlanID, item.CurrentPlanName = effectiveInstancePlan(ctx, ownerID, item.ID)
		var changeStatus, changeID, changeSaga, changeFund string
		_ = instanceDB.QueryRowContext(ctx, `SELECT id,status,COALESCE(saga_status,''),COALESCE(fund_status,'') FROM xcloud_instance_plan_changes WHERE instance_id=? ORDER BY created_at DESC LIMIT 1`, item.ID).Scan(&changeID, &changeStatus, &changeSaga, &changeFund)
		item.PlanChangeID, item.PlanChangeStatus, item.PlanChangeSagaStatus, item.PlanChangeFundStatus = changeID, changeStatus, changeSaga, changeFund
		if activeTask.ID != "" {
			item.ActiveTask = &activeTask
		}
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
	var cpu float64
	var memoryMB int
	err := instanceDB.QueryRowContext(ctx, `SELECT i.id,i.name,i.image,i.version,i.spec,i.cpu,i.memory_mb,i.status,COALESCE(i.runtime_status,''),i.access_address,i.container_name,i.created_at,i.destroy_at,i.destroyed_at,i.purge_at,COALESCE(i.destroy_reason,''),i.archived_at,COALESCE(img.terminal_only,FALSE) FROM xcloud_instances i LEFT JOIN xcloud_orders source_order ON source_order.id=i.order_id LEFT JOIN xcloud_images img ON img.id=source_order.image_id WHERE i.id=? AND i.owner_id=? AND i.archived_at IS NULL`, id, ownerID).Scan(&item.ID, &item.Name, &item.Image, &item.Version, &item.Spec, &cpu, &memoryMB, &item.Status, &item.RuntimeStatus, &item.IP, &item.ContainerName, &created, &item.DestroyAt, &item.DestroyedAt, &item.PurgeAt, &item.DestroyReason, &item.ArchivedAt, &item.TerminalOnly)
	if err == sql.ErrNoRows {
		return instance{}, false, nil
	}
	if err != nil {
		return instance{}, false, err
	}
	item.OwnerID = ownerID
	item.Spec = displayInstanceSpec(cpu, memoryMB, item.Spec)
	item.CreatedAt = created.Format("2006-01-02 15:04")
	return item, true, nil
}

func displayInstanceSpec(cpu float64, memoryMB int, fallback string) string {
	if cpu > 0 && memoryMB > 0 {
		return fmt.Sprintf("%g 核 / %d GB", cpu, (memoryMB+1023)/1024)
	}
	// A pre-upgrade row may lack CPU/memory columns.  Never expose its old
	// bandwidth suffix in that narrow compatibility case.
	if index := strings.Index(fallback, "/"); index >= 0 {
		return strings.TrimSpace(fallback[:index])
	}
	return fallback
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
