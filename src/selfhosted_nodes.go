package cloud

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const selfHostedNodeKind = "selfhosted"

type selfHostedReadinessIssue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type selfHostedNode struct {
	ID              string                     `json:"id"`
	Name            string                     `json:"name"`
	DeviceID        string                     `json:"deviceId"`
	Status          string                     `json:"status"`
	Ready           bool                       `json:"ready"`
	ReadinessIssues []selfHostedReadinessIssue `json:"readinessIssues,omitempty"`
	LastAgentError  string                     `json:"lastAgentError,omitempty"`
	LastHeartbeat   *time.Time                 `json:"lastHeartbeatAt,omitempty"`
	CPUDetected     float64                    `json:"cpuDetected"`
	MemoryDetected  int                        `json:"memoryDetectedMB"`
	DiskAvailable   int64                      `json:"diskAvailableBytes"`
	DiskTotal       int64                      `json:"diskTotalBytes"`
	CPUQuota        float64                    `json:"cpuQuota"`
	MemoryQuota     int                        `json:"memoryQuotaMB"`
	CPUUsed         float64                    `json:"cpuUsed"`
	MemoryUsed      int                        `json:"memoryUsedMB"`
	AgentVersion    string                     `json:"agentVersion,omitempty"`
	Capabilities    []string                   `json:"capabilities,omitempty"`
}

func selfHostedNodes(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	rows, err := instanceDB.QueryContext(c.Request.Context(), `SELECT n.id,n.name,n.control_device_id,n.enabled,n.last_heartbeat_at,COALESCE(n.selfhosted_ready,FALSE),COALESCE(n.selfhosted_readiness,JSON_ARRAY()),COALESCE(n.last_agent_error,''),n.cpu_detected,n.memory_detected_mb,COALESCE(n.disk_available_bytes,0),COALESCE(n.disk_total_bytes,0),n.cpu_quota,n.memory_quota_mb,COALESCE(n.agent_version,''),COALESCE(n.agent_capabilities,JSON_ARRAY()),COALESCE(SUM(CASE WHEN i.status IN ('deploying','running','stopped','destroy_scheduled') THEN i.cpu ELSE 0 END),0),COALESCE(SUM(CASE WHEN i.status IN ('deploying','running','stopped','destroy_scheduled') THEN i.memory_mb ELSE 0 END),0) FROM xcloud_nodes n LEFT JOIN xcloud_instances i ON i.node_id=n.id AND i.placement_type='selfhosted' WHERE n.node_kind=? AND n.owner_id=? AND n.enabled=TRUE GROUP BY n.id ORDER BY n.created_at`, selfHostedNodeKind, user.ID)
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()
	items := []selfHostedNode{}
	for rows.Next() {
		var item selfHostedNode
		var enabled bool
		var raw, readinessRaw []byte
		if err := rows.Scan(&item.ID, &item.Name, &item.DeviceID, &enabled, &item.LastHeartbeat, &item.Ready, &readinessRaw, &item.LastAgentError, &item.CPUDetected, &item.MemoryDetected, &item.DiskAvailable, &item.DiskTotal, &item.CPUQuota, &item.MemoryQuota, &item.AgentVersion, &raw, &item.CPUUsed, &item.MemoryUsed); err != nil {
			internalError(c, err)
			return
		}
		_ = json.Unmarshal(raw, &item.Capabilities)
		_ = json.Unmarshal(readinessRaw, &item.ReadinessIssues)
		item.Status = "offline"
		if enabled && item.LastHeartbeat != nil && time.Since(*item.LastHeartbeat) <= nodeHeartbeatTTL() {
			item.Status = "online"
		}
		items = append(items, item)
	}
	c.JSON(http.StatusOK, items)
}

