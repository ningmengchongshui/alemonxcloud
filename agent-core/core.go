// Package agentcore contains the host-facing rules shared by xcloud-agent and
// xcloud-control. Transport adapters may only supply a validated instance
// descriptor; this package owns Compose generation and filesystem boundaries.
package agentcore

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var managedName = regexp.MustCompile(`^xcloud-[a-z0-9]{8,32}$`)
var routeKey = regexp.MustCompile(`^r[0-9a-f]{16}$`)

type ComposeInput struct {
	Name, Image, Route, DataDir, WorkspaceDir, Network string
	CPU                                                float64
	MemoryMB                                           int
	TerminalMode                                       bool
}

type WorkspaceEntry struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	Size       int64  `json:"size"`
	ModifiedAt string `json:"modifiedAt"`
}

const TextFileLimit = 1024 * 1024
const UploadFileLimit = 8 * 1024 * 1024

func ValidName(v string) bool  { return managedName.MatchString(v) }
func ValidRoute(v string) bool { return routeKey.MatchString(v) }

func InstanceDir(root, name string) (string, error) {
	if !ValidName(name) {
		return "", errors.New("受管实例标识无效")
	}
	return filepath.Join(root, name), nil
}

// WorkspacePath rejects traversal and all existing symlinks before returning a
// host path. Callers must use this for every read and write.
func WorkspacePath(root, raw string) (string, string, error) {
	rel := filepath.Clean(filepath.FromSlash(strings.TrimSpace(raw)))
	if rel == "." {
		rel = ""
	}
	if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", errors.New("工作区路径无效")
	}
	current := root
	for _, segment := range strings.Split(rel, string(filepath.Separator)) {
		if segment == "" {
			continue
		}
		current = filepath.Join(current, segment)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			break
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return "", "", errors.New("工作区路径无效")
		}
	}
	return filepath.Join(root, rel), filepath.ToSlash(rel), nil
}

// ListWorkspace never follows or exposes symlinks created from inside a
// container. The returned paths are always relative to the mounted workspace.
func ListWorkspace(root, raw string) (string, []WorkspaceEntry, error) {
	directory, relative, err := WorkspacePath(root, raw)
	if err != nil {
		return "", nil, err
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return "", nil, err
	}
	items := make([]WorkspaceEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		kind := "file"
		if entry.IsDir() {
			kind = "directory"
		}
		child := entry.Name()
		if relative != "" {
			child = relative + "/" + child
		}
		items = append(items, WorkspaceEntry{Name: entry.Name(), Path: child, Kind: kind, Size: info.Size(), ModifiedAt: info.ModTime().UTC().Format(time.RFC3339)})
	}
	return relative, items, nil
}

func ReadWorkspaceText(root, raw string) (string, int64, string, error) {
	path, _, err := WorkspacePath(root, raw)
	if err != nil {
		return "", 0, "", err
	}
	info, err := os.Lstat(path)
	if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", 0, "", os.ErrNotExist
	}
	if info.Size() > TextFileLimit {
		return "", 0, "", errors.New("文件超过 1 MB，无法在编辑器中打开")
	}
	content, err := os.ReadFile(path)
	if err != nil || !utf8.Valid(content) {
		return "", 0, "", errors.New("仅支持打开 UTF-8 文本文件")
	}
	return string(content), info.Size(), info.ModTime().UTC().Format(time.RFC3339), nil
}

func WriteWorkspaceText(root, raw, content string) (string, error) {
	if len(content) > TextFileLimit {
		return "", errors.New("文件内容超过 1 MB")
	}
	path, relative, err := WorkspacePath(root, raw)
	if err != nil {
		return "", err
	}
	if info, statErr := os.Lstat(path); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("工作区路径无效")
	}
	if err = os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return "", err
	}
	if err = os.WriteFile(path, []byte(content), 0600); err != nil {
		return "", err
	}
	return relative, nil
}

