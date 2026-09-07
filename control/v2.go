package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
	"xcloud/agent-core"
)

const (
	v2Hello byte = iota + 1
	v2Challenge
	v2Auth
	v2Config
	v2RequestStart
	v2Data
	v2End
	v2ResponseStart
	v2Cancel
	v2Ping
	v2Pong
	v2Error
	v2Command
	v2CommandResult
	v2TerminalOpen
	v2TerminalData
	v2TerminalResize
	v2TerminalClose
)

type v2Frame struct {
	kind       byte
	id         string
	meta, data []byte
}
type v2Request struct {
	Method, Path string
	Headers      http.Header
}
type v2Response struct {
	Status  int
	Headers http.Header
}
type v2CommandRequest struct {
	DeviceID string          `json:"deviceId"`
	Action   string          `json:"action"`
	Payload  json.RawMessage `json:"payload"`
}
type managedPayload struct {
	Name          string  `json:"name"`
	Image         string  `json:"image"`
	CPU           float64 `json:"cpu"`
	MemoryMB      int     `json:"memoryMB"`
	BandwidthMbps int     `json:"bandwidthMbps"`
	Route         string  `json:"route"`
	KeepStopped   bool    `json:"keepStopped"`
	Path          string  `json:"path,omitempty"`
	Content       string  `json:"content,omitempty"`
	Tail          string  `json:"tail,omitempty"`
	Since         string  `json:"since,omitempty"`
}

func serveV2(cfg *config, path string) error {
	base := cfg.TunnelURL
	if base == "" {
		base = cfg.ServerURL
	}
	u, err := url.Parse(base)
	if err != nil {
		return err
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else if u.Scheme == "http" {
		u.Scheme = "ws"
	} else {
		return errors.New("网关地址必须是 http 或 https")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/connect"
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	var write sync.Mutex
	send := func(f v2Frame) error { write.Lock(); defer write.Unlock(); return writeV2(conn, f) }
	private, err := base64Raw(cfg.PrivateKey)
	if err != nil || len(private) != ed25519.PrivateKeySize {
		return errors.New("设备密钥缺失，请重新写入接入 Token")
	}
	public := ed25519.PrivateKey(private).Public().(ed25519.PublicKey)
	helloRaw, _ := json.Marshal(map[string]string{"deviceId": cfg.DeviceID, "credential": cfg.Credential, "enrollToken": cfg.EnrollToken, "publicKey": base64.RawStdEncoding.EncodeToString(public), "protocol": protocol})
	if err = send(v2Frame{kind: v2Hello, meta: helloRaw}); err != nil {
		return err
	}
	challenge, err := readV2(conn)
	if err != nil || challenge.kind != v2Challenge {
		return errors.New("网关认证挑战失败")
	}
	if err = send(v2Frame{kind: v2Auth, data: ed25519.Sign(ed25519.PrivateKey(private), challenge.data)}); err != nil {
		return err
	}
	pipes := struct {
		sync.RWMutex
		values map[string]*io.PipeWriter
	}{values: map[string]*io.PipeWriter{}}
	terminals := struct {
		sync.RWMutex
		values map[string]*os.File
	}{values: map[string]*os.File{}}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				status, _ := json.Marshal(localAgentStatus())
				_ = send(v2Frame{kind: v2Ping, meta: status})
			}
		}
	}()
	for {
		_ = conn.SetReadDeadline(time.Now().Add(50 * time.Second))
		f, err := readV2(conn)
		if err != nil {
			return err
		}
		switch f.kind {
		case v2Config:
			var item map[string]string
			_ = json.Unmarshal(f.meta, &item)
			cfg.TargetURL = item["targetURL"]
			cfg.RouteStatus = item["status"]
			if item["deviceID"] != "" && cfg.DeviceID == "" {
				cfg.DeviceID, cfg.Credential, cfg.EnrollToken = item["deviceID"], item["credential"], ""
				if err := save(path, *cfg); err != nil {
					return err
				}
			}
		case v2RequestStart:
			go handleV2Request(*cfg, f, send, &pipes)
		case v2Command:
			go handleV2Command(cfg, f, send)
		case v2TerminalOpen:
			go handleV2Terminal(cfg, f, send, &terminals)
		case v2TerminalData:
			terminals.RLock()
			terminal := terminals.values[f.id]
			terminals.RUnlock()
			if terminal != nil {
				_, _ = terminal.Write(f.data)
			}
		case v2TerminalResize:
			var size struct {
				Columns uint16 `json:"columns"`
				Rows    uint16 `json:"rows"`
			}
			terminals.RLock()
			terminal := terminals.values[f.id]
			terminals.RUnlock()
			if terminal != nil && json.Unmarshal(f.meta, &size) == nil && size.Columns > 0 && size.Rows > 0 {
				_ = pty.Setsize(terminal, &pty.Winsize{Cols: size.Columns, Rows: size.Rows})
			}
		case v2TerminalClose:
			terminals.Lock()
			terminal := terminals.values[f.id]
			delete(terminals.values, f.id)
			terminals.Unlock()
			if terminal != nil {
				_ = terminal.Close()
			}
		case v2Data:
			pipes.RLock()
			p := pipes.values[f.id]
			pipes.RUnlock()
			if p != nil {
				_, _ = p.Write(f.data)
			}
		case v2End, v2Cancel:
			pipes.RLock()
			p := pipes.values[f.id]
			pipes.RUnlock()
			if p != nil {
				_ = p.Close()
			}
		case v2Ping:
			_ = send(v2Frame{kind: v2Pong})
		case v2Error:
			return errors.New("网关拒绝连接")
		}
	}
}

