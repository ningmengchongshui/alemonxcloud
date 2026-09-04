package cloud

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
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
	if instanceDB == nil {
		return nil
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS xcloud_users (id VARCHAR(191) PRIMARY KEY, username VARCHAR(191) NOT NULL, email VARCHAR(255) NOT NULL, last_login_at DATETIME NOT NULL, created_at DATETIME NOT NULL, INDEX idx_xcloud_users_username (username)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS xcloud_wallets (user_id VARCHAR(191) PRIMARY KEY, balance_fen BIGINT NOT NULL DEFAULT 0, updated_at DATETIME NOT NULL) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS xcloud_wallet_entries (id VARCHAR(64) PRIMARY KEY, user_id VARCHAR(191) NOT NULL, amount_fen BIGINT NOT NULL, balance_after_fen BIGINT NOT NULL, entry_type VARCHAR(32) NOT NULL, note VARCHAR(255) NOT NULL, actor_id VARCHAR(191) NOT NULL, created_at DATETIME NOT NULL, INDEX idx_xcloud_wallet_entries_user (user_id, created_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS xcloud_notifications (id VARCHAR(64) PRIMARY KEY, user_id VARCHAR(191) NOT NULL, notification_type VARCHAR(48) NOT NULL, title VARCHAR(128) NOT NULL, body VARCHAR(512) NOT NULL, data JSON NULL, read_at DATETIME NULL, created_at DATETIME NOT NULL, INDEX idx_xcloud_notifications_user (user_id, read_at, created_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS xcloud_task_events (id BIGINT AUTO_INCREMENT PRIMARY KEY, task_id VARCHAR(64) NOT NULL, event_type VARCHAR(32) NOT NULL, detail VARCHAR(1024) NOT NULL DEFAULT '', created_at DATETIME NOT NULL, INDEX idx_xcloud_task_events_task (task_id, created_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`ALTER TABLE xcloud_nodes ADD COLUMN agent_token_ciphertext TEXT NULL`,
		`ALTER TABLE xcloud_nodes ADD COLUMN docker_version VARCHAR(64) NULL`,
		`ALTER TABLE xcloud_nodes ADD COLUMN disk_available_bytes BIGINT NULL`,
		`ALTER TABLE xcloud_nodes ADD COLUMN managed_container_count INT NULL`,
		`ALTER TABLE xcloud_orders ADD COLUMN payment_source VARCHAR(32) NULL`,
		`ALTER TABLE xcloud_orders ADD COLUMN wallet_entry_id VARCHAR(64) NULL`,
		`ALTER TABLE xcloud_orders ADD COLUMN scheduled_node_id VARCHAR(64) NULL`,
		`ALTER TABLE xcloud_orders ADD COLUMN selected_image_version VARCHAR(64) NULL`,
		`ALTER TABLE xcloud_images MODIFY COLUMN image_digest VARCHAR(255) NULL`,
	}
	for _, statement := range statements {
		if _, err := instanceDB.ExecContext(ctx, statement); err != nil && !isDuplicateMigration(err) {
			return err
		}
	}
	if err := normalizeImageSources(ctx); err != nil {
		return err
	}
	if _, err := instanceDB.ExecContext(ctx, `ALTER TABLE xcloud_images ADD UNIQUE KEY uq_xcloud_image_ref (image_ref)`); err != nil && !isDuplicateMigration(err) {
		return fmt.Errorf("为镜像地址创建唯一约束: %w", err)
	}
	return removeBootstrapData(ctx)
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
	rows, err := instanceDB.QueryContext(ctx, `SELECT id,user_id,amount_fen,balance_after_fen,entry_type,note,actor_id,created_at FROM xcloud_wallet_entries WHERE user_id=? ORDER BY created_at DESC LIMIT ?`, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []walletEntry{}
	for rows.Next() {
		var item walletEntry
		if err := rows.Scan(&item.ID, &item.UserID, &item.AmountFen, &item.BalanceAfterFen, &item.Type, &item.Note, &item.ActorID, &item.CreatedAt); err != nil {
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
	tx, err := instanceDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
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
	_ = createNotification(ctx, userID, "wallet", "代币余额已调整", fmt.Sprintf("本次变动 %+.2f 代币。", float64(amount)/100), map[string]any{"entryId": entry.ID})
	return entry, nil
}
func createNotification(ctx context.Context, userID, kind, title, body string, data any) error {
	payload, _ := json.Marshal(data)
	_, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_notifications (id,user_id,notification_type,title,body,data,created_at) VALUES (?,?,?,?,?,?,NOW())`, newID("ntf"), userID, kind, title, body, payload)
	return err
}
func notifications(ctx context.Context, userID string, limit int) ([]notification, error) {
	rows, err := instanceDB.QueryContext(ctx, `SELECT id,notification_type,title,body,COALESCE(data,JSON_OBJECT()),read_at,created_at FROM xcloud_notifications WHERE user_id=? ORDER BY created_at DESC LIMIT ?`, userID, limit)
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
	rows, err := tx.QueryContext(ctx, `SELECT id,name,agent_url,cpu_total,memory_total_mb,enabled,last_heartbeat_at FROM xcloud_nodes WHERE enabled=TRUE AND last_heartbeat_at>=? FOR UPDATE`, time.Now().Add(-nodeHeartbeatTTL()))
	if err != nil {
		return node{}, err
	}
	defer rows.Close()
	var best node
	bestScore := math.Inf(1)
	healthy := 0
	for rows.Next() {
		var n node
		if err := rows.Scan(&n.ID, &n.Name, &n.AgentURL, &n.CPUTotal, &n.MemoryTotalMB, &n.Enabled, &n.LastHeartbeatAt); err != nil {
			return node{}, err
		}
		healthy++
		var cpu float64
		var memory int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(cpu),0),COALESCE(SUM(memory_mb),0) FROM xcloud_instances WHERE node_id=? AND status IN ('deploying','running','stopped','expired','retention')`, n.ID).Scan(&cpu, &memory); err != nil {
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
		if healthy == 0 {
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
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}

func purchaseWithWallet(ctx context.Context, ownerID, planID, imageID, imageVersion string, months int) (order, controlTask, error) {
	if months < 1 || months > 24 {
		return order{}, controlTask{}, errors.New("订阅周期应为 1 至 24 个月")
	}
	// Ping before opening the serializable transaction. database/sql can discard
	// an idle connection here and obtain a fresh one from an external MySQL pool.
	if err := instanceDB.PingContext(ctx); err != nil {
		return order{}, controlTask{}, fmt.Errorf("数据库暂不可用: %w", err)
	}
	tx, err := instanceDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return order{}, controlTask{}, err
	}
	defer tx.Rollback()
	var p plan
	if err = tx.QueryRowContext(ctx, `SELECT id,name,cpu,memory_mb,monthly_price_fen,enabled,sort_order,created_at FROM xcloud_plans WHERE id=? AND enabled=TRUE FOR UPDATE`, planID).Scan(&p.ID, &p.Name, &p.CPU, &p.MemoryMB, &p.MonthlyFen, &p.Enabled, &p.SortOrder, &p.CreatedAt); err != nil {
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
	var balance int
	if err = tx.QueryRowContext(ctx, `SELECT balance_fen FROM xcloud_wallets WHERE user_id=? FOR UPDATE`, ownerID).Scan(&balance); err != nil {
		return order{}, controlTask{}, errors.New("请重新登录后再购买")
	}
	amount := p.MonthlyFen * months
	if balance < amount {
		return order{}, controlTask{}, errors.New("代币余额不足")
	}
	n, err := selectNodeForPlan(ctx, tx, p)
	if err != nil {
		return order{}, controlTask{}, err
	}
	now := time.Now()
	instanceID := newID("ins")
	route := routeKey(ownerID + "\x00" + instanceID)
	container := fmt.Sprintf("xcloud-%s", strings.TrimPrefix(routeKey(instanceID), "r"))
	expires := now.AddDate(0, months, 0)
	entry := walletEntry{ID: newID("wal"), UserID: ownerID, AmountFen: -amount, BalanceAfterFen: balance - amount, Type: "purchase", Note: "购买 " + p.Name, ActorID: ownerID, CreatedAt: now}
	if _, err = tx.ExecContext(ctx, `UPDATE xcloud_wallets SET balance_fen=?,updated_at=? WHERE user_id=?`, entry.BalanceAfterFen, now, ownerID); err != nil {
		return order{}, controlTask{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_wallet_entries (id,user_id,amount_fen,balance_after_fen,entry_type,note,actor_id,created_at) VALUES (?,?,?,?,?,?,?,?)`, entry.ID, entry.UserID, entry.AmountFen, entry.BalanceAfterFen, entry.Type, entry.Note, entry.ActorID, now); err != nil {
		return order{}, controlTask{}, err
	}
	o := order{ID: newID("ord"), OwnerID: ownerID, PlanID: p.ID, ImageID: img.ID, InstanceID: instanceID, AmountFen: amount, Status: orderDeploy, ExpiresAt: &expires, CreatedAt: &now, UpdatedAt: &now, PlanName: p.Name, ImageName: img.Name, ImageVersion: imageVersion}
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_orders (id,owner_id,plan_id,image_id,instance_id,amount_fen,status,payment_source,wallet_entry_id,scheduled_node_id,selected_image_version,expires_at,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, o.ID, ownerID, p.ID, img.ID, instanceID, amount, orderDeploy, "wallet", entry.ID, n.ID, imageVersion, expires, now, now); err != nil {
		return order{}, controlTask{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_instances (id,owner_id,name,image,version,spec,status,access_address,container_name,created_at,cpu,memory_mb,node_id,order_id,route_key,expires_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, instanceID, ownerID, img.Name, img.ImageRef, imageVersion, fmt.Sprintf("%g 核 / %d GB", p.CPU, p.MemoryMB/1024), "deploying", "https://xcloud-"+route+"."+env("XCLOUD_INSTANCE_DOMAIN", "alemonjs.com"), container, now, p.CPU, p.MemoryMB, n.ID, o.ID, route, expires); err != nil {
		return order{}, controlTask{}, err
	}
	t := controlTask{ID: newID("task"), InstanceID: instanceID, Action: "create", IdempotencyKey: "create:" + instanceID, Status: taskPending, RunAfter: now, CreatedAt: now, UpdatedAt: now}
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_tasks (id,instance_id,action,idempotency_key,status,attempts,run_after,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?)`, t.ID, t.InstanceID, t.Action, t.IdempotencyKey, t.Status, 0, now, now, now); err != nil {
		return order{}, controlTask{}, err
	}
	if err = writeAuditTx(ctx, tx, ownerID, "purchase.create", "order", o.ID, map[string]any{"amountFen": amount, "nodeId": n.ID, "taskId": t.ID}); err != nil {
		return order{}, controlTask{}, err
	}
	if err = tx.Commit(); err != nil {
		return order{}, controlTask{}, err
	}
	appendTaskEvent(ctx, t.ID, "queued", "钱包扣款后等待部署")
	_ = createNotification(ctx, ownerID, "purchase", "购买已提交", fmt.Sprintf("已扣除 %.2f 代币，正在部署。", float64(amount)/100), map[string]any{"orderId": o.ID, "instanceId": instanceID, "taskId": t.ID})
	return o, t, nil
}

// renewWithWallet extends one existing instance.  It deliberately does not
// reserve fresh capacity: the instance already owns its node reservation until
// it is purged.  Expired instances are restarted only after the debit commits.
func renewWithWallet(ctx context.Context, ownerID, sourceOrderID string, months int) (order, *controlTask, error) {
	if months < 1 || months > 24 {
		return order{}, nil, errors.New("订阅周期应为 1 至 24 个月")
	}
	tx, err := instanceDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return order{}, nil, err
	}
	defer tx.Rollback()
	var source order
	var p plan
	var img catalogImage
	var instanceStatus string
	var currentExpiry sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT o.id,o.owner_id,o.plan_id,o.image_id,COALESCE(o.instance_id,''),o.status,p.id,p.name,p.cpu,p.memory_mb,p.monthly_price_fen,p.enabled,p.sort_order,p.created_at,i.id,i.name,i.image_ref,COALESCE(i.image_digest,''),i.version,i.enabled,i.created_at,ins.status,ins.expires_at FROM xcloud_orders o JOIN xcloud_plans p ON p.id=o.plan_id JOIN xcloud_images i ON i.id=o.image_id JOIN xcloud_instances ins ON ins.id=o.instance_id WHERE o.id=? AND o.owner_id=? AND o.status IN (?,?) FOR UPDATE`, sourceOrderID, ownerID, orderActive, orderExpired).Scan(&source.ID, &source.OwnerID, &source.PlanID, &source.ImageID, &source.InstanceID, &source.Status, &p.ID, &p.Name, &p.CPU, &p.MemoryMB, &p.MonthlyFen, &p.Enabled, &p.SortOrder, &p.CreatedAt, &img.ID, &img.Name, &img.ImageRef, &img.ImageDigest, &img.Version, &img.Enabled, &img.CreatedAt, &instanceStatus, &currentExpiry)
	if err != nil {
		return order{}, nil, errors.New("订单不可续费")
	}
	if !p.Enabled || !img.Enabled {
		return order{}, nil, errors.New("套餐或镜像版本已下架，无法续费")
	}
	if instanceStatus != "running" && instanceStatus != "stopped" && instanceStatus != "expired" {
		return order{}, nil, errors.New("实例当前不可续费")
	}
	var balance int
	if err = tx.QueryRowContext(ctx, `SELECT balance_fen FROM xcloud_wallets WHERE user_id=? FOR UPDATE`, ownerID).Scan(&balance); err != nil {
		return order{}, nil, errors.New("请重新登录后再续费")
	}
	amount := p.MonthlyFen * months
	if balance < amount {
		return order{}, nil, errors.New("代币余额不足")
	}
	now := time.Now()
	base := now
	if currentExpiry.Valid && currentExpiry.Time.After(now) {
		base = currentExpiry.Time
	}
	expires := base.AddDate(0, months, 0)
	entry := walletEntry{ID: newID("wal"), UserID: ownerID, AmountFen: -amount, BalanceAfterFen: balance - amount, Type: "renewal", Note: "续费 " + p.Name, ActorID: ownerID, CreatedAt: now}
	if _, err = tx.ExecContext(ctx, `UPDATE xcloud_wallets SET balance_fen=?,updated_at=? WHERE user_id=?`, entry.BalanceAfterFen, now, ownerID); err != nil {
		return order{}, nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_wallet_entries (id,user_id,amount_fen,balance_after_fen,entry_type,note,actor_id,created_at) VALUES (?,?,?,?,?,?,?,?)`, entry.ID, entry.UserID, entry.AmountFen, entry.BalanceAfterFen, entry.Type, entry.Note, entry.ActorID, now); err != nil {
		return order{}, nil, err
	}
	status := orderActive
	if instanceStatus == "expired" {
		status = orderDeploy
	}
	item := order{ID: newID("ord"), OwnerID: ownerID, PlanID: p.ID, ImageID: img.ID, InstanceID: source.InstanceID, AmountFen: amount, Status: status, ExpiresAt: &expires, CreatedAt: &now, UpdatedAt: &now, PlanName: p.Name, ImageName: img.Name, ImageVersion: img.Version}
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_orders (id,owner_id,plan_id,image_id,instance_id,amount_fen,status,payment_note,payment_source,wallet_entry_id,selected_image_version,expires_at,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, item.ID, ownerID, p.ID, img.ID, source.InstanceID, amount, status, "续费订单："+sourceOrderID, "wallet", entry.ID, img.Version, expires, now, now); err != nil {
		return order{}, nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE xcloud_instances SET expires_at=?,purge_at=NULL,status=CASE WHEN status='expired' THEN 'deploying' ELSE status END WHERE id=?`, expires, source.InstanceID); err != nil {
		return order{}, nil, err
	}
	var task *controlTask
	if instanceStatus == "expired" {
		value := controlTask{ID: newID("task"), InstanceID: source.InstanceID, Action: "start", IdempotencyKey: "renew:" + item.ID, Status: taskPending, RunAfter: now, CreatedAt: now, UpdatedAt: now}
		if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_tasks (id,instance_id,action,idempotency_key,status,attempts,run_after,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?)`, value.ID, value.InstanceID, value.Action, value.IdempotencyKey, value.Status, 0, now, now, now); err != nil {
			return order{}, nil, err
		}
		task = &value
	}
	taskID := ""
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
	_ = createNotification(ctx, ownerID, "renewal", "续费成功", fmt.Sprintf("已扣除 %.2f 代币，服务有效期已延长。", float64(amount)/100), map[string]any{"orderId": item.ID, "instanceId": source.InstanceID})
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
