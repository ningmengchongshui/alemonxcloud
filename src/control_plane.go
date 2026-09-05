package cloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	orderPending = "pending_payment"
	orderReview  = "pending_review"
	orderDeploy  = "deploying"
	orderActive  = "active"
	orderExpired = "expired"
	orderCancel  = "cancelled"
	orderReject  = "rejected"
	orderRefund  = "refunded"

	taskPending = "pending"
	taskRunning = "running"
	taskDone    = "succeeded"
	taskFailed  = "failed"
	taskReview  = "needs_review"
)

const taskLeaseDuration = 5 * time.Minute

// instanceStateExecutor is shared by *sql.DB and *sql.Tx so lifecycle
// transitions remain conditional even while a caller owns a larger business
// transaction (purchase, renewal, or refund).
type instanceStateExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

var errInstanceStateConflict = errors.New("实例状态已变化")

func canTransitionInstance(from, to string) bool {
	if from == to {
		return true
	}
	allowed := map[string]map[string]bool{
		"deploying":         {"running": true, "deployment_failed": true},
		"deployment_failed": {"deploying": true},
		"running":           {"stopped": true, "destroy_scheduled": true},
		"stopped":           {"running": true, "destroy_scheduled": true},
		"destroy_scheduled": {"running": true, "stopped": true, "destroyed": true},
		"destroyed":         {"purged": true},
	}
	return allowed[from][to]
}

// transitionInstance is the sole writer for xcloud_instances.status after an
// instance has been created. It validates every expected edge before issuing a
// conditional update, so a stale task cannot overwrite a renewal, cancellation
// or a newer lifecycle operation.
func transitionInstance(ctx context.Context, executor instanceStateExecutor, instanceID string, expected []string, next string, runtimeStatus *string, extraSet string, extraArgs ...any) (bool, error) {
	if len(expected) == 0 {
		return false, errors.New("缺少实例当前状态")
	}
	for _, current := range expected {
		if !canTransitionInstance(current, next) {
			return false, fmt.Errorf("非法实例状态转换: %s -> %s", current, next)
		}
	}
	sets, args := []string{"status=?"}, []any{next}
	if runtimeStatus != nil {
		sets = append(sets, "runtime_status=?")
		args = append(args, *runtimeStatus)
	}
	if strings.TrimSpace(extraSet) != "" {
		sets = append(sets, extraSet)
		args = append(args, extraArgs...)
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(expected)), ",")
	args = append(args, instanceID)
	for _, current := range expected {
		args = append(args, current)
	}
	result, err := executor.ExecContext(ctx, "UPDATE xcloud_instances SET "+strings.Join(sets, ",")+" WHERE id=? AND status IN ("+placeholders+")", args...)
	if err != nil {
		return false, err
	}
	affected, _ := result.RowsAffected()
	return affected == 1, nil
}

type catalogImage struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	ImageRef    string         `json:"imageRef"`
	ImageDigest string         `json:"imageDigest"`
	Version     string         `json:"version"`
	Enabled     bool           `json:"enabled"`
	CreatedAt   time.Time      `json:"createdAt"`
	Versions    []imageVersion `json:"versions,omitempty"`
}
type imageVersion struct {
	ID          string     `json:"id"`
	ImageID     string     `json:"imageId"`
	Tag         string     `json:"tag"`
	ImageDigest string     `json:"imageDigest"`
	Enabled     bool       `json:"enabled"`
	Status      string     `json:"status"`
	LastError   string     `json:"lastError,omitempty"`
	PublishedAt *time.Time `json:"publishedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}
type publicImageVersion struct {
	Tag string `json:"tag"`
}
type publicCatalogImage struct {
	ID       string               `json:"id"`
	Name     string               `json:"name"`
	Versions []publicImageVersion `json:"versions"`
}

type plan struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	CPU           float64     `json:"cpu"`
	MemoryMB      int         `json:"memoryMB"`
	BandwidthMbps int         `json:"bandwidthMbps"`
	MonthlyFen    int         `json:"monthlyPriceFen"`
	Enabled       bool        `json:"enabled"`
	SortOrder     int         `json:"sortOrder"`
	CreatedAt     time.Time   `json:"createdAt"`
	TierDiscounts map[int]int `json:"tierDiscounts,omitempty"`
}

type node struct {
	ID                    string     `json:"id"`
	Name                  string     `json:"name"`
	AgentURL              string     `json:"agentURL"`
	CPUTotal              float64    `json:"cpuTotal"`
	MemoryTotalMB         int        `json:"memoryTotalMB"`
	CPUDetected           float64    `json:"cpuDetected"`
	MemoryDetectedMB      int        `json:"memoryDetectedMB"`
	Enabled               bool       `json:"enabled"`
	LastHeartbeatAt       *time.Time `json:"lastHeartbeatAt"`
	CPUReserved           float64    `json:"cpuReserved"`
	MemoryReservedMB      int        `json:"memoryReservedMB"`
	DockerVersion         string     `json:"dockerVersion,omitempty"`
	DiskAvailableBytes    int64      `json:"diskAvailableBytes,omitempty"`
	ManagedContainerCount int        `json:"managedContainerCount,omitempty"`
	AgentVersion          string     `json:"agentVersion,omitempty"`
	AgentAPIVersion       int        `json:"agentApiVersion,omitempty"`
	AgentCapabilities     []string   `json:"agentCapabilities,omitempty"`
	AgentCompatibility    string     `json:"agentCompatibility,omitempty"`
	AgentToken            string     `json:"agentToken,omitempty"`
	OfflineInstanceCount  int        `json:"offlineInstanceCount,omitempty"`
	PendingCleanupTasks   int        `json:"pendingCleanupTasks,omitempty"`
	LastAgentError        string     `json:"lastAgentError,omitempty"`
}

const minimumAgentAPIVersion = 1

func (n node) supportsAgentCapability(capability string) bool {
	for _, value := range n.AgentCapabilities {
		if value == capability {
			return true
		}
	}
	return false
}

func (n node) compatibility() string {
	if n.AgentAPIVersion == 0 {
		return "legacy"
	}
	if n.AgentAPIVersion < minimumAgentAPIVersion {
		return "outdated"
	}
	return "compatible"
}

type order struct {
	ID                  string          `json:"id"`
	OwnerID             string          `json:"ownerId"`
	PlanID              string          `json:"planId"`
	ImageID             string          `json:"imageId"`
	InstanceID          string          `json:"instanceId"`
	Status              string          `json:"status"`
	PaymentNote         string          `json:"paymentNote"`
	AmountFen           int             `json:"amountFen"`
	ListAmountFen       int             `json:"listAmountFen"`
	DiscountAmountFen   int             `json:"discountAmountFen"`
	BenefitSnapshot     json.RawMessage `json:"benefitSnapshot,omitempty"`
	BonusDays           int             `json:"bonusDays,omitempty"`
	PromoCodeMask       string          `json:"promoCodeMask,omitempty"`
	ServiceStartsAt     *time.Time      `json:"serviceStartsAt,omitempty"`
	ExpiresAt           *time.Time      `json:"expiresAt"`
	RefundedAt          *time.Time      `json:"refundedAt,omitempty"`
	RefundAmountFen     int             `json:"refundAmountFen,omitempty"`
	RefundWalletEntryID string          `json:"refundWalletEntryId,omitempty"`
	CreatedAt           *time.Time      `json:"createdAt"`
	UpdatedAt           *time.Time      `json:"updatedAt"`
	PlanName            string          `json:"planName"`
	ImageName           string          `json:"imageName"`
	ImageVersion        string          `json:"imageVersion"`
}

type controlTask struct {
	ID             string          `json:"id"`
	InstanceID     string          `json:"instanceId"`
	Action         string          `json:"action"`
	IdempotencyKey string          `json:"-"`
	Status         string          `json:"status"`
	LastError      string          `json:"lastError"`
	Attempts       int             `json:"attempts"`
	RunAfter       time.Time       `json:"runAfter"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
	ClaimedAt      *time.Time      `json:"claimedAt,omitempty"`
	ClaimExpiresAt *time.Time      `json:"claimExpiresAt,omitempty"`
	WorkerID       string          `json:"workerId,omitempty"`
	ExecutionToken string          `json:"-"`
	HeartbeatAt    *time.Time      `json:"heartbeatAt,omitempty"`
	RecoveryCount  int             `json:"recoveryCount"`
	Payload        json.RawMessage `json:"-"`
}
type auditLog struct {
	ID         int64     `json:"id"`
	ActorID    string    `json:"actorId"`
	Action     string    `json:"action"`
	TargetType string    `json:"targetType"`
	TargetID   string    `json:"targetId"`
	CreatedAt  time.Time `json:"createdAt"`
}

func initializeControlPlane(ctx context.Context) error {
	_ = ctx
	return nil
}

func isDuplicateMigration(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate column") ||
		strings.Contains(message, "duplicate key") ||
		strings.Contains(message, "already exists") ||
		(strings.Contains(message, "can't drop") && strings.Contains(message, "check that column/key exists"))
}

func listCatalog(ctx context.Context, includeDisabled bool) ([]catalogImage, []plan, error) {
	if instanceDB == nil {
		return nil, nil, errors.New("开发模式未配置 MySQL")
	}
	imageSQL := `SELECT id,name,image_ref,COALESCE(image_digest,''),version,enabled,created_at FROM xcloud_images`
	planSQL := `SELECT id,name,cpu,memory_mb,bandwidth_mbps,monthly_price_fen,enabled,sort_order,created_at FROM xcloud_plans`
	if !includeDisabled {
		imageSQL += ` WHERE enabled=TRUE`
		planSQL += ` WHERE enabled=TRUE`
	}
	imageSQL += ` ORDER BY name,version DESC`
	planSQL += ` ORDER BY sort_order,name`
	images, err := scanImages(ctx, imageSQL)
	if err != nil {
		return nil, nil, err
	}
	for index := range images {
		versions, versionErr := listImageVersions(ctx, images[index].ID, includeDisabled)
		if versionErr != nil {
			return nil, nil, versionErr
		}
		images[index].Versions = versions
	}
	plans, err := scanPlans(ctx, planSQL)
	if err != nil {
		return nil, nil, err
	}
	if err := loadPlanTierDiscounts(ctx, plans); err != nil {
		return nil, nil, err
	}
	return images, plans, err
}

