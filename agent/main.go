package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
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
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

// Agent 作为宿主机 systemd 服务运行，绝不能作为 Docker 容器或暴露到公网。
var safeContainerName = regexp.MustCompile(`^xcloud-[a-z0-9]{8,32}$`)
var safeRouteKey = regexp.MustCompile(`^r[0-9a-f]{16}$`)

// Version is injected by the release build.  The API version changes only for
// backwards-incompatible protocol changes; individual additions are announced
// through Capabilities instead, so the control plane can roll out safely.
var Version = "dev"

const AgentAPIVersion = 1

var agentCapabilities = []string{
	"container.lifecycle.v1",
	"container.inspect.v1",
	"container.logs.v1",
	"container.list.v1",
	"container.compose.v1",
	"container.destroy.v1",
	"image.pull.v1",
	"image.inspect.v1",
	"image.list.v1",
	"route.proxy.v1",
	"node.resources.v1",
	"network.bandwidth.v1",
	"network.bandwidth.status.v1",
	"network.bandwidth.queue.v1",
}

const (
	agentServiceName = "xcloud-agent.service"
	agentInstallDir  = "/opt/xcloud-agent"
	agentInstallPath = agentInstallDir + "/xcloud-agent"
	agentUnitPath    = "/etc/systemd/system/" + agentServiceName
)

type createRequest struct {
	Name          string  `json:"name" binding:"required"`
	Image         string  `json:"image" binding:"required"`
	CPU           float64 `json:"cpu" binding:"required"`
	MemoryMB      int     `json:"memoryMB" binding:"required"`
	BandwidthMbps int     `json:"bandwidthMbps" binding:"required"`
	Route         string  `json:"route" binding:"required"`
}

func main() {
	serve := flag.Bool("serve", false, "run the HTTP agent (used by systemd)")
	showVersion := flag.Bool("version", false, "print Agent version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Printf("xcloud-agent %s (api v%d)\n", Version, AgentAPIVersion)
		return
	}
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
	// Agent secrets belong to the node, never to the control-plane .env.
	if err := godotenv.Overload("agent/.env"); err != nil && !os.IsNotExist(err) {
		log.Printf("load agent/.env: %v", err)
	}
	gin.SetMode(env("GIN_MODE", gin.ReleaseMode))
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	control := r.Group("/container", requireControlToken)
	control.POST("/create", createContainer)
	control.POST("/pull", pullImage)
	control.GET("/status", agentStatus)
	control.GET("", listContainers)
	control.GET("/images", listImages)
	control.GET("/images/inspect", inspectImage)
	control.POST("/:name/start", startContainer)
	control.POST("/:name/stop", stopContainer)
	control.POST("/:name/restart", restartContainer)
	control.POST("/:name/destroy", destroyContainer)
	control.POST("/:name/bandwidth", applyContainerBandwidth)
	control.GET("/:name/status", containerStatus)
	control.GET("/:name/inspect", inspectContainer)
	control.GET("/:name/logs", containerLogs)
	control.DELETE("/:name", deleteContainer)
	// Nginx 在请求中写入实例路由键；控制接口会先于该路由命中。
	r.NoRoute(proxyContainer)
	// The control plane runs in Docker and reaches this service through the
	// Docker bridge gateway.  Exposure is constrained by the host firewall;
	// See docs/03-部署指南.md for the required allow rule.
	address := env("AGENT_ADDR", "0.0.0.0:13092")
	go reconcileBandwidthLoop()
	log.Printf("xcloud agent listening on %s", address)
	if err := r.Run(address); err != nil {
		log.Fatal(err)
	}
}

func pullImage(c *gin.Context) {
	var input struct {
		Image string `json:"image" binding:"required"`
	}
	if c.ShouldBindJSON(&input) != nil || !validManagedImage(input.Image) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "镜像参数无效"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Minute)
	defer cancel()
	if _, err := docker(ctx, "pull", input.Image); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"message": "镜像拉取失败"})
		return
	}
	metadata, err := inspectLocalImage(ctx, input.Image)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"message": "镜像拉取后无法确认摘要"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"image": input.Image, "status": "pulled", "imageID": metadata.id, "repoDigests": metadata.repoDigests})
}

type localImageMetadata struct {
	id          string
	repoDigests []string
	sizeBytes   string
}

