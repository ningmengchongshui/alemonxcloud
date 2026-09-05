package cloud

import (
	"context"
	"encoding/json"
	"log"
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
			recoverPendingTasks()
			syncNodeHeartbeat(context.Background())
			syncInstanceStates(context.Background())
		}
	}()
	syncNodeHeartbeat(context.Background())
	syncInstanceStates(context.Background())
}
func enabledNodes(ctx context.Context) ([]node, error) {
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
	nodes, err := enabledNodes(ctx)
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
		}
		probe, cancel := context.WithTimeout(ctx, 8*time.Second)
		err := nodeRequest(probe, n, "GET", "/container/status", nil, &s)
		cancel()
		if err != nil {
			log.Printf("node %s heartbeat: %v", n.ID, err)
			continue
		}
		capabilities, _ := json.Marshal(s.Capabilities)
		query := `UPDATE xcloud_nodes SET last_heartbeat_at=NOW(),docker_version=?,agent_version=?,agent_api_version=?,agent_capabilities=?,disk_available_bytes=?,managed_container_count=?,updated_at=NOW() WHERE id=?`
		args := []any{s.DockerVersion, s.AgentVersion, s.APIVersion, string(capabilities), s.DiskAvailableBytes, s.ManagedContainerCount, n.ID}
		if s.CPUTotal > 0 && s.MemoryTotalMB >= 256 {
			query = `UPDATE xcloud_nodes SET last_heartbeat_at=NOW(),cpu_detected=?,memory_detected_mb=?,docker_version=?,agent_version=?,agent_api_version=?,agent_capabilities=?,disk_available_bytes=?,managed_container_count=?,updated_at=NOW() WHERE id=?`
			args = []any{s.CPUTotal, s.MemoryTotalMB, s.DockerVersion, s.AgentVersion, s.APIVersion, string(capabilities), s.DiskAvailableBytes, s.ManagedContainerCount, n.ID}
		}
		if _, err := instanceDB.ExecContext(ctx, query, args...); err != nil {
			log.Printf("save node %s heartbeat: %v", n.ID, err)
		}
	}
}
func syncInstanceStates(ctx context.Context) {
	if instanceDB == nil {
		return
	}
	rows, err := instanceDB.QueryContext(ctx, `SELECT i.id,i.container_name,i.status,n.id,n.name,n.agent_url,n.cpu_total,n.memory_total_mb,n.enabled,n.last_heartbeat_at,COALESCE(n.agent_token_ciphertext,'') FROM xcloud_instances i JOIN xcloud_nodes n ON n.id=i.node_id WHERE i.status IN ('deploying','running','stopped')`)
	if err != nil {
		log.Printf("load instance state: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, name, stored string
		var n node
		if err := rows.Scan(&id, &name, &stored, &n.ID, &n.Name, &n.AgentURL, &n.CPUTotal, &n.MemoryTotalMB, &n.Enabled, &n.LastHeartbeatAt, &n.AgentToken); err != nil {
			continue
		}
		var body struct {
			Status string `json:"status"`
		}
		probe, cancel := context.WithTimeout(ctx, 8*time.Second)
		err := nodeRequest(probe, n, "GET", "/container/"+name+"/status", nil, &body)
		cancel()
		if err != nil {
			continue
		}
		next := stored
		if body.Status == "running" {
			next = "running"
		} else if body.Status == "exited" || body.Status == "created" {
			next = "stopped"
		}
		if next != stored {
			if _, err := instanceDB.ExecContext(ctx, `UPDATE xcloud_instances SET status=? WHERE id=?`, next, id); err != nil {
				log.Printf("sync instance %s: %v", id, err)
			}
		}
	}
}
