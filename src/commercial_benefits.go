package cloud

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Price tiers are product pricing. Benefit programs are a separate, optional
// commercial layer that is evaluated once for both purchase and renewal.
type planPriceTier struct {
	ID          string    `json:"id"`
	PlanID      string    `json:"planId"`
	Months      int       `json:"months"`
	DiscountBps int       `json:"discountBps"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type benefitProgram struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Goal             string     `json:"goal"`
	Status           string     `json:"status"`
	TriggerType      string     `json:"triggerType"`
	OrderScope       string     `json:"orderScope"`
	BenefitType      string     `json:"benefitType"`
	BenefitValue     int        `json:"benefitValue"`
	MinAmountFen     int        `json:"minAmountFen"`
	PlanIDs          []string   `json:"planIds"`
	MonthValues      []int      `json:"monthValues"`
	AudienceType     string     `json:"audienceType"`
	StartsAt         *time.Time `json:"startsAt,omitempty"`
	EndsAt           *time.Time `json:"endsAt,omitempty"`
	PerUserLimit     int        `json:"perUserLimit"`
	TotalLimit       int        `json:"totalLimit"`
	UsedCount        int        `json:"usedCount"`
	CashBudgetFen    int        `json:"cashBudgetFen"`
	CashSpentFen     int        `json:"cashSpentFen"`
	GrantDaysLimit   int        `json:"grantDaysLimit"`
	GrantDaysUsed    int        `json:"grantDaysUsed"`
	Priority         int        `json:"priority"`
	ChannelLabel     string     `json:"channelLabel,omitempty"`
	Code             string     `json:"code,omitempty"`
	CodeMask         string     `json:"codeMask,omitempty"`
	CodeTotalLimit   int        `json:"codeTotalLimit,omitempty"`
	CodePerUserLimit int        `json:"codePerUserLimit,omitempty"`
	CodeEnabled      bool       `json:"codeEnabled"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

type benefitQuote struct {
	ListAmountFen     int    `json:"listAmountFen"`
	DiscountAmountFen int    `json:"discountAmountFen"`
	AmountFen         int    `json:"amountFen"`
	BonusDays         int    `json:"bonusDays"`
	TierMonths        int    `json:"tierMonths,omitempty"`
	QuoteSummary      string `json:"quoteSummary"`
	Program           *struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Goal        string `json:"goal"`
		BenefitType string `json:"benefitType"`
		TriggerType string `json:"triggerType"`
		CodeMask    string `json:"codeMask,omitempty"`
	} `json:"program,omitempty"`

	program *benefitProgram
	codeID  string
	grantID string
}

func benefitCodeHash(value string) string {
	s := sha256.Sum256([]byte(env("XCLOUD_BENEFIT_CODE_SECRET", env("XCLOUD_COUPON_CODE_SECRET", "xcloud-dev-benefit-secret")) + "\x00" + normalizeBenefitCode(value)))
	return hex.EncodeToString(s[:])
}
func normalizeBenefitCode(value string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(value), " ", ""))
}
func benefitCodeMask(value string) string {
	v := normalizeBenefitCode(value)
	if len(v) <= 4 {
		return "****"
	}
	return v[:2] + strings.Repeat("*", len(v)-4) + v[len(v)-2:]
}

