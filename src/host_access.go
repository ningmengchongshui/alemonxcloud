package cloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const hostAccessCapability = "container.host-access.v1"

type hostAccessRequestError struct {
	status  int
	message string
}

func (e *hostAccessRequestError) Error() string { return e.message }

func respondHostAccessError(c *gin.Context, err error) {
	var requestError *hostAccessRequestError
	if errors.As(err, &requestError) {
		c.JSON(requestError.status, gin.H{"message": requestError.message})
	} else {
		internalError(c, err)
	}
}

func lockHostAccessUser(ctx context.Context, tx *sql.Tx, ownerID string) (bool, error) {
	var allowed bool
	err := tx.QueryRowContext(ctx, `SELECT host_access_allowed FROM xcloud_users WHERE id=? FOR UPDATE`, ownerID).Scan(&allowed)
	if errors.Is(err, sql.ErrNoRows) {
		return false, &hostAccessRequestError{http.StatusNotFound, "用户不存在"}
	}
	return allowed, err
}

// The user lock serializes enable requests against whitelist removal. Instance
// intent and the task are committed together, so a queue outage cannot lose it.
func setInstanceHostAccessTx(ctx context.Context, tx *sql.Tx, id, ownerID string, enabled bool, version *int64) (*controlTask, error) {
	var lifecycle, nodeID, power, recreate, deletion string
	var current bool
	var resourceVersion int64
	err := tx.QueryRowContext(ctx, `SELECT status,COALESCE(node_id,''),host_access_enabled,resource_version,COALESCE(desired_power_state,''),COALESCE(desired_recreate_nonce,''),COALESCE(deletion_intent,'') FROM xcloud_instances WHERE id=? AND owner_id=? AND archived_at IS NULL FOR UPDATE`, id, ownerID).Scan(&lifecycle, &nodeID, &current, &resourceVersion, &power, &recreate, &deletion)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &hostAccessRequestError{http.StatusNotFound, "实例不存在"}
	}
	if err != nil {
		return nil, err
	}
	if version != nil && *version != resourceVersion {
		return nil, &hostAccessRequestError{http.StatusConflict, "实例配置已更新，请刷新后重试"}
	}
	if current == enabled {
		return nil, nil
	}
	live := lifecycle == "running" || lifecycle == "stopped" || lifecycle == "destroy_scheduled"
	if enabled && lifecycle != "running" && lifecycle != "stopped" {
		return nil, &hostAccessRequestError{http.StatusConflict, "只有运行中或已关机的实例可以开启宿主机访问"}
	}
	var active int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM xcloud_tasks WHERE instance_id=? AND status IN ('pending','running') AND action IN ('create','retry-deploy','start','stop','update','restart','reinstall','destroy','purge','resize','compensate-resize','host-access') AND NOT (status='pending' AND action IN ('destroy','purge') AND run_after>NOW())`, id).Scan(&active); err != nil {
		return nil, err
	}
	if active > 0 {
		return nil, &hostAccessRequestError{http.StatusConflict, "实例正在处理中，请等待任务完成后再修改宿主机访问权限"}
	}
	if live {
		var raw sql.NullString
		if err = tx.QueryRowContext(ctx, `SELECT agent_capabilities FROM xcloud_nodes WHERE id=?`, nodeID).Scan(&raw); err != nil {
			return nil, err
		}
		capabilities := node{}
		if err = json.Unmarshal([]byte(raw.String), &capabilities.AgentCapabilities); err != nil {
			return nil, err
		}
		if !capabilities.supportsAgentCapability(hostAccessCapability) {
			return nil, &hostAccessRequestError{http.StatusConflict, "节点尚未支持宿主机访问开关，请先升级节点 Agent"}
		}
	}
	specHash := desiredSpecHash(power, recreate, deletion, enabled)
	if _, err = tx.ExecContext(ctx, `UPDATE xcloud_instances SET host_access_enabled=?,resource_version=resource_version+1,desired_generation=desired_generation+1,spec_hash=?,spec_updated_at=NOW(),reconcile_requested_at=NOW() WHERE id=?`, enabled, specHash, id); err != nil {
		return nil, err
	}
	if !live {
		return nil, nil
	}
	var generation int64
	if err = tx.QueryRowContext(ctx, `SELECT desired_generation FROM xcloud_instances WHERE id=?`, id).Scan(&generation); err != nil {
		return nil, err
	}
	now := time.Now()
	t := controlTask{ID: newID("task"), InstanceID: id, Action: "host-access", Status: taskPending, RunAfter: now, CreatedAt: now, UpdatedAt: now, DesiredGeneration: generation}
	t.IdempotencyKey = "host-access:" + t.ID
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_tasks (id,instance_id,action,idempotency_key,status,attempts,run_after,created_at,updated_at,desired_generation) VALUES (?,?,?,?,?,0,?,?,?,?)`, t.ID, id, t.Action, t.IdempotencyKey, t.Status, now, now, now, generation); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_instance_operations (id,instance_id,generation,operation_kind,status,idempotency_key,spec_hash,task_id,requested_at,next_retry_at) VALUES (?,?,?,?,?,?,?,?,?,?)`, "op-"+t.ID, id, generation, t.Action, "pending", t.IdempotencyKey, specHash, t.ID, now, now); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_instance_conditions (instance_id,condition_type,condition_status,reason,message,operation_id,observed_generation,updated_at) VALUES (?,'Reconciling','True','HostAccessChanged','宿主机访问配置已保存，等待节点应用',?,?,NOW()) ON DUPLICATE KEY UPDATE condition_status=VALUES(condition_status),reason=VALUES(reason),message=VALUES(message),operation_id=VALUES(operation_id),observed_generation=VALUES(observed_generation),updated_at=NOW()`, id, t.ID, generation); err != nil {
		return nil, err
	}
	return &t, nil
}