func inspectLocalImage(ctx context.Context, image string) (localImageMetadata, error) {
	output, err := docker(ctx, "image", "inspect", "--format", "{{.Id}}|{{join .RepoDigests \",\"}}|{{.Size}}", image)
	if err != nil {
		return localImageMetadata{}, err
	}
	parts := strings.SplitN(strings.TrimSpace(output), "|", 3)
	item := localImageMetadata{}
	if len(parts) > 0 {
		item.id = parts[0]
	}
	if len(parts) > 1 && parts[1] != "" {
		item.repoDigests = strings.Split(parts[1], ",")
	}
	if len(parts) > 2 {
		item.sizeBytes = parts[2]
	}
	return item, nil
}

// listImages and inspectImage intentionally expose only Docker metadata. They
// are control-token protected and give the control plane a stable way to
// verify a selected image before a future deployment feature depends on it.
func listImages(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	output, err := docker(ctx, "image", "ls", "--format", "{{.Repository}}:{{.Tag}}|{{.ID}}|{{.Size}}")
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "Docker 镜像列表不可用"})
		return
	}
	items := make([]gin.H, 0)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 || parts[0] == "<none>:<none>" {
			continue
		}
		items = append(items, gin.H{"image": parts[0], "id": parts[1], "size": parts[2]})
	}
	c.JSON(http.StatusOK, gin.H{"images": items})
}

func inspectImage(c *gin.Context) {
	image := strings.TrimSpace(c.Query("image"))
	if !validManagedImage(image) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "镜像参数无效"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	metadata, err := inspectLocalImage(ctx, image)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "镜像尚未拉取"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"image": image, "id": metadata.id, "repoDigests": metadata.repoDigests, "sizeBytes": metadata.sizeBytes})
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
Description=ALemonX Cloud bare-metal agent
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
	if c.ShouldBindJSON(&input) != nil || !safeContainerName.MatchString(input.Name) || !safeRouteKey.MatchString(input.Route) || !validManagedImage(input.Image) || input.CPU <= 0 || input.CPU > 64 || input.MemoryMB < 256 || input.MemoryMB > 262144 || input.BandwidthMbps < 1 || input.BandwidthMbps > 10000 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "容器参数无效"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Minute)
	defer cancel()
	if _, err := docker(ctx, "inspect", input.Name); err == nil {
		// A worker may retry after the Docker command succeeded but before its
		// database acknowledgement. Treat the managed name as an idempotent key.
		respondWithBandwidthStatus(c, http.StatusOK, input.Name, "existing", applyBandwidthLimit(ctx, input.Name, input.BandwidthMbps))
		return
	}
	dataDir := env("XCLOUD_INSTANCE_DATA_ROOT", "/var/lib/xcloud/instances") + "/" + input.Name
	workspaceDir := dataDir + "/workspace"
	if err := os.MkdirAll(workspaceDir, 0750); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "无法准备实例数据目录"})
		return
	}
	composePath := filepath.Join(dataDir, "docker-compose.yml")
	if err := writeFileAtomically(composePath, []byte(instanceCompose(input, dataDir, workspaceDir)), 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "无法写入实例 Compose 配置"})
		return
	}
	if _, err := docker(ctx, "compose", "-p", composeProject(input.Route), "-f", composePath, "up", "-d", "--remove-orphans"); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"message": "Docker Compose 创建容器失败"})
		return
	}
	// Resource availability comes first. A transient traffic-control failure
	// must never turn a successful deployment into a destroyed container.
	respondWithBandwidthStatus(c, http.StatusCreated, input.Name, "running", applyBandwidthLimit(ctx, input.Name, input.BandwidthMbps))
}

func respondWithBandwidthStatus(c *gin.Context, code int, name, status string, err error) {
	if err != nil {
		log.Printf("bandwidth apply warning for %s: %v", name, err)
		c.JSON(code, gin.H{"name": name, "status": status, "bandwidthApplied": false, "bandwidthWarning": "带宽规则暂未应用，实例保持运行并将自动重试"})
		return
	}
	c.JSON(code, gin.H{"name": name, "status": status, "bandwidthApplied": true})
}

func bandwidthToolsAvailable() bool {
	for _, tool := range []string{"tc", "ip", "nsenter"} {
		if _, err := exec.LookPath(tool); err != nil {
			return false
		}
	}
	return true
}

