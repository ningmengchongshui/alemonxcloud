package cloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type planChangeQuote struct {
	QuoteID           string                  `json:"quoteId"`
	InstanceID        string                  `json:"instanceId"`
	CurrentPlanID     string                  `json:"currentPlanId"`
	CurrentPlanName   string                  `json:"currentPlanName"`
	TargetPlanID      string                  `json:"targetPlanId"`
	TargetPlanName    string                  `json:"targetPlanName"`
	CurrentCPU        float64                 `json:"currentCpu"`
	CurrentMemoryMB   int                     `json:"currentMemoryMB"`
	TargetCPU         float64                 `json:"targetCpu"`
	TargetMemoryMB    int                     `json:"targetMemoryMB"`
	RemainingSeconds  int64                   `json:"remainingSeconds"`
	DeltaFen          int                     `json:"deltaFen"`
	ChargeFen         int                     `json:"chargeFen"`
	RefundFen         int                     `json:"refundFen"`
	ExpiresAt         time.Time               `json:"expiresAt"`
	Summary           string                  `json:"summary"`
	OperationCutoffAt time.Time               `json:"operationCutoffAt"`
	Items             []planExchangeQuoteItem `json:"items"`
}

type planExchangeQuoteItem struct {
	SourceOrderID        string    `json:"sourceOrderId"`
	SourcePlanID         string    `json:"sourcePlanId"`
	SourcePlanName       string    `json:"sourcePlanName"`
	SourceStartsAt       time.Time `json:"sourceStartsAt"`
	SourceExpiresAt      time.Time `json:"sourceExpiresAt"`
	ActualPaidFen        int       `json:"actualPaidFen"`
	RetainedFen          int       `json:"retainedFen"`
	RefundFen            int       `json:"refundFen"`
	ReplacementChargeFen int       `json:"replacementChargeFen"`
	ReplacementStartsAt  time.Time `json:"replacementStartsAt"`
	ReplacementExpiresAt time.Time `json:"replacementExpiresAt"`
	TierDiscountBps      int       `json:"tierDiscountBps"`
	TierMonths           int       `json:"tierMonths"`
	DiscountAmountFen    int       `json:"discountAmountFen"`
	BenefitProgramID     string    `json:"benefitProgramId,omitempty"`
	BenefitName          string    `json:"benefitName,omitempty"`
}

func prorateFen(amount int, totalStart, totalEnd, from time.Time) int {
	if !totalEnd.After(totalStart) || !totalEnd.After(from) {
		return 0
	}
	if from.Before(totalStart) {
		from = totalStart
	}
	// Database service windows are second-granular. Never multiply an amount
	// by a nanosecond duration: even ordinary package prices overflow int64
	// across multi-month windows and can turn an upgrade into a fake refund.
	totalSeconds := int64(totalEnd.Sub(totalStart) / time.Second)
	remainingSeconds := int64(totalEnd.Sub(from) / time.Second)
	if amount <= 0 || totalSeconds <= 0 || remainingSeconds <= 0 {
		return 0
	}
	return int(int64(amount) * remainingSeconds / totalSeconds)
}

// prorateMonthlyFen prices a partial service window using the platform's
// fixed 30-day monthly billing denominator. The source order duration is not
// used: replacement orders retain only the old order's tier discount.
func prorateMonthlyFen(monthlyFen int, start, end time.Time) int {
	if monthlyFen <= 0 || !end.After(start) {
		return 0
	}
	seconds := int64(end.Sub(start) / time.Second)
	const monthSeconds = int64(30 * 24 * 60 * 60)
	if seconds <= 0 {
		return 0
	}
	return int(int64(monthlyFen) * seconds / monthSeconds)
}

func planChangeTierMonths(start, end time.Time) int {
	days := end.Sub(start) / (24 * time.Hour)
	switch {
	case days >= 360:
		return 12
	case days >= 180:
		return 6
	case days >= 90:
		return 3
	default:
		return 1
	}
}

// planChangeTargetRate selects the longest tier the remaining service period
// reaches. The selected tier's discounted monthly rate applies to every
// remaining day, including the final partial month. This keeps the service
// end unchanged while making 100 remaining days, for example, price as a
// three-month tier plus ten days at that same tier's daily rate.
func planChangeTargetRate(ctx context.Context, tx *sql.Tx, planID string, monthly int, start, end time.Time, lock bool) (int, int, error) {
	if monthly < 0 {
		return 0, 0, errors.New("目标套餐价格异常，请联系管理员核对套餐价格")
	}
	months := planChangeTierMonths(start, end)
	_, _, bps, err := tierPrice(ctx, tx, planID, months, monthly, lock)
	if err != nil {
		return 0, 0, err
	}
	if bps < 0 || bps > 10000 {
		return 0, 0, errors.New("目标套餐阶梯价格异常，请联系管理员核对套餐价格")
	}
	return months, bps, nil
}

// planChangeOrderRefundFen returns the exact unused value of a source order.
// It deliberately uses the order's real paid amount and its real service
// window: no arbitrary retention day, current list price, or new promotion
// may alter the user's remaining credit.
func planChangeOrderRefundFen(actualPaidFen int, start, end, now time.Time) int {
	if actualPaidFen <= 0 || !end.After(start) {
		return 0
	}
	return prorateFen(actualPaidFen, start, end, now)
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
	SagaStatus              string     `json:"sagaStatus,omitempty"`
	CompensationTaskID      string     `json:"compensationTaskId,omitempty"`
	CancelRequestedAt       *time.Time `json:"cancelRequestedAt,omitempty"`
	CancelReason            string     `json:"cancelReason,omitempty"`
	ObservedCPU             *float64   `json:"observedCpu,omitempty"`
	ObservedMemoryMB        *int       `json:"observedMemoryMB,omitempty"`
	ObservedAt              *time.Time `json:"observedAt,omitempty"`
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
	return planID, name, cpu, memory, expiry, instanceStatus, nil
}