func validBenefitProgram(v benefitProgram) error {
	v.Name = strings.TrimSpace(v.Name)
	if v.Name == "" || len(v.Name) > 128 {
		return errors.New("权益方案名称长度应为 1 至 128")
	}
	if !oneOf(v.Goal, "first_purchase", "multi_month", "renewal_recovery", "channel") {
		return errors.New("权益目标无效")
	}
	if !oneOf(v.Status, "draft", "scheduled", "active", "paused", "ended") {
		return errors.New("权益状态无效")
	}
	if !oneOf(v.TriggerType, "automatic", "promo_code", "targeted") {
		return errors.New("触发方式无效")
	}
	if !oneOf(v.AudienceType, "all", "first_paid", "first_plan", "expiring", "lapsed", "targeted") {
		return errors.New("适用人群无效")
	}
	if !oneOf(v.OrderScope, "purchase", "renewal", "both") {
		return errors.New("适用订单无效")
	}
	if !oneOf(v.BenefitType, "fixed_discount", "percent_discount", "bonus_days") {
		return errors.New("权益类型无效")
	}
	if v.BenefitValue <= 0 || (v.BenefitType == "percent_discount" && v.BenefitValue > 10000) {
		return errors.New("权益值无效")
	}
	if v.MinAmountFen < 0 || v.PerUserLimit < 0 || v.TotalLimit < 0 || v.CashBudgetFen < 0 || v.GrantDaysLimit < 0 {
		return errors.New("权益限制无效")
	}
	if v.EndsAt != nil && v.StartsAt != nil && !v.EndsAt.After(*v.StartsAt) {
		return errors.New("结束时间必须晚于开始时间")
	}
	if v.TriggerType == "promo_code" && normalizeBenefitCode(v.Code) == "" && v.ID == "" {
		return errors.New("推广码不能为空")
	}
	return nil
}
func oneOf(value string, values ...string) bool {
	for _, item := range values {
		if value == item {
			return true
		}
	}
	return false
}

func jsonIDs(value []string) []byte { b, _ := json.Marshal(value); return b }
func jsonMonths(value []int) []byte { b, _ := json.Marshal(value); return b }

func scanBenefitProgram(scanner interface{ Scan(...any) error }) (benefitProgram, error) {
	var p benefitProgram
	var plans, months []byte
	var starts, ends sql.NullTime
	var channel, codeMask sql.NullString
	err := scanner.Scan(&p.ID, &p.Name, &p.Goal, &p.Status, &p.TriggerType, &p.OrderScope, &p.BenefitType, &p.BenefitValue, &p.MinAmountFen, &plans, &months, &p.AudienceType, &starts, &ends, &p.PerUserLimit, &p.TotalLimit, &p.UsedCount, &p.CashBudgetFen, &p.CashSpentFen, &p.GrantDaysLimit, &p.GrantDaysUsed, &p.Priority, &channel, &p.CreatedAt, &p.UpdatedAt, &codeMask, &p.CodeTotalLimit, &p.CodePerUserLimit, &p.CodeEnabled)
	if err != nil {
		return p, err
	}
	_ = json.Unmarshal(plans, &p.PlanIDs)
	_ = json.Unmarshal(months, &p.MonthValues)
	if starts.Valid {
		p.StartsAt = &starts.Time
	}
	if ends.Valid {
		p.EndsAt = &ends.Time
	}
	if channel.Valid {
		p.ChannelLabel = channel.String
	}
	if codeMask.Valid {
		p.CodeMask = codeMask.String
	}
	return p, nil
}

const benefitProgramSelect = `SELECT p.id,p.name,p.goal,p.status,p.trigger_type,p.order_scope,p.benefit_type,p.benefit_value,p.min_amount_fen,p.plan_ids,p.month_values,p.audience_type,p.starts_at,p.ends_at,p.per_user_limit,p.total_limit,p.used_count,p.cash_budget_fen,p.cash_spent_fen,p.grant_days_limit,p.grant_days_used,p.priority,p.channel_label,p.created_at,p.updated_at,COALESCE(c.code_mask,''),COALESCE(c.total_limit,0),COALESCE(c.per_user_limit,0),COALESCE(c.enabled,FALSE) FROM xcloud_benefit_programs p LEFT JOIN xcloud_benefit_codes c ON c.program_id=p.id`

func tierPrice(ctx context.Context, tx *sql.Tx, planID string, months, monthly int, lock bool) (int, bool, error) {
	var discountBps int
	query := `SELECT discount_bps FROM xcloud_plan_price_tiers WHERE plan_id=? AND months=? AND enabled=TRUE`
	var err error
	if tx != nil {
		if lock {
			query += ` FOR UPDATE`
		}
		err = tx.QueryRowContext(ctx, query, planID, months).Scan(&discountBps)
	} else {
		err = instanceDB.QueryRowContext(ctx, query, planID, months).Scan(&discountBps)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return monthly * months, false, nil
	}
	if discountBps < 0 || discountBps > 10000 {
		return 0, false, errors.New("套餐阶梯折扣无效")
	}
	return monthly * months * discountBps / 10000, err == nil, err
}

