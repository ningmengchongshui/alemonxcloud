package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstanceComposeCarriesPlanLimitsAndRuntimeTuning(t *testing.T) {
	compose := instanceCompose(createRequest{Name: "xcloud-12345678", Image: "registry.example/alemonx:latest", CPU: 4, MemoryMB: 8192, Route: "r0123456789abcdef"}, "/data/xcloud-12345678/data", "/data/xcloud-12345678/workspace")
	for _, expected := range []string{"container_name: \"xcloud-12345678\"", "cpus: \"4\"", "mem_limit: \"8192m\"", "memswap_limit: \"8192m\"", "GOMAXPROCS: \"4\"", "NODE_OPTIONS: \"--max-old-space-size=6144\"", "xcloud.route: \"r0123456789abcdef\"", "\"/data/xcloud-12345678/data:/root\"", "\"/data/xcloud-12345678/workspace:/app/workspace\""} {
		if !strings.Contains(compose, expected) {
			t.Fatalf("compose missing %q:\n%s", expected, compose)
		}
	}
	if strings.Contains(compose, "ports:") {
		t.Fatal("user container must not publish host ports")
	}
	if strings.Contains(compose, "bandwidth") || strings.Contains(compose, "Mbps") {
		t.Fatalf("Compose must not contain retired bandwidth settings:\n%s", compose)
	}
	for _, incompatible := range []string{"cap_drop:", "no-new-privileges", "cgroup: private", "pids_limit:"} {
		if strings.Contains(compose, incompatible) {
			t.Fatalf("official AlemonX runtime must not add %q:\n%s", incompatible, compose)
		}
	}
}

func TestInstanceComposeEnablesInteractiveTerminalModeOnlyWhenRequested(t *testing.T) {
	base := createRequest{Name: "xcloud-12345678", Image: "registry.example/alemonx:latest", CPU: 1, MemoryMB: 1024, Route: "r0123456789abcdef"}
	terminalCompose := instanceCompose(createRequest{Name: base.Name, Image: base.Image, CPU: base.CPU, MemoryMB: base.MemoryMB, Route: base.Route, TerminalMode: true}, "/data/home", "/data/workspace")
	if !strings.Contains(terminalCompose, "    stdin_open: true\n    tty: true\n") {
		t.Fatalf("terminal compose must allocate an interactive TTY:\n%s", terminalCompose)
	}
	webCompose := instanceCompose(base, "/data/home", "/data/workspace")
	if strings.Contains(webCompose, "stdin_open:") || strings.Contains(webCompose, "    tty:") {
		t.Fatalf("web compose must not receive terminal-only runtime settings:\n%s", webCompose)
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
		"container.compose.restart.v1", "container.compose.resize.v1", "container.reinstall.v1",
		"image.inspect.v1", "image.list.v1", "route.proxy.v1", "node.resources.v1",
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

func TestBandwidthShapingCannotBeReenabledByEnvironment(t *testing.T) {
	t.Setenv("XCLOUD_TRAFFIC_CONTROL_ENABLED", "true")
	if bandwidthShapingEnabled() {
		t.Fatal("retired traffic shaping must stay disabled")
	}
}
