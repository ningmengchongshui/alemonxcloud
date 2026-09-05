package cloud

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func catalog(c *gin.Context) {
	images, plans, err := listCatalog(c.Request.Context(), false)
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"images": images, "plans": plans})
}

// manualPaymentDisabled retires the manual-transfer workflow. Purchases must
// be paid from wallet balance and are atomically scheduled only after capacity
// has been reserved on a healthy node.
func manualPaymentDisabled(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{"message": "人工付款订单已停用，请充值后使用钱包直接购买"})
}
func myOrders(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	items, err := listOrders(c.Request.Context(), user.ID)
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}
func createOrderHandler(c *gin.Context) {
	var body struct {
		PlanID      string `json:"planId" binding:"required"`
		ImageID     string `json:"imageId" binding:"required"`
		Months      int    `json:"months"`
		PaymentNote string `json:"paymentNote"`
	}
	if c.ShouldBindJSON(&body) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "订单参数无效"})
		return
	}
	user := c.MustGet("user").(oidcUser)
	item, err := createOrder(c.Request.Context(), user.ID, body.PlanID, body.ImageID, body.Months, body.PaymentNote)
	if err != nil {
		businessError(c, err)
		return
	}
	_ = writeAudit(c.Request.Context(), user.ID, "order.create", "order", item.ID, map[string]any{"amountFen": item.AmountFen})
	c.JSON(http.StatusCreated, item)
}
func purchaseHandler(c *gin.Context) {
	var body struct {
		PlanID       string `json:"planId" binding:"required"`
		ImageID      string `json:"imageId" binding:"required"`
		ImageVersion string `json:"imageVersion"`
		Months       int    `json:"months"`
		CouponCode   string `json:"couponCode"`
		PromotionID  string `json:"promotionId"`
	}
	if c.ShouldBindJSON(&body) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "购买参数无效"})
		return
	}
	user := c.MustGet("user").(oidcUser)
	item, task, err := purchaseWithWallet(c.Request.Context(), user.ID, body.PlanID, body.ImageID, body.ImageVersion, body.Months, body.CouponCode, body.PromotionID)
	if err != nil {
		businessError(c, err)
		return
	}
	if err := enqueuePersistedTask(c.Request.Context(), task); err != nil {
		c.JSON(http.StatusAccepted, gin.H{"order": item, "task": task, "message": "购买已记录，等待任务队列恢复"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"order": item, "task": task})
}
func walletHandler(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	item, err := walletForUser(c.Request.Context(), user.ID)
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}
func walletEntriesHandler(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	items, err := walletEntries(c.Request.Context(), user.ID, 100)
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}
func notificationsHandler(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	items, err := notifications(c.Request.Context(), user.ID, 100)
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}
func readNotificationHandler(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	result, err := instanceDB.ExecContext(c.Request.Context(), `UPDATE xcloud_notifications SET read_at=NOW() WHERE id=? AND user_id=? AND read_at IS NULL`, c.Param("id"), user.ID)
	if err != nil {
		internalError(c, err)
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		c.Status(http.StatusNoContent)
		return
	}
	c.Status(http.StatusNoContent)
}
func readAllNotificationsHandler(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	if _, err := instanceDB.ExecContext(c.Request.Context(), `UPDATE xcloud_notifications SET read_at=NOW() WHERE user_id=? AND read_at IS NULL`, user.ID); err != nil {
		internalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
func instanceTasksHandler(c *gin.Context) {
	if _, ok := ownedInstance(c); !ok {
		return
	}
	rows, err := instanceDB.QueryContext(c.Request.Context(), `SELECT id,instance_id,action,idempotency_key,status,attempts,COALESCE(last_error,''),run_after,created_at,updated_at FROM xcloud_tasks WHERE instance_id=? ORDER BY created_at DESC LIMIT 50`, c.Param("id"))
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()
	items := []gin.H{}
	for rows.Next() {
		var t controlTask
		if err := rows.Scan(&t.ID, &t.InstanceID, &t.Action, &t.IdempotencyKey, &t.Status, &t.Attempts, &t.LastError, &t.RunAfter, &t.CreatedAt, &t.UpdatedAt); err != nil {
			internalError(c, err)
			return
		}
		events, _ := taskEvents(c.Request.Context(), t.ID)
		items = append(items, gin.H{"task": t, "events": events})
	}
	c.JSON(http.StatusOK, items)
}
func cancelOrderHandler(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	if err := cancelOrder(c.Request.Context(), c.Param("id"), user.ID); err != nil {
		businessError(c, err)
		return
	}
	_ = writeAudit(c.Request.Context(), user.ID, "order.cancel", "order", c.Param("id"), nil)
	c.Status(http.StatusNoContent)
}
func submitPaymentHandler(c *gin.Context) {
	var body struct {
		Reference string `json:"reference" binding:"required"`
	}
	if c.ShouldBindJSON(&body) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "付款流水号无效"})
		return
	}
	user := c.MustGet("user").(oidcUser)
	if err := submitPayment(c.Request.Context(), c.Param("id"), user.ID, body.Reference); err != nil {
		businessError(c, err)
		return
	}
	_ = writeAudit(c.Request.Context(), user.ID, "payment.submit", "order", c.Param("id"), map[string]any{"reference": body.Reference})
	c.Status(http.StatusAccepted)
}
func renewOrderHandler(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	var body struct {
		Months      int    `json:"months"`
		CouponCode  string `json:"couponCode"`
		PromotionID string `json:"promotionId"`
	}
	if c.ShouldBindJSON(&body) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "续费参数无效"})
		return
	}
	var planID, imageID, instanceID string
	err := instanceDB.QueryRowContext(c.Request.Context(), `SELECT plan_id,image_id,instance_id FROM xcloud_orders WHERE id=? AND owner_id=? AND status IN (?,?,?)`, c.Param("id"), user.ID, orderActive, orderExpired, orderRefund).Scan(&planID, &imageID, &instanceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "订单不可续费"})
		return
	}
	item, err := createOrder(c.Request.Context(), user.ID, planID, imageID, body.Months, "续费订单："+c.Param("id"))
	if err != nil {
		businessError(c, err)
		return
	}
	if _, err := instanceDB.ExecContext(c.Request.Context(), `UPDATE xcloud_orders SET renewal_instance_id=? WHERE id=?`, instanceID, item.ID); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, item)
}

