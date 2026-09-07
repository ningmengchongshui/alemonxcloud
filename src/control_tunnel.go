package cloud

// This file implements the small, deliberately separate reverse-tunnel plane
// used by user-owned xcloud-control clients. It never grants Docker or shell
// access to a customer machine.

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const controlProtocolVersion = "xcloud-control.v1"
const controlBandwidthBytesPerSecond = 10 * 1000 * 1000 / 8

type controlFrame struct {
	Type       string      `json:"type"`
	ID         string      `json:"id,omitempty"`
	Method     string      `json:"method,omitempty"`
	Path       string      `json:"path,omitempty"`
	Headers    http.Header `json:"headers,omitempty"`
	Body       []byte      `json:"body,omitempty"`
	Status     int         `json:"status,omitempty"`
	TargetURL  string      `json:"targetURL,omitempty"`
	Protocol   string      `json:"protocol,omitempty"`
	Error      string      `json:"error,omitempty"`
	DeviceName string      `json:"deviceName,omitempty"`
}

type controlRoute struct {
	ID            string     `json:"id"`
	DeviceID      string     `json:"deviceId"`
	OwnerID       string     `json:"-"`
	RouteKey      string     `json:"routeKey"`
	TargetURL     string     `json:"targetURL"`
	Status        string     `json:"status"`
	AccessAddress string     `json:"accessAddress"`
	DeviceName    string     `json:"deviceName"`
	ClientVersion string     `json:"clientVersion"`
	LastHeartbeat *time.Time `json:"lastHeartbeatAt,omitempty"`
}

type controlSession struct {
	deviceID string
	conn     *websocket.Conn
	mu       sync.Mutex
	streams  map[string]chan controlFrame
	streamMu sync.Mutex
	limitMu  sync.Mutex
	nextByte time.Time
}

var controlSessions = struct {
	sync.RWMutex
	byDevice map[string]*controlSession
}{byDevice: map[string]*controlSession{}}

var controlUpgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

func controlHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// Enrollment tokens are created by an already authenticated xCloud user and
// copied to a server configuration file. The plaintext is deliberately
// returned only once; the database keeps only its SHA-256 hash.
func controlEnrollmentTokenCreate(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	token, err := randomToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "无法生成接入令牌"})
		return
	}
	id := newID("cet")
	value := "xctl_enroll_" + token
	_, err = instanceDB.ExecContext(c.Request.Context(), `INSERT INTO xcloud_control_enrollment_tokens (id,owner_id,token_hash,expires_at,created_at) VALUES (?,?,?,DATE_ADD(NOW(),INTERVAL 10 MINUTE),NOW())`, id, user.ID, controlHash(value))
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "服务暂不可用，请稍后重试"})
		return
	}
	_ = writeAudit(c.Request.Context(), user.ID, "control.enrollment_token.create", "control_enrollment_token", id, nil)
	c.JSON(http.StatusCreated, gin.H{"id": id, "token": value, "expiresIn": 600, "expiresAt": time.Now().Add(10 * time.Minute)})
}
func controlEnrollmentTokens(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	rows, err := instanceDB.QueryContext(c.Request.Context(), `SELECT id,expires_at,used_at,revoked_at,COALESCE(device_id,'') FROM xcloud_control_enrollment_tokens WHERE owner_id=? ORDER BY created_at DESC LIMIT 20`, user.ID)
	if err != nil {
		c.JSON(503, gin.H{"message": "服务暂不可用，请稍后重试"})
		return
	}
	defer rows.Close()
	items := []gin.H{}
	for rows.Next() {
		var id, device string
		var expires time.Time
		var used, revoked sql.NullTime
		if rows.Scan(&id, &expires, &used, &revoked, &device) != nil {
			continue
		}
		status := "pending"
		if revoked.Valid {
			status = "revoked"
		} else if used.Valid {
			status = "used"
		} else if !expires.After(time.Now()) {
			status = "expired"
		}
		items = append(items, gin.H{"id": id, "status": status, "expiresAt": expires, "deviceId": device})
	}
	c.JSON(200, items)
}
func controlEnrollmentTokenRevoke(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	result, err := instanceDB.ExecContext(c.Request.Context(), `UPDATE xcloud_control_enrollment_tokens SET revoked_at=NOW() WHERE id=? AND owner_id=? AND used_at IS NULL AND revoked_at IS NULL`, c.Param("id"), user.ID)
	if err != nil {
		c.JSON(503, gin.H{"message": "服务暂不可用，请稍后重试"})
		return
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		c.JSON(404, gin.H{"message": "接入令牌不存在或无法撤销"})
		return
	}
	_ = writeAudit(c.Request.Context(), user.ID, "control.enrollment_token.revoke", "control_enrollment_token", c.Param("id"), nil)
	c.Status(204)
}

