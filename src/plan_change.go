package cloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type planChangeQuote struct {
	QuoteID          string    `json:"quoteId"`
	InstanceID       string    `json:"instanceId"`
	CurrentPlanID    string    `json:"currentPlanId"`
	CurrentPlanName  string    `json:"currentPlanName"`
	TargetPlanID     string    `json:"targetPlanId"`
	TargetPlanName   string    `json:"targetPlanName"`
	CurrentCPU       float64   `json:"currentCpu"`
	CurrentMemoryMB  int       `json:"currentMemoryMB"`
	TargetCPU        float64   `json:"targetCpu"`
	TargetMemoryMB   int       `json:"targetMemoryMB"`
	RemainingSeconds int64     `json:"remainingSeconds"`
	DeltaFen         int       `json:"deltaFen"`
	ChargeFen        int       `json:"chargeFen"`
	RefundFen        int       `json:"refundFen"`
	ExpiresAt        time.Time `json:"expiresAt"`
	Summary          string    `json:"summary"`
}

type planChangeRecord struct {
	ID                      string     `json:"id"`
	InstanceID              string     `json:"instanceId"`
	OwnerID                 string     `json:"ownerId"`
	SourcePlanID            string     `json:"sourcePlanId"`
	TargetPlanID            string     `json:"targetPlanId"`
	SourceCPU               float64    `json:"sourceCpu"`
	SourceMemoryMB          int        `json:"sourceMemoryMB"`
	TargetCPU               float64    `json:"targetCpu"`
	TargetMemoryMB          int        `json:"targetMemoryMB"`
	RemainingSeconds        int64      `json:"remainingSeconds"`
	DeltaFen                int        `json:"deltaFen"`
	ChargeFen               int        `json:"chargeFen"`
	RefundFen               int        `json:"refundFen"`
	Status                  string     `json:"status"`
	FundStatus              string     `json:"fundStatus"`
	AgentVerifyStatus       string     `json:"agentVerifyStatus"`
	AgentVerifiedAt         *time.Time `json:"agentVerifiedAt,omitempty"`
	AgentVerifyResult       string     `json:"agentVerifyResult,omitempty"`
	AgentVerifyError        string     `json:"agentVerifyError,omitempty"`
	TaskID                  string     `json:"taskId"`
	IdempotencyKey          string     `json:"idempotencyKey"`
	PendingWalletEntryID    string     `json:"pendingWalletEntryId"`
	SettlementWalletEntryID string     `json:"settlementWalletEntryId"`
	ErrorMessage            string     `json:"errorMessage,omitempty"`
	CreatedAt               time.Time  `json:"createdAt"`
	UpdatedAt               time.Time  `json:"updatedAt"`
	CompletedAt             *time.Time `json:"completedAt,omitempty"`
}

func planChangeHandler(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	instanceID := c.Param("id")
	var body struct {
		TargetPlanID string `json:"targetPlanId" binding:"required"`
	}
	if c.ShouldBindJSON(&body) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "目标套餐无效"})
		return
	}
	quote, err := buildPlanChangeQuote(c.Request.Context(), user.ID, instanceID, strings.TrimSpace(body.TargetPlanID))
	if err != nil {
		businessError(c, err)
		return
	}
	c.JSON(http.StatusOK, quote)
}

func planChangeQuoteHandler(c *gin.Context) { planChangeHandler(c) }

