package cloud

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
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
	ID, Username, Email, Avatar string
	Roles, Permissions          []string
	IsAdmin                     bool `json:"isAdmin"`
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
	initSessionStore()
	initInstanceStore()
	if err := initTaskQueue(); err != nil {
		if mode == gin.ReleaseMode {
			log.Fatalf("RabbitMQ unavailable: %v", err)
		}
		log.Printf("task queue unavailable: %v", err)
	} else {
		consumeTasks()
	}
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	router.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok", "version": Version}) })
	router.GET("/api/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"message": "pong"}) })
	router.GET("/api/instances", requireSession, listInstances)
	router.POST("/api/instances", requireSession, createInstance)
	router.POST("/api/instances/:id/:action", requireSession, instanceAction)
	router.DELETE("/api/instances/:id", requireSession, deleteInstance)
	router.GET("/api/admin/nodes", requireAdmin, listNodes)
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
	log.Printf("AlemonX Cloud %s listening on %s", Version, address)
	if err := router.Run(address); err != nil {
		log.Fatal(err)
	}
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
}
func instanceAction(c *gin.Context) {
	item, ok := ownedInstance(c)
	if !ok {
		return
	}
	action := c.Param("action")
	if action != "start" && action != "stop" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "不支持的实例操作"})
		return
	}
	if !agentConfigured() {
		c.JSON(http.StatusConflict, gin.H{"message": "尚未接入裸机节点"})
		return
	}
	if err := callAgent(c.Request.Context(), http.MethodPost, "/container/"+item.ContainerName+"/"+action, nil); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"message": "节点操作失败"})
		return
	}
	if action == "start" {
		item.Status = "运行中"
	} else {
		item.Status = "已停止"
	}
	if err := saveStoredInstance(c.Request.Context(), item); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "保存实例状态失败"})
		return
	}
	c.JSON(http.StatusOK, item)
}
func deleteInstance(c *gin.Context) {
	item, ok := ownedInstance(c)
	if !ok {
		return
	}
	if agentConfigured() && callAgent(c.Request.Context(), http.MethodDelete, "/container/"+item.ContainerName, nil) != nil {
		c.JSON(http.StatusBadGateway, gin.H{"message": "节点删除失败"})
		return
	}
	if err := removeStoredInstance(c.Request.Context(), item.ID, item.OwnerID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "删除实例记录失败"})
		return
	}
	c.Status(http.StatusNoContent)
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
func listNodes(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"nodes": []gin.H{{"name": "baremetal-sh-01", "status": "online", "cpuAvailable": 28, "memoryAvailableGB": 92}}})
}
func agentConfigured() bool {
	return env("XCLOUD_AGENT_URL", "") != "" && env("XCLOUD_AGENT_TOKEN", "") != ""
}
func imageReference(image, version string) string {
	if strings.Contains(image, "@") || strings.Contains(strings.TrimPrefix(image[strings.LastIndex(image, "/")+1:], "/"), ":") {
		return image
	}
	return image + ":" + version
}
func routeKey(value string) string {
	digest := sha256.Sum256([]byte(value))
	return fmt.Sprintf("r%x", digest[:8])
}
func callAgent(ctx context.Context, method, path string, payload any) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = strings.NewReader(string(data))
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(env("XCLOUD_AGENT_URL", ""), "/")+path, body)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+env("XCLOUD_AGENT_TOKEN", ""))
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 3 * time.Minute}).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("agent responded %d", response.StatusCode)
	}
	return nil
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
func initSessionStore() {
	redisURL := env("SESSION_REDIS_URL", "")
	if redisURL == "" {
		log.Printf("session store: in-memory (development only)")
		return
	}
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Printf("invalid SESSION_REDIS_URL; using in-memory session store")
		return
	}
	client := redis.NewClient(options)
	if err := client.Ping(context.Background()).Err(); err != nil {
		log.Printf("Redis unavailable; using in-memory session store: %v", err)
		return
	}
	sessionRedis = client
	log.Printf("session store: Redis")
}
func storeSession(sid string, value session) error {
	if sessionRedis != nil {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return sessionRedis.Set(context.Background(), sessionPrefix+sid, data, time.Until(value.ExpiresAt)).Err()
	}
	sessionsMu.Lock()
	sessions[sid] = value
	sessionsMu.Unlock()
	return nil
}
func loadSession(sid string) (session, bool) {
	if sessionRedis != nil {
		data, err := sessionRedis.Get(context.Background(), sessionPrefix+sid).Bytes()
		if err != nil {
			return session{}, false
		}
		var value session
		if json.Unmarshal(data, &value) != nil {
			return session{}, false
		}
		return value, true
	}
	sessionsMu.RLock()
	value, ok := sessions[sid]
	sessionsMu.RUnlock()
	return value, ok
}
func deleteSession(sid string) {
	if sessionRedis != nil {
		_ = sessionRedis.Del(context.Background(), sessionPrefix+sid).Err()
		return
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

func validateProductionConfig() {
	for _, key := range []string{"MYSQL_DSN", "SESSION_REDIS_URL", "RABBITMQ_URL", "AUTH_OIDC_ISSUER", "AUTH_OIDC_CLIENT_ID", "AUTH_OIDC_REDIRECT_URL", "XCLOUD_AGENT_URL", "XCLOUD_AGENT_TOKEN", "XCLOUD_INSTANCE_DOMAIN", "XCLOUD_INSTANCE_DATA_ROOT"} {
		if strings.TrimSpace(os.Getenv(key)) == "" {
			log.Fatalf("production configuration missing: %s", key)
		}
	}
}