// bandwidthQueueReady actively verifies the kernel path used by production
// instances. Checking only for tc/ip/nsenter binaries allowed nodes with a
// missing IFB module or qdisc support to advertise a capability they could not
// actually deliver.
func bandwidthQueueReady(ctx context.Context) bool {
	if !bandwidthToolsAvailable() {
		return false
	}
	probe := "ifb-xc-probe"
	_, _ = exec.CommandContext(ctx, "modprobe", "ifb").CombinedOutput()
	if _, err := exec.CommandContext(ctx, "ip", "link", "add", probe, "type", "ifb").CombinedOutput(); err != nil {
		return false
	}
	defer func() { _, _ = exec.CommandContext(ctx, "ip", "link", "del", "dev", probe).CombinedOutput() }()
	if _, err := exec.CommandContext(ctx, "ip", "link", "set", "dev", probe, "up").CombinedOutput(); err != nil {
		return false
	}
	if _, err := exec.CommandContext(ctx, "tc", "qdisc", "replace", "dev", probe, "root", "handle", "1:", "htb", "default", "10").CombinedOutput(); err != nil {
		return false
	}
	if _, err := exec.CommandContext(ctx, "tc", "class", "replace", "dev", probe, "parent", "1:", "classid", "1:10", "htb", "rate", "1mbit", "ceil", "1mbit").CombinedOutput(); err != nil {
		return false
	}
	_, err := exec.CommandContext(ctx, "tc", "qdisc", "replace", "dev", probe, "parent", "1:10", "handle", "10:", "fq_codel").CombinedOutput()
	return err == nil
}

func hostVethForContainer(name string) (string, error) {
	pid, err := docker(context.Background(), "inspect", "-f", "{{.State.Pid}}", name)
	if err != nil {
		return "", err
	}
	iflink, err := os.ReadFile(filepath.Join("/proc", strings.TrimSpace(pid), "root/sys/class/net/eth0/iflink"))
	if err != nil {
		return "", fmt.Errorf("read container iflink: %w", err)
	}
	want := strings.TrimSpace(string(iflink))
	paths, err := filepath.Glob("/sys/class/net/*/ifindex")
	if err != nil {
		return "", err
	}
	for _, path := range paths {
		value, readErr := os.ReadFile(path)
		if readErr == nil && strings.TrimSpace(string(value)) == want {
			return filepath.Base(filepath.Dir(path)), nil
		}
	}
	return "", errors.New("host veth not found")
}

func applyBandwidthLimit(ctx context.Context, name string, mbps int) error {
	if !bandwidthToolsAvailable() {
		return errors.New("tc/ip/nsenter unavailable")
	}
	host, err := hostVethForContainer(name)
	if err != nil {
		return err
	}
	rate := strconv.Itoa(mbps) + "mbit"
	// A root qdisc on the host-side veth throttles host -> container traffic,
	// i.e. package/image downloads made from inside the instance.  Plans limit
	// service egress, not a user's ability to install dependencies. Remove the
	// old two-way rule first so existing instances are corrected on reconcile.
	_, _ = exec.CommandContext(ctx, "tc", "qdisc", "del", "dev", host, "root").CombinedOutput()
	// Remove the legacy ingress police rule before installing clsact. Its packet
	// drops were the source of intermittent package-install and WebSocket pain.
	_, _ = exec.CommandContext(ctx, "tc", "qdisc", "del", "dev", host, "ingress").CombinedOutput()
	ifb, err := ensureBandwidthIFB(ctx, name)
	if err != nil {
		return err
	}
	if _, err = exec.CommandContext(ctx, "tc", "qdisc", "replace", "dev", host, "clsact").CombinedOutput(); err != nil {
		return err
	}
	// Redirect container egress into an IFB, where HTB enforces the plan rate
	// and FQ-CoDel queues fairly across flows instead of dropping bursts.
	if _, err = exec.CommandContext(ctx, "tc", "filter", "replace", "dev", host, "ingress", "protocol", "all", "pref", "10", "u32", "match", "u32", "0", "0", "action", "mirred", "egress", "redirect", "dev", ifb).CombinedOutput(); err != nil {
		return err
	}
	if _, err = exec.CommandContext(ctx, "tc", "qdisc", "replace", "dev", ifb, "root", "handle", "1:", "htb", "default", "10").CombinedOutput(); err != nil {
		return err
	}
	if _, err = exec.CommandContext(ctx, "tc", "class", "replace", "dev", ifb, "parent", "1:", "classid", "1:10", "htb", "rate", rate, "ceil", rate).CombinedOutput(); err != nil {
		return err
	}
	_, err = exec.CommandContext(ctx, "tc", "qdisc", "replace", "dev", ifb, "parent", "1:10", "handle", "10:", "fq_codel").CombinedOutput()
	return err
}