// renewWithWalletHandler renews an existing service without reviving the
// retired manual-payment flow.  The debit, ledger entry, renewal order and
// (when necessary) restart task are committed as one transaction.
func renewWithWalletHandler(c *gin.Context) {
	var body struct {
		Months      int    `json:"months"`
		CouponCode  string `json:"couponCode"`
		PromotionID string `json:"promotionId"`
	}
	if c.ShouldBindJSON(&body) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "续费参数无效"})
		return
	}
	user := c.MustGet("user").(oidcUser)
	item, task, err := renewWithWallet(c.Request.Context(), user.ID, c.Param("id"), body.Months, body.CouponCode, body.PromotionID)
	if err != nil {
		businessError(c, err)
		return
	}
	if task != nil {
		if err := enqueuePersistedTask(c.Request.Context(), *task); err != nil {
			c.JSON(http.StatusAccepted, gin.H{"order": item, "task": task, "message": "续费已记录，等待任务队列恢复"})
			return
		}
	}
	c.JSON(http.StatusAccepted, gin.H{"order": item, "task": task})
}

func refundQuoteHandler(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	quote, err := refundQuoteForOrder(c.Request.Context(), user.ID, c.Param("id"))
	if err != nil {
		businessError(c, err)
		return
	}
	c.JSON(http.StatusOK, quote)
}

func refundOrderHandler(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	quote, entry, err := refundOrder(c.Request.Context(), user.ID, c.Param("id"))
	if err != nil {
		businessError(c, err)
		return
	}
	wallet, err := walletForUser(c.Request.Context(), user.ID)
	if err != nil {
		internalError(c, err)
		return
	}
	items, err := listOrders(c.Request.Context(), user.ID)
	if err != nil {
		internalError(c, err)
		return
	}
	for _, item := range items {
		if item.ID == c.Param("id") {
			c.JSON(http.StatusOK, gin.H{"order": item, "quote": quote, "entry": entry, "wallet": wallet})
			return
		}
	}
	internalError(c, errors.New("退款订单读取失败"))
}

