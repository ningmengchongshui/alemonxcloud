package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

const protocol = "xcloud-control.v3"
const sessionTTL = 45 * time.Second
const chunkSize = 32 * 1024
const bandwidthBps = 10 * 1000 * 1000 / 8

const (
	fHello byte = iota + 1
	fChallenge
	fAuth
	fConfig
	fRequestStart
	fData
	fEnd
	fResponseStart
	fCancel
	fPing
	fPong
	fError
	fCommand
	fCommandResult
	fTerminalOpen
	fTerminalData
	fTerminalResize
	fTerminalClose
)

type frame struct {
	kind       byte
	id         string
	meta, data []byte
}
type hello struct {
	DeviceID    string `json:"deviceId"`
	Credential  string `json:"credential"`
	EnrollToken string `json:"enrollToken"`
	PublicKey   string `json:"publicKey"`
	Protocol    string `json:"protocol"`
}
type requestMeta struct {
	Method, Path string
	Headers      http.Header
}
type responseMeta struct {
	Status  int
	Headers http.Header
}
type commandRequest struct {
	DeviceID string          `json:"deviceId"`
	Action   string          `json:"action"`
	Payload  json.RawMessage `json:"payload"`
}
type commandResult struct {
	OK    bool            `json:"ok"`
	Error string          `json:"error,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}
type sessionRecord struct{ GatewayID, InternalURL, Epoch string }
type config struct {
	id, internalURL, certFile, keyFile string
	cluster                            bool
	db                                 *sql.DB
	redis                              *redis.Client
	internalClient                     *http.Client
	internalTLS                        *tls.Config
}
type tunnelSession struct {
	deviceID, ownerID, epoch, credential string
	conn                                 *websocket.Conn
	write                                sync.Mutex
	streams                              map[string]chan frame
	commands                             map[string]chan frame
	terminals                            map[string]chan frame
	mu                                   sync.RWMutex
	limiter                              *limiter
}
type limiter struct {
	mu   sync.Mutex
	next time.Time
}

var sessions = struct {
	sync.RWMutex
	values map[string]*tunnelSession
}{values: map[string]*tunnelSession{}}
var upgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }, ReadBufferSize: 64 * 1024, WriteBufferSize: 64 * 1024}

func main() {
	cfg, err := newConfig()
	if err != nil {
		log.Fatal(err)
	}
	go cfg.heartbeat()
	public := http.NewServeMux()
	public.HandleFunc("/healthz", cfg.health)
	public.HandleFunc("/connect", cfg.connect)
	public.HandleFunc("/proxy", cfg.publicProxy)
	public.HandleFunc("/command", cfg.command)
	public.HandleFunc("/terminal", cfg.terminal)
	public.HandleFunc("/metrics", cfg.metrics)
	if cfg.cluster {
		internal := http.NewServeMux()
		internal.HandleFunc("/internal/forward", cfg.internalForward)
		internal.HandleFunc("/internal/command", cfg.internalCommand)
		internal.HandleFunc("/internal/terminal", cfg.internalTerminal)
		go func() {
			server := &http.Server{Addr: env("GATEWAY_INTERNAL_LISTEN", ":18443"), Handler: internal, TLSConfig: cfg.internalTLS}
			log.Fatal(server.ListenAndServeTLS(cfg.certFile, cfg.keyFile))
		}()
	}
	log.Fatal(http.ListenAndServe(env("GATEWAY_PUBLIC_LISTEN", ":13072"), public))
}
func newConfig() (*config, error) {
	db, err := sql.Open("mysql", env("MYSQL_DSN", ""))
	if err != nil {
		return nil, err
	}
	r, err := redis.ParseURL(env("SESSION_REDIS_URL", ""))
	if err != nil {
		return nil, err
	}
	cluster, err := gatewayClusterMode()
	if err != nil {
		return nil, err
	}
	cfg := &config{id: env("GATEWAY_ID", hostname()), cluster: cluster, db: db, redis: redis.NewClient(r)}
	if !cfg.cluster {
		return cfg, nil
	}
	cfg.internalURL = strings.TrimRight(env("GATEWAY_INTERNAL_URL", ""), "/")
	cfg.certFile, cfg.keyFile = env("GATEWAY_TLS_CERT", ""), env("GATEWAY_TLS_KEY", "")
	if cfg.internalURL == "" {
		return nil, errors.New("cluster mode requires GATEWAY_INTERNAL_URL")
	}
	tlsCfg, err := internalTLS(cfg.certFile, cfg.keyFile, env("GATEWAY_TLS_CA", ""))
	if err != nil {
		return nil, err
	}
	cfg.internalTLS = tlsCfg
	cfg.internalClient = &http.Client{Timeout: 70 * time.Second, Transport: &http.Transport{TLSClientConfig: tlsCfg.Clone(), ForceAttemptHTTP2: true}}
	return cfg, nil
}

func gatewayClusterMode() (bool, error) {
	switch strings.ToLower(strings.TrimSpace(env("GATEWAY_MODE", "single"))) {
	case "", "single":
		return false, nil
	case "cluster":
		return true, nil
	default:
		return false, errors.New("GATEWAY_MODE must be single or cluster")
	}
}
func internalTLS(certFile, keyFile, caFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load gateway mTLS certificate: %w", err)
	}
	ca, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, errors.New("invalid gateway mTLS CA")
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}, RootCAs: pool, ClientCAs: pool, ClientAuth: tls.RequireAndVerifyClientCert, MinVersion: tls.VersionTLS13}, nil
}
func (c *config) heartbeat() {
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	for {
		c.updateGateway()
		<-tick.C
	}
}
func (c *config) updateGateway() {
	_, _ = c.db.Exec(`INSERT INTO xcloud_tunnel_gateways (id,internal_url,last_heartbeat_at,created_at,updated_at) VALUES (?,?,NOW(),NOW(),NOW()) ON DUPLICATE KEY UPDATE internal_url=VALUES(internal_url),status='online',last_heartbeat_at=NOW(),updated_at=NOW()`, c.id, c.internalURL)
}
func (c *config) health(w http.ResponseWriter, r *http.Request) {
	if c.db.PingContext(r.Context()) != nil || c.redis.Ping(r.Context()).Err() != nil {
		http.Error(w, "not ready", 503)
		return
	}
	w.WriteHeader(204)
}
func (c *config) metrics(w http.ResponseWriter, r *http.Request) {
	sessions.RLock()
	n := len(sessions.values)
	sessions.RUnlock()
	_, _ = fmt.Fprintf(w, "xcloud_tunnel_sessions %d\n", n)
}
func hash(v string) string { s := sha256.Sum256([]byte(v)); return hex.EncodeToString(s[:]) }
func (c *config) enroll(ctx context.Context, token, publicKey string) (string, string, string, string, error) {
	if len(decodeKey(publicKey)) != ed25519.PublicKeySize {
		return "", "", "", "", errors.New("invalid device key")
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", "", "", err
	}
	defer tx.Rollback()
	var owner string
	var expires time.Time
	var used, revoked sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT owner_id,expires_at,used_at,revoked_at FROM xcloud_control_enrollment_tokens WHERE token_hash=? FOR UPDATE`, hash(token)).Scan(&owner, &expires, &used, &revoked)
	if err != nil || used.Valid || revoked.Valid || !expires.After(time.Now()) {
		return "", "", "", "", errors.New("enrollment token invalid")
	}
	var locked string
	if err = tx.QueryRowContext(ctx, `SELECT id FROM xcloud_users WHERE id=? FOR UPDATE`, owner).Scan(&locked); err != nil {
		return "", "", "", "", err
	}
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM xcloud_control_devices WHERE owner_id=? AND status='enabled'`, owner).Scan(&count); err != nil {
		return "", "", "", "", err
	}
	if count > 0 {
		return "", "", "", "", errors.New("device quota exhausted")
	}
	deviceID := "ctl_" + randomID()
	credential := "xctl_device_" + randomID()
	route := "r" + hash(owner + "\x00" + deviceID)[:16]
	address := "https://control-" + route + "." + env("XCLOUD_INSTANCE_DOMAIN", "alemonjs.com")
	_, err = tx.ExecContext(ctx, `INSERT INTO xcloud_control_devices (id,owner_id,name,public_key,credential_hash,status,client_version,key_version,credential_version,rebind_required,created_at,updated_at) VALUES (?,?,?,?,?,'enabled','',3,1,FALSE,NOW(),NOW())`, deviceID, owner, "xcloud-control", publicKey, hash(credential))
	if err != nil {
		return "", "", "", "", err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO xcloud_control_routes (id,owner_id,device_id,route_key,target_url,status,access_address,created_at,updated_at) VALUES (?,?,?,?,?,'paused',?,NOW(),NOW())`, "ctr_"+randomID(), owner, deviceID, route, "", address)
	if err != nil {
		return "", "", "", "", err
	}
	// Enrollment is the only provisioning path for a self-hosted node. Keep
	// the node linked to the long-lived device identity; it is intentionally
	// not a platform Agent URL and must never enter platform scheduling.
	_, err = tx.ExecContext(ctx, `INSERT INTO xcloud_nodes (id,name,agent_url,cpu_total,memory_total_mb,enabled,node_kind,owner_id,control_device_id,created_at,updated_at) VALUES (?,?, '',0,0,TRUE,'selfhosted',?,?,NOW(),NOW())`, "snode_"+randomID(), "自建节点", owner, deviceID)
	if err != nil {
		return "", "", "", "", err
	}
	_, err = tx.ExecContext(ctx, `UPDATE xcloud_control_enrollment_tokens SET used_at=NOW(),device_id=? WHERE token_hash=?`, deviceID, hash(token))
	if err != nil {
		return "", "", "", "", err
	}
	if err = tx.Commit(); err != nil {
		return "", "", "", "", err
	}
	return deviceID, credential, owner, publicKey, nil
}
func (c *config) connect(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	first, err := readFrame(conn)
	if err != nil || first.kind != fHello {
		return
	}
	var h hello
	if json.Unmarshal(first.meta, &h) != nil || h.Protocol != protocol {
		_ = writeFrame(conn, frame{kind: fError, meta: []byte(`{"message":"协议版本不兼容"}`)})
		return
	}
	var owner, pub, status string
	var rebind bool
	if h.DeviceID == "" && h.EnrollToken != "" {
		h.DeviceID, h.Credential, owner, pub, err = c.enroll(r.Context(), h.EnrollToken, h.PublicKey)
		status, rebind = "enabled", false
	}
	if err == nil && h.EnrollToken == "" {
		err = c.db.QueryRowContext(r.Context(), `SELECT owner_id,public_key,status,rebind_required FROM xcloud_control_devices WHERE id=? AND credential_hash=?`, h.DeviceID, hash(h.Credential)).Scan(&owner, &pub, &status, &rebind)
	}
	if err != nil || status != "enabled" || rebind {
		_ = writeFrame(conn, frame{kind: fError, meta: []byte(`{"message":"设备凭证无效或需要重新绑定"}`)})
		return
	}
	challenge := make([]byte, 32)
	_, _ = rand.Read(challenge)
	if err = writeFrame(conn, frame{kind: fChallenge, data: challenge}); err != nil {
		return
	}
	proof, err := readFrame(conn)
	if err != nil || proof.kind != fAuth || !ed25519.Verify(ed25519.PublicKey(decodeKey(pub)), challenge, proof.data) {
		return
	}
	s := &tunnelSession{deviceID: h.DeviceID, ownerID: owner, credential: h.Credential, epoch: randomID(), conn: conn, streams: map[string]chan frame{}, commands: map[string]chan frame{}, terminals: map[string]chan frame{}, limiter: &limiter{}}
	c.putSession(s)
	defer c.dropSession(s)
	c.setDirectory(r.Context(), s)
	defer c.clearDirectory(context.Background(), s)
	go c.refreshSession(s)
	c.sendConfig(s)
	for {
		f, err := readFrame(conn)
		if err != nil {
			return
		}
		if f.kind == fPing {
			c.updateSelfHostedHeartbeat(s, f.meta)
			c.sendConfig(s)
			_ = s.send(frame{kind: fPong})
			continue
		}
		if f.kind == fCommandResult {
			s.mu.RLock()
			ch := s.commands[f.id]
			s.mu.RUnlock()
			if ch != nil {
				select {
				case ch <- f:
				default:
				}
			}
			continue
		}
		if f.kind == fTerminalData || f.kind == fTerminalClose {
			s.mu.RLock()
			ch := s.terminals[f.id]
			s.mu.RUnlock()
			if ch != nil {
				select {
				case ch <- f:
				default:
				}
			}
			continue
		}
		s.mu.RLock()
		ch := s.streams[f.id]
		s.mu.RUnlock()
		if ch != nil {
			select {
			case ch <- f:
			default:
				_ = s.send(frame{kind: fCancel, id: f.id})
			}
		}
	}
}

