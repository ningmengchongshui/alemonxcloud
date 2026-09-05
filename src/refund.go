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
	OrderID         string    `json:"orderId"`
	Eligible        bool      `json:"eligible"`
	Reason          string    `json:"reason,omitempty"`
	TotalDays       int       `json:"totalDays"`
	RemainingDays   int       `json:"remainingDays"`
	PrepaidDays     int       `json:"prepaidDays"`
	RefundableDays  int       `json:"refundableDays"`
	RefundAmountFen int       `json:"refundAmountFen"`
	ServiceEndsAt   time.Time `json:"serviceEndsAt"`
	DataPurgeAt     time.Time `json:"dataPurgeAt"`
}

type refundSegment struct {
	ID        string
	Status    string
	AmountFen int
	Start     time.Time
	End       time.Time
	Source    string
}

func quoteRefund(segments []refundSegment, orderID string, now time.Time) (refundQuote, time.Duration, int, error) {
	if err := validateRefundSegments(segments); err != nil {
		return refundQuote{}, 0, 0, err
	}
	for index, item := range segments {
		if item.ID != orderID {
			continue
		}
		if item.Source != "wallet" {
			return refundQuote{}, 0, 0, errors.New("仅钱包购买的订单支持自助退款")
		}
		if item.Status == orderRefund {
			return refundQuote{}, 0, 0, errors.New("该订单已退款")
		}
		if item.Status != orderActive {
			return refundQuote{}, 0, 0, errors.New("订单当前不可退款")
		}
		if !item.End.After(item.Start) {
			return refundQuote{}, 0, 0, errors.New("订单服务期无效")
		}

		eligibleFrom := item.Start
		if now.After(eligibleFrom) {
			eligibleFrom = now
		}
		totalDays := int(item.End.Sub(item.Start) / refundDay)
		remainingDays := int(item.End.Sub(eligibleFrom) / refundDay)
		if totalDays < 1 || remainingDays <= refundPrepaidDays {
			return refundQuote{}, 0, 0, errors.New("剩余服务期不足 3 个完整自然日，暂不可退款")
		}
		refundableDays := remainingDays - refundPrepaidDays
		amount := int(int64(item.AmountFen) * int64(refundableDays) / int64(totalDays))
		if amount < 1 {
			return refundQuote{}, 0, 0, errors.New("可退款金额不足 0.01 XCoin")
		}
		shift := time.Duration(refundableDays) * refundDay
		finalEnd := segments[len(segments)-1].End
		if index == len(segments)-1 {
			finalEnd = item.End
		}
		finalEnd = finalEnd.Add(-shift)
		return refundQuote{
			OrderID:         item.ID,
			Eligible:        true,
			TotalDays:       totalDays,
			RemainingDays:   remainingDays,
			PrepaidDays:     refundPrepaidDays,
			RefundableDays:  refundableDays,
			RefundAmountFen: amount,
			ServiceEndsAt:   finalEnd,
			DataPurgeAt:     finalEnd.Add(refundRetentionDays * refundDay),
		}, shift, index, nil
	}
	return refundQuote{}, 0, 0, errors.New("订单不存在或不具备可退款服务期")
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
	statement := `SELECT id,status,amount_fen,service_starts_at,expires_at,COALESCE(payment_source,'') FROM xcloud_orders WHERE instance_id=? AND owner_id=? AND service_starts_at IS NOT NULL AND expires_at IS NOT NULL ORDER BY service_starts_at,id`
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
	segments, err := refundSegments(ctx, instanceDB, instanceID, ownerID, false)
	if err != nil {
		return refundQuote{}, err
	}
	quote, _, _, err := quoteRefund(segments, orderID, time.Now())
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
	var lockedInstanceID string
	if err = tx.QueryRowContext(ctx, `SELECT id FROM xcloud_instances WHERE id=? AND owner_id=? FOR UPDATE`, instanceID, ownerID).Scan(&lockedInstanceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return refundQuote{}, walletEntry{}, errors.New("实例不存在或已清理")
		}
		return refundQuote{}, walletEntry{}, err
	}
	segments, err := refundSegments(ctx, tx, instanceID, ownerID, true)
	if err != nil {
		return refundQuote{}, walletEntry{}, err
	}
	now := time.Now()
	quote, shift, targetIndex, err := quoteRefund(segments, orderID, now)
	if err != nil {
		return refundQuote{}, walletEntry{}, err
	}

	var balance int
	if err = tx.QueryRowContext(ctx, `SELECT balance_fen FROM xcloud_wallets WHERE user_id=? FOR UPDATE`, ownerID).Scan(&balance); err != nil {
		return refundQuote{}, walletEntry{}, errors.New("钱包账户不可用，请重新登录后重试")
	}
	nextBalance := balance + quote.RefundAmountFen
	entry := walletEntry{ID: newID("wal"), UserID: ownerID, AmountFen: quote.RefundAmountFen, BalanceAfterFen: nextBalance, Type: "refund", Note: "订单退款 " + orderID, ActorID: ownerID, OrderID: orderID, CreatedAt: now}
	if _, err = tx.ExecContext(ctx, `UPDATE xcloud_wallets SET balance_fen=?,updated_at=? WHERE user_id=?`, nextBalance, now, ownerID); err != nil {
		return refundQuote{}, walletEntry{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_wallet_entries (id,user_id,amount_fen,balance_after_fen,entry_type,note,actor_id,order_id,created_at) VALUES (?,?,?,?,?,?,?,?,?)`, entry.ID, entry.UserID, entry.AmountFen, entry.BalanceAfterFen, entry.Type, entry.Note, entry.ActorID, entry.OrderID, entry.CreatedAt); err != nil {
		return refundQuote{}, walletEntry{}, err
	}

	for index, segment := range segments {
		if index < targetIndex {
			continue
		}
		start, end := segment.Start, segment.End.Add(-shift)
		if index > targetIndex {
			start = start.Add(-shift)
		}
		if index == targetIndex {
			if _, err = tx.ExecContext(ctx, `UPDATE xcloud_orders SET status=?,expires_at=?,refunded_at=?,refund_amount_fen=?,refund_wallet_entry_id=?,updated_at=? WHERE id=? AND status=?`, orderRefund, end, now, quote.RefundAmountFen, entry.ID, now, segment.ID, orderActive); err != nil {
				return refundQuote{}, walletEntry{}, err
			}
			continue
		}
		if _, err = tx.ExecContext(ctx, `UPDATE xcloud_orders SET service_starts_at=?,expires_at=?,updated_at=? WHERE id=?`, start, end, now, segment.ID); err != nil {
			return refundQuote{}, walletEntry{}, err
		}
	}
	var runtimeStatus string
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(runtime_status,status) FROM xcloud_instances WHERE id=? FOR UPDATE`, instanceID).Scan(&runtimeStatus); err != nil {
		return refundQuote{}, walletEntry{}, err
	}
	if runtimeStatus != "running" && runtimeStatus != "stopped" {
		runtimeStatus = "stopped"
	}
	if _, err = tx.ExecContext(ctx, `UPDATE xcloud_instances SET expires_at=?,status='destroy_scheduled',runtime_status=?,destroy_at=?,destroy_reason='refund',destroyed_at=NULL,purge_at=NULL,retention_days=? WHERE id=?`, quote.ServiceEndsAt, runtimeStatus, quote.ServiceEndsAt, refundRetentionDays, instanceID); err != nil {
		return refundQuote{}, walletEntry{}, err
	}
	if err = writeAuditTx(ctx, tx, ownerID, "order.refund", "order", orderID, map[string]any{"refundAmountFen": quote.RefundAmountFen, "refundableDays": quote.RefundableDays, "serviceEndsAt": quote.ServiceEndsAt, "walletEntryId": entry.ID}); err != nil {
		return refundQuote{}, walletEntry{}, err
	}
	if err = tx.Commit(); err != nil {
		return refundQuote{}, walletEntry{}, err
	}
	_ = createNotification(ctx, ownerID, "refund", "订单退款已到账", fmt.Sprintf("已退回 %.2f XCoin。服务将继续可用至 %s；届时销毁容器资源，数据再保留 30 天。", float64(entry.AmountFen)/100, quote.ServiceEndsAt.Format("2006-01-02 15:04")), map[string]any{"orderId": orderID, "walletEntryId": entry.ID, "serviceEndsAt": quote.ServiceEndsAt, "dataPurgeAt": quote.DataPurgeAt})
	return quote, entry, nil
}
