package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstanceComposeCarriesPlanLimitsAndRuntimeTuning(t *testing.T) {
	compose := instanceCompose(createRequest{Name: "xcloud-12345678", Image: "registry.example/alemonx:latest", CPU: 4, MemoryMB: 8192, BandwidthMbps: 10, Route: "r0123456789abcdef"}, "/data/xcloud-12345678/data", "/data/xcloud-12345678/workspace")
	for _, expected := range []string{"container_name: \"xcloud-12345678\"", "cpus: \"4\"", "mem_limit: \"8192m\"", "memswap_limit: \"8192m\"", "GOMAXPROCS: \"4\"", "NODE_OPTIONS: \"--max-old-space-size=6144\"", "xcloud.bandwidth_mbps: \"10\"", "xcloud.route: \"r0123456789abcdef\"", "\"/data/xcloud-12345678/data:/root\"", "\"/data/xcloud-12345678/workspace:/app/workspace\""} {
		if !strings.Contains(compose, expected) {
			t.Fatalf("compose missing %q:\n%s", expected, compose)
		}
	}
	if strings.Contains(compose, "ports:") {
		t.Fatal("user container must not publish host ports")
	}
	for _, incompatible := range []string{"cap_drop:", "no-new-privileges", "cgroup: private", "pids_limit:"} {
		if strings.Contains(compose, incompatible) {
			t.Fatalf("official AlemonX runtime must not add %q:\n%s", incompatible, compose)
		}
	}
}

func TestPrepareInstanceDirsMigratesLegacyRootData(t *testing.T) {
	root := t.TempDir()
	instanceDir := filepath.Join(root, "xcloud-12345678")
	workspaceDir := filepath.Join(instanceDir, "workspace")
	homeDir := filepath.Join(instanceDir, "data")
	if err := os.MkdirAll(workspaceDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(instanceDir, instanceComposeFile), []byte("services: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(instanceDir, ".alemonxrc"), []byte("state"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspaceDir, "project.txt"), []byte("workspace"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := prepareInstanceDirs(instanceDir, homeDir, workspaceDir); err != nil {
		t.Fatalf("migrate legacy layout: %v", err)
	}
	if content, err := os.ReadFile(filepath.Join(homeDir, ".alemonxrc")); err != nil || string(content) != "state" {
		t.Fatalf("legacy root data was not moved: content=%q err=%v", content, err)
	}
	if _, err := os.Stat(filepath.Join(instanceDir, instanceComposeFile)); err != nil {
		t.Fatalf("compose file must remain at instance root: %v", err)
	}
	if content, err := os.ReadFile(filepath.Join(workspaceDir, "project.txt")); err != nil || string(content) != "workspace" {
		t.Fatalf("workspace must remain separate: content=%q err=%v", content, err)
	}
	if _, err := os.Stat(filepath.Join(instanceDir, instanceMigrationMarker)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("migration marker should be removed, got %v", err)
	}
}

func TestAgentProtocolDeclaresStableExecutionCapabilities(t *testing.T) {
	required := []string{
		"container.lifecycle.v1", "container.inspect.v1", "container.logs.v1",
		"container.list.v1", "container.compose.v1", "container.destroy.v1", "image.pull.v1",
		"container.compose.restart.v1",
		"image.inspect.v1", "image.list.v1", "route.proxy.v1", "node.resources.v1", "network.bandwidth.v1",
		"network.bandwidth.status.v1",
		"network.bandwidth.queue.v1",
	}
	declared := strings.Join(agentCapabilities, ",")
	for _, capability := range required {
		if !strings.Contains(declared, capability) {
			t.Fatalf("Agent status must declare %s", capability)
		}
	}
	if AgentAPIVersion < 1 {
		t.Fatal("Agent API version must be positive")
	}
}

func TestBandwidthIFBNameIsStableAndFitsLinuxInterfaceLimit(t *testing.T) {
	name := bandwidthIFBName("xcloud-0123456789abcdef0123456789abcdef")
	if name != bandwidthIFBName("xcloud-0123456789abcdef0123456789abcdef") {
		t.Fatal("IFB name must be stable")
	}
	if !strings.HasPrefix(name, "ifb-xc-") || len(name) > 15 {
		t.Fatalf("unsafe IFB name %q", name)
	}
}

func TestBurstBandwidthBorrowsIdleCapacityWithinConfiguredLimit(t *testing.T) {
	t.Setenv("XCLOUD_BANDWIDTH_BURST_MULTIPLIER", "5")
	if got := burstBandwidthMbps(6); got != 30 {
		t.Fatalf("burst rate = %d, want 30", got)
	}
	t.Setenv("XCLOUD_BANDWIDTH_BURST_MULTIPLIER", "invalid")
	if got := burstBandwidthMbps(6); got != 24 {
		t.Fatalf("fallback burst rate = %d, want 24", got)
	}
}

func TestBandwidthShapingIsOptIn(t *testing.T) {
	t.Setenv("XCLOUD_TRAFFIC_CONTROL_ENABLED", "")
	if bandwidthShapingEnabled() {
		t.Fatal("bandwidth shaping must be disabled by default")
	}
	t.Setenv("XCLOUD_ENABLE_BANDWIDTH_SHAPING", "true")
	if bandwidthShapingEnabled() {
		t.Fatal("legacy enable setting must not override the hard default-off switch")
	}
	t.Setenv("XCLOUD_TRAFFIC_CONTROL_ENABLED", "true")
	if !bandwidthShapingEnabled() {
		t.Fatal("explicit traffic-control opt-in was ignored")
	}
}