func controlAuthorizationCreate(c *gin.Context) {
	var body struct{ Name, PublicKey, ClientVersion string }
	if c.ShouldBindJSON(&body) != nil || strings.TrimSpace(body.Name) == "" || len(body.Name) > 96 || strings.TrimSpace(body.PublicKey) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "设备名称或公钥无效"})
		return
	}
	poll, err := randomToken()
	if err != nil {
		c.JSON(500, gin.H{"message": "无法创建设备授权"})
		return
	}
	id := newID("ca")
	_, err = instanceDB.ExecContext(c.Request.Context(), `INSERT INTO xcloud_control_authorizations (id,name,public_key,client_version,poll_token_hash,status,expires_at,created_at) VALUES (?,?,?,?,?,'pending',DATE_ADD(NOW(),INTERVAL 10 MINUTE),NOW())`, id, strings.TrimSpace(body.Name), strings.TrimSpace(body.PublicKey), strings.TrimSpace(body.ClientVersion), controlHash(poll))
	if err != nil {
		c.JSON(503, gin.H{"message": "服务暂不可用，请稍后重试"})
		return
	}
	base := strings.TrimRight(env("PUBLIC_URL", "http://localhost:8082"), "/")
	c.JSON(http.StatusCreated, gin.H{"id": id, "pollToken": poll, "expiresIn": 600, "authorizeURL": base + "/api/control/authorizations/" + id + "/approve?token=" + url.QueryEscape(poll)})
}

func controlAuthorizationPage(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "缺少设备授权码"})
		return
	}
	csrf, err := randomToken()
	if err != nil {
		c.JSON(500, gin.H{"message": "无法创建设备确认"})
		return
	}
	c.SetCookie("xcloud_control_pair_csrf", csrf, 600, "/api/control/authorizations/", "", cookieSecure(), true)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte("<form method=\"post\"><input type=\"hidden\" name=\"token\" value=\""+template.HTMLEscapeString(token)+"\"><input type=\"hidden\" name=\"csrf\" value=\""+template.HTMLEscapeString(csrf)+"\"><h3>确认绑定 xcloud-control 设备？</h3><button type=\"submit\">确认绑定</button></form>"))
}

