package cloud

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
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
	c.JSON(http.StatusOK, gin.H{"images": publicCatalogImages(images), "plans": plans})
}

func publicCatalogImages(images []catalogImage) []publicCatalogImage {
	publicImages := make([]publicCatalogImage, 0, len(images))
	for _, image := range images {
		versions := make([]publicImageVersion, 0, len(image.Versions))
		for _, version := range image.Versions {
			versions = append(versions, publicImageVersion{Tag: version.Tag})
		}
		// Docker-style fallback: a source without configured release versions
		// remains purchasable through its moving latest tag.
		if len(versions) == 0 {
			versions = append(versions, publicImageVersion{Tag: "latest"})
		}
		publicImages = append(publicImages, publicCatalogImage{ID: image.ID, Name: image.Name, Versions: versions})
	}
	return publicImages
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
		PromoCode    string `json:"promoCode"`
	}
	if c.ShouldBindJSON(&body) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "购买参数无效"})
		return
	}
	user := c.MustGet("user").(oidcUser)
	item, task, err := purchaseWithWallet(c.Request.Context(), user.ID, body.PlanID, body.ImageID, body.ImageVersion, body.Months, body.PromoCode)
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
func taskStatusHandler(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	task, err := loadTask(c.Request.Context(), c.Param("id"))
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"message": "任务不存在"})
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}
	var ownerID string
	if err := instanceDB.QueryRowContext(c.Request.Context(), `SELECT owner_id FROM xcloud_instances WHERE id=?`, task.InstanceID).Scan(&ownerID); err != nil {
		internalError(c, err)
		return
	}
	if ownerID != user.ID {
		c.JSON(http.StatusNotFound, gin.H{"message": "任务不存在"})
		return
	}
	c.JSON(http.StatusOK, task)
}
func instanceTasksHandler(c *gin.Context) {
	if _, ok := ownedInstance(c); !ok {
		return
	}
	rows, err := instanceDB.QueryContext(c.Request.Context(), `SELECT `+taskSelectFields+` FROM xcloud_tasks WHERE instance_id=? ORDER BY created_at DESC LIMIT 50`, c.Param("id"))
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()
	items := []gin.H{}
	for rows.Next() {
		var t controlTask
		if err := scanControlTask(rows, &t); err != nil {
			internalError(c, err)
			return
		}
		events, _ := taskEvents(c.Request.Context(), t.ID)
		items = append(items, gin.H{"task": t, "events": events})
	}
	c.JSON(http.StatusOK, items)
}

// Users may retract a task only while it is still waiting in the durable
// queue. A running task may already be inside the Agent, where marking it
// cancelled would lie about the real container state.
func userCancelableTaskAction(action string) bool {
	switch action {
	case "start", "stop", "update", "restart", "reinstall", "retry-deploy":
		return true
	}
	return false
}

func cancelQueuedInstanceTask(ctx context.Context, item instance, task controlTask, actorID string) (bool, error) {
	if task.InstanceID != item.ID || task.Status != taskPending || !userCancelableTaskAction(task.Action) {
		return false, nil
	}
	result, err := instanceDB.ExecContext(ctx, `UPDATE xcloud_tasks SET status=?,last_error='用户已取消（任务尚未开始执行）',finished_at=NOW(),claimed_at=NULL,claim_expires_at=NULL,worker_id=NULL,execution_token=NULL,updated_at=NOW() WHERE id=? AND instance_id=? AND status=?`, taskCanceled, task.ID, item.ID, taskPending)
	if err != nil {
		return false, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return false, nil
	}
	// These actions update business state before entering the queue, so roll
	// that local marker back only after the queue cancellation has won.
	if task.Action == "update" {
		_, _ = instanceDB.ExecContext(ctx, `UPDATE xcloud_instances SET runtime_status='running' WHERE id=? AND owner_id=? AND status='running' AND runtime_status='updating'`, item.ID, item.OwnerID)
	}
	if task.Action == "retry-deploy" {
		_, _ = transitionInstance(ctx, instanceDB, item.ID, []string{"deploying"}, "deployment_failed", nil, "")
	}
	appendTaskEvent(ctx, task.ID, "cancelled_by_user", "用户取消了尚未开始执行的任务")
	_ = writeAudit(ctx, actorID, "task.cancel", "task", task.ID, map[string]any{"instanceId": item.ID, "action": task.Action})
	return true, nil
}

