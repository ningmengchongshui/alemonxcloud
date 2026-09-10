package cloud

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Nodes are polled independently, so an unavailable Agent never becomes
// schedulable simply because another node remains healthy.
func startControlLoops() {
	startDeclarativeReconciler()
	go func() {
		// Desired writes enqueue immediately; this bounded scan is the recovery
		// path for dropped notifications and must meet the 30 second SLO.
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			rows, err := instanceDB.QueryContext(context.Background(), `SELECT id FROM xcloud_instances WHERE reconcile_requested_at IS NOT NULL AND (reconciled_at IS NULL OR reconciled_at<reconcile_requested_at) LIMIT 500`)
			if err == nil {
				for rows.Next() {
					var id string
					if rows.Scan(&id) == nil {
						requestInstanceReconcile(id)
					}
				}
				rows.Close()
			}
			syncBenefitProgramStates(context.Background())
			scheduleLifecycle(context.Background())
			quarantineDangerousFailedTasks(context.Background())
			recoverExpiredTaskLeases(context.Background())
			recoverPendingTasks()
			syncNodeHeartbeat(context.Background())
			syncInstanceStates(context.Background())
			cleanupExpiredTaskDiagnostics(context.Background())
			reconcileUncertainSelfHostedOperations(context.Background())
			reconcileBlockedPlanChanges(context.Background())
		}
	}()
	syncNodeHeartbeat(context.Background())
	syncBenefitProgramStates(context.Background())
	syncInstanceStates(context.Background())
	quarantineDangerousFailedTasks(context.Background())
	recoverExpiredTaskLeases(context.Background())
	cleanupExpiredTaskDiagnostics(context.Background())
	reconcileUncertainSelfHostedOperations(context.Background())
	reconcileBlockedPlanChanges(context.Background())
}