func controlAuthorizationApprove(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	id, token := c.Param("id"), c.PostForm("token")
	csrf, cookieErr := c.Cookie("xcloud_control_pair_csrf")
	if token == "" || cookieErr != nil || csrf == "" || subtle.ConstantTimeCompare([]byte(csrf), []byte(c.PostForm("csrf"))) != 1 {
		c.JSON(400, gin.H{"message": "缺少设备授权码"})
		return
	}
	c.SetCookie("xcloud_control_pair_csrf", "", -1, "/api/control/authorizations/", "", cookieSecure(), true)
	tx, err := instanceDB.BeginTx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(503, gin.H{"message": "服务暂不可用，请稍后重试"})
		return
	}
	defer tx.Rollback()
	// Lock the user directory row so parallel browser confirmations cannot bypass the one-device quota.
	var lockedUser string
	if err = tx.QueryRowContext(c.Request.Context(), `SELECT id FROM xcloud_users WHERE id=? FOR UPDATE`, user.ID).Scan(&lockedUser); err != nil {
		c.JSON(409, gin.H{"message": "请先登录 xCloud 后再绑定设备"})
		return
	}
	var name, pub, version, status string
	var expires time.Time
	err = tx.QueryRowContext(c.Request.Context(), `SELECT name,public_key,client_version,status,expires_at FROM xcloud_control_authorizations WHERE id=? AND poll_token_hash=? FOR UPDATE`, id, controlHash(token)).Scan(&name, &pub, &version, &status, &expires)
	if err != nil || status != "pending" || !expires.After(time.Now()) {
		c.JSON(410, gin.H{"message": "设备授权已失效或已完成"})
		return
	}
	var count int
	if err = tx.QueryRowContext(c.Request.Context(), `SELECT COUNT(*) FROM xcloud_control_devices WHERE owner_id=? AND status='enabled'`, user.ID).Scan(&count); err != nil {
		c.JSON(503, gin.H{"message": "服务暂不可用，请稍后重试"})
		return
	}
	if count > 0 {
		c.JSON(409, gin.H{"message": "每个用户仅可启用一台 xcloud-control，请先撤销旧设备"})
		return
	}
	credential, err := randomToken()
	if err != nil {
		c.JSON(500, gin.H{"message": "无法签发设备凭证"})
		return
	}
	deviceID, routeID := newID("ctl"), newID("ctr")
	route := routeKey(user.ID + "\x00" + deviceID)
	address := "https://control-" + route + "." + env("XCLOUD_INSTANCE_DOMAIN", "alemonjs.com")
	if _, err = tx.ExecContext(c.Request.Context(), `INSERT INTO xcloud_control_devices (id,owner_id,name,public_key,credential_hash,status,client_version,key_version,credential_version,rebind_required,created_at,updated_at) VALUES (?,?,?,?,?,'enabled',?,3,1,FALSE,NOW(),NOW())`, deviceID, user.ID, name, pub, controlHash(credential), version); err != nil {
		c.JSON(503, gin.H{"message": "无法保存设备"})
		return
	}
	if _, err = tx.ExecContext(c.Request.Context(), `INSERT INTO xcloud_control_routes (id,owner_id,device_id,route_key,target_url,status,access_address,created_at,updated_at) VALUES (?,?,?,?,?,'paused',?,NOW(),NOW())`, routeID, user.ID, deviceID, route, "", address); err != nil {
		c.JSON(503, gin.H{"message": "无法创建自建入口"})
		return
	}
	if _, err = tx.ExecContext(c.Request.Context(), `UPDATE xcloud_control_authorizations SET status='approved',owner_id=?,device_id=?,issued_credential=? WHERE id=?`, user.ID, deviceID, credential, id); err != nil {
		c.JSON(503, gin.H{"message": "无法完成设备绑定"})
		return
	}
	if err = writeAuditTx(c.Request.Context(), tx, user.ID, "control.device.approve", "control_device", deviceID, map[string]any{"name": name}); err != nil {
		c.JSON(503, gin.H{"message": "无法完成设备绑定"})
		return
	}
	if err = tx.Commit(); err != nil {
		c.JSON(503, gin.H{"message": "无法完成设备绑定"})
		return
	}
	_ = createNotification(c.Request.Context(), user.ID, "control_device", "自建设备已绑定", "xcloud-control 已完成绑定，请在控制台配置本地服务。", map[string]any{"deviceId": deviceID})
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte("<h3>xCloud 设备绑定成功</h3><p>请返回服务器终端，客户端将自动保存凭证。</p>"))
}

