package cloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type couponBatch struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Status           string     `json:"status"`
	DistributionMode string     `json:"distributionMode"`
	DiscountType     string     `json:"discountType"`
	Scope            string     `json:"scope"`
	DiscountValue    int        `json:"discountValue"`
	MinAmountFen     int        `json:"minAmountFen"`
	MaxDiscountFen   int        `json:"maxDiscountFen"`
	IssueLimit       int        `json:"issueLimit"`
	PerUserLimit     int        `json:"perUserLimit"`
	IssuedCount      int        `json:"issuedCount"`
	PlanIDs          []string   `json:"planIDs"`
	ImageIDs         []string   `json:"imageIDs"`
	MonthValues      []string   `json:"monthValues"`
	StartsAt         *time.Time `json:"startsAt,omitempty"`
	EndsAt           *time.Time `json:"endsAt,omitempty"`
	CreatedBy        string     `json:"createdBy,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt,omitempty"`
}
type userCoupon struct {
	ID          string       `json:"id"`
	BatchID     string       `json:"batchId"`
	OwnerID     string       `json:"ownerId"`
	Status      string       `json:"status"`
	IssueSource string       `json:"issueSource"`
	IssuedBy    *string      `json:"issuedBy,omitempty"`
	ExpiresAt   *time.Time   `json:"expiresAt,omitempty"`
	UsedAt      *time.Time   `json:"usedAt,omitempty"`
	VoidedAt    *time.Time   `json:"voidedAt,omitempty"`
	OrderID     *string      `json:"orderId,omitempty"`
	CreatedAt   time.Time    `json:"createdAt"`
	Batch       *couponBatch `json:"batch,omitempty"`
}

func scanCouponBatch(row interface{ Scan(...any) error }) (couponBatch, error) {
	var v couponBatch
	var a, b, c []byte
	err := row.Scan(&v.ID, &v.Name, &v.Status, &v.DistributionMode, &v.DiscountType, &v.DiscountValue, &v.MinAmountFen, &v.MaxDiscountFen, &v.Scope, &a, &b, &c, &v.StartsAt, &v.EndsAt, &v.IssueLimit, &v.PerUserLimit, &v.IssuedCount, &v.CreatedBy, &v.CreatedAt, &v.UpdatedAt)
	if err == nil {
		_ = json.Unmarshal(a, &v.PlanIDs)
		_ = json.Unmarshal(b, &v.ImageIDs)
		_ = json.Unmarshal(c, &v.MonthValues)
	}
	return v, err
}

const couponBatchSelect = `SELECT id,name,status,distribution_mode,discount_type,discount_value,min_amount_fen,max_discount_fen,scope,COALESCE(plan_ids,JSON_ARRAY()),COALESCE(image_ids,JSON_ARRAY()),COALESCE(month_values,JSON_ARRAY()),starts_at,ends_at,issue_limit,per_user_limit,issued_count,created_by,created_at,updated_at FROM xcloud_coupon_batches`

