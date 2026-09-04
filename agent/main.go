package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

// Agent 作为宿主机 systemd 服务运行，绝不能作为 Docker 容器或暴露到公网。
var safeContainerName = regexp.MustCompile(`^xcloud-[a-z0-9]{8,32}$`)
var safeRouteKey = regexp.MustCompile(`^r[0-9a-f]{16}$`)

const (
	agentServiceName = "xcloud-agent.service"
	agentInstallDir  = "/opt/xcloud-agent"
	agentInstallPath = agentInstallDir + "/xcloud-agent"
	agentUnitPath    = "/etc/systemd/system/" + agentServiceName
)

type createRequest struct {
	Name     string  `json:"name" binding:"required"`
	Image    string  `json:"image" binding:"required"`
	CPU      float64 `json:"cpu" binding:"required"`
	MemoryMB int     `json:"memoryMB" binding:"required"`
	Route    string  `json:"route" binding:"required"`
}

func main() {
	serve := flag.Bool("serve", false, "run the HTTP agent (used by systemd)")
	flag.Parse()
	if !*serve {
		if err := installAndStartService(); err != nil {
			log.Fatal(err)
		}
		return
	}
	runServer()
}

// runServer is deliberately separate from bootstrap.  The default invocation
// installs the unit and exits; systemd invokes the installed binary with
// --serve, avoiding a recursive self-installation loop.
func runServer() {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Printf("load .env: %v", err)
	}
	gin.SetMode(env("GIN_MODE", gin.ReleaseMode))
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	control := r.Group("/container", requireControlToken)
	control.POST("/create", createContainer)
	control.POST("/:name/start", startContainer)
	control.POST("/:name/stop", stopContainer)
	control.DELETE("/:name", deleteContainer)
	// Nginx 在请求中写入实例路由键；控制接口会先于该路由命中。
	r.NoRoute(proxyContainer)
	address := env("AGENT_ADDR", "127.0.0.1:13092")
	log.Printf("xcloud agent listening on %s", address)
	if err := r.Run(address); err != nil {
		log.Fatal(err)
	}
}

func installAndStartService() error {
	if runtime.GOOS != "linux" {
		return errors.New("automatic service installation requires Linux with systemd; use --serve for direct development")
	}
	if os.Geteuid() != 0 {
		return errors.New("automatic service installation requires root; run: sudo ./xcloud-agent")
	}

	source, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate agent executable: %w", err)
	}
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		return fmt.Errorf("resolve agent executable: %w", err)
	}
	if err := installExecutable(source, agentInstallPath); err != nil {
		return err
	}
	if err := writeFileAtomically(agentUnitPath, []byte(systemdUnit()), 0644); err != nil {
		return fmt.Errorf("install systemd unit: %w", err)
	}
	for _, args := range [][]string{{"daemon-reload"}, {"enable", "--now", agentServiceName}, {"try-restart", agentServiceName}} {
		if err := runSystemctl(args...); err != nil {
			return err
		}
	}
	log.Printf("installed and started %s; inspect with: systemctl status %s", agentServiceName, strings.TrimSuffix(agentServiceName, ".service"))
	return nil
}

func installExecutable(source, destination string) error {
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("stat agent executable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("agent executable is not a regular file: %s", source)
	}
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open agent executable: %w", err)
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return fmt.Errorf("create agent install directory: %w", err)
	}
	output, err := os.CreateTemp(filepath.Dir(destination), ".xcloud-agent-*")
	if err != nil {
		return fmt.Errorf("create agent staging file: %w", err)
	}
	temporary := output.Name()
	defer os.Remove(temporary)
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		return fmt.Errorf("copy agent executable: %w", err)
	}
	if err := output.Chmod(0755); err != nil {
		output.Close()
		return fmt.Errorf("mark agent executable: %w", err)
	}
	if err := output.Close(); err != nil {
		return fmt.Errorf("close agent staging file: %w", err)
	}
	if err := os.Rename(temporary, destination); err != nil {
		return fmt.Errorf("activate agent executable: %w", err)
	}
	return nil
}

func writeFileAtomically(destination string, contents []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".xcloud-agent-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, destination)
}

func runSystemctl(args ...string) error {
	output, err := exec.Command("systemctl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func systemdUnit() string {
	return `[Unit]
Description=AlemonX Cloud bare-metal agent
After=docker.service network-online.target
Requires=docker.service

[Service]
Type=simple
User=root
WorkingDirectory=/opt/xcloud-agent
EnvironmentFile=-/etc/xcloud-agent.env
ExecStart=/opt/xcloud-agent/xcloud-agent --serve
Restart=always
RestartSec=3
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
`
}

func requireControlToken(c *gin.Context) {
	expected := env("XCLOUD_AGENT_TOKEN", "")
	provided := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	if expected == "" || subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) != 1 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "未授权的控制请求"})
		return
	}
	c.Next()
}