func adminCatalog(c *gin.Context) {
	images, plans, err := listCatalog(c.Request.Context(), true)
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"images": images, "plans": plans})
}
func adminSaveImage(c *gin.Context) {
	var body catalogImage
	if c.ShouldBindJSON(&body) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "镜像版本参数无效"})
		return
	}
	if id := c.Param("id"); id != "" {
		body.ID = id
	}
	if strings.TrimSpace(body.Name) == "" || strings.TrimSpace(body.ImageRef) == "" || strings.TrimSpace(body.Version) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "镜像来源参数无效"})
		return
	}
	if err := saveImage(c.Request.Context(), body); err != nil {
		businessError(c, err)
		return
	}
	user := c.MustGet("user").(oidcUser)
	_ = writeAudit(c.Request.Context(), user.ID, "catalog.image.save", "image", body.ID, map[string]any{"imageRef": body.ImageRef, "version": body.Version})
	c.JSON(http.StatusOK, body)
}
func adminSaveImageVersion(c *gin.Context) {
	var body imageVersion
	if c.ShouldBindJSON(&body) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "镜像版本参数无效"})
		return
	}
	body.ImageID = c.Param("id")
	if versionID := c.Param("versionID"); versionID != "" {
		body.ID = versionID
	}
	item, err := saveImageVersion(c.Request.Context(), body)
	if err != nil {
		businessError(c, err)
		return
	}
	user := c.MustGet("user").(oidcUser)
	_ = writeAudit(c.Request.Context(), user.ID, "catalog.image_version.save", "image_version", item.ID, map[string]any{"imageId": item.ImageID, "tag": item.Tag})
	c.JSON(http.StatusOK, item)
}
func adminPullImageVersion(c *gin.Context) {
	versionID := c.Param("versionID")
	var imageRef, tag, digest string
	err := instanceDB.QueryRowContext(c.Request.Context(), `SELECT i.image_ref,v.version_tag,COALESCE(v.image_digest,'') FROM xcloud_image_versions v JOIN xcloud_images i ON i.id=v.image_id WHERE v.id=? AND v.image_id=?`, versionID, c.Param("id")).Scan(&imageRef, &tag, &digest)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "镜像版本不存在"})
		return
	}
	// Always resolve from the tag during publication. A stored digest is a
	// previous immutable snapshot, not a substitute for a fresh verification.
	image := deploymentImage(imageRef, tag, "")
	if _, err := instanceDB.ExecContext(c.Request.Context(), `UPDATE xcloud_image_versions SET enabled=FALSE,version_status='syncing',last_error=NULL,updated_at=NOW() WHERE id=?`, versionID); err != nil {
		internalError(c, err)
		return
	}
	user := c.MustGet("user").(oidcUser)
	_ = writeAudit(c.Request.Context(), user.ID, "catalog.image_version.pull", "image_version", versionID, map[string]any{"image": image})
	go pullImageOnNodes(context.Background(), versionID, image)
	c.JSON(http.StatusAccepted, gin.H{"message": "已提交节点同步与发布校验", "image": image})
}
func pullImageOnNodes(ctx context.Context, versionID, image string) {
	nodes, err := enabledNodes(ctx)
	if err != nil {
		markImageVersionFailed(ctx, versionID, "无法读取健康节点")
		return
	}
	if len(nodes) == 0 {
		markImageVersionFailed(ctx, versionID, "没有启用节点，无法验证镜像版本")
		return
	}
	var expectedDigest string
	var failure string
	for _, n := range nodes {
		if !n.supportsAgentCapability("image.pull.v1") {
			_, _ = instanceDB.ExecContext(ctx, `INSERT INTO xcloud_image_version_pulls (image_version_id,node_id,status,last_error,updated_at) VALUES (?,?,?,?,NOW()) ON DUPLICATE KEY UPDATE status=VALUES(status),last_error=VALUES(last_error),updated_at=NOW()`, versionID, n.ID, "unsupported", "Agent 尚未声明 image.pull.v1；请先完成 Agent 升级")
			failure = "存在不支持镜像拉取的 Agent 节点"
			continue
		}
		_, _ = instanceDB.ExecContext(ctx, `INSERT INTO xcloud_image_version_pulls (image_version_id,node_id,status,updated_at) VALUES (?,?,?,NOW()) ON DUPLICATE KEY UPDATE status=VALUES(status),last_error=NULL,updated_at=NOW()`, versionID, n.ID, "pulling")
		var result struct {
			ImageID     string   `json:"imageID"`
			RepoDigests []string `json:"repoDigests"`
		}
		probe, cancel := context.WithTimeout(ctx, 3*time.Minute)
		err := nodeRequest(probe, n, http.MethodPost, "/container/pull", map[string]any{"image": image}, &result)
		cancel()
		if err != nil {
			_, _ = instanceDB.ExecContext(ctx, `UPDATE xcloud_image_version_pulls SET status='failed',last_error=?,updated_at=NOW() WHERE image_version_id=? AND node_id=?`, truncateError(err.Error()), versionID, n.ID)
			failure = "部分节点拉取镜像失败"
			continue
		}
		digest := immutableDigest(result.RepoDigests)
		if digest == "" {
			_, _ = instanceDB.ExecContext(ctx, `UPDATE xcloud_image_version_pulls SET status='failed',last_error=?,updated_at=NOW() WHERE image_version_id=? AND node_id=?`, "Agent 未返回可验证的 RepoDigest", versionID, n.ID)
			failure = "部分节点未返回可验证镜像摘要"
			continue
		}
		if expectedDigest == "" {
			expectedDigest = digest
		} else if expectedDigest != digest {
			_, _ = instanceDB.ExecContext(ctx, `UPDATE xcloud_image_version_pulls SET status='failed',resolved_digest=?,local_image_id=?,last_error=?,updated_at=NOW() WHERE image_version_id=? AND node_id=?`, digest, result.ImageID, "节点镜像摘要与其他节点不一致", versionID, n.ID)
			failure = "节点镜像摘要不一致"
			continue
		}
		_, _ = instanceDB.ExecContext(ctx, `UPDATE xcloud_image_version_pulls SET status='succeeded',resolved_digest=?,local_image_id=?,last_error=NULL,pulled_at=NOW(),updated_at=NOW() WHERE image_version_id=? AND node_id=?`, digest, result.ImageID, versionID, n.ID)
	}
	if failure != "" || expectedDigest == "" {
		markImageVersionFailed(ctx, versionID, failure)
		return
	}
	_, _ = instanceDB.ExecContext(ctx, `UPDATE xcloud_image_versions SET image_digest=?,enabled=TRUE,version_status='ready',last_error=NULL,published_at=NOW(),updated_at=NOW() WHERE id=?`, expectedDigest, versionID)
}

