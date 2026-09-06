package cloud

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

type cloudUser struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	Email       string    `json:"email"`
	LastLoginAt time.Time `json:"lastLoginAt"`
	BalanceFen  int       `json:"balanceFen"`
}
type walletEntry struct {
	ID              string    `json:"id"`
	UserID          string    `json:"userId"`
	AmountFen       int       `json:"amountFen"`
	BalanceAfterFen int       `json:"balanceAfterFen"`
	Type            string    `json:"type"`
	Note            string    `json:"note"`
	ActorID         string    `json:"actorId"`
	OrderID         string    `json:"orderId,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
}
type notification struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Title     string          `json:"title"`
	Body      string          `json:"body"`
	Data      json.RawMessage `json:"data"`
	ReadAt    *time.Time      `json:"readAt"`
	CreatedAt time.Time       `json:"createdAt"`
}
type taskEvent struct {
	ID        int64     `json:"id"`
	TaskID    string    `json:"taskId"`
	Event     string    `json:"event"`
	Detail    string    `json:"detail"`
	CreatedAt time.Time `json:"createdAt"`
}

func initializeUpgradeSchema(ctx context.Context) error {
	_ = ctx
	return nil
}

// Older deployments could contain the same repository more than once because
// uniqueness used to exist only in application code.  Repository source is the
// product identity, so migrate all historical orders to the oldest record
// before enforcing the database constraint.
func normalizeImageSources(ctx context.Context) error {
	tx, err := instanceDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT image_ref FROM xcloud_images GROUP BY image_ref HAVING COUNT(*) > 1`)
	if err != nil {
		return err
	}
	var refs []string
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			rows.Close()
			return err
		}
		refs = append(refs, ref)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, ref := range refs {
		imageRows, err := tx.QueryContext(ctx, `SELECT id FROM xcloud_images WHERE image_ref=? ORDER BY created_at,id FOR UPDATE`, ref)
		if err != nil {
			return err
		}
		var ids []string
		for imageRows.Next() {
			var id string
			if err := imageRows.Scan(&id); err != nil {
				imageRows.Close()
				return err
			}
			ids = append(ids, id)
		}
		if err := imageRows.Close(); err != nil {
			return err
		}
		if len(ids) < 2 {
			continue
		}
		for _, duplicateID := range ids[1:] {
			if _, err := tx.ExecContext(ctx, `UPDATE xcloud_orders SET image_id=? WHERE image_id=?`, ids[0], duplicateID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM xcloud_images WHERE id=?`, duplicateID); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// Bootstrap rows were useful in early demos but are unsafe operational data:
// every node, source image and plan must now be explicitly created by an admin.
// This migration runs once and never touches rows created later by an operator.
func removeBootstrapData(ctx context.Context) error {
	var marker int
	if err := instanceDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM xcloud_settings WHERE setting_key='bootstrap_data_cleanup_v1'`).Scan(&marker); err != nil {
		return err
	}
	if marker > 0 {
		return nil
	}
	for _, statement := range []string{
		`DELETE FROM xcloud_nodes WHERE id='node-default' AND NOT EXISTS (SELECT 1 FROM xcloud_instances WHERE node_id='node-default')`,
		`DELETE FROM xcloud_images WHERE id='image-alemonx-latest' AND NOT EXISTS (SELECT 1 FROM xcloud_orders WHERE image_id='image-alemonx-latest')`,
		`DELETE FROM xcloud_plans WHERE id IN ('plan-starter','plan-standard','plan-pro') AND NOT EXISTS (SELECT 1 FROM xcloud_orders WHERE plan_id=xcloud_plans.id)`,
	} {
		if _, err := instanceDB.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	_, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_settings (setting_key,setting_value,updated_at) VALUES ('bootstrap_data_cleanup_v1',JSON_OBJECT('completed',TRUE),NOW())`)
	return err
}

func syncCloudUser(ctx context.Context, user oidcUser) error {
	if instanceDB == nil {
		return nil
	}
	now := time.Now()
	_, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_users (id,username,email,last_login_at,created_at) VALUES (?,?,?,?,?) ON DUPLICATE KEY UPDATE username=VALUES(username),email=VALUES(email),last_login_at=VALUES(last_login_at)`, user.ID, user.Username, user.Email, now, now)
	if err != nil {
		return err
	}
	_, err = instanceDB.ExecContext(ctx, `INSERT IGNORE INTO xcloud_wallets (user_id,balance_fen,updated_at) VALUES (?,0,?)`, user.ID, now)
	return err
}

func encryptionKey() ([]byte, error) {
	raw := strings.TrimSpace(env("XCLOUD_NODE_TOKEN_ENCRYPTION_KEY", ""))
	if raw == "" {
		return nil, errors.New("缺少 XCLOUD_NODE_TOKEN_ENCRYPTION_KEY")
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		key, err = base64.RawStdEncoding.DecodeString(raw)
	}
	if err != nil || len(key) != 32 {
		return nil, errors.New("XCLOUD_NODE_TOKEN_ENCRYPTION_KEY 必须是 base64 编码的 32 字节密钥")
	}
	return key, nil
}
func encryptNodeToken(value string) (string, error) {
	key, err := encryptionKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(append(nonce, gcm.Seal(nil, nonce, []byte(value), nil)...)), nil
}
func decryptNodeToken(value string) (string, error) {
	key, err := encryptionKey()
	if err != nil {
		return "", err
	}
	raw, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("节点令牌密文无效")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	return string(plain), err
}

func walletForUser(ctx context.Context, id string) (cloudUser, error) {
	var item cloudUser
	err := instanceDB.QueryRowContext(ctx, `SELECT u.id,u.username,u.email,u.last_login_at,w.balance_fen FROM xcloud_users u JOIN xcloud_wallets w ON w.user_id=u.id WHERE u.id=?`, id).Scan(&item.ID, &item.Username, &item.Email, &item.LastLoginAt, &item.BalanceFen)
	return item, err
}
func walletEntries(ctx context.Context, id string, limit int) ([]walletEntry, error) {
	rows, err := instanceDB.QueryContext(ctx, `SELECT id,user_id,amount_fen,balance_after_fen,entry_type,note,actor_id,COALESCE(order_id,''),created_at FROM xcloud_wallet_entries WHERE user_id=? ORDER BY created_at DESC LIMIT ?`, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []walletEntry{}
	for rows.Next() {
		var item walletEntry
		if err := rows.Scan(&item.ID, &item.UserID, &item.AmountFen, &item.BalanceAfterFen, &item.Type, &item.Note, &item.ActorID, &item.OrderID, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
func adjustWallet(ctx context.Context, userID string, amount int, note, actor string) (walletEntry, error) {
	if amount == 0 {
		return walletEntry{}, errors.New("充值金额不能为零")
	}
	tx, err := beginSerializableTx(ctx)
	if err != nil {
		return walletEntry{}, err
	}
	defer tx.Rollback()
	var balance int
	if err = tx.QueryRowContext(ctx, `SELECT balance_fen FROM xcloud_wallets WHERE user_id=? FOR UPDATE`, userID).Scan(&balance); err != nil {
		return walletEntry{}, errors.New("用户尚未登录 xCloud")
	}
	next := balance + amount
	if next < 0 {
		return walletEntry{}, errors.New("余额不足，不能扣减")
	}
	now := time.Now()
	entryType := "manual_debit"
	if amount > 0 {
		entryType = "manual_credit"
	}
	entry := walletEntry{ID: newID("wal"), UserID: userID, AmountFen: amount, BalanceAfterFen: next, Type: entryType, Note: strings.TrimSpace(note), ActorID: actor, CreatedAt: now}
	if _, err = tx.ExecContext(ctx, `UPDATE xcloud_wallets SET balance_fen=?,updated_at=? WHERE user_id=?`, next, now, userID); err != nil {
		return walletEntry{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_wallet_entries (id,user_id,amount_fen,balance_after_fen,entry_type,note,actor_id,created_at) VALUES (?,?,?,?,?,?,?,?)`, entry.ID, entry.UserID, entry.AmountFen, entry.BalanceAfterFen, entry.Type, entry.Note, entry.ActorID, entry.CreatedAt); err != nil {
		return walletEntry{}, err
	}
	if err = tx.Commit(); err != nil {
		return walletEntry{}, err
	}
	_ = createNotification(ctx, userID, "wallet", "XCoin 余额已调整", fmt.Sprintf("本次变动 %+.2f XCoin。", float64(amount)/100), map[string]any{"entryId": entry.ID})
	return entry, nil
}

// Retrying BeginTx is safe: no wallet row has been locked or modified before a
// transaction exists. We deliberately do not blindly replay a later failure,
// because a commit outcome may be unknown and must never double-charge a user.
func beginSerializableTx(ctx context.Context) (*sql.Tx, error) {
	options := &sql.TxOptions{Isolation: sql.LevelSerializable}
	for attempt := 0; attempt < 2; attempt++ {
		tx, err := instanceDB.BeginTx(ctx, options)
		if err == nil {
			return tx, nil
		}
		if !errors.Is(err, driver.ErrBadConn) || attempt == 1 {
			return nil, err
		}
		if pingErr := instanceDB.PingContext(ctx); pingErr != nil {
			return nil, pingErr
		}
	}
	return nil, driver.ErrBadConn
}
func createNotification(ctx context.Context, userID, kind, title, body string, data any) error {
	payload, _ := json.Marshal(data)
	_, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_notifications (id,user_id,notification_type,title,body,data,created_at) VALUES (?,?,?,?,?,?,NOW())`, newID("ntf"), userID, kind, title, body, payload)
	return err
}
func notifications(ctx context.Context, userID string, limit int) ([]notification, error) {
	rows, err := instanceDB.QueryContext(ctx, `SELECT id,notification_type,title,body,COALESCE(data,JSON_OBJECT()),read_at,created_at FROM xcloud_notifications WHERE user_id=? AND notification_type NOT IN ('task','task_failed') ORDER BY created_at DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []notification{}
	for rows.Next() {
		var item notification
		if err := rows.Scan(&item.ID, &item.Type, &item.Title, &item.Body, &item.Data, &item.ReadAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
func taskEvents(ctx context.Context, id string) ([]taskEvent, error) {
	rows, err := instanceDB.QueryContext(ctx, `SELECT id,task_id,event_type,detail,created_at FROM xcloud_task_events WHERE task_id=? ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []taskEvent{}
	for rows.Next() {
		var item taskEvent
		if err := rows.Scan(&item.ID, &item.TaskID, &item.Event, &item.Detail, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
func appendTaskEvent(ctx context.Context, id, event, detail string) {
	if instanceDB == nil {
		return
	}
	_, _ = instanceDB.ExecContext(ctx, `INSERT INTO xcloud_task_events (task_id,event_type,detail,created_at) VALUES (?,?,?,NOW())`, id, event, truncateError(detail))
}
func selectNodeForPlan(ctx context.Context, tx *sql.Tx, p plan) (node, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id,name,agent_url,cpu_total,memory_total_mb,enabled,last_heartbeat_at,COALESCE(agent_capabilities,JSON_ARRAY()) FROM xcloud_nodes WHERE enabled=TRUE AND last_heartbeat_at>=? FOR UPDATE`, time.Now().Add(-nodeHeartbeatTTL()))
	if err != nil {
		return node{}, err
	}
	// A transaction owns a single MySQL connection.  Do not issue the resource
	// query below while this result set is still open; go-sql-driver/mysql then
	// rejects the connection as busy/bad.  Materialise the small healthy-node
	// list and close the rows before evaluating each candidate.
	candidates := make([]node, 0)
	for rows.Next() {
		var n node
		var capabilities []byte
		if err := rows.Scan(&n.ID, &n.Name, &n.AgentURL, &n.CPUTotal, &n.MemoryTotalMB, &n.Enabled, &n.LastHeartbeatAt, &capabilities); err != nil {
			_ = rows.Close()
			return node{}, err
		}
		_ = json.Unmarshal(capabilities, &n.AgentCapabilities)
		candidates = append(candidates, n)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return node{}, err
	}
	if err := rows.Close(); err != nil {
		return node{}, err
	}
	var best node
	bestScore := math.Inf(1)
	for _, n := range candidates {
		var cpu float64
		var memory int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(cpu),0),COALESCE(SUM(memory_mb),0) FROM xcloud_instances WHERE node_id=? AND status IN ('deploying','running','stopped','destroy_scheduled')`, n.ID).Scan(&cpu, &memory); err != nil {
			return node{}, err
		}
		if cpu+p.CPU > n.CPUTotal || memory+p.MemoryMB > n.MemoryTotalMB {
			continue
		}
		score := math.Max((cpu+p.CPU)/n.CPUTotal, float64(memory+p.MemoryMB)/float64(n.MemoryTotalMB))
		if score < bestScore {
			best, bestScore = n, score
		}
	}
	if best.ID == "" {
		if len(candidates) == 0 {
			return node{}, errors.New("暂无健康可调度节点，请等待节点心跳完成")
		}
		return node{}, errors.New("没有可用裸机节点")
	}
	return best, nil
}

func nodeByID(ctx context.Context, id string) (node, error) {
	var n node
	err := instanceDB.QueryRowContext(ctx, `SELECT id,name,agent_url,cpu_total,memory_total_mb,enabled,last_heartbeat_at,COALESCE(agent_token_ciphertext,'') FROM xcloud_nodes WHERE id=?`, id).Scan(&n.ID, &n.Name, &n.AgentURL, &n.CPUTotal, &n.MemoryTotalMB, &n.Enabled, &n.LastHeartbeatAt, &n.AgentToken)
	return n, err
}

func nodeRequest(ctx context.Context, n node, method, path string, payload any, result any) error {
	token, err := decryptNodeToken(n.AgentToken)
	if err != nil {
		return fmt.Errorf("读取节点控制令牌: %w", err)
	}
	var input io.Reader
	if payload != nil {
		raw, e := json.Marshal(payload)
		if e != nil {
			return e
		}
		input = strings.NewReader(string(raw))
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(n.AgentURL, "/")+path, input)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 3 * time.Minute}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("节点 %s 返回 %d", n.Name, resp.StatusCode)
	}
	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil && !errors.Is(err, io.EOF) {
			return err
		}
	}
	return nil
}

func purchaseWithWallet(ctx context.Context, ownerID, planID, imageID, imageVersion string, months int, promoCode string) (order, controlTask, error) {
	if !validSubscriptionMonths(months) {
		return order{}, controlTask{}, errors.New("订阅周期仅支持 1、3、6 或 12 个月")
	}
	// Ping before opening the serializable transaction. database/sql can discard
	// an idle connection here and obtain a fresh one from an external MySQL pool.
	if err := instanceDB.PingContext(ctx); err != nil {
		return order{}, controlTask{}, fmt.Errorf("数据库暂不可用: %w", err)
	}
	tx, err := beginSerializableTx(ctx)
	if err != nil {
		return order{}, controlTask{}, err
	}
	defer tx.Rollback()
	var p plan
	if err = tx.QueryRowContext(ctx, `SELECT id,name,cpu,memory_mb,bandwidth_mbps,monthly_price_fen,enabled,sort_order,created_at FROM xcloud_plans WHERE id=? AND enabled=TRUE FOR UPDATE`, planID).Scan(&p.ID, &p.Name, &p.CPU, &p.MemoryMB, &p.BandwidthMbps, &p.MonthlyFen, &p.Enabled, &p.SortOrder, &p.CreatedAt); err != nil {
		return order{}, controlTask{}, errors.New("套餐不可购买")
	}
	var img catalogImage
	if err = tx.QueryRowContext(ctx, `SELECT id,name,image_ref,COALESCE(image_digest,''),version,enabled,created_at FROM xcloud_images WHERE id=? AND enabled=TRUE FOR UPDATE`, imageID).Scan(&img.ID, &img.Name, &img.ImageRef, &img.ImageDigest, &img.Version, &img.Enabled, &img.CreatedAt); err != nil {
		return order{}, controlTask{}, errors.New("镜像版本不可购买")
	}
	imageVersion = strings.TrimSpace(imageVersion)
	if imageVersion == "" {
		imageVersion = img.Version
	}
	if !validImageTag(imageVersion) {
		return order{}, controlTask{}, errors.New("镜像版本格式无效")
	}
	var selectedDigest string
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(image_digest,'') FROM xcloud_image_versions WHERE image_id=? AND version_tag=? AND enabled=TRUE AND version_status='ready' FOR UPDATE`, imageID, imageVersion).Scan(&selectedDigest); err != nil {
		if err != sql.ErrNoRows || imageVersion != "latest" {
			return order{}, controlTask{}, errors.New("镜像版本不可购买")
		}
		// latest is the public fallback when no concrete release has been
		// configured. It intentionally has no stored digest: Docker resolves the
		// moving tag when the deployment task pulls it.
		selectedDigest = ""
	}
	var balance int
	if err = tx.QueryRowContext(ctx, `SELECT balance_fen FROM xcloud_wallets WHERE user_id=? FOR UPDATE`, ownerID).Scan(&balance); err != nil {
		return order{}, controlTask{}, errors.New("请重新登录后再购买")
	}
	quote, err := quoteCommercialBenefit(ctx, ownerID, "purchase", p.ID, "", months, p.MonthlyFen, promoCode, tx, true)
	if err != nil {
		return order{}, controlTask{}, err
	}
	amount := quote.AmountFen
	if balance < amount {
		return order{}, controlTask{}, errors.New("XCoin 余额不足")
	}
	n, err := selectNodeForPlan(ctx, tx, p)
	if err != nil {
		return order{}, controlTask{}, err
	}
	now := time.Now()
	instanceID := newID("ins")
	orderID := newID("ord")
	route := routeKey(ownerID + "\x00" + instanceID)
	container := fmt.Sprintf("xcloud-%s", strings.TrimPrefix(routeKey(instanceID), "r"))
	expires := now.AddDate(0, months, 0).AddDate(0, 0, quote.BonusDays)
	entry := walletEntry{UserID: ownerID, BalanceAfterFen: balance - amount}
	if amount > 0 {
		entry = walletEntry{ID: newID("wal"), UserID: ownerID, AmountFen: -amount, BalanceAfterFen: balance - amount, Type: "purchase", Note: "购买 " + p.Name, ActorID: ownerID, OrderID: orderID, CreatedAt: now}
		if _, err = tx.ExecContext(ctx, `UPDATE xcloud_wallets SET balance_fen=?,updated_at=? WHERE user_id=?`, entry.BalanceAfterFen, now, ownerID); err != nil {
			return order{}, controlTask{}, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_wallet_entries (id,user_id,amount_fen,balance_after_fen,entry_type,note,actor_id,order_id,created_at) VALUES (?,?,?,?,?,?,?,?,?)`, entry.ID, entry.UserID, entry.AmountFen, entry.BalanceAfterFen, entry.Type, entry.Note, entry.ActorID, entry.OrderID, now); err != nil {
			return order{}, controlTask{}, err
		}
	}
	o := order{ID: orderID, OwnerID: ownerID, PlanID: p.ID, ImageID: img.ID, InstanceID: instanceID, AmountFen: amount, ListAmountFen: quote.ListAmountFen, DiscountAmountFen: quote.DiscountAmountFen, Status: orderDeploy, ServiceStartsAt: &now, ExpiresAt: &expires, CreatedAt: &now, UpdatedAt: &now, PlanName: p.Name, ImageName: img.Name, ImageVersion: imageVersion}
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_orders (id,owner_id,plan_id,image_id,instance_id,amount_fen,list_amount_fen,discount_amount_fen,bandwidth_mbps,status,payment_source,wallet_entry_id,scheduled_node_id,selected_image_version,selected_image_digest,service_starts_at,expires_at,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, o.ID, ownerID, p.ID, img.ID, instanceID, amount, quote.ListAmountFen, quote.DiscountAmountFen, p.BandwidthMbps, orderDeploy, "wallet", nullableString(entry.ID), n.ID, imageVersion, nullableString(selectedDigest), now, expires, now, now); err != nil {
		return order{}, controlTask{}, err
	}
	if err = consumeCommercialBenefitTx(ctx, tx, ownerID, o.ID, quote); err != nil {
		return order{}, controlTask{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_instances (id,owner_id,name,image,version,spec,status,access_address,container_name,created_at,cpu,memory_mb,bandwidth_mbps,node_id,order_id,route_key,expires_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, instanceID, ownerID, img.Name, img.ImageRef, imageVersion, fmt.Sprintf("%g 核 / %d GB / 最高 %d Mbps", p.CPU, p.MemoryMB/1024, p.BandwidthMbps), "deploying", "https://xcloud-"+route+"."+env("XCLOUD_INSTANCE_DOMAIN", "alemonjs.com"), container, now, p.CPU, p.MemoryMB, p.BandwidthMbps, n.ID, o.ID, route, expires); err != nil {
		return order{}, controlTask{}, err
	}
	t := controlTask{ID: newID("task"), InstanceID: instanceID, Action: "create", IdempotencyKey: "create:" + instanceID, Status: taskPending, RunAfter: now, CreatedAt: now, UpdatedAt: now}
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_tasks (id,instance_id,action,idempotency_key,status,attempts,run_after,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?)`, t.ID, t.InstanceID, t.Action, t.IdempotencyKey, t.Status, 0, now, now, now); err != nil {
		return order{}, controlTask{}, err
	}
	if err = writeAuditTx(ctx, tx, ownerID, "purchase.create", "order", o.ID, map[string]any{"amountFen": amount, "listAmountFen": quote.ListAmountFen, "discountAmountFen": quote.DiscountAmountFen, "nodeId": n.ID, "taskId": t.ID}); err != nil {
		return order{}, controlTask{}, err
	}
	if err = tx.Commit(); err != nil {
		return order{}, controlTask{}, err
	}
	appendTaskEvent(ctx, t.ID, "queued", "钱包扣款后等待部署")
	_ = createNotification(ctx, ownerID, "purchase", "购买已提交", fmt.Sprintf("已扣除 %.2f XCoin，正在部署。", float64(amount)/100), map[string]any{"orderId": o.ID, "instanceId": instanceID, "taskId": t.ID})
	if quote.Program != nil {
		_ = createNotification(ctx, ownerID, "benefit", "权益已生效", fmt.Sprintf("%s%s", fmtBenefit(quote.program), func() string {
			if quote.DiscountAmountFen > 0 {
				return fmt.Sprintf("，已优惠 %.2f XCoin", float64(quote.DiscountAmountFen)/100)
			}
			return ""
		}()), map[string]any{"orderId": o.ID, "benefitProgramId": quote.program.ID})
	}
	return o, t, nil
}

// renewWithWallet extends one existing instance.  It deliberately does not
// reserve fresh capacity: the instance already owns its node reservation until
// it is purged.  Expired instances are restarted only after the debit commits.
func renewWithWallet(ctx context.Context, ownerID, sourceOrderID string, months int, promoCode string) (order, *controlTask, error) {
	if !validSubscriptionMonths(months) {
		return order{}, nil, errors.New("订阅周期仅支持 1、3、6 或 12 个月")
	}
	tx, err := beginSerializableTx(ctx)
	if err != nil {
		return order{}, nil, err
	}
	defer tx.Rollback()
	var source order
	var p plan
	var img catalogImage
	var instanceStatus, runtimeStatus, destroyReason string
	var currentExpiry, activeTaskExpiresAt sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT o.id,o.owner_id,o.plan_id,o.image_id,COALESCE(o.instance_id,''),o.status,p.id,p.name,p.cpu,p.memory_mb,p.monthly_price_fen,p.enabled,p.sort_order,p.created_at,i.id,i.name,i.image_ref,COALESCE(i.image_digest,''),i.version,i.enabled,i.created_at,ins.status,COALESCE(ins.runtime_status,''),COALESCE(ins.destroy_reason,''),ins.expires_at,ins.active_task_expires_at FROM xcloud_orders o JOIN xcloud_plans p ON p.id=o.plan_id JOIN xcloud_images i ON i.id=o.image_id JOIN xcloud_instances ins ON ins.id=o.instance_id WHERE o.id=? AND o.owner_id=? AND o.status IN (?,?,?) FOR UPDATE`, sourceOrderID, ownerID, orderActive, orderExpired, orderRefund).Scan(&source.ID, &source.OwnerID, &source.PlanID, &source.ImageID, &source.InstanceID, &source.Status, &p.ID, &p.Name, &p.CPU, &p.MemoryMB, &p.MonthlyFen, &p.Enabled, &p.SortOrder, &p.CreatedAt, &img.ID, &img.Name, &img.ImageRef, &img.ImageDigest, &img.Version, &img.Enabled, &img.CreatedAt, &instanceStatus, &runtimeStatus, &destroyReason, &currentExpiry, &activeTaskExpiresAt)
	if err != nil {
		return order{}, nil, errors.New("订单不可续费")
	}
	// A successful immediate plan change supersedes the original order's
	// resource plan for future renewals without rewriting that historical order.
	var changedPlanID string
	if err = tx.QueryRowContext(ctx, `SELECT target_plan_id FROM xcloud_instance_plan_changes WHERE instance_id=? AND status='succeeded' ORDER BY completed_at DESC, created_at DESC LIMIT 1`, source.InstanceID).Scan(&changedPlanID); err == nil && changedPlanID != "" && changedPlanID != source.PlanID {
		if err = tx.QueryRowContext(ctx, `SELECT id,name,cpu,memory_mb,monthly_price_fen,enabled,sort_order,created_at FROM xcloud_plans WHERE id=?`, changedPlanID).Scan(&p.ID, &p.Name, &p.CPU, &p.MemoryMB, &p.MonthlyFen, &p.Enabled, &p.SortOrder, &p.CreatedAt); err != nil {
			return order{}, nil, errors.New("实例当前套餐不可用")
		}
		source.PlanID = changedPlanID
	}
	if !p.Enabled {
		return order{}, nil, errors.New("套餐已下架，无法续费")
	}
	if activeTaskExpiresAt.Valid && activeTaskExpiresAt.Time.After(time.Now()) {
		return order{}, nil, errors.New("实例生命周期任务处理中，请稍后再续费")
	}
	// An expired instance may still have the original pending destroy task. A
	// successful renewal intentionally supersedes that task; every other
	// lifecycle task remains a hard conflict.
	allowExpiredDestroy := instanceStatus == "destroy_scheduled" && destroyReason == "expired"
	var pendingLifecycleTasks int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM xcloud_tasks WHERE instance_id=? AND status IN (?,?) AND action IN ('create','retry-deploy','start','stop','update','restart','reinstall','destroy','purge','resize') AND (? OR action<>'destroy')`, source.InstanceID, taskPending, taskRunning, allowExpiredDestroy).Scan(&pendingLifecycleTasks); err != nil {
		return order{}, nil, err
	}
	if pendingLifecycleTasks > 0 {
		return order{}, nil, errors.New("实例生命周期任务处理中，请稍后再续费")
	}
	// Renewal continues the instance's existing immutable image snapshot. A
	// later source/version delisting must not prevent a paying customer from
	// extending an already running service, nor silently replace its image.
	var renewalVersion, renewalDigest string
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(selected_image_version,?),COALESCE(selected_image_digest,'') FROM xcloud_orders WHERE id=? FOR UPDATE`, img.Version, sourceOrderID).Scan(&renewalVersion, &renewalDigest); err != nil {
		return order{}, nil, err
	}
	if instanceStatus == "destroy_scheduled" && destroyReason == "refund" {
		return order{}, nil, errors.New("退款后的实例不可续费，请在销毁后重新购买")
	}
	if instanceStatus != "running" && instanceStatus != "stopped" && !(instanceStatus == "destroy_scheduled" && destroyReason == "expired") {
		return order{}, nil, errors.New("实例当前不可续费")
	}
	var balance int
	if err = tx.QueryRowContext(ctx, `SELECT balance_fen FROM xcloud_wallets WHERE user_id=? FOR UPDATE`, ownerID).Scan(&balance); err != nil {
		return order{}, nil, errors.New("请重新登录后再续费")
	}
	quote, err := quoteCommercialBenefit(ctx, ownerID, "renewal", p.ID, source.InstanceID, months, p.MonthlyFen, promoCode, tx, true)
	if err != nil {
		return order{}, nil, err
	}
	amount := quote.AmountFen
	if balance < amount {
		return order{}, nil, errors.New("XCoin 余额不足")
	}
	now := time.Now()
	orderID := newID("ord")
	base := now
	if currentExpiry.Valid && currentExpiry.Time.After(now) {
		base = currentExpiry.Time
	}
	expires := base.AddDate(0, months, 0).AddDate(0, 0, quote.BonusDays)
	entry := walletEntry{UserID: ownerID, BalanceAfterFen: balance - amount}
	if amount > 0 {
		entry = walletEntry{ID: newID("wal"), UserID: ownerID, AmountFen: -amount, BalanceAfterFen: balance - amount, Type: "renewal", Note: "续费 " + p.Name, ActorID: ownerID, OrderID: orderID, CreatedAt: now}
		if _, err = tx.ExecContext(ctx, `UPDATE xcloud_wallets SET balance_fen=?,updated_at=? WHERE user_id=?`, entry.BalanceAfterFen, now, ownerID); err != nil {
			return order{}, nil, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_wallet_entries (id,user_id,amount_fen,balance_after_fen,entry_type,note,actor_id,order_id,created_at) VALUES (?,?,?,?,?,?,?,?,?)`, entry.ID, entry.UserID, entry.AmountFen, entry.BalanceAfterFen, entry.Type, entry.Note, entry.ActorID, entry.OrderID, now); err != nil {
			return order{}, nil, err
		}
	}
	status := orderActive
	item := order{ID: orderID, OwnerID: ownerID, PlanID: p.ID, ImageID: img.ID, InstanceID: source.InstanceID, AmountFen: amount, ListAmountFen: quote.ListAmountFen, DiscountAmountFen: quote.DiscountAmountFen, Status: status, ServiceStartsAt: &base, ExpiresAt: &expires, CreatedAt: &now, UpdatedAt: &now, PlanName: p.Name, ImageName: img.Name, ImageVersion: renewalVersion}
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_orders (id,owner_id,plan_id,image_id,instance_id,amount_fen,list_amount_fen,discount_amount_fen,status,payment_note,payment_source,wallet_entry_id,selected_image_version,selected_image_digest,service_starts_at,expires_at,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, item.ID, ownerID, p.ID, img.ID, source.InstanceID, amount, quote.ListAmountFen, quote.DiscountAmountFen, status, "续费订单："+sourceOrderID, "wallet", nullableString(entry.ID), renewalVersion, nullableString(renewalDigest), base, expires, now, now); err != nil {
		return order{}, nil, err
	}
	if err = consumeCommercialBenefitTx(ctx, tx, ownerID, item.ID, quote); err != nil {
		return order{}, nil, err
	}
	if runtimeStatus != "running" && runtimeStatus != "stopped" {
		runtimeStatus = "stopped"
	}
	wasExpiredDestroyScheduled := instanceStatus == "destroy_scheduled" && destroyReason == "expired"
	changed, err := transitionInstance(ctx, tx, source.InstanceID, []string{instanceStatus}, runtimeStatus, &runtimeStatus, "expires_at=?,purge_at=NULL,destroy_at=NULL,destroy_reason=NULL,destroyed_at=NULL", expires)
	if err != nil {
		return order{}, nil, err
	}
	if !changed {
		return order{}, nil, errInstanceStateConflict
	}
	var task *controlTask
	taskID := ""
	if wasExpiredDestroyScheduled && runtimeStatus != "running" {
		startTask := controlTask{ID: newID("task"), InstanceID: source.InstanceID, Action: "start", IdempotencyKey: "renew-start:" + source.InstanceID + ":" + orderID, Status: taskPending, RunAfter: now, CreatedAt: now, UpdatedAt: now}
		if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_tasks (id,instance_id,action,idempotency_key,status,attempts,run_after,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?)`, startTask.ID, startTask.InstanceID, startTask.Action, startTask.IdempotencyKey, startTask.Status, 0, startTask.RunAfter, startTask.CreatedAt, startTask.UpdatedAt); err != nil {
			return order{}, nil, err
		}
		task = &startTask
	}
	if task != nil {
		taskID = task.ID
	}
	if err = writeAuditTx(ctx, tx, ownerID, "purchase.renew", "order", item.ID, map[string]any{"sourceOrderId": sourceOrderID, "amountFen": amount, "taskId": taskID}); err != nil {
		return order{}, nil, err
	}
	if err = tx.Commit(); err != nil {
		return order{}, nil, err
	}
	if task != nil {
		appendTaskEvent(ctx, task.ID, "queued", "钱包扣款后等待续费恢复")
	}
	_ = createNotification(ctx, ownerID, "renewal", "续费成功", fmt.Sprintf("已扣除 %.2f XCoin，服务有效期已延长。", float64(amount)/100), map[string]any{"orderId": item.ID, "instanceId": source.InstanceID})
	if quote.Program != nil {
		_ = createNotification(ctx, ownerID, "benefit", "续费权益已生效", fmtBenefit(quote.program), map[string]any{"orderId": item.ID, "benefitProgramId": quote.program.ID})
	}
	return item, task, nil
}
func validImageTag(value string) bool {
	if len(value) == 0 || len(value) > 64 || strings.ContainsAny(value, " \t\r\n/@") {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}
