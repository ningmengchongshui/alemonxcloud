package cloud

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

type promotion struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Kind           string     `json:"kind"`
	Scope          string     `json:"scope"`
	DiscountType   string     `json:"discountType"`
	DiscountValue  int        `json:"discountValue"`
	MinAmountFen   int        `json:"minAmountFen"`
	MaxDiscountFen int        `json:"maxDiscountFen"`
	TotalLimit     int        `json:"totalLimit"`
	PerUserLimit   int        `json:"perUserLimit"`
	UsedCount      int        `json:"usedCount"`
	PlanIDs        []string   `json:"planIDs"`
	ImageIDs       []string   `json:"imageIDs"`
	MonthValues    []string   `json:"monthValues"`
	StartsAt       *time.Time `json:"startsAt,omitempty"`
	EndsAt         *time.Time `json:"endsAt,omitempty"`
	Enabled        bool       `json:"enabled"`
	CreatedBy      string     `json:"createdBy,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt,omitempty"`
}
type coupon struct {
	ID           string    `json:"id"`
	UserCouponID string    `json:"userCouponId,omitempty"`
	PromotionID  string    `json:"promotionId"`
	CodeMask     string    `json:"codeMask"`
	Mode         string    `json:"mode"`
	Enabled      bool      `json:"enabled"`
	TotalLimit   int       `json:"totalLimit"`
	PerUserLimit int       `json:"perUserLimit"`
	UsedCount    int       `json:"usedCount"`
	CreatedAt    time.Time `json:"createdAt"`
}
type couponClaim struct {
	ID          string    `json:"id"`
	OwnerID     string    `json:"ownerId"`
	PromotionID string    `json:"promotionId"`
	CouponID    string    `json:"couponId"`
	Status      string    `json:"status"`
	ClaimedAt   time.Time `json:"claimedAt"`
}
type priceCandidate struct {
	ID                string `json:"id"`
	Kind              string `json:"kind"`
	Name              string `json:"name"`
	CouponCode        string `json:"-"`
	DiscountAmountFen int    `json:"discountAmountFen"`
	PayableAmountFen  int    `json:"payableAmountFen"`
	IsDefault         bool   `json:"isDefault"`
	Label             string `json:"label"`
	EligibilityReason string `json:"eligibilityReason,omitempty"`
}
type priceQuote struct {
	ListAmountFen     int              `json:"listAmountFen"`
	DiscountAmountFen int              `json:"discountAmountFen"`
	AmountFen         int              `json:"amountFen"`
	Candidates        []priceCandidate `json:"candidates"`
	SelectedID        string           `json:"selectedId,omitempty"`
	PayFullPrice      bool             `json:"payFullPrice"`
}

