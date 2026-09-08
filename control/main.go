package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const version = "0.2.2"
const protocol = "xcloud-control.v3"

//go:embed xcloud-control.service
var systemdUnit []byte

type config struct{ ServerURL, TunnelURL, DeviceID, Credential, EnrollToken, PrivateKey, DeviceName, TargetURL, RouteStatus string }
type frame struct {
	Type, ID, Method, Path, TargetURL, Protocol, Error, DeviceName string
	Headers                                                        http.Header `json:"headers,omitempty"`
	Body                                                           []byte      `json:"body,omitempty"`
	Status                                                         int         `json:"status,omitempty"`
}

func configPath() string {
	if value := os.Getenv("XCLOUD_CONTROL_CONFIG"); value != "" {
		return value
	}
	return "/etc/xcloud-control/config.json"
}
func load(path string) (config, error) {
	var v config
	raw, err := os.ReadFile(path)
	if err != nil {
		return v, err
	}
	return v, json.Unmarshal(raw, &v)
}
func save(path string, v config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0600)
}
func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}
	switch os.Args[1] {
	case "version":
		fmt.Printf("xcloud-control %s (%s)\n", version, protocol)
	case "login":
		fatal("xcloud-control 已改为接入 Token 模式，请在 xCloud 控制台生成 Token 后写入配置文件")
	case "run":
		run(os.Args[2:])
	case "install":
		install(os.Args[2:])
	default:
		usage()
	}
}
func usage() { fmt.Fprintln(os.Stderr, "用法: xcloud-control <run|install|version>") }
func login(args []string) {
	fs := flag.NewFlagSet("login", flag.ExitOnError)
	server := fs.String("server", env("XCLOUD_CONTROL_SERVER", ""), "xCloud 地址")
	name := fs.String("name", hostname(), "设备名称")
	path := fs.String("config", configPath(), "配置文件")
	fs.Parse(args)
	if *server == "" {
		fatal("请使用 --server https://xcloud.example.com")
	}
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fatal(err.Error())
	}
	body, _ := json.Marshal(map[string]string{"name": *name, "publicKey": base64.RawStdEncoding.EncodeToString(pub), "clientVersion": version})
	res, err := http.Post(strings.TrimRight(*server, "/")+"/api/control/authorizations", "application/json", bytes.NewReader(body))
	if err != nil {
		fatal(err.Error())
	}
	defer res.Body.Close()
	var reply struct{ ID, PollToken, AuthorizeURL string }
	if res.StatusCode/100 != 2 {
		fatal(readMessage(res))
	}
	if err = json.NewDecoder(res.Body).Decode(&reply); err != nil {
		fatal(err.Error())
	}
	fmt.Printf("请在浏览器登录 xCloud 并打开以下链接完成绑定：\n%s\n", reply.AuthorizeURL)
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		req, _ := http.NewRequest(http.MethodGet, strings.TrimRight(*server, "/")+"/api/control/authorizations/"+url.PathEscape(reply.ID), nil)
		req.Header.Set("X-Control-Poll-Token", reply.PollToken)
		poll, err := http.DefaultClient.Do(req)
		if err != nil {
			continue
		}
		var got struct{ Status, DeviceID, Credential, TunnelURL string }
		_ = json.NewDecoder(poll.Body).Decode(&got)
		poll.Body.Close()
		if got.Status == "approved" && got.Credential != "" {
			if err = save(*path, config{ServerURL: strings.TrimRight(*server, "/"), TunnelURL: got.TunnelURL, DeviceID: got.DeviceID, Credential: got.Credential, PrivateKey: base64.RawStdEncoding.EncodeToString(private), DeviceName: *name}); err != nil {
				fatal(err.Error())
			}
			fmt.Printf("设备已绑定，凭证已保存至 %s。请执行 xcloud-control run。\n", *path)
			return
		}
	}
	fatal("等待设备授权超时")
}
func run(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	path := fs.String("config", configPath(), "配置文件")
	fs.Parse(args)
	cfg, err := load(*path)
	if err != nil {
		fatal("读取配置失败: " + err.Error())
	}
	if cfg.EnrollToken != "" && cfg.PrivateKey == "" {
		_, private, generateErr := ed25519.GenerateKey(rand.Reader)
		if generateErr != nil {
			fatal("生成设备密钥失败: " + generateErr.Error())
		}
		cfg.PrivateKey = base64.RawStdEncoding.EncodeToString(private)
		if err = save(*path, cfg); err != nil {
			fatal("保存设备密钥失败: " + err.Error())
		}
	}
	for delay := time.Second; ; delay = minDuration(delay*2, 30*time.Second) {
		if err := serveV2(&cfg, *path); err != nil {
			fmt.Fprintln(os.Stderr, "连接已断开:", err)
		}
		time.Sleep(delay)
	}
}
func serve(cfg config) error {
	u, err := url.Parse(cfg.ServerURL)
	if err != nil {
		return err
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else if u.Scheme == "http" {
		u.Scheme = "ws"
	} else {
		return errors.New("服务地址必须是 http 或 https")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/control/connect"
	header := http.Header{"Authorization": []string{"Bearer " + cfg.Credential}}
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), header)
	if err != nil {
		return err
	}
	defer conn.Close()
	var write sync.Mutex
	send := func(f frame) error {
		write.Lock()
		defer write.Unlock()
		f.Protocol = protocol
		return conn.WriteJSON(f)
	}
	if err := send(frame{Type: "hello", DeviceName: cfg.DeviceName}); err != nil {
		return err
	}
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case <-done:
				return
			case <-heartbeat.C:
				_ = send(frame{Type: "heartbeat", DeviceName: version})
			}
		}
	}()
	wsStreams := struct {
		sync.RWMutex
		values map[string]chan frame
	}{values: map[string]chan frame{}}
	for {
		conn.SetReadDeadline(time.Now().Add(50 * time.Second))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var f frame
		if err = json.Unmarshal(raw, &f); err != nil {
			return err
		}
		if f.Protocol != "" && f.Protocol != protocol {
			return errors.New("服务端隧道协议不兼容")
		}
		if f.Type == "config" {
			cfg.TargetURL = f.TargetURL
			cfg.RouteStatus = f.Error
			continue
		}
		if f.Type == "request" {
			go handleRequest(cfg, f, send)
			continue
		}
		if f.Type == "ws_open" {
			go handleWebsocket(cfg, f, send, &wsStreams)
			continue
		}
		if f.Type == "ws_data" || f.Type == "ws_close" {
			wsStreams.RLock()
			stream := wsStreams.values[f.ID]
			wsStreams.RUnlock()
			if stream != nil {
				select {
				case stream <- f:
				default:
				}
			}
			continue
		}
	}
}