func handleV2Terminal(cfg *config, f v2Frame, send func(v2Frame) error, terminals *struct {
	sync.RWMutex
	values map[string]*os.File
}) {
	var meta struct {
		Name  string `json:"name"`
		Route string `json:"route"`
	}
	if json.Unmarshal(f.meta, &meta) != nil || !safeManagedName(meta.Name) || len(meta.Route) != 17 {
		_ = send(v2Frame{kind: v2TerminalClose, id: f.id})
		return
	}
	if _, err := managedRouteTarget(*cfg, meta.Route); err != nil {
		_ = send(v2Frame{kind: v2TerminalClose, id: f.id})
		return
	}
	args, argsErr := agentcore.TerminalDockerArgs(meta.Name)
	if argsErr != nil {
		_ = send(v2Frame{kind: v2TerminalClose, id: f.id})
		return
	}
	cmd := exec.Command("docker", args...)
	terminal, err := pty.Start(cmd)
	if err != nil {
		_ = send(v2Frame{kind: v2TerminalData, id: f.id, data: []byte("\r\n无法启动容器终端。\r\n")})
		_ = send(v2Frame{kind: v2TerminalClose, id: f.id})
		return
	}
	terminals.Lock()
	terminals.values[f.id] = terminal
	terminals.Unlock()
	go func() {
		defer func() {
			terminals.Lock()
			delete(terminals.values, f.id)
			terminals.Unlock()
			_ = terminal.Close()
			_ = send(v2Frame{kind: v2TerminalClose, id: f.id})
		}()
		buf := make([]byte, 4096)
		for {
			n, e := terminal.Read(buf)
			if n > 0 {
				if send(v2Frame{kind: v2TerminalData, id: f.id, data: append([]byte(nil), buf[:n]...)}) != nil {
					return
				}
			}
			if e != nil {
				return
			}
		}
	}()
}

