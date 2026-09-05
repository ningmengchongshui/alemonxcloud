package cloud

import (
	"database/sql"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
	"time"
)

func getCouponBatch(ctx *gin.Context, tx *sql.Tx, id string, lock bool) (couponBatch, error) {
	q := couponBatchSelect + ` WHERE id=?`
	if lock {
		q += ` FOR UPDATE`
	}
	if tx != nil {
		return scanCouponBatch(tx.QueryRowContext(ctx, q, id))
	}
	return scanCouponBatch(instanceDB.QueryRowContext(ctx, q, id))
}
func publicCouponBatchesHandler(c *gin.Context) {
	items, e := listCouponBatches(c, true)
	if e != nil {
		internalError(c, e)
		return
	}
	c.JSON(200, items)
}
func myCouponsHandler(c *gin.Context) {
	u := c.MustGet("user").(oidcUser)
	items, e := listUserCoupons(c, u.ID)
	if e != nil {
		internalError(c, e)
		return
	}
	c.JSON(200, items)
}
func claimCouponBatchHandler(c *gin.Context) {
	u := c.MustGet("user").(oidcUser)
	tx, e := beginSerializableTx(c)
	if e != nil {
		internalError(c, e)
		return
	}
	defer tx.Rollback()
	b, e := getCouponBatch(c, tx, c.Param("id"), true)
	if e != nil || b.Status != "active" || b.DistributionMode != "public" || (b.StartsAt != nil && time.Now().Before(*b.StartsAt)) || (b.EndsAt != nil && !time.Now().Before(*b.EndsAt)) {
		businessError(c, errors.New("当前券批次不可领取"))
		return
	}
	run, items, e := issueUserCoupons(c, tx, b, []string{u.ID}, u.ID, "public")
	if e != nil {
		internalError(c, e)
		return
	}
	if items[0]["status"] != "issued" {
		businessError(c, errors.New(fmtString(items[0]["reason"], "领取失败")))
		return
	}
	if e = writeAuditTx(c, tx, u.ID, "coupon.claim", "coupon_batch", b.ID, map[string]any{"runId": run}); e != nil {
		internalError(c, e)
		return
	}
	if e = tx.Commit(); e != nil {
		internalError(c, e)
		return
	}
	_ = createNotification(c, u.ID, "promotion", "代金券已领取", "已放入优惠中心，可在结算时选择使用。", map[string]any{"batchId": b.ID})
	c.JSON(http.StatusCreated, items[0])
}
func fmtString(v any, fallback string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return fallback
}
func adminCouponBatches(c *gin.Context) {
	items, e := listCouponBatches(c, false)
	if e != nil {
		internalError(c, e)
		return
	}
	c.JSON(200, items)
}
func adminSaveCouponBatch(c *gin.Context) {
	var v couponBatch
	if c.ShouldBindJSON(&v) != nil {
		c.JSON(400, gin.H{"message": "券批次参数无效"})
		return
	}
	if id := c.Param("id"); id != "" {
		v.ID = id
	}
	v.Name = strings.TrimSpace(v.Name)
	if e := validCouponBatch(v); e != nil {
		businessError(c, e)
		return
	}
	u := c.MustGet("user").(oidcUser)
	now := time.Now()
	if v.ID == "" {
		v.ID = newID("cb")
		v.CreatedBy = u.ID
		v.CreatedAt = now
	}
	v.UpdatedAt = now
	_, e := instanceDB.ExecContext(c, `INSERT INTO xcloud_coupon_batches (id,name,status,distribution_mode,discount_type,discount_value,min_amount_fen,max_discount_fen,scope,plan_ids,image_ids,month_values,starts_at,ends_at,issue_limit,per_user_limit,issued_count,created_by,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE name=VALUES(name),status=VALUES(status),distribution_mode=VALUES(distribution_mode),discount_type=VALUES(discount_type),discount_value=VALUES(discount_value),min_amount_fen=VALUES(min_amount_fen),max_discount_fen=VALUES(max_discount_fen),scope=VALUES(scope),plan_ids=VALUES(plan_ids),image_ids=VALUES(image_ids),month_values=VALUES(month_values),starts_at=VALUES(starts_at),ends_at=VALUES(ends_at),issue_limit=VALUES(issue_limit),per_user_limit=VALUES(per_user_limit),updated_at=VALUES(updated_at)`, v.ID, v.Name, v.Status, v.DistributionMode, v.DiscountType, v.DiscountValue, v.MinAmountFen, v.MaxDiscountFen, v.Scope, jsonStringSlice(v.PlanIDs), jsonStringSlice(v.ImageIDs), jsonStringSlice(v.MonthValues), v.StartsAt, v.EndsAt, v.IssueLimit, v.PerUserLimit, 0, v.CreatedBy, v.CreatedAt, v.UpdatedAt)
	if e != nil {
		internalError(c, e)
		return
	}
	_ = writeAudit(c, u.ID, "coupon_batch.save", "coupon_batch", v.ID, map[string]any{"status": v.Status, "mode": v.DistributionMode})
	c.JSON(200, v)
}
func adminIssueCouponBatch(c *gin.Context) {
	var body struct {
		OwnerIDs []string `json:"ownerIds"`
	}
	if c.ShouldBindJSON(&body) != nil || len(body.OwnerIDs) == 0 || len(body.OwnerIDs) > 100 {
		c.JSON(400, gin.H{"message": "请选择 1 至 100 位用户"})
		return
	}
	u := c.MustGet("user").(oidcUser)
	tx, e := beginSerializableTx(c)
	if e != nil {
		internalError(c, e)
		return
	}
	defer tx.Rollback()
	b, e := getCouponBatch(c, tx, c.Param("id"), true)
	if e != nil {
		businessError(c, errors.New("券批次不存在"))
		return
	}
	if b.DistributionMode != "targeted" {
		businessError(c, errors.New("该券批次不是定向发放"))
		return
	}
	run, items, e := issueUserCoupons(c, tx, b, body.OwnerIDs, u.ID, "targeted")
	if e != nil {
		internalError(c, e)
		return
	}
	if e = writeAuditTx(c, tx, u.ID, "coupon_batch.issue", "coupon_batch", b.ID, map[string]any{"runId": run, "count": len(body.OwnerIDs)}); e != nil {
		internalError(c, e)
		return
	}
	if e = tx.Commit(); e != nil {
		internalError(c, e)
		return
	}
	for _, item := range items {
		if item["status"] == "issued" {
			_ = createNotification(c, item["ownerId"].(string), "promotion", "收到代金券", "管理员向你发放了一张代金券。", map[string]any{"batchId": b.ID, "couponId": item["couponId"]})
		}
	}
	c.JSON(200, gin.H{"runId": run, "items": items})
}
func adminVoidCouponBatch(c *gin.Context) {
	u := c.MustGet("user").(oidcUser)
	r, e := instanceDB.ExecContext(c, `UPDATE xcloud_user_coupons SET status='voided',voided_at=NOW() WHERE batch_id=? AND status='available'`, c.Param("id"))
	if e != nil {
		internalError(c, e)
		return
	}
	n, _ := r.RowsAffected()
	_ = writeAudit(c, u.ID, "coupon_batch.void_unused", "coupon_batch", c.Param("id"), map[string]any{"count": n})
	c.JSON(200, gin.H{"voidedCount": n})
}
func adminCouponUserSearch(c *gin.Context) {
	q := "%" + strings.TrimSpace(c.Query("q")) + "%"
	rows, e := instanceDB.QueryContext(c, `SELECT id,username,email FROM xcloud_users WHERE id LIKE ? OR username LIKE ? OR email LIKE ? ORDER BY last_login_at DESC LIMIT 50`, q, q, q)
	if e != nil {
		internalError(c, e)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id, name, email string
		if e = rows.Scan(&id, &name, &email); e != nil {
			internalError(c, e)
			return
		}
		out = append(out, gin.H{"id": id, "username": name, "email": email})
	}
	c.JSON(http.StatusOK, out)
}