func bandwidthIFBName(containerName string) string {
	// Linux interface names max out at 15 characters. The prefix makes the
	// ownership obvious while the hash avoids collisions for long instance IDs.
	digest := sha256.Sum256([]byte(containerName))
	return fmt.Sprintf("ifb-xc-%x", digest[:5])
}

func ensureBandwidthIFB(ctx context.Context, containerName string) (string, error) {
	ifb := bandwidthIFBName(containerName)
	if _, err := exec.CommandContext(ctx, "ip", "link", "show", ifb).CombinedOutput(); err != nil {
		// IFB is commonly a module. A failed modprobe is not fatal when it is
		// built into the kernel, so attempt creation either way.
		_, _ = exec.CommandContext(ctx, "modprobe", "ifb").CombinedOutput()
		if output, createErr := exec.CommandContext(ctx, "ip", "link", "add", ifb, "type", "ifb").CombinedOutput(); createErr != nil {
			return "", fmt.Errorf("创建 IFB 队列接口失败: %w: %s", createErr, strings.TrimSpace(string(output)))
		}
	}
	if output, err := exec.CommandContext(ctx, "ip", "link", "set", "dev", ifb, "up").CombinedOutput(); err != nil {
		return "", fmt.Errorf("启用 IFB 队列接口失败: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return ifb, nil
}

func clearBandwidthLimit(ctx context.Context, name string) error {
	ifb := bandwidthIFBName(name)
	host, err := hostVethForContainer(name)
	if err != nil {
		// The container may already be gone after a crash or a retried destroy;
		// its dedicated IFB still needs cleanup.
		_, _ = exec.CommandContext(ctx, "tc", "qdisc", "del", "dev", ifb, "root").CombinedOutput()
		_, _ = exec.CommandContext(ctx, "ip", "link", "del", "dev", ifb).CombinedOutput()
		return nil
	}
	_, _ = exec.CommandContext(ctx, "tc", "qdisc", "del", "dev", host, "root").CombinedOutput()
	_, _ = exec.CommandContext(ctx, "tc", "qdisc", "del", "dev", host, "ingress").CombinedOutput()
	_, _ = exec.CommandContext(ctx, "tc", "qdisc", "del", "dev", host, "clsact").CombinedOutput()
	_, _ = exec.CommandContext(ctx, "tc", "qdisc", "del", "dev", ifb, "root").CombinedOutput()
	_, _ = exec.CommandContext(ctx, "ip", "link", "del", "dev", ifb).CombinedOutput()
	return nil
}

func applyContainerBandwidth(c *gin.Context) {
	if _, ok := checkedName(c); !ok {
		return
	}
	var input struct {
		BandwidthMbps int `json:"bandwidthMbps"`
	}
	if c.ShouldBindJSON(&input) != nil || input.BandwidthMbps < 1 || input.BandwidthMbps > 10000 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "带宽参数无效"})
		return
	}
	if err := applyBandwidthLimit(c.Request.Context(), c.Param("name"), input.BandwidthMbps); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"message": "带宽规则加载失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"name": c.Param("name"), "bandwidthApplied": true})
}

func reconcileBandwidthLoop() {
	reconcileBandwidth(context.Background())
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		reconcileBandwidth(context.Background())
	}
}
func reconcileBandwidth(ctx context.Context) {
	output, err := docker(ctx, "ps", "--filter", "label=xcloud.managed=true", "--format", "{{.Names}}|{{.Label \"xcloud.bandwidth_mbps\"}}")
	if err != nil {
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		parts := strings.Split(line, "|")
		if len(parts) != 2 || !safeContainerName.MatchString(parts[0]) {
			continue
		}
		mbps, e := strconv.Atoi(parts[1])
		if e == nil && mbps > 0 {
			if e = applyBandwidthLimit(ctx, parts[0], mbps); e != nil {
				log.Printf("restore bandwidth %s: %v", parts[0], e)
			}
		}
	}
}