func loadPlanTierDiscounts(ctx context.Context, plans []plan) error {
	if len(plans) == 0 {
		return nil
	}
	rows, err := instanceDB.QueryContext(ctx, `SELECT plan_id,months,discount_bps FROM xcloud_plan_price_tiers WHERE enabled=TRUE`)
	if err != nil {
		return err
	}
	defer rows.Close()
	byID := make(map[string]*plan, len(plans))
	for index := range plans {
		plans[index].TierDiscounts = map[int]int{}
		byID[plans[index].ID] = &plans[index]
	}
	for rows.Next() {
		var planID string
		var months, discount int
		if err := rows.Scan(&planID, &months, &discount); err != nil {
			return err
		}
		if plan := byID[planID]; plan != nil {
			plan.TierDiscounts[months] = discount
		}
	}
	return rows.Err()
}
func listImageVersions(ctx context.Context, imageID string, includeDisabled bool) ([]imageVersion, error) {
	statement := `SELECT id,image_id,version_tag,COALESCE(image_digest,''),enabled,version_status,COALESCE(last_error,''),published_at,created_at FROM xcloud_image_versions WHERE image_id=?`
	if !includeDisabled {
		statement += ` AND enabled=TRUE AND version_status='ready'`
	}
	statement += ` ORDER BY created_at DESC,version_tag`
	rows, err := instanceDB.QueryContext(ctx, statement, imageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []imageVersion{}
	for rows.Next() {
		var item imageVersion
		if err := rows.Scan(&item.ID, &item.ImageID, &item.Tag, &item.ImageDigest, &item.Enabled, &item.Status, &item.LastError, &item.PublishedAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
func saveImageVersion(ctx context.Context, value imageVersion) (imageVersion, error) {
	value.Tag = strings.TrimSpace(value.Tag)
	if !validImageTag(value.Tag) {
		return imageVersion{}, errors.New("镜像版本格式无效")
	}
	// Digest is resolved by the Agent after pull. It is never trusted from a
	// browser request, otherwise a tag could be published without verification.
	value.ImageDigest = ""
	isNew := value.ID == ""
	if value.ID == "" {
		var existing string
		err := instanceDB.QueryRowContext(ctx, `SELECT id FROM xcloud_image_versions WHERE image_id=? AND version_tag=?`, value.ImageID, value.Tag).Scan(&existing)
		if err == nil {
			value.ID = existing
			isNew = false
		} else if err == sql.ErrNoRows {
			value.ID = newID("ver")
		} else {
			return imageVersion{}, err
		}
	}
	now := time.Now()
	if value.CreatedAt.IsZero() {
		value.CreatedAt = now
	}
	var exists int
	if err := instanceDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM xcloud_images WHERE id=?`, value.ImageID).Scan(&exists); err != nil || exists != 1 {
		return imageVersion{}, errors.New("镜像来源不存在")
	}
	if isNew {
		value.Enabled = false
		value.Status = "draft"
		_, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_image_versions (id,image_id,version_tag,image_digest,enabled,version_status,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?)`, value.ID, value.ImageID, value.Tag, nil, false, value.Status, value.CreatedAt, now)
		return value, err
	}
	var current imageVersion
	if err := instanceDB.QueryRowContext(ctx, `SELECT COALESCE(image_digest,''),enabled,version_status,COALESCE(last_error,''),published_at,created_at FROM xcloud_image_versions WHERE id=? AND image_id=? FOR UPDATE`, value.ID, value.ImageID).Scan(&current.ImageDigest, &current.Enabled, &current.Status, &current.LastError, &current.PublishedAt, &current.CreatedAt); err != nil {
		return imageVersion{}, errors.New("镜像版本不存在")
	}
	if current.Status != "ready" && value.Enabled {
		return imageVersion{}, errors.New("版本尚未完成节点校验，不能上架")
	}
	_, err := instanceDB.ExecContext(ctx, `UPDATE xcloud_image_versions SET enabled=?,updated_at=? WHERE id=? AND image_id=?`, value.Enabled, now, value.ID, value.ImageID)
	current.Enabled = value.Enabled
	return current, err
}

func scanImages(ctx context.Context, statement string, args ...any) ([]catalogImage, error) {
	rows, err := instanceDB.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []catalogImage{}
	for rows.Next() {
		var item catalogImage
		if err := rows.Scan(&item.ID, &item.Name, &item.ImageRef, &item.ImageDigest, &item.Version, &item.Enabled, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
func scanPlans(ctx context.Context, statement string, args ...any) ([]plan, error) {
	rows, err := instanceDB.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []plan{}
	for rows.Next() {
		var item plan
		if err := rows.Scan(&item.ID, &item.Name, &item.CPU, &item.MemoryMB, &item.BandwidthMbps, &item.MonthlyFen, &item.Enabled, &item.SortOrder, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func saveImage(ctx context.Context, value catalogImage) error {
	isNew := value.ID == ""
	if value.ID == "" {
		value.ID = newID("img")
	}
	if value.CreatedAt.IsZero() {
		value.CreatedAt = time.Now()
	}
	value.ImageRef = strings.TrimSpace(value.ImageRef)
	if value.ImageRef == "" || strings.ContainsAny(value.ImageRef, " \t\r\n@") {
		return errors.New("镜像地址无效")
	}
	checkDuplicate := isNew
	if !isNew {
		var currentRef string
		err := instanceDB.QueryRowContext(ctx, `SELECT image_ref FROM xcloud_images WHERE id=?`, value.ID).Scan(&currentRef)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if err == nil && strings.TrimSpace(currentRef) != value.ImageRef {
			return errors.New("镜像仓库地址不可修改，请新建软件")
		}
		checkDuplicate = err == sql.ErrNoRows
	}
	if checkDuplicate {
		var duplicate int
		if err := instanceDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM xcloud_images WHERE image_ref=? AND id<>?`, value.ImageRef, value.ID).Scan(&duplicate); err != nil {
			return err
		}
		if duplicate > 0 {
			return errors.New("镜像地址已存在，请勿重复添加")
		}
	}
	if value.Version == "" {
		value.Version = "latest"
	}
	// Digest is optional operational metadata. Source approval happens at the
	// repository level, while every user's selected tag is persisted on order.
	value.ImageDigest = strings.ToLower(strings.TrimSpace(value.ImageDigest))
	// Older admin pages send the bootstrap placeholder back on every update.
	// It is not a user-supplied digest and is intentionally ignored now that
	// administrators manage repository sources rather than immutable versions.
	if strings.HasPrefix(value.ImageDigest, "seed:") || value.ImageDigest == "sha256:"+strings.Repeat("0", 64) {
		value.ImageDigest = ""
	}
	if value.ImageDigest != "" && !validImageDigest(value.ImageDigest) {
		return errors.New("镜像摘要格式无效")
	}
	_, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_images (id,name,image_ref,image_digest,version,enabled,created_at) VALUES (?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE name=VALUES(name),image_ref=VALUES(image_ref),version=VALUES(version),enabled=VALUES(enabled)`, value.ID, value.Name, value.ImageRef, nullableString(value.ImageDigest), value.Version, value.Enabled, value.CreatedAt)
	if err != nil || !isNew {
		return err
	}
	return nil
}
func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func savePlan(ctx context.Context, value plan) error {
	if value.ID == "" {
		value.ID = newID("plan")
	}
	if value.CreatedAt.IsZero() {
		value.CreatedAt = time.Now()
	}
	if value.BandwidthMbps < 1 || value.BandwidthMbps > 10000 {
		return errors.New("套餐最高带宽应为 1 至 10000 Mbps")
	}
	_, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_plans (id,name,cpu,memory_mb,bandwidth_mbps,monthly_price_fen,enabled,sort_order,created_at) VALUES (?,?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE name=VALUES(name),cpu=VALUES(cpu),memory_mb=VALUES(memory_mb),bandwidth_mbps=VALUES(bandwidth_mbps),monthly_price_fen=VALUES(monthly_price_fen),enabled=VALUES(enabled),sort_order=VALUES(sort_order)`, value.ID, value.Name, value.CPU, value.MemoryMB, value.BandwidthMbps, value.MonthlyFen, value.Enabled, value.SortOrder, value.CreatedAt)
	return err
}

func createOrder(ctx context.Context, ownerID, planID, imageID string, months int, note string) (order, error) {
	if months < 1 || months > 60 {
		return order{}, errors.New("订阅周期应为 1 至 60 个月")
	}
	var selected plan
	if err := instanceDB.QueryRowContext(ctx, `SELECT id,name,cpu,memory_mb,monthly_price_fen,enabled,sort_order,created_at FROM xcloud_plans WHERE id=? AND enabled=TRUE`, planID).Scan(&selected.ID, &selected.Name, &selected.CPU, &selected.MemoryMB, &selected.MonthlyFen, &selected.Enabled, &selected.SortOrder, &selected.CreatedAt); err != nil {
		return order{}, errors.New("套餐不可购买")
	}
	var image catalogImage
	if err := instanceDB.QueryRowContext(ctx, `SELECT id,name,image_ref,COALESCE(image_digest,''),version,enabled,created_at FROM xcloud_images WHERE id=? AND enabled=TRUE`, imageID).Scan(&image.ID, &image.Name, &image.ImageRef, &image.ImageDigest, &image.Version, &image.Enabled, &image.CreatedAt); err != nil {
		return order{}, errors.New("镜像版本不可购买")
	}
	now := time.Now()
	result := order{ID: newID("ord"), OwnerID: ownerID, PlanID: planID, ImageID: imageID, AmountFen: selected.MonthlyFen * months, Status: orderPending, PaymentNote: strings.TrimSpace(note), CreatedAt: &now, UpdatedAt: &now, PlanName: selected.Name, ImageName: image.Name, ImageVersion: image.Version}
	_, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_orders (id,owner_id,plan_id,image_id,amount_fen,status,payment_note,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?)`, result.ID, result.OwnerID, result.PlanID, result.ImageID, result.AmountFen, result.Status, result.PaymentNote, now, now)
	return result, err
}

func listOrders(ctx context.Context, ownerID string) ([]order, error) {
	return scanOrders(ctx, `SELECT o.id,o.owner_id,o.plan_id,o.image_id,COALESCE(o.instance_id,''),o.amount_fen,COALESCE(o.list_amount_fen,o.amount_fen),COALESCE(o.discount_amount_fen,0),o.benefit_snapshot,COALESCE(o.bonus_days,0),COALESCE(o.promo_code_mask,''),o.status,COALESCE(o.payment_note,''),o.service_starts_at,o.expires_at,o.refunded_at,COALESCE(o.refund_amount_fen,0),COALESCE(o.refund_wallet_entry_id,''),o.created_at,o.updated_at,p.name,i.name,i.version FROM xcloud_orders o JOIN xcloud_plans p ON p.id=o.plan_id JOIN xcloud_images i ON i.id=o.image_id WHERE o.owner_id=? ORDER BY o.created_at DESC`, ownerID)
}
func listAllOrders(ctx context.Context) ([]order, error) {
	return scanOrders(ctx, `SELECT o.id,o.owner_id,o.plan_id,o.image_id,COALESCE(o.instance_id,''),o.amount_fen,COALESCE(o.list_amount_fen,o.amount_fen),COALESCE(o.discount_amount_fen,0),o.benefit_snapshot,COALESCE(o.bonus_days,0),COALESCE(o.promo_code_mask,''),o.status,COALESCE(o.payment_note,''),o.service_starts_at,o.expires_at,o.refunded_at,COALESCE(o.refund_amount_fen,0),COALESCE(o.refund_wallet_entry_id,''),o.created_at,o.updated_at,p.name,i.name,i.version FROM xcloud_orders o JOIN xcloud_plans p ON p.id=o.plan_id JOIN xcloud_images i ON i.id=o.image_id ORDER BY o.created_at DESC`)
}
func scanOrders(ctx context.Context, statement string, args ...any) ([]order, error) {
	rows, err := instanceDB.QueryContext(ctx, statement, args...)
	if err != nil {
		// A rolling deployment may briefly run the new binary against a database
		// whose optional commercial-benefit migration has not completed. Orders must stay
		// readable during that window; retry with legacy-compatible projections.
		if !isMissingOrderUpgrade(err) {
			return nil, err
		}
		legacy := strings.NewReplacer(
			"COALESCE(o.list_amount_fen,o.amount_fen)", "o.amount_fen",
			"COALESCE(o.discount_amount_fen,0)", "0",
			"o.benefit_snapshot", "NULL",
			"COALESCE(o.bonus_days,0)", "0",
			"COALESCE(o.promo_code_mask,'')", "''",
		).Replace(statement)
		rows, err = instanceDB.QueryContext(ctx, legacy, args...)
		if err != nil {
			return nil, err
		}
	}
	defer rows.Close()
	items := []order{}
	for rows.Next() {
		var v order
		// JSON is nullable for historical orders. sql.NullString accepts both a
		// driver []byte and NULL, whereas json.RawMessage cannot scan NULL.
		var snapshot sql.NullString
		if err := rows.Scan(&v.ID, &v.OwnerID, &v.PlanID, &v.ImageID, &v.InstanceID, &v.AmountFen, &v.ListAmountFen, &v.DiscountAmountFen, &snapshot, &v.BonusDays, &v.PromoCodeMask, &v.Status, &v.PaymentNote, &v.ServiceStartsAt, &v.ExpiresAt, &v.RefundedAt, &v.RefundAmountFen, &v.RefundWalletEntryID, &v.CreatedAt, &v.UpdatedAt, &v.PlanName, &v.ImageName, &v.ImageVersion); err != nil {
			return nil, err
		}
		if snapshot.Valid {
			v.BenefitSnapshot = json.RawMessage(snapshot.String)
		}
		items = append(items, v)
	}
	return items, rows.Err()
}

func isMissingOrderUpgrade(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unknown column") && (strings.Contains(message, "benefit_snapshot") || strings.Contains(message, "bonus_days") || strings.Contains(message, "promo_code_mask") || strings.Contains(message, "list_amount_fen") || strings.Contains(message, "discount_amount_fen"))
}

func cancelOrder(ctx context.Context, id, ownerID string) error {
	result, err := instanceDB.ExecContext(ctx, `UPDATE xcloud_orders SET status=?,updated_at=NOW() WHERE id=? AND owner_id=? AND status IN (?,?)`, orderCancel, id, ownerID, orderPending, orderReview)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return errors.New("订单当前不可取消")
	}
	return nil
}

func submitPayment(ctx context.Context, orderID, ownerID, reference string) error {
	reference = strings.TrimSpace(reference)
	if len(reference) < 3 || len(reference) > 128 {
		return errors.New("付款流水号长度应为 3 至 128 个字符")
	}
	tx, err := instanceDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var amount int
	var status string
	if err = tx.QueryRowContext(ctx, `SELECT amount_fen,status FROM xcloud_orders WHERE id=? AND owner_id=? FOR UPDATE`, orderID, ownerID).Scan(&amount, &status); err != nil {
		return err
	}
	if status != orderPending {
		return errors.New("订单当前不可提交付款信息")
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_payments (id,order_id,payer_id,amount_fen,reference_no,status,submitted_at) VALUES (?,?,?,?,?,'submitted',NOW())`, newID("pay"), orderID, ownerID, amount, reference); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE xcloud_orders SET status=?,payment_note=?,updated_at=NOW() WHERE id=?`, orderReview, reference, orderID); err != nil {
		return err
	}
	return tx.Commit()
}

func confirmOrder(ctx context.Context, id, actorID string) (controlTask, error) {
	tx, err := instanceDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return controlTask{}, err
	}
	defer tx.Rollback()
	var o order
	var p plan
	var img catalogImage
	err = tx.QueryRowContext(ctx, `SELECT o.id,o.owner_id,o.plan_id,o.image_id,o.amount_fen,o.status,p.id,p.name,p.cpu,p.memory_mb,p.bandwidth_mbps,p.monthly_price_fen,p.enabled,p.sort_order,p.created_at,i.id,i.name,i.image_ref,COALESCE(i.image_digest,''),i.version,i.enabled,i.created_at FROM xcloud_orders o JOIN xcloud_plans p ON p.id=o.plan_id JOIN xcloud_images i ON i.id=o.image_id WHERE o.id=? FOR UPDATE`, id).Scan(&o.ID, &o.OwnerID, &o.PlanID, &o.ImageID, &o.AmountFen, &o.Status, &p.ID, &p.Name, &p.CPU, &p.MemoryMB, &p.BandwidthMbps, &p.MonthlyFen, &p.Enabled, &p.SortOrder, &p.CreatedAt, &img.ID, &img.Name, &img.ImageRef, &img.ImageDigest, &img.Version, &img.Enabled, &img.CreatedAt)
	if err != nil {
		return controlTask{}, err
	}
	if o.Status != orderPending && o.Status != orderReview {
		return controlTask{}, errors.New("订单当前不可确认")
	}
	if !p.Enabled || !img.Enabled {
		return controlTask{}, errors.New("套餐或镜像版本已下架")
	}
	var renewal sql.NullString
	if err = tx.QueryRowContext(ctx, `SELECT renewal_instance_id FROM xcloud_orders WHERE id=?`, o.ID).Scan(&renewal); err != nil {
		return controlTask{}, err
	}
	if renewal.Valid && renewal.String != "" {
		var currentExpiry, activeTaskExpiresAt sql.NullTime
		var instanceStatus, runtimeStatus string
		if err = tx.QueryRowContext(ctx, `SELECT expires_at,status,COALESCE(runtime_status,'stopped'),active_task_expires_at FROM xcloud_instances WHERE id=? AND owner_id=? FOR UPDATE`, renewal.String, o.OwnerID).Scan(&currentExpiry, &instanceStatus, &runtimeStatus, &activeTaskExpiresAt); err != nil {
			return controlTask{}, errors.New("待续费实例不存在或已清理")
		}
		if activeTaskExpiresAt.Valid && activeTaskExpiresAt.Time.After(time.Now()) {
			return controlTask{}, errors.New("实例生命周期任务处理中，请稍后再续费")
		}
		if instanceStatus != "running" && instanceStatus != "stopped" && instanceStatus != "destroy_scheduled" {
			return controlTask{}, errors.New("待续费实例当前不可恢复")
		}
		if runtimeStatus != "running" && runtimeStatus != "stopped" {
			runtimeStatus = "stopped"
		}
		now := time.Now()
		base := now
		if currentExpiry.Valid && currentExpiry.Time.After(now) {
			base = currentExpiry.Time
		}
		months := o.AmountFen / p.MonthlyFen
		if months < 1 {
			months = 1
		}
		expires := base.AddDate(0, months, 0)
		changed, transitionErr := transitionInstance(ctx, tx, renewal.String, []string{instanceStatus}, runtimeStatus, &runtimeStatus, "expires_at=?,purge_at=NULL,destroy_at=NULL,destroy_reason=NULL,destroyed_at=NULL", expires)
		if transitionErr != nil {
			return controlTask{}, transitionErr
		}
		if !changed {
			return controlTask{}, errInstanceStateConflict
		}
		if _, err = tx.ExecContext(ctx, `UPDATE xcloud_orders SET instance_id=?,status=?,service_starts_at=?,expires_at=?,updated_at=? WHERE id=?`, renewal.String, orderDeploy, base, expires, now, o.ID); err != nil {
			return controlTask{}, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE xcloud_payments SET status='confirmed',reviewed_at=?,reviewer_id=? WHERE order_id=? AND status='submitted'`, now, actorID, o.ID); err != nil {
			return controlTask{}, err
		}
		task := controlTask{ID: newID("task"), InstanceID: renewal.String, Action: "start", IdempotencyKey: "renew:" + o.ID, Status: taskPending, RunAfter: now, CreatedAt: now, UpdatedAt: now}
		if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_tasks (id,instance_id,action,idempotency_key,status,attempts,run_after,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?)`, task.ID, task.InstanceID, task.Action, task.IdempotencyKey, task.Status, 0, task.RunAfter, now, now); err != nil {
			return controlTask{}, err
		}
		if err = writeAuditTx(ctx, tx, actorID, "order.confirm_renewal", "order", o.ID, map[string]any{"instanceId": renewal.String}); err != nil {
			return controlTask{}, err
		}
		if err = tx.Commit(); err != nil {
			return controlTask{}, err
		}
		appendTaskEvent(ctx, task.ID, "queued", "人工确认收款后等待续费恢复")
		_ = createNotification(ctx, o.OwnerID, "order", "续费已确认", "实例正在恢复服务。", map[string]any{"orderId": o.ID, "taskId": task.ID})
		return task, nil
	}
	n, err := selectNodeForPlan(ctx, tx, p)
	if err != nil {
		return controlTask{}, errors.New("资源不足，请联系官方客服")
	}
	instanceID := newID("ins")
	route := routeKey(o.OwnerID + "\x00" + instanceID)
	digest := routeKey(instanceID)
	container := fmt.Sprintf("xcloud-%s", strings.TrimPrefix(digest, "r"))
	now := time.Now()
	months := o.AmountFen / p.MonthlyFen
	if months < 1 {
		months = 1
	}
	expires := now.AddDate(0, months, 0)
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_instances (id,owner_id,name,image,version,spec,status,access_address,container_name,created_at,cpu,memory_mb,bandwidth_mbps,node_id,order_id,route_key,expires_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, instanceID, o.OwnerID, "AlemonX", img.ImageRef, img.Version, fmt.Sprintf("%g 核 / %d GB / 最高 %d Mbps", p.CPU, p.MemoryMB/1024, p.BandwidthMbps), "deploying", "https://xcloud-"+route+"."+env("XCLOUD_INSTANCE_DOMAIN", "alemonjs.com"), container, now, p.CPU, p.MemoryMB, p.BandwidthMbps, n.ID, o.ID, route, expires); err != nil {
		return controlTask{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE xcloud_orders SET instance_id=?,status=?,service_starts_at=?,expires_at=?,updated_at=? WHERE id=?`, instanceID, orderDeploy, now, expires, now, o.ID); err != nil {
		return controlTask{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE xcloud_payments SET status='confirmed',reviewed_at=?,reviewer_id=? WHERE order_id=? AND status='submitted'`, now, actorID, o.ID); err != nil {
		return controlTask{}, err
	}
	task := controlTask{ID: newID("task"), InstanceID: instanceID, Action: "create", IdempotencyKey: "create:" + instanceID, Status: taskPending, RunAfter: now, CreatedAt: now, UpdatedAt: now}
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_tasks (id,instance_id,action,idempotency_key,status,attempts,run_after,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?)`, task.ID, task.InstanceID, task.Action, task.IdempotencyKey, task.Status, 0, task.RunAfter, now, now); err != nil {
		return controlTask{}, err
	}
	if err = writeAuditTx(ctx, tx, actorID, "order.confirm", "order", o.ID, map[string]any{"instanceId": instanceID, "amountFen": o.AmountFen}); err != nil {
		return controlTask{}, err
	}
	if err = tx.Commit(); err != nil {
		return controlTask{}, err
	}
	appendTaskEvent(ctx, task.ID, "queued", "人工确认收款后等待部署")
	_ = createNotification(ctx, o.OwnerID, "order", "付款已确认", "实例已进入部署队列。", map[string]any{"orderId": o.ID, "instanceId": instanceID, "taskId": task.ID})
	return task, nil
}

func rejectOrder(ctx context.Context, id, actorID, reason string) error {
	result, err := instanceDB.ExecContext(ctx, `UPDATE xcloud_orders SET status=?,payment_note=?,updated_at=NOW() WHERE id=? AND status IN (?,?)`, orderReject, strings.TrimSpace(reason), id, orderPending, orderReview)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return errors.New("订单当前不可拒绝")
	}
	_, _ = instanceDB.ExecContext(ctx, `UPDATE xcloud_payments SET status='rejected',reviewed_at=NOW(),reviewer_id=? WHERE order_id=? AND status='submitted'`, actorID, id)
	return writeAudit(ctx, actorID, "order.reject", "order", id, map[string]any{"reason": reason})
}
func writeAudit(ctx context.Context, actor, action, targetType, targetID string, detail any) error {
	data, _ := json.Marshal(detail)
	_, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_audit_logs (actor_id,action,target_type,target_id,detail,created_at) VALUES (?,?,?,?,?,NOW())`, actor, action, targetType, targetID, data)
	return err
}
func writeAuditTx(ctx context.Context, tx *sql.Tx, actor, action, targetType, targetID string, detail any) error {
	data, _ := json.Marshal(detail)
	_, err := tx.ExecContext(ctx, `INSERT INTO xcloud_audit_logs (actor_id,action,target_type,target_id,detail,created_at) VALUES (?,?,?,?,?,NOW())`, actor, action, targetType, targetID, data)
	return err
}

func newID(prefix string) string {
	value, err := randomToken()
	if err != nil {
		panic(err)
	}
	return prefix + "_" + strings.ToLower(strings.TrimRight(strings.NewReplacer("-", "", "_", "", "=", "").Replace(value), "="))[:22]
}

var imageDigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func validImageDigest(value string) bool {
	return imageDigestPattern.MatchString(strings.ToLower(strings.TrimSpace(value)))
}

// deploymentImage always prefers the approved immutable digest. A tag is used
// only for sources whose operator intentionally has not recorded a digest.
func deploymentImage(imageRef, version, digest string) string {
	if validImageDigest(digest) {
		return strings.TrimSpace(imageRef) + "@" + strings.ToLower(strings.TrimSpace(digest))
	}
	return strings.TrimSpace(imageRef) + ":" + strings.TrimSpace(version)
}
func nodeHeartbeatTTL() time.Duration {
	seconds := envInt("XCLOUD_NODE_HEARTBEAT_TTL_SECONDS", 90)
	if seconds < 15 {
		seconds = 15
	}
	return time.Duration(seconds) * time.Second
}

const taskSelectFields = `id,instance_id,action,idempotency_key,status,attempts,COALESCE(last_error,''),run_after,created_at,updated_at,claimed_at,claim_expires_at,COALESCE(worker_id,''),COALESCE(execution_token,''),heartbeat_at,COALESCE(recovery_count,0),COALESCE(payload,JSON_OBJECT())`

func scanControlTask(scanner interface{ Scan(...any) error }, task *controlTask) error {
	return scanner.Scan(&task.ID, &task.InstanceID, &task.Action, &task.IdempotencyKey, &task.Status, &task.Attempts, &task.LastError, &task.RunAfter, &task.CreatedAt, &task.UpdatedAt, &task.ClaimedAt, &task.ClaimExpiresAt, &task.WorkerID, &task.ExecutionToken, &task.HeartbeatAt, &task.RecoveryCount, &task.Payload)
}

func loadTask(ctx context.Context, id string) (controlTask, error) {
	var task controlTask
	err := scanControlTask(instanceDB.QueryRowContext(ctx, `SELECT `+taskSelectFields+` FROM xcloud_tasks WHERE id=?`, id), &task)
	return task, err
}

func pendingTasks(ctx context.Context, limit int) ([]controlTask, error) {
	rows, err := instanceDB.QueryContext(ctx, `SELECT `+taskSelectFields+` FROM xcloud_tasks WHERE status=? AND run_after<=NOW() ORDER BY created_at LIMIT ?`, taskPending, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []controlTask{}
	for rows.Next() {
		var t controlTask
		if err := scanControlTask(rows, &t); err != nil {
			return nil, err
		}
		items = append(items, t)
	}
	return items, rows.Err()
}

func listTasks(ctx context.Context, limit int) ([]controlTask, error) {
	rows, err := instanceDB.QueryContext(ctx, `SELECT `+taskSelectFields+` FROM xcloud_tasks ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []controlTask{}
	for rows.Next() {
		var t controlTask
		if err := scanControlTask(rows, &t); err != nil {
			return nil, err
		}
		items = append(items, t)
	}
	return items, rows.Err()
}
func listAuditLogs(ctx context.Context, limit int) ([]auditLog, error) {
	rows, err := instanceDB.QueryContext(ctx, `SELECT id,actor_id,action,target_type,target_id,created_at FROM xcloud_audit_logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []auditLog{}
	for rows.Next() {
		var item auditLog
		if err := rows.Scan(&item.ID, &item.ActorID, &item.Action, &item.TargetType, &item.TargetID, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func taskWorkerID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		return fmt.Sprintf("xcloud-server:%d", os.Getpid())
	}
	return fmt.Sprintf("xcloud-server@%s:%d", host, os.Getpid())
}

func lifecycleTask(action string) bool {
	switch action {
	case "create", "retry-deploy", "start", "stop", "update", "restart", "destroy", "purge":
		return true
	}
	return false
}

func claimTask(ctx context.Context, task controlTask) (bool, error) {
	tx, err := beginSerializableTx(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	token := newID("exec")
	// Lease timestamps and recovery checks must use the same MySQL clock. Using
	// Go wall-clock values here while recovery compares with NOW() can make a
	// freshly claimed task look expired when server/session timezones differ.
	result, err := tx.ExecContext(ctx, `UPDATE xcloud_tasks SET status=?,attempts=attempts+1,claimed_at=NOW(),heartbeat_at=NOW(),claim_expires_at=DATE_ADD(NOW(), INTERVAL 5 MINUTE),worker_id=?,execution_token=?,updated_at=NOW() WHERE id=? AND status=?`, taskRunning, taskWorkerID(), token, task.ID, taskPending)
	if err != nil {
		return false, err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return false, nil
	}
	if lifecycleTask(task.Action) {
		locked, lockErr := tx.ExecContext(ctx, `UPDATE xcloud_instances SET active_task_id=?,active_task_token=?,active_task_expires_at=DATE_ADD(NOW(), INTERVAL 5 MINUTE) WHERE id=? AND (active_task_expires_at IS NULL OR active_task_expires_at<=NOW())`, task.ID, token, task.InstanceID)
		if lockErr != nil {
			return false, lockErr
		}
		if n, _ := locked.RowsAffected(); n != 1 {
			// The deferred rollback returns this task to pending. It must not be
			// executed while a different lifecycle task holds the instance lock.
			_ = tx.Rollback()
			appendTaskEvent(ctx, task.ID, "instance_lock_conflict", "实例正在由另一个生命周期任务处理")
			return false, nil
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	appendTaskEvent(ctx, task.ID, "claimed", "消费者已领取任务")
	return true, nil
}

// taskMayCallAgent is the final fencing check immediately before a lifecycle
// request leaves the control plane. A worker that lost its lease or whose task
// was recovered must stop before it can destroy or recreate a container.
func taskMayCallAgent(ctx context.Context, task controlTask, expected ...string) error {
	if !lifecycleTask(task.Action) {
		return nil
	}
	if task.ExecutionToken == "" || task.WorkerID == "" {
		return errors.New("任务未持有执行租约")
	}
	query := `SELECT COUNT(*) FROM xcloud_tasks t JOIN xcloud_instances i ON i.id=t.instance_id
		WHERE t.id=? AND t.status=? AND t.worker_id=? AND t.execution_token=?
		AND t.claim_expires_at>NOW() AND i.active_task_id=t.id
		AND i.active_task_token=t.execution_token AND i.active_task_expires_at>NOW()`
	args := []any{task.ID, taskRunning, task.WorkerID, task.ExecutionToken}
	if len(expected) > 0 {
		marks := strings.TrimRight(strings.Repeat("?,", len(expected)), ",")
		query += " AND i.status IN (" + marks + ")"
		for _, status := range expected {
			args = append(args, status)
		}
	}
	var matches int
	if err := instanceDB.QueryRowContext(ctx, query, args...).Scan(&matches); err != nil {
		return err
	}
	if matches != 1 {
		return errors.New("任务已失去实例生命周期锁或实例状态已变化")
	}
	return nil
}

// finishTask only accepts the consumer that still owns an unexpired lease.
// A recovered task may be executing in a late, crashed worker; that worker
// must never be able to mark the replacement attempt as complete or failed.
func finishTask(ctx context.Context, task controlTask, err error) (bool, error) {
	workerID := task.WorkerID
	if workerID == "" {
		return false, errors.New("任务缺少消费者租约")
	}
	if err == nil {
		result, e := instanceDB.ExecContext(ctx, `UPDATE xcloud_tasks SET status=?,last_error=NULL,finished_at=NOW(),claimed_at=NULL,claim_expires_at=NULL,worker_id=NULL,execution_token=NULL,updated_at=NOW() WHERE id=? AND status=? AND worker_id=? AND execution_token=? AND claim_expires_at>NOW()`, taskDone, task.ID, taskRunning, workerID, task.ExecutionToken)
		if e != nil {
			return false, e
		}
		if affected, _ := result.RowsAffected(); affected == 1 {
			releaseInstanceTaskLock(ctx, task)
			appendTaskEvent(ctx, task.ID, "succeeded", "任务执行成功")
			return true, nil
		}
		return false, nil
	}
	message := truncateError(err.Error())
	if task.Attempts >= 3 {
		result, e := instanceDB.ExecContext(ctx, `UPDATE xcloud_tasks SET status=?,last_error=?,finished_at=NOW(),claimed_at=NULL,claim_expires_at=NULL,worker_id=NULL,execution_token=NULL,updated_at=NOW() WHERE id=? AND status=? AND worker_id=? AND execution_token=? AND claim_expires_at>NOW()`, taskFailed, message, task.ID, taskRunning, workerID, task.ExecutionToken)
		if e != nil {
			return false, e
		}
		if affected, _ := result.RowsAffected(); affected == 1 {
			releaseInstanceTaskLock(ctx, task)
			appendTaskEvent(ctx, task.ID, "dead_letter", message)
			return true, nil
		}
		return false, nil
	}
	delay := time.Duration(task.Attempts*task.Attempts) * time.Minute
	result, e := instanceDB.ExecContext(ctx, `UPDATE xcloud_tasks SET status=?,last_error=?,run_after=DATE_ADD(NOW(), INTERVAL ? MINUTE),claimed_at=NULL,claim_expires_at=NULL,worker_id=NULL,execution_token=NULL,updated_at=NOW() WHERE id=? AND status=? AND worker_id=? AND execution_token=? AND claim_expires_at>NOW()`, taskPending, message, int(delay/time.Minute), task.ID, taskRunning, workerID, task.ExecutionToken)
	if e != nil {
		return false, e
	}
	if affected, _ := result.RowsAffected(); affected == 1 {
		releaseInstanceTaskLock(ctx, task)
		appendTaskEvent(ctx, task.ID, "retry", message)
		return true, nil
	}
	return false, nil
}

func releaseInstanceTaskLock(ctx context.Context, task controlTask) {
	if lifecycleTask(task.Action) {
		_, _ = instanceDB.ExecContext(ctx, `UPDATE xcloud_instances SET active_task_id=NULL,active_task_token=NULL,active_task_expires_at=NULL WHERE id=? AND active_task_id=? AND active_task_token=?`, task.InstanceID, task.ID, task.ExecutionToken)
	}
}

// failDeployment marks the business resources failed after the create task has
// exhausted its retries. Keeping this transition next to task finalization
// prevents a paid instance from remaining in deploying forever while still
// consuming node capacity.
func failDeployment(ctx context.Context, task controlTask, cause error) {
	if task.Action == "update" && task.Attempts >= 3 {
		_, _ = instanceDB.ExecContext(ctx, `UPDATE xcloud_instances SET runtime_status='missing' WHERE id=? AND status='running'`, task.InstanceID)
		_ = writeAudit(ctx, "system", "instance.update_failed", "instance", task.InstanceID, map[string]any{"taskId": task.ID, "error": truncateError(cause.Error())})
		return
	}
	if task.Action == "bandwidth" {
		_, _ = instanceDB.ExecContext(ctx, `UPDATE xcloud_instances SET bandwidth_status='failed',bandwidth_last_error=? WHERE id=?`, truncateError(cause.Error()), task.InstanceID)
		return
	}
	if (task.Action != "create" && task.Action != "retry-deploy") || task.Attempts < 3 {
		return
	}
	runtime := "unknown"
	changed, err := transitionInstance(ctx, instanceDB, task.InstanceID, []string{"deploying"}, "deployment_failed", &runtime, "")
	if err != nil {
		log.Printf("mark deployment failed %s: %v", task.InstanceID, err)
		return
	}
	if !changed {
		return
	}
	_, _ = instanceDB.ExecContext(ctx, `UPDATE xcloud_orders SET status=?,updated_at=NOW() WHERE instance_id=? AND status=?`, "deployment_failed", task.InstanceID, orderDeploy)
	var ownerID string
	if err := instanceDB.QueryRowContext(ctx, `SELECT owner_id FROM xcloud_instances WHERE id=?`, task.InstanceID).Scan(&ownerID); err == nil {
		_ = createNotification(ctx, ownerID, "deployment_failed", "实例部署失败", "实例连续部署失败，资源已释放，请重试部署或联系客服处理。", map[string]any{"instanceId": task.InstanceID, "taskId": task.ID, "error": truncateError(cause.Error())})
	}
	_ = writeAudit(ctx, "system", "instance.deployment_failed", "instance", task.InstanceID, map[string]any{"taskId": task.ID, "error": truncateError(cause.Error())})
}
func truncateError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 1000 {
		return value[:1000]
	}
	return value
}

type agentLifecycleResult struct {
	BandwidthApplied *bool  `json:"bandwidthApplied"`
	BandwidthWarning string `json:"bandwidthWarning"`
}

// persistBandwidthOutcome deliberately never fails the lifecycle action. A
// running instance is useful even if traffic control needs later remediation.
func persistBandwidthOutcome(ctx context.Context, instanceID string, result agentLifecycleResult) {
	if result.BandwidthApplied == nil {
		return // An older Agent has no structured bandwidth result yet.
	}
	if *result.BandwidthApplied {
		_, _ = instanceDB.ExecContext(ctx, `UPDATE xcloud_instances SET bandwidth_status='applied',bandwidth_applied_at=NOW(),bandwidth_last_error=NULL WHERE id=?`, instanceID)
		return
	}
	warning := result.BandwidthWarning
	if warning == "" {
		warning = "节点未确认带宽规则已应用"
	}
	_, _ = instanceDB.ExecContext(ctx, `UPDATE xcloud_instances SET bandwidth_status='failed',bandwidth_last_error=? WHERE id=?`, truncateError(warning), instanceID)
}

func executeTask(ctx context.Context, task controlTask) error {
	var item instance
	var route string
	var nodeID string
	err := instanceDB.QueryRowContext(ctx, `SELECT id,owner_id,name,image,version,spec,status,access_address,container_name,created_at,route_key,COALESCE(node_id,'') FROM xcloud_instances WHERE id=?`, task.InstanceID).Scan(&item.ID, &item.OwnerID, &item.Name, &item.Image, &item.Version, &item.Spec, &item.Status, &item.IP, &item.ContainerName, &item.CreatedAt, &route, &nodeID)
	if err != nil {
		return err
	}
	expectedStates := map[string][]string{
		"create":       {"deploying"},
		"retry-deploy": {"deploying"},
		"start":        {"stopped", "destroy_scheduled"},
		"stop":         {"running", "destroy_scheduled"},
		"update":       {"running"},
		"restart":      {"running", "destroy_scheduled"},
		"destroy":      {"destroy_scheduled"},
		"purge":        {"destroyed"},
	}
	if states := expectedStates[task.Action]; len(states) > 0 {
		if err := taskMayCallAgent(ctx, task, states...); err != nil {
			return err
		}
	}
	n, err := nodeByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("实例节点不可用: %w", err)
	}
	switch task.Action {
	case "create":
		payload, payloadErr := instanceRuntimePayload(ctx, item.ID, item.ContainerName, route)
		if payloadErr != nil {
			return payloadErr
		}
		var lifecycleResult agentLifecycleResult
		err = nodeRequest(ctx, n, httpMethodPost, "/container/create", payload, &lifecycleResult)
		if err == nil {
			persistBandwidthOutcome(ctx, item.ID, lifecycleResult)
			runtime := "running"
			changed, transitionErr := transitionInstance(ctx, instanceDB, item.ID, []string{"deploying"}, "running", &runtime, "")
			if transitionErr != nil {
				err = transitionErr
			} else if changed {
				_, err = instanceDB.ExecContext(ctx, `UPDATE xcloud_orders SET status=?,updated_at=NOW() WHERE instance_id=? AND status=?`, orderActive, item.ID, orderDeploy)
			}
		}
	case "retry-deploy":
		// A retry clears only a possible stale runtime container. Data lives in
		// the instance volume and is deliberately preserved. Both calls stay
		// behind the same token and instance lock.
		if err = taskMayCallAgent(ctx, task, "deploying"); err != nil {
			return err
		}
		if err = nodeRequest(ctx, n, httpMethodPost, "/container/"+item.ContainerName+"/destroy", nil, nil); err != nil {
			return err
		}
		payload, payloadErr := instanceRuntimePayload(ctx, item.ID, item.ContainerName, route)
		if payloadErr != nil {
			return payloadErr
		}
		var lifecycleResult agentLifecycleResult
		if err = taskMayCallAgent(ctx, task, "deploying"); err != nil {
			return err
		}
		err = nodeRequest(ctx, n, httpMethodPost, "/container/create", payload, &lifecycleResult)
		if err == nil {
			persistBandwidthOutcome(ctx, item.ID, lifecycleResult)
			runtime := "running"
			changed, transitionErr := transitionInstance(ctx, instanceDB, item.ID, []string{"deploying"}, "running", &runtime, "")
			if transitionErr != nil {
				err = transitionErr
			} else if changed {
				_, err = instanceDB.ExecContext(ctx, `UPDATE xcloud_orders SET status=?,updated_at=NOW() WHERE instance_id=? AND status=?`, orderActive, item.ID, orderDeploy)
			}
		}
	case "start":
		var lifecycleResult agentLifecycleResult
		err = nodeRequest(ctx, n, httpMethodPost, "/container/"+item.ContainerName+"/start", nil, &lifecycleResult)
		if err == nil {
			persistBandwidthOutcome(ctx, item.ID, lifecycleResult)
			runtime := "running"
			next := "running"
			if item.Status == "destroy_scheduled" {
				next = item.Status
			}
			changed, transitionErr := transitionInstance(ctx, instanceDB, item.ID, []string{item.Status}, next, &runtime, "")
			if transitionErr != nil {
				err = transitionErr
			} else if changed {
				_, err = instanceDB.ExecContext(ctx, `UPDATE xcloud_orders SET status=?,updated_at=NOW() WHERE instance_id=? AND status=?`, orderActive, item.ID, orderDeploy)
			}
		}
	case "stop":
		err = nodeRequest(ctx, n, httpMethodPost, "/container/"+item.ContainerName+"/stop", nil, nil)
		if err == nil {
			runtime := "stopped"
			next := "stopped"
			if item.Status == "destroy_scheduled" {
				next = item.Status
			}
			_, err = transitionInstance(ctx, instanceDB, item.ID, []string{item.Status}, next, &runtime, "")
		}
	case "restart":
		if !n.supportsAgentCapability("container.compose.restart.v1") {
			return errors.New("节点 Agent 尚未支持按 Compose 配置重启，请先升级该节点 Agent")
		}
		payload, payloadErr := instanceRuntimePayload(ctx, item.ID, item.ContainerName, route)
		if payloadErr != nil {
			return payloadErr
		}
		var lifecycleResult agentLifecycleResult
		err = nodeRequest(ctx, n, httpMethodPost, "/container/"+item.ContainerName+"/restart", payload, &lifecycleResult)
		if err == nil {
			persistBandwidthOutcome(ctx, item.ID, lifecycleResult)
		}
		if err == nil {
			runtime := "running"
			next := "running"
			if item.Status == "destroy_scheduled" {
				next = item.Status
			}
			_, err = transitionInstance(ctx, instanceDB, item.ID, []string{item.Status}, next, &runtime, "")
		}
	case "bandwidth":
		var mbps int
		err = instanceDB.QueryRowContext(ctx, `SELECT bandwidth_mbps FROM xcloud_instances WHERE id=?`, item.ID).Scan(&mbps)
		if err == nil {
			var lifecycleResult agentLifecycleResult
			err = nodeRequest(ctx, n, httpMethodPost, "/container/"+item.ContainerName+"/bandwidth", map[string]any{"bandwidthMbps": mbps}, &lifecycleResult)
			if err == nil {
				persistBandwidthOutcome(ctx, item.ID, lifecycleResult)
			}
		}
		if err == nil {
			runtime := "running"
			next := "running"
			if item.Status == "destroy_scheduled" {
				next = item.Status
			}
			_, err = transitionInstance(ctx, instanceDB, item.ID, []string{item.Status}, next, &runtime, "")
		}
	case "update":
		if !validImageTag(item.Version) {
			return errors.New("实例没有可更新的版本标记")
		}
		var pull struct {
			RepoDigests []string `json:"repoDigests"`
		}
		err = nodeRequest(ctx, n, httpMethodPost, "/container/pull", map[string]any{"image": deploymentImage(item.Image, item.Version, "")}, &pull)
		if err != nil {
			return err
		}
		digest := immutableDigest(pull.RepoDigests)
		if digest == "" {
			return errors.New("节点未返回可验证的镜像摘要")
		}
		var cpu float64
		var memoryMB, bandwidthMbps int
		if err = instanceDB.QueryRowContext(ctx, `SELECT cpu,memory_mb,bandwidth_mbps FROM xcloud_instances WHERE id=?`, item.ID).Scan(&cpu, &memoryMB, &bandwidthMbps); err == nil {
			if err = taskMayCallAgent(ctx, task, "running"); err != nil {
				return err
			}
			err = nodeRequest(ctx, n, httpMethodPost, "/container/"+item.ContainerName+"/destroy", nil, nil)
		}
		if err == nil {
			var lifecycleResult agentLifecycleResult
			if err = taskMayCallAgent(ctx, task, "running"); err != nil {
				return err
			}
			err = nodeRequest(ctx, n, httpMethodPost, "/container/create", map[string]any{"name": item.ContainerName, "image": deploymentImage(item.Image, item.Version, digest), "cpu": cpu, "memoryMB": memoryMB, "bandwidthMbps": bandwidthMbps, "route": route}, &lifecycleResult)
			if err == nil {
				persistBandwidthOutcome(ctx, item.ID, lifecycleResult)
			}
		}
		if err == nil {
			runtime := "running"
			_, err = transitionInstance(ctx, instanceDB, item.ID, []string{"running"}, "running", &runtime, "image_digest=?", digest)
		}
	case "destroy":
		err = executeDestroyTask(ctx, task, item.ID, item.ContainerName, n)
	case "purge":
		err = executePurgeTask(ctx, task, item.ID, item.ContainerName, n)
	default:
		return errors.New("未知任务动作")
	}
	return err
}

// instanceRuntimePayload is the single source of desired Compose state for
// creation and restart. It deliberately reads the order's immutable image
// snapshot: a restart applies current platform configuration, while a user
// initiated update is the only operation allowed to pull a newer image.
func instanceRuntimePayload(ctx context.Context, instanceID, containerName, route string) (map[string]any, error) {
	var cpu float64
	var memoryMB, bandwidthMbps int
	if err := instanceDB.QueryRowContext(ctx, `SELECT cpu,memory_mb,bandwidth_mbps FROM xcloud_instances WHERE id=?`, instanceID).Scan(&cpu, &memoryMB, &bandwidthMbps); err != nil {
		return nil, err
	}
	var imageRef, digest, selectedVersion string
	if err := instanceDB.QueryRowContext(ctx, `SELECT i.image_ref,COALESCE(ins.image_digest,o.selected_image_digest,i.image_digest,''),COALESCE(ins.version,o.selected_image_version,i.version) FROM xcloud_instances ins JOIN xcloud_orders o ON o.instance_id=ins.id JOIN xcloud_images i ON i.id=o.image_id WHERE ins.id=? ORDER BY o.created_at DESC LIMIT 1`, instanceID).Scan(&imageRef, &digest, &selectedVersion); err != nil {
		return nil, err
	}
	return map[string]any{"name": containerName, "image": deploymentImage(imageRef, selectedVersion, digest), "cpu": cpu, "memoryMB": memoryMB, "bandwidthMbps": bandwidthMbps, "route": route}, nil
}

func executePurgeTask(ctx context.Context, task controlTask, instanceID, containerName string, n node) error {
	// Never hold a database transaction across a network call. The task token
	// and instance lock are the fence: check them immediately before the Agent
	// request, then conditionally persist the result with that same fence.
	if err := taskMayCallAgent(ctx, task, "destroyed"); err != nil {
		return err
	}
	var eligible int
	err := instanceDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM xcloud_instances i
		WHERE i.id=? AND i.status='destroyed' AND i.purge_at<=NOW()
		AND NOT EXISTS (SELECT 1 FROM xcloud_orders o WHERE o.instance_id=i.id AND o.status=? AND o.expires_at>NOW())`, instanceID, orderActive).Scan(&eligible)
	if err != nil {
		return err
	}
	if eligible != 1 {
		return nil
	}
	if err := taskMayCallAgent(ctx, task, "destroyed"); err != nil {
		return err
	}
	if err := nodeRequest(ctx, n, httpMethodDelete, "/container/"+containerName+"?purge=true", nil, nil); err != nil {
		return err
	}
	result, err := instanceDB.ExecContext(ctx, `UPDATE xcloud_instances i JOIN xcloud_tasks t ON t.instance_id=i.id
		SET i.status='purged',i.archived_at=NOW(),i.purge_at=i.purge_at
		WHERE i.id=? AND i.status='destroyed' AND i.purge_at<=NOW()
		AND t.id=? AND t.status=? AND t.worker_id=? AND t.execution_token=?
		AND t.claim_expires_at>NOW() AND i.active_task_id=t.id
		AND i.active_task_token=t.execution_token AND i.active_task_expires_at>NOW()
		AND NOT EXISTS (SELECT 1 FROM xcloud_orders o WHERE o.instance_id=i.id AND o.status=? AND o.expires_at>NOW())`, instanceID, task.ID, taskRunning, task.WorkerID, task.ExecutionToken, orderActive)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return errInstanceStateConflict
	}
	var ownerID string
	if err := instanceDB.QueryRowContext(ctx, `SELECT owner_id FROM xcloud_instances WHERE id=?`, instanceID).Scan(&ownerID); err == nil {
		_ = createNotification(ctx, ownerID, "instance_purged", "实例数据已物理清除", "实例容器和数据已按保留规则完成物理清除。", map[string]any{"instanceId": instanceID})
	}
	return nil
}

// executeDestroyTask uses two short database phases around the Agent request.
// This avoids holding row locks during slow Docker calls while token fencing
// prevents a stale worker from committing a result after cancellation/renewal.
func executeDestroyTask(ctx context.Context, task controlTask, instanceID, containerName string, n node) error {
	var destroyAt sql.NullTime
	if err := instanceDB.QueryRowContext(ctx, `SELECT destroy_at FROM xcloud_instances WHERE id=? AND status='destroy_scheduled'`, instanceID).Scan(&destroyAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if !destroyAt.Valid || task.IdempotencyKey != lifecycleTaskKey(instanceID, "destroy", destroyAt.Time) {
		return nil
	}
	if err := taskMayCallAgent(ctx, task, "destroy_scheduled"); err != nil {
		return err
	}
	if err := nodeRequest(ctx, n, httpMethodPost, "/container/"+containerName+"/destroy", nil, nil); err != nil {
		return err
	}
	result, err := instanceDB.ExecContext(ctx, `UPDATE xcloud_instances i JOIN xcloud_tasks t ON t.instance_id=i.id
		SET i.status='destroyed',i.runtime_status='stopped',i.destroyed_at=NOW(),i.purge_at=DATE_ADD(NOW(), INTERVAL 30 DAY),i.retention_days=30,i.destroy_at=i.destroy_at
		WHERE i.id=? AND i.status='destroy_scheduled' AND i.destroy_at=?
		AND t.id=? AND t.status=? AND t.worker_id=? AND t.execution_token=?
		AND t.claim_expires_at>NOW() AND i.active_task_id=t.id
		AND i.active_task_token=t.execution_token AND i.active_task_expires_at>NOW()`, instanceID, destroyAt.Time, task.ID, taskRunning, task.WorkerID, task.ExecutionToken)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return errInstanceStateConflict
	}
	notifyInstanceRetention(ctx, instanceID, "destroyed", "实例资源已销毁", "容器已销毁，数据将在保留期结束后物理清除。")
	return nil
}

func notifyInstanceRetention(ctx context.Context, instanceID, eventType, title, intro string) {
	var ownerID string
	var purgeAt time.Time
	var retentionDays int
	if err := instanceDB.QueryRowContext(ctx, `SELECT owner_id,purge_at,retention_days FROM xcloud_instances WHERE id=?`, instanceID).Scan(&ownerID, &purgeAt, &retentionDays); err != nil {
		return
	}
	result, err := instanceDB.ExecContext(ctx, `INSERT IGNORE INTO xcloud_instance_notification_events (instance_id,event_type,created_at) VALUES (?,?,NOW())`, instanceID, eventType)
	if err != nil {
		return
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return
	}
	_ = createNotification(ctx, ownerID, "instance_retention", title, fmt.Sprintf("%s 数据保留 %d 天，预计于 %s 清理。", intro, retentionDays, purgeAt.Format("2006-01-02 15:04")), map[string]any{"instanceId": instanceID, "purgeAt": purgeAt, "retentionDays": retentionDays})
}

const httpMethodPost = "POST"
const httpMethodDelete = "DELETE"

func enqueuePersistedTask(ctx context.Context, task controlTask) error {
	if err := enqueueTask(ctx, deploymentTask{ID: task.ID, InstanceID: task.InstanceID, Action: task.Action, Attempt: task.Attempts}); err != nil {
		return err
	}
	return nil
}
func scheduleInstanceTask(ctx context.Context, instanceID, action, actorID string) (controlTask, error) {
	if instanceDB == nil {
		return controlTask{}, errors.New("开发模式未配置 MySQL")
	}
	now := time.Now()
	t := controlTask{ID: newID("task"), InstanceID: instanceID, Action: action, IdempotencyKey: action + ":" + instanceID + ":" + now.Format("20060102150405"), Status: taskPending, RunAfter: now, CreatedAt: now, UpdatedAt: now}
	_, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_tasks (id,instance_id,action,idempotency_key,status,attempts,run_after,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?)`, t.ID, t.InstanceID, t.Action, t.IdempotencyKey, t.Status, 0, t.RunAfter, now, now)
	if err != nil {
		return controlTask{}, err
	}
	_ = writeAudit(ctx, actorID, "instance."+action, "instance", instanceID, map[string]any{"taskId": t.ID})
	source := "用户操作"
	if actorID == "system" {
		source = "系统自动规则"
	}
	appendTaskEvent(ctx, t.ID, "queued", source+"：等待执行 "+action)
	return t, nil
}

// scheduleBandwidthTask keeps a single active remediation task per instance.
// Unlike user actions, it is invoked by the periodic reconciler and may run on
// multiple control-plane processes, so the INSERT itself carries the guard.
func scheduleBandwidthTask(ctx context.Context, instanceID, actorID string) (controlTask, bool, error) {
	if instanceDB == nil {
		return controlTask{}, false, errors.New("开发模式未配置 MySQL")
	}
	now := time.Now()
	t := controlTask{
		ID:             newID("task"),
		InstanceID:     instanceID,
		Action:         "bandwidth",
		IdempotencyKey: "bandwidth:" + instanceID + ":" + now.UTC().Truncate(5*time.Minute).Format("200601021504"),
		Status:         taskPending,
		RunAfter:       now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	result, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_tasks (id,instance_id,action,idempotency_key,status,attempts,run_after,created_at,updated_at)
		SELECT ?,?,?,?,?,?,?,?,? WHERE NOT EXISTS (
			SELECT 1 FROM xcloud_tasks WHERE instance_id=? AND action='bandwidth' AND status IN ('pending','running')
		)`, t.ID, t.InstanceID, t.Action, t.IdempotencyKey, t.Status, 0, t.RunAfter, now, now, instanceID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return controlTask{}, false, nil
		}
		return controlTask{}, false, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return controlTask{}, false, nil
	}
	_ = writeAudit(ctx, actorID, "bandwidth.reconcile", "instance", instanceID, map[string]any{"taskId": t.ID})
	appendTaskEvent(ctx, t.ID, "queued", "等待执行 bandwidth")
	return t, true, nil
}

func scheduleLifecycle(ctx context.Context) {
	if instanceDB == nil {
		return
	}
	rows, err := instanceDB.QueryContext(ctx, `SELECT i.id,i.status FROM xcloud_instances i WHERE i.expires_at<NOW() AND i.status IN ('running','stopped')`)
	if err == nil {
		for rows.Next() {
			var id, runtimeStatus string
			if rows.Scan(&id, &runtimeStatus) == nil {
				changed, updateErr := transitionInstance(ctx, instanceDB, id, []string{"running", "stopped"}, "destroy_scheduled", &runtimeStatus, "destroy_reason='expired',destroy_at=DATE_ADD(expires_at, INTERVAL 7 DAY),purge_at=NULL")
				if updateErr == nil {
					if !changed {
						continue
					}
					_, _ = instanceDB.ExecContext(ctx, `UPDATE xcloud_orders SET status=?,updated_at=NOW() WHERE instance_id=? AND status IN (?,?)`, orderExpired, id, orderActive, orderDeploy)
					_ = writeAudit(ctx, "system", "instance.destroy_scheduled", "instance", id, map[string]any{"reason": "expired"})
					notifyDestroyScheduled(ctx, id, "expired")
				}
			}
		}
		rows.Close()
	}
	rows, err = instanceDB.QueryContext(ctx, `SELECT i.id,i.destroy_at FROM xcloud_instances i WHERE i.status='destroy_scheduled' AND i.destroy_at<=NOW() AND NOT EXISTS (SELECT 1 FROM xcloud_tasks t WHERE t.instance_id=i.id AND t.action='destroy' AND t.status IN ('pending','running'))`)
	if err == nil {
		for rows.Next() {
			var id string
			var scheduledAt time.Time
			if rows.Scan(&id, &scheduledAt) == nil {
				if task, e := scheduleLifecycleTask(ctx, id, "destroy", scheduledAt); e == nil {
					_ = enqueuePersistedTask(ctx, task)
				}
			}
		}
		rows.Close()
	}
	rows, err = instanceDB.QueryContext(ctx, `SELECT i.id,i.purge_at FROM xcloud_instances i WHERE i.status='destroyed' AND i.purge_at<=NOW() AND NOT EXISTS (SELECT 1 FROM xcloud_tasks t WHERE t.instance_id=i.id AND t.action='purge' AND t.status IN ('pending','running'))`)
	if err == nil {
		for rows.Next() {
			var id string
			var scheduledAt time.Time
			if rows.Scan(&id, &scheduledAt) == nil {
				if task, e := scheduleLifecycleTask(ctx, id, "purge", scheduledAt); e == nil {
					_ = enqueuePersistedTask(ctx, task)
				}
			}
		}
		rows.Close()
	}
	notifyRetentionReminders(ctx)
}

func notifyDestroyScheduled(ctx context.Context, instanceID, reason string) {
	var ownerID string
	var destroyAt time.Time
	if err := instanceDB.QueryRowContext(ctx, `SELECT owner_id,destroy_at FROM xcloud_instances WHERE id=?`, instanceID).Scan(&ownerID, &destroyAt); err != nil {
		return
	}
	result, err := instanceDB.ExecContext(ctx, `INSERT IGNORE INTO xcloud_instance_notification_events (instance_id,event_type,created_at) VALUES (?,?,NOW())`, instanceID, "destroy_scheduled_"+reason)
	if err != nil {
		return
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return
	}
	message := fmt.Sprintf("服务将保持当前状态至 %s；届时将销毁容器资源。", destroyAt.Format("2006-01-02 15:04"))
	switch reason {
	case "refund":
		message = fmt.Sprintf("退款后的服务将持续至 %s；届时将销毁容器资源，数据再保留 30 天。", destroyAt.Format("2006-01-02 15:04"))
	case "manual":
		message = fmt.Sprintf("实例将保持当前运行状态至 %s；届时将销毁容器资源，数据再保留 30 天。", destroyAt.Format("2006-01-02 15:04"))
	case "expired":
		message = fmt.Sprintf("服务已到期，仍可继续使用至 %s；届时将销毁容器资源，数据再保留 30 天。", destroyAt.Format("2006-01-02 15:04"))
	}
	_ = createNotification(ctx, ownerID, "instance_destroy_scheduled", "实例已进入待销毁", message, map[string]any{"instanceId": instanceID, "destroyAt": destroyAt, "reason": reason})
}

func scheduleLifecycleTask(ctx context.Context, instanceID, action string, at time.Time) (controlTask, error) {
	return scheduleLifecycleTaskAt(ctx, instanceID, action, at, time.Now())
}

// scheduleLifecycleTaskAt keeps the scheduled lifecycle timestamp in the
// idempotency key while allowing a future destroy plan to be visible in the
// user's execution history before it is eligible for delivery.
func scheduleLifecycleTaskAt(ctx context.Context, instanceID, action string, at, runAfter time.Time) (controlTask, error) {
	key := lifecycleTaskKey(instanceID, action, at)
	now := time.Now()
	t := controlTask{ID: newID("task"), InstanceID: instanceID, Action: action, IdempotencyKey: key, Status: taskPending, RunAfter: runAfter, CreatedAt: now, UpdatedAt: now}
	result, err := instanceDB.ExecContext(ctx, `INSERT IGNORE INTO xcloud_tasks (id,instance_id,action,idempotency_key,status,attempts,run_after,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?)`, t.ID, t.InstanceID, t.Action, t.IdempotencyKey, t.Status, 0, t.RunAfter, t.CreatedAt, t.UpdatedAt)
	if err != nil {
		return controlTask{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		if err := scanControlTask(instanceDB.QueryRowContext(ctx, `SELECT `+taskSelectFields+` FROM xcloud_tasks WHERE idempotency_key=?`, key), &t); err != nil {
			return controlTask{}, err
		}
		return t, nil
	}
	appendTaskEvent(ctx, t.ID, "queued", "系统自动规则：等待执行 "+action)
	return t, nil
}

func lifecycleTaskKey(instanceID, action string, at time.Time) string {
	return action + ":" + instanceID + ":" + at.UTC().Format(time.RFC3339Nano)
}

func notifyRetentionReminders(ctx context.Context) {
	for _, reminder := range []struct {
		eventType string
		window    string
		message   string
	}{
		{"purge_7d", "purge_at > NOW() + INTERVAL 6 DAY AND purge_at <= NOW() + INTERVAL 7 DAY", "数据将在约 7 天后清理，请及时备份。"},
		{"purge_1d", "purge_at > NOW() AND purge_at <= NOW() + INTERVAL 1 DAY", "数据将在约 1 天后清理，请及时备份。"},
	} {
		rows, err := instanceDB.QueryContext(ctx, `SELECT id,owner_id,purge_at FROM xcloud_instances WHERE status='destroyed' AND archived_at IS NULL AND `+reminder.window)
		if err != nil {
			continue
		}
		for rows.Next() {
			var instanceID, ownerID string
			var purgeAt time.Time
			if rows.Scan(&instanceID, &ownerID, &purgeAt) != nil {
				continue
			}
			result, err := instanceDB.ExecContext(ctx, `INSERT IGNORE INTO xcloud_instance_notification_events (instance_id,event_type,created_at) VALUES (?,?,NOW())`, instanceID, reminder.eventType)
			if err == nil {
				if affected, _ := result.RowsAffected(); affected == 1 {
					_ = createNotification(ctx, ownerID, "instance_retention", "实例数据清理提醒", fmt.Sprintf("%s 预计清理时间：%s。", reminder.message, purgeAt.Format("2006-01-02 15:04")), map[string]any{"instanceId": instanceID, "purgeAt": purgeAt})
				}
			}
		}
		rows.Close()
	}
}

func listNodesWithUsage(ctx context.Context) ([]node, error) {
	rows, err := instanceDB.QueryContext(ctx, `SELECT n.id,n.name,n.agent_url,n.cpu_total,n.memory_total_mb,n.cpu_detected,n.memory_detected_mb,n.enabled,n.last_heartbeat_at,COALESCE(n.docker_version,''),COALESCE(n.disk_available_bytes,0),COALESCE(n.managed_container_count,0),COALESCE(n.agent_version,''),COALESCE(n.agent_api_version,0),COALESCE(n.agent_capabilities,JSON_ARRAY()),COALESCE(n.last_agent_error,''),COALESCE(SUM(CASE WHEN i.status IN ('deploying','running','stopped','destroy_scheduled') THEN i.cpu ELSE 0 END),0),COALESCE(SUM(CASE WHEN i.status IN ('deploying','running','stopped','destroy_scheduled') THEN i.memory_mb ELSE 0 END),0),COALESCE(SUM(CASE WHEN i.status IN ('destroy_scheduled','destroyed') THEN 1 ELSE 0 END),0),COALESCE((SELECT COUNT(*) FROM xcloud_tasks t JOIN xcloud_instances ti ON ti.id=t.instance_id WHERE ti.node_id=n.id AND t.action IN ('destroy','purge') AND t.status IN ('pending','running')),0) FROM xcloud_nodes n LEFT JOIN xcloud_instances i ON i.node_id=n.id GROUP BY n.id ORDER BY n.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []node{}
	for rows.Next() {
		var item node
		var capabilities []byte
		if err := rows.Scan(&item.ID, &item.Name, &item.AgentURL, &item.CPUTotal, &item.MemoryTotalMB, &item.CPUDetected, &item.MemoryDetectedMB, &item.Enabled, &item.LastHeartbeatAt, &item.DockerVersion, &item.DiskAvailableBytes, &item.ManagedContainerCount, &item.AgentVersion, &item.AgentAPIVersion, &capabilities, &item.LastAgentError, &item.CPUReserved, &item.MemoryReservedMB, &item.OfflineInstanceCount, &item.PendingCleanupTasks); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(capabilities, &item.AgentCapabilities)
		item.AgentCompatibility = item.compatibility()
		items = append(items, item)
	}
	return items, rows.Err()
}

func saveNode(ctx context.Context, value node) error {
	if value.ID == "" {
		value.ID = newID("node")
	}
	if strings.TrimSpace(value.Name) == "" {
		return errors.New("节点参数无效")
	}
	if value.CPUTotal < 0 || value.MemoryTotalMB < 0 || value.CPUTotal > value.CPUDetected || value.MemoryTotalMB > value.MemoryDetectedMB {
		return errors.New("可调度容量不能超过节点硬件")
	}
	if value.Enabled && (value.CPUTotal <= 0 || value.MemoryTotalMB < 256) {
		return errors.New("启用节点前请确认可调度容量")
	}
	if !strings.HasPrefix(value.AgentURL, "http://") && !strings.HasPrefix(value.AgentURL, "https://") {
		return errors.New("Agent 地址必须是 http 或 https 地址")
	}
	var cipherText any = nil
	if strings.TrimSpace(value.AgentToken) != "" {
		encoded, err := encryptNodeToken(strings.TrimSpace(value.AgentToken))
		if err != nil {
			return err
		}
		cipherText = encoded
	}
	if cipherText == nil {
		var existing string
		err := instanceDB.QueryRowContext(ctx, `SELECT COALESCE(agent_token_ciphertext,'') FROM xcloud_nodes WHERE id=?`, value.ID).Scan(&existing)
		if err == sql.ErrNoRows {
			return errors.New("新增节点必须提供 Agent 令牌")
		}
		if err != nil {
			return err
		}
		cipherText = existing
	}
	capabilities, _ := json.Marshal(value.AgentCapabilities)
	if len(value.AgentCapabilities) == 0 {
		capabilities = []byte("[]")
	}
	_, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_nodes (id,name,agent_url,agent_token_ciphertext,cpu_total,memory_total_mb,cpu_detected,memory_detected_mb,agent_version,agent_api_version,agent_capabilities,enabled,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,NOW(),NOW()) ON DUPLICATE KEY UPDATE name=VALUES(name),agent_url=VALUES(agent_url),agent_token_ciphertext=VALUES(agent_token_ciphertext),cpu_total=VALUES(cpu_total),memory_total_mb=VALUES(memory_total_mb),cpu_detected=VALUES(cpu_detected),memory_detected_mb=VALUES(memory_detected_mb),agent_version=IF(VALUES(agent_version)='',agent_version,VALUES(agent_version)),agent_api_version=IF(VALUES(agent_api_version)=0,agent_api_version,VALUES(agent_api_version)),agent_capabilities=IF(COALESCE(JSON_LENGTH(VALUES(agent_capabilities)),0)=0,agent_capabilities,VALUES(agent_capabilities)),enabled=VALUES(enabled),updated_at=NOW()`, value.ID, value.Name, value.AgentURL, cipherText, value.CPUTotal, value.MemoryTotalMB, value.CPUDetected, value.MemoryDetectedMB, value.AgentVersion, value.AgentAPIVersion, string(capabilities), value.Enabled)
	return err
}

func adminMetrics(ctx context.Context) (map[string]any, error) {
	nodes, err := listNodesWithUsage(ctx)
	if err != nil {
		return nil, err
	}
	var failures, pending, openTickets, urgentTickets, deploymentFailed, runtimeMissing, destroyBlocked, offlineInstances, leaseRecoveries, needsReview int
	if err = instanceDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM xcloud_tasks WHERE status=?`, taskFailed).Scan(&failures); err != nil {
		return nil, err
	}
	if err = instanceDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM xcloud_tasks WHERE status IN (?,?)`, taskPending, taskRunning).Scan(&pending); err != nil {
		return nil, err
	}
	if err = instanceDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM xcloud_tickets WHERE status<>?`, ticketClosed).Scan(&openTickets); err != nil {
		return nil, err
	}
	if err = instanceDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM xcloud_tickets WHERE status<>? AND priority=?`, ticketClosed, "urgent").Scan(&urgentTickets); err != nil {
		return nil, err
	}
	queries := []struct {
		query string
		dest  *int
		args  []any
	}{
		{query: `SELECT COUNT(*) FROM xcloud_instances WHERE status='deployment_failed'`, dest: &deploymentFailed},
		{query: `SELECT COUNT(*) FROM xcloud_instances WHERE runtime_status='missing'`, dest: &runtimeMissing},
		{query: `SELECT COUNT(*) FROM xcloud_instances WHERE status='destroy_scheduled' AND destroy_at<=NOW()`, dest: &destroyBlocked},
		{query: `SELECT COUNT(*) FROM xcloud_instances i JOIN xcloud_nodes n ON n.id=i.node_id WHERE i.status IN ('destroy_scheduled','destroyed') AND (n.enabled=FALSE OR n.last_heartbeat_at IS NULL OR n.last_heartbeat_at<?)`, dest: &offlineInstances, args: []any{time.Now().Add(-nodeHeartbeatTTL())}},
		{query: `SELECT COUNT(*) FROM xcloud_task_events WHERE event_type='lease_recovered' AND created_at>=NOW()-INTERVAL 24 HOUR`, dest: &leaseRecoveries},
		{query: `SELECT COUNT(*) FROM xcloud_tasks WHERE status='needs_review'`, dest: &needsReview},
	}
	for _, item := range queries {
		if err = instanceDB.QueryRowContext(ctx, item.query, item.args...).Scan(item.dest); err != nil {
			return nil, err
		}
	}
	return map[string]any{"nodes": nodes, "taskFailures": failures, "taskBacklog": pending, "openTickets": openTickets, "urgentTickets": urgentTickets, "deploymentFailed": deploymentFailed, "runtimeMissing": runtimeMissing, "destroyBlocked": destroyBlocked, "offlineInstances": offlineInstances, "leaseRecoveries24h": leaseRecoveries, "tasksNeedsReview": needsReview}, nil
}
