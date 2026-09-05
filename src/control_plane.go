package cloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
)

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

type plan struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	CPU        float64   `json:"cpu"`
	MemoryMB   int       `json:"memoryMB"`
	MonthlyFen int       `json:"monthlyPriceFen"`
	Enabled    bool      `json:"enabled"`
	SortOrder  int       `json:"sortOrder"`
	CreatedAt  time.Time `json:"createdAt"`
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
	PromotionSnapshot   json.RawMessage `json:"promotionSnapshot,omitempty"`
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
	ID             string    `json:"id"`
	InstanceID     string    `json:"instanceId"`
	Action         string    `json:"action"`
	IdempotencyKey string    `json:"-"`
	Status         string    `json:"status"`
	LastError      string    `json:"lastError"`
	Attempts       int       `json:"attempts"`
	RunAfter       time.Time `json:"runAfter"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
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
	return strings.Contains(message, "duplicate column") || strings.Contains(message, "duplicate key") || strings.Contains(message, "already exists")
}

func listCatalog(ctx context.Context, includeDisabled bool) ([]catalogImage, []plan, error) {
	if instanceDB == nil {
		return nil, nil, errors.New("开发模式未配置 MySQL")
	}
	imageSQL := `SELECT id,name,image_ref,COALESCE(image_digest,''),version,enabled,created_at FROM xcloud_images`
	planSQL := `SELECT id,name,cpu,memory_mb,monthly_price_fen,enabled,sort_order,created_at FROM xcloud_plans`
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
	return images, plans, err
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
		if err := rows.Scan(&item.ID, &item.Name, &item.CPU, &item.MemoryMB, &item.MonthlyFen, &item.Enabled, &item.SortOrder, &item.CreatedAt); err != nil {
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
		checkDuplicate = err == sql.ErrNoRows || strings.TrimSpace(currentRef) != value.ImageRef
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
	_, err = instanceDB.ExecContext(ctx, `INSERT IGNORE INTO xcloud_image_versions (id,image_id,version_tag,image_digest,enabled,version_status,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?)`, newID("ver"), value.ID, "latest", nil, false, "draft", value.CreatedAt, time.Now())
	return err
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
	_, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_plans (id,name,cpu,memory_mb,monthly_price_fen,enabled,sort_order,created_at) VALUES (?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE name=VALUES(name),cpu=VALUES(cpu),memory_mb=VALUES(memory_mb),monthly_price_fen=VALUES(monthly_price_fen),enabled=VALUES(enabled),sort_order=VALUES(sort_order)`, value.ID, value.Name, value.CPU, value.MemoryMB, value.MonthlyFen, value.Enabled, value.SortOrder, value.CreatedAt)
	return err
}