func controlAuthorizationPoll(c *gin.Context) {
	id, token := c.Param("id"), c.GetHeader("X-Control-Poll-Token")
	if token == "" {
		token = c.Query("token")
	}
	tx, err := instanceDB.BeginTx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(503, gin.H{"message": "服务暂不可用，请稍后重试"})
		return
	}
	defer tx.Rollback()
	var status, credential, deviceID string
	var expires time.Time
	var delivered sql.NullTime
	err = tx.QueryRowContext(c.Request.Context(), `SELECT status,COALESCE(issued_credential,''),COALESCE(device_id,''),expires_at,delivered_at FROM xcloud_control_authorizations WHERE id=? AND poll_token_hash=? FOR UPDATE`, id, controlHash(token)).Scan(&status, &credential, &deviceID, &expires, &delivered)
	if err != nil || !expires.After(time.Now()) {
		c.JSON(410, gin.H{"message": "设备授权已失效"})
		return
	}
	if status != "approved" {
		_ = tx.Commit()
		c.JSON(200, gin.H{"status": status})
		return
	}
	if delivered.Valid || credential == "" {
		c.JSON(410, gin.H{"message": "设备凭证已领取"})
		return
	}
	if _, err = tx.ExecContext(c.Request.Context(), `UPDATE xcloud_control_authorizations SET delivered_at=NOW(),issued_credential=NULL WHERE id=?`, id); err != nil || tx.Commit() != nil {
		c.JSON(503, gin.H{"message": "无法领取设备凭证"})
		return
	}
	c.JSON(200, gin.H{"status": "approved", "deviceID": deviceID, "credential": credential, "protocol": "xcloud-control.v3", "tunnelURL": strings.TrimRight(env("XCLOUD_TUNNEL_URL", env("PUBLIC_URL", "http://localhost:8082")), "/")})
}

func controlSelfHosted(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	var r controlRoute
	var heartbeat sql.NullTime
	err := instanceDB.QueryRowContext(c.Request.Context(), `SELECT r.id,r.device_id,r.owner_id,r.route_key,r.target_url,r.status,r.access_address,d.name,d.client_version,d.last_heartbeat_at FROM xcloud_control_routes r JOIN xcloud_control_devices d ON d.id=r.device_id WHERE r.owner_id=? AND d.status='enabled' LIMIT 1`, user.ID).Scan(&r.ID, &r.DeviceID, &r.OwnerID, &r.RouteKey, &r.TargetURL, &r.Status, &r.AccessAddress, &r.DeviceName, &r.ClientVersion, &heartbeat)
	if err == sql.ErrNoRows {
		c.JSON(200, gin.H{"device": nil, "route": nil, "online": false, "bandwidthMbps": 10})
		return
	}
	if err != nil {
		c.JSON(503, gin.H{"message": "服务暂不可用，请稍后重试"})
		return
	}
	if heartbeat.Valid {
		r.LastHeartbeat = &heartbeat.Time
	}
	online := controlSessionFor(r.DeviceID) != nil
	c.JSON(200, gin.H{"device": gin.H{"id": r.DeviceID, "name": r.DeviceName, "version": r.ClientVersion, "lastHeartbeatAt": r.LastHeartbeat}, "route": r, "online": online, "bandwidthMbps": 10})
}

