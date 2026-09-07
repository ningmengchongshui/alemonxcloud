package cloud

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	refundPrepaidDays   = 3
	refundRetentionDays = 30
	refundDay           = 24 * time.Hour
)

// refundQuote is computed exclusively by the control plane.  The client only
// displays it; confirmation always repeats the same calculation under locks.
type refundQuote struct {
	OrderID             string    `json:"orderId"`
	Eligible            bool      `json:"eligible"`
	Reason              string    `json:"reason,omitempty"`
	TotalDays           int       `json:"totalDays"`
	RemainingDays       int       `json:"remainingDays"`
	PrepaidDays         int       `json:"prepaidDays"`
	RefundableDays      int       `json:"refundableDays"`
	BaseRefundAmountFen int       `json:"baseRefundAmountFen"`
	RefundAmountFen     int       `json:"refundAmountFen"`
	ServiceEndsAt       time.Time `json:"serviceEndsAt"`
	DataPurgeAt         time.Time `json:"dataPurgeAt"`
}

type refundSegment struct {
	ID        string
	Status    string
	AmountFen int
	Start     time.Time
	End       time.Time
	Source    string
}

// refundCutoffAt keeps the operation day and the following two natural days.
// A request on any time of day is therefore refundable from the fourth
// Asia/Shanghai calendar day at 00:00, never from an arbitrary 72-hour point.
func refundCutoffAt(now time.Time) time.Time {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	local := now.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day()+refundPrepaidDays, 0, 0, 0, 0, loc)
}

// quoteInstanceRefund settles every active service order for an instance at
// one cutoff. It deliberately does not look at exchanged source orders:
// replacement orders are now the sole financial fact after a plan change.
func quoteInstanceRefund(segments []refundSegment, orderID string, now time.Time) (refundQuote, error) {
	if err := validateRefundSegments(segments); err != nil {
		return refundQuote{}, err
	}
	cutoff := refundCutoffAt(now)
	var amount, totalDays, remainingDays, refundableDays int
	active := 0
	for _, segment := range segments {
		if segment.Status != orderActive {
			continue
		}
		active++
		if segment.Source != "wallet" {
			return refundQuote{}, errors.New("仅钱包购买的订单支持自助退款")
		}
		if !segment.End.After(segment.Start) {
			return refundQuote{}, errors.New("订单服务期无效")
		}
		totalDays += int(segment.End.Sub(segment.Start) / refundDay)
		if !segment.End.After(cutoff) {
			continue
		}
		start := cutoff
		if segment.Start.After(start) {
			start = segment.Start
		}
		days := int(segment.End.Sub(start) / refundDay)
		if days <= 0 {
			continue
		}
		remainingDays += days
		refundableDays += days
		amount += prorateFen(segment.AmountFen, segment.Start, segment.End, start)
	}
	if active == 0 || amount < 1 {
		return refundQuote{}, errors.New("剩余服务期不足 3 个自然日，暂不可退款")
	}
	return refundQuote{
		OrderID: orderID, Eligible: true, TotalDays: totalDays,
		RemainingDays: remainingDays, PrepaidDays: refundPrepaidDays,
		RefundableDays: refundableDays, BaseRefundAmountFen: amount,
		RefundAmountFen: amount, ServiceEndsAt: cutoff,
		DataPurgeAt: cutoff.Add(refundRetentionDays * refundDay),
	}, nil
}

func validateRefundSegments(segments []refundSegment) error {
	for index, item := range segments {
		if !item.End.After(item.Start) {
			return errors.New("订单服务期无效，请提交工单处理")
		}
		if index > 0 && !item.Start.Equal(segments[index-1].End) {
			return errors.New("订单服务期未连续衔接，请提交工单处理")
		}
	}
	return nil
}