func createOrder(ctx context.Context, ownerID, planID, imageID string, months int, note string) (order, error) {
	if months < 1 || months > 24 {
		return order{}, errors.New("订阅周期应为 1 至 24 个月")
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
	return scanOrders(ctx, `SELECT o.id,o.owner_id,o.plan_id,o.image_id,COALESCE(o.instance_id,''),o.amount_fen,COALESCE(o.list_amount_fen,o.amount_fen),COALESCE(o.discount_amount_fen,0),o.promotion_snapshot,o.status,COALESCE(o.payment_note,''),o.service_starts_at,o.expires_at,o.refunded_at,COALESCE(o.refund_amount_fen,0),COALESCE(o.refund_wallet_entry_id,''),o.created_at,o.updated_at,p.name,i.name,i.version FROM xcloud_orders o JOIN xcloud_plans p ON p.id=o.plan_id JOIN xcloud_images i ON i.id=o.image_id WHERE o.owner_id=? ORDER BY o.created_at DESC`, ownerID)
}
func listAllOrders(ctx context.Context) ([]order, error) {
	return scanOrders(ctx, `SELECT o.id,o.owner_id,o.plan_id,o.image_id,COALESCE(o.instance_id,''),o.amount_fen,COALESCE(o.list_amount_fen,o.amount_fen),COALESCE(o.discount_amount_fen,0),o.promotion_snapshot,o.status,COALESCE(o.payment_note,''),o.service_starts_at,o.expires_at,o.refunded_at,COALESCE(o.refund_amount_fen,0),COALESCE(o.refund_wallet_entry_id,''),o.created_at,o.updated_at,p.name,i.name,i.version FROM xcloud_orders o JOIN xcloud_plans p ON p.id=o.plan_id JOIN xcloud_images i ON i.id=o.image_id ORDER BY o.created_at DESC`)
}
func scanOrders(ctx context.Context, statement string, args ...any) ([]order, error) {
	rows, err := instanceDB.QueryContext(ctx, statement, args...)
	if err != nil {
		// A rolling deployment may briefly run the new binary against a database
		// whose optional promotion migration has not completed. Orders must stay
		// readable during that window; retry with legacy-compatible projections.
		if !isMissingOrderUpgrade(err) {
			return nil, err
		}
		legacy := strings.NewReplacer(
			"COALESCE(o.list_amount_fen,o.amount_fen)", "o.amount_fen",
			"COALESCE(o.discount_amount_fen,0)", "0",
			"o.promotion_snapshot", "NULL",
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
		var snapshot []byte
		if err := rows.Scan(&v.ID, &v.OwnerID, &v.PlanID, &v.ImageID, &v.InstanceID, &v.AmountFen, &v.ListAmountFen, &v.DiscountAmountFen, &snapshot, &v.Status, &v.PaymentNote, &v.ServiceStartsAt, &v.ExpiresAt, &v.RefundedAt, &v.RefundAmountFen, &v.RefundWalletEntryID, &v.CreatedAt, &v.UpdatedAt, &v.PlanName, &v.ImageName, &v.ImageVersion); err != nil {
			return nil, err
		}
		v.PromotionSnapshot = snapshot
		items = append(items, v)
	}
	return items, rows.Err()
}

func isMissingOrderUpgrade(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unknown column") && (strings.Contains(message, "promotion_snapshot") || strings.Contains(message, "list_amount_fen") || strings.Contains(message, "discount_amount_fen"))
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
	err = tx.QueryRowContext(ctx, `SELECT o.id,o.owner_id,o.plan_id,o.image_id,o.amount_fen,o.status,p.id,p.name,p.cpu,p.memory_mb,p.monthly_price_fen,p.enabled,p.sort_order,p.created_at,i.id,i.name,i.image_ref,COALESCE(i.image_digest,''),i.version,i.enabled,i.created_at FROM xcloud_orders o JOIN xcloud_plans p ON p.id=o.plan_id JOIN xcloud_images i ON i.id=o.image_id WHERE o.id=? FOR UPDATE`, id).Scan(&o.ID, &o.OwnerID, &o.PlanID, &o.ImageID, &o.AmountFen, &o.Status, &p.ID, &p.Name, &p.CPU, &p.MemoryMB, &p.MonthlyFen, &p.Enabled, &p.SortOrder, &p.CreatedAt, &img.ID, &img.Name, &img.ImageRef, &img.ImageDigest, &img.Version, &img.Enabled, &img.CreatedAt)
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
		var currentExpiry sql.NullTime
		if err = tx.QueryRowContext(ctx, `SELECT expires_at FROM xcloud_instances WHERE id=? AND owner_id=? FOR UPDATE`, renewal.String, o.OwnerID).Scan(&currentExpiry); err != nil {
			return controlTask{}, errors.New("待续费实例不存在或已清理")
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
		if _, err = tx.ExecContext(ctx, `UPDATE xcloud_instances SET status='deploying',expires_at=?,purge_at=NULL WHERE id=?`, expires, renewal.String); err != nil {
			return controlTask{}, err
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
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_instances (id,owner_id,name,image,version,spec,status,access_address,container_name,created_at,cpu,memory_mb,node_id,order_id,route_key,expires_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, instanceID, o.OwnerID, "AlemonX", img.ImageRef, img.Version, fmt.Sprintf("%g 核 / %d GB", p.CPU, p.MemoryMB/1024), "deploying", "https://xcloud-"+route+"."+env("XCLOUD_INSTANCE_DOMAIN", "alemonjs.com"), container, now, p.CPU, p.MemoryMB, n.ID, o.ID, route, expires); err != nil {
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

func loadTask(ctx context.Context, id string) (controlTask, error) {
	var task controlTask
	err := instanceDB.QueryRowContext(ctx, `SELECT id,instance_id,action,idempotency_key,status,attempts,COALESCE(last_error,''),run_after,created_at,updated_at FROM xcloud_tasks WHERE id=?`, id).Scan(&task.ID, &task.InstanceID, &task.Action, &task.IdempotencyKey, &task.Status, &task.Attempts, &task.LastError, &task.RunAfter, &task.CreatedAt, &task.UpdatedAt)
	return task, err
}

func pendingTasks(ctx context.Context, limit int) ([]controlTask, error) {
	rows, err := instanceDB.QueryContext(ctx, `SELECT id,instance_id,action,idempotency_key,status,attempts,COALESCE(last_error,''),run_after,created_at,updated_at FROM xcloud_tasks WHERE status=? AND run_after<=NOW() ORDER BY created_at LIMIT ?`, taskPending, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []controlTask{}
	for rows.Next() {
		var t controlTask
		if err := rows.Scan(&t.ID, &t.InstanceID, &t.Action, &t.IdempotencyKey, &t.Status, &t.Attempts, &t.LastError, &t.RunAfter, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, t)
	}
	return items, rows.Err()
}

func listTasks(ctx context.Context, limit int) ([]controlTask, error) {
	rows, err := instanceDB.QueryContext(ctx, `SELECT id,instance_id,action,idempotency_key,status,attempts,COALESCE(last_error,''),run_after,created_at,updated_at FROM xcloud_tasks ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []controlTask{}
	for rows.Next() {
		var t controlTask
		if err := rows.Scan(&t.ID, &t.InstanceID, &t.Action, &t.IdempotencyKey, &t.Status, &t.Attempts, &t.LastError, &t.RunAfter, &t.CreatedAt, &t.UpdatedAt); err != nil {
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

func claimTask(ctx context.Context, id string) (bool, error) {
	result, err := instanceDB.ExecContext(ctx, `UPDATE xcloud_tasks SET status=?,attempts=attempts+1,updated_at=NOW() WHERE id=? AND status=?`, taskRunning, id, taskPending)
	if err != nil {
		return false, err
	}
	affected, _ := result.RowsAffected()
	if affected == 1 {
		appendTaskEvent(ctx, id, "claimed", "消费者已领取任务")
	}
	return affected == 1, nil
}
func finishTask(ctx context.Context, task controlTask, err error) error {
	if err == nil {
		_, e := instanceDB.ExecContext(ctx, `UPDATE xcloud_tasks SET status=?,last_error=NULL,finished_at=NOW(),updated_at=NOW() WHERE id=?`, taskDone, task.ID)
		if e == nil {
			appendTaskEvent(ctx, task.ID, "succeeded", "任务执行成功")
		}
		return e
	}
	message := truncateError(err.Error())
	if task.Attempts >= 3 {
		_, e := instanceDB.ExecContext(ctx, `UPDATE xcloud_tasks SET status=?,last_error=?,finished_at=NOW(),updated_at=NOW() WHERE id=?`, taskFailed, message, task.ID)
		if e == nil {
			appendTaskEvent(ctx, task.ID, "dead_letter", message)
		}
		return e
	}
	delay := time.Duration(task.Attempts*task.Attempts) * time.Minute
	_, e := instanceDB.ExecContext(ctx, `UPDATE xcloud_tasks SET status=?,last_error=?,run_after=?,updated_at=NOW() WHERE id=?`, taskPending, message, time.Now().Add(delay), task.ID)
	if e == nil {
		appendTaskEvent(ctx, task.ID, "retry", message)
	}
	return e
}
func truncateError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 1000 {
		return value[:1000]
	}
	return value
}

func executeTask(ctx context.Context, task controlTask) error {
	var item instance
	var route string
	var nodeID string
	err := instanceDB.QueryRowContext(ctx, `SELECT id,owner_id,name,image,version,spec,status,access_address,container_name,created_at,route_key,COALESCE(node_id,'') FROM xcloud_instances WHERE id=?`, task.InstanceID).Scan(&item.ID, &item.OwnerID, &item.Name, &item.Image, &item.Version, &item.Spec, &item.Status, &item.IP, &item.ContainerName, &item.CreatedAt, &route, &nodeID)
	if err != nil {
		return err
	}
	n, err := nodeByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("实例节点不可用: %w", err)
	}
	switch task.Action {
	case "create":
		var cpu float64
		var memoryMB int
		err = instanceDB.QueryRowContext(ctx, `SELECT cpu,memory_mb FROM xcloud_instances WHERE id=?`, item.ID).Scan(&cpu, &memoryMB)
		if err != nil {
			return err
		}
		var imageRef, digest, selectedVersion string
		err = instanceDB.QueryRowContext(ctx, `SELECT i.image_ref,COALESCE(o.selected_image_digest,i.image_digest,''),COALESCE(o.selected_image_version,i.version) FROM xcloud_orders o JOIN xcloud_images i ON i.id=o.image_id WHERE o.instance_id=? ORDER BY o.created_at DESC LIMIT 1`, item.ID).Scan(&imageRef, &digest, &selectedVersion)
		if err != nil {
			return err
		}
		image := deploymentImage(imageRef, selectedVersion, digest)
		err = nodeRequest(ctx, n, httpMethodPost, "/container/create", map[string]any{"name": item.ContainerName, "image": image, "cpu": cpu, "memoryMB": memoryMB, "route": route}, nil)
		if err == nil {
			_, err = instanceDB.ExecContext(ctx, `UPDATE xcloud_instances SET status='running' WHERE id=?`, item.ID)
			if err == nil {
				_, err = instanceDB.ExecContext(ctx, `UPDATE xcloud_orders SET status=?,updated_at=NOW() WHERE instance_id=? AND status=?`, orderActive, item.ID, orderDeploy)
			}
		}
	case "start":
		err = nodeRequest(ctx, n, httpMethodPost, "/container/"+item.ContainerName+"/start", nil, nil)
		if err == nil {
			_, err = instanceDB.ExecContext(ctx, `UPDATE xcloud_instances SET status='running' WHERE id=?`, item.ID)
			if err == nil {
				_, err = instanceDB.ExecContext(ctx, `UPDATE xcloud_orders SET status=?,updated_at=NOW() WHERE instance_id=? AND status=?`, orderActive, item.ID, orderDeploy)
			}
		}
	case "stop":
		err = nodeRequest(ctx, n, httpMethodPost, "/container/"+item.ContainerName+"/stop", nil, nil)
		if err == nil {
			_, err = instanceDB.ExecContext(ctx, `UPDATE xcloud_instances SET status='stopped' WHERE id=?`, item.ID)
		}
	case "restart":
		// The Agent deliberately exposes only the primitive lifecycle calls.  A
		// restart remains an auditable persisted task while preserving that small
		// Agent surface: stop must complete before start is attempted.
		err = nodeRequest(ctx, n, httpMethodPost, "/container/"+item.ContainerName+"/stop", nil, nil)
		if err == nil {
			err = nodeRequest(ctx, n, httpMethodPost, "/container/"+item.ContainerName+"/start", nil, nil)
		}
		if err == nil {
			_, err = instanceDB.ExecContext(ctx, `UPDATE xcloud_instances SET status='running' WHERE id=?`, item.ID)
		}
	case "expire-stop":
		err = nodeRequest(ctx, n, httpMethodPost, "/container/"+item.ContainerName+"/stop", nil, nil)
		if err == nil {
			_, err = instanceDB.ExecContext(ctx, `UPDATE xcloud_instances SET status='expired',purge_at=DATE_ADD(NOW(), INTERVAL retention_days DAY) WHERE id=?`, item.ID)
			if err == nil {
				_, err = instanceDB.ExecContext(ctx, `UPDATE xcloud_orders SET status=?,updated_at=NOW() WHERE instance_id=? AND status IN (?,?)`, orderExpired, item.ID, orderActive, orderDeploy)
				if err == nil {
					notifyInstanceRetention(ctx, item.ID, "stopped", "实例已停止", "服务已到期并停止，数据将在保留期结束后清理。")
				}
			}
		}
	case "delete":
		err = nodeRequest(ctx, n, httpMethodPost, "/container/"+item.ContainerName+"/stop", nil, nil)
		if err == nil {
			_, err = instanceDB.ExecContext(ctx, `UPDATE xcloud_instances SET status='retention',purge_at=DATE_ADD(NOW(), INTERVAL retention_days DAY) WHERE id=?`, item.ID)
			if err == nil {
				notifyInstanceRetention(ctx, item.ID, "deleted", "实例已停止", "实例已删除，数据将在保留期结束后清理。")
			}
		}
	case "purge":
		err = nodeRequest(ctx, n, httpMethodDelete, "/container/"+item.ContainerName+"?purge=true", nil, nil)
		if err == nil {
			_, err = instanceDB.ExecContext(ctx, `DELETE FROM xcloud_instances WHERE id=?`, item.ID)
		}
	default:
		return errors.New("未知任务动作")
	}
	return err
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
	appendTaskEvent(ctx, t.ID, "queued", "等待执行 "+action)
	return t, nil
}

func scheduleLifecycle(ctx context.Context) {
	if instanceDB == nil {
		return
	}
	rows, err := instanceDB.QueryContext(ctx, `SELECT i.id FROM xcloud_instances i WHERE i.expires_at<NOW() AND i.status IN ('running','stopped') AND NOT EXISTS (SELECT 1 FROM xcloud_tasks t WHERE t.instance_id=i.id AND t.action='expire-stop' AND t.status IN ('pending','running'))`)
	if err == nil {
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				if task, e := scheduleInstanceTask(ctx, id, "expire-stop", "system"); e == nil {
					_ = enqueuePersistedTask(ctx, task)
				}
			}
		}
		rows.Close()
	}
	rows, err = instanceDB.QueryContext(ctx, `SELECT i.id FROM xcloud_instances i WHERE i.purge_at<NOW() AND i.status IN ('expired','retention') AND NOT EXISTS (SELECT 1 FROM xcloud_tasks t WHERE t.instance_id=i.id AND t.action='purge' AND t.status IN ('pending','running'))`)
	if err == nil {
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				if task, e := scheduleInstanceTask(ctx, id, "purge", "system"); e == nil {
					_ = enqueuePersistedTask(ctx, task)
				}
			}
		}
		rows.Close()
	}
	notifyRetentionReminders(ctx)
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
		rows, err := instanceDB.QueryContext(ctx, `SELECT id,owner_id,purge_at FROM xcloud_instances WHERE status IN ('expired','retention') AND `+reminder.window)
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
	rows, err := instanceDB.QueryContext(ctx, `SELECT n.id,n.name,n.agent_url,n.cpu_total,n.memory_total_mb,n.cpu_detected,n.memory_detected_mb,n.enabled,n.last_heartbeat_at,COALESCE(n.docker_version,''),COALESCE(n.disk_available_bytes,0),COALESCE(n.managed_container_count,0),COALESCE(n.agent_version,''),COALESCE(n.agent_api_version,0),COALESCE(n.agent_capabilities,JSON_ARRAY()),COALESCE(SUM(CASE WHEN i.status IN ('deploying','running','stopped','expired','retention') THEN i.cpu ELSE 0 END),0),COALESCE(SUM(CASE WHEN i.status IN ('deploying','running','stopped','expired','retention') THEN i.memory_mb ELSE 0 END),0) FROM xcloud_nodes n LEFT JOIN xcloud_instances i ON i.node_id=n.id GROUP BY n.id ORDER BY n.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []node{}
	for rows.Next() {
		var item node
		var capabilities []byte
		if err := rows.Scan(&item.ID, &item.Name, &item.AgentURL, &item.CPUTotal, &item.MemoryTotalMB, &item.CPUDetected, &item.MemoryDetectedMB, &item.Enabled, &item.LastHeartbeatAt, &item.DockerVersion, &item.DiskAvailableBytes, &item.ManagedContainerCount, &item.AgentVersion, &item.AgentAPIVersion, &capabilities, &item.CPUReserved, &item.MemoryReservedMB); err != nil {
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
	var failures, pending, openTickets, urgentTickets int
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
	return map[string]any{"nodes": nodes, "taskFailures": failures, "taskBacklog": pending, "openTickets": openTickets, "urgentTickets": urgentTickets}, nil
}