func latestPlanForInstance(ctx context.Context, tx *sql.Tx, ownerID, instanceID string) (string, string, float64, int, time.Time, string, error) {
	var planID, name, instanceStatus string
	var cpu float64
	var memory int
	var expiry time.Time
	err := tx.QueryRowContext(ctx, `SELECT p.id,p.name,ins.cpu,ins.memory_mb,ins.expires_at,ins.status FROM xcloud_orders o JOIN xcloud_plans p ON p.id=o.plan_id JOIN xcloud_instances ins ON ins.id=o.instance_id WHERE o.owner_id=? AND o.instance_id=? AND o.status IN (?,?) ORDER BY o.created_at DESC LIMIT 1`, ownerID, instanceID, orderActive, orderExpired).Scan(&planID, &name, &cpu, &memory, &expiry, &instanceStatus)
	if err != nil {
		return planID, name, cpu, memory, expiry, instanceStatus, err
	}
	var changedPlanID string
	if err = tx.QueryRowContext(ctx, `SELECT target_plan_id FROM xcloud_instance_plan_changes WHERE instance_id=? AND status='succeeded' ORDER BY completed_at DESC, created_at DESC LIMIT 1`, instanceID).Scan(&changedPlanID); err == nil && changedPlanID != "" {
		if err = tx.QueryRowContext(ctx, `SELECT id,name FROM xcloud_plans WHERE id=?`, changedPlanID).Scan(&planID, &name); err != nil {
			return planID, name, cpu, memory, expiry, instanceStatus, err
		}
	}
	return planID, name, cpu, memory, expiry, instanceStatus, nil
}

func effectiveInstancePlan(ctx context.Context, ownerID, instanceID string) (string, string) {
	var planID, planName string
	if err := instanceDB.QueryRowContext(ctx, `SELECT p.id,p.name FROM xcloud_orders o JOIN xcloud_plans p ON p.id=o.plan_id WHERE o.owner_id=? AND o.instance_id=? AND o.status IN (?,?) ORDER BY o.created_at DESC LIMIT 1`, ownerID, instanceID, orderActive, orderExpired).Scan(&planID, &planName); err != nil {
		return "", ""
	}
	var changedID string
	if err := instanceDB.QueryRowContext(ctx, `SELECT target_plan_id FROM xcloud_instance_plan_changes WHERE instance_id=? AND status='succeeded' ORDER BY completed_at DESC,created_at DESC LIMIT 1`, instanceID).Scan(&changedID); err == nil && changedID != "" {
		_ = instanceDB.QueryRowContext(ctx, `SELECT id,name FROM xcloud_plans WHERE id=?`, changedID).Scan(&planID, &planName)
	}
	return planID, planName
}

func calculatePlanDelta(currentMonthly, targetMonthly int, expiry time.Time, now time.Time) (int64, int) {
	seconds := int64(math.Max(0, expiry.Sub(now).Seconds()))
	// A 30-day month is the fixed billing denominator used by the quote.
	delta := int(math.Round(float64(targetMonthly-currentMonthly) * float64(seconds) / float64(30*24*60*60)))
	return seconds, delta
}

func validPlanChangeQuoteExpiry(expiresAt, now time.Time) bool {
	if expiresAt.IsZero() || !expiresAt.After(now) {
		return false
	}
	// The quote endpoint issues five-minute quotes. Do not trust a client to
	// extend that window and submit a stale price snapshot.
	return !expiresAt.After(now.Add(5 * time.Minute))
}

func buildPlanChangeQuote(ctx context.Context, ownerID, instanceID, targetPlanID string) (planChangeQuote, error) {
	tx, err := beginSerializableTx(ctx)
	if err != nil {
		return planChangeQuote{}, err
	}
	defer tx.Rollback()
	var currentID, currentName, status string
	var currentCPU, targetCPU float64
	var currentMemory, targetMemory int
	var expiry time.Time
	currentID, currentName, currentCPU, currentMemory, expiry, status, err = latestPlanForInstance(ctx, tx, ownerID, instanceID)
	if err != nil {
		return planChangeQuote{}, errors.New("实例没有可变更的有效套餐")
	}
	if status != "running" && status != "stopped" {
		return planChangeQuote{}, errors.New("仅运行中或已关机实例可以变更套餐")
	}
	var targetName string
	var monthly, targetMonthly int
	if err = tx.QueryRowContext(ctx, `SELECT name,cpu,memory_mb,monthly_price_fen FROM xcloud_plans WHERE id=? AND enabled=TRUE`, targetPlanID).Scan(&targetName, &targetCPU, &targetMemory, &targetMonthly); err != nil {
		return planChangeQuote{}, errors.New("目标套餐不可用")
	}
	if err = tx.QueryRowContext(ctx, `SELECT monthly_price_fen FROM xcloud_plans WHERE id=?`, currentID).Scan(&monthly); err != nil {
		return planChangeQuote{}, err
	}
	if currentID == targetPlanID {
		return planChangeQuote{}, errors.New("目标套餐与当前套餐相同")
	}
	seconds, delta := calculatePlanDelta(monthly, targetMonthly, expiry, time.Now())
	quote := planChangeQuote{QuoteID: newID("quote"), InstanceID: instanceID, CurrentPlanID: currentID, CurrentPlanName: currentName, TargetPlanID: targetPlanID, TargetPlanName: targetName, CurrentCPU: currentCPU, CurrentMemoryMB: currentMemory, TargetCPU: targetCPU, TargetMemoryMB: targetMemory, RemainingSeconds: seconds, DeltaFen: delta, ExpiresAt: time.Now().Add(5 * time.Minute), Summary: "套餐变更立即生效；本次仅调整 CPU 和内存，不调整网络"}
	if delta > 0 {
		quote.ChargeFen = delta
		quote.Summary += "，需补差价"
	} else {
		quote.RefundFen = -delta
		quote.Summary += "，差额退回 XCoin 钱包"
	}
	return quote, nil
}