func benefitDiscount(p benefitProgram, list int) (discount, days int) {
	switch p.BenefitType {
	case "fixed_discount":
		discount = p.BenefitValue
	case "percent_discount":
		discount = list * p.BenefitValue / 10000
	case "bonus_days":
		days = p.BenefitValue
	}
	if discount > list {
		discount = list
	}
	return discount, days
}
func containsString(items []string, value string) bool {
	if len(items) == 0 {
		return true
	}
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}
func containsMonth(items []int, value int) bool {
	if len(items) == 0 {
		return true
	}
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func programEligible(ctx context.Context, tx *sql.Tx, p benefitProgram, ownerID, scope, planID, renewalInstanceID string, months, list int, grantID *string, lock bool) (bool, error) {
	now := time.Now()
	if (p.Status != "active" && p.Status != "scheduled") || (p.OrderScope != "both" && p.OrderScope != scope) || (p.StartsAt != nil && now.Before(*p.StartsAt)) || (p.EndsAt != nil && !now.Before(*p.EndsAt)) || list < p.MinAmountFen || !containsString(p.PlanIDs, planID) || !containsMonth(p.MonthValues, months) || (p.TotalLimit > 0 && p.UsedCount >= p.TotalLimit) {
		return false, nil
	}
	discount, days := benefitDiscount(p, list)
	if (p.CashBudgetFen > 0 && p.CashSpentFen+discount > p.CashBudgetFen) || (p.GrantDaysLimit > 0 && p.GrantDaysUsed+days > p.GrantDaysLimit) {
		return false, nil
	}
	if p.PerUserLimit > 0 {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM xcloud_benefit_redemptions WHERE program_id=? AND owner_id=?`, p.ID, ownerID).Scan(&n); err != nil {
			return false, err
		}
		if n >= p.PerUserLimit {
			return false, nil
		}
	}
	if p.AudienceType == "first_paid" {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM xcloud_orders WHERE owner_id=? AND payment_source='wallet' AND status NOT IN ('cancelled','rejected')`, ownerID).Scan(&n); err != nil {
			return false, err
		}
		if n > 0 {
			return false, nil
		}
	}
	if p.AudienceType == "first_plan" {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM xcloud_orders WHERE owner_id=? AND plan_id=? AND payment_source='wallet' AND status NOT IN ('cancelled','rejected')`, ownerID, planID).Scan(&n); err != nil {
			return false, err
		}
		if n > 0 {
			return false, nil
		}
	}
	if p.AudienceType == "expiring" {
		var n int
		if renewalInstanceID == "" {
			return false, nil
		}
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM xcloud_instances WHERE id=? AND owner_id=? AND expires_at>NOW() AND expires_at<=DATE_ADD(NOW(),INTERVAL 7 DAY)`, renewalInstanceID, ownerID).Scan(&n); err != nil {
			return false, err
		}
		if n == 0 {
			return false, nil
		}
	}
	if p.AudienceType == "lapsed" {
		var n int
		if renewalInstanceID == "" {
			return false, nil
		}
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM xcloud_instances WHERE id=? AND owner_id=? AND expires_at<=NOW()`, renewalInstanceID, ownerID).Scan(&n); err != nil {
			return false, err
		}
		if n == 0 {
			return false, nil
		}
	}
	if p.TriggerType == "targeted" || p.AudienceType == "targeted" {
		var id string
		query := `SELECT id FROM xcloud_benefit_grants WHERE program_id=? AND owner_id=? AND status='available' AND (expires_at IS NULL OR expires_at>NOW())`
		if lock {
			query += ` FOR UPDATE`
		}
		if err := tx.QueryRowContext(ctx, query, p.ID, ownerID).Scan(&id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return false, nil
			}
			return false, err
		}
		*grantID = id
	}
	return true, nil
}

// quoteCommercialBenefit is also called inside the settlement transaction. It
// intentionally has no "full price" or user-selected benefit escape hatch.
func quoteCommercialBenefit(ctx context.Context, ownerID, scope, planID, renewalInstanceID string, months, monthly int, promoCode string, tx *sql.Tx, lock bool) (benefitQuote, error) {
	if tx == nil {
		return benefitQuote{}, errors.New("权益报价需要事务上下文")
	}
	list, tiered, err := tierPrice(ctx, tx, planID, months, monthly, lock)
	if err != nil {
		return benefitQuote{}, err
	}
	q := benefitQuote{ListAmountFen: list, AmountFen: list}
	if tiered {
		q.TierMonths = months
	}
	programQuery := benefitProgramSelect + ` WHERE p.status IN ('active','scheduled') ORDER BY p.priority DESC,p.created_at ASC`
	if lock {
		programQuery += ` FOR UPDATE`
	}
	rows, err := tx.QueryContext(ctx, programQuery)
	if err != nil {
		return q, err
	}
	defer rows.Close()
	programs := []benefitProgram{}
	for rows.Next() {
		p, e := scanBenefitProgram(rows)
		if e != nil {
			return q, e
		}
		programs = append(programs, p)
	}
	if err := rows.Err(); err != nil {
		return q, err
	}
	promoCode = normalizeBenefitCode(promoCode)
	if promoCode != "" {
		var matched *benefitProgram
		var codeID string
		for i := range programs {
			p := &programs[i]
			if p.TriggerType != "promo_code" {
				continue
			}
			var id string
			codeQuery := `SELECT id FROM xcloud_benefit_codes WHERE program_id=? AND code_hash=? AND enabled=TRUE AND (starts_at IS NULL OR starts_at<=NOW()) AND (ends_at IS NULL OR ends_at>NOW())`
			if lock {
				codeQuery += ` FOR UPDATE`
			}
			err := tx.QueryRowContext(ctx, codeQuery, p.ID, benefitCodeHash(promoCode)).Scan(&id)
			if err == nil {
				matched = p
				codeID = id
				break
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return q, err
			}
		}
		if matched == nil {
			return q, errors.New("推广码无效或当前订单不适用")
		}
		grantID := ""
		ok, err := programEligible(ctx, tx, *matched, ownerID, scope, planID, renewalInstanceID, months, list, &grantID, lock)
		if err != nil {
			return q, err
		}
		if !ok {
			return q, errors.New("推广码当前订单不适用")
		}
		q.program, q.codeID, q.grantID = matched, codeID, grantID
	} else {
		matches := []benefitQuote{}
		for i := range programs {
			p := &programs[i]
			if p.TriggerType == "promo_code" {
				continue
			}
			grantID := ""
			ok, err := programEligible(ctx, tx, *p, ownerID, scope, planID, renewalInstanceID, months, list, &grantID, lock)
			if err != nil {
				return q, err
			}
			if !ok {
				continue
			}
			matches = append(matches, benefitQuote{program: p, grantID: grantID})
		}
		sort.SliceStable(matches, func(i, j int) bool {
			if matches[i].program.Priority != matches[j].program.Priority {
				return matches[i].program.Priority > matches[j].program.Priority
			}
			a, _ := benefitDiscount(*matches[i].program, list)
			b, _ := benefitDiscount(*matches[j].program, list)
			return a > b
		})
		if len(matches) > 0 {
			q.program, q.grantID = matches[0].program, matches[0].grantID
		}
	}
	if q.program != nil {
		q.DiscountAmountFen, q.BonusDays = benefitDiscount(*q.program, list)
		q.AmountFen = list - q.DiscountAmountFen
		q.Program = &struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Goal        string `json:"goal"`
			BenefitType string `json:"benefitType"`
			TriggerType string `json:"triggerType"`
			CodeMask    string `json:"codeMask,omitempty"`
		}{q.program.ID, q.program.Name, q.program.Goal, q.program.BenefitType, q.program.TriggerType, q.program.CodeMask}
	}
	q.QuoteSummary = fmt.Sprintf("基础价 %d 分，优惠 %d 分，赠送 %d 天", q.ListAmountFen, q.DiscountAmountFen, q.BonusDays)
	return q, nil
}

func consumeCommercialBenefitTx(ctx context.Context, tx *sql.Tx, ownerID, orderID string, q benefitQuote) error {
	if q.program == nil {
		return nil
	}
	p := q.program
	if p.PerUserLimit > 0 {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM xcloud_benefit_redemptions WHERE program_id=? AND owner_id=? FOR UPDATE`, p.ID, ownerID).Scan(&n); err != nil {
			return err
		}
		if n >= p.PerUserLimit {
			return errors.New("该权益已达到你的使用上限")
		}
	}
	r, err := tx.ExecContext(ctx, `UPDATE xcloud_benefit_programs SET used_count=used_count+1,cash_spent_fen=cash_spent_fen+?,grant_days_used=grant_days_used+?,updated_at=NOW() WHERE id=? AND status IN ('active','scheduled') AND (starts_at IS NULL OR starts_at<=NOW()) AND (ends_at IS NULL OR ends_at>NOW()) AND (total_limit=0 OR used_count<total_limit) AND (cash_budget_fen=0 OR cash_spent_fen+?<=cash_budget_fen) AND (grant_days_limit=0 OR grant_days_used+?<=grant_days_limit)`, q.DiscountAmountFen, q.BonusDays, p.ID, q.DiscountAmountFen, q.BonusDays)
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n != 1 {
		return errors.New("权益已暂停、用尽或预算不足")
	}
	if q.codeID != "" {
		if p.CodePerUserLimit > 0 {
			var used int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM xcloud_benefit_redemptions WHERE code_id=? AND owner_id=? FOR UPDATE`, q.codeID, ownerID).Scan(&used); err != nil {
				return err
			}
			if used >= p.CodePerUserLimit {
				return errors.New("该推广码已达到你的使用上限")
			}
		}
		r, err = tx.ExecContext(ctx, `UPDATE xcloud_benefit_codes SET used_count=used_count+1 WHERE id=? AND enabled=TRUE AND (starts_at IS NULL OR starts_at<=NOW()) AND (ends_at IS NULL OR ends_at>NOW()) AND (total_limit=0 OR used_count<total_limit)`, q.codeID)
		if err != nil {
			return err
		}
		if n, _ := r.RowsAffected(); n != 1 {
			return errors.New("推广码已失效或已用完")
		}
	}
	if q.grantID != "" {
		r, err = tx.ExecContext(ctx, `UPDATE xcloud_benefit_grants SET status='used',used_order_id=?,used_at=NOW() WHERE id=? AND owner_id=? AND status='available'`, orderID, q.grantID, ownerID)
		if err != nil {
			return err
		}
		if n, _ := r.RowsAffected(); n != 1 {
			return errors.New("定向权益已失效")
		}
	}
	snapshot, _ := json.Marshal(map[string]any{"id": p.ID, "name": p.Name, "goal": p.Goal, "triggerType": p.TriggerType, "benefitType": p.BenefitType, "priority": p.Priority, "channelLabel": p.ChannelLabel, "discountAmountFen": q.DiscountAmountFen, "bonusDays": q.BonusDays, "codeMask": p.CodeMask})
	redemptionID := newID("ben")
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_benefit_redemptions (id,program_id,code_id,grant_id,owner_id,order_id,discount_amount_fen,bonus_days,created_at) VALUES (?,?,?,?,?,?,?,?,NOW())`, redemptionID, p.ID, nullableString(q.codeID), nullableString(q.grantID), ownerID, orderID, q.DiscountAmountFen, q.BonusDays); err != nil {
		return err
	}
	tierSnapshot, _ := json.Marshal(map[string]any{"months": q.TierMonths, "listAmountFen": q.ListAmountFen})
	_, err = tx.ExecContext(ctx, `UPDATE xcloud_orders SET benefit_program_id=?,benefit_snapshot=?,price_tier_snapshot=?,benefit_trigger=?,benefit_priority=?,benefit_channel_label=?,bonus_days=?,promo_code_mask=? WHERE id=?`, p.ID, snapshot, tierSnapshot, p.TriggerType, p.Priority, nullableString(p.ChannelLabel), q.BonusDays, nullableString(p.CodeMask), orderID)
	return err
}

func fmtBenefit(p *benefitProgram) string {
	if p == nil {
		return ""
	}
	if p.BenefitType == "bonus_days" {
		return fmt.Sprintf("赠送 %d 天", p.BenefitValue)
	}
	return p.Name
}
