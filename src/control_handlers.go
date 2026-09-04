package cloud

import (
	"context"
	"database/sql"
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
	}
	if c.ShouldBindJSON(&body) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "购买参数无效"})
		return
	}
	user := c.MustGet("user").(oidcUser)
	item, task, err := purchaseWithWallet(c.Request.Context(), user.ID, body.PlanID, body.ImageID, body.ImageVersion, body.Months)
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
		Months int `json:"months"`
	}
	if c.ShouldBindJSON(&body) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "续费参数无效"})
		return
	}
	var planID, imageID, instanceID string
	err := instanceDB.QueryRowContext(c.Request.Context(), `SELECT plan_id,image_id,instance_id FROM xcloud_orders WHERE id=? AND owner_id=? AND status IN (?,?)`, c.Param("id"), user.ID, orderActive, orderExpired).Scan(&planID, &imageID, &instanceID)
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
	if strings.TrimSpace(body.Name) == "" || strings.TrimSpace(body.ImageRef) == "" || strings.TrimSpace(body.ImageDigest) == "" || strings.TrimSpace(body.Version) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "镜像版本参数无效"})
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
		Note      string `json:"note"`
	}
	if c.ShouldBindJSON(&body) != nil || strings.TrimSpace(body.Note) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "金额和运营备注不能为空"})
		return
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
			CPUTotal      float64 `json:"cpuTotal"`
			MemoryTotalMB int     `json:"memoryTotalMB"`
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
		body.CPUTotal = 0
		body.MemoryTotalMB = 0
		body.Enabled = false
	}
	if err := saveNode(c.Request.Context(), body); err != nil {
		businessError(c, err)
		return
	}
	user := c.MustGet("user").(oidcUser)
	_ = writeAudit(c.Request.Context(), user.ID, "node.save", "node", body.ID, map[string]any{"enabled": body.Enabled})
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
	if action != "start" && action != "stop" && action != "delete" {
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
