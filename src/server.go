package cloud

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

//go:embed all:dist
var staticFiles embed.FS

var Version = "dev"

const sessionCookieName = "alemonx_sid"
const oidcStateCookieName = "alemonx_oidc_state"
const oidcVerifierCookieName = "alemonx_oidc_verifier"

type instance struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Image         string `json:"image"`
	Version       string `json:"version"`
	Spec          string `json:"spec"`
	Status        string `json:"status"`
	IP            string `json:"ip"`
	CreatedAt     string `json:"createdAt"`
	ContainerName string `json:"-"`
	OwnerID       string `json:"-"`
}
type oidcUser struct {
	ID          string   `json:"id"`
	Username    string   `json:"username"`
	Email       string   `json:"email"`
	Avatar      string   `json:"avatar"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
	IsAdmin     bool     `json:"isAdmin"`
}
type session struct {
	User      oidcUser
	ExpiresAt time.Time
}

const sessionPrefix = "alemonxcloud:session:"

var sessions = map[string]session{}
var sessionsMu sync.RWMutex
var sessionRedis *redis.Client

func Run() {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Printf("load .env: %v", err)
	}
	mode := env("GIN_MODE", gin.DebugMode)
	gin.SetMode(mode)
	if mode == gin.ReleaseMode {
		validateProductionConfig()
	}
	if err := initSessionStore(); err != nil && mode == gin.ReleaseMode {
		log.Fatalf("initialize Redis: %v", err)
	}
	if err := initInstanceStore(); err != nil && mode == gin.ReleaseMode {
		log.Fatalf("initialize MySQL: %v", err)
	}
	if err := initializeSchemaMigrations(context.Background()); err != nil {
		if mode == gin.ReleaseMode {
			log.Fatalf("initialize schema migrations: %v", err)
		}
		log.Printf("schema migrations unavailable: %v", err)
	}
	if err := initTaskQueue(); err != nil {
		if mode == gin.ReleaseMode {
			log.Fatalf("RabbitMQ unavailable: %v", err)
		}
		log.Printf("task queue unavailable: %v", err)
	} else {
		consumeTasks()
		recoverPendingTasks()
	}
	// Node health polling must not depend on RabbitMQ. A newly registered
	// node needs to become schedulable even while task delivery is recovering.
	startControlLoops()
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	router.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok", "version": Version}) })
	router.GET("/readyz", readiness)
	router.GET("/metrics", metrics)
	router.Any("/__instance_proxy", instanceGateway)
	router.GET("/api/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"message": "pong"}) })
	router.GET("/api/instances", requireSession, listInstances)
	router.POST("/api/instances", requireSession, createInstance)
	router.POST("/api/instances/:id/:action", requireSession, queueInstanceAction)
	router.DELETE("/api/instances/:id", requireSession, queueDeleteInstance)
	router.GET("/api/instances/:id/logs", requireSession, instanceLogs)
	router.GET("/api/catalog", requireSession, catalog)
	router.GET("/api/wallet", requireSession, walletHandler)
	router.GET("/api/wallet/entries", requireSession, walletEntriesHandler)
	router.POST("/api/purchases", requireSession, purchaseHandler)
	router.POST("/api/purchases/quote", requireSession, quotePurchaseHandler)
	router.GET("/api/notifications", requireSession, notificationsHandler)
	router.POST("/api/telemetry/console-events", requireSession, consoleTelemetryHandler)
	router.POST("/api/notifications/read-all", requireSession, readAllNotificationsHandler)
	router.POST("/api/notifications/:id/read", requireSession, readNotificationHandler)
	router.GET("/api/instances/:id/tasks", requireSession, instanceTasksHandler)
	router.GET("/api/orders", requireSession, myOrders)
	router.GET("/api/tickets", requireSession, myTicketsHandler)
	router.POST("/api/tickets", requireSession, createTicketHandler)
	router.GET("/api/tickets/:id", requireSession, ticketDetailHandler)
	router.POST("/api/tickets/:id/messages", requireSession, ticketReplyHandler)
	router.POST("/api/tickets/:id/reopen", requireSession, reopenTicketHandler)
	router.POST("/api/orders", requireSession, manualPaymentDisabled)
	router.POST("/api/orders/:id/cancel", requireSession, manualPaymentDisabled)
	router.POST("/api/orders/:id/payment", requireSession, manualPaymentDisabled)
	router.POST("/api/orders/:id/renew", requireSession, renewWithWalletHandler)
	router.POST("/api/orders/:id/renew/quote", requireSession, quoteRenewHandler)
	router.GET("/api/orders/:id/refund-quote", requireSession, refundQuoteHandler)
	router.POST("/api/orders/:id/refund", requireSession, refundOrderHandler)
	router.GET("/api/admin/catalog", requireAdmin, adminCatalog)
	router.POST("/api/admin/images", requireAdmin, adminSaveImage)
	router.PUT("/api/admin/images/:id", requireAdmin, adminSaveImage)
	router.POST("/api/admin/images/:id/versions", requireAdmin, adminSaveImageVersion)
	router.PUT("/api/admin/images/:id/versions/:versionID", requireAdmin, adminSaveImageVersion)
	router.POST("/api/admin/images/:id/versions/:versionID/pull", requireAdmin, adminPullImageVersion)
	router.POST("/api/admin/plans", requireAdmin, adminSavePlan)
	router.PUT("/api/admin/plans/:id", requireAdmin, adminSavePlan)
	router.GET("/api/admin/orders", requireAdmin, adminOrders)
	router.GET("/api/admin/promotions", requireAdmin, adminPromotions)
	router.POST("/api/admin/promotions", requireAdmin, adminSavePromotion)
	router.PUT("/api/admin/promotions/:id", requireAdmin, adminSavePromotion)
	router.GET("/api/admin/coupons", requireAdmin, adminCoupons)
	router.POST("/api/admin/coupons", requireAdmin, adminCreateCoupons)
	router.POST("/api/admin/coupons/:id/status", requireAdmin, adminCouponStatus)
	router.GET("/api/admin/coupon-redemptions", requireAdmin, adminRedemptions)
	router.GET("/api/admin/tickets", requireAdmin, adminTicketsHandler)
	router.GET("/api/admin/tickets/:id", requireAdmin, adminTicketDetailHandler)
	router.POST("/api/admin/tickets/:id/messages", requireAdmin, adminTicketReplyHandler)
	router.POST("/api/admin/tickets/:id/status", requireAdmin, adminTicketStatusHandler)
	router.POST("/api/admin/tickets/:id/priority", requireAdmin, adminTicketPriorityHandler)
	router.POST("/api/admin/orders/:id/confirm", requireAdmin, manualPaymentDisabled)
	router.POST("/api/admin/orders/:id/reject", requireAdmin, manualPaymentDisabled)
	router.GET("/api/admin/nodes", requireAdmin, adminNodes)
	router.POST("/api/admin/nodes", requireAdmin, adminSaveNode)
	router.PUT("/api/admin/nodes/:id", requireAdmin, adminSaveNode)
	router.GET("/api/admin/users", requireAdmin, adminUsers)
	router.GET("/api/admin/users/:id/wallet/entries", requireAdmin, adminWalletEntries)
	router.POST("/api/admin/users/:id/wallet/adjust", requireAdmin, adminAdjustWallet)
	router.GET("/api/admin/tasks", requireAdmin, adminTasks)
	router.GET("/api/admin/audit-logs", requireAdmin, adminAuditLogs)
	router.POST("/api/admin/tasks/:id/retry", requireAdmin, retryTask)
	router.GET("/api/admin/metrics", requireAdmin, adminMetricsHandler)
	router.POST("/api/oidc/authorize", oidcAuthorize)
	router.POST("/api/oidc/callback", oidcCallback)
	router.GET("/api/oidc/session", sessionInfo)
	router.POST("/api/logout", logout)
	if mode == gin.DebugMode {
		router.POST("/api/dev/login", devLogin)
	}

	staticFS, err := fs.Sub(staticFiles, "dist")
	if err != nil {
		log.Fatalf("load embedded frontend: %v", err)
	}
	router.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"message": "接口不存在"})
			return
		}
		if path != "/" {
			if info, err := fs.Stat(staticFS, path[1:]); err == nil && !info.IsDir() {
				c.FileFromFS(path, http.FS(staticFS))
				return
			}
		}
		content, err := fs.ReadFile(staticFS, "index.html")
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", content)
	})
	address := listenAddress(env("PORT", ":8082"))
	listener, err := listenServer(address, mode)
	if err != nil {
		log.Fatal(err)
	}
	address = listener.Addr().String()
	log.Printf("AlemonX Cloud %s listening on %s", Version, address)
	if err := router.RunListener(listener); err != nil {
		log.Fatal(err)
	}
}

func listenServer(address, mode string) (net.Listener, error) {
	listener, err := net.Listen("tcp", address)
	if err == nil || mode != gin.DebugMode {
		return listener, err
	}
	if !isInteractiveInput() {
		return nil, fmt.Errorf("端口 %s 已被占用（开发模式非交互启动不会自动终止占用进程）", address)
	}

	pids, findErr := portListenerPIDs(address)
	if findErr != nil {
		return nil, findErr
	}
	if len(pids) == 0 {
		return nil, fmt.Errorf("端口 %s 已被占用，但无法定位占用进程", address)
	}
	reader := bufio.NewReader(os.Stdin)
	fmt.Fprintf(os.Stderr, "端口 %s 已被进程 %s 占用，是否终止该进程后继续启动？[y/N] ", address, strings.Join(pids, ", "))
	answer, readErr := reader.ReadString('\n')
	if readErr != nil && len(answer) == 0 {
		return nil, fmt.Errorf("端口 %s 已被占用，未获得继续启动确认", address)
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer != "y" && answer != "yes" {
		return nil, fmt.Errorf("端口 %s 已被占用，已取消启动", address)
	}
	for _, pid := range pids {
		value, convertErr := strconv.Atoi(pid)
		if convertErr != nil {
			return nil, fmt.Errorf("占用进程 PID 无效: %s", pid)
		}
		process, processErr := os.FindProcess(value)
		if processErr != nil {
			return nil, fmt.Errorf("定位占用进程 %s: %w", pid, processErr)
		}
		if signalErr := process.Signal(syscall.SIGTERM); signalErr != nil {
			return nil, fmt.Errorf("终止占用进程 %s: %w", pid, signalErr)
		}
	}
	for attempt := 0; attempt < 20; attempt++ {
		time.Sleep(100 * time.Millisecond)
		listener, err = net.Listen("tcp", address)
		if err == nil {
			fmt.Fprintf(os.Stderr, "已终止占用进程，继续使用端口 %s 启动\n", address)
			return listener, nil
		}
	}
	return nil, fmt.Errorf("端口 %s 的占用进程未在超时时间内退出", address)
}

func isInteractiveInput() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func portListenerPIDs(address string) ([]string, error) {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("端口地址无效: %s", address)
	}
	output, err := exec.Command("lsof", "-tiTCP:"+port, "-sTCP:LISTEN").Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("无法定位端口 %s 的占用进程: %w", address, err)
	}
	seen := map[string]bool{}
	result := []string{}
	for _, value := range strings.Fields(string(output)) {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result, nil
}

// instanceGateway is the only public cross-node route. It resolves the random
// route key in the control-plane database, then proxies to that instance's
// Agent. The Agent itself still resolves the Docker container from its label.
func instanceGateway(c *gin.Context) {
	route := strings.TrimSpace(c.GetHeader("X-Route-Key"))
	if !regexpRouteKey(route) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "实例路由无效"})
		return
	}
	var nodeID string
	if err := instanceDB.QueryRowContext(c.Request.Context(), `SELECT node_id FROM xcloud_instances WHERE route_key=? AND status IN ('running','deploying')`, route).Scan(&nodeID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "实例不可用"})
		return
	}
	n, err := nodeByID(c.Request.Context(), nodeID)
	if err != nil || !n.Enabled {
		c.JSON(http.StatusBadGateway, gin.H{"message": "实例节点不可用"})
		return
	}
	target, err := url.Parse(strings.TrimRight(n.AgentURL, "/"))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"message": "节点地址无效"})
		return
	}
	if original := c.GetHeader("X-Forwarded-Uri"); strings.HasPrefix(original, "/") {
		c.Request.URL.Path = original
		c.Request.URL.RawQuery = ""
	}
	c.Request.Header.Set("X-Route-Key", route)
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, e error) {
		log.Printf("instance gateway %s: %v", route, e)
		http.Error(w, "实例暂不可用", http.StatusBadGateway)
	}
	proxy.ServeHTTP(c.Writer, c.Request)
}
func regexpRouteKey(value string) bool {
	if len(value) != 17 || value[0] != 'r' {
		return false
	}
	for _, v := range value[1:] {
		if !(v >= '0' && v <= '9' || v >= 'a' && v <= 'f') {
			return false
		}
	}
	return true
}

// listenAddress accepts both common port-only forms ("8082" and ":8082")
// as well as complete addresses such as "127.0.0.1:8082".
func listenAddress(value string) string {
	address := strings.TrimSpace(value)
	if address == "" {
		return ":8082"
	}
	if _, err := strconv.ParseUint(address, 10, 16); err == nil {
		return ":" + address
	}
	return address
}

func oidcAuthorize(c *gin.Context) {
	if !oidcConfigured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "统一认证尚未配置"})
		return
	}
	var body struct {
		RedirectURL string `json:"redirect_uri" binding:"required"`
	}
	if c.ShouldBindJSON(&body) != nil || body.RedirectURL != env("AUTH_OIDC_REDIRECT_URL", "") {
		c.JSON(http.StatusBadRequest, gin.H{"message": "回调地址无效"})
		return
	}
	state, err := randomToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "无法初始化登录"})
		return
	}
	verifier, err := randomToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "无法初始化登录"})
		return
	}
	challenge := sha256.Sum256([]byte(verifier))
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(oidcStateCookieName, state, 600, "/", "", cookieSecure(), true)
	c.SetCookie(oidcVerifierCookieName, verifier, 600, "/", "", cookieSecure(), true)
	query := url.Values{"response_type": {"code"}, "client_id": {env("AUTH_OIDC_CLIENT_ID", "")}, "redirect_uri": {body.RedirectURL}, "scope": {"openid profile email"}, "state": {state}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}, "code_challenge_method": {"S256"}}
	c.JSON(http.StatusOK, gin.H{"authorizeURL": strings.TrimRight(env("AUTH_OIDC_ISSUER", ""), "/") + "/oauth/authorize?" + query.Encode()})
}

func oidcCallback(c *gin.Context) {
	var body struct {
		Code  string `json:"code" binding:"required"`
		State string `json:"state" binding:"required"`
	}
	if c.ShouldBindJSON(&body) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "授权响应无效"})
		return
	}
	state, err := c.Cookie(oidcStateCookieName)
	if err != nil || state == "" || state != body.State {
		c.JSON(http.StatusForbidden, gin.H{"message": "登录请求已失效，请重新发起登录"})
		return
	}
	verifier, err := c.Cookie(oidcVerifierCookieName)
	if err != nil || verifier == "" {
		c.JSON(http.StatusForbidden, gin.H{"message": "登录请求已失效，请重新发起登录"})
		return
	}
	c.SetCookie(oidcStateCookieName, "", -1, "/", "", cookieSecure(), true)
	c.SetCookie(oidcVerifierCookieName, "", -1, "/", "", cookieSecure(), true)
	user, err := exchangeOIDCCode(body.Code, verifier)
	if err != nil {
		log.Printf("OIDC callback failed: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{"message": "统一认证失败"})
		return
	}
	if err := syncCloudUser(c.Request.Context(), user); err != nil {
		log.Printf("sync cloud user: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "用户目录暂不可用"})
		return
	}
	sid, err := randomToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "无法创建会话"})
		return
	}
	if err := storeSession(sid, session{User: user, ExpiresAt: time.Now().Add(24 * time.Hour)}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "无法保存会话"})
		return
	}
	c.SetCookie(sessionCookieName, sid, 86400, "/", "", cookieSecure(), true)
	c.JSON(http.StatusOK, gin.H{"user": user})
}

func exchangeOIDCCode(code, verifier string) (oidcUser, error) {
	issuer := strings.TrimRight(env("AUTH_OIDC_ISSUER", ""), "/")
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {env("AUTH_OIDC_CLIENT_ID", "")}, "redirect_uri": {env("AUTH_OIDC_REDIRECT_URL", "")}, "code_verifier": {verifier}}
	if secret := env("AUTH_OIDC_CLIENT_SECRET", ""); secret != "" {
		form.Set("client_secret", secret)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.PostForm(issuer+"/oauth/token", form)
	if err != nil {
		return oidcUser{}, err
	}
	defer response.Body.Close()
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&token) != nil || token.AccessToken == "" {
		return oidcUser{}, fmt.Errorf("OIDC token exchange rejected")
	}
	request, err := http.NewRequest(http.MethodGet, issuer+"/oauth/userinfo", nil)
	if err != nil {
		return oidcUser{}, err
	}
	request.Header.Set("Authorization", "Bearer "+token.AccessToken)
	response, err = client.Do(request)
	if err != nil {
		return oidcUser{}, err
	}
	defer response.Body.Close()
	var profile struct {
		Subject           string   `json:"sub"`
		Name              string   `json:"name"`
		PreferredUsername string   `json:"preferred_username"`
		Email             string   `json:"email"`
		Picture           string   `json:"picture"`
		Roles             []string `json:"roles"`
		Permissions       []string `json:"permissions"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&profile) != nil || profile.Subject == "" {
		return oidcUser{}, fmt.Errorf("OIDC userinfo rejected")
	}
	username := profile.PreferredUsername
	if username == "" {
		username = profile.Name
	}
	return oidcUser{ID: profile.Subject, Username: username, Email: profile.Email, Avatar: profile.Picture, Roles: profile.Roles, Permissions: profile.Permissions, IsAdmin: hasPermission(profile.Permissions, env("AUTH_ADMIN_PERMISSION", "cloud:admin"))}, nil
}