func effectiveInstancePlan(ctx context.Context, ownerID, instanceID string) (string, string) {
	var planID, planName string
	if err := instanceDB.QueryRowContext(ctx, `SELECT p.id,p.name FROM xcloud_orders o JOIN xcloud_plans p ON p.id=o.plan_id WHERE o.owner_id=? AND o.instance_id=? AND o.status IN (?,?) ORDER BY o.created_at DESC LIMIT 1`, ownerID, instanceID, orderActive, orderExpired).Scan(&planID, &planName); err != nil {
		return "", ""
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
	if targetMonthly < 0 {
		return planChangeQuote{}, errors.New("目标套餐价格异常，请联系管理员核对套餐价格")
	}
	if err = tx.QueryRowContext(ctx, `SELECT monthly_price_fen FROM xcloud_plans WHERE id=?`, currentID).Scan(&monthly); err != nil {
		return planChangeQuote{}, err
	}
	if currentID == targetPlanID {
		return planChangeQuote{}, errors.New("目标套餐与当前套餐相同")
	}
	now := time.Now()
	seconds, _ := calculatePlanDelta(monthly, targetMonthly, expiry, now)
	quote := planChangeQuote{QuoteID: newID("quote"), InstanceID: instanceID, CurrentPlanID: currentID, CurrentPlanName: currentName, TargetPlanID: targetPlanID, TargetPlanName: targetName, CurrentCPU: currentCPU, CurrentMemoryMB: currentMemory, TargetCPU: targetCPU, TargetMemoryMB: targetMemory, RemainingSeconds: seconds, ExpiresAt: now.Add(5 * time.Minute), OperationCutoffAt: now, Summary: "按每笔订单的真实支付金额和未使用服务时长折算。"}
	rows, queryErr := tx.QueryContext(ctx, `SELECT o.id,o.plan_id,p.name,o.amount_fen,o.service_starts_at,o.expires_at
		FROM xcloud_orders o JOIN xcloud_plans p ON p.id=o.plan_id
		WHERE o.owner_id=? AND o.instance_id=? AND o.status=? AND o.service_starts_at IS NOT NULL AND o.expires_at>? ORDER BY o.service_starts_at,o.id`, ownerID, instanceID, orderActive, now)
	if queryErr != nil {
		return planChangeQuote{}, queryErr
	}
	type sourceOrderQuoteRow struct {
		item      planExchangeQuoteItem
		startsAt  time.Time
		expiresAt time.Time
	}
	sources := []sourceOrderQuoteRow{}
	// A *sql.Tx owns exactly one physical MySQL connection. Fully consume and
	// close this result set before querying tier prices or benefit programs;
	// issuing those nested queries with unread order rows makes go-sql-driver
	// discard the connection as bad, which surfaced as a deterministic 503.
	for rows.Next() {
		var source sourceOrderQuoteRow
		if err := rows.Scan(&source.item.SourceOrderID, &source.item.SourcePlanID, &source.item.SourcePlanName, &source.item.ActualPaidFen, &source.startsAt, &source.expiresAt); err != nil {
			rows.Close()
			return planChangeQuote{}, err
		}
		sources = append(sources, source)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return planChangeQuote{}, err
	}
	if err := rows.Close(); err != nil {
		return planChangeQuote{}, err
	}
	for _, source := range sources {
		item, start, end := source.item, source.startsAt, source.expiresAt
		item.ReplacementStartsAt = now
		if start.After(item.ReplacementStartsAt) {
			item.ReplacementStartsAt = start
		}
		item.ReplacementExpiresAt = end
		item.SourceStartsAt = start
		item.SourceExpiresAt = end
		item.RefundFen = planChangeOrderRefundFen(item.ActualPaidFen, start, end, now)
		item.RetainedFen = item.ActualPaidFen - item.RefundFen
		quote.Items = append(quote.Items, item)
	}
	if len(quote.Items) == 0 {
		return planChangeQuote{}, errors.New("没有可替换的未结束订单")
	}
	periodEnd := now
	for _, item := range quote.Items {
		if item.ReplacementExpiresAt.After(periodEnd) {
			periodEnd = item.ReplacementExpiresAt
		}
	}
	tierMonths, tierBps, err := planChangeTargetRate(ctx, tx, targetPlanID, targetMonthly, now, periodEnd, false)
	if err != nil {
		return planChangeQuote{}, err
	}
	discountedMonthly := targetMonthly * tierBps / 10000
	quote.ChargeFen, quote.RefundFen = 0, 0
	for index := range quote.Items {
		item := &quote.Items[index]
		item.TierMonths = tierMonths
		item.TierDiscountBps = tierBps
		item.ReplacementChargeFen = prorateMonthlyFen(discountedMonthly, item.ReplacementStartsAt, item.ReplacementExpiresAt)
		if item.ReplacementChargeFen < 0 || item.RefundFen < 0 {
			return planChangeQuote{}, errors.New("套餐变更报价异常，请联系管理员核对套餐价格")
		}
		quote.ChargeFen += item.ReplacementChargeFen
		quote.RefundFen += item.RefundFen
	}
	quote.DeltaFen = quote.ChargeFen - quote.RefundFen
	if quote.DeltaFen > 0 {
		quote.Summary += " 新套餐剩余价格高于旧套餐剩余价值，需补钱包差额。"
	} else if quote.DeltaFen < 0 {
		quote.Summary += " 旧套餐剩余价值高于新套餐剩余价格，差额退回 XCoin 钱包。"
	} else {
		quote.Summary += " 两边剩余价值相同，无需补退。"
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
	quote, err := buildPlanChangeQuote(c.Request.Context(), user.ID, c.Param("id"), strings.TrimSpace(body.TargetPlanID))
	if err != nil {
		businessError(c, err)
		return
	}
	if body.CurrentPlanID != "" && body.CurrentPlanID != quote.CurrentPlanID {
		c.JSON(http.StatusConflict, gin.H{"message": "当前套餐已变化，请重新报价"})
		return
	}
	change, task, err := createPlanChange(c.Request.Context(), user.ID, c.Param("id"), body.TargetPlanID, body.CurrentPlanID, quote)
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
	rows, err := instanceDB.QueryContext(c.Request.Context(), `SELECT id,instance_id,owner_id,source_plan_id,target_plan_id,source_cpu,source_memory_mb,target_cpu,target_memory_mb,remaining_seconds,delta_fen,charge_fen,refund_fen,status,fund_status,agent_verify_status,agent_verified_at,COALESCE(CAST(agent_verify_result AS CHAR),''),COALESCE(agent_verify_error,''),COALESCE(task_id,''),idempotency_key,COALESCE(pending_wallet_entry_id,''),COALESCE(settlement_wallet_entry_id,''),COALESCE(error_message,''),created_at,updated_at,completed_at,COALESCE(saga_status,''),COALESCE(compensation_task_id,''),cancel_requested_at,COALESCE(cancel_reason,''),observed_cpu,observed_memory_mb,observed_at FROM xcloud_instance_plan_changes WHERE instance_id=? AND owner_id=? ORDER BY created_at DESC LIMIT 20`, c.Param("id"), user.ID)
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()
	out := []planChangeRecord{}
	for rows.Next() {
		var v planChangeRecord
		if err := rows.Scan(&v.ID, &v.InstanceID, &v.OwnerID, &v.SourcePlanID, &v.TargetPlanID, &v.SourceCPU, &v.SourceMemoryMB, &v.TargetCPU, &v.TargetMemoryMB, &v.RemainingSeconds, &v.DeltaFen, &v.ChargeFen, &v.RefundFen, &v.Status, &v.FundStatus, &v.AgentVerifyStatus, &v.AgentVerifiedAt, &v.AgentVerifyResult, &v.AgentVerifyError, &v.TaskID, &v.IdempotencyKey, &v.PendingWalletEntryID, &v.SettlementWalletEntryID, &v.ErrorMessage, &v.CreatedAt, &v.UpdatedAt, &v.CompletedAt, &v.SagaStatus, &v.CompensationTaskID, &v.CancelRequestedAt, &v.CancelReason, &v.ObservedCPU, &v.ObservedMemoryMB, &v.ObservedAt); err != nil {
			internalError(c, err)
			return
		}
		out = append(out, v)
	}
	c.JSON(http.StatusOK, out)
}

// cancelPlanChangeHandler makes cancellation a user-owned compensating
// operation. It never releases money merely because the original resize is
// uncertain: the source resources must first be observed or re-applied.
func cancelPlanChangeHandler(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	task, err := requestPlanChangeCompensation(c.Request.Context(), user.ID, c.Param("id"), c.Param("changeID"))
	if err != nil {
		businessError(c, err)
		return
	}
	if task.ID != "" {
		if err = enqueuePersistedTask(c.Request.Context(), task); err != nil {
			c.JSON(http.StatusAccepted, gin.H{"task": task, "message": "已请求回退原套餐；节点恢复后会自动继续"})
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"task": task, "message": "已请求回退原套餐"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已确认实例仍为原套餐，暂扣资金已释放"})
}

func requestPlanChangeCompensation(ctx context.Context, ownerID, instanceID, changeID string) (controlTask, error) {
	tx, err := beginSerializableTx(ctx)
	if err != nil {
		return controlTask{}, err
	}
	defer tx.Rollback()
	var sourceCPU float64
	var sourceMemory int
	var status, saga string
	var observedCPU sql.NullFloat64
	var observedMemory sql.NullInt64
	if err = tx.QueryRowContext(ctx, `SELECT source_cpu,source_memory_mb,status,COALESCE(saga_status,''),observed_cpu,observed_memory_mb FROM xcloud_instance_plan_changes WHERE id=? AND instance_id=? AND owner_id=? FOR UPDATE`, changeID, instanceID, ownerID).Scan(&sourceCPU, &sourceMemory, &status, &saga, &observedCPU, &observedMemory); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return controlTask{}, errors.New("套餐变更不存在")
		}
		return controlTask{}, err
	}
	if saga == "cancelled" || status == "failed" {
		return controlTask{}, errors.New("套餐变更已取消")
	}
	if saga == "settled" || status == "succeeded" {
		return controlTask{}, errors.New("已结算的套餐变更不可取消")
	}
	if saga == "compensating" {
		var existing controlTask
		if err = scanControlTask(tx.QueryRowContext(ctx, `SELECT `+taskSelectFields+` FROM xcloud_tasks WHERE id=(SELECT compensation_task_id FROM xcloud_instance_plan_changes WHERE id=?)`, changeID), &existing); err == nil {
			return existing, nil
		}
	}
	var activeResize int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM xcloud_tasks WHERE instance_id=? AND action='resize' AND status IN (?,?)`, instanceID, taskPending, taskRunning).Scan(&activeResize); err != nil {
		return controlTask{}, err
	}
	if observedCPU.Valid && observedMemory.Valid && math.Abs(observedCPU.Float64-sourceCPU) < 0.001 && int(observedMemory.Int64) == sourceMemory && activeResize == 0 {
		// No old resize is still able to change the runtime after this observed
		// source snapshot, so compensation can settle without another Agent call.
		if _, err = tx.ExecContext(ctx, `UPDATE xcloud_instances SET desired_generation=desired_generation+1 WHERE id=? AND owner_id=?`, instanceID, ownerID); err != nil {
			return controlTask{}, err
		}
		if err = cancelPlanChangeSettlementTx(ctx, tx, changeID, instanceID, ownerID, "用户取消；已观测到原套餐资源"); err != nil {
			return controlTask{}, err
		}
		if err = tx.Commit(); err != nil {
			return controlTask{}, err
		}
		return controlTask{}, nil
	}
	if _, err = tx.ExecContext(ctx, `UPDATE xcloud_instances SET desired_generation=desired_generation+1 WHERE id=? AND owner_id=? AND status IN ('running','stopped')`, instanceID, ownerID); err != nil {
		return controlTask{}, err
	}
	var generation int64
	if err = tx.QueryRowContext(ctx, `SELECT desired_generation FROM xcloud_instances WHERE id=? FOR UPDATE`, instanceID).Scan(&generation); err != nil {
		return controlTask{}, err
	}
	now := time.Now()
	payload, _ := json.Marshal(map[string]any{"changeId": changeID, "cpu": sourceCPU, "memoryMB": sourceMemory, "wasRunning": true, "compensation": true})
	task := controlTask{ID: newID("task"), InstanceID: instanceID, Action: "compensate-resize", IdempotencyKey: "plan-compensate:" + changeID, Status: taskPending, RunAfter: now, CreatedAt: now, UpdatedAt: now, DesiredGeneration: generation, Payload: payload}
	if _, err = tx.ExecContext(ctx, `UPDATE xcloud_tasks SET status=?,last_error='套餐变更已由用户取消，等待补偿任务',finished_at=NOW(),updated_at=NOW() WHERE instance_id=? AND action='resize' AND status=?`, taskCanceled, instanceID, taskPending); err != nil {
		return controlTask{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_tasks (id,instance_id,action,idempotency_key,status,attempts,run_after,created_at,updated_at,desired_generation,payload) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, task.ID, task.InstanceID, task.Action, task.IdempotencyKey, task.Status, 0, task.RunAfter, now, now, task.DesiredGeneration, task.Payload); err != nil {
		return controlTask{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE xcloud_instance_plan_changes SET status='needs_review',saga_status='compensating',fund_status='reserved',compensation_task_id=?,cancel_requested_at=NOW(),cancel_reason='user_requested',updated_at=NOW() WHERE id=?`, task.ID, changeID); err != nil {
		return controlTask{}, err
	}
	if err = writeAuditTx(ctx, tx, ownerID, "instance.plan_change.cancel", "plan_change", changeID, map[string]any{"instanceId": instanceID, "taskId": task.ID}); err != nil {
		return controlTask{}, err
	}
	if err = tx.Commit(); err != nil {
		return controlTask{}, err
	}
	appendTaskEvent(ctx, task.ID, "queued", "用户取消套餐变更：等待回退原套餐资源")
	return task, nil
}

func createPlanChange(ctx context.Context, ownerID, instanceID, targetPlanID, expectedCurrentPlan string, quote planChangeQuote) (planChangeRecord, controlTask, error) {
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
	seconds, _ := calculatePlanDelta(0, 0, expiry, time.Now())
	if quote.InstanceID != instanceID || quote.TargetPlanID != targetPlanID || len(quote.Items) == 0 {
		return planChangeRecord{}, controlTask{}, errors.New("套餐报价已变化，请重新报价")
	}
	effectiveAt := quote.OperationCutoffAt
	if effectiveAt.IsZero() || effectiveAt.After(time.Now()) {
		return planChangeRecord{}, controlTask{}, errors.New("套餐报价已变化，请重新报价")
	}
	delta := quote.DeltaFen
	var active int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM xcloud_tasks WHERE instance_id=? AND status IN (?,?) AND action IN ('create','retry-deploy','start','stop','update','restart','reinstall','destroy','purge','resize','compensate-resize')`, instanceID, taskPending, taskRunning).Scan(&active); err != nil {
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
	periodEnd := effectiveAt
	for _, item := range quote.Items {
		if item.ReplacementExpiresAt.After(periodEnd) {
			periodEnd = item.ReplacementExpiresAt
		}
	}
	tierMonths, tierBps, err := planChangeTargetRate(ctx, tx, targetPlanID, targetMonthly, effectiveAt, periodEnd, true)
	if err != nil {
		return planChangeRecord{}, controlTask{}, err
	}
	discountedMonthly := targetMonthly * tierBps / 10000
	changeID := newID("resize")
	taskID := newID("task")
	idem := "resize:" + instanceID + ":" + changeID
	before, _ := json.Marshal(map[string]any{"planId": currentPlan, "planName": currentName, "cpu": currentCPU, "memoryMB": currentMem})
	after, _ := json.Marshal(map[string]any{"planId": targetPlanID, "planName": targetName, "cpu": targetCPU, "memoryMB": targetMem})
	now := time.Now()
	// Reserve the exact target-package price before calling the Agent. The old
	// order credit is settled only after Agent success.
	charge := quote.ChargeFen
	refund := quote.RefundFen
	reservation := charge
	var pending string
	if reservation > 0 {
		var balance int
		if err = tx.QueryRowContext(ctx, `SELECT balance_fen FROM xcloud_wallets WHERE user_id=? FOR UPDATE`, ownerID).Scan(&balance); err != nil || balance < reservation {
			return planChangeRecord{}, controlTask{}, errors.New("XCoin 余额不足")
		}
		pending = newID("wal")
		if _, err = tx.ExecContext(ctx, `UPDATE xcloud_wallets SET balance_fen=balance_fen-?,updated_at=NOW() WHERE user_id=?`, reservation, ownerID); err != nil {
			return planChangeRecord{}, controlTask{}, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_wallet_entries (id,user_id,amount_fen,balance_after_fen,entry_type,note,actor_id,plan_change_id,business_key,created_at) SELECT ?,user_id,?,?,?, ?,?,?,?,NOW() FROM xcloud_wallets WHERE user_id=?`, pending, -reservation, balance-reservation, "plan_change_purchase_pending", "套餐变更新购暂扣", ownerID, changeID, "plan-change:purchase:"+changeID, ownerID); err != nil {
			return planChangeRecord{}, controlTask{}, err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_instance_plan_changes (id,instance_id,owner_id,source_plan_id,target_plan_id,source_cpu,source_memory_mb,target_cpu,target_memory_mb,remaining_seconds,delta_fen,charge_fen,reservation_fen,refund_fen,status,saga_status,fund_status,idempotency_key,pending_wallet_entry_id,before_snapshot,after_snapshot,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, changeID, instanceID, ownerID, currentPlan, targetPlanID, currentCPU, currentMem, targetCPU, targetMem, seconds, delta, charge, reservation, refund, "processing", "applying", "pending", idem, pending, before, after, now, now); err != nil {
		return planChangeRecord{}, controlTask{}, err
	}
	exchangeID := newID("exchange")
	priceSnapshot, _ := json.Marshal(quote)
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_order_exchanges (id,instance_id,owner_id,target_plan_id,effective_at,cutoff_at,task_id,status,refund_total_fen,charge_total_fen,net_settlement_fen,price_snapshot,idempotency_key,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,NOW())`, exchangeID, instanceID, ownerID, targetPlanID, effectiveAt, effectiveAt, taskID, "processing", quote.RefundFen, quote.ChargeFen, quote.ChargeFen-quote.RefundFen, priceSnapshot, idem); err != nil {
		return planChangeRecord{}, controlTask{}, err
	}
	for _, item := range quote.Items {
		var actualPaid int
		var startsAt, expiresAt time.Time
		if err = tx.QueryRowContext(ctx, `SELECT amount_fen,service_starts_at,expires_at FROM xcloud_orders WHERE id=? AND owner_id=? AND instance_id=? AND status=? FOR UPDATE`, item.SourceOrderID, ownerID, instanceID, orderActive).Scan(&actualPaid, &startsAt, &expiresAt); err != nil || actualPaid != item.ActualPaidFen || !startsAt.Equal(item.SourceStartsAt) || !expiresAt.Equal(item.SourceExpiresAt) {
			return planChangeRecord{}, controlTask{}, errors.New("订单服务期已变化，请重新报价")
		}
		expectedStart := effectiveAt
		if startsAt.After(expectedStart) {
			expectedStart = startsAt
		}
		expectedRefund := planChangeOrderRefundFen(actualPaid, startsAt, expiresAt, effectiveAt)
		expectedCharge := prorateMonthlyFen(discountedMonthly, expectedStart, expiresAt)
		if item.RefundFen != expectedRefund || item.ReplacementChargeFen != expectedCharge || item.TierMonths != tierMonths || item.TierDiscountBps != tierBps || !item.ReplacementStartsAt.Equal(expectedStart) || !item.ReplacementExpiresAt.Equal(expiresAt) {
			return planChangeRecord{}, controlTask{}, errors.New("套餐价格或服务期已变化，请重新报价")
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_order_exchange_items (id,exchange_id,source_order_id,source_plan_id,source_starts_at,source_expires_at,replacement_starts_at,replacement_expires_at,source_refund_fen,replacement_charge_fen,tier_discount_bps,tier_months,benefit_program_id,benefit_discount_fen,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,NOW())`, newID("exchangeitem"), exchangeID, item.SourceOrderID, item.SourcePlanID, item.SourceStartsAt, item.SourceExpiresAt, item.ReplacementStartsAt, item.ReplacementExpiresAt, item.RefundFen, item.ReplacementChargeFen, item.TierDiscountBps, item.TierMonths, nullableString(item.BenefitProgramID), item.DiscountAmountFen); err != nil {
			return planChangeRecord{}, controlTask{}, err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE xcloud_instance_plan_changes SET exchange_id=? WHERE id=?`, exchangeID, changeID); err != nil {
		return planChangeRecord{}, controlTask{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE xcloud_instances SET desired_generation=desired_generation+1 WHERE id=?`, instanceID); err != nil {
		return planChangeRecord{}, controlTask{}, err
	}
	var generation int64
	if err = tx.QueryRowContext(ctx, `SELECT desired_generation FROM xcloud_instances WHERE id=?`, instanceID).Scan(&generation); err != nil {
		return planChangeRecord{}, controlTask{}, err
	}
	payload, _ := json.Marshal(map[string]any{"changeId": changeID, "cpu": targetCPU, "memoryMB": targetMem, "wasRunning": status == "running"})
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_tasks (id,instance_id,action,idempotency_key,status,attempts,run_after,created_at,updated_at,desired_generation,payload) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, taskID, instanceID, "resize", idem, taskPending, 0, now, now, now, generation, payload); err != nil {
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
	return planChangeRecord{ID: changeID, InstanceID: instanceID, OwnerID: ownerID, SourcePlanID: currentPlan, TargetPlanID: targetPlanID, SourceCPU: currentCPU, SourceMemoryMB: currentMem, TargetCPU: targetCPU, TargetMemoryMB: targetMem, RemainingSeconds: seconds, DeltaFen: delta, ChargeFen: charge, RefundFen: refund, Status: "processing", SagaStatus: "applying", TaskID: taskID, IdempotencyKey: idem, PendingWalletEntryID: pending, CreatedAt: now, UpdatedAt: now}, controlTask{ID: taskID, InstanceID: instanceID, Action: "resize", IdempotencyKey: idem, Status: taskPending, RunAfter: now, CreatedAt: now, UpdatedAt: now, DesiredGeneration: generation, Payload: payload}, nil
}