func patchInstanceHostAccess(c *gin.Context) {
	if instanceDB == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "平台数据服务尚未就绪，请配置 MySQL 后重试"})
		return
	}
	var body struct {
		Enabled         *bool `json:"enabled"`
		ResourceVersion int64 `json:"resourceVersion"`
	}
	if c.ShouldBindJSON(&body) != nil || body.Enabled == nil || body.ResourceVersion < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "请提供开关状态和当前实例版本"})
		return
	}
	u := c.MustGet("user").(oidcUser)
	ctx := c.Request.Context()
	tx, err := beginSerializableTx(ctx)
	if err != nil {
		internalError(c, err)
		return
	}
	defer tx.Rollback()
	allowed, err := lockHostAccessUser(ctx, tx, u.ID)
	if err != nil {
		respondHostAccessError(c, err)
		return
	}
	if *body.Enabled && !allowed {
		c.JSON(http.StatusForbidden, gin.H{"message": "宿主机访问仅限后台白名单用户，请联系管理员"})
		return
	}
	task, err := setInstanceHostAccessTx(ctx, tx, c.Param("id"), u.ID, *body.Enabled, &body.ResourceVersion)
	if err != nil {
		respondHostAccessError(c, err)
		return
	}
	if err = tx.Commit(); err != nil {
		internalError(c, err)
		return
	}
	if task != nil {
		_ = enqueuePersistedTask(ctx, *task)
	}
	_ = writeAudit(ctx, u.ID, "instance.host_access", "instance", c.Param("id"), map[string]any{"enabled": *body.Enabled, "taskId": nullableTaskID(task)})
	c.JSON(http.StatusAccepted, gin.H{"task": task, "message": "配置已保存，等待任务生效"})
}

func adminUserHostAccess(c *gin.Context) {
	var body struct {
		Allowed *bool `json:"allowed"`
	}
	if c.ShouldBindJSON(&body) != nil || body.Allowed == nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "请提供白名单状态"})
		return
	}
	ctx := c.Request.Context()
	tx, err := beginSerializableTx(ctx)
	if err != nil {
		internalError(c, err)
		return
	}
	defer tx.Rollback()
	ownerID := c.Param("id")
	if _, err = lockHostAccessUser(ctx, tx, ownerID); err != nil {
		respondHostAccessError(c, err)
		return
	}
	var tasks []controlTask
	if !*body.Allowed {
		rows, queryErr := tx.QueryContext(ctx, `SELECT id FROM xcloud_instances WHERE owner_id=? AND host_access_enabled=TRUE AND archived_at IS NULL ORDER BY id FOR UPDATE`, ownerID)
		if queryErr != nil {
			internalError(c, queryErr)
			return
		}
		var ids []string
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				break
			}
			ids = append(ids, id)
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			internalError(c, err)
			return
		}
		for _, id := range ids {
			task, changeErr := setInstanceHostAccessTx(ctx, tx, id, ownerID, false, nil)
			if changeErr != nil {
				respondHostAccessError(c, changeErr)
				return
			}
			if task != nil {
				tasks = append(tasks, *task)
			}
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE xcloud_users SET host_access_allowed=? WHERE id=?`, *body.Allowed, ownerID); err != nil {
		internalError(c, err)
		return
	}
	if err = tx.Commit(); err != nil {
		internalError(c, err)
		return
	}
	for _, task := range tasks {
		_ = enqueuePersistedTask(ctx, task)
	}
	u := c.MustGet("user").(oidcUser)
	_ = writeAudit(ctx, u.ID, "user.host_access_allowlist", "user", ownerID, map[string]any{"allowed": *body.Allowed, "tasks": len(tasks)})
	c.JSON(http.StatusOK, gin.H{"allowed": *body.Allowed, "tasks": tasks})
}

func composeTaskAction(action string) bool {
	switch action {
	case "create", "retry-deploy", "start", "restart", "reinstall", "update", "resize", "compensate-resize", "host-access":
		return true
	default:
		return false
	}
}
