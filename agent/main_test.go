package main

import (
	"strings"
	"testing"
)

func TestInstanceComposeCarriesPlanLimitsAndRuntimeTuning(t *testing.T) {
	compose := instanceCompose(createRequest{Name: "xcloud-12345678", Image: "registry.example/alemonx:latest", CPU: 4, MemoryMB: 8192, BandwidthMbps: 10, Route: "r0123456789abcdef"}, "/data/xcloud-12345678", "/data/xcloud-12345678/workspace")
	for _, expected := range []string{"container_name: \"xcloud-12345678\"", "cpus: \"4\"", "mem_limit: \"8192m\"", "memswap_limit: \"8192m\"", "pids_limit: 512", "cgroup: private", "GOMAXPROCS: \"4\"", "NODE_OPTIONS: \"--max-old-space-size=6144\"", "xcloud.bandwidth_mbps: \"10\"", "xcloud.route: \"r0123456789abcdef\""} {
		if !strings.Contains(compose, expected) {
			t.Fatalf("compose missing %q:\n%s", expected, compose)
		}
	}
	if strings.Contains(compose, "ports:") {
		t.Fatal("user container must not publish host ports")
	}
}

func TestAgentProtocolDeclaresStableExecutionCapabilities(t *testing.T) {
	required := []string{
		"container.lifecycle.v1", "container.inspect.v1", "container.logs.v1",
		"container.list.v1", "container.compose.v1", "container.destroy.v1", "image.pull.v1",
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

func TestInstanceDownloadMbpsUsesSafeConfig(t *testing.T) {
	t.Setenv("XCLOUD_INSTANCE_DOWNLOAD_MBPS", "48")
	if got := instanceDownloadMbps(); got != 48 {
		t.Fatalf("download limit = %d, want 48", got)
	}
	t.Setenv("XCLOUD_INSTANCE_DOWNLOAD_MBPS", "invalid")
	if got := instanceDownloadMbps(); got != 20 {
		t.Fatalf("invalid download limit = %d, want fallback 20", got)
	}
}

func TestNodeDownloadBudgetMbpsUsesSafeConfig(t *testing.T) {
	t.Setenv("XCLOUD_NODE_DOWNLOAD_MBPS", "120")
	if got := nodeDownloadBudgetMbps(); got != 120 {
		t.Fatalf("node download budget = %d, want 120", got)
	}
	t.Setenv("XCLOUD_NODE_DOWNLOAD_MBPS", "0")
	if got := nodeDownloadBudgetMbps(); got != 20 {
		t.Fatalf("invalid node budget = %d, want fallback 20", got)
	}
}