// instanceCompose is deliberately generated by the Agent rather than accepted
// from callers: the control plane supplies only approved image/tag and plan
// limits, while the Agent owns all host-facing security and network settings.
func instanceCompose(input createRequest, dataDir, workspaceDir string) string {
	cpu := strconv.FormatFloat(input.CPU, 'f', -1, 64)
	nodeHeap := input.MemoryMB * 3 / 4
	if nodeHeap < 128 {
		nodeHeap = 128
	}
	env := map[string]string{
		"TZ": "Asia/Shanghai", "HOME": "/root", "XDG_CONFIG_HOME": "/root/config", "XDG_CACHE_HOME": "/root/cache",
		"ALX_DEPLOYMENT": "production", "ALX_OPS_STORAGE": "sqlite", "ALX_CONTAINER": "1", "ALX_WORKSPACE": "/app/workspace", "ALEMONJS_SETUP_ROOTS": "/app/workspace", "ALX_PRIVILEGED_MODE": "enabled",
		"GOMAXPROCS": cpu, "NODE_OPTIONS": fmt.Sprintf("--max-old-space-size=%d", nodeHeap), "UV_THREADPOOL_SIZE": cpu,
		"OMP_NUM_THREADS": cpu, "MKL_NUM_THREADS": cpu, "OPENBLAS_NUM_THREADS": cpu, "NUMEXPR_MAX_THREADS": cpu, "PYTHONUNBUFFERED": "1",
	}
	keys := []string{"TZ", "HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "ALX_DEPLOYMENT", "ALX_OPS_STORAGE", "ALX_CONTAINER", "ALX_WORKSPACE", "ALEMONJS_SETUP_ROOTS", "ALX_PRIVILEGED_MODE", "GOMAXPROCS", "NODE_OPTIONS", "UV_THREADPOOL_SIZE", "OMP_NUM_THREADS", "MKL_NUM_THREADS", "OPENBLAS_NUM_THREADS", "NUMEXPR_MAX_THREADS", "PYTHONUNBUFFERED"}
	var out strings.Builder
	fmt.Fprintf(&out, "# Managed by xcloud-agent. Manual edits apply on the next compose up.\nname: %s\nservices:\n  alemonx:\n    container_name: %s\n    image: %s\n    restart: unless-stopped\n    init: true\n    cgroup: private\n    cpus: %s\n    mem_limit: %s\n    memswap_limit: %s\n    pids_limit: 512\n    shm_size: 1g\n", composeProject(input.Route), yamlString(input.Name), yamlString(input.Image), yamlString(cpu), yamlString(memory(input.MemoryMB)), yamlString(memory(input.MemoryMB)))
	out.WriteString("    security_opt:\n      - no-new-privileges:true\n    cap_drop:\n      - ALL\n    healthcheck:\n      test: [\"CMD-SHELL\", \"curl -fsS http://127.0.0.1:17390/healthz >/dev/null\"]\n      interval: 30s\n      timeout: 5s\n      retries: 3\n      start_period: 20s\n    environment:\n")
	for _, key := range keys {
		fmt.Fprintf(&out, "      %s: %s\n", key, yamlString(env[key]))
	}
	fmt.Fprintf(&out, "    volumes:\n      - %s\n      - %s\n    labels:\n      xcloud.managed: \"true\"\n      xcloud.route: %s\n      xcloud.bandwidth_mbps: %q\n    networks:\n      - xcloud_network\nnetworks:\n  xcloud_network:\n    external: true\n    name: %s\n", yamlString(dataDir+":/root"), yamlString(workspaceDir+":/app/workspace"), yamlString(input.Route), strconv.Itoa(input.BandwidthMbps), yamlString(envString("XCLOUD_DOCKER_NETWORK", "xcloud_network")))
	return out.String()
}
func composeProject(route string) string    { return "xcloud-" + route }
func yamlString(value string) string        { encoded, _ := json.Marshal(value); return string(encoded) }
func envString(key, fallback string) string { return env(key, fallback) }

func startContainer(c *gin.Context)   { containerAction(c, "start") }
func stopContainer(c *gin.Context)    { containerAction(c, "stop") }
func restartContainer(c *gin.Context) { containerAction(c, "restart") }
func deleteContainer(c *gin.Context) {
	name, ok := checkedName(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Minute)
	defer cancel()
	if c.Query("purge") == "true" {
		dataDir := filepath.Join(env("XCLOUD_INSTANCE_DATA_ROOT", "/var/lib/xcloud/instances"), name)
		root := filepath.Clean(env("XCLOUD_INSTANCE_DATA_ROOT", "/var/lib/xcloud/instances")) + string(os.PathSeparator)
		if !strings.HasPrefix(filepath.Clean(dataDir)+string(os.PathSeparator), root) {
			c.JSON(http.StatusBadRequest, gin.H{"message": "实例数据目录无效"})
			return
		}
		composePath := filepath.Join(dataDir, "docker-compose.yml")
		if _, err := os.Stat(composePath); err == nil {
			if _, err := docker(ctx, "compose", "-f", composePath, "down", "--remove-orphans"); err != nil {
				c.JSON(http.StatusBadGateway, gin.H{"message": "Docker Compose 删除容器失败"})
				return
			}
		} else if _, err := docker(ctx, "rm", "-f", name); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"message": "Docker 删除容器失败"})
			return
		}
		if err := os.RemoveAll(dataDir); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "无法清理实例数据目录"})
			return
		}
	} else if _, err := docker(ctx, "rm", "-f", name); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"message": "Docker 删除容器失败"})
		return
	}
	c.Status(http.StatusNoContent)
}