func cancelInstanceTaskHandler(c *gin.Context) {
	item, ok := ownedInstance(c)
	if !ok {
		return
	}
	task, err := loadTask(c.Request.Context(), c.Param("taskID"))
	if err == sql.ErrNoRows || task.InstanceID != item.ID {
		c.JSON(http.StatusNotFound, gin.H{"message": "任务不存在"})
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}
	user := c.MustGet("user").(oidcUser)
	cancelled, err := cancelQueuedInstanceTask(c.Request.Context(), item, task, user.ID)
	if err != nil {
		internalError(c, err)
		return
	}
	if !cancelled {
		c.JSON(http.StatusConflict, gin.H{"message": "仅等待执行的用户操作可以取消；执行中的任务不能强制中断"})
		return
	}
	c.Status(http.StatusNoContent)
}

func cancelAllInstanceTasksHandler(c *gin.Context) {
	item, ok := ownedInstance(c)
	if !ok {
		return
	}
	rows, err := instanceDB.QueryContext(c.Request.Context(), `SELECT `+taskSelectFields+` FROM xcloud_tasks WHERE instance_id=? AND status=? AND action IN ('start','stop','update','restart','reinstall','retry-deploy') ORDER BY created_at`, item.ID, taskPending)
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()
	tasks := []controlTask{}
	for rows.Next() {
		var task controlTask
		if err := scanControlTask(rows, &task); err != nil {
			internalError(c, err)
			return
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		internalError(c, err)
		return
	}
	user := c.MustGet("user").(oidcUser)
	count := 0
	for _, task := range tasks {
		cancelled, err := cancelQueuedInstanceTask(c.Request.Context(), item, task, user.ID)
		if err != nil {
			internalError(c, err)
			return
		}
		if cancelled {
			count++
		}
	}
	_ = writeAudit(c.Request.Context(), user.ID, "task.cancel_all", "instance", item.ID, map[string]any{"count": count})
	c.JSON(http.StatusOK, gin.H{"cancelled": count})
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

// renewWithWalletHandler renews an existing service without reviving the
// retired manual-payment flow.  The debit, ledger entry, renewal order and
// (when necessary) restart task are committed as one transaction.
func renewWithWalletHandler(c *gin.Context) {
	var body struct {
		Months    int    `json:"months"`
		PromoCode string `json:"promoCode"`
	}
	if c.ShouldBindJSON(&body) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "续费参数无效"})
		return
	}
	user := c.MustGet("user").(oidcUser)
	item, task, err := renewWithWallet(c.Request.Context(), user.ID, c.Param("id"), body.Months, body.PromoCode)
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
	_ = writeAudit(c.Request.Context(), user.ID, "catalog.image.save", "image", body.ID, map[string]any{"imageRef": body.ImageRef, "version": body.Version, "terminalOnly": body.TerminalOnly})
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
func adminDeleteImageVersion(c *gin.Context) {
	var tag string
	if err := instanceDB.QueryRowContext(c.Request.Context(), `SELECT version_tag FROM xcloud_image_versions WHERE id=? AND image_id=?`, c.Param("versionID"), c.Param("id")).Scan(&tag); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "版本不存在"})
		return
	}
	var used int
	if err := instanceDB.QueryRowContext(c.Request.Context(), `SELECT COUNT(*) FROM xcloud_orders WHERE image_id=? AND selected_image_version=?`, c.Param("id"), tag).Scan(&used); err != nil {
		internalError(c, err)
		return
	}
	if used > 0 {
		c.JSON(http.StatusConflict, gin.H{"message": "该版本已有历史订单，不能删除；可改为下架"})
		return
	}
	_, _ = instanceDB.ExecContext(c.Request.Context(), `DELETE FROM xcloud_image_version_pulls WHERE image_version_id=?`, c.Param("versionID"))
	result, err := instanceDB.ExecContext(c.Request.Context(), `DELETE FROM xcloud_image_versions WHERE id=? AND image_id=?`, c.Param("versionID"), c.Param("id"))
	if err != nil {
		internalError(c, err)
		return
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		c.JSON(http.StatusConflict, gin.H{"message": "版本已变化，请刷新后重试"})
		return
	}
	user := c.MustGet("user").(oidcUser)
	_ = writeAudit(c.Request.Context(), user.ID, "catalog.image_version.delete", "image_version", c.Param("versionID"), map[string]any{"imageId": c.Param("id"), "tag": tag})
	c.Status(http.StatusNoContent)
}
func adminPullImageVersion(c *gin.Context) {
	versionID := c.Param("versionID")
	user := c.MustGet("user").(oidcUser)
	if err := publishImageVersion(c.Request.Context(), c.Param("id"), versionID, user.ID); err != nil {
		businessError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"message": "正在验证并上架版本"})
}