func validControlTarget(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "http" || u.Port() == "" || u.Path != "" && u.Path != "/" || (u.Hostname() != "127.0.0.1" && strings.ToLower(u.Hostname()) != "localhost") {
		return "", fmt.Errorf("本地目标仅支持 http://127.0.0.1:端口 或 http://localhost:端口")
	}
	return strings.TrimRight(u.String(), "/"), nil
}
func controlRouteUpdate(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{"message": "旧本地目标反代已下线，请使用新版 xcloud-control 创建受管实例"})
	return
	/*
		user := c.MustGet("user").(oidcUser)
		var body struct {
			TargetURL string `json:"targetURL"`
			Enabled   bool   `json:"enabled"`
		}
		if c.ShouldBindJSON(&body) != nil {
			c.JSON(400, gin.H{"message": "请求无效"})
			return
		}
		target, err := validControlTarget(body.TargetURL)
		if err != nil {
			c.JSON(400, gin.H{"message": err.Error()})
			return
		}
		status := "paused"
		if body.Enabled {
			status = "enabled"
		}
		result, err := instanceDB.ExecContext(c.Request.Context(), `UPDATE xcloud_control_routes r JOIN xcloud_control_devices d ON d.id=r.device_id SET r.target_url=?,r.status=?,r.updated_at=NOW() WHERE r.owner_id=? AND d.status='enabled'`, target, status, user.ID)
		if err != nil {
			c.JSON(503, gin.H{"message": "服务暂不可用，请稍后重试"})
			return
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			c.JSON(404, gin.H{"message": "请先绑定 xcloud-control 设备"})
			return
		}
		_ = writeAudit(c.Request.Context(), user.ID, "control.route.update", "control_route", user.ID, map[string]any{"enabled": body.Enabled})
		controlPushConfig(user.ID)
		c.Status(http.StatusNoContent) */
}
func controlRouteAction(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{"message": "旧本地目标反代已下线，请使用自建节点实例管理"})
	return
	/*
		user := c.MustGet("user").(oidcUser)
		action := c.Param("action")
		status := ""
		if action == "pause" {
			status = "paused"
		}
		if action == "resume" {
			status = "enabled"
		}
		if status == "" {
			c.JSON(404, gin.H{"message": "操作不存在"})
			return
		}
		result, err := instanceDB.ExecContext(c.Request.Context(), `UPDATE xcloud_control_routes SET status=?,updated_at=NOW() WHERE owner_id=? AND target_url<>''`, status, user.ID)
		if err != nil {
			c.JSON(503, gin.H{"message": "服务暂不可用，请稍后重试"})
			return
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			c.JSON(409, gin.H{"message": "请先配置本地服务"})
			return
		}
		_ = writeAudit(c.Request.Context(), user.ID, "control.route."+action, "control_route", user.ID, nil)
		controlPushConfig(user.ID)
		c.Status(204) */
}
func controlDeviceRevoke(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	tx, err := instanceDB.BeginTx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(503, gin.H{"message": "服务暂不可用，请稍后重试"})
		return
	}
	defer tx.Rollback()
	var id string
	if err = tx.QueryRowContext(c.Request.Context(), `SELECT id FROM xcloud_control_devices WHERE owner_id=? AND status='enabled' FOR UPDATE`, user.ID).Scan(&id); err == sql.ErrNoRows {
		c.JSON(404, gin.H{"message": "未绑定自建设备"})
		return
	}
	if err != nil {
		c.JSON(503, gin.H{"message": "服务暂不可用，请稍后重试"})
		return
	}
	_, err = tx.ExecContext(c.Request.Context(), `UPDATE xcloud_control_devices SET status='revoked',revoked_at=NOW(),updated_at=NOW() WHERE id=?`, id)
	if err == nil {
		_, err = tx.ExecContext(c.Request.Context(), `UPDATE xcloud_control_routes SET status='disabled',updated_at=NOW() WHERE device_id=?`, id)
	}
	if err == nil {
		err = writeAuditTx(c.Request.Context(), tx, user.ID, "control.device.revoke", "control_device", id, nil)
	}
	if err != nil || tx.Commit() != nil {
		c.JSON(503, gin.H{"message": "撤销失败，请稍后重试"})
		return
	}
	controlRemoveSession(id)
	_ = createNotification(c.Request.Context(), user.ID, "control_device", "自建设备已撤销", "该设备和公网入口已停止访问。", map[string]any{"deviceId": id})
	c.Status(204)
}

