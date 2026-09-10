package cloud

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

var instanceReconcileQueue = make(chan string, 1024)

func requestInstanceReconcile(instanceID string) {
	select {
	case instanceReconcileQueue <- instanceID:
	default:
	}
}

func startDeclarativeReconciler() {
	go func() {
		for id := range instanceReconcileQueue {
			reconcileInstance(context.Background(), id)
		}
	}()
}

// reconcileInstance is the controller-owned convergence point. Task records
// are execution transport only; their terminal state is projected back into
// the durable operation and conditions here.
func reconcileInstance(ctx context.Context, id string) {
	if instanceDB == nil || id == "" {
		return
	}
	_, _ = instanceDB.ExecContext(ctx, `UPDATE xcloud_instance_operations o JOIN xcloud_tasks t ON t.id=o.task_id SET o.status=CASE WHEN t.status='pending' THEN 'pending' WHEN t.status='running' THEN 'running' WHEN t.status='succeeded' THEN 'done' WHEN t.status='cancelled' THEN 'cancel_requested' ELSE 'error' END,o.started_at=CASE WHEN t.status='running' THEN COALESCE(o.started_at,NOW()) ELSE o.started_at END,o.finished_at=CASE WHEN t.status IN ('succeeded','failed','needs_review','discarded','cancelled') THEN COALESCE(o.finished_at,NOW()) ELSE o.finished_at END,o.next_retry_at=CASE WHEN t.status='pending' THEN t.run_after ELSE NULL END,o.error_message=NULLIF(t.last_error,'' ) WHERE o.instance_id=? AND o.status NOT IN ('done','cancel_requested')`, id)
	_, _ = instanceDB.ExecContext(ctx, `UPDATE xcloud_instances SET reconciled_at=NOW() WHERE id=?`, id)
}

// instanceResource is the public declarative envelope. Spec is user intent;
// Status is only controller/Agent fact and never authorizes billing changes.
type instanceResource struct {
	Metadata struct {
		ID              string `json:"id"`
		ResourceVersion int64  `json:"resourceVersion"`
	} `json:"metadata"`
	Spec struct {
		PowerState     string `json:"powerState,omitempty"`
		RecreateNonce  string `json:"recreateNonce,omitempty"`
		DeletionIntent string `json:"deletionIntent,omitempty"`
	} `json:"spec"`
	Status struct {
		Lifecycle          string     `json:"lifecycle"`
		RuntimeState       string     `json:"runtimeState,omitempty"`
		ObservedCPU        *float64   `json:"observedCpu,omitempty"`
		ObservedMemoryMB   *int       `json:"observedMemoryMB,omitempty"`
		ObservedAt         *time.Time `json:"observedAt,omitempty"`
		ObservedGeneration int64      `json:"observedGeneration"`
	} `json:"status"`
	Conditions []instanceCondition `json:"conditions"`
	Operations []controlTask       `json:"operations,omitempty"`
}