func normalizeCouponCode(v string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(v), " ", ""))
}
func couponHash(v string) string {
	s := sha256.Sum256([]byte(env("XCLOUD_COUPON_CODE_SECRET", "xcloud-dev-coupon-secret") + "\x00" + normalizeCouponCode(v)))
	return hex.EncodeToString(s[:])
}
func couponMask(v string) string {
	v = normalizeCouponCode(v)
	if len(v) <= 4 {
		return "****"
	}
	return v[:2] + strings.Repeat("*", len(v)-4) + v[len(v)-2:]
}
func validPromotion(v promotion) error {
	v.Name = strings.TrimSpace(v.Name)
	if v.Name == "" || len(v.Name) > 128 {
		return errors.New("活动名称长度应为 1 至 128")
	}
	if v.Kind != "campaign" && v.Kind != "newcomer" && v.Kind != "first_plan_purchase" {
		return errors.New("活动类型无效")
	}
	if v.Scope != "purchase" && v.Scope != "renewal" && v.Scope != "both" {
		return errors.New("适用范围无效")
	}
	if (v.Kind == "newcomer" || v.Kind == "first_plan_purchase") && v.Scope != "purchase" {
		return errors.New("新人专属和套餐新购优惠只能适用于新购")
	}
	if v.Kind == "first_plan_purchase" && len(v.PlanIDs) == 0 {
		return errors.New("套餐新购优惠必须至少选择一个套餐")
	}
	if v.DiscountType != "fixed" && v.DiscountType != "percent" {
		return errors.New("优惠类型无效")
	}
	if v.DiscountValue <= 0 || (v.DiscountType == "percent" && v.DiscountValue > 10000) {
		return errors.New("优惠值无效")
	}
	if v.MinAmountFen < 0 || v.MaxDiscountFen < 0 || v.TotalLimit < 0 || v.PerUserLimit < 0 {
		return errors.New("优惠限制无效")
	}
	if v.EndsAt != nil && v.StartsAt != nil && !v.EndsAt.After(*v.StartsAt) {
		return errors.New("结束时间必须晚于开始时间")
	}
	return nil
}
func jsonStringSlice(v []string) []byte { b, _ := json.Marshal(v); return b }
func promotionMatches(v promotion, ownerID, scope, planID, imageID string, months, list int, now time.Time, tx *sql.Tx) (bool, error) {
	if !v.Enabled || (v.Scope != "both" && v.Scope != scope) || (v.StartsAt != nil && now.Before(*v.StartsAt)) || (v.EndsAt != nil && !now.Before(*v.EndsAt)) || list < v.MinAmountFen {
		return false, nil
	}
	contains := func(xs []string, x string) bool {
		for _, v := range xs {
			if v == x {
				return true
			}
		}
		return len(xs) == 0
	}
	if !contains(v.PlanIDs, planID) || !contains(v.ImageIDs, imageID) || !contains(v.MonthValues, fmt.Sprint(months)) {
		return false, nil
	}
	if v.TotalLimit > 0 && v.UsedCount >= v.TotalLimit {
		return false, nil
	}
	if v.PerUserLimit > 0 {
		var count int
		q := instanceDB.QueryRowContext
		if tx != nil {
			q = tx.QueryRowContext
		}
		if err := q(context.Background(), `SELECT COUNT(*) FROM xcloud_coupon_redemptions WHERE promotion_id=? AND owner_id=?`, v.ID, ownerID).Scan(&count); err != nil {
			return false, err
		}
		if count >= v.PerUserLimit {
			return false, nil
		}
	}
	if v.Kind == "newcomer" {
		var count int
		q := instanceDB.QueryRowContext
		if tx != nil {
			q = tx.QueryRowContext
		}
		if err := q(context.Background(), `SELECT COUNT(*) FROM xcloud_orders WHERE owner_id=? AND payment_source='wallet' AND status NOT IN ('cancelled','rejected')`, ownerID).Scan(&count); err != nil {
			return false, err
		}
		if count > 0 {
			return false, nil
		}
	}
	if v.Kind == "first_plan_purchase" {
		var count int
		q := instanceDB.QueryRowContext
		if tx != nil {
			q = tx.QueryRowContext
		}
		if err := q(context.Background(), `SELECT COUNT(*) FROM xcloud_orders WHERE owner_id=? AND plan_id=? AND payment_source='wallet' AND status NOT IN ('cancelled','rejected')`, ownerID, planID).Scan(&count); err != nil {
			return false, err
		}
		if count > 0 {
			return false, nil
		}
	}
	return true, nil
}