func requireSession(c *gin.Context) {
	value, ok := currentSession(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
		c.Abort()
		return
	}
	c.Set("user", value.User)
	c.Next()
}
func requireAdmin(c *gin.Context) {
	value, ok := currentSession(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
		c.Abort()
		return
	}
	if !value.User.IsAdmin {
		c.JSON(http.StatusForbidden, gin.H{"message": "需要平台管理员权限"})
		c.Abort()
		return
	}
	if instanceDB == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "平台数据服务尚未就绪，请配置 MYSQL_DSN 后重启服务"})
		c.Abort()
		return
	}
	c.Set("user", value.User)
	c.Next()
}
func currentSession(c *gin.Context) (session, bool) {
	sid, err := c.Cookie(sessionCookieName)
	if err != nil || sid == "" {
		return session{}, false
	}
	value, ok := loadSession(sid)
	if !ok || time.Now().After(value.ExpiresAt) {
		if ok {
			deleteSession(sid)
		}
		return session{}, false
	}
	return value, true
}
func sessionInfo(c *gin.Context) {
	value, ok := currentSession(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"authenticated": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"authenticated": true, "user": value.User})
}
func logout(c *gin.Context) {
	sid, _ := c.Cookie(sessionCookieName)
	if sid != "" {
		deleteSession(sid)
	}
	c.SetCookie(sessionCookieName, "", -1, "/", "", cookieSecure(), true)
	c.Status(http.StatusNoContent)
}
func devLogin(c *gin.Context) {
	user := oidcUser{ID: "dev-super-admin", Username: "开发超级管理员", Email: "dev-admin@localhost", Roles: []string{"cloud-admin"}, Permissions: []string{"*"}, IsAdmin: true}
	if instanceDB != nil {
		_ = syncCloudUser(c.Request.Context(), user)
	}
	sid, err := randomToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "无法创建开发会话"})
		return
	}
	if err := storeSession(sid, session{User: user, ExpiresAt: time.Now().Add(8 * time.Hour)}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "无法保存开发会话"})
		return
	}
	c.SetCookie(sessionCookieName, sid, 8*60*60, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"user": user})
}
func listInstances(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	items, err := listStoredInstances(c.Request.Context(), user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "读取实例失败"})
		return
	}
	c.JSON(http.StatusOK, items)
}
func createInstance(c *gin.Context) {
	// Instances are created only after an administrator confirms a catalog order.
	// Keeping this legacy path would let callers submit arbitrary image references
	// and bypass both the product catalog and capacity reservation.
	c.JSON(http.StatusGone, gin.H{"message": "请通过订单中心创建实例"})
	return
	/*
		var body struct {
			Name, Image, Version, Spec string
			CPU                        float64 `json:"cpu"`
			MemoryMB                   int     `json:"memoryMB"`
		}
		if c.ShouldBindJSON(&body) != nil || strings.TrimSpace(body.Name) == "" || strings.TrimSpace(body.Image) == "" || strings.TrimSpace(body.Version) == "" || strings.TrimSpace(body.Spec) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"message": "实例参数无效"})
			return
		}
		id, err := randomToken()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "无法创建实例"})
			return
		}
		if body.CPU <= 0 {
			body.CPU = 2
		}
		if body.MemoryMB <= 0 {
			body.MemoryMB = 4096
		}
		user := c.MustGet("user").(oidcUser)
		digest := sha256.Sum256([]byte(id))
		containerName := fmt.Sprintf("xcloud-%x", digest[:6])
		route := routeKey(user.ID + "\x00" + id)
		accessAddress := "https://xcloud-" + route + "." + env("XCLOUD_INSTANCE_DOMAIN", "alemonjs.com")
		item := instance{ID: id, Name: body.Name, Image: body.Image, Version: body.Version, Spec: body.Spec, Status: "等待节点接入", IP: accessAddress, CreatedAt: time.Now().Format("2006-01-02 15:04"), ContainerName: containerName, OwnerID: user.ID}
		if agentConfigured() {
			if err := callAgent(c.Request.Context(), http.MethodPost, "/container/create", gin.H{"name": containerName, "image": imageReference(body.Image, body.Version), "cpu": body.CPU, "memoryMB": body.MemoryMB, "route": route}); err != nil {
				log.Printf("create instance %s: %v", id, err)
				c.JSON(http.StatusBadGateway, gin.H{"message": "裸机节点暂不可用，实例未创建"})
				return
			}
			item.Status = "运行中"
		}
		if err := saveStoredInstance(c.Request.Context(), item); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "保存实例失败"})
			return
		}
		c.JSON(http.StatusAccepted, item)
	*/
}
func ownedInstance(c *gin.Context) (instance, bool) {
	user := c.MustGet("user").(oidcUser)
	id := c.Param("id")
	item, ok, err := getStoredInstance(c.Request.Context(), id, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "读取实例失败"})
		return instance{}, false
	}
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"message": "实例不存在"})
		return instance{}, false
	}
	return item, true
}
func routeKey(value string) string {
	digest := sha256.Sum256([]byte(value))
	return fmt.Sprintf("r%x", digest[:8])
}
func oidcConfigured() bool {
	return env("AUTH_OIDC_ISSUER", "") != "" && env("AUTH_OIDC_CLIENT_ID", "") != "" && env("AUTH_OIDC_REDIRECT_URL", "") != ""
}
func hasPermission(permissions []string, wanted string) bool {
	for _, permission := range permissions {
		if permission == wanted || permission == "*" {
			return true
		}
	}
	return false
}
func initSessionStore() error {
	redisURL := env("SESSION_REDIS_URL", "")
	if redisURL == "" {
		log.Printf("session store: in-memory (development only)")
		return nil
	}
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		return fmt.Errorf("parse Redis URL: %w", err)
	}
	client := redis.NewClient(options)
	if err := client.Ping(context.Background()).Err(); err != nil {
		return fmt.Errorf("ping Redis: %w", err)
	}
	sessionRedis = client
	log.Printf("session store: Redis")
	return nil
}
func storeSession(sid string, value session) error {
	if sessionRedis != nil {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if err := sessionRedis.Set(context.Background(), sessionPrefix+sid, data, time.Until(value.ExpiresAt)).Err(); err == nil {
			return nil
		} else if env("GIN_MODE", gin.DebugMode) == gin.ReleaseMode {
			return err
		} else {
			log.Printf("session store Redis write failed; using in-memory development fallback: %v", err)
		}
	}
	sessionsMu.Lock()
	sessions[sid] = value
	sessionsMu.Unlock()
	return nil
}
func loadSession(sid string) (session, bool) {
	if sessionRedis != nil {
		data, err := sessionRedis.Get(context.Background(), sessionPrefix+sid).Bytes()
		if err == nil {
			var value session
			if json.Unmarshal(data, &value) == nil {
				return value, true
			}
		}
		if env("GIN_MODE", gin.DebugMode) == gin.ReleaseMode {
			return session{}, false
		}
	}
	sessionsMu.RLock()
	value, ok := sessions[sid]
	sessionsMu.RUnlock()
	return value, ok
}
func deleteSession(sid string) {
	if sessionRedis != nil {
		_ = sessionRedis.Del(context.Background(), sessionPrefix+sid).Err()
	}
	sessionsMu.Lock()
	delete(sessions, sid)
	sessionsMu.Unlock()
}
func cookieSecure() bool { return strings.HasPrefix(env("PUBLIC_URL", ""), "https://") }
func randomToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}
func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	value, err := strconv.Atoi(raw)
	if err != nil || raw == "" {
		return fallback
	}
	return value
}

func validateProductionConfig() {
	for _, key := range []string{"MYSQL_DSN", "SESSION_REDIS_URL", "RABBITMQ_URL", "AUTH_OIDC_ISSUER", "AUTH_OIDC_CLIENT_ID", "AUTH_OIDC_REDIRECT_URL", "XCLOUD_NODE_TOKEN_ENCRYPTION_KEY", "XCLOUD_COUPON_CODE_SECRET", "XCLOUD_METRICS_TOKEN", "XCLOUD_INSTANCE_DOMAIN", "XCLOUD_INSTANCE_DATA_ROOT"} {
		if strings.TrimSpace(os.Getenv(key)) == "" {
			log.Fatalf("production configuration missing: %s", key)
		}
	}
}