func (c *config) updateSelfHostedHeartbeat(s *tunnelSession, raw []byte) {
	var report struct {
		CPUDetected           float64  `json:"cpuDetected"`
		MemoryDetectedMB      int      `json:"memoryDetectedMB"`
		AgentVersion          string   `json:"agentVersion"`
		AgentAPIVersion       int      `json:"agentApiVersion"`
		Capabilities          []string `json:"capabilities"`
		DiskAvailableBytes    int64    `json:"diskAvailableBytes"`
		DiskTotalBytes        int64    `json:"diskTotalBytes"`
		DockerVersion         string   `json:"dockerVersion"`
		ManagedContainerCount int      `json:"managedContainerCount"`
		Instances             []struct {
			Route string `json:"route"`
			State string `json:"state"`
		} `json:"instances"`
	}
	if json.Unmarshal(raw, &report) != nil || report.CPUDetected <= 0 || report.MemoryDetectedMB <= 0 {
		_, _ = c.db.Exec(`UPDATE xcloud_control_devices SET last_heartbeat_at=NOW(),last_connected_at=NOW(),gateway_id=?,last_error=NULL,updated_at=NOW() WHERE id=?`, c.id, s.deviceID)
		return
	}
	caps, _ := json.Marshal(report.Capabilities)
	_, _ = c.db.Exec(`UPDATE xcloud_nodes SET cpu_detected=?,memory_detected_mb=?,cpu_total=?,memory_total_mb=?,cpu_quota=IF(selfhosted_quota_mode='auto' OR cpu_quota<=0,ROUND(?*0.8,2),cpu_quota),memory_quota_mb=IF(selfhosted_quota_mode='auto' OR memory_quota_mb<=0,FLOOR(?*0.8),memory_quota_mb),docker_version=?,disk_available_bytes=?,disk_total_bytes=?,managed_container_count=?,agent_version=?,agent_api_version=?,agent_capabilities=?,last_heartbeat_at=NOW(),updated_at=NOW() WHERE control_device_id=? AND node_kind='selfhosted'`, report.CPUDetected, report.MemoryDetectedMB, report.CPUDetected, report.MemoryDetectedMB, report.CPUDetected, report.MemoryDetectedMB, report.DockerVersion, report.DiskAvailableBytes, report.DiskTotalBytes, report.ManagedContainerCount, report.AgentVersion, report.AgentAPIVersion, string(caps), s.deviceID)
	_, _ = c.db.Exec(`UPDATE xcloud_control_devices SET last_heartbeat_at=NOW(),last_connected_at=NOW(),gateway_id=?,last_error=NULL,updated_at=NOW() WHERE id=?`, c.id, s.deviceID)
	_, _ = c.db.Exec(`UPDATE xcloud_instances i JOIN xcloud_nodes n ON n.id=i.node_id SET i.runtime_status='missing' WHERE i.placement_type='selfhosted' AND n.control_device_id=? AND i.status IN ('running','stopped','destroy_scheduled')`, s.deviceID)
	for _, item := range report.Instances {
		runtime := "stopped"
		if item.State == "running" {
			runtime = "running"
		}
		_, _ = c.db.Exec(`UPDATE xcloud_instances i JOIN xcloud_nodes n ON n.id=i.node_id SET i.runtime_status=? WHERE i.route_key=? AND i.placement_type='selfhosted' AND n.control_device_id=? AND i.status IN ('running','stopped','destroy_scheduled')`, runtime, item.Route, s.deviceID)
	}
}