// destroyContainer removes only running Docker resources. The Compose file and
// instance directory survive so the control plane can retain data for its
// recovery window before issuing the separate purge operation.
func destroyContainer(c *gin.Context) {
	name, ok := checkedName(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Minute)
	defer cancel()
	_ = clearBandwidthLimit(ctx, name)
	dataRoot := filepath.Clean(env("XCLOUD_INSTANCE_DATA_ROOT", "/var/lib/xcloud/instances"))
	dataDir := filepath.Join(dataRoot, name)
	if !strings.HasPrefix(filepath.Clean(dataDir)+string(os.PathSeparator), dataRoot+string(os.PathSeparator)) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "实例数据目录无效"})
		return
	}
	composePath := filepath.Join(dataDir, "docker-compose.yml")
	if _, err := os.Stat(composePath); err == nil {
		if _, err := docker(ctx, "compose", "-f", composePath, "down", "--remove-orphans"); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"message": "Docker Compose 销毁运行资源失败"})
			return
		}
	} else if _, err := docker(ctx, "rm", "-f", name); err != nil {
		// Docker returns an error when a retried destroy finds no container. A
		// missing managed container is already the desired idempotent outcome.
		if _, inspectErr := docker(ctx, "inspect", name); inspectErr == nil {
			c.JSON(http.StatusBadGateway, gin.H{"message": "Docker 销毁运行资源失败"})
			return
		}
	}
	c.Status(http.StatusNoContent)
}

func containerStatus(c *gin.Context) {
	name, ok := checkedName(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	output, err := docker(ctx, "inspect", "--format", "{{.State.Status}}|{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}", name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "容器不存在"})
		return
	}
	parts := strings.SplitN(strings.TrimSpace(output), "|", 2)
	status := parts[0]
	health := "none"
	if len(parts) == 2 {
		health = parts[1]
	}
	c.JSON(http.StatusOK, gin.H{"name": name, "status": status, "health": health})
}

func inspectContainer(c *gin.Context) {
	name, ok := checkedName(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	// Return lifecycle facts only. Environment variables and host mounts remain
	// private to the node even for authenticated control-plane callers.
	output, err := docker(ctx, "inspect", "--format", "{{.State.Status}}|{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}|{{.Image}}|{{.Created}}|{{.RestartCount}}", name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "容器不存在"})
		return
	}
	parts := strings.SplitN(strings.TrimSpace(output), "|", 5)
	result := gin.H{"name": name}
	if len(parts) > 0 {
		result["status"] = parts[0]
	}
	if len(parts) > 1 {
		result["health"] = parts[1]
	}
	if len(parts) > 2 {
		result["imageID"] = parts[2]
	}
	if len(parts) > 3 {
		result["createdAt"] = parts[3]
	}
	if len(parts) > 4 {
		result["restartCount"] = parts[4]
	}
	c.JSON(http.StatusOK, result)
}