func controlConnect(c *gin.Context) {
	token := strings.TrimSpace(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))
	if token == "" {
		c.JSON(401, gin.H{"message": "缺少设备凭证"})
		return
	}
	var id, owner, status string
	err := instanceDB.QueryRowContext(c.Request.Context(), `SELECT id,owner_id,status FROM xcloud_control_devices WHERE credential_hash=?`, controlHash(token)).Scan(&id, &owner, &status)
	if err != nil || status != "enabled" {
		c.JSON(401, gin.H{"message": "设备凭证无效或已撤销"})
		return
	}
	conn, err := controlUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	s := &controlSession{deviceID: id, conn: conn, streams: map[string]chan controlFrame{}}
	controlSessions.Lock()
	old := controlSessions.byDevice[id]
	controlSessions.byDevice[id] = s
	controlSessions.Unlock()
	if old != nil {
		_ = old.conn.Close()
	}
	_, _ = instanceDB.ExecContext(c.Request.Context(), `UPDATE xcloud_control_devices SET last_heartbeat_at=NOW(),last_error=NULL,updated_at=NOW() WHERE id=?`, id)
	_ = s.send(controlFrame{Type: "hello", Protocol: controlProtocolVersion})
	controlPushConfig(owner)
	defer controlRemoveSession(id)
	for {
		var f controlFrame
		if err := conn.ReadJSON(&f); err != nil {
			return
		}
		if f.Protocol != "" && f.Protocol != controlProtocolVersion {
			_ = s.send(controlFrame{Type: "error", Error: "隧道协议版本不兼容"})
			return
		}
		if f.Type == "heartbeat" {
			_, _ = instanceDB.ExecContext(context.Background(), `UPDATE xcloud_control_devices SET last_heartbeat_at=NOW(),client_version=?,updated_at=NOW() WHERE id=?`, f.DeviceName, id)
			continue
		}
		s.streamMu.Lock()
		ch := s.streams[f.ID]
		s.streamMu.Unlock()
		if ch != nil {
			select {
			case ch <- f:
			default:
			}
		}
	}
}
func (s *controlSession) send(f controlFrame) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conn.WriteJSON(f)
}
func controlSessionFor(id string) *controlSession {
	controlSessions.RLock()
	defer controlSessions.RUnlock()
	return controlSessions.byDevice[id]
}
func controlRemoveSession(id string) {
	controlSessions.Lock()
	s := controlSessions.byDevice[id]
	delete(controlSessions.byDevice, id)
	controlSessions.Unlock()
	if s != nil {
		_ = s.conn.Close()
	}
}
func (s *controlSession) waitBytes(n int) {
	s.limitMu.Lock()
	defer s.limitMu.Unlock()
	d := time.Duration(int64(n) * int64(time.Second) / controlBandwidthBytesPerSecond)
	now := time.Now()
	if s.nextByte.Before(now) {
		s.nextByte = now
	}
	s.nextByte = s.nextByte.Add(d)
	if wait := time.Until(s.nextByte); wait > 0 {
		time.Sleep(wait)
	}
}
func controlPushConfig(owner string) {
	var deviceID, target, status string
	err := instanceDB.QueryRow(`SELECT r.device_id,r.target_url,r.status FROM xcloud_control_routes r JOIN xcloud_control_devices d ON d.id=r.device_id WHERE r.owner_id=? AND d.status='enabled' LIMIT 1`, owner).Scan(&deviceID, &target, &status)
	if err == nil {
		if s := controlSessionFor(deviceID); s != nil {
			_ = s.send(controlFrame{Type: "config", TargetURL: target, Status: 0, Error: status})
		}
	}
}