func createContainer(c *gin.Context) {
	var input createRequest
	if c.ShouldBindJSON(&input) != nil || !safeContainerName.MatchString(input.Name) || !safeRouteKey.MatchString(input.Route) || !validImage(input.Image) || input.CPU <= 0 || input.CPU > 64 || input.MemoryMB < 256 || input.MemoryMB > 262144 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "容器参数无效"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Minute)
	defer cancel()
	dataDir := env("XCLOUD_INSTANCE_DATA_ROOT", "/var/lib/xcloud/instances") + "/" + input.Name
	workspaceDir := dataDir + "/workspace"
	if err := os.MkdirAll(workspaceDir, 0750); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "无法准备实例数据目录"})
		return
	}
	args := []string{"run", "-d", "--name", input.Name, "--network", env("XCLOUD_DOCKER_NETWORK", "xcloud_network"), "--restart", "unless-stopped", "--shm-size", "1g", "--cpus", strconv.FormatFloat(input.CPU, 'f', -1, 64), "--memory", memory(input.MemoryMB), "--memory-swap", memory(input.MemoryMB), "--health-cmd", "curl -fsS http://127.0.0.1:17390/healthz >/dev/null", "--health-interval", "30s", "--health-timeout", "5s", "--health-retries", "3", "--health-start-period", "20s", "--env", "TZ=Asia/Shanghai", "--env", "HOME=/root", "--env", "XDG_CONFIG_HOME=/root/config", "--env", "XDG_CACHE_HOME=/root/cache", "--env", "ALX_DEPLOYMENT=production", "--env", "ALX_OPS_STORAGE=sqlite", "--env", "ALX_CONTAINER=1", "--env", "ALX_WORKSPACE=/app/workspace", "--env", "ALEMONJS_SETUP_ROOTS=/app/workspace", "--env", "ALX_PRIVILEGED_MODE=enabled", "--volume", dataDir + ":/root", "--volume", workspaceDir + ":/app/workspace", "--label", "xcloud.managed=true", "--label", "xcloud.route=" + input.Route, input.Image}
	if _, err := docker(ctx, args...); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"message": "Docker 创建容器失败"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"name": input.Name, "status": "running"})
}

func startContainer(c *gin.Context) { containerAction(c, "start") }
func stopContainer(c *gin.Context)  { containerAction(c, "stop") }
func deleteContainer(c *gin.Context) {
	name, ok := checkedName(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Minute)
	defer cancel()
	if _, err := docker(ctx, "rm", "-f", name); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"message": "Docker 删除容器失败"})
		return
	}
	c.Status(http.StatusNoContent)
}
func containerAction(c *gin.Context, action string) {
	name, ok := checkedName(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Minute)
	defer cancel()
	if _, err := docker(ctx, action, name); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"message": "Docker 操作失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"name": name, "status": action})
}
func checkedName(c *gin.Context) (string, bool) {
	name := c.Param("name")
	if !safeContainerName.MatchString(name) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "容器名称无效"})
		return "", false
	}
	return name, true
}

func proxyContainer(c *gin.Context) {
	route := c.GetHeader("X-Route-Key")
	if !safeRouteKey.MatchString(route) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "缺少实例路由信息"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	output, err := docker(ctx, "ps", "--filter", "label=xcloud.managed=true", "--filter", "label=xcloud.route="+route, "--format", "{{.Names}}")
	name := strings.TrimSpace(output)
	if err != nil || !safeContainerName.MatchString(name) {
		c.JSON(http.StatusNotFound, gin.H{"message": "未找到运行中的实例"})
		return
	}
	ip, err := docker(ctx, "inspect", "--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", name)
	if err != nil || net.ParseIP(strings.TrimSpace(ip)) == nil {
		c.JSON(http.StatusBadGateway, gin.H{"message": "实例网络暂不可用"})
		return
	}
	target, _ := url.Parse("http://" + strings.TrimSpace(ip) + ":17390")
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		http.Error(w, "实例暂不可用", http.StatusBadGateway)
	}
	c.Request.Header.Del("X-Route-Key")
	proxy.ServeHTTP(c.Writer, c.Request)
}

func docker(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		log.Printf("docker %s: %s", strings.Join(args[:min(2, len(args))], " "), strings.TrimSpace(string(out)))
	}
	return string(out), err
}
func validImage(value string) bool {
	return len(value) <= 255 && !strings.ContainsAny(value, " \t\n\r;&|`$()")
}
func memory(value int) string { return strconv.Itoa(value) + "m" }
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
