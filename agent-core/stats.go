package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"time"
)

// ContainerStats contains only public runtime counters, never host paths or environment.
type ContainerStats struct {
	CPUPercent    string `json:"cpuPercent"`
	MemoryUsage   string `json:"memoryUsage"`
	MemoryPercent string `json:"memoryPercent"`
	NetworkIO     string `json:"networkIO"`
	BlockIO       string `json:"blockIO"`
	PIDs          string `json:"pids"`
	SampledAt     string `json:"sampledAt"`
}

func ParseContainerStats(raw []byte) (ContainerStats, error) {
	var row struct{ CPUPerc, MemUsage, MemPerc, NetIO, BlockIO, PIDs string }
	if json.Unmarshal(raw, &row) != nil || strings.TrimSpace(row.CPUPerc) == "" || strings.TrimSpace(row.MemUsage) == "" {
		return ContainerStats{}, errors.New("容器监控数据无效")
	}
	return ContainerStats{row.CPUPerc, row.MemUsage, row.MemPerc, row.NetIO, row.BlockIO, row.PIDs, time.Now().UTC().Format(time.RFC3339)}, nil
}

func ReadContainerStats(ctx context.Context, name string) (ContainerStats, error) {
	if !ValidName(name) {
		return ContainerStats{}, errors.New("受管实例标识无效")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	// A single sample bounds the process lifetime even if the HTTP client disconnects.
	raw, err := exec.CommandContext(ctx, "docker", "stats", "--no-stream", "--format", "{{json .}}", name).Output()
	if err != nil {
		return ContainerStats{}, errors.New("读取容器监控失败")
	}
	return ParseContainerStats(raw)
}