func ownedSelfHostedNode(c *gin.Context) (selfHostedNode, bool) {
	user := c.MustGet("user").(oidcUser)
	id := c.Param("nodeID")
	var item selfHostedNode
	var enabled bool
	var raw, readinessRaw []byte
	err := instanceDB.QueryRowContext(c.Request.Context(), `SELECT id,name,control_device_id,enabled,last_heartbeat_at,COALESCE(selfhosted_ready,FALSE),COALESCE(selfhosted_readiness,JSON_ARRAY()),COALESCE(last_agent_error,''),cpu_detected,memory_detected_mb,COALESCE(disk_available_bytes,0),COALESCE(disk_total_bytes,0),cpu_quota,memory_quota_mb,COALESCE(agent_version,''),COALESCE(agent_capabilities,JSON_ARRAY()),COALESCE((SELECT SUM(cpu) FROM xcloud_instances WHERE node_id=xcloud_nodes.id AND placement_type='selfhosted' AND status IN ('deploying','running','stopped','destroy_scheduled')),0),COALESCE((SELECT SUM(memory_mb) FROM xcloud_instances WHERE node_id=xcloud_nodes.id AND placement_type='selfhosted' AND status IN ('deploying','running','stopped','destroy_scheduled')),0) FROM xcloud_nodes WHERE id=? AND node_kind=? AND owner_id=? AND enabled=TRUE`, id, selfHostedNodeKind, user.ID).Scan(&item.ID, &item.Name, &item.DeviceID, &enabled, &item.LastHeartbeat, &item.Ready, &readinessRaw, &item.LastAgentError, &item.CPUDetected, &item.MemoryDetected, &item.DiskAvailable, &item.DiskTotal, &item.CPUQuota, &item.MemoryQuota, &item.AgentVersion, &raw, &item.CPUUsed, &item.MemoryUsed)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(404, gin.H{"message": "自建节点不存在"})
		return item, false
	}
	if err != nil {
		internalError(c, err)
		return item, false
	}
	_ = json.Unmarshal(raw, &item.Capabilities)
	_ = json.Unmarshal(readinessRaw, &item.ReadinessIssues)
	item.Status = "offline"
	if enabled && item.LastHeartbeat != nil && time.Since(*item.LastHeartbeat) <= nodeHeartbeatTTL() {
		item.Status = "online"
	}
	return item, true
}

func selfHostedNodeDetail(c *gin.Context) {
	if item, ok := ownedSelfHostedNode(c); ok {
		c.JSON(200, item)
	}
}

func updateSelfHostedNode(c *gin.Context) {
	item, ok := ownedSelfHostedNode(c)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	name := ""
	if c.ShouldBindJSON(&body) == nil {
		name = strings.TrimSpace(body.Name)
	}
	if name == "" || len(name) > 96 {
		c.JSON(400, gin.H{"message": "节点名称应为 1 至 96 个字符"})
		return
	}
	if _, err := instanceDB.ExecContext(c.Request.Context(), `UPDATE xcloud_nodes SET name=?,updated_at=NOW() WHERE id=? AND owner_id=? AND node_kind='selfhosted' AND enabled=TRUE`, name, item.ID, c.MustGet("user").(oidcUser).ID); err != nil {
		internalError(c, err)
		return
	}
	_ = writeAudit(c.Request.Context(), c.MustGet("user").(oidcUser).ID, "selfhosted_node.rename", "node", item.ID, map[string]any{"name": name})
	c.Status(http.StatusNoContent)
}