func submitPlanChangeHandler(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	var body struct {
		TargetPlanID   string    `json:"targetPlanId" binding:"required"`
		QuoteID        string    `json:"quoteId"`
		QuoteExpiresAt time.Time `json:"quoteExpiresAt"`
		CurrentPlanID  string    `json:"currentPlanId"`
	}
	if c.ShouldBindJSON(&body) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "套餐变更参数无效"})
		return
	}
	if !validPlanChangeQuoteExpiry(body.QuoteExpiresAt, time.Now()) {
		c.JSON(http.StatusConflict, gin.H{"message": "报价已过期，请重新报价"})
		return
	}
	change, task, err := createPlanChange(c.Request.Context(), user.ID, c.Param("id"), body.TargetPlanID, body.CurrentPlanID)
	if err != nil {
		businessError(c, err)
		return
	}
	if err = enqueuePersistedTask(c.Request.Context(), task); err != nil {
		c.JSON(http.StatusAccepted, gin.H{"change": change, "task": task, "message": "套餐变更已记录，等待队列恢复"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"change": change, "task": task})
}

func getPlanChangesHandler(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	rows, err := instanceDB.QueryContext(c.Request.Context(), `SELECT id,instance_id,owner_id,source_plan_id,target_plan_id,source_cpu,source_memory_mb,target_cpu,target_memory_mb,remaining_seconds,delta_fen,charge_fen,refund_fen,status,fund_status,agent_verify_status,agent_verified_at,COALESCE(CAST(agent_verify_result AS CHAR),''),COALESCE(agent_verify_error,''),COALESCE(task_id,''),idempotency_key,COALESCE(pending_wallet_entry_id,''),COALESCE(settlement_wallet_entry_id,''),COALESCE(error_message,''),created_at,updated_at,completed_at FROM xcloud_instance_plan_changes WHERE instance_id=? AND owner_id=? ORDER BY created_at DESC LIMIT 20`, c.Param("id"), user.ID)
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()
	out := []planChangeRecord{}
	for rows.Next() {
		var v planChangeRecord
		if err := rows.Scan(&v.ID, &v.InstanceID, &v.OwnerID, &v.SourcePlanID, &v.TargetPlanID, &v.SourceCPU, &v.SourceMemoryMB, &v.TargetCPU, &v.TargetMemoryMB, &v.RemainingSeconds, &v.DeltaFen, &v.ChargeFen, &v.RefundFen, &v.Status, &v.FundStatus, &v.AgentVerifyStatus, &v.AgentVerifiedAt, &v.AgentVerifyResult, &v.AgentVerifyError, &v.TaskID, &v.IdempotencyKey, &v.PendingWalletEntryID, &v.SettlementWalletEntryID, &v.ErrorMessage, &v.CreatedAt, &v.UpdatedAt, &v.CompletedAt); err != nil {
			internalError(c, err)
			return
		}
		out = append(out, v)
	}
	c.JSON(http.StatusOK, out)
}