func immutableDigest(repoDigests []string) string {
	for _, value := range repoDigests {
		if _, digest, ok := strings.Cut(strings.TrimSpace(value), "@"); ok && validImageDigest(digest) {
			return strings.ToLower(digest)
		}
	}
	return ""
}

func markImageVersionFailed(ctx context.Context, versionID, reason string) {
	if strings.TrimSpace(reason) == "" {
		reason = "镜像发布校验失败"
	}
	_, _ = instanceDB.ExecContext(ctx, `UPDATE xcloud_image_versions SET enabled=FALSE,version_status='failed',last_error=?,updated_at=NOW() WHERE id=?`, truncateError(reason), versionID)
}
func adminSavePlan(c *gin.Context) {
	var body plan
	if c.ShouldBindJSON(&body) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "套餐参数无效"})
		return
	}
	if id := c.Param("id"); id != "" {
		body.ID = id
	}
	if strings.TrimSpace(body.Name) == "" || body.CPU <= 0 || body.MemoryMB < 256 || body.MonthlyFen < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "套餐参数无效"})
		return
	}
	if err := savePlan(c.Request.Context(), body); err != nil {
		businessError(c, err)
		return
	}
	user := c.MustGet("user").(oidcUser)
	_ = writeAudit(c.Request.Context(), user.ID, "catalog.plan.save", "plan", body.ID, map[string]any{"cpu": body.CPU, "memoryMB": body.MemoryMB, "monthlyPriceFen": body.MonthlyFen})
	c.JSON(http.StatusOK, body)
}
func adminOrders(c *gin.Context) {
	items, err := listAllOrders(c.Request.Context())
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}
func adminConfirmOrder(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	task, err := confirmOrder(c.Request.Context(), c.Param("id"), user.ID)
	if err != nil {
		businessError(c, err)
		return
	}
	if err := enqueuePersistedTask(c.Request.Context(), task); err != nil { // task remains durable and the recovery dispatcher will publish it.
		c.JSON(http.StatusAccepted, gin.H{"task": task, "message": "已确认收款，任务等待队列恢复"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"task": task})
}
func adminRejectOrder(c *gin.Context) {
	var body struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&body)
	user := c.MustGet("user").(oidcUser)
	if err := rejectOrder(c.Request.Context(), c.Param("id"), user.ID, body.Reason); err != nil {
		businessError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
func adminNodes(c *gin.Context) {
	items, err := listNodesWithUsage(c.Request.Context())
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}
func adminUsers(c *gin.Context) {
	q := "%" + strings.TrimSpace(c.Query("q")) + "%"
	rows, err := instanceDB.QueryContext(c.Request.Context(), `SELECT u.id,u.username,u.email,u.last_login_at,w.balance_fen FROM xcloud_users u JOIN xcloud_wallets w ON w.user_id=u.id WHERE u.id LIKE ? OR u.username LIKE ? OR u.email LIKE ? ORDER BY u.last_login_at DESC LIMIT 100`, q, q, q)
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()
	items := []cloudUser{}
	for rows.Next() {
		var x cloudUser
		if err := rows.Scan(&x.ID, &x.Username, &x.Email, &x.LastLoginAt, &x.BalanceFen); err != nil {
			internalError(c, err)
			return
		}
		items = append(items, x)
	}
	c.JSON(http.StatusOK, items)
}
func adminWalletEntries(c *gin.Context) {
	items, err := walletEntries(c.Request.Context(), c.Param("id"), 200)
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}
func adminAdjustWallet(c *gin.Context) {
	var body struct {
		AmountFen int    `json:"amountFen"`
		Direction string `json:"direction"`
		Note      string `json:"note"`
	}
	if c.ShouldBindJSON(&body) != nil || body.AmountFen <= 0 || (body.Direction != "increase" && body.Direction != "decrease") || strings.TrimSpace(body.Note) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "操作类型、正数金额和运营备注不能为空"})
		return
	}
	if body.Direction == "decrease" {
		body.AmountFen = -body.AmountFen
	}
	user := c.MustGet("user").(oidcUser)
	entry, err := adjustWallet(c.Request.Context(), c.Param("id"), body.AmountFen, body.Note, user.ID)
	if err != nil {
		businessError(c, err)
		return
	}
	_ = writeAudit(c.Request.Context(), user.ID, "wallet.adjust", "user", entry.UserID, map[string]any{"entryId": entry.ID, "amountFen": entry.AmountFen, "note": entry.Note})
	c.JSON(http.StatusOK, entry)
}
func adminSaveNode(c *gin.Context) {
	var body node
	if c.ShouldBindJSON(&body) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "节点参数无效"})
		return
	}
	if id := c.Param("id"); id != "" {
		body.ID = id
	}
	if strings.TrimSpace(body.AgentToken) != "" {
		encrypted, err := encryptNodeToken(strings.TrimSpace(body.AgentToken))
		if err != nil {
			businessError(c, err)
			return
		}
		probe := body
		probe.AgentToken = encrypted
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		var status struct {
			CPUTotal      float64  `json:"cpuTotal"`
			MemoryTotalMB int      `json:"memoryTotalMB"`
			AgentVersion  string   `json:"agentVersion"`
			APIVersion    int      `json:"apiVersion"`
			Capabilities  []string `json:"capabilities"`
		}
		err = nodeRequest(ctx, probe, http.MethodGet, "/container/status", nil, &status)
		cancel()
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"message": "Agent 验证失败，节点未保存"})
			return
		}
		if status.CPUTotal <= 0 || status.MemoryTotalMB < 256 {
			c.JSON(http.StatusBadGateway, gin.H{"message": "Agent 未返回有效硬件信息，节点未保存"})
			return
		}
		body.CPUDetected = status.CPUTotal
		body.MemoryDetectedMB = status.MemoryTotalMB
		body.AgentVersion = status.AgentVersion
		body.AgentAPIVersion = status.APIVersion
		body.AgentCapabilities = status.Capabilities
		body.AgentCompatibility = body.compatibility()
		body.CPUTotal = 0
		body.MemoryTotalMB = 0
		body.Enabled = false
	}
	if err := saveNode(c.Request.Context(), body); err != nil {
		businessError(c, err)
		return
	}
	user := c.MustGet("user").(oidcUser)
	_ = writeAudit(c.Request.Context(), user.ID, "node.save", "node", body.ID, map[string]any{"enabled": body.Enabled, "agentVersion": body.AgentVersion, "agentApiVersion": body.AgentAPIVersion})
	body.AgentToken = ""
	c.JSON(http.StatusOK, body)
}
func adminTasks(c *gin.Context) {
	items, err := listTasks(c.Request.Context(), 100)
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}
func adminAuditLogs(c *gin.Context) {
	items, err := listAuditLogs(c.Request.Context(), 200)
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}
func adminMetricsHandler(c *gin.Context) {
	value, err := adminMetrics(c.Request.Context())
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, value)
}
func retryTask(c *gin.Context) {
	result, err := instanceDB.ExecContext(c.Request.Context(), `UPDATE xcloud_tasks SET status=?,run_after=NOW(),last_error=NULL,updated_at=NOW() WHERE id=? AND status=?`, taskPending, c.Param("id"), taskFailed)
	if err != nil {
		internalError(c, err)
		return
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		c.JSON(http.StatusConflict, gin.H{"message": "任务当前不可重试"})
		return
	}
	appendTaskEvent(c.Request.Context(), c.Param("id"), "manual_retry", "管理员手动重试")
	user := c.MustGet("user").(oidcUser)
	_ = writeAudit(c.Request.Context(), user.ID, "task.retry", "task", c.Param("id"), nil)
	task, err := loadTask(c.Request.Context(), c.Param("id"))
	if err == nil {
		_ = enqueuePersistedTask(c.Request.Context(), task)
	}
	c.Status(http.StatusAccepted)
}

