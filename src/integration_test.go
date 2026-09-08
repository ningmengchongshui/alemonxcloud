//go:build integration

package cloud

import (
	"context"
	"database/sql"
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

func TestIntegrationNodeByIDLoadsAgentCapabilities(t *testing.T) {
	setupIntegrationDB(t)
	ctx := context.Background()
	nodeID := "node_capability_lookup"
	_, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_nodes (id,name,agent_url,cpu_total,memory_total_mb,enabled,agent_capabilities,created_at,updated_at)
		VALUES (?,?,?, ?,?,?,JSON_ARRAY('container.reinstall.v1'),NOW(),NOW())
		ON DUPLICATE KEY UPDATE agent_capabilities=VALUES(agent_capabilities),updated_at=NOW()`,
		nodeID, nodeID, "http://agent.invalid", 1, 1024, true)
	if err != nil {
		t.Fatalf("insert node: %v", err)
	}
	t.Cleanup(func() { _, _ = instanceDB.ExecContext(ctx, `DELETE FROM xcloud_nodes WHERE id=?`, nodeID) })

	n, err := nodeByID(ctx, nodeID)
	if err != nil {
		t.Fatalf("load node: %v", err)
	}
	if !n.supportsAgentCapability("container.reinstall.v1") {
		t.Fatalf("node lookup dropped agent capabilities: %#v", n.AgentCapabilities)
	}
}

func TestIntegrationRevokeSelfHostedDeviceReleasesNodeAndCancelsPendingTasks(t *testing.T) {
	setupIntegrationDB(t)
	ctx := context.Background()
	suffix := newID("revoke")
	ownerID := "user_" + suffix
	deviceID := "ctl_" + suffix
	nodeID := "snode_" + suffix
	instanceID := "shi_" + suffix
	taskID := "task_" + suffix
	runningTaskID := "task_running_" + suffix
	workerID := "worker_" + suffix
	executionToken := "exec_" + suffix

	defer func() {
		_, _ = instanceDB.ExecContext(ctx, `DELETE FROM xcloud_task_events WHERE task_id=?`, taskID)
		_, _ = instanceDB.ExecContext(ctx, `DELETE FROM xcloud_task_events WHERE task_id=?`, runningTaskID)
		_, _ = instanceDB.ExecContext(ctx, `DELETE FROM xcloud_tasks WHERE id=?`, taskID)
		_, _ = instanceDB.ExecContext(ctx, `DELETE FROM xcloud_tasks WHERE id=?`, runningTaskID)
		_, _ = instanceDB.ExecContext(ctx, `DELETE FROM xcloud_instances WHERE id=?`, instanceID)
		_, _ = instanceDB.ExecContext(ctx, `DELETE FROM xcloud_control_routes WHERE device_id=?`, deviceID)
		_, _ = instanceDB.ExecContext(ctx, `DELETE FROM xcloud_nodes WHERE id=?`, nodeID)
		_, _ = instanceDB.ExecContext(ctx, `DELETE FROM xcloud_control_devices WHERE id=?`, deviceID)
		_, _ = instanceDB.ExecContext(ctx, `DELETE FROM xcloud_audit_logs WHERE actor_id=? AND action='control.device.revoke'`, ownerID)
	}()

	if _, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_control_devices (id,owner_id,name,public_key,credential_hash,status,client_version,created_at,updated_at) VALUES (?,?,?,?,?,'enabled','test',NOW(),NOW())`, deviceID, ownerID, "test-control", "test-key", controlHash(deviceID)); err != nil {
		t.Fatal(err)
	}
	if _, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_control_routes (id,owner_id,device_id,route_key,target_url,status,access_address,created_at,updated_at) VALUES (?,?,?,?,?,'enabled',?,NOW(),NOW())`, "ctr_"+suffix, ownerID, deviceID, "r"+suffix[:16], "", "https://example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_nodes (id,name,agent_url,cpu_total,memory_total_mb,enabled,node_kind,owner_id,control_device_id,selfhosted_ready,created_at,updated_at) VALUES (?,?,?,1,1024,TRUE,'selfhosted',?,?,TRUE,NOW(),NOW())`, nodeID, "自建节点", "", ownerID, deviceID); err != nil {
		t.Fatal(err)
	}
	if _, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_instances (id,owner_id,name,image,version,spec,status,access_address,container_name,created_at,node_id,placement_type,runtime_status) VALUES (?,?,?,?,?,?,?,'https://example.test',?,NOW(),?,'selfhosted','running')`, instanceID, ownerID, "test", "example/test", "latest", "1 核 / 1 GB", "running", "xcloud-"+suffix[:16], nodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_tasks (id,instance_id,action,idempotency_key,status,attempts,run_after,created_at,updated_at) VALUES (?,?,? ,? ,?,0,NOW(),NOW(),NOW())`, taskID, instanceID, "restart", "revoke:"+taskID, taskPending); err != nil {
		t.Fatal(err)
	}
	if _, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_tasks (id,instance_id,action,idempotency_key,status,attempts,run_after,created_at,updated_at,claimed_at,heartbeat_at,claim_expires_at,worker_id,execution_token) VALUES (?,?,? ,? ,?,1,NOW(),NOW(),NOW(),NOW(),NOW(),DATE_ADD(NOW(), INTERVAL 5 MINUTE),?,?)`, runningTaskID, instanceID, "restart", "revoke:"+runningTaskID, taskRunning, workerID, executionToken); err != nil {
		t.Fatal(err)
	}
	if _, err := instanceDB.ExecContext(ctx, `UPDATE xcloud_instances SET active_task_id=?,active_task_token=?,active_task_expires_at=DATE_ADD(NOW(), INTERVAL 5 MINUTE) WHERE id=?`, runningTaskID, executionToken, instanceID); err != nil {
		t.Fatal(err)
	}

	result, err := revokeSelfHostedDevice(ctx, ownerID)
	if err != nil {
		t.Fatalf("revoke self-hosted device: %v", err)
	}
	if result.DeviceID != deviceID || result.NodeCount != 1 || len(result.CancelledTaskIDs) != 1 || result.CancelledTaskIDs[0] != taskID {
		t.Fatalf("unexpected revoke result: %#v", result)
	}
	var deviceStatus, taskStatus, runningTaskStatus, instanceStatus, runtimeStatus string
	var enabled, ready bool
	if err := instanceDB.QueryRowContext(ctx, `SELECT status FROM xcloud_control_devices WHERE id=?`, deviceID).Scan(&deviceStatus); err != nil {
		t.Fatal(err)
	}
	if err := instanceDB.QueryRowContext(ctx, `SELECT enabled,selfhosted_ready FROM xcloud_nodes WHERE id=?`, nodeID).Scan(&enabled, &ready); err != nil {
		t.Fatal(err)
	}
	if err := instanceDB.QueryRowContext(ctx, `SELECT status,runtime_status FROM xcloud_instances WHERE id=?`, instanceID).Scan(&instanceStatus, &runtimeStatus); err != nil {
		t.Fatal(err)
	}
	if err := instanceDB.QueryRowContext(ctx, `SELECT status FROM xcloud_tasks WHERE id=?`, taskID).Scan(&taskStatus); err != nil {
		t.Fatal(err)
	}
	if err := instanceDB.QueryRowContext(ctx, `SELECT status FROM xcloud_tasks WHERE id=?`, runningTaskID).Scan(&runningTaskStatus); err != nil {
		t.Fatal(err)
	}
	if deviceStatus != "revoked" || enabled || ready || instanceStatus != "running" || runtimeStatus != "unknown" || taskStatus != taskCanceled || runningTaskStatus != taskRunning {
		t.Fatalf("revoke cleanup mismatch: device=%s enabled=%v ready=%v instance=%s/%s task=%s runningTask=%s", deviceStatus, enabled, ready, instanceStatus, runtimeStatus, taskStatus, runningTaskStatus)
	}
	if err := taskMayCallAgent(ctx, controlTask{ID: runningTaskID, InstanceID: instanceID, Action: "restart", WorkerID: workerID, ExecutionToken: executionToken}); err == nil {
		t.Fatal("a running task must be fenced from calling a revoked self-hosted node")
	}

	// Retrying the user action after a device has already been revoked is a
	// cleanup operation, not an error. It must remove any stale node record
	// that survived an older release without requiring a new device binding.
	if _, err := instanceDB.ExecContext(ctx, `UPDATE xcloud_nodes SET enabled=TRUE,selfhosted_ready=TRUE WHERE id=?`, nodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := instanceDB.ExecContext(ctx, `UPDATE xcloud_tasks SET status=?,finished_at=NULL,last_error=NULL,updated_at=NOW() WHERE id=?`, taskPending, taskID); err != nil {
		t.Fatal(err)
	}
	alreadyUnbound, err := revokeSelfHostedDevice(ctx, ownerID)
	if err != nil {
		t.Fatalf("clean up already-unbound self-hosted node: %v", err)
	}
	if alreadyUnbound.DeviceID != "" || alreadyUnbound.NodeCount != 1 || len(alreadyUnbound.CancelledTaskIDs) != 1 || alreadyUnbound.CancelledTaskIDs[0] != taskID {
		t.Fatalf("already-unbound cleanup mismatch: %#v", alreadyUnbound)
	}
}

func TestIntegrationConcurrentRefundCreditsOnce(t *testing.T) {
	setupIntegrationDB(t)
	ctx := context.Background()
	suffix := newID("it")
	ownerID, instanceID, orderID := "user_"+suffix, "ins_"+suffix, "ord_"+suffix
	secondOrderID := "ord_second_" + suffix
	now := time.Now().UTC().Truncate(time.Second)
	start, end := now.Add(-5*24*time.Hour), now.Add(10*24*time.Hour)
	secondEnd := end.Add(10 * 24 * time.Hour)
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
	if _, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_instances (id,owner_id,name,image,version,spec,status,access_address,container_name,created_at,expires_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, instanceID, ownerID, "integration", "example/test", "latest", "1 核 / 1 GB", "running", "https://example.test", "xcloud-abcdef123456", now, secondEnd); err != nil {
		t.Fatal(err)
	}
	if _, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_orders (id,owner_id,plan_id,image_id,instance_id,amount_fen,status,payment_source,service_starts_at,expires_at,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, orderID, ownerID, "plan", "image", instanceID, 1500, orderActive, "wallet", start, end, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_orders (id,owner_id,plan_id,image_id,instance_id,amount_fen,status,payment_source,service_starts_at,expires_at,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, secondOrderID, ownerID, "plan", "image", instanceID, 900, orderActive, "wallet", end, secondEnd, now, now); err != nil {
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
	if err := instanceDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM xcloud_wallet_entries WHERE user_id=? AND entry_type='refund'`, ownerID).Scan(&refunds); err != nil {
		t.Fatal(err)
	}
	if err := instanceDB.QueryRowContext(ctx, `SELECT balance_fen FROM xcloud_wallets WHERE user_id=?`, ownerID).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if refunds != 2 || balance <= 0 {
		t.Fatalf("refund ledger inconsistent: count=%d balance=%d", refunds, balance)
	}
}
func TestIntegrationRefundSucceedsAfterManualPurge(t *testing.T) {
	setupIntegrationDB(t)
	ctx := context.Background()
	suffix := newID("purged-refund")
	ownerID, instanceID, orderID := "user_"+suffix, "ins_"+suffix, "ord_"+suffix
	now := time.Now().UTC().Truncate(time.Second)
	start, end := now.Add(-5*24*time.Hour), now.Add(10*24*time.Hour)
	defer func() {
		for _, table := range []string{"xcloud_wallet_entries", "xcloud_orders", "xcloud_instances", "xcloud_wallets", "xcloud_users"} {
			_, _ = instanceDB.Exec(`DELETE FROM `+table+` WHERE `+map[string]string{"xcloud_wallet_entries": "user_id", "xcloud_orders": "owner_id", "xcloud_instances": "owner_id", "xcloud_wallets": "user_id", "xcloud_users": "id"}[table]+`=?`, ownerID)
		}
	}()
	if _, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_users (id,username,email,last_login_at,created_at) VALUES (?,?,?,?,?)`, ownerID, ownerID, ownerID+"@example.test", now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_wallets (user_id,balance_fen,updated_at) VALUES (?,0,?)`, ownerID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_instances (id,owner_id,name,image,version,spec,status,access_address,container_name,created_at,expires_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, instanceID, ownerID, "purged", "example/test", "v1", "1 核 / 1 GB", "purged", "https://example.test", "xcloud-purged", now, end); err != nil {
		t.Fatal(err)
	}
	if _, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_orders (id,owner_id,plan_id,image_id,instance_id,amount_fen,status,payment_source,service_starts_at,expires_at,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, orderID, ownerID, "plan", "image", instanceID, 1500, orderActive, "wallet", start, end, now, now); err != nil {
		t.Fatal(err)
	}
	if _, _, err := refundOrder(ctx, ownerID, orderID); err != nil {
		t.Fatalf("refund after purge: %v", err)
	}
	var status string
	if err := instanceDB.QueryRowContext(ctx, `SELECT status FROM xcloud_instances WHERE id=?`, instanceID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "purged" {
		t.Fatalf("purged instance must remain purged, got %q", status)
	}
	var orderStatus string
	var balance int
	if err := instanceDB.QueryRowContext(ctx, `SELECT status FROM xcloud_orders WHERE id=?`, orderID).Scan(&orderStatus); err != nil {
		t.Fatal(err)
	}
	if err := instanceDB.QueryRowContext(ctx, `SELECT balance_fen FROM xcloud_wallets WHERE user_id=?`, ownerID).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if orderStatus != orderRefund || balance <= 0 {
		t.Fatalf("purged refund did not settle: status=%q balance=%d", orderStatus, balance)
	}
}

func TestIntegrationLifecycleTransitionAndLeaseRecovery(t *testing.T) {
	setupIntegrationDB(t)
	ctx := context.Background()
	suffix := newID("lifecycle")
	ownerID, instanceID, taskID := "user_"+suffix, "ins_"+suffix, "task_"+suffix
	now := time.Now().UTC().Truncate(time.Second)
	defer func() {
		_, _ = instanceDB.Exec(`DELETE FROM xcloud_task_events WHERE task_id=?`, taskID)
		_, _ = instanceDB.Exec(`DELETE FROM xcloud_audit_logs WHERE target_id=?`, taskID)
		_, _ = instanceDB.Exec(`DELETE FROM xcloud_tasks WHERE id=?`, taskID)
		_, _ = instanceDB.Exec(`DELETE FROM xcloud_instances WHERE id=?`, instanceID)
	}()
	if _, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_instances (id,owner_id,name,image,version,spec,status,access_address,container_name,created_at,expires_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, instanceID, ownerID, "lifecycle", "example/test", "v1", "1 核 / 1 GB", "running", "https://example.test", "xcloud-lifecycle", now, now.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	runtime := "stopped"
	changed, err := transitionInstance(ctx, instanceDB, instanceID, []string{"running"}, "stopped", &runtime, "")
	if err != nil || !changed {
		t.Fatalf("transition running -> stopped: changed=%v err=%v", changed, err)
	}
	changed, err = transitionInstance(ctx, instanceDB, instanceID, []string{"running"}, "destroy_scheduled", &runtime, "")
	if err != nil || changed {
		t.Fatalf("stale transition must not overwrite newer state: changed=%v err=%v", changed, err)
	}
	if _, err = instanceDB.ExecContext(ctx, `INSERT INTO xcloud_tasks (id,instance_id,action,idempotency_key,status,attempts,run_after,created_at,updated_at,claimed_at,claim_expires_at,worker_id) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, taskID, instanceID, "start", "lease:"+taskID, taskRunning, 1, now, now, now, now.Add(-10*time.Minute), now.Add(-time.Minute), "crashed-worker"); err != nil {
		t.Fatal(err)
	}
	recoverExpiredTaskLeases(ctx)
	var status string
	var claimedAt, expiresAt sql.NullTime
	var workerID sql.NullString
	if err = instanceDB.QueryRowContext(ctx, `SELECT status,claimed_at,claim_expires_at,worker_id FROM xcloud_tasks WHERE id=?`, taskID).Scan(&status, &claimedAt, &expiresAt, &workerID); err != nil {
		t.Fatal(err)
	}
	if status != taskPending || claimedAt.Valid || expiresAt.Valid || workerID.Valid {
		t.Fatalf("expired lease was not safely recovered: status=%s claimed=%v expires=%v worker=%v", status, claimedAt.Valid, expiresAt.Valid, workerID.Valid)
	}
	var events int
	if err = instanceDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM xcloud_task_events WHERE task_id=? AND event_type='lease_recovered'`, taskID).Scan(&events); err != nil || events != 1 {
		t.Fatalf("lease recovery event: count=%d err=%v", events, err)
	}
}

func setupIntegrationDB(t *testing.T) {
	t.Helper()
	dsn := os.Getenv("XCLOUD_INTEGRATION_MYSQL_DSN")
	if dsn == "" {
		t.Fatal("XCLOUD_INTEGRATION_MYSQL_DSN is required: integration tests must run against the isolated Compose MySQL")
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
