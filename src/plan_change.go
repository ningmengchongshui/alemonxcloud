package cloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
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
	ID                      string
	InstanceID              string
	OwnerID                 string
	SourcePlanID            string
	TargetPlanID            string
	SourceCPU               float64
	SourceMemoryMB          int
	TargetCPU               float64
	TargetMemoryMB          int
	RemainingSeconds        int64
	DeltaFen                int
	ChargeFen               int
	RefundFen               int
	Status                  string
	TaskID                  string
	IdempotencyKey          string
	PendingWalletEntryID    string
	SettlementWalletEntryID string
	ErrorMessage            string
	CreatedAt               time.Time
	UpdatedAt               time.Time
	CompletedAt             *time.Time
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

func calculatePlanDelta(currentMonthly, targetMonthly int, expiry time.Time, now time.Time) (int64, int) {
	seconds := int64(math.Max(0, expiry.Sub(now).Seconds()))
	// A 30-day month is the fixed billing denominator used by the quote.
	delta := int(math.Round(float64(targetMonthly-currentMonthly) * float64(seconds) / float64(30*24*60*60)))
	return seconds, delta
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
	if !body.QuoteExpiresAt.IsZero() && time.Now().After(body.QuoteExpiresAt) {
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
	rows, err := instanceDB.QueryContext(c.Request.Context(), `SELECT id,instance_id,owner_id,source_plan_id,target_plan_id,source_cpu,source_memory_mb,target_cpu,target_memory_mb,remaining_seconds,delta_fen,charge_fen,refund_fen,status,COALESCE(task_id,''),idempotency_key,COALESCE(pending_wallet_entry_id,''),COALESCE(settlement_wallet_entry_id,''),COALESCE(error_message,''),created_at,updated_at,completed_at FROM xcloud_instance_plan_changes WHERE instance_id=? AND owner_id=? ORDER BY created_at DESC LIMIT 20`, c.Param("id"), user.ID)
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()
	out := []planChangeRecord{}
	for rows.Next() {
		var v planChangeRecord
		if err := rows.Scan(&v.ID, &v.InstanceID, &v.OwnerID, &v.SourcePlanID, &v.TargetPlanID, &v.SourceCPU, &v.SourceMemoryMB, &v.TargetCPU, &v.TargetMemoryMB, &v.RemainingSeconds, &v.DeltaFen, &v.ChargeFen, &v.RefundFen, &v.Status, &v.TaskID, &v.IdempotencyKey, &v.PendingWalletEntryID, &v.SettlementWalletEntryID, &v.ErrorMessage, &v.CreatedAt, &v.UpdatedAt, &v.CompletedAt); err != nil {
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
		if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_wallet_entries (id,user_id,amount_fen,balance_after_fen,entry_type,note,actor_id,created_at) SELECT ?,user_id,?,?,?, ?,?,NOW() FROM xcloud_wallets WHERE user_id=?`, pending, -charge, balance-charge, "plan_change_pending", "套餐升级暂扣", ownerID, ownerID); err != nil {
			return planChangeRecord{}, controlTask{}, err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_instance_plan_changes (id,instance_id,owner_id,source_plan_id,target_plan_id,source_cpu,source_memory_mb,target_cpu,target_memory_mb,remaining_seconds,delta_fen,charge_fen,refund_fen,status,idempotency_key,pending_wallet_entry_id,before_snapshot,after_snapshot,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, changeID, instanceID, ownerID, currentPlan, targetPlanID, currentCPU, currentMem, targetCPU, targetMem, seconds, delta, charge, refund, "processing", idem, pending, before, after, now, now); err != nil {
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
	var id, owner, target string
	var delta, refund int
	var status string
	if err = tx.QueryRowContext(ctx, `SELECT id,owner_id,target_plan_id,delta_fen,refund_fen,status FROM xcloud_instance_plan_changes WHERE id=? FOR UPDATE`, p.ChangeID).Scan(&id, &owner, &target, &delta, &refund, &status); err != nil {
		return err
	}
	if status != "processing" {
		return nil
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
	if refund > 0 {
		var balance int
		if err = tx.QueryRowContext(ctx, `SELECT balance_fen FROM xcloud_wallets WHERE user_id=? FOR UPDATE`, owner).Scan(&balance); err != nil {
			return err
		}
		entryID = newID("wal")
		if _, err = tx.ExecContext(ctx, `UPDATE xcloud_wallets SET balance_fen=balance_fen+?,updated_at=NOW() WHERE user_id=?`, refund, owner); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_wallet_entries (id,user_id,amount_fen,balance_after_fen,entry_type,note,actor_id,created_at) VALUES (?,?,?,?,?,?,?,NOW())`, entryID, owner, refund, balance+refund, "plan_change_refund", "套餐降级差额退款", "system"); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE xcloud_instance_plan_changes SET status='succeeded',settlement_wallet_entry_id=?,updated_at=NOW(),completed_at=NOW() WHERE id=? AND status='processing'`, entryID, p.ChangeID); err != nil {
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
	var owner string
	var amount, status string
	if tx.QueryRowContext(ctx, `SELECT owner_id,charge_fen,status FROM xcloud_instance_plan_changes WHERE id=? FOR UPDATE`, p.ChangeID).Scan(&owner, &amount, &status) != nil || status != "processing" {
		return
	}
	var charge int
	fmt.Sscan(amount, &charge)
	var entryID string
	if charge > 0 {
		var balance int
		if tx.QueryRowContext(ctx, `SELECT balance_fen FROM xcloud_wallets WHERE user_id=? FOR UPDATE`, owner).Scan(&balance) != nil {
			return
		}
		entryID = newID("wal")
		if _, err = tx.ExecContext(ctx, `UPDATE xcloud_wallets SET balance_fen=balance_fen+?,updated_at=NOW() WHERE user_id=?`, charge, owner); err != nil {
			return
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_wallet_entries (id,user_id,amount_fen,balance_after_fen,entry_type,note,actor_id,created_at) VALUES (?,?,?,?,?,?,?,NOW())`, entryID, owner, charge, balance+charge, "plan_change_refund", "套餐变更失败退回暂扣", "system"); err != nil {
			return
		}
	}
	_, _ = tx.ExecContext(ctx, `UPDATE xcloud_instance_plan_changes SET status='failed',settlement_wallet_entry_id=?,error_message=?,updated_at=NOW(),completed_at=NOW() WHERE id=? AND status='processing'`, entryID, truncateError(cause.Error()), p.ChangeID)
	_ = tx.Commit()
}