func UploadWorkspaceBase64(root, raw, encoded string) (string, int, error) {
	content, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(content) > UploadFileLimit {
		return "", 0, errors.New("上传文件无效或超过 8 MB")
	}
	path, relative, err := WorkspacePath(root, raw)
	if err != nil {
		return "", 0, err
	}
	if info, statErr := os.Lstat(path); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", 0, errors.New("工作区路径无效")
	}
	if err = os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return "", 0, err
	}
	if err = os.WriteFile(path, content, 0600); err != nil {
		return "", 0, err
	}
	return relative, len(content), nil
}

// TerminalDockerArgs deliberately offers no caller-selected shell or command.
func TerminalDockerArgs(name string) ([]string, error) {
	if !ValidName(name) {
		return nil, errors.New("受管实例标识无效")
	}
	return []string{"exec", "-i", "-t", "-e", "TERM=xterm-256color", "-e", "COLORTERM=truecolor", name, "/bin/sh", "-c", "if [ -x /bin/bash ]; then exec /bin/bash -i; elif [ -x /usr/bin/bash ]; then exec /usr/bin/bash -i; else exec /bin/sh -i; fi"}, nil
}

func Compose(input ComposeInput) (string, error) {
	if !ValidName(input.Name) || !ValidRoute(input.Route) || strings.TrimSpace(input.Image) == "" || input.CPU <= 0 || input.MemoryMB < 256 || input.DataDir == "" || input.WorkspaceDir == "" {
		return "", errors.New("实例配置无效")
	}
	if input.Network == "" {
		input.Network = "xcloud_network"
	}
	cpu := strconv.FormatFloat(input.CPU, 'f', -1, 64)
	heap := input.MemoryMB * 3 / 4
	if heap < 128 {
		heap = 128
	}
	stdin := ""
	if input.TerminalMode {
		stdin = "    stdin_open: true\n    tty: true\n"
	}
	return fmt.Sprintf("# Managed by xCloud. Manual edits are overwritten on the next lifecycle action.\nname: xcloud-%s\nservices:\n  alemonx:\n    container_name: %q\n    image: %q\n    restart: unless-stopped\n    init: true\n    cpus: %q\n    mem_limit: %q\n    memswap_limit: %q\n    shm_size: 1g\n%s    healthcheck:\n      test: [\"CMD-SHELL\", \"curl -fsS http://127.0.0.1:17390/healthz >/dev/null\"]\n      interval: 30s\n      timeout: 5s\n      retries: 3\n      start_period: 20s\n    environment:\n      TZ: \"Asia/Shanghai\"\n      HOME: \"/root\"\n      XDG_CONFIG_HOME: \"/root/config\"\n      XDG_CACHE_HOME: \"/root/cache\"\n      ALX_DEPLOYMENT: \"production\"\n      ALX_OPS_STORAGE: \"sqlite\"\n      ALX_CONTAINER: \"1\"\n      ALX_WORKSPACE: \"/app/workspace\"\n      ALEMONJS_SETUP_ROOTS: \"/app/workspace\"\n      ALX_PRIVILEGED_MODE: \"enabled\"\n      GOMAXPROCS: %q\n      NODE_OPTIONS: %q\n      UV_THREADPOOL_SIZE: %q\n      OMP_NUM_THREADS: %q\n      MKL_NUM_THREADS: %q\n      OPENBLAS_NUM_THREADS: %q\n      NUMEXPR_MAX_THREADS: %q\n      PYTHONUNBUFFERED: \"1\"\n    volumes:\n      - %q\n      - %q\n    labels:\n      xcloud.managed: \"true\"\n      xcloud.route: %q\n    networks:\n      - xcloud_network\nnetworks:\n  xcloud_network:\n    external: true\n    name: %q\n", input.Route, input.Name, input.Image, cpu, fmt.Sprintf("%dm", input.MemoryMB), fmt.Sprintf("%dm", input.MemoryMB), stdin, cpu, fmt.Sprintf("--max-old-space-size=%d", heap), cpu, cpu, cpu, cpu, cpu, input.DataDir+":/root", input.WorkspaceDir+":/app/workspace", input.Route, input.Network), nil
}