func createPlanChange(ctx context.Context, ownerID, instanceID, targetPlanID, expectedCurrentPlan string) (planChangeRecord, controlTask, error) {
	tx, err := beginSerializableTx(ctx)
	if err != nil {
		return planChangeRecord{}, controlTask{}, err
	}
	defer tx.Rollback()
	var currentPlan, currentName string
	var currentCPU, targetCPU float64
	var currentMem, targetMem int
	var expiry time.Time
	var status string
	if err = tx.QueryRowContext(ctx, `SELECT id FROM xcloud_instances WHERE id=? AND owner_id=? FOR UPDATE`, instanceID, ownerID).Scan(new(string)); err != nil {
		return planChangeRecord{}, controlTask{}, errors.New("实例没有可变更的有效套餐")
	}
	currentPlan, currentName, currentCPU, currentMem, expiry, status, err = latestPlanForInstance(ctx, tx, ownerID, instanceID)
	if err != nil {
		return planChangeRecord{}, controlTask{}, errors.New("实例没有可变更的有效套餐")
	}
	if status != "running" && status != "stopped" {
		return planChangeRecord{}, controlTask{}, errors.New("仅运行中或已关机实例可以变更套餐")
	}
	if expectedCurrentPlan != "" && expectedCurrentPlan != currentPlan {
		return planChangeRecord{}, controlTask{}, errors.New("当前套餐已变化，请重新报价")
	}
	var targetName string
	var targetMonthly int
	if err = tx.QueryRowContext(ctx, `SELECT name,cpu,memory_mb,monthly_price_fen FROM xcloud_plans WHERE id=? AND enabled=TRUE FOR UPDATE`, targetPlanID).Scan(&targetName, &targetCPU, &targetMem, &targetMonthly); err != nil {
		return planChangeRecord{}, controlTask{}, errors.New("目标套餐不可用")
	}
	if targetPlanID == currentPlan {
		return planChangeRecord{}, controlTask{}, errors.New("目标套餐与当前套餐相同")
	}
	var currentMonthly int
	if err = tx.QueryRowContext(ctx, `SELECT monthly_price_fen FROM xcloud_plans WHERE id=?`, currentPlan).Scan(&currentMonthly); err != nil {
		return planChangeRecord{}, controlTask{}, err
	}
	seconds, delta := calculatePlanDelta(currentMonthly, targetMonthly, expiry, time.Now())
	var active int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM xcloud_tasks WHERE instance_id=? AND status IN (?,?) AND action IN ('create','retry-deploy','start','stop','update','restart','reinstall','destroy','purge','resize')`, instanceID, taskPending, taskRunning).Scan(&active); err != nil {
		return planChangeRecord{}, controlTask{}, err
	}
	if active > 0 {
		return planChangeRecord{}, controlTask{}, errors.New("实例正在处理中")
	}
	var nodeEnabled bool
	var heartbeat sql.NullTime
	var nodeID string
	if err = tx.QueryRowContext(ctx, `SELECT n.id,n.enabled,n.last_heartbeat_at FROM xcloud_instances i JOIN xcloud_nodes n ON n.id=i.node_id WHERE i.id=? FOR UPDATE`, instanceID).Scan(&nodeID, &nodeEnabled, &heartbeat); err != nil || !nodeEnabled || !heartbeat.Valid || time.Since(heartbeat.Time) > nodeHeartbeatTTL() {
		return planChangeRecord{}, controlTask{}, errors.New("实例节点暂不可用")
	}
	var capabilities []byte
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(agent_capabilities,JSON_ARRAY()) FROM xcloud_nodes WHERE id=?`, nodeID).Scan(&capabilities); err != nil {
		return planChangeRecord{}, controlTask{}, err
	}
	var capabilityList []string
	_ = json.Unmarshal(capabilities, &capabilityList)
	resizeSupported := false
	for _, capability := range capabilityList {
		if capability == "container.compose.resize.v1" {
			resizeSupported = true
			break
		}
	}
	if !resizeSupported {
		return planChangeRecord{}, controlTask{}, errors.New("当前节点 Agent 不支持无启动套餐变更，请先升级 Agent")
	}
	var usedCPU float64
	var usedMem int
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(cpu),0),COALESCE(SUM(memory_mb),0) FROM xcloud_instances WHERE node_id=? AND id<>? AND status IN ('deploying','running','stopped','destroy_scheduled')`, nodeID, instanceID).Scan(&usedCPU, &usedMem); err != nil {
		return planChangeRecord{}, controlTask{}, err
	}
	var capCPU float64
	var capMem int
	if err = tx.QueryRowContext(ctx, `SELECT cpu_total,memory_total_mb FROM xcloud_nodes WHERE id=?`, nodeID).Scan(&capCPU, &capMem); err != nil {
		return planChangeRecord{}, controlTask{}, err
	}
	if usedCPU+targetCPU > capCPU || usedMem+targetMem > capMem {
		return planChangeRecord{}, controlTask{}, errors.New("当前节点容量不足，无法变更套餐")
	}
	changeID := newID("resize")
	taskID := newID("task")
	idem := "resize:" + instanceID + ":" + changeID
	before, _ := json.Marshal(map[string]any{"planId": currentPlan, "planName": currentName, "cpu": currentCPU, "memoryMB": currentMem})
	after, _ := json.Marshal(map[string]any{"planId": targetPlanID, "planName": targetName, "cpu": targetCPU, "memoryMB": targetMem})
	now := time.Now()
	charge := 0
	refund := 0
	if delta > 0 {
		charge = delta
	} else {
		refund = -delta
	}
	var pending string
	if charge > 0 {
		var balance int
		if err = tx.QueryRowContext(ctx, `SELECT balance_fen FROM xcloud_wallets WHERE user_id=? FOR UPDATE`, ownerID).Scan(&balance); err != nil || balance < charge {
			return planChangeRecord{}, controlTask{}, errors.New("XCoin 余额不足")
		}
		pending = newID("wal")
		if _, err = tx.ExecContext(ctx, `UPDATE xcloud_wallets SET balance_fen=balance_fen-?,updated_at=NOW() WHERE user_id=?`, charge, ownerID); err != nil {
			return planChangeRecord{}, controlTask{}, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_wallet_entries (id,user_id,amount_fen,balance_after_fen,entry_type,note,actor_id,plan_change_id,business_key,created_at) SELECT ?,user_id,?,?,?, ?,?,?,?,NOW() FROM xcloud_wallets WHERE user_id=?`, pending, -charge, balance-charge, "plan_change_pending", "套餐升级暂扣", ownerID, changeID, "plan-change:pending:"+changeID, ownerID); err != nil {
			return planChangeRecord{}, controlTask{}, err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_instance_plan_changes (id,instance_id,owner_id,source_plan_id,target_plan_id,source_cpu,source_memory_mb,target_cpu,target_memory_mb,remaining_seconds,delta_fen,charge_fen,refund_fen,status,fund_status,idempotency_key,pending_wallet_entry_id,before_snapshot,after_snapshot,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, changeID, instanceID, ownerID, currentPlan, targetPlanID, currentCPU, currentMem, targetCPU, targetMem, seconds, delta, charge, refund, "processing", "pending", idem, pending, before, after, now, now); err != nil {
		return planChangeRecord{}, controlTask{}, err
	}
	payload, _ := json.Marshal(map[string]any{"changeId": changeID, "cpu": targetCPU, "memoryMB": targetMem, "wasRunning": status == "running"})
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_tasks (id,instance_id,action,idempotency_key,status,attempts,run_after,created_at,updated_at,payload) VALUES (?,?,?,?,?,?,?,?,?,?)`, taskID, instanceID, "resize", idem, taskPending, 0, now, now, now, payload); err != nil {
		return planChangeRecord{}, controlTask{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE xcloud_instance_plan_changes SET task_id=? WHERE id=?`, taskID, changeID); err != nil {
		return planChangeRecord{}, controlTask{}, err
	}
	if err = writeAuditTx(ctx, tx, ownerID, "instance.plan_change", "instance", instanceID, map[string]any{"changeId": changeID, "targetPlanId": targetPlanID, "deltaFen": delta}); err != nil {
		return planChangeRecord{}, controlTask{}, err
	}
	if err = tx.Commit(); err != nil {
		return planChangeRecord{}, controlTask{}, err
	}
	return planChangeRecord{ID: changeID, InstanceID: instanceID, OwnerID: ownerID, SourcePlanID: currentPlan, TargetPlanID: targetPlanID, SourceCPU: currentCPU, SourceMemoryMB: currentMem, TargetCPU: targetCPU, TargetMemoryMB: targetMem, RemainingSeconds: seconds, DeltaFen: delta, ChargeFen: charge, RefundFen: refund, Status: "processing", TaskID: taskID, IdempotencyKey: idem, PendingWalletEntryID: pending, CreatedAt: now, UpdatedAt: now}, controlTask{ID: taskID, InstanceID: instanceID, Action: "resize", IdempotencyKey: idem, Status: taskPending, RunAfter: now, CreatedAt: now, UpdatedAt: now, Payload: payload}, nil
}