func promotionLabel(kind string) string {
	switch kind {
	case "newcomer":
		return "新人专属"
	case "first_plan_purchase":
		return "套餐新购优惠"
	case "coupon":
		return "已领取代金券"
	default:
		return "活动优惠"
	}
}
func promotionDiscount(v promotion, list int) int {
	d := v.DiscountValue
	if v.DiscountType == "percent" {
		// percent is a payable rate: 9500 means the user pays 95% (95 折).
		d = list - int(math.Floor(float64(list)*float64(d)/10000))
	}
	if v.MaxDiscountFen > 0 && d > v.MaxDiscountFen {
		d = v.MaxDiscountFen
	}
	if d > list {
		d = list
	}
	return d
}
func consumePromotionTx(ctx context.Context, tx *sql.Tx, ownerID, orderID string, quote priceQuote, p *promotion, cp *coupon) error {
	if quote.SelectedID == "" || p == nil {
		return nil
	}
	if p.Kind == "coupon_batch" {
		if cp == nil || cp.UserCouponID == "" {
			return errors.New("用户代金券无效")
		}
		r, e := tx.ExecContext(ctx, `UPDATE xcloud_user_coupons SET status='used',used_at=NOW(),order_id=? WHERE id=? AND owner_id=? AND status='available' AND (expires_at IS NULL OR expires_at>NOW())`, orderID, cp.UserCouponID, ownerID)
		if e != nil {
			return e
		}
		if n, _ := r.RowsAffected(); n != 1 {
			return errors.New("代金券已失效或已使用")
		}
		snap, _ := json.Marshal(map[string]any{"id": p.ID, "name": p.Name, "kind": "coupon", "label": "代金券", "discountType": p.DiscountType, "discountValue": p.DiscountValue, "discountAmountFen": quote.DiscountAmountFen, "userCouponId": cp.UserCouponID})
		redemptionID := newID("red")
		if _, e = tx.ExecContext(ctx, `INSERT INTO xcloud_coupon_redemptions (id,promotion_id,coupon_id,user_coupon_id,owner_id,order_id,discount_amount_fen,created_at) VALUES (?,?,?,?,?,?,?,NOW())`, redemptionID, p.ID, nil, cp.UserCouponID, ownerID, orderID, quote.DiscountAmountFen); e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `UPDATE xcloud_orders SET promotion_snapshot=?,coupon_redemption_id=? WHERE id=?`, snap, redemptionID, orderID)
		return e
	}
	r, e := tx.ExecContext(ctx, `UPDATE xcloud_promotions SET used_count=used_count+1,updated_at=NOW() WHERE id=? AND enabled=TRUE AND (starts_at IS NULL OR starts_at<=NOW()) AND (ends_at IS NULL OR ends_at>NOW()) AND (total_limit=0 OR used_count<total_limit)`, p.ID)
	if e != nil {
		return e
	}
	if n, _ := r.RowsAffected(); n != 1 {
		return errors.New("优惠已失效或名额已用完")
	}
	if cp != nil {
		if _, e := tx.ExecContext(ctx, `UPDATE xcloud_coupon_claims SET status='used',used_at=NOW(),order_id=? WHERE owner_id=? AND coupon_id=? AND status='active'`, orderID, ownerID, cp.ID); e != nil {
			return e
		}
		if cp.PerUserLimit > 0 {
			var used int
			if e := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM xcloud_coupon_redemptions WHERE coupon_id=? AND owner_id=? FOR UPDATE`, cp.ID, ownerID).Scan(&used); e != nil {
				return e
			}
			if used >= cp.PerUserLimit {
				return errors.New("该券已达到你的使用上限")
			}
		}
		if cp.TotalLimit > 0 {
			r, e := tx.ExecContext(ctx, `UPDATE xcloud_coupons SET used_count=used_count+1 WHERE id=? AND enabled=TRUE AND used_count<?`, cp.ID, cp.TotalLimit)
			if e != nil {
				return e
			}
			n, _ := r.RowsAffected()
			if n != 1 {
				return errors.New("券码已用完")
			}
		}
	}
	snap, _ := json.Marshal(map[string]any{"id": p.ID, "name": p.Name, "kind": p.Kind, "label": promotionLabel(p.Kind), "discountType": p.DiscountType, "discountValue": p.DiscountValue, "discountAmountFen": quote.DiscountAmountFen, "couponMask": func() string {
		if cp != nil {
			return cp.CodeMask
		}
		return ""
	}()})
	couponID := any(nil)
	if cp != nil {
		couponID = cp.ID
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO xcloud_coupon_redemptions (id,promotion_id,coupon_id,owner_id,order_id,discount_amount_fen,created_at) VALUES (?,?,?,?,?,?,NOW())`, newID("red"), p.ID, couponID, ownerID, orderID, quote.DiscountAmountFen)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `UPDATE xcloud_orders SET promotion_snapshot=?,coupon_redemption_id=(SELECT id FROM xcloud_coupon_redemptions WHERE order_id=?) WHERE id=?`, snap, orderID, orderID)
	return e
}
func activePromotions(ctx context.Context, tx *sql.Tx, lock bool) ([]promotion, error) {
	return loadPromotions(ctx, tx, lock, true)
}

func allPromotions(ctx context.Context) ([]promotion, error) {
	return loadPromotions(ctx, nil, false, false)
}