func queueInstanceAction(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	item, ok := ownedInstance(c)
	if !ok {
		return
	}
	action := c.Param("action")
	if action != "start" && action != "stop" && action != "restart" && action != "delete" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "不支持的实例操作"})
		return
	}
	task, err := scheduleInstanceTask(c.Request.Context(), item.ID, action, user.ID)
	if err != nil {
		businessError(c, err)
		return
	}
	if err := enqueuePersistedTask(c.Request.Context(), task); err != nil {
		c.JSON(http.StatusAccepted, gin.H{"task": task, "message": "任务已记录，等待队列恢复"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"task": task})
}
func queueDeleteInstance(c *gin.Context) {
	c.Params = append(c.Params, gin.Param{Key: "action", Value: "delete"})
	queueInstanceAction(c)
}
func instanceLogs(c *gin.Context) {
	item, ok := ownedInstance(c)
	if !ok {
		return
	}
	var nodeID string
	if err := instanceDB.QueryRowContext(c.Request.Context(), `SELECT COALESCE(node_id,'') FROM xcloud_instances WHERE id=?`, item.ID).Scan(&nodeID); err != nil {
		internalError(c, err)
		return
	}
	n, err := nodeByID(c.Request.Context(), nodeID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"message": "实例节点不可用"})
		return
	}
	var body struct {
		Lines []string `json:"lines"`
	}
	if err := nodeRequest(c.Request.Context(), n, http.MethodGet, "/container/"+item.ContainerName+"/logs", nil, &body); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"message": "读取实例日志失败"})
		return
	}
	c.JSON(http.StatusOK, body)
}

func businessError(c *gin.Context, err error) {
	if errors.Is(err, driver.ErrBadConn) || strings.Contains(strings.ToLower(err.Error()), "bad connection") {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "数据库连接暂不可用，请稍后重试"})
		return
	}
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"message": "未找到资源"})
		return
	}
	c.JSON(http.StatusConflict, gin.H{"message": err.Error()})
}
func internalError(c *gin.Context, err error) {
	c.Error(err)
	c.JSON(http.StatusInternalServerError, gin.H{"message": "服务暂不可用，请稍后重试"})
}