func markPlanChangeBlocked(ctx context.Context, changeID, message string) {
	_, _ = instanceDB.ExecContext(ctx, `UPDATE xcloud_instance_plan_changes SET status='needs_review',fund_status='blocked',agent_verify_status='unavailable',agent_verified_at=NOW(),agent_verify_result=JSON_OBJECT('status','unknown'),agent_verify_error=?,updated_at=NOW() WHERE id=? AND status='processing'`, truncateError(message), changeID)
}

// reconcileRecoveredPlanChange resolves a resize task whose worker lease
// expired after the Agent call. It never guesses: only an exact target or
// exact source resource match is settled automatically.
func reconcileRecoveredPlanChange(ctx context.Context, task controlTask) {
	var payload struct {
		ChangeID string `json:"changeId"`
	}
	if json.Unmarshal(task.Payload, &payload) != nil || payload.ChangeID == "" {
		return
	}
	var containerName, nodeID string
	if err := instanceDB.QueryRowContext(ctx, `SELECT container_name,COALESCE(node_id,'') FROM xcloud_instances WHERE id=?`, task.InstanceID).Scan(&containerName, &nodeID); err != nil {
		markPlanChangeBlocked(ctx, payload.ChangeID, err.Error())
		return
	}
	n, err := nodeByID(ctx, nodeID)
	if err != nil {
		markPlanChangeBlocked(ctx, payload.ChangeID, err.Error())
		return
	}
	var inspection struct {
		NanoCPUs    string `json:"nanoCPUs"`
		MemoryBytes string `json:"memoryBytes"`
	}
	if err = nodeRequest(ctx, n, "GET", "/container/"+containerName+"/inspect", nil, &inspection); err != nil {
		markPlanChangeBlocked(ctx, payload.ChangeID, err.Error())
		appendTaskEvent(ctx, task.ID, "plan_change_verify_failed", truncateError(err.Error()))
		return
	}
	var targetCPU, sourceCPU float64
	var targetMemory, sourceMemory int
	if err = instanceDB.QueryRowContext(ctx, `SELECT source_cpu,source_memory_mb,target_cpu,target_memory_mb FROM xcloud_instance_plan_changes WHERE id=?`, payload.ChangeID).Scan(&sourceCPU, &sourceMemory, &targetCPU, &targetMemory); err != nil {
		markPlanChangeBlocked(ctx, payload.ChangeID, err.Error())
		return
	}
	nano, nanoErr := strconv.ParseInt(strings.TrimSpace(inspection.NanoCPUs), 10, 64)
	memoryBytes, memoryErr := strconv.ParseInt(strings.TrimSpace(inspection.MemoryBytes), 10, 64)
	if nanoErr != nil || memoryErr != nil {
		markPlanChangeBlocked(ctx, payload.ChangeID, "Agent 未返回可核实的 CPU/内存配置")
		return
	}
	actualCPU := float64(nano) / 1e9
	actualMemory := int(memoryBytes / (1024 * 1024))
	if math.Abs(actualCPU-targetCPU) < 0.001 && actualMemory == targetMemory {
		if err = completePlanChange(ctx, task); err != nil {
			markPlanChangeBlocked(ctx, payload.ChangeID, err.Error())
			return
		}
		_, _ = instanceDB.ExecContext(ctx, `UPDATE xcloud_tasks SET status='succeeded',finished_at=NOW(),last_error=NULL,updated_at=NOW() WHERE id=? AND status='needs_review'`, task.ID)
		releaseInstanceTaskLock(ctx, task)
		appendTaskEvent(ctx, task.ID, "plan_change_verified_target", "Agent 配置已匹配目标套餐，变更自动完成")
		return
	}
	if math.Abs(actualCPU-sourceCPU) < 0.001 && actualMemory == sourceMemory {
		failPlanChange(ctx, task, errors.New("Agent 配置仍为变更前资源"))
		_, _ = instanceDB.ExecContext(ctx, `UPDATE xcloud_instance_plan_changes SET agent_verify_status='verified_old',agent_verified_at=NOW(),agent_verify_result=JSON_OBJECT('cpu',?,'memoryMB',?,'status','source') WHERE id=? AND status='failed'`, actualCPU, actualMemory, payload.ChangeID)
		_, _ = instanceDB.ExecContext(ctx, `UPDATE xcloud_tasks SET status='failed',finished_at=NOW(),last_error='Agent 配置未发生变更',updated_at=NOW() WHERE id=? AND status='needs_review'`, task.ID)
		releaseInstanceTaskLock(ctx, task)
		appendTaskEvent(ctx, task.ID, "plan_change_verified_source", "Agent 配置仍为原套餐，已退款并结束变更")
		return
	}
	markPlanChangeBlocked(ctx, payload.ChangeID, fmt.Sprintf("Agent 当前资源 %.3f 核/%d MB 与目标及原配置均不一致", actualCPU, actualMemory))
	appendTaskEvent(ctx, task.ID, "plan_change_verify_ambiguous", "Agent 资源无法与目标或原配置匹配，等待人工复核")
}