func localAgentStatus() map[string]any {
	mem := 0
	if raw, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			var kb int
			if _, err := fmt.Sscanf(line, "MemTotal: %d kB", &kb); err == nil {
				mem = kb / 1024
				break
			}
		}
	}
	if mem == 0 {
		mem = 1024
	}
	root := os.Getenv("XCLOUD_CONTROL_DATA_ROOT")
	if root == "" {
		root = "/var/lib/xcloud-control/instances"
	}
	_ = os.MkdirAll(root, 0700)
	var stat syscall.Statfs_t
	available, total := int64(0), int64(0)
	if syscall.Statfs(root, &stat) == nil {
		available = int64(stat.Bavail) * int64(stat.Bsize)
		total = int64(stat.Blocks) * int64(stat.Bsize)
	}
	dockerVersion := ""
	if raw, err := exec.Command("docker", "info", "--format", "{{.ServerVersion}}").Output(); err == nil {
		dockerVersion = strings.TrimSpace(string(raw))
	}
	instances := []map[string]string{}
	if raw, err := exec.Command("docker", "ps", "-a", "--filter", "label=xcloud.managed=true", "--format", "{{.Label \"xcloud.route\"}}|{{.State}}").Output(); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
			parts := strings.SplitN(line, "|", 2)
			if len(parts) == 2 && len(parts[0]) == 17 {
				instances = append(instances, map[string]string{"route": parts[0], "state": parts[1]})
			}
		}
	}
	return map[string]any{"cpuDetected": float64(runtime.NumCPU()), "memoryDetectedMB": mem, "diskAvailableBytes": available, "diskTotalBytes": total, "dockerVersion": dockerVersion, "managedContainerCount": len(instances), "instances": instances, "agentVersion": "0.2.0", "agentApiVersion": 1, "capabilities": []string{"container.lifecycle.v1", "container.inspect.v1", "container.logs.v1", "container.terminal.v1", "container.compose.v1", "container.compose.restart.v1", "container.compose.resize.v1", "container.reinstall.v1", "container.destroy.v1", "image.pull.v1", "route.proxy.v1", "node.resources.v1", "workspace.files.v1", "network.bandwidth.v1"}}
}