func handleWebsocket(cfg config, open frame, send func(frame) error, streams *struct {
	sync.RWMutex
	values map[string]chan frame
}) {
	if cfg.RouteStatus != "enabled" || cfg.TargetURL == "" {
		_ = send(frame{Type: "error", ID: open.ID, Error: "入口未配置或已暂停"})
		return
	}
	target, err := url.Parse(strings.TrimRight(cfg.TargetURL, "/") + open.Path)
	if err != nil {
		_ = send(frame{Type: "error", ID: open.ID, Error: "本地目标无效"})
		return
	}
	if target.Scheme == "http" {
		target.Scheme = "ws"
	} else if target.Scheme == "https" {
		target.Scheme = "wss"
	}
	headers := open.Headers.Clone()
	headers.Del("Connection")
	headers.Del("Upgrade")
	headers.Del("Host")
	local, _, err := websocket.DefaultDialer.Dial(target.String(), headers)
	if err != nil {
		_ = send(frame{Type: "error", ID: open.ID, Error: "本地 WebSocket 服务不可用"})
		return
	}
	defer local.Close()
	inbound := make(chan frame, 32)
	streams.Lock()
	streams.values[open.ID] = inbound
	streams.Unlock()
	defer func() {
		streams.Lock()
		delete(streams.values, open.ID)
		streams.Unlock()
		_ = send(frame{Type: "ws_close", ID: open.ID})
	}()
	if err = send(frame{Type: "ws_ready", ID: open.ID}); err != nil {
		return
	}
	localInbound := make(chan frame, 32)
	go func() {
		defer close(localInbound)
		for {
			kind, data, err := local.ReadMessage()
			if err != nil {
				return
			}
			localInbound <- frame{Type: "ws_data", ID: open.ID, Status: kind, Body: data}
		}
	}()
	for {
		select {
		case item, ok := <-localInbound:
			if !ok {
				return
			}
			if err := send(item); err != nil {
				return
			}
		case item := <-inbound:
			if item.Type == "ws_close" {
				return
			}
			if err := local.WriteMessage(item.Status, item.Body); err != nil {
				return
			}
		}
	}
}
func handleRequest(cfg config, f frame, send func(frame) error) {
	if cfg.RouteStatus != "enabled" || cfg.TargetURL == "" {
		_ = send(frame{Type: "error", ID: f.ID, Error: "入口未配置或已暂停"})
		return
	}
	target := strings.TrimRight(cfg.TargetURL, "/") + f.Path
	req, err := http.NewRequest(f.Method, target, bytes.NewReader(f.Body))
	if err != nil {
		_ = send(frame{Type: "error", ID: f.ID, Error: "请求无效"})
		return
	}
	req.Header = f.Headers.Clone()
	req.Header.Del("Host")
	req.Header.Del("Connection")
	req.Header.Del("Upgrade")
	req.Header.Del("X-Control-Route-Key")
	client := &http.Client{Timeout: 60 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		_ = send(frame{Type: "error", ID: f.ID, Error: "本地服务不可用"})
		return
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		_ = send(frame{Type: "error", ID: f.ID, Error: "本地响应过大"})
		return
	}
	headers := res.Header.Clone()
	headers.Del("Connection")
	headers.Del("Upgrade")
	_ = send(frame{Type: "response", ID: f.ID, Status: res.StatusCode, Headers: headers, Body: body})
}
func install(args []string) {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	target := fs.String("target", "/usr/local/bin/xcloud-control", "安装路径")
	service := fs.String("service", "/etc/systemd/system/xcloud-control.service", "systemd 服务文件")
	fs.Parse(args)
	source, err := os.Executable()
	if err != nil {
		fatal(err.Error())
	}
	if err = copyFile(source, *target, 0755); err != nil {
		fatal("安装二进制失败: " + err.Error())
	}
	if err = os.WriteFile(*service, systemdUnit, 0644); err != nil {
		fatal("安装 systemd 服务失败: " + err.Error())
	}
	fmt.Printf("已安装 %s 和 %s。写入接入 Token 配置后执行 systemctl daemon-reload && systemctl enable --now xcloud-control。\n", *target, *service)
}
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	closeErr := out.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func readMessage(res *http.Response) string {
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	var body struct{ Message string }
	if json.Unmarshal(raw, &body) == nil && body.Message != "" {
		return body.Message
	}
	return res.Status
}
func hostname() string {
	v, _ := os.Hostname()
	if v == "" {
		return "xcloud-control"
	}
	return v
}
func env(k, f string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return f
}
func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
func fatal(s string) { fmt.Fprintln(os.Stderr, s); os.Exit(1) }