func completePlanChange(ctx context.Context, task controlTask) error {
	var p struct {
		ChangeID string  `json:"changeId"`
		CPU      float64 `json:"cpu"`
		MemoryMB int     `json:"memoryMB"`
	}
	if err := json.Unmarshal(task.Payload, &p); err != nil {
		return err
	}
	tx, err := beginSerializableTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id, owner, target, pendingEntry string
	var delta, refund int
	var status string
	if err = tx.QueryRowContext(ctx, `SELECT id,owner_id,target_plan_id,delta_fen,refund_fen,status,COALESCE(pending_wallet_entry_id,'') FROM xcloud_instance_plan_changes WHERE id=? FOR UPDATE`, p.ChangeID).Scan(&id, &owner, &target, &delta, &refund, &status, &pendingEntry); err != nil {
		return err
	}
	if status != "processing" {
		return nil
	}
	if task.ExecutionToken != "" {
		var owned int
		if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM xcloud_tasks t JOIN xcloud_instances i ON i.id=t.instance_id WHERE t.id=? AND t.instance_id=? AND t.status=? AND t.worker_id=? AND t.execution_token=? AND t.claim_expires_at>NOW() AND i.active_task_id=t.id AND i.active_task_token=t.execution_token AND i.active_task_expires_at>NOW() AND i.status IN ('running','stopped')`, task.ID, task.InstanceID, taskRunning, task.WorkerID, task.ExecutionToken).Scan(&owned); err != nil {
			return err
		}
		if owned != 1 {
			return errors.New("任务已失去套餐变更执行租约")
		}
	}
	var currentStatus string
	var bandwidthMbps int
	if err = tx.QueryRowContext(ctx, `SELECT status,bandwidth_mbps FROM xcloud_instances WHERE id=? FOR UPDATE`, task.InstanceID).Scan(&currentStatus, &bandwidthMbps); err != nil {
		return err
	}
	spec := fmt.Sprintf("%g 核 / %d GB / 最高 %d Mbps", p.CPU, p.MemoryMB/1024, bandwidthMbps)
	result, err := tx.ExecContext(ctx, `UPDATE xcloud_instances SET cpu=?,memory_mb=?,spec=? WHERE id=? AND status IN ('running','stopped')`, p.CPU, p.MemoryMB, spec, task.InstanceID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return errors.New("实例状态已变化，套餐变更未提交")
	}
	var entryID string
	fundStatus := "charged"
	if refund > 0 {
		var balance int
		if err = tx.QueryRowContext(ctx, `SELECT balance_fen FROM xcloud_wallets WHERE user_id=? FOR UPDATE`, owner).Scan(&balance); err != nil {
			return err
		}
		entryID = newID("wal")
		if _, err = tx.ExecContext(ctx, `UPDATE xcloud_wallets SET balance_fen=balance_fen+?,updated_at=NOW() WHERE user_id=?`, refund, owner); err != nil {
			return err
		}
		fundStatus = "refunded"
		if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_wallet_entries (id,user_id,amount_fen,balance_after_fen,entry_type,note,actor_id,plan_change_id,business_key,created_at) VALUES (?,?,?,?,?,?,?,?,?,NOW())`, entryID, owner, refund, balance+refund, "plan_change_refund", "套餐降级差额退款", "system", p.ChangeID, "plan-change:refund:"+p.ChangeID); err != nil {
			return err
		}
	}
	if entryID == "" {
		entryID = pendingEntry
	}
	if _, err = tx.ExecContext(ctx, `UPDATE xcloud_instance_plan_changes SET status='succeeded',fund_status=?,settlement_wallet_entry_id=?,agent_verify_status='verified',agent_verified_at=NOW(),agent_verify_result=JSON_OBJECT('cpu',?,'memoryMB',?,'status','target'),updated_at=NOW(),completed_at=NOW() WHERE id=? AND status='processing'`, fundStatus, entryID, p.CPU, p.MemoryMB, p.ChangeID); err != nil {
		return err
	}
	return tx.Commit()
}