type instanceCondition struct {
	Type               string    `json:"type"`
	Status             string    `json:"status"`
	Reason             string    `json:"reason,omitempty"`
	Message            string    `json:"message,omitempty"`
	OperationID        string    `json:"operationId,omitempty"`
	ObservedGeneration int64     `json:"observedGeneration"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

func operationStatus(status string) string {
	switch status {
	case taskPending:
		return "pending"
	case taskRunning:
		return "running"
	case taskDone:
		return "done"
	case taskCanceled:
		return "cancel_requested"
	default:
		return "error"
	}
}

func desiredSpecHash(power, recreate, deletion string) string {
	s := sha256.Sum256([]byte(power + "\x00" + recreate + "\x00" + deletion))
	return fmt.Sprintf("%x", s[:])
}

func setDeclarativeCondition(ctx context.Context, task controlTask, typ, status, reason, message string) {
	if instanceDB == nil || task.InstanceID == "" {
		return
	}
	_, _ = instanceDB.ExecContext(ctx, `INSERT INTO xcloud_instance_conditions (instance_id,condition_type,condition_status,reason,message,operation_id,observed_generation,updated_at) VALUES (?,?,?,?,?,?,?,NOW()) ON DUPLICATE KEY UPDATE condition_status=VALUES(condition_status),reason=VALUES(reason),message=VALUES(message),operation_id=VALUES(operation_id),observed_generation=VALUES(observed_generation),updated_at=NOW()`, task.InstanceID, typ, status, reason, message, task.ID, task.DesiredGeneration)
}

func loadInstanceResource(ctx context.Context, ownerID, id string) (instanceResource, error) {
	var r instanceResource
	var cpu sql.NullFloat64
	var mem sql.NullInt64
	var observedAt sql.NullTime
	err := instanceDB.QueryRowContext(ctx, `SELECT id,resource_version,COALESCE(desired_power_state,''),COALESCE(desired_recreate_nonce,''),COALESCE(deletion_intent,''),status,COALESCE(runtime_status,''),observed_cpu,observed_memory_mb,observed_at,desired_generation FROM xcloud_instances WHERE id=? AND owner_id=? AND archived_at IS NULL`, id, ownerID).
		Scan(&r.Metadata.ID, &r.Metadata.ResourceVersion, &r.Spec.PowerState, &r.Spec.RecreateNonce, &r.Spec.DeletionIntent, &r.Status.Lifecycle, &r.Status.RuntimeState, &cpu, &mem, &observedAt, &r.Status.ObservedGeneration)
	if err != nil {
		return r, err
	}
	if cpu.Valid {
		r.Status.ObservedCPU = &cpu.Float64
	}
	if mem.Valid {
		v := int(mem.Int64)
		r.Status.ObservedMemoryMB = &v
	}
	if observedAt.Valid {
		r.Status.ObservedAt = &observedAt.Time
	}
	rows, err := instanceDB.QueryContext(ctx, `SELECT condition_type,condition_status,reason,message,COALESCE(operation_id,''),observed_generation,updated_at FROM xcloud_instance_conditions WHERE instance_id=? ORDER BY condition_type`, id)
	if err != nil {
		return r, err
	}
	defer rows.Close()
	for rows.Next() {
		var c instanceCondition
		if err = rows.Scan(&c.Type, &c.Status, &c.Reason, &c.Message, &c.OperationID, &c.ObservedGeneration, &c.UpdatedAt); err != nil {
			return r, err
		}
		r.Conditions = append(r.Conditions, c)
	}
	return r, rows.Err()
}

func getInstanceResourceHandler(c *gin.Context) {
	u := c.MustGet("user").(oidcUser)
	r, err := loadInstanceResource(c.Request.Context(), u.ID, c.Param("id"))
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"message": "实例不存在"})
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}
	rows, err := instanceDB.QueryContext(c.Request.Context(), `SELECT `+taskSelectFields+` FROM xcloud_tasks WHERE instance_id=? ORDER BY created_at DESC LIMIT 20`, r.Metadata.ID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var t controlTask
			if scanControlTask(rows, &t) == nil {
				t.OperationStatus = operationStatus(t.Status)
				r.Operations = append(r.Operations, t)
			}
		}
	}
	c.JSON(http.StatusOK, r)
}

func patchInstanceDesiredHandler(c *gin.Context) {
	u := c.MustGet("user").(oidcUser)
	var body struct {
		ResourceVersion int64   `json:"resourceVersion"`
		IdempotencyKey  string  `json:"idempotencyKey"`
		PowerState      *string `json:"powerState"`
		RecreateNonce   *string `json:"recreateNonce"`
		DeletionIntent  *string `json:"deletionIntent"`
	}
	if c.ShouldBindJSON(&body) != nil || body.ResourceVersion < 1 || strings.TrimSpace(body.IdempotencyKey) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "resourceVersion 与 idempotencyKey 为必填项"})
		return
	}
	if body.PowerState == nil && body.RecreateNonce == nil && body.DeletionIntent == nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "缺少期望字段"})
		return
	}
	id := c.Param("id")
	if active, err := activeLifecycleTask(c.Request.Context(), id); err != nil {
		internalError(c, err)
		return
	} else if active != nil {
		c.JSON(http.StatusConflict, gin.H{"message": "实例正在执行冲突操作", "operation": active})
		return
	}
	tx, err := beginSerializableTx(c.Request.Context())
	if err != nil {
		internalError(c, err)
		return
	}
	defer tx.Rollback()
	var lifecycle string
	if err = tx.QueryRowContext(c.Request.Context(), `SELECT status FROM xcloud_instances WHERE id=? AND owner_id=? FOR UPDATE`, id, u.ID).Scan(&lifecycle); errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"message": "实例不存在"})
		return
	} else if err != nil {
		internalError(c, err)
		return
	}
	var action string
	if body.DeletionIntent != nil && *body.DeletionIntent == "absent" {
		c.JSON(http.StatusConflict, gin.H{"message": "销毁声明请暂使用兼容 destroy 接口；该能力将在下一次兼容窗口结束前迁移"})
		return
	}
	if body.PowerState != nil {
		if *body.PowerState != "running" && *body.PowerState != "stopped" {
			c.JSON(http.StatusBadRequest, gin.H{"message": "powerState 必须为 running 或 stopped"})
			return
		}
		if *body.PowerState == "running" && lifecycle == "stopped" {
			action = "start"
		}
		if *body.PowerState == "stopped" && lifecycle == "running" {
			action = "stop"
		}
	}
	if body.RecreateNonce != nil {
		if lifecycle != "running" && lifecycle != "stopped" {
			c.JSON(http.StatusConflict, gin.H{"message": "当前实例不能重建"})
			return
		}
		action = "restart"
	}
	var oldPower, oldRecreate, oldDeletion string
	_ = tx.QueryRowContext(c.Request.Context(), `SELECT COALESCE(desired_power_state,''),COALESCE(desired_recreate_nonce,''),COALESCE(deletion_intent,'') FROM xcloud_instances WHERE id=?`, id).Scan(&oldPower, &oldRecreate, &oldDeletion)
	power, recreate, deletion := oldPower, oldRecreate, oldDeletion
	if body.PowerState != nil {
		power = *body.PowerState
	}
	if body.RecreateNonce != nil {
		recreate = *body.RecreateNonce
	}
	if body.DeletionIntent != nil {
		deletion = *body.DeletionIntent
	}
	specHash := desiredSpecHash(power, recreate, deletion)
	result, err := tx.ExecContext(c.Request.Context(), `UPDATE xcloud_instances SET desired_power_state=?,desired_recreate_nonce=?,deletion_intent=?,spec_hash=?,resource_version=resource_version+1,desired_generation=desired_generation+1,spec_updated_at=NOW(),reconcile_requested_at=NOW() WHERE id=? AND owner_id=? AND resource_version=?`, power, recreate, nullableString(deletion), specHash, id, u.ID, body.ResourceVersion)
	if err != nil {
		internalError(c, err)
		return
	}
	if n, _ := result.RowsAffected(); n != 1 {
		latest, _ := loadInstanceResource(c.Request.Context(), u.ID, id)
		c.JSON(http.StatusConflict, gin.H{"message": "实例声明已更新，请刷新后重试", "resource": latest})
		return
	}
	var generation, version int64
	_ = tx.QueryRowContext(c.Request.Context(), `SELECT desired_generation,resource_version FROM xcloud_instances WHERE id=?`, id).Scan(&generation, &version)
	var task *controlTask
	if action != "" {
		now := time.Now()
		t := controlTask{ID: newID("op"), InstanceID: id, Action: action, IdempotencyKey: "desired:" + id + ":" + body.IdempotencyKey, Status: taskPending, RunAfter: now, CreatedAt: now, UpdatedAt: now, DesiredGeneration: generation}
		if _, err = tx.ExecContext(c.Request.Context(), `INSERT INTO xcloud_tasks (id,instance_id,action,idempotency_key,status,attempts,run_after,created_at,updated_at,desired_generation) VALUES (?,?,?,?,?,?,?,?,?,?)`, t.ID, t.InstanceID, t.Action, t.IdempotencyKey, t.Status, 0, t.RunAfter, now, now, t.DesiredGeneration); err != nil {
			internalError(c, err)
			return
		}
		task = &t
		_, err = tx.ExecContext(c.Request.Context(), `INSERT INTO xcloud_instance_operations (id,instance_id,generation,operation_kind,status,idempotency_key,spec_hash,task_id,requested_at,next_retry_at) VALUES (?,?,?,?,?,?,?,?,NOW(),NOW())`, "op-"+t.ID, id, generation, action, "pending", body.IdempotencyKey, specHash, t.ID)
		if err != nil {
			internalError(c, err)
			return
		}
	}
	if _, err = tx.ExecContext(c.Request.Context(), `INSERT INTO xcloud_instance_conditions (instance_id,condition_type,condition_status,reason,message,operation_id,observed_generation,updated_at) VALUES (?,?,?,?,?,?,?,NOW()) ON DUPLICATE KEY UPDATE condition_status=VALUES(condition_status),reason=VALUES(reason),message=VALUES(message),operation_id=VALUES(operation_id),observed_generation=VALUES(observed_generation),updated_at=NOW()`, id, "Reconciling", "True", "SpecChanged", "声明已接受，等待控制器调谐", nullableTaskID(task), generation); err != nil {
		internalError(c, err)
		return
	}
	if err = tx.Commit(); err != nil {
		internalError(c, err)
		return
	}
	if task != nil {
		_ = enqueuePersistedTask(c.Request.Context(), *task)
	}
	requestInstanceReconcile(id)
	_ = writeAudit(c.Request.Context(), u.ID, "instance.desired.patch", "instance", id, map[string]any{"resourceVersion": version, "operationId": nullableTaskID(task)})
	r, err := loadInstanceResource(c.Request.Context(), u.ID, id)
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"resource": r, "operation": task})
}

func nullableTaskID(task *controlTask) any {
	if task == nil {
		return nil
	}
	return task.ID
}