func selfHostedNodeReadinessEvents(c *gin.Context) {
	item, ok := ownedSelfHostedNode(c)
	if !ok {
		return
	}
	rows, err := instanceDB.QueryContext(c.Request.Context(), `SELECT ready,issue_code,message,created_at FROM xcloud_selfhosted_readiness_events WHERE node_id=? AND created_at>DATE_SUB(NOW(),INTERVAL 30 DAY) ORDER BY id DESC LIMIT 100`, item.ID)
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()
	items := []gin.H{}
	for rows.Next() {
		var ready bool
		var code, message string
		var createdAt time.Time
		if err := rows.Scan(&ready, &code, &message, &createdAt); err != nil {
			internalError(c, err)
			return
		}
		items = append(items, gin.H{"ready": ready, "code": code, "message": message, "createdAt": createdAt})
	}
	if err := rows.Err(); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

func updateSelfHostedNodeQuota(c *gin.Context) {
	item, ok := ownedSelfHostedNode(c)
	if !ok {
		return
	}
	var body struct {
		CPUQuota    float64 `json:"cpuQuota"`
		MemoryQuota int     `json:"memoryQuotaMB"`
	}
	if c.ShouldBindJSON(&body) != nil || body.CPUQuota <= 0 || body.MemoryQuota <= 0 {
		c.JSON(400, gin.H{"message": "资源配额无效"})
		return
	}
	if item.CPUDetected <= 0 || item.MemoryDetected <= 0 {
		c.JSON(409, gin.H{"message": "节点尚未上报资源，请等待 Agent 心跳"})
		return
	}
	if body.CPUQuota > item.CPUDetected || body.MemoryQuota > item.MemoryDetected {
		c.JSON(400, gin.H{"message": "配额不能超过节点实际资源"})
		return
	}
	if body.CPUQuota < item.CPUUsed || body.MemoryQuota < item.MemoryUsed {
		c.JSON(409, gin.H{"message": "配额不能低于已分配资源"})
		return
	}
	_, err := instanceDB.ExecContext(c.Request.Context(), `UPDATE xcloud_nodes SET cpu_quota=?,memory_quota_mb=?,selfhosted_quota_mode='manual',updated_at=NOW() WHERE id=?`, body.CPUQuota, body.MemoryQuota, item.ID)
	if err != nil {
		internalError(c, err)
		return
	}
	user := c.MustGet("user").(oidcUser)
	_ = writeAudit(c.Request.Context(), user.ID, "selfhosted_node.quota.update", "node", item.ID, map[string]any{"cpu": body.CPUQuota, "memoryMB": body.MemoryQuota})
	c.Status(204)
}

func selfHostedNodeInstances(c *gin.Context) {
	item, ok := ownedSelfHostedNode(c)
	if !ok {
		return
	}
	user := c.MustGet("user").(oidcUser)
	rows, err := instanceDB.QueryContext(c.Request.Context(), `SELECT id FROM xcloud_instances WHERE node_id=? AND owner_id=? AND placement_type='selfhosted' AND archived_at IS NULL ORDER BY created_at DESC`, item.ID, user.ID)
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()
	items := []instance{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			continue
		}
		value, found, lookupErr := getStoredInstance(c.Request.Context(), id, user.ID)
		if lookupErr != nil {
			internalError(c, lookupErr)
			return
		}
		if found {
			items = append(items, value)
		}
	}
	c.JSON(200, items)
}