func listContainers(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	output, err := docker(ctx, "ps", "-a", "--filter", "label=xcloud.managed=true", "--format", "{{.Names}}|{{.Status}}|{{.Image}}|{{.Label \"xcloud.route\"}}")
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "Docker 容器列表不可用"})
		return
	}
	items := make([]gin.H, 0)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		parts := strings.SplitN(line, "|", 4)
		if len(parts) != 4 || !safeContainerName.MatchString(parts[0]) || !safeRouteKey.MatchString(parts[3]) {
			continue
		}
		items = append(items, gin.H{"name": parts[0], "status": parts[1], "image": parts[2], "route": parts[3]})
	}
	c.JSON(http.StatusOK, gin.H{"containers": items})
}

func containerLogs(c *gin.Context) {
	name, ok := checkedName(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	output, err := docker(ctx, "logs", "--tail", "200", "--timestamps", name)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"message": "读取容器日志失败"})
		return
	}
	if len(output) > 64*1024 {
		output = output[len(output)-64*1024:]
	}
	c.JSON(http.StatusOK, gin.H{"lines": strings.Split(strings.TrimSpace(output), "\n")})
}

func agentStatus(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	output, err := docker(ctx, "info", "--format", "{{.ServerVersion}}")
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "Docker 不可用"})
		return
	}
	dataRoot := env("XCLOUD_INSTANCE_DATA_ROOT", "/var/lib/xcloud/instances")
	if err := os.MkdirAll(dataRoot, 0750); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "实例数据目录不可用"})
		return
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(dataRoot, &stat); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "实例数据目录不可用"})
		return
	}
	managed, err := docker(ctx, "ps", "-a", "--filter", "label=xcloud.managed=true", "--format", "{{.ID}}")
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "Docker 容器统计失败"})
		return
	}
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(managed), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	queueCheck, queueCancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	toolsReady := bandwidthQueueReady(queueCheck)
	queueCancel()
	capabilities := make([]string, 0, len(agentCapabilities))
	for _, capability := range agentCapabilities {
		if !toolsReady && (capability == "network.bandwidth.v1" || capability == "network.bandwidth.status.v1" || capability == "network.bandwidth.queue.v1") {
			continue
		}
		capabilities = append(capabilities, capability)
	}
	c.JSON(http.StatusOK, gin.H{
		"status": "ok", "agentVersion": Version, "apiVersion": AgentAPIVersion,
		"capabilities": capabilities, "bandwidthToolsReady": toolsReady, "dockerVersion": strings.TrimSpace(output),
		"cpuTotal": runtime.NumCPU(), "memoryTotalMB": hostMemoryMB(),
		"diskAvailableBytes": int64(stat.Bavail) * int64(stat.Bsize), "managedContainerCount": count,
	})
}

func hostMemoryMB() int {
	raw, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "MemTotal:" {
			value, err := strconv.Atoi(fields[1])
			if err == nil {
				return value / 1024
			}
		}
	}
	return 0
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
	if action == "start" || action == "restart" {
		raw, err := docker(ctx, "inspect", "-f", "{{ index .Config.Labels \"xcloud.bandwidth_mbps\" }}", name)
		mbps, parseErr := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || parseErr != nil || mbps < 1 || applyBandwidthLimit(ctx, name, mbps) != nil {
			// A bandwidth rule is an operational safeguard, not a reason to undo a
			// successful user-requested start. Stopping here made a stopped instance
			// impossible to recover when a node temporarily lost tc/ip/nsenter or its
			// veth could not be resolved. The periodic reconciler can retry later.
			log.Printf("bandwidth restore warning for %s after %s: inspect=%v parse=%v mbps=%d", name, action, err, parseErr, mbps)
			respondWithBandwidthStatus(c, http.StatusOK, name, action, errors.New("带宽规则恢复失败"))
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"name": name, "status": action, "bandwidthApplied": true})
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
func validDigestImage(value string) bool {
	parts := strings.Split(value, "@sha256:")
	if len(parts) != 2 || !validImage(parts[0]) || len(parts[1]) != 64 {
		return false
	}
	for _, char := range parts[1] {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return false
		}
	}
	return true
}
func validManagedImage(value string) bool {
	if validDigestImage(value) {
		return true
	}
	parts := strings.Split(value, ":")
	if len(parts) < 2 {
		return false
	}
	tag := parts[len(parts)-1]
	ref := strings.Join(parts[:len(parts)-1], ":")
	return tag != "" && validImage(ref) && !strings.ContainsAny(tag, " \t\r\n/@")
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