// handleV2Command deliberately recognises a small fixed command set.  The
// control client never accepts a shell string, a host path, or an arbitrary
// Docker object from the tunnel.
func handleV2Command(cfg *config, f v2Frame, send func(v2Frame) error) {
	var command v2CommandRequest
	if json.Unmarshal(f.meta, &command) != nil {
		_ = send(v2Frame{kind: v2CommandResult, id: f.id, meta: []byte(`{"ok":false,"error":"命令格式无效"}`)})
		return
	}
	var p managedPayload
	_ = json.Unmarshal(command.Payload, &p)
	data, err := runManagedCommand(*cfg, command.Action, p)
	result := map[string]any{"ok": err == nil}
	if err != nil {
		result["error"] = err.Error()
	}
	if data != nil {
		result["data"] = data
	}
	raw, _ := json.Marshal(result)
	_ = send(v2Frame{kind: v2CommandResult, id: f.id, meta: raw})
}
func runManagedCommand(cfg config, action string, p managedPayload) (any, error) {
	if action == "logs" {
		if !safeManagedName(p.Name) {
			return nil, errors.New("受管实例标识无效")
		}
		tail := p.Tail
		if tail == "" {
			tail = "300"
		}
		cmd := exec.Command("docker", "logs", "--tail", tail, "--timestamps", p.Name)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("读取容器日志失败: %s", strings.TrimSpace(string(out)))
		}
		lines := []string{}
		if text := strings.TrimSpace(string(out)); text != "" {
			lines = strings.Split(text, "\n")
		}
		return map[string]any{"lines": lines, "tail": tail, "truncated": false}, nil
	}
	if action == "files" || action == "file-read" || action == "file-write" || action == "file-upload" {
		dir, err := managedDir(cfg, p.Name)
		if err != nil {
			return nil, err
		}
		root := filepath.Join(dir, "workspace")
		if action == "file-read" {
			content, size, modifiedAt, err := agentcore.ReadWorkspaceText(root, p.Path)
			if err != nil {
				return nil, errors.New("文件不存在或无法访问")
			}
			_, rel, _ := agentcore.WorkspacePath(root, p.Path)
			return map[string]any{"path": rel, "content": content, "size": size, "modifiedAt": modifiedAt}, nil
		}
		if action == "file-write" {
			rel, err := agentcore.WriteWorkspaceText(root, p.Path, p.Content)
			if err != nil {
				return nil, err
			}
			return map[string]any{"path": rel}, nil
		}
		if action == "file-upload" {
			rel, size, err := agentcore.UploadWorkspaceBase64(root, p.Path, p.Content)
			if err != nil {
				return nil, err
			}
			return map[string]any{"path": rel, "size": size}, nil
		}
		rel, entries, err := agentcore.ListWorkspace(root, p.Path)
		if err != nil {
			return nil, errors.New("目录不存在或无法访问")
		}
		return map[string]any{"path": rel, "entries": entries}, nil
	}
	err := runManagedCompose(cfg, action, p)
	return nil, err
}
func safeManagedName(v string) bool {
	if len(v) < 8 || len(v) > 80 {
		return false
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}
func managedDir(cfg config, name string) (string, error) {
	if !safeManagedName(name) {
		return "", errors.New("受管实例标识无效")
	}
	root := os.Getenv("XCLOUD_CONTROL_DATA_ROOT")
	if root == "" {
		root = "/var/lib/xcloud-control/instances"
	}
	return agentcore.InstanceDir(root, name)
}
func runManagedCompose(cfg config, action string, p managedPayload) error {
	dir, err := managedDir(cfg, p.Name)
	if err != nil {
		return err
	}
	allowed := map[string]bool{"create": true, "start": true, "stop": true, "restart": true, "destroy": true, "purge": true, "resize": true, "reinstall": true, "bandwidth": true, "pull-image": true}
	if !allowed[action] {
		return errors.New("不支持的受管操作")
	}
	if action == "pull-image" {
		if p.Image == "" {
			return errors.New("镜像地址无效")
		}
		cmd := exec.Command("docker", "pull", p.Image)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("镜像拉取失败: %s", strings.TrimSpace(string(output)))
		}
		return nil
	}
	if action == "create" || action == "resize" || action == "reinstall" || action == "start" {
		if p.Image == "" || p.CPU <= 0 || p.MemoryMB <= 0 || !safeManagedName(p.Name) {
			return errors.New("实例配置无效")
		}
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(dir, "data"), 0700); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(dir, "workspace"), 0700); err != nil {
			return err
		}
		compose, composeErr := agentcore.Compose(agentcore.ComposeInput{Name: p.Name, Image: p.Image, Route: p.Route, DataDir: filepath.Join(dir, "data"), WorkspaceDir: filepath.Join(dir, "workspace"), Network: os.Getenv("XCLOUD_DOCKER_NETWORK"), CPU: p.CPU, MemoryMB: p.MemoryMB, BandwidthMbps: maxBandwidth(p.BandwidthMbps), TerminalMode: false})
		if composeErr != nil {
			return composeErr
		}
		if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(compose), 0600); err != nil {
			return err
		}
	}
	if action == "purge" {
		if err := composeCommand(dir, p.Name, "down", "--remove-orphans", "--volumes"); err != nil {
			return err
		}
		return os.RemoveAll(dir)
	}
	if action == "destroy" {
		return composeCommand(dir, p.Name, "down", "--remove-orphans")
	}
	switch action {
	case "stop":
		return composeCommand(dir, p.Name, "stop")
	case "restart":
		return composeCommand(dir, p.Name, "restart")
	case "create", "start", "resize", "reinstall":
		if p.KeepStopped {
			return composeCommand(dir, p.Name, "up", "-d", "--no-start")
		}
		return composeCommand(dir, p.Name, "up", "-d", "--remove-orphans")
	case "bandwidth":
		return nil
	}
	return nil
}
func maxBandwidth(value int) int {
	if value < 1 {
		return 10
	}
	return value
}
func composeCommand(dir, name string, args ...string) error {
	base := []string{"compose", "--project-name", name, "--file", filepath.Join(dir, "docker-compose.yml")}
	cmd := exec.Command("docker", append(base, args...)...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("受管 Compose 执行失败: %s", strings.TrimSpace(string(output)))
	}
	return nil
}
func handleV2Request(cfg config, f v2Frame, send func(v2Frame) error, pipes *struct {
	sync.RWMutex
	values map[string]*io.PipeWriter
}) {
	var meta v2Request
	if json.Unmarshal(f.meta, &meta) != nil {
		_ = send(v2Frame{kind: v2Error, id: f.id})
		return
	}
	reader, writer := io.Pipe()
	pipes.Lock()
	pipes.values[f.id] = writer
	pipes.Unlock()
	defer func() { pipes.Lock(); delete(pipes.values, f.id); pipes.Unlock(); _ = writer.Close() }()
	// v3 has no arbitrary local-target mode. Every public request must name a
	// managed instance route selected by the Gateway; this prevents an old
	// device configuration from exposing unrelated localhost services.
	route := meta.Headers.Get("X-Xcloud-Managed-Route")
	if route == "" {
		_ = send(v2Frame{kind: v2Error, id: f.id})
		return
	}
	target, err := managedRouteTarget(cfg, route)
	if err != nil {
		_ = send(v2Frame{kind: v2Error, id: f.id})
		return
	}
	req, err := http.NewRequest(meta.Method, target+meta.Path, reader)
	if err != nil {
		_ = send(v2Frame{kind: v2Error, id: f.id})
		return
	}
	req.Header = meta.Headers.Clone()
	req.Header.Del("Connection")
	req.Header.Del("Upgrade")
	req.Header.Del("Host")
	res, err := (&http.Client{Timeout: 0}).Do(req)
	if err != nil {
		_ = send(v2Frame{kind: v2Error, id: f.id})
		return
	}
	defer res.Body.Close()
	raw, _ := json.Marshal(v2Response{res.StatusCode, res.Header})
	if send(v2Frame{kind: v2ResponseStart, id: f.id, meta: raw}) != nil {
		return
	}
	buf := make([]byte, 32*1024)
	for {
		n, e := res.Body.Read(buf)
		if n > 0 {
			if send(v2Frame{kind: v2Data, id: f.id, data: append([]byte(nil), buf[:n]...)}) != nil {
				return
			}
		}
		if e != nil {
			_ = send(v2Frame{kind: v2End, id: f.id})
			return
		}
	}
}
func managedRouteTarget(cfg config, route string) (string, error) {
	if len(route) != 17 || !strings.HasPrefix(route, "r") {
		return "", errors.New("路由无效")
	}
	name := "xcloud-" + route
	cmd := exec.Command("docker", "inspect", "-f", `{{ index .Config.Labels "xcloud.managed" }}|{{ index .Config.Labels "xcloud.route" }}|{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}`, name)
	raw, err := cmd.Output()
	if err != nil {
		return "", errors.New("受管实例离线")
	}
	parts := strings.Split(strings.TrimSpace(string(raw)), "|")
	if len(parts) != 3 || parts[0] != "true" || parts[1] != route || parts[2] == "" {
		return "", errors.New("受管实例校验失败")
	}
	return "http://" + parts[2] + ":17390", nil
}
func writeV2(c *websocket.Conn, f v2Frame) error {
	buf := make([]byte, 7+len(f.id)+len(f.meta)+len(f.data))
	buf[0] = f.kind
	binary.BigEndian.PutUint16(buf[1:3], uint16(len(f.id)))
	binary.BigEndian.PutUint32(buf[3:7], uint32(len(f.meta)))
	copy(buf[7:], f.id)
	copy(buf[7+len(f.id):], f.meta)
	copy(buf[7+len(f.id)+len(f.meta):], f.data)
	return c.WriteMessage(websocket.BinaryMessage, buf)
}
func readV2(c *websocket.Conn) (v2Frame, error) {
	kind, data, err := c.ReadMessage()
	if err != nil {
		return v2Frame{}, err
	}
	if kind != websocket.BinaryMessage || len(data) < 7 {
		return v2Frame{}, errors.New("无效隧道帧")
	}
	il := int(binary.BigEndian.Uint16(data[1:3]))
	ml := int(binary.BigEndian.Uint32(data[3:7]))
	if len(data) < 7+il+ml {
		return v2Frame{}, errors.New("截断隧道帧")
	}
	return v2Frame{kind: data[0], id: string(data[7 : 7+il]), meta: data[7+il : 7+il+ml], data: data[7+il+ml:]}, nil
}
func base64Raw(v string) ([]byte, error) { return base64.RawStdEncoding.DecodeString(v) }
