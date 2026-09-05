package cloud

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"
)

// Nodes are polled independently, so an unavailable Agent never becomes
// schedulable simply because another node remains healthy.
func startControlLoops() {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			scheduleLifecycle(context.Background())
			recoverExpiredTaskLeases(context.Background())
			recoverPendingTasks()
			syncNodeHeartbeat(context.Background())
			syncInstanceStates(context.Background())
		}
	}()
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			reconcileBandwidthTasks(context.Background())
		}
	}()
	syncNodeHeartbeat(context.Background())
	syncInstanceStates(context.Background())
	recoverExpiredTaskLeases(context.Background())
	reconcileBandwidthTasks(context.Background())
}

// reconcileBandwidthTasks is intentionally limited to running instances on a
// healthy node. A stopped container has no reliable veth to shape; its start
// action reapplies bandwidth instead of generating a false failure task.
func reconcileBandwidthTasks(ctx context.Context) {
	if instanceDB == nil {
		return
	}
	rows, err := instanceDB.QueryContext(ctx, `SELECT i.id,COALESCE(n.agent_capabilities,JSON_ARRAY())
		FROM xcloud_instances i
		JOIN xcloud_nodes n ON n.id=i.node_id
		WHERE i.status IN ('running','destroy_scheduled')
		  AND COALESCE(i.runtime_status,i.status)='running'
		  AND COALESCE(i.bandwidth_status,'pending') IN ('pending','failed')
		  AND n.enabled=TRUE AND n.last_heartbeat_at>=?`, time.Now().Add(-nodeHeartbeatTTL()))
	if err != nil {
		log.Printf("load bandwidth reconciliation: %v", err)
		return
	}
	defer rows.Close()
	instanceIDs := make([]string, 0)
	for rows.Next() {
		var instanceID string
		var rawCapabilities []byte
		if err := rows.Scan(&instanceID, &rawCapabilities); err != nil {
			continue
		}
		var capabilities []string
		_ = json.Unmarshal(rawCapabilities, &capabilities)
		statusSupported := false
		queueSupported := false
		for _, capability := range capabilities {
			if capability == "network.bandwidth.status.v1" {
				statusSupported = true
			}
			if capability == "network.bandwidth.queue.v1" {
				queueSupported = true
			}
		}
		if statusSupported && queueSupported {
			instanceIDs = append(instanceIDs, instanceID)
		}
	}
	if err := rows.Close(); err != nil {
		log.Printf("close bandwidth reconciliation rows: %v", err)
		return
	}
	for _, instanceID := range instanceIDs {
		task, scheduled, err := scheduleBandwidthTask(ctx, instanceID, "system")
		if err != nil {
			log.Printf("schedule bandwidth reconciliation for %s: %v", instanceID, err)
			continue
		}
		if scheduled {
			if err := enqueuePersistedTask(ctx, task); err != nil {
				log.Printf("enqueue bandwidth reconciliation for %s: %v", instanceID, err)
			}
		}
	}
}
func enabledNodes(ctx context.Context) ([]node, error) {
	rows, err := instanceDB.QueryContext(ctx, `SELECT id,name,agent_url,cpu_total,memory_total_mb,enabled,last_heartbeat_at,COALESCE(agent_token_ciphertext,''),COALESCE(agent_version,''),COALESCE(agent_api_version,0),COALESCE(agent_capabilities,JSON_ARRAY()) FROM xcloud_nodes WHERE enabled=TRUE AND last_heartbeat_at>=?`, time.Now().Add(-nodeHeartbeatTTL()))
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
	rows, err := instanceDB.QueryContext(ctx, `SELECT id,name,agent_url,cpu_total,memory_total_mb,enabled,last_heartbeat_at,COALESCE(agent_token_ciphertext,''),COALESCE(agent_version,''),COALESCE(agent_api_version,0),COALESCE(agent_capabilities,JSON_ARRAY()) FROM xcloud_nodes WHERE enabled=TRUE`)
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
			ManagedContainerCount int      `json:"managedContainerCount"`
			BandwidthToolsReady   bool     `json:"bandwidthToolsReady"`
		}
		probe, cancel := context.WithTimeout(ctx, 8*time.Second)
		err := nodeRequest(probe, n, "GET", "/container/status", nil, &s)
		cancel()
		if err != nil {
			log.Printf("node %s heartbeat: %v", n.ID, err)
			_, _ = instanceDB.ExecContext(ctx, `UPDATE xcloud_nodes SET last_agent_error=?,updated_at=NOW() WHERE id=?`, truncateError(err.Error()), n.ID)
			continue
		}
		if !s.BandwidthToolsReady {
			s.Capabilities = removeAgentCapability(s.Capabilities, "network.bandwidth.v1")
		}
		capabilities, _ := json.Marshal(s.Capabilities)
		query := `UPDATE xcloud_nodes SET last_heartbeat_at=NOW(),last_agent_error=NULL,docker_version=?,agent_version=?,agent_api_version=?,agent_capabilities=?,disk_available_bytes=?,managed_container_count=?,updated_at=NOW() WHERE id=?`
		args := []any{s.DockerVersion, s.AgentVersion, s.APIVersion, string(capabilities), s.DiskAvailableBytes, s.ManagedContainerCount, n.ID}
		if s.CPUTotal > 0 && s.MemoryTotalMB >= 256 {
			query = `UPDATE xcloud_nodes SET last_heartbeat_at=NOW(),last_agent_error=NULL,cpu_detected=?,memory_detected_mb=?,docker_version=?,agent_version=?,agent_api_version=?,agent_capabilities=?,disk_available_bytes=?,managed_container_count=?,updated_at=NOW() WHERE id=?`
			args = []any{s.CPUTotal, s.MemoryTotalMB, s.DockerVersion, s.AgentVersion, s.APIVersion, string(capabilities), s.DiskAvailableBytes, s.ManagedContainerCount, n.ID}
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
	rows, err := instanceDB.QueryContext(ctx, `SELECT i.id,i.container_name,i.status,COALESCE(i.runtime_status,''),n.id,n.name,n.agent_url,n.cpu_total,n.memory_total_mb,n.enabled,n.last_heartbeat_at,COALESCE(n.agent_token_ciphertext,'') FROM xcloud_instances i JOIN xcloud_nodes n ON n.id=i.node_id WHERE i.status IN ('deploying','running','stopped','destroy_scheduled') AND COALESCE(i.runtime_status,'')<>'updating'`)
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