func createSelfHostedInstance(c *gin.Context) {
	n, ok := ownedSelfHostedNode(c)
	if !ok {
		return
	}
	if n.Status != "online" {
		c.JSON(409, gin.H{"message": "自建节点离线，请启动 xcloud-control 并等待心跳"})
		return
	}
	if !n.Ready {
		message := "自建节点运行环境未就绪"
		if len(n.ReadinessIssues) > 0 {
			message += "：" + n.ReadinessIssues[0].Message
		}
		c.JSON(409, gin.H{"message": message})
		return
	}
	var body struct {
		Name         string  `json:"name"`
		ImageID      string  `json:"imageId"`
		ImageVersion string  `json:"imageVersion"`
		CPU          float64 `json:"cpu"`
		MemoryMB     int     `json:"memoryMB"`
	}
	if c.ShouldBindJSON(&body) != nil || strings.TrimSpace(body.Name) == "" || body.CPU <= 0 || body.MemoryMB <= 0 || !validImageTag(body.ImageVersion) {
		c.JSON(400, gin.H{"message": "实例参数无效"})
		return
	}
	user := c.MustGet("user").(oidcUser)
	tx, err := instanceDB.BeginTx(c.Request.Context(), nil)
	if err != nil {
		internalError(c, err)
		return
	}
	defer tx.Rollback()
	var imageRef, digest string
	var terminal bool
	err = tx.QueryRowContext(c.Request.Context(), `SELECT i.image_ref,COALESCE(v.image_digest,''),COALESCE(i.terminal_only,TRUE) FROM xcloud_images i JOIN xcloud_image_versions v ON v.image_id=i.id WHERE i.id=? AND i.enabled=TRUE AND v.version_tag=? AND v.enabled=TRUE AND v.version_status='ready' FOR UPDATE`, body.ImageID, body.ImageVersion).Scan(&imageRef, &digest, &terminal)
	if err != nil {
		c.JSON(409, gin.H{"message": "镜像来源不可用"})
		return
	}
	var usedCPU float64
	var usedMem int
	var quotaCPU float64
	var quotaMem int
	err = tx.QueryRowContext(c.Request.Context(), `SELECT cpu_quota,memory_quota_mb,COALESCE((SELECT SUM(cpu) FROM xcloud_instances WHERE node_id=? AND placement_type='selfhosted' AND status IN ('deploying','running','stopped','destroy_scheduled')),0),COALESCE((SELECT SUM(memory_mb) FROM xcloud_instances WHERE node_id=? AND placement_type='selfhosted' AND status IN ('deploying','running','stopped','destroy_scheduled')),0) FROM xcloud_nodes WHERE id=? FOR UPDATE`, n.ID, n.ID, n.ID).Scan(&quotaCPU, &quotaMem, &usedCPU, &usedMem)
	if err != nil || usedCPU+body.CPU > quotaCPU || usedMem+body.MemoryMB > quotaMem {
		c.JSON(409, gin.H{"message": "自建节点资源不足"})
		return
	}
	id := newID("shi")
	route := routeKey(user.ID + "\x00" + id)
	name := strings.TrimSpace(body.Name)
	container := "xcloud-" + route
	address := "https://control-" + route + "." + env("XCLOUD_INSTANCE_DOMAIN", "alemonjs.com")
	_, err = tx.ExecContext(c.Request.Context(), `INSERT INTO xcloud_instances (id,owner_id,name,image,image_digest,version,spec,status,access_address,container_name,created_at,cpu,memory_mb,node_id,route_key,placement_type,runtime_status) VALUES (?,?,?,?,?,?,?,'deploying',?,?,NOW(),?,?,?,?,?,'running')`, id, user.ID, name, imageRef, nullableString(digest), body.ImageVersion, fmt.Sprintf("%g 核 / %d GB", body.CPU, body.MemoryMB/1024), address, container, body.CPU, body.MemoryMB, n.ID, route, "selfhosted")
	if err != nil {
		internalError(c, err)
		return
	}
	payload, _ := json.Marshal(map[string]any{"selfhosted": true})
	now := time.Now()
	task := controlTask{ID: newID("task"), InstanceID: id, Action: "create", IdempotencyKey: "selfhosted:create:" + id, Status: taskPending, RunAfter: now, CreatedAt: now, UpdatedAt: now, Payload: payload}
	if _, err = tx.ExecContext(c.Request.Context(), `INSERT INTO xcloud_tasks (id,instance_id,action,idempotency_key,status,attempts,run_after,created_at,updated_at,payload) VALUES (?,?,?,?,?,?,?,?,?,?)`, task.ID, task.InstanceID, task.Action, task.IdempotencyKey, task.Status, 0, task.RunAfter, now, now, task.Payload); err != nil {
		internalError(c, err)
		return
	}
	if err = writeAuditTx(c.Request.Context(), tx, user.ID, "selfhosted_instance.create", "instance", id, map[string]any{"nodeId": n.ID, "imageId": body.ImageID, "version": body.ImageVersion}); err != nil {
		internalError(c, err)
		return
	}
	if err = tx.Commit(); err != nil {
		internalError(c, err)
		return
	}
	appendTaskEvent(c.Request.Context(), task.ID, "queued", "自建节点实例等待部署")
	if err = enqueuePersistedTask(c.Request.Context(), task); err != nil {
		c.JSON(202, gin.H{"instanceId": id, "task": task, "message": "实例已创建，等待任务队列恢复"})
		return
	}
	c.JSON(202, gin.H{"instanceId": id, "task": task})
}
