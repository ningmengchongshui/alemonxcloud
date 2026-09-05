package cloud

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func adminPlanPriceTiers(c *gin.Context) {
	rows, err := instanceDB.QueryContext(c, `SELECT id,plan_id,months,discount_bps,enabled,created_at,updated_at FROM xcloud_plan_price_tiers ORDER BY plan_id,months`)
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()
	out := []planPriceTier{}
	for rows.Next() {
		var v planPriceTier
		if err := rows.Scan(&v.ID, &v.PlanID, &v.Months, &v.DiscountBps, &v.Enabled, &v.CreatedAt, &v.UpdatedAt); err != nil {
			internalError(c, err)
			return
		}
		out = append(out, v)
	}
	c.JSON(http.StatusOK, out)
}

func adminSavePlanPriceTier(c *gin.Context) {
	var v planPriceTier
	if c.ShouldBindJSON(&v) != nil || v.Months < 1 || v.Months > 60 || v.DiscountBps < 0 || v.DiscountBps > 10000 || strings.TrimSpace(v.PlanID) == "" {
		c.JSON(400, gin.H{"message": "阶梯价参数无效"})
		return
	}
	if v.ID == "" {
		v.ID = newID("tier")
	}
	now := time.Now()
	if v.CreatedAt.IsZero() {
		v.CreatedAt = now
	}
	v.UpdatedAt = now
	_, err := instanceDB.ExecContext(c, `INSERT INTO xcloud_plan_price_tiers (id,plan_id,months,discount_bps,enabled,created_at,updated_at) VALUES (?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE discount_bps=VALUES(discount_bps),enabled=VALUES(enabled),updated_at=VALUES(updated_at)`, v.ID, v.PlanID, v.Months, v.DiscountBps, v.Enabled, v.CreatedAt, v.UpdatedAt)
	if err != nil {
		internalError(c, err)
		return
	}
	user := c.MustGet("user").(oidcUser)
	_ = writeAudit(c, user.ID, "benefit.tier.save", "plan_price_tier", v.ID, map[string]any{"planId": v.PlanID, "months": v.Months})
	c.JSON(http.StatusOK, v)
}

