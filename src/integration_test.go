//go:build integration

package cloud

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

// TestIntegrationSchema verifies that the opt-in Compose stack can apply the
// same migrations used by release startup. Functional lifecycle tests share
// this isolated database and never touch a developer's configured MySQL.
func TestIntegrationSchema(t *testing.T) {
	setupIntegrationDB(t)
	var count int
	if err := instanceDB.QueryRow(`SELECT COUNT(*) FROM xcloud_schema_migrations`).Scan(&count); err != nil || count < 2 {
		t.Fatalf("migration registry: count=%d err=%v", count, err)
	}
}

func TestIntegrationConcurrentRefundCreditsOnce(t *testing.T) {
	setupIntegrationDB(t)
	ctx := context.Background()
	suffix := newID("it")
	ownerID, instanceID, orderID := "user_"+suffix, "ins_"+suffix, "ord_"+suffix
	now := time.Now().UTC().Truncate(time.Second)
	start, end := now.Add(-5*24*time.Hour), now.Add(10*24*time.Hour)
	cleanup := func() {
		for _, table := range []string{"xcloud_wallet_entries", "xcloud_orders", "xcloud_instances", "xcloud_wallets", "xcloud_users"} {
			_, _ = instanceDB.Exec(`DELETE FROM `+table+` WHERE `+map[string]string{"xcloud_wallet_entries": "user_id", "xcloud_orders": "owner_id", "xcloud_instances": "owner_id", "xcloud_wallets": "user_id", "xcloud_users": "id"}[table]+`=?`, ownerID)
		}
	}
	defer cleanup()
	if _, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_users (id,username,email,last_login_at,created_at) VALUES (?,?,?,?,?)`, ownerID, ownerID, ownerID+"@example.test", now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_wallets (user_id,balance_fen,updated_at) VALUES (?,0,?)`, ownerID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_instances (id,owner_id,name,image,version,spec,status,access_address,container_name,created_at,expires_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, instanceID, ownerID, "integration", "example/test", "latest", "1 核 / 1 GB", "running", "https://example.test", "xcloud-abcdef123456", now, end); err != nil {
		t.Fatal(err)
	}
	if _, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_orders (id,owner_id,plan_id,image_id,instance_id,amount_fen,status,payment_source,service_starts_at,expires_at,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, orderID, ownerID, "plan", "image", instanceID, 1500, orderActive, "wallet", start, end, now, now); err != nil {
		t.Fatal(err)
	}

	results := make(chan error, 2)
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, _, err := refundOrder(ctx, ownerID, orderID)
			results <- err
		}()
	}
	group.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("refund must commit once, successes=%d", successes)
	}
	var refunds, balance int
	if err := instanceDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM xcloud_wallet_entries WHERE user_id=? AND order_id=? AND entry_type='refund'`, ownerID, orderID).Scan(&refunds); err != nil {
		t.Fatal(err)
	}
	if err := instanceDB.QueryRowContext(ctx, `SELECT balance_fen FROM xcloud_wallets WHERE user_id=?`, ownerID).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if refunds != 1 || balance <= 0 {
		t.Fatalf("refund ledger inconsistent: count=%d balance=%d", refunds, balance)
	}
}

func setupIntegrationDB(t *testing.T) {
	t.Helper()
	dsn := os.Getenv("XCLOUD_INTEGRATION_MYSQL_DSN")
	if dsn == "" {
		t.Skip("XCLOUD_INTEGRATION_MYSQL_DSN is not configured")
	}
	if instanceDB == nil {
		if err := initInstanceStoreWithDSN(dsn); err != nil {
			t.Fatalf("connect integration MySQL: %v", err)
		}
	}
	if err := initializeSchemaMigrations(context.Background()); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if instanceDB == nil {
		t.Fatal(fmt.Errorf("integration database unavailable"))
	}
}