// reconcileUncertainSelfHostedOperations closes the gap between a cancelled
// Docker CLI and Docker's eventual state.  Only simple lifecycle actions are
// finalized automatically; billing or destructive actions deliberately stay
// in needs_review even if their container state is known.
func reconcileUncertainSelfHostedOperations(ctx context.Context) {
	if instanceDB == nil {
		return
	}
	rows, err := instanceDB.QueryContext(ctx, `SELECT o.id,o.task_id,o.instance_id,o.node_id,o.action,o.desired_state,i.container_name,i.status FROM xcloud_selfhosted_agent_operations o JOIN xcloud_tasks t ON t.id=o.task_id JOIN xcloud_instances i ON i.id=o.instance_id WHERE t.status='needs_review' AND o.status IN ('running','unknown') AND i.placement_type='selfhosted' ORDER BY o.updated_at LIMIT 100`)
	if err != nil {
		log.Printf("load uncertain Agent operations: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var operationID, taskID, instanceID, nodeID, action, desired, containerName, lifecycle string
		if err := rows.Scan(&operationID, &taskID, &instanceID, &nodeID, &action, &desired, &containerName, &lifecycle); err != nil {
			continue
		}
		n, nodeErr := nodeByID(ctx, nodeID)
		if nodeErr != nil || n.NodeKind != selfHostedNodeKind || !n.Enabled {
			continue
		}
		var live struct {
			Status        string `json:"status"`
			ObservedState string `json:"observedState"`
			SafeError     string `json:"safeError"`
		}
		path := "/container/" + url.PathEscape(containerName) + "/operation-status?operationId=" + url.QueryEscape(operationID)
		if nodeRequest(ctx, n, http.MethodGet, path, nil, &live) != nil {
			continue
		}
		_, _ = instanceDB.ExecContext(ctx, `UPDATE xcloud_selfhosted_agent_operations SET observed_state=?,safe_error=?,updated_at=NOW() WHERE id=?`, nullableString(live.ObservedState), nullableString(live.SafeError), operationID)
		if !operationMatchesDesired(desired, live.ObservedState) {
			continue
		}
		if !finalizeSafeReconciledTask(ctx, taskID, instanceID, action, lifecycle, operationID, live.ObservedState) {
			continue
		}
		_, _ = instanceDB.ExecContext(ctx, `UPDATE xcloud_selfhosted_agent_operations SET status='succeeded',finished_at=NOW(),updated_at=NOW() WHERE id=?`, operationID)
		appendTaskEvent(ctx, taskID, "agent_operation_reconciled", "Agent 断线后已确认最终容器状态："+live.ObservedState)
		_ = writeAudit(ctx, "system", "task.agent_operation_reconciled", "task", taskID, map[string]any{"instanceId": instanceID, "action": action, "operationId": operationID})
	}
}

func operationMatchesDesired(desired, observed string) bool {
	switch desired {
	case "running":
		return observed == "running"
	case "stopped":
		return observed == "exited" || observed == "created"
	case "absent":
		return observed == "absent"
	default:
		return false
	}
}

func finalizeSafeReconciledTask(ctx context.Context, taskID, instanceID, action, lifecycle, operationID, observed string) bool {
	// Resize changes wallet/order facts; update/reinstall change immutable image
	// facts; destroy/purge have retention requirements. Those remain operator
	// decisions rather than guessing from a container inspect result.
	if action != "create" && action != "retry-deploy" && action != "start" && action != "stop" && action != "restart" {
		return false
	}
	next, runtime := lifecycle, "running"
	if action == "create" || action == "retry-deploy" {
		next = "running"
	}
	if action == "stop" {
		runtime = "stopped"
		if lifecycle != "destroy_scheduled" {
			next = "stopped"
		}
	}
	if action == "start" || action == "restart" {
		if lifecycle != "destroy_scheduled" {
			next = "running"
		}
	}
	if changed, err := transitionInstance(ctx, instanceDB, instanceID, []string{lifecycle}, next, &runtime, ""); err != nil || !changed {
		return false
	}
	if next == "running" && (action == "create" || action == "retry-deploy" || action == "start") {
		_, _ = instanceDB.ExecContext(ctx, `UPDATE xcloud_orders SET status=?,updated_at=NOW() WHERE instance_id=? AND status=?`, orderActive, instanceID, orderDeploy)
	}
	result, err := instanceDB.ExecContext(ctx, `UPDATE xcloud_tasks SET status='succeeded',last_error=NULL,finished_at=NOW(),agent_operation_state='succeeded',updated_at=NOW() WHERE id=? AND status='needs_review' AND agent_operation_id=?`, taskID, operationID)
	if err != nil {
		return false
	}
	changed, _ := result.RowsAffected()
	return changed == 1
}

func syncBenefitProgramStates(ctx context.Context) {
	if instanceDB == nil {
		return
	}
	if _, err := instanceDB.ExecContext(ctx, `UPDATE xcloud_benefit_programs SET status='active',updated_at=NOW() WHERE status='scheduled' AND (starts_at IS NULL OR starts_at<=NOW()) AND (ends_at IS NULL OR ends_at>NOW())`); err != nil {
		log.Printf("activate benefit programs: %v", err)
	}
	if _, err := instanceDB.ExecContext(ctx, `UPDATE xcloud_benefit_programs SET status='ended',updated_at=NOW() WHERE status IN ('scheduled','active') AND ends_at IS NOT NULL AND ends_at<=NOW()`); err != nil {
		log.Printf("end benefit programs: %v", err)
	}
	if _, err := instanceDB.ExecContext(ctx, `UPDATE xcloud_benefit_codes c JOIN xcloud_benefit_programs p ON p.id=c.program_id SET c.enabled=FALSE WHERE p.status IN ('paused','ended') AND c.enabled=TRUE`); err != nil {
		log.Printf("disable inactive benefit codes: %v", err)
	}
}

func enabledNodes(ctx context.Context) ([]node, error) {
	rows, err := instanceDB.QueryContext(ctx, `SELECT id,name,agent_url,cpu_total,memory_total_mb,enabled,last_heartbeat_at,COALESCE(agent_token_ciphertext,''),COALESCE(agent_version,''),COALESCE(agent_api_version,0),COALESCE(agent_capabilities,JSON_ARRAY()) FROM xcloud_nodes WHERE enabled=TRUE AND node_kind='platform' AND last_heartbeat_at>=?`, time.Now().Add(-nodeHeartbeatTTL()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []node{}
	for rows.Next() {
		var n node
		var capabilities []byte
		if err := rows.Scan(&n.ID, &n.Name, &n.AgentURL, &n.CPUTotal, &n.MemoryTotalMB, &n.Enabled, &n.LastHeartbeatAt, &n.AgentToken, &n.AgentVersion, &n.AgentAPIVersion, &capabilities); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(capabilities, &n.AgentCapabilities)
		items = append(items, n)
	}
	return items, rows.Err()
}

// heartbeatNodes deliberately includes enabled nodes without a recent
// heartbeat. A newly enabled node and a temporarily disconnected node must be
// probed again, otherwise a stale or NULL heartbeat would make it permanently
// impossible for the control plane to mark that node healthy.
func heartbeatNodes(ctx context.Context) ([]node, error) {
	rows, err := instanceDB.QueryContext(ctx, `SELECT id,name,agent_url,cpu_total,memory_total_mb,enabled,last_heartbeat_at,COALESCE(agent_token_ciphertext,''),COALESCE(agent_version,''),COALESCE(agent_api_version,0),COALESCE(agent_capabilities,JSON_ARRAY()) FROM xcloud_nodes WHERE enabled=TRUE AND node_kind='platform'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []node{}
	for rows.Next() {
		var n node
		var capabilities []byte
		if err := rows.Scan(&n.ID, &n.Name, &n.AgentURL, &n.CPUTotal, &n.MemoryTotalMB, &n.Enabled, &n.LastHeartbeatAt, &n.AgentToken, &n.AgentVersion, &n.AgentAPIVersion, &capabilities); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(capabilities, &n.AgentCapabilities)
		items = append(items, n)
	}
	return items, rows.Err()
}
func syncNodeHeartbeat(ctx context.Context) {
	if instanceDB == nil {
		return
	}
	nodes, err := heartbeatNodes(ctx)
	if err != nil {
		log.Printf("load nodes: %v", err)
		return
	}
	for _, n := range nodes {
		var s struct {
			DockerVersion         string   `json:"dockerVersion"`
			AgentVersion          string   `json:"agentVersion"`
			APIVersion            int      `json:"apiVersion"`
			Capabilities          []string `json:"capabilities"`
			CPUTotal              float64  `json:"cpuTotal"`
			MemoryTotalMB         int      `json:"memoryTotalMB"`
			DiskAvailableBytes    int64    `json:"diskAvailableBytes"`
			DiskTotalBytes        int64    `json:"diskTotalBytes"`
			ManagedContainerCount int      `json:"managedContainerCount"`
		}
		probe, cancel := context.WithTimeout(ctx, 8*time.Second)
		err := nodeRequest(probe, n, "GET", "/container/status", nil, &s)
		cancel()
		if err != nil {
			log.Printf("node %s heartbeat: %v", n.ID, err)
			_, _ = instanceDB.ExecContext(ctx, `UPDATE xcloud_nodes SET last_agent_error=?,updated_at=NOW() WHERE id=?`, truncateError(err.Error()), n.ID)
			continue
		}
		capabilities, _ := json.Marshal(s.Capabilities)
		query := `UPDATE xcloud_nodes SET last_heartbeat_at=NOW(),last_agent_error=NULL,docker_version=?,agent_version=?,agent_api_version=?,agent_capabilities=?,disk_available_bytes=?,disk_total_bytes=?,managed_container_count=?,updated_at=NOW() WHERE id=?`
		args := []any{s.DockerVersion, s.AgentVersion, s.APIVersion, string(capabilities), s.DiskAvailableBytes, s.DiskTotalBytes, s.ManagedContainerCount, n.ID}
		if s.CPUTotal > 0 && s.MemoryTotalMB >= 256 {
			query = `UPDATE xcloud_nodes SET last_heartbeat_at=NOW(),last_agent_error=NULL,cpu_detected=?,memory_detected_mb=?,docker_version=?,agent_version=?,agent_api_version=?,agent_capabilities=?,disk_available_bytes=?,disk_total_bytes=?,managed_container_count=?,updated_at=NOW() WHERE id=?`
			args = []any{s.CPUTotal, s.MemoryTotalMB, s.DockerVersion, s.AgentVersion, s.APIVersion, string(capabilities), s.DiskAvailableBytes, s.DiskTotalBytes, s.ManagedContainerCount, n.ID}
		}
		if _, err := instanceDB.ExecContext(ctx, query, args...); err != nil {
			log.Printf("save node %s heartbeat: %v", n.ID, err)
		}
	}
}
func removeAgentCapability(values []string, excluded string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != excluded {
			out = append(out, value)
		}
	}
	return out
}
func syncInstanceStates(ctx context.Context) {
	if instanceDB == nil {
		return
	}
	rows, err := instanceDB.QueryContext(ctx, `SELECT i.id,i.container_name,i.status,COALESCE(i.runtime_status,''),n.id,n.name,n.agent_url,n.cpu_total,n.memory_total_mb,n.enabled,n.last_heartbeat_at,COALESCE(n.agent_token_ciphertext,'') FROM xcloud_instances i JOIN xcloud_nodes n ON n.id=i.node_id WHERE n.node_kind='platform' AND i.status IN ('deploying','running','stopped','destroy_scheduled') AND COALESCE(i.runtime_status,'')<>'updating'`)
	if err != nil {
		log.Printf("load instance state: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, name, stored, runtimeStatus string
		var n node
		if err := rows.Scan(&id, &name, &stored, &runtimeStatus, &n.ID, &n.Name, &n.AgentURL, &n.CPUTotal, &n.MemoryTotalMB, &n.Enabled, &n.LastHeartbeatAt, &n.AgentToken); err != nil {
			continue
		}
		var body struct {
			Status string `json:"status"`
		}
		probe, cancel := context.WithTimeout(ctx, 8*time.Second)
		err := nodeRequest(probe, n, "GET", "/container/"+name+"/status", nil, &body)
		cancel()
		if err != nil {
			if strings.Contains(err.Error(), "返回 404") {
				// A missing container is a runtime fault, not permission to alter the
				// paid lifecycle. Keep data and the destruction plan intact.
				if result, updateErr := instanceDB.ExecContext(ctx, `UPDATE xcloud_instances SET runtime_status='missing' WHERE id=? AND status=? AND COALESCE(runtime_status,'')<>'missing'`, id, stored); updateErr != nil {
					log.Printf("mark missing instance %s: %v", id, updateErr)
				} else if affected, _ := result.RowsAffected(); affected == 1 {
					_ = writeAudit(ctx, "system", "instance.runtime_missing", "instance", id, map[string]any{"nodeId": n.ID})
				}
			}
			continue
		}
		next := stored
		nextRuntime := runtimeStatus
		if body.Status == "running" {
			nextRuntime = "running"
			if stored != "destroy_scheduled" {
				next = "running"
			}
		} else if body.Status == "exited" || body.Status == "created" {
			nextRuntime = "stopped"
			if stored != "destroy_scheduled" {
				next = "stopped"
			}
		}
		if next != stored || nextRuntime != runtimeStatus {
			if _, err := transitionInstance(ctx, instanceDB, id, []string{stored}, next, &nextRuntime, ""); err != nil {
				log.Printf("sync instance %s: %v", id, err)
			}
		}
	}
}