func listCouponBatches(ctx context.Context, publicOnly bool) ([]couponBatch, error) {
	q := couponBatchSelect
	args := []any{}
	if publicOnly {
		q += ` WHERE status='active' AND distribution_mode='public' AND (starts_at IS NULL OR starts_at<=NOW()) AND (ends_at IS NULL OR ends_at>NOW())`
	}
	q += " ORDER BY created_at DESC"
	rows, err := instanceDB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []couponBatch{}
	for rows.Next() {
		v, e := scanCouponBatch(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func validCouponBatch(v couponBatch) error {
	if v.Name == "" || len(v.Name) > 128 {
		return errors.New("券批次名称长度应为 1 至 128")
	}
	if v.Status != "active" && v.Status != "paused" {
		return errors.New("券批次状态无效")
	}
	if v.DistributionMode != "public" && v.DistributionMode != "targeted" {
		return errors.New("发放方式无效")
	}
	if v.DiscountType != "fixed" && v.DiscountType != "percent" || v.DiscountValue <= 0 || v.DiscountType == "percent" && v.DiscountValue > 10000 {
		return errors.New("优惠规则无效")
	}
	if v.Scope != "purchase" && v.Scope != "renewal" && v.Scope != "both" || v.IssueLimit < 0 || v.PerUserLimit < 1 || v.MinAmountFen < 0 || v.MaxDiscountFen < 0 {
		return errors.New("券批次限制无效")
	}
	if v.StartsAt != nil && v.EndsAt != nil && !v.EndsAt.After(*v.StartsAt) {
		return errors.New("结束时间必须晚于开始时间")
	}
	return nil
}
func batchAsPromotion(v couponBatch, expires *time.Time) promotion {
	return promotion{ID: v.ID, Name: v.Name, Kind: "coupon_batch", Scope: v.Scope, DiscountType: v.DiscountType, DiscountValue: v.DiscountValue, MinAmountFen: v.MinAmountFen, MaxDiscountFen: v.MaxDiscountFen, PlanIDs: v.PlanIDs, ImageIDs: v.ImageIDs, MonthValues: v.MonthValues, EndsAt: expires, Enabled: true}
}
func listUserCoupons(ctx context.Context, ownerID string) ([]userCoupon, error) {
	rows, err := instanceDB.QueryContext(ctx, `SELECT id,batch_id,owner_id,status,issue_source,issued_by,expires_at,used_at,voided_at,order_id,created_at FROM xcloud_user_coupons WHERE owner_id=? ORDER BY created_at DESC`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []userCoupon{}
	for rows.Next() {
		var v userCoupon
		if e := rows.Scan(&v.ID, &v.BatchID, &v.OwnerID, &v.Status, &v.IssueSource, &v.IssuedBy, &v.ExpiresAt, &v.UsedAt, &v.VoidedAt, &v.OrderID, &v.CreatedAt); e != nil {
			return nil, e
		}
		if v.Status == "available" && v.ExpiresAt != nil && !time.Now().Before(*v.ExpiresAt) {
			v.Status = "expired"
		}
		if batch, batchErr := scanCouponBatch(instanceDB.QueryRowContext(ctx, couponBatchSelect+` WHERE id=?`, v.BatchID)); batchErr == nil {
			v.Batch = &batch
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func issueUserCoupons(ctx context.Context, tx *sql.Tx, batch couponBatch, ownerIDs []string, actor, mode string) (string, []map[string]any, error) {
	runID := newID("iss")
	if _, e := tx.ExecContext(ctx, `INSERT INTO xcloud_coupon_issuance_runs (id,batch_id,mode,actor_id,created_at) VALUES (?,?,?,?,NOW())`, runID, batch.ID, mode, actor); e != nil {
		return "", nil, e
	}
	out := []map[string]any{}
	for _, ownerID := range ownerIDs {
		item := map[string]any{"ownerId": ownerID, "status": "issued"}
		var used int
		_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM xcloud_user_coupons WHERE batch_id=? AND owner_id=?`, batch.ID, ownerID).Scan(&used)
		if used >= batch.PerUserLimit {
			item["status"] = "skipped"
			item["reason"] = "已达到每人限领"
		} else {
			r, e := tx.ExecContext(ctx, `UPDATE xcloud_coupon_batches SET issued_count=issued_count+1,updated_at=NOW() WHERE id=? AND (issue_limit=0 OR issued_count<issue_limit)`, batch.ID)
			if e != nil {
				return "", nil, e
			}
			n, _ := r.RowsAffected()
			if n != 1 {
				item["status"] = "failed"
				item["reason"] = "券库存不足"
			} else {
				id := newID("ucp")
				_, e = tx.ExecContext(ctx, `INSERT INTO xcloud_user_coupons (id,batch_id,owner_id,status,issued_by,issue_source,expires_at,created_at) VALUES (?,?,?,'available',?,?,?,NOW())`, id, batch.ID, ownerID, actor, mode, batch.EndsAt)
				if e != nil {
					return "", nil, e
				}
				item["couponId"] = id
			}
		}
		_, e := tx.ExecContext(ctx, `INSERT INTO xcloud_coupon_issuance_items (id,run_id,owner_id,user_coupon_id,status,reason,created_at) VALUES (?,?,?,?,?,?,NOW())`, newID("isi"), runID, ownerID, item["couponId"], item["status"], item["reason"])
		if e != nil {
			return "", nil, e
		}
		out = append(out, item)
	}
	return runID, out, nil
}
func couponBatchCandidates(ctx context.Context, ownerID, scope, planID, imageID string, months, list int, tx *sql.Tx) ([]priceCandidate, map[string]struct {
	p promotion
	c coupon
}, error) {
	q := `SELECT uc.id,uc.expires_at,b.id,b.name,b.status,b.distribution_mode,b.discount_type,b.discount_value,b.min_amount_fen,b.max_discount_fen,b.scope,COALESCE(b.plan_ids,JSON_ARRAY()),COALESCE(b.image_ids,JSON_ARRAY()),COALESCE(b.month_values,JSON_ARRAY()),b.starts_at,b.ends_at,b.issue_limit,b.per_user_limit,b.issued_count,b.created_by,b.created_at,b.updated_at FROM xcloud_user_coupons uc JOIN xcloud_coupon_batches b ON b.id=uc.batch_id WHERE uc.owner_id=? AND uc.status='available' AND (uc.expires_at IS NULL OR uc.expires_at>NOW())`
	var rows *sql.Rows
	var err error
	if tx != nil {
		q += ` FOR UPDATE`
		rows, err = tx.QueryContext(ctx, q, ownerID)
	} else {
		rows, err = instanceDB.QueryContext(ctx, q, ownerID)
	}
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	out := []priceCandidate{}
	lookup := map[string]struct {
		p promotion
		c coupon
	}{}
	for rows.Next() {
		var couponID string
		var expires *time.Time
		var v couponBatch
		var a, b, c []byte
		e := rows.Scan(&couponID, &expires, &v.ID, &v.Name, &v.Status, &v.DistributionMode, &v.DiscountType, &v.DiscountValue, &v.MinAmountFen, &v.MaxDiscountFen, &v.Scope, &a, &b, &c, &v.StartsAt, &v.EndsAt, &v.IssueLimit, &v.PerUserLimit, &v.IssuedCount, &v.CreatedBy, &v.CreatedAt, &v.UpdatedAt)
		if e != nil {
			return nil, nil, e
		}
		_ = json.Unmarshal(a, &v.PlanIDs)
		_ = json.Unmarshal(b, &v.ImageIDs)
		_ = json.Unmarshal(c, &v.MonthValues)
		p := batchAsPromotion(v, expires)
		ok, e := promotionMatches(p, ownerID, scope, planID, imageID, months, list, time.Now(), tx)
		if e != nil {
			return nil, nil, e
		}
		if !ok {
			continue
		}
		d := promotionDiscount(p, list)
		out = append(out, priceCandidate{ID: couponID, Kind: "coupon", Name: v.Name, Label: "代金券", EligibilityReason: "券包中可用", DiscountAmountFen: d, PayableAmountFen: list - d})
		lookup[couponID] = struct {
			p promotion
			c coupon
		}{p, coupon{ID: v.ID, UserCouponID: couponID}}
	}
	return out, lookup, rows.Err()
}

// quoteForModern is the only settlement path. Coupon codes are deliberately
// absent: a user may choose only from coupons already in their own wallet.
func quoteForModern(ctx context.Context, ownerID, scope, planID, imageID string, months, list int, selectedID string, payFullPrice bool, tx *sql.Tx) (priceQuote, *promotion, *coupon, error) {
	activities, err := activePromotions(ctx, tx, false)
	if err != nil {
		return priceQuote{}, nil, nil, err
	}
	now := time.Now()
	candidates := []priceCandidate{}
	selected := map[string]struct {
		p promotion
		c *coupon
	}{}
	for i := range activities {
		p := activities[i]
		ok, e := promotionMatches(p, ownerID, scope, planID, imageID, months, list, now, tx)
		if e != nil {
			return priceQuote{}, nil, nil, e
		}
		if !ok {
			continue
		}
		d := promotionDiscount(p, list)
		candidates = append(candidates, priceCandidate{ID: p.ID, Kind: p.Kind, Name: p.Name, Label: promotionLabel(p.Kind), EligibilityReason: "当前订单符合使用条件", DiscountAmountFen: d, PayableAmountFen: list - d})
		selected[p.ID] = struct {
			p promotion
			c *coupon
		}{p, nil}
	}
	items, couponMap, err := couponBatchCandidates(ctx, ownerID, scope, planID, imageID, months, list, tx)
	if err != nil {
		return priceQuote{}, nil, nil, err
	}
	candidates = append(candidates, items...)
	for id, v := range couponMap {
		cp := v.c
		selected[id] = struct {
			p promotion
			c *coupon
		}{v.p, &cp}
	}
	q := priceQuote{ListAmountFen: list, AmountFen: list, Candidates: candidates}
	for _, v := range candidates {
		if v.DiscountAmountFen > q.DiscountAmountFen {
			q.DiscountAmountFen = v.DiscountAmountFen
			q.SelectedID = v.ID
		}
	}
	if payFullPrice {
		q.SelectedID = ""
		q.DiscountAmountFen = 0
		q.PayFullPrice = true
	} else if selectedID != "" {
		v, ok := selected[selectedID]
		if !ok {
			return priceQuote{}, nil, nil, errors.New("所选优惠不可用")
		}
		q.SelectedID = selectedID
		q.DiscountAmountFen = promotionDiscount(v.p, list)
	}
	q.AmountFen = list - q.DiscountAmountFen
	for i := range q.Candidates {
		q.Candidates[i].IsDefault = q.Candidates[i].ID == q.SelectedID
	}
	if q.SelectedID == "" {
		return q, nil, nil, nil
	}
	v := selected[q.SelectedID]
	return q, &v.p, v.c, nil
}