func loadPromotions(ctx context.Context, tx *sql.Tx, lock, enabledOnly bool) ([]promotion, error) {
	q := `SELECT id,name,kind,scope,discount_type,discount_value,min_amount_fen,max_discount_fen,COALESCE(plan_ids,JSON_ARRAY()),COALESCE(image_ids,JSON_ARRAY()),COALESCE(month_values,JSON_ARRAY()),starts_at,ends_at,total_limit,per_user_limit,used_count,enabled,created_by,created_at,updated_at FROM xcloud_promotions`
	if enabledOnly {
		q += " WHERE enabled=TRUE"
	}
	if lock {
		q += " FOR UPDATE"
	}
	var rows *sql.Rows
	var err error
	if tx != nil {
		rows, err = tx.QueryContext(ctx, q)
	} else {
		rows, err = instanceDB.QueryContext(ctx, q)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []promotion{}
	for rows.Next() {
		var v promotion
		var a, b, c []byte
		if err = rows.Scan(&v.ID, &v.Name, &v.Kind, &v.Scope, &v.DiscountType, &v.DiscountValue, &v.MinAmountFen, &v.MaxDiscountFen, &a, &b, &c, &v.StartsAt, &v.EndsAt, &v.TotalLimit, &v.PerUserLimit, &v.UsedCount, &v.Enabled, &v.CreatedBy, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(a, &v.PlanIDs)
		_ = json.Unmarshal(b, &v.ImageIDs)
		_ = json.Unmarshal(c, &v.MonthValues)
		out = append(out, v)
	}
	return out, rows.Err()
}
func claimedCoupons(ctx context.Context, ownerID string) ([]couponClaim, error) {
	rows, err := instanceDB.QueryContext(ctx, `SELECT id,owner_id,promotion_id,coupon_id,status,claimed_at FROM xcloud_coupon_claims WHERE owner_id=? AND status='active' ORDER BY claimed_at`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []couponClaim{}
	for rows.Next() {
		var v couponClaim
		if err := rows.Scan(&v.ID, &v.OwnerID, &v.PromotionID, &v.CouponID, &v.Status, &v.ClaimedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func quoteFor(ctx context.Context, ownerID, scope, planID, imageID string, months, list int, couponCode, selectedID string, payFullPrice bool, tx *sql.Tx) (priceQuote, *promotion, *coupon, error) {
	values, err := activePromotions(ctx, tx, false)
	if err != nil {
		return priceQuote{}, nil, nil, err
	}
	now := time.Now()
	candidates := []priceCandidate{}
	var selectedPromo *promotion
	var selectedCoupon *coupon
	var couponPromotion *promotion
	var enteredCoupon *coupon
	for i := range values {
		ok, e := promotionMatches(values[i], ownerID, scope, planID, imageID, months, list, now, tx)
		if e != nil {
			return priceQuote{}, nil, nil, e
		}
		if !ok {
			continue
		}
		d := promotionDiscount(values[i], list)
		candidates = append(candidates, priceCandidate{ID: values[i].ID, Kind: values[i].Kind, Name: values[i].Name, Label: promotionLabel(values[i].Kind), EligibilityReason: "当前订单符合使用条件", DiscountAmountFen: d, PayableAmountFen: list - d})
		if selectedID == values[i].ID {
			selectedPromo = &values[i]
		}
	}
	// Claimed vouchers are visible without exposing or requiring a raw code.
	claims, claimErr := claimedCoupons(ctx, ownerID)
	if claimErr != nil {
		return priceQuote{}, nil, nil, claimErr
	}
	for _, claim := range claims {
		var cp coupon
		var p promotion
		var a, b, c []byte
		err := instanceDB.QueryRowContext(ctx, `SELECT c.id,c.promotion_id,c.code_mask,c.mode,c.enabled,c.total_limit,c.per_user_limit,c.used_count,c.created_at,p.id,p.name,p.kind,p.scope,p.discount_type,p.discount_value,p.min_amount_fen,p.max_discount_fen,COALESCE(p.plan_ids,JSON_ARRAY()),COALESCE(p.image_ids,JSON_ARRAY()),COALESCE(p.month_values,JSON_ARRAY()),p.starts_at,p.ends_at,p.total_limit,p.per_user_limit,p.used_count,p.enabled,p.created_by,p.created_at,p.updated_at FROM xcloud_coupons c JOIN xcloud_promotions p ON p.id=c.promotion_id WHERE c.id=?`, claim.CouponID).Scan(&cp.ID, &cp.PromotionID, &cp.CodeMask, &cp.Mode, &cp.Enabled, &cp.TotalLimit, &cp.PerUserLimit, &cp.UsedCount, &cp.CreatedAt, &p.ID, &p.Name, &p.Kind, &p.Scope, &p.DiscountType, &p.DiscountValue, &p.MinAmountFen, &p.MaxDiscountFen, &a, &b, &c, &p.StartsAt, &p.EndsAt, &p.TotalLimit, &p.PerUserLimit, &p.UsedCount, &p.Enabled, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
		if err != nil {
			continue
		}
		_ = json.Unmarshal(a, &p.PlanIDs)
		_ = json.Unmarshal(b, &p.ImageIDs)
		_ = json.Unmarshal(c, &p.MonthValues)
		ok, e := promotionMatches(p, ownerID, scope, planID, imageID, months, list, now, tx)
		if e != nil {
			return priceQuote{}, nil, nil, e
		}
		if !ok || !cp.Enabled || (cp.TotalLimit > 0 && cp.UsedCount >= cp.TotalLimit) {
			continue
		}
		candidates = append(candidates, priceCandidate{ID: claim.ID, Kind: "coupon", Name: p.Name + " · 已领取", Label: promotionLabel("coupon"), EligibilityReason: "已领取至优惠券包", DiscountAmountFen: promotionDiscount(p, list), PayableAmountFen: list - promotionDiscount(p, list)})
	}
	if couponCode != "" {
		var cp coupon
		var p promotion
		var a, b, c []byte
		q := `SELECT c.id,c.promotion_id,c.code_mask,c.mode,c.enabled,c.total_limit,c.per_user_limit,c.used_count,c.created_at,p.id,p.name,p.kind,p.scope,p.discount_type,p.discount_value,p.min_amount_fen,p.max_discount_fen,COALESCE(p.plan_ids,JSON_ARRAY()),COALESCE(p.image_ids,JSON_ARRAY()),COALESCE(p.month_values,JSON_ARRAY()),p.starts_at,p.ends_at,p.total_limit,p.per_user_limit,p.used_count,p.enabled,p.created_by,p.created_at,p.updated_at FROM xcloud_coupons c JOIN xcloud_promotions p ON p.id=c.promotion_id WHERE c.code_hash=?`
		if tx != nil {
			q += " FOR UPDATE"
			err = tx.QueryRowContext(ctx, q, couponHash(couponCode)).Scan(&cp.ID, &cp.PromotionID, &cp.CodeMask, &cp.Mode, &cp.Enabled, &cp.TotalLimit, &cp.PerUserLimit, &cp.UsedCount, &cp.CreatedAt, &p.ID, &p.Name, &p.Kind, &p.Scope, &p.DiscountType, &p.DiscountValue, &p.MinAmountFen, &p.MaxDiscountFen, &a, &b, &c, &p.StartsAt, &p.EndsAt, &p.TotalLimit, &p.PerUserLimit, &p.UsedCount, &p.Enabled, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
		} else {
			err = instanceDB.QueryRowContext(ctx, q, couponHash(couponCode)).Scan(&cp.ID, &cp.PromotionID, &cp.CodeMask, &cp.Mode, &cp.Enabled, &cp.TotalLimit, &cp.PerUserLimit, &cp.UsedCount, &cp.CreatedAt, &p.ID, &p.Name, &p.Kind, &p.Scope, &p.DiscountType, &p.DiscountValue, &p.MinAmountFen, &p.MaxDiscountFen, &a, &b, &c, &p.StartsAt, &p.EndsAt, &p.TotalLimit, &p.PerUserLimit, &p.UsedCount, &p.Enabled, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
		}
		_ = json.Unmarshal(a, &p.PlanIDs)
		_ = json.Unmarshal(b, &p.ImageIDs)
		_ = json.Unmarshal(c, &p.MonthValues)
		if err != nil {
			return priceQuote{}, nil, nil, errors.New("券码无效或已停用")
		}
		if !cp.Enabled || (cp.TotalLimit > 0 && cp.UsedCount >= cp.TotalLimit) {
			return priceQuote{}, nil, nil, errors.New("券码已用完或已停用")
		}
		ok, e := promotionMatches(p, ownerID, scope, planID, imageID, months, list, now, tx)
		if e != nil || !ok {
			return priceQuote{}, nil, nil, errors.New("该券不适用于当前订单")
		}
		if cp.PerUserLimit > 0 {
			var used int
			qCount := instanceDB.QueryRowContext
			if tx != nil {
				qCount = tx.QueryRowContext
			}
			if err := qCount(ctx, `SELECT COUNT(*) FROM xcloud_coupon_redemptions WHERE coupon_id=? AND owner_id=?`, cp.ID, ownerID).Scan(&used); err != nil {
				return priceQuote{}, nil, nil, err
			}
			if used >= cp.PerUserLimit {
				return priceQuote{}, nil, nil, errors.New("该券已达到你的使用上限")
			}
		}
		d := promotionDiscount(p, list)
		candidates = append(candidates, priceCandidate{ID: cp.ID, Kind: "coupon", Name: p.Name, Label: promotionLabel("coupon"), EligibilityReason: "券码有效", DiscountAmountFen: d, PayableAmountFen: list - d})
		couponPromotion = &p
		enteredCoupon = &cp
	}
	q := priceQuote{ListAmountFen: list, AmountFen: list, Candidates: candidates}
	for i := range candidates {
		if candidates[i].DiscountAmountFen > q.DiscountAmountFen {
			q.DiscountAmountFen = candidates[i].DiscountAmountFen
			q.SelectedID = candidates[i].ID
		}
	}
	if payFullPrice {
		q.SelectedID = ""
		q.DiscountAmountFen = 0
		q.PayFullPrice = true
	} else if selectedID != "" {
		found := false
		for _, v := range candidates {
			if v.ID == selectedID {
				q.DiscountAmountFen = v.DiscountAmountFen
				q.SelectedID = v.ID
				found = true
			}
		}
		if !found {
			return priceQuote{}, nil, nil, errors.New("所选优惠不可用")
		}
	}
	q.AmountFen = list - q.DiscountAmountFen
	for i := range q.Candidates {
		q.Candidates[i].IsDefault = q.Candidates[i].ID == q.SelectedID
	}
	selectedPromo, selectedCoupon = nil, nil
	if q.SelectedID != "" {
		for i := range values {
			if values[i].ID == q.SelectedID {
				selectedPromo = &values[i]
			}
		}
		if enteredCoupon != nil && q.SelectedID == enteredCoupon.ID {
			selectedPromo = couponPromotion
			selectedCoupon = enteredCoupon
		}
		for _, claim := range claims {
			if claim.ID == q.SelectedID {
				var cp coupon
				var p promotion
				_ = instanceDB.QueryRowContext(ctx, `SELECT c.id,c.promotion_id,c.code_mask,c.mode,c.enabled,c.total_limit,c.per_user_limit,c.used_count,c.created_at,p.id,p.name,p.kind,p.scope,p.discount_type,p.discount_value,p.min_amount_fen,p.max_discount_fen,p.total_limit,p.per_user_limit,p.used_count,p.enabled,p.created_by,p.created_at,p.updated_at FROM xcloud_coupons c JOIN xcloud_promotions p ON p.id=c.promotion_id WHERE c.id=?`, claim.CouponID).Scan(&cp.ID, &cp.PromotionID, &cp.CodeMask, &cp.Mode, &cp.Enabled, &cp.TotalLimit, &cp.PerUserLimit, &cp.UsedCount, &cp.CreatedAt, &p.ID, &p.Name, &p.Kind, &p.Scope, &p.DiscountType, &p.DiscountValue, &p.MinAmountFen, &p.MaxDiscountFen, &p.TotalLimit, &p.PerUserLimit, &p.UsedCount, &p.Enabled, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
				selectedPromo = &p
				selectedCoupon = &cp
			}
		}
	}
	return q, selectedPromo, selectedCoupon, nil
}