func failPlanChange(ctx context.Context, task controlTask, cause error) {
	var p struct {
		ChangeID string `json:"changeId"`
	}
	if json.Unmarshal(task.Payload, &p) != nil {
		return
	}
	tx, err := beginSerializableTx(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback()
	var owner, status string
	var chargeFen int
	if tx.QueryRowContext(ctx, `SELECT owner_id,charge_fen,status FROM xcloud_instance_plan_changes WHERE id=? FOR UPDATE`, p.ChangeID).Scan(&owner, &chargeFen, &status) != nil || status != "processing" {
		return
	}
	var entryID string
	if chargeFen > 0 {
		var balance int
		if tx.QueryRowContext(ctx, `SELECT balance_fen FROM xcloud_wallets WHERE user_id=? FOR UPDATE`, owner).Scan(&balance) != nil {
			return
		}
		entryID = newID("wal")
		if _, err = tx.ExecContext(ctx, `UPDATE xcloud_wallets SET balance_fen=balance_fen+?,updated_at=NOW() WHERE user_id=?`, chargeFen, owner); err != nil {
			return
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_wallet_entries (id,user_id,amount_fen,balance_after_fen,entry_type,note,actor_id,plan_change_id,business_key,created_at) VALUES (?,?,?,?,?,?,?,?,?,NOW())`, entryID, owner, chargeFen, balance+chargeFen, "plan_change_refund", "套餐变更失败退回暂扣", "system", p.ChangeID, "plan-change:refund:"+p.ChangeID); err != nil {
			return
		}
	}
	_, _ = tx.ExecContext(ctx, `UPDATE xcloud_instance_plan_changes SET status='failed',fund_status='refunded',settlement_wallet_entry_id=?,agent_verify_status='not_checked',agent_verify_result=JSON_OBJECT('status','agent_error'),error_message=?,updated_at=NOW(),completed_at=NOW() WHERE id=? AND status='processing'`, entryID, truncateError(cause.Error()), p.ChangeID)
	_ = tx.Commit()
}