func markPlanChangeBlocked(ctx context.Context, changeID, message string) {
	_, _ = instanceDB.ExecContext(ctx, `UPDATE xcloud_instance_plan_changes SET status='needs_review',saga_status='observing',fund_status='blocked',agent_verify_status='unavailable',agent_verified_at=NOW(),agent_verify_result=JSON_OBJECT('status','unknown'),agent_verify_error=?,updated_at=NOW() WHERE id=? AND status='processing'`, truncateError(message), changeID)
}

// markResizeTaskObserving translates an uncertain Agent result into the
// independently recoverable plan-change saga. It intentionally does not
// release the reservation: only an exact source observation may do that.
func markResizeTaskObserving(ctx context.Context, task controlTask, message string) {
	if task.Action != "resize" {
		return
	}
	var payload struct {
		ChangeID string `json:"changeId"`
	}
	if json.Unmarshal(task.Payload, &payload) == nil && payload.ChangeID != "" {
		markPlanChangeBlocked(ctx, payload.ChangeID, message)
	}
}

// reconcileBlockedPlanChanges keeps verification automatic after a transient
// node or tunnel outage.  It never guesses about money: the existing
// reconciliation only settles when the Agent reports exactly the requested
// resources, or refunds when it reports exactly the previous resources.
func reconcileBlockedPlanChanges(ctx context.Context) {
	if instanceDB == nil {
		return
	}
	rows, err := instanceDB.QueryContext(ctx, `SELECT `+taskSelectFields+`
		FROM xcloud_tasks t
		JOIN xcloud_instance_plan_changes p ON p.task_id=t.id
		WHERE t.action='resize' AND t.status=? AND p.status='needs_review'
		ORDER BY p.updated_at ASC LIMIT 100`, taskReview)
	if err != nil {
		log.Printf("load blocked plan changes: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var task controlTask
		if err := scanControlTask(rows, &task); err != nil {
			continue
		}
		reconcileRecoveredPlanChange(ctx, task)
	}
}

// resumeBlockedPlanChange restores a blocked settlement only after an exact
// Agent inspection has made its result deterministic. A freshly recovered
// task may still be processing rather than blocked; that state is already
// eligible for the same deterministic settlement. The conditional update
// prevents an automatic check from racing an administrator's decision.
func resumeBlockedPlanChange(ctx context.Context, changeID string) bool {
	result, err := instanceDB.ExecContext(ctx, `UPDATE xcloud_instance_plan_changes
		SET status='processing',agent_verify_error=NULL,updated_at=NOW()
		WHERE id=? AND (status='processing' OR (status='needs_review' AND fund_status='blocked'))`, changeID)
	if err != nil {
		return false
	}
	affected, _ := result.RowsAffected()
	return affected == 1
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
	_, _ = instanceDB.ExecContext(ctx, `UPDATE xcloud_instances SET observed_cpu=?,observed_memory_mb=?,observed_runtime_status='observed',observed_at=NOW() WHERE id=?`, actualCPU, actualMemory, task.InstanceID)
	_, _ = instanceDB.ExecContext(ctx, `UPDATE xcloud_instance_plan_changes SET observed_cpu=?,observed_memory_mb=?,observed_at=NOW(),updated_at=NOW() WHERE id=?`, actualCPU, actualMemory, payload.ChangeID)
	if math.Abs(actualCPU-targetCPU) < 0.001 && actualMemory == targetMemory {
		if !resumeBlockedPlanChange(ctx, payload.ChangeID) {
			return
		}
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
		if !resumeBlockedPlanChange(ctx, payload.ChangeID) {
			return
		}
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
	var id, owner, target, pendingEntry, exchangeID string
	var reservation int
	var status string
	if err = tx.QueryRowContext(ctx, `SELECT id,owner_id,target_plan_id,reservation_fen,status,COALESCE(pending_wallet_entry_id,''),COALESCE(exchange_id,'') FROM xcloud_instance_plan_changes WHERE id=? FOR UPDATE`, p.ChangeID).Scan(&id, &owner, &target, &reservation, &status, &pendingEntry, &exchangeID); err != nil {
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
	if err = tx.QueryRowContext(ctx, `SELECT status FROM xcloud_instances WHERE id=? FOR UPDATE`, task.InstanceID).Scan(&currentStatus); err != nil {
		return err
	}
	spec := fmt.Sprintf("%g 核 / %d GB", p.CPU, p.MemoryMB/1024)
	result, err := tx.ExecContext(ctx, `UPDATE xcloud_instances SET cpu=?,memory_mb=?,spec=? WHERE id=? AND status IN ('running','stopped')`, p.CPU, p.MemoryMB, spec, task.InstanceID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return errors.New("实例状态已变化，套餐变更未提交")
	}
	if exchangeID == "" {
		return errors.New("套餐变更缺少订单替换记录")
	}
	var balance int
	if err = tx.QueryRowContext(ctx, `SELECT balance_fen FROM xcloud_wallets WHERE user_id=? FOR UPDATE`, owner).Scan(&balance); err != nil {
		return err
	}
	// The pre-Agent reservation guarantees funds but is not an order payment.
	// Release it first, then write one immutable purchase entry per replacement
	// order and one refund entry per source order.
	if reservation > 0 {
		balance += reservation
		if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_wallet_entries (id,user_id,amount_fen,balance_after_fen,entry_type,note,actor_id,plan_change_id,business_key,created_at) VALUES (?,?,?,?,?,?,?,?,?,NOW())`, newID("wal"), owner, reservation, balance, "plan_change_reservation_release", "套餐变更新购暂扣释放", "system", p.ChangeID, "plan-change:release:"+p.ChangeID); err != nil {
			return err
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT item.source_order_id,item.source_refund_fen,item.replacement_charge_fen,item.replacement_starts_at,item.replacement_expires_at,item.tier_discount_bps,item.tier_months,COALESCE(item.benefit_program_id,''),item.benefit_discount_fen
		FROM xcloud_order_exchange_items item WHERE item.exchange_id=? FOR UPDATE`, exchangeID)
	if err != nil {
		return err
	}
	type exchangeSettlementItem struct {
		sourceID, benefitProgramID                          string
		sourceRefund, charge, tierBps, tierMonths, discount int
		startsAt, expiresAt                                 time.Time
	}
	items := []exchangeSettlementItem{}
	for rows.Next() {
		var item exchangeSettlementItem
		if err = rows.Scan(&item.sourceID, &item.sourceRefund, &item.charge, &item.startsAt, &item.expiresAt, &item.tierBps, &item.tierMonths, &item.benefitProgramID, &item.discount); err != nil {
			return err
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	for _, item := range items {
		sourceID, sourceRefund, charge := item.sourceID, item.sourceRefund, item.charge
		startsAt, expiresAt := item.startsAt, item.expiresAt
		tierBps, tierMonths := item.tierBps, item.tierMonths
		benefitProgramID, benefitDiscount := item.benefitProgramID, item.discount
		var imageID, imageVersion, imageDigest string
		if err = tx.QueryRowContext(ctx, `SELECT image_id,COALESCE(selected_image_version,''),COALESCE(selected_image_digest,'') FROM xcloud_orders WHERE id=? FOR UPDATE`, sourceID).Scan(&imageID, &imageVersion, &imageDigest); err != nil {
			return err
		}
		replacementID := newID("ord")
		purchaseEntryID := newID("wal")
		if balance < charge {
			return errors.New("套餐变更暂扣金额不足，请人工复核")
		}
		balance -= charge
		if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_wallet_entries (id,user_id,amount_fen,balance_after_fen,entry_type,note,actor_id,order_id,plan_change_id,business_key,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, purchaseEntryID, owner, -charge, balance, "plan_change_purchase", "套餐变更新购 "+replacementID, owner, replacementID, p.ChangeID, "plan-change:purchase:"+replacementID, time.Now()); err != nil {
			return err
		}
		newTierSnapshot, _ := json.Marshal(map[string]any{"months": tierMonths, "tierDiscountBps": tierBps, "source": "plan_change_current_price"})
		if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_orders (
			id,owner_id,plan_id,image_id,instance_id,amount_fen,list_amount_fen,discount_amount_fen,
			benefit_snapshot,bonus_days,status,payment_note,payment_source,exchange_id,replaces_order_id,
			order_role,price_tier_snapshot,wallet_entry_id,selected_image_version,selected_image_digest,service_starts_at,
			expires_at,created_at,updated_at
		) VALUES (?,?,?,?,?,?,?,?,NULL,0,?,?,?,?,?,?,?,?,?,?,?,?,NOW(),NOW())`,
			replacementID, owner, target, imageID, task.InstanceID, charge, charge+benefitDiscount, benefitDiscount,
			orderActive, "套餐变更替换订单", "wallet", exchangeID, sourceID, "replacement", newTierSnapshot, purchaseEntryID,
			nullableString(imageVersion), nullableString(imageDigest), startsAt, expiresAt); err != nil {
			return err
		}
		q := benefitQuote{ListAmountFen: charge + benefitDiscount, DiscountAmountFen: benefitDiscount, AmountFen: charge, TierMonths: tierMonths, TierDiscountBps: tierBps}
		if err = consumePlanChangeAutomaticBenefitTx(ctx, tx, owner, replacementID, benefitProgramID, q); err != nil {
			return err
		}
		refundEntryID := ""
		if sourceRefund > 0 {
			refundEntryID = newID("wal")
			balance += sourceRefund
			if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_wallet_entries (id,user_id,amount_fen,balance_after_fen,entry_type,note,actor_id,order_id,plan_change_id,business_key,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,NOW())`, refundEntryID, owner, sourceRefund, balance, "plan_change_refund", "套餐变更旧订单退款 "+sourceID, "system", sourceID, p.ChangeID, "plan-change:refund:"+sourceID); err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, `UPDATE xcloud_orders SET status='exchanged',exchange_id=?,order_role='source',refunded_at=NOW(),refund_amount_fen=?,refund_wallet_entry_id=?,updated_at=NOW() WHERE id=? AND status=?`, exchangeID, sourceRefund, nullableString(refundEntryID), sourceID, orderActive); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE xcloud_order_exchange_items SET replacement_order_id=? WHERE exchange_id=? AND source_order_id=?`, replacementID, exchangeID, sourceID); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE xcloud_wallets SET balance_fen=?,updated_at=NOW() WHERE user_id=?`, balance, owner); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE xcloud_order_exchanges SET status='succeeded',completed_at=NOW() WHERE id=? AND status='processing'`, exchangeID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE xcloud_instance_plan_changes SET status='succeeded',saga_status='settled',fund_status='settled',settlement_wallet_entry_id=?,agent_verify_status='verified',agent_verified_at=NOW(),agent_verify_result=JSON_OBJECT('cpu',?,'memoryMB',?,'status','target'),updated_at=NOW(),completed_at=NOW() WHERE id=? AND status='processing'`, nullableString(pendingEntry), p.CPU, p.MemoryMB, p.ChangeID); err != nil {
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
	var owner, status, exchangeID string
	var reservationFen int
	if tx.QueryRowContext(ctx, `SELECT owner_id,reservation_fen,status,COALESCE(exchange_id,'') FROM xcloud_instance_plan_changes WHERE id=? FOR UPDATE`, p.ChangeID).Scan(&owner, &reservationFen, &status, &exchangeID) != nil || status != "processing" {
		return
	}
	var entryID string
	if reservationFen > 0 {
		var balance int
		if tx.QueryRowContext(ctx, `SELECT balance_fen FROM xcloud_wallets WHERE user_id=? FOR UPDATE`, owner).Scan(&balance) != nil {
			return
		}
		entryID = newID("wal")
		if _, err = tx.ExecContext(ctx, `UPDATE xcloud_wallets SET balance_fen=balance_fen+?,updated_at=NOW() WHERE user_id=?`, reservationFen, owner); err != nil {
			return
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_wallet_entries (id,user_id,amount_fen,balance_after_fen,entry_type,note,actor_id,plan_change_id,business_key,created_at) VALUES (?,?,?,?,?,?,?,?,?,NOW())`, entryID, owner, reservationFen, balance+reservationFen, "plan_change_reservation_release", "套餐变更失败退回暂扣", "system", p.ChangeID, "plan-change:refund:"+p.ChangeID); err != nil {
			return
		}
	}
	_, _ = tx.ExecContext(ctx, `UPDATE xcloud_instance_plan_changes SET status='failed',saga_status='cancelled',fund_status='released',settlement_wallet_entry_id=?,agent_verify_status='not_checked',agent_verify_result=JSON_OBJECT('status','agent_error'),error_message=?,updated_at=NOW(),completed_at=NOW() WHERE id=? AND status='processing'`, entryID, truncateError(cause.Error()), p.ChangeID)
	if exchangeID != "" {
		_, _ = tx.ExecContext(ctx, `UPDATE xcloud_order_exchanges SET status='failed',completed_at=NOW() WHERE id=? AND status='processing'`, exchangeID)
	}
	_ = tx.Commit()
}

// completePlanChangeCompensation commits the user-requested rollback only
// after the Agent has applied the source resources under the latest intent
// generation.
func completePlanChangeCompensation(ctx context.Context, task controlTask) error {
	var payload struct {
		ChangeID string  `json:"changeId"`
		CPU      float64 `json:"cpu"`
		MemoryMB int     `json:"memoryMB"`
	}
	if err := json.Unmarshal(task.Payload, &payload); err != nil || payload.ChangeID == "" {
		return errors.New("套餐补偿任务缺少变更信息")
	}
	tx, err := beginSerializableTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var owner string
	if err = tx.QueryRowContext(ctx, `SELECT owner_id FROM xcloud_instance_plan_changes WHERE id=? AND instance_id=? AND saga_status='compensating' FOR UPDATE`, payload.ChangeID, task.InstanceID).Scan(&owner); err != nil {
		return err
	}
	if task.DesiredGeneration > 0 {
		var current int64
		if err = tx.QueryRowContext(ctx, `SELECT desired_generation FROM xcloud_instances WHERE id=? FOR UPDATE`, task.InstanceID).Scan(&current); err != nil {
			return err
		}
		if current != task.DesiredGeneration {
			return errors.New("补偿任务已被更新的实例意图取代")
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE xcloud_instances SET cpu=?,memory_mb=?,spec=? WHERE id=? AND status IN ('running','stopped')`, payload.CPU, payload.MemoryMB, fmt.Sprintf("%g 核 / %d GB", payload.CPU, payload.MemoryMB/1024), task.InstanceID); err != nil {
		return err
	}
	if err = cancelPlanChangeSettlementTx(ctx, tx, payload.ChangeID, task.InstanceID, owner, "用户取消后已回退原套餐资源"); err != nil {
		return err
	}
	return tx.Commit()
}

func cancelPlanChangeSettlementTx(ctx context.Context, tx *sql.Tx, changeID, instanceID, ownerID, detail string) error {
	var reservation int
	var exchangeID string
	var saga string
	if err := tx.QueryRowContext(ctx, `SELECT reservation_fen,COALESCE(exchange_id,''),COALESCE(saga_status,'') FROM xcloud_instance_plan_changes WHERE id=? AND instance_id=? AND owner_id=? FOR UPDATE`, changeID, instanceID, ownerID).Scan(&reservation, &exchangeID, &saga); err != nil {
		return err
	}
	if saga == "cancelled" {
		return nil
	}
	var entryID any
	if reservation > 0 {
		var balance int
		if err := tx.QueryRowContext(ctx, `SELECT balance_fen FROM xcloud_wallets WHERE user_id=? FOR UPDATE`, ownerID).Scan(&balance); err != nil {
			return err
		}
		entryID = newID("wal")
		if _, err := tx.ExecContext(ctx, `UPDATE xcloud_wallets SET balance_fen=balance_fen+?,updated_at=NOW() WHERE user_id=?`, reservation, ownerID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO xcloud_wallet_entries (id,user_id,amount_fen,balance_after_fen,entry_type,note,actor_id,plan_change_id,business_key,created_at) VALUES (?,?,?,?,?,?,?,?,?,NOW())`, entryID, ownerID, reservation, balance+reservation, "plan_change_reservation_release", "用户取消套餐变更，释放暂扣", ownerID, changeID, "plan-change:cancel:"+changeID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE xcloud_instance_plan_changes SET status='failed',saga_status='cancelled',fund_status='released',settlement_wallet_entry_id=?,agent_verify_status='verified_old',agent_verify_error=NULL,error_message=?,updated_at=NOW(),completed_at=NOW() WHERE id=? AND saga_status<>'cancelled'`, entryID, detail, changeID); err != nil {
		return err
	}
	if exchangeID != "" {
		if _, err := tx.ExecContext(ctx, `UPDATE xcloud_order_exchanges SET status='cancelled',completed_at=NOW() WHERE id=? AND status='processing'`, exchangeID); err != nil {
			return err
		}
	}
	return nil
}