func controlGateway(c *gin.Context) {
	route := strings.TrimSpace(c.GetHeader("X-Control-Route-Key"))
	if !regexpRouteKey(route) {
		c.JSON(400, gin.H{"message": "自建入口路由无效"})
		return
	}
	var deviceID, status string
	if err := instanceDB.QueryRowContext(c.Request.Context(), `SELECT r.device_id,r.status FROM xcloud_control_routes r JOIN xcloud_control_devices d ON d.id=r.device_id WHERE r.route_key=? AND d.status='enabled'`, route).Scan(&deviceID, &status); err != nil || status != "enabled" {
		c.JSON(404, gin.H{"message": "自建入口不可用"})
		return
	}
	s := controlSessionFor(deviceID)
	if s == nil {
		c.JSON(502, gin.H{"message": "自建设备离线"})
		return
	}
	if websocket.IsWebSocketUpgrade(c.Request) {
		controlWebsocketGateway(c, s)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 32<<20))
	if err != nil {
		c.JSON(413, gin.H{"message": "请求体过大"})
		return
	}
	s.waitBytes(len(body))
	id := newID("cs")
	ch := make(chan controlFrame, 1)
	s.streamMu.Lock()
	s.streams[id] = ch
	s.streamMu.Unlock()
	defer func() { s.streamMu.Lock(); delete(s.streams, id); s.streamMu.Unlock() }()
	path := c.GetHeader("X-Forwarded-Uri")
	if !strings.HasPrefix(path, "/") {
		path = c.Request.URL.RequestURI()
	}
	headers := c.Request.Header.Clone()
	headers.Del("X-Control-Route-Key")
	headers.Del("X-Forwarded-Uri")
	if err = s.send(controlFrame{Type: "request", ID: id, Method: c.Request.Method, Path: path, Headers: headers, Body: body, Protocol: controlProtocolVersion}); err != nil {
		c.JSON(502, gin.H{"message": "自建设备离线"})
		return
	}
	select {
	case response := <-ch:
		if response.Type == "error" {
			c.JSON(502, gin.H{"message": "本地服务不可用"})
			return
		}
		for k, values := range response.Headers {
			for _, v := range values {
				c.Header(k, v)
			}
		}
		s.waitBytes(len(response.Body))
		c.Data(response.Status, "application/octet-stream", response.Body)
	case <-time.After(65 * time.Second):
		c.JSON(504, gin.H{"message": "自建设备响应超时"})
	}
}

func controlWebsocketGateway(c *gin.Context, s *controlSession) {
	id := newID("cws")
	ch := make(chan controlFrame, 32)
	s.streamMu.Lock()
	s.streams[id] = ch
	s.streamMu.Unlock()
	defer func() { s.streamMu.Lock(); delete(s.streams, id); s.streamMu.Unlock() }()
	path := c.GetHeader("X-Forwarded-Uri")
	if !strings.HasPrefix(path, "/") {
		path = c.Request.URL.RequestURI()
	}
	headers := c.Request.Header.Clone()
	headers.Del("X-Control-Route-Key")
	headers.Del("X-Forwarded-Uri")
	if err := s.send(controlFrame{Type: "ws_open", ID: id, Path: path, Headers: headers, Protocol: controlProtocolVersion}); err != nil {
		c.JSON(502, gin.H{"message": "自建设备离线"})
		return
	}
	select {
	case response := <-ch:
		if response.Type == "error" {
			c.JSON(502, gin.H{"message": "本地 WebSocket 服务不可用"})
			return
		}
	case <-time.After(20 * time.Second):
		c.JSON(504, gin.H{"message": "自建设备响应超时"})
		return
	}
	browser, err := controlUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer browser.Close()
	browserInbound := make(chan controlFrame, 16)
	go func() {
		defer close(browserInbound)
		for {
			kind, data, err := browser.ReadMessage()
			if err != nil {
				return
			}
			browserInbound <- controlFrame{Type: "ws_data", ID: id, Status: kind, Body: data}
		}
	}()
	for {
		select {
		case item, ok := <-browserInbound:
			if !ok {
				_ = s.send(controlFrame{Type: "ws_close", ID: id})
				return
			}
			s.waitBytes(len(item.Body))
			if err := s.send(item); err != nil {
				return
			}
		case item := <-ch:
			if item.Type == "ws_close" || item.Type == "error" {
				return
			}
			s.waitBytes(len(item.Body))
			if err := browser.WriteMessage(item.Status, item.Body); err != nil {
				return
			}
		}
	}
}