func (c *config) command(w http.ResponseWriter, r *http.Request) {
	if token := env("GATEWAY_COMMAND_TOKEN", ""); token == "" || r.Header.Get("Authorization") != "Bearer "+token {
		http.Error(w, "forbidden", 403)
		return
	}
	c.commandRequest(w, r)
}

// terminal is a private control-plane endpoint. Browser sessions are
// authenticated by xCloud before reaching it; the additional gateway token
// prevents a user from opening a terminal by guessing a device identifier.
func (c *config) terminal(w http.ResponseWriter, r *http.Request) {
	if token := env("GATEWAY_COMMAND_TOKEN", ""); token == "" || r.Header.Get("Authorization") != "Bearer "+token {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	device, name, route := r.URL.Query().Get("deviceId"), r.URL.Query().Get("name"), r.URL.Query().Get("route")
	if device == "" || !validManagedName(name) || !validRoute(route) {
		http.Error(w, "invalid terminal", http.StatusBadRequest)
		return
	}
	if owner, err := c.directory(r.Context(), device); err != nil || owner.GatewayID == "" {
		http.Error(w, "device offline", http.StatusBadGateway)
		return
	} else if owner.GatewayID != c.id {
		if !c.cluster {
			http.Error(w, "gateway owner unavailable", http.StatusBadGateway)
			return
		}
		c.terminalRemote(w, r, owner)
		return
	}
	c.terminalLocal(w, r, device, name, route)
}
func (c *config) internalTerminal(w http.ResponseWriter, r *http.Request) {
	device, name, route := r.URL.Query().Get("deviceId"), r.URL.Query().Get("name"), r.URL.Query().Get("route")
	if device == "" || !validManagedName(name) || !validRoute(route) {
		http.Error(w, "invalid terminal", 400)
		return
	}
	c.terminalLocal(w, r, device, name, route)
}
func (c *config) terminalRemote(w http.ResponseWriter, r *http.Request, owner sessionRecord) {
	browser, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer browser.Close()
	target := strings.TrimRight(owner.InternalURL, "/") + "/internal/terminal?" + r.URL.RawQuery
	parsed, parseErr := url.Parse(target)
	if parseErr != nil {
		return
	}
	if parsed.Scheme == "https" {
		parsed.Scheme = "wss"
	} else if parsed.Scheme == "http" {
		parsed.Scheme = "ws"
	}
	dialer := websocket.Dialer{TLSClientConfig: c.internalTLS.Clone()}
	agent, _, err := dialer.Dial(parsed.String(), nil)
	if err != nil {
		return
	}
	defer agent.Close()
	bridgeWebSockets(browser, agent)
}
func bridgeWebSockets(left, right *websocket.Conn) {
	var once sync.Once
	closeBoth := func() { once.Do(func() { _ = left.Close(); _ = right.Close() }) }
	copyOne := func(dst, src *websocket.Conn) {
		defer closeBoth()
		for {
			kind, data, err := src.ReadMessage()
			if err != nil {
				return
			}
			if dst.WriteMessage(kind, data) != nil {
				return
			}
		}
	}
	go copyOne(right, left)
	copyOne(left, right)
}
func (c *config) terminalLocal(w http.ResponseWriter, r *http.Request, device, name, route string) {
	s := sessionFor(device)
	if s == nil {
		http.Error(w, "device offline", http.StatusBadGateway)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	id := "term_" + randomID()
	ch := make(chan frame, 32)
	s.mu.Lock()
	s.terminals[id] = ch
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.terminals, id)
		s.mu.Unlock()
		_ = s.send(frame{kind: fTerminalClose, id: id})
	}()
	meta, _ := json.Marshal(map[string]string{"name": name, "route": route})
	if s.send(frame{kind: fTerminalOpen, id: id, meta: meta}) != nil {
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			kind, data, e := conn.ReadMessage()
			if e != nil {
				return
			}
			if kind == websocket.TextMessage || kind == websocket.BinaryMessage {
				const resizePrefix = "__XCLOUD_TERM_RESIZE__:"
				if kind == websocket.TextMessage && strings.HasPrefix(string(data), resizePrefix) {
					var size struct {
						Columns uint16 `json:"columns"`
						Rows    uint16 `json:"rows"`
					}
					if json.Unmarshal([]byte(strings.TrimPrefix(string(data), resizePrefix)), &size) == nil && size.Columns > 0 && size.Rows > 0 {
						raw, _ := json.Marshal(size)
						if s.send(frame{kind: fTerminalResize, id: id, meta: raw}) != nil {
							return
						}
						continue
					}
				}
				if s.send(frame{kind: fTerminalData, id: id, data: data}) != nil {
					return
				}
			}
		}
	}()
	for {
		select {
		case f := <-ch:
			if f.kind == fTerminalClose {
				return
			}
			if f.kind == fTerminalData {
				if conn.WriteMessage(websocket.TextMessage, f.data) != nil {
					return
				}
			}
		case <-done:
			return
		}
	}
}
func validManagedName(v string) bool {
	if len(v) < 15 || len(v) > 40 || !strings.HasPrefix(v, "xcloud-") {
		return false
	}
	for _, r := range v[len("xcloud-"):] {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}
func (c *config) internalCommand(w http.ResponseWriter, r *http.Request) { c.commandRequest(w, r) }
func (c *config) commandRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", 405)
		return
	}
	var req commandRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.DeviceID == "" || req.Action == "" {
		http.Error(w, "invalid command", 400)
		return
	}
	if remote, err := c.directory(r.Context(), req.DeviceID); err == nil && remote.GatewayID != c.id {
		if !c.cluster {
			http.Error(w, "gateway owner unavailable", http.StatusBadGateway)
			return
		}
		body, _ := json.Marshal(req)
		forward, err := http.NewRequestWithContext(r.Context(), http.MethodPost, strings.TrimRight(remote.InternalURL, "/")+"/internal/command", strings.NewReader(string(body)))
		if err != nil {
			http.Error(w, "forward failed", 502)
			return
		}
		forward.Header.Set("Content-Type", "application/json")
		res, err := c.internalClient.Do(forward)
		if err != nil {
			http.Error(w, "owner unavailable", 502)
			return
		}
		defer res.Body.Close()
		w.WriteHeader(res.StatusCode)
		_, _ = io.Copy(w, res.Body)
		return
	}
	s := sessionFor(req.DeviceID)
	if s == nil {
		http.Error(w, "device offline", 502)
		return
	}
	id := "cmd_" + randomID()
	ch := make(chan frame, 1)
	s.mu.Lock()
	s.commands[id] = ch
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.commands, id); s.mu.Unlock() }()
	meta, _ := json.Marshal(req)
	if s.send(frame{kind: fCommand, id: id, meta: meta}) != nil {
		http.Error(w, "device unavailable", 502)
		return
	}
	select {
	case f := <-ch:
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(f.meta)
	case <-time.After(70 * time.Second):
		http.Error(w, "command timeout", 504)
	case <-r.Context().Done():
	}
}
func decodeKey(v string) []byte {
	b, _ := hex.DecodeString(v)
	if len(b) == ed25519.PublicKeySize {
		return b
	}
	b, _ = base64.RawStdEncoding.DecodeString(v)
	return b
}
func (c *config) refreshSession(s *tunnelSession) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for range t.C {
		if sessionFor(s.deviceID) != s {
			return
		}
		c.setDirectory(context.Background(), s)
		_, _ = c.db.Exec(`UPDATE xcloud_control_devices SET last_heartbeat_at=NOW(),last_connected_at=NOW(),gateway_id=?,last_error=NULL,updated_at=NOW() WHERE id=?`, c.id, s.deviceID)
	}
}
func (c *config) sendConfig(s *tunnelSession) {
	var target, status string
	_ = c.db.QueryRow(`SELECT target_url,status FROM xcloud_control_routes WHERE device_id=?`, s.deviceID).Scan(&target, &status)
	raw, _ := json.Marshal(map[string]string{"targetURL": target, "status": status, "protocol": protocol, "deviceID": s.deviceID, "credential": s.credential})
	_ = s.send(frame{kind: fConfig, meta: raw})
}
func (c *config) setDirectory(ctx context.Context, s *tunnelSession) {
	raw, _ := json.Marshal(sessionRecord{c.id, c.internalURL, s.epoch})
	_ = c.redis.Set(ctx, "xcloud:tunnel:"+s.deviceID, raw, sessionTTL).Err()
}
func (c *config) clearDirectory(ctx context.Context, s *tunnelSession) {
	key := "xcloud:tunnel:" + s.deviceID
	var raw string
	if c.redis.Get(ctx, key).Scan(&raw) == nil {
		var record sessionRecord
		if json.Unmarshal([]byte(raw), &record) == nil && record.Epoch == s.epoch {
			_ = c.redis.Del(ctx, key).Err()
		}
	}
}
func (c *config) putSession(s *tunnelSession) {
	sessions.Lock()
	old := sessions.values[s.deviceID]
	sessions.values[s.deviceID] = s
	sessions.Unlock()
	if old != nil {
		_ = old.conn.Close()
	}
}
func (c *config) dropSession(s *tunnelSession) {
	sessions.Lock()
	if sessions.values[s.deviceID] == s {
		delete(sessions.values, s.deviceID)
	}
	sessions.Unlock()
}
func sessionFor(id string) *tunnelSession {
	sessions.RLock()
	defer sessions.RUnlock()
	return sessions.values[id]
}
func (s *tunnelSession) send(f frame) error {
	s.write.Lock()
	defer s.write.Unlock()
	return writeFrame(s.conn, f)
}
func (l *limiter) wait(n int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	d := time.Duration(int64(n) * int64(time.Second) / bandwidthBps)
	now := time.Now()
	if l.next.Before(now) {
		l.next = now
	}
	l.next = l.next.Add(d)
	if wait := time.Until(l.next); wait > 0 {
		time.Sleep(wait)
	}
}
func (c *config) publicProxy(w http.ResponseWriter, r *http.Request) {
	route := r.Header.Get("X-Control-Route-Key")
	instanceRoute := r.Header.Get("X-Instance-Route-Key")
	if instanceRoute != "" {
		route = instanceRoute
	}
	if !validRoute(route) {
		http.Error(w, "自建入口路由无效", 400)
		return
	}
	var device, status string
	query := `SELECT r.device_id,r.status FROM xcloud_control_routes r JOIN xcloud_control_devices d ON d.id=r.device_id WHERE r.route_key=? AND d.status='enabled' AND d.rebind_required=FALSE`
	if instanceRoute != "" {
		query = `SELECT n.control_device_id,'enabled' FROM xcloud_instances i JOIN xcloud_nodes n ON n.id=i.node_id JOIN xcloud_control_devices d ON d.id=n.control_device_id WHERE i.route_key=? AND i.placement_type='selfhosted' AND i.status IN ('running','deploying','destroy_scheduled') AND COALESCE(i.runtime_status,'running')='running' AND n.enabled=TRUE AND d.status='enabled' AND d.rebind_required=FALSE`
	}
	err := c.db.QueryRowContext(r.Context(), query, route).Scan(&device, &status)
	if err != nil || status != "enabled" {
		http.Error(w, "自建入口不可用", 404)
		return
	}
	record, err := c.directory(r.Context(), device)
	if err != nil {
		http.Error(w, "自建设备离线", 502)
		return
	}
	if instanceRoute != "" {
		r.Header.Set("X-Xcloud-Managed-Route", route)
	}
	if record.GatewayID == c.id {
		c.proxyLocal(w, r, device)
		return
	}
	if !c.cluster {
		http.Error(w, "gateway owner unavailable", http.StatusBadGateway)
		return
	}
	c.proxyRemote(w, r, device, record)
}
func (c *config) directory(ctx context.Context, device string) (sessionRecord, error) {
	var raw string
	if err := c.redis.Get(ctx, "xcloud:tunnel:"+device).Scan(&raw); err != nil {
		return sessionRecord{}, err
	}
	var r sessionRecord
	return r, json.Unmarshal([]byte(raw), &r)
}
func (c *config) proxyRemote(w http.ResponseWriter, r *http.Request, device string, owner sessionRecord) {
	target := strings.TrimRight(owner.InternalURL, "/") + "/internal/forward"
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
	if err != nil {
		http.Error(w, "网关转发失败", 502)
		return
	}
	req.Header = safeHeaders(r.Header)
	req.Header.Set("X-Xcloud-Internal-Device", device)
	req.Header.Set("X-Xcloud-Original-Method", r.Method)
	req.Header.Set("X-Xcloud-Original-URI", r.URL.RequestURI())
	if managed := r.Header.Get("X-Xcloud-Managed-Route"); managed != "" {
		req.Header.Set("X-Xcloud-Managed-Route", managed)
	}
	res, err := c.internalClient.Do(req)
	if err != nil {
		http.Error(w, "自建设备暂不可用", 502)
		return
	}
	defer res.Body.Close()
	copyResponse(w, res)
}
func (c *config) internalForward(w http.ResponseWriter, r *http.Request) {
	device := r.Header.Get("X-Xcloud-Internal-Device")
	if device == "" {
		http.Error(w, "forbidden", 403)
		return
	}
	method := r.Header.Get("X-Xcloud-Original-Method")
	uri := r.Header.Get("X-Xcloud-Original-URI")
	if method == "" || !strings.HasPrefix(uri, "/") {
		http.Error(w, "invalid", 400)
		return
	}
	r.Method = method
	r.URL, _ = url.Parse(uri)
	if managed := r.Header.Get("X-Xcloud-Managed-Route"); managed != "" {
		r.Header.Set("X-Xcloud-Managed-Route", managed)
	}
	c.proxyLocal(w, r, device)
}
func (c *config) proxyLocal(w http.ResponseWriter, r *http.Request, device string) {
	s := sessionFor(device)
	if s == nil {
		http.Error(w, "自建设备离线", 502)
		return
	}
	id := randomID()
	in := make(chan frame, 32)
	s.mu.Lock()
	s.streams[id] = in
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.streams, id); s.mu.Unlock(); _ = s.send(frame{kind: fCancel, id: id}) }()
	meta, _ := json.Marshal(requestMeta{r.Method, originalURI(r), safeHeaders(r.Header)})
	if err := s.send(frame{kind: fRequestStart, id: id, meta: meta}); err != nil {
		http.Error(w, "自建设备离线", 502)
		return
	}
	go func() {
		buf := make([]byte, chunkSize)
		for {
			n, e := r.Body.Read(buf)
			if n > 0 {
				s.limiter.wait(n)
				_ = s.send(frame{kind: fData, id: id, data: append([]byte(nil), buf[:n]...)})
			}
			if e != nil {
				_ = s.send(frame{kind: fEnd, id: id})
				return
			}
		}
	}()
	select {
	case first := <-in:
		if first.kind == fError {
			http.Error(w, "本地服务不可用", 502)
			return
		}
		if first.kind != fResponseStart {
			http.Error(w, "隧道响应异常", 502)
			return
		}
		var response responseMeta
		if json.Unmarshal(first.meta, &response) != nil {
			http.Error(w, "隧道响应异常", 502)
			return
		}
		for k, values := range response.Headers {
			for _, v := range values {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(response.Status)
		for {
			f, ok := <-in
			if !ok || f.kind == fEnd {
				return
			}
			if f.kind == fData {
				s.limiter.wait(len(f.data))
				_, _ = w.Write(f.data)
			}
			if f.kind == fError {
				return
			}
		}
	case <-time.After(30 * time.Second):
		http.Error(w, "自建设备响应超时", 504)
	}
}
func safeHeaders(h http.Header) http.Header {
	out := h.Clone()
	for _, key := range []string{"Connection", "Upgrade", "X-Control-Route-Key", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Xcloud-Internal-Device", "X-Xcloud-Original-Method", "X-Xcloud-Original-URI"} {
		out.Del(key)
	}
	return out
}
func copyResponse(w http.ResponseWriter, res *http.Response) {
	for k, v := range res.Header {
		for _, x := range v {
			w.Header().Add(k, x)
		}
	}
	w.WriteHeader(res.StatusCode)
	_, _ = io.Copy(w, res.Body)
}
func originalURI(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-Uri"); strings.HasPrefix(v, "/") {
		return v
	}
	return r.URL.RequestURI()
}
func validRoute(v string) bool {
	if len(v) != 17 || v[0] != 'r' {
		return false
	}
	for _, c := range v[1:] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func writeFrame(c *websocket.Conn, f frame) error {
	buf := make([]byte, 7+len(f.id)+len(f.meta)+len(f.data))
	buf[0] = f.kind
	binary.BigEndian.PutUint16(buf[1:3], uint16(len(f.id)))
	binary.BigEndian.PutUint32(buf[3:7], uint32(len(f.meta)))
	copy(buf[7:], f.id)
	copy(buf[7+len(f.id):], f.meta)
	copy(buf[7+len(f.id)+len(f.meta):], f.data)
	return c.WriteMessage(websocket.BinaryMessage, buf)
}
func readFrame(c *websocket.Conn) (frame, error) {
	kind, data, err := c.ReadMessage()
	if err != nil {
		return frame{}, err
	}
	if kind != websocket.BinaryMessage || len(data) < 7 {
		return frame{}, errors.New("invalid tunnel frame")
	}
	il := int(binary.BigEndian.Uint16(data[1:3]))
	ml := int(binary.BigEndian.Uint32(data[3:7]))
	if len(data) < 7+il+ml {
		return frame{}, errors.New("truncated tunnel frame")
	}
	return frame{kind: data[0], id: string(data[7 : 7+il]), meta: data[7+il : 7+il+ml], data: data[7+il+ml:]}, nil
}
func randomID() string { b := make([]byte, 12); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func hostname() string { v, _ := os.Hostname(); return v }