func adminPublishImageVersion(c *gin.Context) {
	var body struct {
		Tag string `json:"tag"`
	}
	if c.ShouldBindJSON(&body) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "版本号无效"})
		return
	}
	item, err := saveImageVersion(c.Request.Context(), imageVersion{ImageID: c.Param("id"), Tag: body.Tag})
	if err != nil {
		businessError(c, err)
		return
	}
	user := c.MustGet("user").(oidcUser)
	if err := publishImageVersion(c.Request.Context(), item.ImageID, item.ID, user.ID); err != nil {
		businessError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"version": item, "message": "正在验证并上架版本"})
}

func publishImageVersion(ctx context.Context, imageID, versionID, actorID string) error {
	var imageRef, tag, digest string
	err := instanceDB.QueryRowContext(ctx, `SELECT i.image_ref,v.version_tag,COALESCE(v.image_digest,'') FROM xcloud_image_versions v JOIN xcloud_images i ON i.id=v.image_id WHERE v.id=? AND v.image_id=?`, versionID, imageID).Scan(&imageRef, &tag, &digest)
	if err != nil {
		return errors.New("软件版本不存在")
	}
	// Always resolve from the tag during publication. A stored digest is a
	// previous immutable snapshot, not a substitute for a fresh verification.
	image := deploymentImage(imageRef, tag, "")
	if _, err := instanceDB.ExecContext(ctx, `UPDATE xcloud_image_versions SET enabled=FALSE,version_status='syncing',last_error=NULL,updated_at=NOW() WHERE id=?`, versionID); err != nil {
		return err
	}
	_ = writeAudit(ctx, actorID, "catalog.image_version.publish", "image_version", versionID, map[string]any{"image": image})
	go pullImageOnNodes(context.Background(), versionID, image)
	return nil
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
		// The registration dialog intentionally sends zero capacities and leaves
		// the node disabled for a second configuration step. API callers may
		// however provide a valid capacity and enable the verified node in the
		// same request; never silently turn that request back into a disabled
		// zero-capacity node.
		if body.CPUTotal <= 0 {
			body.CPUTotal = status.CPUTotal
		}
		if body.MemoryTotalMB <= 0 {
			body.MemoryTotalMB = status.MemoryTotalMB
		}
	}
	if err := saveNode(c.Request.Context(), body); err != nil {
		businessError(c, err)
		return
	}
	// A freshly verified enabled node should be schedulable immediately instead
	// of waiting for the next periodic heartbeat pass.
	if body.Enabled {
		syncNodeHeartbeat(c.Request.Context())
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

func reconcileAllBandwidthHandler(c *gin.Context) {
	rows, err := instanceDB.QueryContext(c.Request.Context(), `SELECT i.id,COALESCE(n.agent_capabilities,JSON_ARRAY()) FROM xcloud_instances i JOIN xcloud_nodes n ON n.id=i.node_id WHERE i.status IN ('running','destroy_scheduled') AND COALESCE(i.runtime_status,i.status)='running'`)
	if err != nil {
		internalError(c, err)
		return
	}
	instanceIDs := make([]string, 0)
	for rows.Next() {
		var id string
		var rawCapabilities []byte
		if rows.Scan(&id, &rawCapabilities) != nil {
			continue
		}
		var capabilities []string
		_ = json.Unmarshal(rawCapabilities, &capabilities)
		statusSupported := false
		queueSupported := false
		for _, capability := range capabilities {
			if capability == "network.bandwidth.status.v1" {
				statusSupported = true
			}
			if capability == "network.bandwidth.queue.v1" {
				queueSupported = true
			}
		}
		if statusSupported && queueSupported {
			instanceIDs = append(instanceIDs, id)
		}
	}
	if err := rows.Close(); err != nil {
		internalError(c, err)
		return
	}
	count := 0
	for _, id := range instanceIDs {
		task, scheduled, e := scheduleBandwidthTask(c.Request.Context(), id, "system")
		if e == nil && scheduled {
			_ = enqueuePersistedTask(c.Request.Context(), task)
			count++
		}
	}
	user := c.MustGet("user").(oidcUser)
	_ = writeAudit(c.Request.Context(), user.ID, "bandwidth.reconcile", "instance", "all", map[string]any{"tasks": count})
	c.JSON(http.StatusAccepted, gin.H{"tasks": count})
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
	ctx := c.Request.Context()
	task, err := loadTask(ctx, c.Param("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"message": "任务不存在"})
			return
		}
		internalError(c, err)
		return
	}
	if task.Status != taskFailed {
		c.JSON(http.StatusConflict, gin.H{"message": "任务当前不可重试"})
		return
	}

	// A failed destructive lifecycle operation is evidence of an unknown
	// container state. It must never be replayed from a convenience button:
	// require an operator to inspect the task and explicitly resume it instead.
	if dangerousRecoveredTask(task.Action) {
		result, err := instanceDB.ExecContext(ctx, `UPDATE xcloud_tasks SET status=?,last_error=CONCAT(COALESCE(last_error,''), '\n已由管理员操作隔离：危险生命周期任务需人工复核后才能恢复'),claimed_at=NULL,claim_expires_at=NULL,worker_id=NULL,execution_token=NULL,updated_at=NOW() WHERE id=? AND status=?`, taskReview, task.ID, taskFailed)
		if err != nil {
			internalError(c, err)
			return
		}
		if n, _ := result.RowsAffected(); n != 1 {
			c.JSON(http.StatusConflict, gin.H{"message": "任务状态已变化，请刷新后重试"})
			return
		}
		appendTaskEvent(ctx, task.ID, "manual_retry_quarantined", "危险生命周期任务已转入人工复核，未重放容器操作")
		user := c.MustGet("user").(oidcUser)
		_ = writeAudit(ctx, user.ID, "task.retry_quarantined", "task", task.ID, map[string]any{"action": task.Action, "instanceId": task.InstanceID})
		c.JSON(http.StatusAccepted, gin.H{"message": "危险任务已转入人工复核，未执行重试"})
		return
	}

	// A deliberate new attempt starts a fresh retry budget. Without this reset a
	// task that previously reached its retry limit fails immediately forever.
	result, err := instanceDB.ExecContext(ctx, `UPDATE xcloud_tasks SET status=?,attempts=0,run_after=NOW(),last_error=NULL,finished_at=NULL,claimed_at=NULL,claim_expires_at=NULL,worker_id=NULL,execution_token=NULL,updated_at=NOW() WHERE id=? AND status=?`, taskPending, task.ID, taskFailed)
	if err != nil {
		internalError(c, err)
		return
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		c.JSON(http.StatusConflict, gin.H{"message": "任务当前不可重试"})
		return
	}
	appendTaskEvent(ctx, task.ID, "manual_retry", "管理员手动重试，已开始新的执行代次")
	user := c.MustGet("user").(oidcUser)
	_ = writeAudit(ctx, user.ID, "task.retry", "task", task.ID, nil)
	task, err = loadTask(ctx, task.ID)
	if err == nil {
		_ = enqueuePersistedTask(ctx, task)
	}
	c.Status(http.StatusAccepted)
}

func resumeReviewTask(c *gin.Context) {
	result, err := instanceDB.ExecContext(c.Request.Context(), `UPDATE xcloud_tasks SET status=?,run_after=NOW(),last_error=NULL,claimed_at=NULL,claim_expires_at=NULL,worker_id=NULL,execution_token=NULL,updated_at=NOW() WHERE id=? AND status=?`, taskPending, c.Param("id"), taskReview)
	if err != nil {
		internalError(c, err)
		return
	}
	if n, _ := result.RowsAffected(); n != 1 {
		c.JSON(http.StatusConflict, gin.H{"message": "任务当前不可恢复"})
		return
	}
	user := c.MustGet("user").(oidcUser)
	_, _ = instanceDB.ExecContext(c.Request.Context(), `UPDATE xcloud_instances SET active_task_id=NULL,active_task_token=NULL,active_task_expires_at=NULL WHERE active_task_id=?`, c.Param("id"))
	appendTaskEvent(c.Request.Context(), c.Param("id"), "review_resumed", "管理员确认恢复")
	_ = writeAudit(c.Request.Context(), user.ID, "task.review_resume", "task", c.Param("id"), nil)
	if task, err := loadTask(c.Request.Context(), c.Param("id")); err == nil {
		_ = enqueuePersistedTask(c.Request.Context(), task)
	}
	c.Status(http.StatusAccepted)
}
func discardReviewTask(c *gin.Context) {
	task, loadErr := loadTask(c.Request.Context(), c.Param("id"))
	if loadErr != nil {
		internalError(c, loadErr)
		return
	}
	result, err := instanceDB.ExecContext(c.Request.Context(), `UPDATE xcloud_tasks SET status=?,finished_at=NOW(),last_error='管理员作废',updated_at=NOW() WHERE id=? AND status=?`, taskFailed, c.Param("id"), taskReview)
	if err != nil {
		internalError(c, err)
		return
	}
	if n, _ := result.RowsAffected(); n != 1 {
		c.JSON(http.StatusConflict, gin.H{"message": "任务当前不可作废"})
		return
	}
	if task.Action == "resize" {
		failPlanChange(c.Request.Context(), task, errors.New("管理员作废套餐变更任务"))
	}
	user := c.MustGet("user").(oidcUser)
	_, _ = instanceDB.ExecContext(c.Request.Context(), `UPDATE xcloud_instances SET active_task_id=NULL,active_task_token=NULL,active_task_expires_at=NULL WHERE active_task_id=?`, c.Param("id"))
	appendTaskEvent(c.Request.Context(), c.Param("id"), "review_discarded", "管理员作废隔离任务")
	_ = writeAudit(c.Request.Context(), user.ID, "task.review_discard", "task", c.Param("id"), nil)
	c.Status(http.StatusNoContent)
}

// discardAllAdminTasks closes all historical failed/review work in one
// deliberate administrative operation. It never touches pending or running
// work, which could still be picked up by a worker or executing in an Agent.
func discardAllAdminTasks(c *gin.Context) {
	ctx := c.Request.Context()
	rows, err := instanceDB.QueryContext(ctx, `SELECT id,instance_id,action FROM xcloud_tasks WHERE status IN (?,?) ORDER BY created_at LIMIT 500`, taskFailed, taskReview)
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()
	type taskRef struct{ id, instanceID, action string }
	items := []taskRef{}
	for rows.Next() {
		var item taskRef
		if err := rows.Scan(&item.id, &item.instanceID, &item.action); err != nil {
			internalError(c, err)
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		internalError(c, err)
		return
	}
	count := 0
	for _, item := range items {
		result, err := instanceDB.ExecContext(ctx, `UPDATE xcloud_tasks SET status=?,last_error=CONCAT(COALESCE(last_error,''), '\n管理员已一键作废：任务不会再次执行'),finished_at=NOW(),claimed_at=NULL,claim_expires_at=NULL,worker_id=NULL,execution_token=NULL,updated_at=NOW() WHERE id=? AND status IN (?,?)`, taskDiscarded, item.id, taskFailed, taskReview)
		if err != nil {
			internalError(c, err)
			return
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			continue
		}
		if item.action == "resize" {
			task, loadErr := loadTask(ctx, item.id)
			if loadErr == nil {
				failPlanChange(ctx, task, errors.New("管理员批量作废套餐变更任务"))
			}
		}
		_, _ = instanceDB.ExecContext(ctx, `UPDATE xcloud_instances SET active_task_id=NULL,active_task_token=NULL,active_task_expires_at=NULL WHERE id=? AND active_task_id=?`, item.instanceID, item.id)
		appendTaskEvent(ctx, item.id, "discarded_by_admin", "管理员一键作废异常任务；任务不会再次执行")
		count++
	}
	user := c.MustGet("user").(oidcUser)
	_ = writeAudit(ctx, user.ID, "task.discard_all", "task", "all", map[string]any{"count": count})
	c.JSON(http.StatusOK, gin.H{"discarded": count})
}

func queueInstanceAction(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	item, ok := ownedInstance(c)
	if !ok {
		return
	}
	action := c.Param("action")
	if action == "delete" {
		action = "destroy"
	}
	if action == "destroy" {
		scheduleManualDestroy(c, item, user.ID)
		return
	}
	if action == "cancel-destroy" {
		cancelManualDestroy(c, item, user.ID)
		return
	}
	if action == "archive" {
		archiveDestroyedInstance(c, item, user.ID)
		return
	}
	if active, err := activeLifecycleTask(c.Request.Context(), item.ID); err != nil {
		internalError(c, err)
		return
	} else if active != nil {
		c.JSON(http.StatusConflict, gin.H{"message": "实例正在处理中", "task": active})
		return
	}
	if action == "destroy-now" {
		queueImmediateDestroy(c, item, user.ID)
		return
	}
	if action == "retry-deploy" {
		queueDeploymentRetry(c, item, user.ID)
		return
	}
	if action == "update" {
		queueInstanceUpdate(c, item, user.ID)
		return
	}
	if action != "start" && action != "stop" && action != "restart" && action != "reinstall" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "不支持的实例操作"})
		return
	}
	if item.Status == "destroyed" || item.Status == "purged" {
		c.JSON(http.StatusConflict, gin.H{"message": "实例资源已销毁，不能执行运行操作"})
		return
	}
	if action == "reinstall" && item.Status != "running" && item.Status != "stopped" {
		c.JSON(http.StatusConflict, gin.H{"message": "仅运行中或已关机的实例可以重装"})
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

// activeLifecycleTask prevents a user action from creating a second queue
// behind an in-flight lifecycle task. The caller receives the task summary and
// can refresh its true state instead of risking a conflicting Agent call.
func activeLifecycleTask(ctx context.Context, instanceID string) (*controlTask, error) {
	var taskID string
	err := instanceDB.QueryRowContext(ctx, `SELECT id FROM xcloud_tasks
		WHERE instance_id=? AND status IN ('pending','running')
		AND action IN ('create','retry-deploy','start','stop','update','restart','reinstall','destroy','purge','resize')
		ORDER BY created_at DESC LIMIT 1`, instanceID).Scan(&taskID)
	if err == sql.ErrNoRows || taskID == "" {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	task, err := loadTask(ctx, taskID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func queueInstanceUpdate(c *gin.Context, item instance, actorID string) {
	if item.Status != "running" {
		c.JSON(http.StatusConflict, gin.H{"message": "仅运行中的实例可以更新"})
		return
	}
	if !validImageTag(item.Version) {
		c.JSON(http.StatusConflict, gin.H{"message": "实例没有可更新的版本标记"})
		return
	}
	now := time.Now()
	task := controlTask{ID: newID("task"), InstanceID: item.ID, Action: "update", IdempotencyKey: "update:" + item.ID + ":" + now.UTC().Format(time.RFC3339Nano), Status: taskPending, RunAfter: now, CreatedAt: now, UpdatedAt: now}
	result, err := instanceDB.ExecContext(c.Request.Context(), `UPDATE xcloud_instances SET runtime_status='updating' WHERE id=? AND owner_id=? AND status='running' AND COALESCE(runtime_status,'')<>'updating'`, item.ID, item.OwnerID)
	if err != nil {
		internalError(c, err)
		return
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		c.JSON(http.StatusConflict, gin.H{"message": "实例正在更新或状态已变化"})
		return
	}
	if _, err = instanceDB.ExecContext(c.Request.Context(), `INSERT INTO xcloud_tasks (id,instance_id,action,idempotency_key,status,attempts,run_after,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?)`, task.ID, task.InstanceID, task.Action, task.IdempotencyKey, task.Status, 0, task.RunAfter, now, now); err != nil {
		_, _ = instanceDB.ExecContext(c.Request.Context(), `UPDATE xcloud_instances SET runtime_status='running' WHERE id=? AND status='running' AND runtime_status='updating'`, item.ID)
		internalError(c, err)
		return
	}
	appendTaskEvent(c.Request.Context(), task.ID, "queued", "等待执行实例更新")
	_ = writeAudit(c.Request.Context(), actorID, "instance.update", "instance", item.ID, map[string]any{"tag": item.Version, "taskId": task.ID})
	if err = enqueuePersistedTask(c.Request.Context(), task); err != nil {
		c.JSON(http.StatusAccepted, gin.H{"task": task, "message": "更新任务已记录，等待队列恢复"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"task": task})
}

func queueDeploymentRetry(c *gin.Context, item instance, actorID string) {
	if item.Status != "deployment_failed" {
		c.JSON(http.StatusConflict, gin.H{"message": "实例当前不可重试部署"})
		return
	}
	runtime := "unknown"
	changed, err := transitionInstance(c.Request.Context(), instanceDB, item.ID, []string{"deployment_failed"}, "deploying", &runtime, "")
	if err != nil {
		internalError(c, err)
		return
	}
	if !changed {
		c.JSON(http.StatusConflict, gin.H{"message": "实例状态已变化，请刷新后重试"})
		return
	}
	task, err := scheduleInstanceTask(c.Request.Context(), item.ID, "retry-deploy", actorID)
	if err != nil {
		_, _ = transitionInstance(c.Request.Context(), instanceDB, item.ID, []string{"deploying"}, "deployment_failed", nil, "")
		businessError(c, err)
		return
	}
	_ = writeAudit(c.Request.Context(), actorID, "instance.deployment_retry", "instance", item.ID, map[string]any{"taskId": task.ID})
	if err := enqueuePersistedTask(c.Request.Context(), task); err != nil {
		c.JSON(http.StatusAccepted, gin.H{"task": task, "message": "重试任务已记录，等待队列恢复"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"task": task})
}

func queueDeleteInstance(c *gin.Context) {
	c.Params = append(c.Params, gin.Param{Key: "action", Value: "destroy"})
	queueInstanceAction(c)
}

func scheduleManualDestroy(c *gin.Context, item instance, actorID string) {
	if item.Status == "destroy_scheduled" {
		c.JSON(http.StatusConflict, gin.H{"message": "实例已在待销毁队列中"})
		return
	}
	if item.Status == "destroyed" || item.Status == "purged" {
		c.JSON(http.StatusConflict, gin.H{"message": "实例资源已销毁"})
		return
	}
	runtimeStatus := item.Status
	if runtimeStatus != "running" && runtimeStatus != "stopped" {
		c.JSON(http.StatusConflict, gin.H{"message": "实例当前不能计划销毁"})
		return
	}
	destroyAt := time.Now().AddDate(0, 0, 7)
	changed, err := transitionInstance(c.Request.Context(), instanceDB, item.ID, []string{"running", "stopped"}, "destroy_scheduled", &runtimeStatus, "destroy_reason='manual',destroy_at=?,destroyed_at=NULL,purge_at=NULL", destroyAt)
	if err != nil {
		internalError(c, err)
		return
	}
	if !changed {
		c.JSON(http.StatusConflict, gin.H{"message": "实例状态已变化，请刷新后重试"})
		return
	}
	task, taskErr := scheduleLifecycleTaskAt(c.Request.Context(), item.ID, "destroy", destroyAt, destroyAt)
	if taskErr != nil {
		internalError(c, taskErr)
		return
	}
	appendTaskEvent(c.Request.Context(), task.ID, "manual_destroy_scheduled", "用户手动计划销毁；将在计划时间执行")
	_ = writeAudit(c.Request.Context(), actorID, "instance.destroy_scheduled", "instance", item.ID, map[string]any{"reason": "manual", "destroyAt": destroyAt, "taskId": task.ID})
	_ = createNotification(c.Request.Context(), item.OwnerID, "instance_destroy_scheduled", "实例已计划销毁", fmt.Sprintf("服务将继续保持当前状态，实例资源预计于 %s 销毁。", destroyAt.Format("2006-01-02 15:04")), map[string]any{"instanceId": item.ID, "destroyAt": destroyAt, "reason": "manual"})
	c.JSON(http.StatusAccepted, gin.H{"task": task, "message": "已计划 7 天后销毁实例资源", "destroyAt": destroyAt})
}

func cancelManualDestroy(c *gin.Context, item instance, actorID string) {
	if active, err := activeLifecycleTask(c.Request.Context(), item.ID); err != nil {
		internalError(c, err)
		return
	} else if active != nil {
		c.JSON(http.StatusConflict, gin.H{"message": "实例正在处理中，暂不能取消销毁", "task": active})
		return
	}
	if item.Status != "destroy_scheduled" {
		c.JSON(http.StatusConflict, gin.H{"message": "实例当前未处于待销毁状态"})
		return
	}
	if item.DestroyReason == "refund" {
		c.JSON(http.StatusConflict, gin.H{"message": "退款后的销毁计划不可撤销"})
		return
	}
	if item.DestroyReason == "expired" {
		c.JSON(http.StatusConflict, gin.H{"message": "正常到期的销毁计划请先续费后撤销"})
		return
	}
	if item.DestroyReason != "manual" {
		c.JSON(http.StatusConflict, gin.H{"message": "实例销毁计划不可撤销"})
		return
	}
	runtimeStatus := item.RuntimeStatus
	if runtimeStatus != "running" && runtimeStatus != "stopped" {
		runtimeStatus = "stopped"
	}
	changed, err := transitionInstance(c.Request.Context(), instanceDB, item.ID, []string{"destroy_scheduled"}, runtimeStatus, &runtimeStatus, "destroy_at=NULL,destroy_reason=NULL")
	if err != nil {
		internalError(c, err)
		return
	}
	if !changed {
		c.JSON(http.StatusConflict, gin.H{"message": "实例状态已变化，请刷新后重试"})
		return
	}
	_ = writeAudit(c.Request.Context(), actorID, "instance.destroy_cancelled", "instance", item.ID, nil)
	_ = createNotification(c.Request.Context(), item.OwnerID, "instance_destroy_cancelled", "实例销毁计划已取消", "实例会继续保持当前运行状态。", map[string]any{"instanceId": item.ID})
	c.JSON(http.StatusOK, gin.H{"message": "销毁计划已取消"})
}

func queueImmediateDestroy(c *gin.Context, item instance, actorID string) {
	if item.Status != "destroy_scheduled" {
		c.JSON(http.StatusConflict, gin.H{"message": "仅待销毁实例可以立即销毁资源"})
		return
	}
	at := time.Now()
	if item.DestroyAt != nil {
		at = *item.DestroyAt
	}
	task, err := scheduleLifecycleTask(c.Request.Context(), item.ID, "destroy", at)
	if err != nil {
		businessError(c, err)
		return
	}
	_ = writeAudit(c.Request.Context(), actorID, "instance.destroy_now", "instance", item.ID, map[string]any{"taskId": task.ID})
	if err := enqueuePersistedTask(c.Request.Context(), task); err != nil {
		c.JSON(http.StatusAccepted, gin.H{"task": task, "message": "销毁任务已记录，等待队列恢复"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"task": task})
}

func archiveDestroyedInstance(c *gin.Context, item instance, actorID string) {
	if item.Status != "destroyed" {
		c.JSON(http.StatusConflict, gin.H{"message": "仅已销毁实例可以从列表移除"})
		return
	}
	result, err := instanceDB.ExecContext(c.Request.Context(), `UPDATE xcloud_instances SET archived_at=NOW() WHERE id=? AND owner_id=? AND status='destroyed' AND archived_at IS NULL`, item.ID, item.OwnerID)
	if err != nil {
		internalError(c, err)
		return
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		c.JSON(http.StatusConflict, gin.H{"message": "实例已从列表移除或状态已变化"})
		return
	}
	_ = writeAudit(c.Request.Context(), actorID, "instance.archived", "instance", item.ID, nil)
	c.JSON(http.StatusOK, gin.H{"message": "实例已从列表移除"})
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
	tail := 300
	if raw := strings.TrimSpace(c.Query("tail")); raw != "" {
		value, parseErr := strconv.Atoi(raw)
		if parseErr != nil || value < 50 || value > 1000 {
			c.JSON(http.StatusBadRequest, gin.H{"message": "日志行数必须介于 50 和 1000 之间"})
			return
		}
		tail = value
	}
	path := "/container/" + item.ContainerName + "/logs?tail=" + strconv.Itoa(tail)
	if raw := strings.TrimSpace(c.Query("since")); raw != "" {
		if _, parseErr := time.Parse(time.RFC3339, raw); parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "日志起始时间无效"})
			return
		}
		path += "&since=" + url.QueryEscape(raw)
	}
	var body struct {
		Lines     []string `json:"lines"`
		Tail      int      `json:"tail"`
		Truncated bool     `json:"truncated"`
	}
	if err := nodeRequest(c.Request.Context(), n, http.MethodGet, path, nil, &body); err != nil {
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