func adminBenefitPrograms(c *gin.Context) {
	rows, err := instanceDB.QueryContext(c, benefitProgramSelect+` ORDER BY p.updated_at DESC`)
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()
	out := []benefitProgram{}
	for rows.Next() {
		v, e := scanBenefitProgram(rows)
		if e != nil {
			internalError(c, e)
			return
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func adminSaveBenefitProgram(c *gin.Context) {
	var v benefitProgram
	if c.ShouldBindJSON(&v) != nil {
		c.JSON(400, gin.H{"message": "权益方案参数无效"})
		return
	}
	if id := c.Param("id"); id != "" {
		v.ID = id
	}
	if err := validBenefitProgram(v); err != nil {
		businessError(c, err)
		return
	}
	user := c.MustGet("user").(oidcUser)
	now := time.Now()
	creating := v.ID == ""
	if creating {
		v.ID = newID("bp")
	}
	if v.Status == "active" && v.StartsAt != nil && v.StartsAt.After(now) {
		v.Status = "scheduled"
	}
	if v.CreatedAt.IsZero() {
		v.CreatedAt = now
	}
	v.UpdatedAt = now
	tx, err := beginSerializableTx(c)
	if err != nil {
		internalError(c, err)
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(c, `INSERT INTO xcloud_benefit_programs (id,name,goal,status,trigger_type,order_scope,benefit_type,benefit_value,min_amount_fen,plan_ids,month_values,audience_type,starts_at,ends_at,per_user_limit,total_limit,used_count,cash_budget_fen,cash_spent_fen,grant_days_limit,grant_days_used,priority,channel_label,created_by,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE name=VALUES(name),goal=VALUES(goal),status=VALUES(status),trigger_type=VALUES(trigger_type),order_scope=VALUES(order_scope),benefit_type=VALUES(benefit_type),benefit_value=VALUES(benefit_value),min_amount_fen=VALUES(min_amount_fen),plan_ids=VALUES(plan_ids),month_values=VALUES(month_values),audience_type=VALUES(audience_type),starts_at=VALUES(starts_at),ends_at=VALUES(ends_at),per_user_limit=VALUES(per_user_limit),total_limit=VALUES(total_limit),cash_budget_fen=VALUES(cash_budget_fen),grant_days_limit=VALUES(grant_days_limit),priority=VALUES(priority),channel_label=VALUES(channel_label),updated_at=VALUES(updated_at)`, v.ID, strings.TrimSpace(v.Name), v.Goal, v.Status, v.TriggerType, v.OrderScope, v.BenefitType, v.BenefitValue, v.MinAmountFen, jsonIDs(v.PlanIDs), jsonMonths(v.MonthValues), v.AudienceType, v.StartsAt, v.EndsAt, v.PerUserLimit, v.TotalLimit, 0, v.CashBudgetFen, 0, v.GrantDaysLimit, 0, v.Priority, nullableString(v.ChannelLabel), user.ID, v.CreatedAt, v.UpdatedAt)
	if err != nil {
		internalError(c, err)
		return
	}
	if v.TriggerType == "promo_code" && normalizeBenefitCode(v.Code) != "" {
		var existing string
		_ = tx.QueryRowContext(c, `SELECT id FROM xcloud_benefit_codes WHERE program_id=? FOR UPDATE`, v.ID).Scan(&existing)
		if existing == "" {
			existing = newID("code")
		}
		_, err = tx.ExecContext(c, `INSERT INTO xcloud_benefit_codes (id,program_id,code_hash,code_mask,enabled,total_limit,per_user_limit,used_count,starts_at,ends_at,channel_label,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE code_hash=VALUES(code_hash),code_mask=VALUES(code_mask),enabled=VALUES(enabled),total_limit=VALUES(total_limit),per_user_limit=VALUES(per_user_limit),starts_at=VALUES(starts_at),ends_at=VALUES(ends_at),channel_label=VALUES(channel_label)`, existing, v.ID, benefitCodeHash(v.Code), benefitCodeMask(v.Code), v.Status == "active" || v.Status == "scheduled", v.CodeTotalLimit, v.CodePerUserLimit, 0, v.StartsAt, v.EndsAt, nullableString(v.ChannelLabel), now)
		if err != nil {
			internalError(c, err)
			return
		}
		v.CodeMask = benefitCodeMask(v.Code)
	}
	if err = writeAuditTx(c, tx, user.ID, "benefit.program.save", "benefit_program", v.ID, map[string]any{"goal": v.Goal, "status": v.Status, "creating": creating}); err != nil {
		internalError(c, err)
		return
	}
	if err = tx.Commit(); err != nil {
		internalError(c, err)
		return
	}
	v.Code = ""
	c.JSON(http.StatusOK, v)
}

func adminGrantBenefit(c *gin.Context) {
	var body struct {
		OwnerIDs  []string   `json:"ownerIds"`
		ExpiresAt *time.Time `json:"expiresAt"`
	}
	if c.ShouldBindJSON(&body) != nil || len(body.OwnerIDs) == 0 {
		c.JSON(400, gin.H{"message": "请选择至少一名用户"})
		return
	}
	programID := c.Param("id")
	user := c.MustGet("user").(oidcUser)
	var trigger string
	if err := instanceDB.QueryRowContext(c, `SELECT trigger_type FROM xcloud_benefit_programs WHERE id=?`, programID).Scan(&trigger); err != nil {
		businessError(c, errors.New("权益方案不存在"))
		return
	}
	if trigger != "targeted" {
		businessError(c, errors.New("只有定向权益方案可以发放"))
		return
	}
	created := 0
	for _, ownerID := range body.OwnerIDs {
		ownerID = strings.TrimSpace(ownerID)
		if ownerID == "" {
			continue
		}
		result, err := instanceDB.ExecContext(c, `INSERT IGNORE INTO xcloud_benefit_grants (id,program_id,owner_id,status,expires_at,issued_by,created_at) VALUES (?,?,?,'available',?,?,NOW())`, newID("grant"), programID, ownerID, body.ExpiresAt, user.ID)
		if err != nil {
			internalError(c, err)
			return
		}
		n, _ := result.RowsAffected()
		created += int(n)
	}
	_ = writeAudit(c, user.ID, "benefit.grant", "benefit_program", programID, map[string]any{"count": created})
	c.JSON(http.StatusOK, gin.H{"created": created})
}

func adminBenefitGrants(c *gin.Context) {
	rows, err := instanceDB.QueryContext(c, `SELECT id,program_id,owner_id,status,expires_at,used_order_id,used_at,voided_at,created_at FROM xcloud_benefit_grants WHERE program_id=? ORDER BY created_at DESC LIMIT 500`, c.Param("id"))
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, programID, ownerID, status string
		var expires, used, voided sql.NullTime
		var orderID sql.NullString
		var created time.Time
		if err = rows.Scan(&id, &programID, &ownerID, &status, &expires, &orderID, &used, &voided, &created); err != nil {
			internalError(c, err)
			return
		}
		out = append(out, map[string]any{"id": id, "programId": programID, "ownerId": ownerID, "status": status, "expiresAt": expires, "usedOrderId": orderID, "usedAt": used, "voidedAt": voided, "createdAt": created})
	}
	c.JSON(http.StatusOK, out)
}

func adminVoidBenefitGrant(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	r, err := instanceDB.ExecContext(c, `UPDATE xcloud_benefit_grants SET status='voided',voided_at=NOW(),voided_by=? WHERE id=? AND program_id=? AND status='available'`, user.ID, c.Param("grantID"), c.Param("id"))
	if err != nil {
		internalError(c, err)
		return
	}
	n, _ := r.RowsAffected()
	if n != 1 {
		businessError(c, errors.New("权益已使用、过期或不存在"))
		return
	}
	_ = writeAudit(c, user.ID, "benefit.grant.void", "benefit_grant", c.Param("grantID"), nil)
	c.Status(http.StatusNoContent)
}

func adminBenefitRedemptions(c *gin.Context) {
	rows, err := instanceDB.QueryContext(c, `SELECT id,program_id,COALESCE(code_id,''),COALESCE(grant_id,''),owner_id,order_id,discount_amount_fen,bonus_days,created_at FROM xcloud_benefit_redemptions ORDER BY created_at DESC LIMIT 500`)
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, programID, codeID, grantID, ownerID, orderID string
		var discount, days int
		var created time.Time
		if err = rows.Scan(&id, &programID, &codeID, &grantID, &ownerID, &orderID, &discount, &days, &created); err != nil {
			internalError(c, err)
			return
		}
		out = append(out, map[string]any{"id": id, "programId": programID, "codeId": codeID, "grantId": grantID, "ownerId": ownerID, "orderId": orderID, "discountAmountFen": discount, "bonusDays": days, "createdAt": created})
	}
	c.JSON(http.StatusOK, out)
}
