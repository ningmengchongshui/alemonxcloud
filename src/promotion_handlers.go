package cloud

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
	"time"
)

func quotePurchaseHandler(c *gin.Context) {
	var b struct {
		PlanID       string `json:"planId"`
		ImageID      string `json:"imageId"`
		Months       int    `json:"months"`
		CouponCode   string `json:"couponCode"`
		SelectionID  string `json:"selectionId"`
		PayFullPrice bool   `json:"payFullPrice"`
	}
	if c.ShouldBindJSON(&b) != nil || b.Months < 1 || b.Months > 24 {
		c.JSON(400, gin.H{"message": "报价参数无效"})
		return
	}
	var monthly int
	if err := instanceDB.QueryRowContext(c, `SELECT monthly_price_fen FROM xcloud_plans WHERE id=? AND enabled=TRUE`, b.PlanID).Scan(&monthly); err != nil {
		businessError(c, errors.New("套餐不可购买"))
		return
	}
	var ok int
	if err := instanceDB.QueryRowContext(c, `SELECT 1 FROM xcloud_images WHERE id=? AND enabled=TRUE`, b.ImageID).Scan(&ok); err != nil {
		businessError(c, errors.New("镜像版本不可购买"))
		return
	}
	user := c.MustGet("user").(oidcUser)
	q, _, _, err := quoteFor(c, user.ID, "purchase", b.PlanID, b.ImageID, b.Months, monthly*b.Months, b.CouponCode, b.SelectionID, b.PayFullPrice, nil)
	if err != nil {
		businessError(c, err)
		return
	}
	c.JSON(200, q)
}
func couponBackpackHandler(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	items, err := claimedCoupons(c.Request.Context(), user.ID)
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}
func publicPromotionsHandler(c *gin.Context) {
	items, err := activePromotions(c.Request.Context(), nil, false)
	if err != nil {
		internalError(c, err)
		return
	}
	out := []promotion{}
	for _, item := range items {
		if item.Kind == "campaign" && item.Enabled && (item.StartsAt == nil || !time.Now().Before(*item.StartsAt)) && (item.EndsAt == nil || time.Now().Before(*item.EndsAt)) {
			out = append(out, item)
		}
	}
	c.JSON(http.StatusOK, out)
}
func claimPromotionHandler(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	tx, err := beginSerializableTx(c.Request.Context())
	if err != nil {
		internalError(c, err)
		return
	}
	defer tx.Rollback()
	var couponID string
	err = tx.QueryRowContext(c.Request.Context(), `SELECT c.id FROM xcloud_coupons c JOIN xcloud_promotions p ON p.id=c.promotion_id WHERE p.id=? AND p.kind='campaign' AND p.enabled=TRUE AND c.mode='general' AND c.enabled=TRUE AND (p.starts_at IS NULL OR p.starts_at<=NOW()) AND (p.ends_at IS NULL OR p.ends_at>NOW()) ORDER BY c.created_at LIMIT 1 FOR UPDATE`, c.Param("id")).Scan(&couponID)
	if err != nil {
		businessError(c, errors.New("当前活动暂无可领取代金券"))
		return
	}
	var limit, count int
	if err = tx.QueryRowContext(c.Request.Context(), `SELECT total_limit FROM xcloud_coupons WHERE id=? FOR UPDATE`, couponID).Scan(&limit); err != nil {
		internalError(c, err)
		return
	}
	if err = tx.QueryRowContext(c.Request.Context(), `SELECT COUNT(*) FROM xcloud_coupon_claims WHERE coupon_id=?`, couponID).Scan(&count); err != nil {
		internalError(c, err)
		return
	}
	if limit > 0 && count >= limit {
		businessError(c, errors.New("代金券已领完"))
		return
	}
	claim := couponClaim{ID: newID("clm"), OwnerID: user.ID, PromotionID: c.Param("id"), CouponID: couponID, Status: "active", ClaimedAt: time.Now()}
	if _, err = tx.ExecContext(c.Request.Context(), `INSERT INTO xcloud_coupon_claims (id,owner_id,promotion_id,coupon_id,status,claimed_at) VALUES (?,?,?,?,?,?)`, claim.ID, claim.OwnerID, claim.PromotionID, claim.CouponID, claim.Status, claim.ClaimedAt); err != nil {
		businessError(c, errors.New("你已领取该代金券"))
		return
	}
	if err = writeAuditTx(c.Request.Context(), tx, user.ID, "coupon.claim", "promotion", claim.PromotionID, map[string]any{"claimId": claim.ID}); err != nil {
		internalError(c, err)
		return
	}
	if err = tx.Commit(); err != nil {
		internalError(c, err)
		return
	}
	_ = createNotification(c.Request.Context(), user.ID, "promotion", "代金券已领取", "已放入你的优惠券包，可在结算时自动使用。", map[string]any{"claimId": claim.ID, "promotionId": claim.PromotionID})
	c.JSON(http.StatusCreated, claim)
}
func quoteRenewHandler(c *gin.Context) {
	var b struct {
		Months       int    `json:"months"`
		CouponCode   string `json:"couponCode"`
		SelectionID  string `json:"selectionId"`
		PayFullPrice bool   `json:"payFullPrice"`
	}
	if c.ShouldBindJSON(&b) != nil || b.Months < 1 || b.Months > 24 {
		c.JSON(400, gin.H{"message": "报价参数无效"})
		return
	}
	user := c.MustGet("user").(oidcUser)
	var planID, imageID string
	var monthly int
	if err := instanceDB.QueryRowContext(c, `SELECT o.plan_id,o.image_id,p.monthly_price_fen FROM xcloud_orders o JOIN xcloud_plans p ON p.id=o.plan_id WHERE o.id=? AND o.owner_id=?`, c.Param("id"), user.ID).Scan(&planID, &imageID, &monthly); err != nil {
		businessError(c, errors.New("订单不可续费"))
		return
	}
	q, _, _, err := quoteFor(c, user.ID, "renewal", planID, imageID, b.Months, monthly*b.Months, b.CouponCode, b.SelectionID, b.PayFullPrice, nil)
	if err != nil {
		businessError(c, err)
		return
	}
	c.JSON(200, q)
}
func adminPromotions(c *gin.Context) {
	values, err := allPromotions(c)
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(200, values)
}
func adminSavePromotion(c *gin.Context) {
	// Timestamps and usage counters are server-owned. In particular, an empty
	// createdAt from a new-form must never be parsed as a client time value.
	var raw map[string]json.RawMessage
	if c.ShouldBindJSON(&raw) != nil {
		c.JSON(400, gin.H{"message": "活动参数无效"})
		return
	}
	delete(raw, "createdAt")
	delete(raw, "updatedAt")
	delete(raw, "usedCount")
	payload, err := json.Marshal(raw)
	var v promotion
	if err != nil || json.Unmarshal(payload, &v) != nil {
		c.JSON(400, gin.H{"message": "活动参数无效"})
		return
	}
	if id := c.Param("id"); id != "" {
		v.ID = id
	}
	if err := validPromotion(v); err != nil {
		businessError(c, err)
		return
	}
	user := c.MustGet("user").(oidcUser)
	now := time.Now()
	if v.ID == "" {
		v.ID = newID("pro")
		v.CreatedBy = user.ID
		v.CreatedAt = now
	}
	v.UpdatedAt = now
	if _, err := instanceDB.ExecContext(c, `INSERT INTO xcloud_promotions (id,name,kind,scope,discount_type,discount_value,min_amount_fen,max_discount_fen,plan_ids,image_ids,month_values,starts_at,ends_at,total_limit,per_user_limit,used_count,enabled,created_by,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE name=VALUES(name),kind=VALUES(kind),scope=VALUES(scope),discount_type=VALUES(discount_type),discount_value=VALUES(discount_value),min_amount_fen=VALUES(min_amount_fen),max_discount_fen=VALUES(max_discount_fen),plan_ids=VALUES(plan_ids),image_ids=VALUES(image_ids),month_values=VALUES(month_values),starts_at=VALUES(starts_at),ends_at=VALUES(ends_at),total_limit=VALUES(total_limit),per_user_limit=VALUES(per_user_limit),enabled=VALUES(enabled),updated_at=VALUES(updated_at)`, v.ID, strings.TrimSpace(v.Name), v.Kind, v.Scope, v.DiscountType, v.DiscountValue, v.MinAmountFen, v.MaxDiscountFen, jsonStringSlice(v.PlanIDs), jsonStringSlice(v.ImageIDs), jsonStringSlice(v.MonthValues), v.StartsAt, v.EndsAt, v.TotalLimit, v.PerUserLimit, 0, v.Enabled, v.CreatedBy, v.CreatedAt, v.UpdatedAt); err != nil {
		internalError(c, err)
		return
	}
	_ = writeAudit(c, user.ID, "promotion.save", "promotion", v.ID, map[string]any{"kind": v.Kind, "label": promotionLabel(v.Kind), "enabled": v.Enabled})
	c.JSON(200, v)
}
func adminCoupons(c *gin.Context) {
	rows, err := instanceDB.QueryContext(c, `SELECT id,promotion_id,code_mask,mode,enabled,total_limit,per_user_limit,used_count,created_at FROM xcloud_coupons ORDER BY created_at DESC LIMIT 500`)
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()
	out := []coupon{}
	for rows.Next() {
		var v coupon
		if err = rows.Scan(&v.ID, &v.PromotionID, &v.CodeMask, &v.Mode, &v.Enabled, &v.TotalLimit, &v.PerUserLimit, &v.UsedCount, &v.CreatedAt); err != nil {
			internalError(c, err)
			return
		}
		out = append(out, v)
	}
	c.JSON(200, out)
}
func freshCouponCode() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "XC-" + strings.ToUpper(hex.EncodeToString(b[:4])) + "-" + strings.ToUpper(hex.EncodeToString(b[4:]))
}
func adminCreateCoupons(c *gin.Context) {
	var b struct {
		PromotionID  string `json:"promotionId"`
		Mode         string `json:"mode"`
		Count        int    `json:"count"`
		Code         string `json:"code"`
		TotalLimit   int    `json:"totalLimit"`
		PerUserLimit int    `json:"perUserLimit"`
	}
	if c.ShouldBindJSON(&b) != nil || b.PromotionID == "" {
		c.JSON(400, gin.H{"message": "券码参数无效"})
		return
	}
	if b.Mode == "" {
		b.Mode = "single"
	}
	if b.Mode != "single" && b.Mode != "general" {
		c.JSON(400, gin.H{"message": "券码模式无效"})
		return
	}
	if b.Mode == "general" {
		b.Count = 1
	}
	if b.Count < 1 || b.Count > 500 {
		c.JSON(400, gin.H{"message": "批量数量应为 1 至 500"})
		return
	}
	var found string
	if err := instanceDB.QueryRowContext(c, `SELECT id FROM xcloud_promotions WHERE id=? AND kind='campaign'`, b.PromotionID).Scan(&found); err != nil {
		businessError(c, errors.New("活动不存在或不支持券码"))
		return
	}
	if b.TotalLimit <= 0 {
		if b.Mode == "single" {
			b.TotalLimit = 1
		} else {
			b.TotalLimit = 100
		}
	}
	if b.PerUserLimit <= 0 {
		b.PerUserLimit = 1
	}
	if b.Mode == "single" {
		b.TotalLimit, b.PerUserLimit = 1, 1
	}
	user := c.MustGet("user").(oidcUser)
	tx, err := beginSerializableTx(c.Request.Context())
	if err != nil {
		internalError(c, err)
		return
	}
	defer tx.Rollback()
	created := []gin.H{}
	for i := 0; i < b.Count; i++ {
		code := normalizeCouponCode(b.Code)
		if code == "" || i > 0 {
			code = freshCouponCode()
		}
		id := newID("cpn")
		_, err = tx.ExecContext(c, `INSERT INTO xcloud_coupons (id,promotion_id,code_hash,code_mask,mode,enabled,total_limit,per_user_limit,used_count,created_by,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,NOW())`, id, b.PromotionID, couponHash(code), couponMask(code), b.Mode, true, b.TotalLimit, b.PerUserLimit, 0, user.ID)
		if err != nil {
			businessError(c, errors.New("券码重复，请重试"))
			return
		}
		created = append(created, gin.H{"id": id, "code": code, "codeMask": couponMask(code)})
	}
	if err := writeAuditTx(c, tx, user.ID, "coupon.create", "promotion", b.PromotionID, map[string]any{"count": b.Count, "mode": b.Mode}); err != nil {
		internalError(c, err)
		return
	}
	if err := tx.Commit(); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"coupons": created})
}
func adminCouponStatus(c *gin.Context) {
	var b struct {
		Enabled bool `json:"enabled"`
	}
	if c.ShouldBindJSON(&b) != nil {
		c.JSON(400, gin.H{"message": "状态参数无效"})
		return
	}
	if _, err := instanceDB.ExecContext(c, `UPDATE xcloud_coupons SET enabled=?,disabled_at=CASE WHEN ? THEN NULL ELSE NOW() END WHERE id=?`, b.Enabled, b.Enabled, c.Param("id")); err != nil {
		internalError(c, err)
		return
	}
	user := c.MustGet("user").(oidcUser)
	_ = writeAudit(c, user.ID, "coupon.status", "coupon", c.Param("id"), map[string]any{"enabled": b.Enabled})
	c.Status(204)
}
func adminRedemptions(c *gin.Context) {
	rows, err := instanceDB.QueryContext(c, `SELECT id,promotion_id,COALESCE(coupon_id,''),owner_id,order_id,discount_amount_fen,created_at FROM xcloud_coupon_redemptions ORDER BY created_at DESC LIMIT 500`)
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id, pid, cid, uid, oid string
		var amount int
		var at time.Time
		if err = rows.Scan(&id, &pid, &cid, &uid, &oid, &amount, &at); err != nil {
			internalError(c, err)
			return
		}
		out = append(out, gin.H{"id": id, "promotionId": pid, "couponId": cid, "ownerId": uid, "orderId": oid, "discountAmountFen": amount, "createdAt": at})
	}
	c.JSON(200, out)
}