func refundSegments(ctx context.Context, queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, instanceID, ownerID string, lock bool) ([]refundSegment, error) {
	// Exchanged source orders intentionally overlap their replacement service
	// windows in history. Refund settlement must therefore read only the
	// current active chain, not every historical order attached to the instance.
	statement := `SELECT id,status,amount_fen,service_starts_at,expires_at,COALESCE(payment_source,'') FROM xcloud_orders WHERE instance_id=? AND owner_id=? AND status='active' AND service_starts_at IS NOT NULL AND expires_at IS NOT NULL ORDER BY service_starts_at,id`
	if lock {
		statement += " FOR UPDATE"
	}
	rows, err := queryer.QueryContext(ctx, statement, instanceID, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []refundSegment{}
	for rows.Next() {
		var item refundSegment
		if err := rows.Scan(&item.ID, &item.Status, &item.AmountFen, &item.Start, &item.End, &item.Source); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func refundAmountForSegment(segment refundSegment, cutoff time.Time) int {
	if segment.AmountFen <= 0 || !segment.End.After(cutoff) {
		return 0
	}
	start := cutoff
	if segment.Start.After(start) {
		start = segment.Start
	}
	return prorateFen(segment.AmountFen, segment.Start, segment.End, start)
}

func refundQuoteForOrder(ctx context.Context, ownerID, orderID string) (refundQuote, error) {
	var instanceID string
	if err := instanceDB.QueryRowContext(ctx, `SELECT COALESCE(instance_id,'') FROM xcloud_orders WHERE id=? AND owner_id=?`, orderID, ownerID).Scan(&instanceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return refundQuote{}, errors.New("订单不存在")
		}
		return refundQuote{}, err
	}
	if instanceID == "" {
		return refundQuote{}, errors.New("订单尚未生成可退款服务")
	}
	var destroyReason string
	if err := instanceDB.QueryRowContext(ctx, `SELECT COALESCE(destroy_reason,'') FROM xcloud_instances WHERE id=? AND owner_id=?`, instanceID, ownerID).Scan(&destroyReason); err == nil && destroyReason == "user_no_refund" {
		return refundQuote{OrderID: orderID, Eligible: false, Reason: "该实例已由用户选择直接销毁，不能申请退款", PrepaidDays: refundPrepaidDays}, nil
	}
	segments, err := refundSegments(ctx, instanceDB, instanceID, ownerID, false)
	if err != nil {
		return refundQuote{}, err
	}
	quote, err := quoteInstanceRefund(segments, orderID, time.Now())
	if err != nil {
		return refundQuote{OrderID: orderID, Eligible: false, Reason: err.Error(), PrepaidDays: refundPrepaidDays}, nil
	}
	return quote, nil
}

func refundOrder(ctx context.Context, ownerID, orderID string) (refundQuote, walletEntry, error) {
	tx, err := beginSerializableTx(ctx)
	if err != nil {
		return refundQuote{}, walletEntry{}, err
	}
	defer tx.Rollback()

	var instanceID string
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(instance_id,'') FROM xcloud_orders WHERE id=? AND owner_id=? FOR UPDATE`, orderID, ownerID).Scan(&instanceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return refundQuote{}, walletEntry{}, errors.New("订单不存在")
		}
		return refundQuote{}, walletEntry{}, err
	}
	if instanceID == "" {
		return refundQuote{}, walletEntry{}, errors.New("订单尚未生成可退款服务")
	}
	segments, err := refundSegments(ctx, tx, instanceID, ownerID, true)
	if err != nil {
		return refundQuote{}, walletEntry{}, err
	}
	now := time.Now()
	quote, err := quoteInstanceRefund(segments, orderID, now)
	if err != nil {
		return refundQuote{}, walletEntry{}, err
	}

	settlementID := newID("refund")
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_instance_refund_settlements (id,instance_id,owner_id,requested_order_id,total_refund_fen,service_ends_at,data_purge_at,idempotency_key,created_at) VALUES (?,?,?,?,?,?,?,?,?)`, settlementID, instanceID, ownerID, orderID, quote.RefundAmountFen, quote.ServiceEndsAt, quote.DataPurgeAt, "instance-refund:"+instanceID+":"+orderID, now); err != nil {
		return refundQuote{}, walletEntry{}, err
	}
	var balance int
	if err = tx.QueryRowContext(ctx, `SELECT balance_fen FROM xcloud_wallets WHERE user_id=? FOR UPDATE`, ownerID).Scan(&balance); err != nil {
		return refundQuote{}, walletEntry{}, errors.New("钱包账户不可用，请重新登录后重试")
	}
	entry := walletEntry{ID: settlementID, UserID: ownerID, AmountFen: quote.RefundAmountFen, BalanceAfterFen: balance + quote.RefundAmountFen, Type: "refund", Note: "实例退款 " + orderID, ActorID: ownerID, OrderID: orderID, CreatedAt: now}
	currentBalance := balance
	for _, segment := range segments {
		if segment.Status != orderActive {
			continue
		}
		refundAmount := refundAmountForSegment(segment, quote.ServiceEndsAt)
		// Future renewal orders have not started. Preserve a non-negative service
		// interval in history while recording that their full paid amount was
		// refunded as part of this instance-level settlement.
		end := quote.ServiceEndsAt
		if segment.Start.After(end) {
			end = segment.Start
		}
		var walletEntryID any
		if refundAmount > 0 {
			currentBalance += refundAmount
			walletEntryID = newID("wal")
			if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_wallet_entries (id,user_id,amount_fen,balance_after_fen,entry_type,note,actor_id,order_id,business_key,created_at) VALUES (?,?,?,?,?,?,?,?,?,?)`, walletEntryID, ownerID, refundAmount, currentBalance, "refund", "实例退款 "+segment.ID, ownerID, segment.ID, "instance-refund:"+settlementID+":"+segment.ID, now); err != nil {
				return refundQuote{}, walletEntry{}, err
			}
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_instance_refund_settlement_items (id,settlement_id,order_id,refund_fen,wallet_entry_id,created_at) VALUES (?,?,?,?,?,?)`, newID("refunditem"), settlementID, segment.ID, refundAmount, walletEntryID, now); err != nil {
			return refundQuote{}, walletEntry{}, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE xcloud_orders SET status=?,expires_at=?,refunded_at=?,refund_amount_fen=?,refund_wallet_entry_id=?,updated_at=? WHERE id=? AND status=?`, orderRefund, end, now, refundAmount, walletEntryID, now, segment.ID, orderActive); err != nil {
			return refundQuote{}, walletEntry{}, err
		}
	}
	if currentBalance != balance+quote.RefundAmountFen {
		return refundQuote{}, walletEntry{}, errors.New("退款明细与汇总金额不一致")
	}
	if _, err = tx.ExecContext(ctx, `UPDATE xcloud_wallets SET balance_fen=?,updated_at=? WHERE user_id=?`, currentBalance, now, ownerID); err != nil {
		return refundQuote{}, walletEntry{}, err
	}
	// A user may manually destroy an instance before requesting a refund. The
	// financial settlement is still determined by the immutable order service
	// periods; there is simply no remaining runtime to schedule for destruction.
	// A missing row is treated the same way so an already-purged legacy record
	// cannot trap a legitimate wallet refund.
	instanceAlreadyGone := false
	var runtimeStatus, instanceStatus, destroyReason string
	err = tx.QueryRowContext(ctx, `SELECT status,COALESCE(runtime_status,status),COALESCE(destroy_reason,'') FROM xcloud_instances WHERE id=? AND owner_id=? FOR UPDATE`, instanceID, ownerID).Scan(&instanceStatus, &runtimeStatus, &destroyReason)
	if errors.Is(err, sql.ErrNoRows) {
		instanceAlreadyGone = true
	} else if err != nil {
		return refundQuote{}, walletEntry{}, err
	} else if destroyReason == "user_no_refund" {
		return refundQuote{}, walletEntry{}, errors.New("该实例已由用户选择直接销毁，不能申请退款")
	} else if instanceStatus == "destroyed" || instanceStatus == "purged" {
		instanceAlreadyGone = true
	} else if instanceStatus != "running" && instanceStatus != "stopped" {
		return refundQuote{}, walletEntry{}, errors.New("实例当前不可退款")
	} else {
		if runtimeStatus != "running" && runtimeStatus != "stopped" {
			runtimeStatus = "stopped"
		}
		changed, transitionErr := transitionInstance(ctx, tx, instanceID, []string{instanceStatus}, "destroy_scheduled", &runtimeStatus, "expires_at=?,destroy_at=?,destroy_reason='refund',destroyed_at=NULL,purge_at=NULL,retention_days=?", quote.ServiceEndsAt, quote.ServiceEndsAt, refundRetentionDays)
		if transitionErr != nil {
			return refundQuote{}, walletEntry{}, transitionErr
		}
		if !changed {
			return refundQuote{}, walletEntry{}, errInstanceStateConflict
		}
	}
	if err = writeAuditTx(ctx, tx, ownerID, "order.refund", "order", orderID, map[string]any{"refundSettlementId": settlementID, "refundAmountFen": quote.RefundAmountFen, "baseRefundAmountFen": quote.BaseRefundAmountFen, "refundableDays": quote.RefundableDays, "serviceEndsAt": quote.ServiceEndsAt}); err != nil {
		return refundQuote{}, walletEntry{}, err
	}
	if err = tx.Commit(); err != nil {
		return refundQuote{}, walletEntry{}, err
	}
	message := fmt.Sprintf("已退回 %.2f XCoin。服务将继续可用至 %s；届时销毁容器资源，数据再保留 30 天。", float64(entry.AmountFen)/100, quote.ServiceEndsAt.Format("2006-01-02 15:04"))
	if instanceAlreadyGone {
		message = fmt.Sprintf("已退回 %.2f XCoin。实例资源已由此前操作销毁或清理，本次退款不会再执行容器操作。", float64(entry.AmountFen)/100)
	}
	_ = createNotification(ctx, ownerID, "refund", "订单退款已到账", message, map[string]any{"orderId": orderID, "refundSettlementId": settlementID, "serviceEndsAt": quote.ServiceEndsAt, "dataPurgeAt": quote.DataPurgeAt, "instanceAlreadyGone": instanceAlreadyGone})
	return quote, entry, nil
}
