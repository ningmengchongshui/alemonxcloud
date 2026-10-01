package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedComposeHostAccessSurvivesRestartAndCanBeRemoved(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XCLOUD_CONTROL_DATA_ROOT", root)
	t.Setenv("XCLOUD_DOCKER_NETWORK", "xcloud_network")
	bin := t.TempDir()
	logPath := filepath.Join(bin, "calls")
	t.Setenv("XCLOUD_TEST_DOCKER_LOG", logPath)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$XCLOUD_TEST_DOCKER_LOG\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	previous := controlDockerCommand
	t.Cleanup(func() { controlDockerCommand = previous })
	controlDockerCommand = func(args ...string) (string, error) { return "ok", nil }
	p := managedPayload{Name: "xcloud-12345678", Image: "example/app:latest", CPU: 1, MemoryMB: 1024, Route: "r0123456789abcdef", HostAccess: true}
	for _, step := range []struct {
		action           string
		enabled, stopped bool
	}{{"create", true, false}, {"restart", true, false}, {"resize", false, true}} {
		p.HostAccess, p.KeepStopped = step.enabled, step.stopped
		if err := runManagedCompose(context.Background(), config{}, step.action, p); err != nil {
			t.Fatal(err)
		}
		path, err := managedDir(config{}, p.Name)
		if err != nil {
			t.Fatal(err)
		}
		compose, err := os.ReadFile(filepath.Join(path, "docker-compose.yml"))
		if err != nil || strings.Contains(string(compose), "host.docker.internal:host-gateway") != step.enabled {
			t.Fatalf("%s Compose mapping: %s %v", step.action, compose, err)
		}
	}
	commands, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(commands), " restart") || !strings.Contains(string(commands), "up -d --no-start") {
		t.Fatalf("must reconcile Compose and preserve stopped state: %s", commands)
	}
}

func TestMemoryMBFromMeminfo(t *testing.T) {
	if got := memoryMBFromMeminfo("MemFree: 1 kB\nMemTotal: 16777216 kB\n"); got != 16384 {
		t.Fatalf("memoryMBFromMeminfo() = %d, want 16384", got)
	}
	if got := memoryMBFromMeminfo("MemTotal: 0 kB\n"); got != 0 {
		t.Fatalf("zero MemTotal = %d, want 0", got)
	}
}

func TestControlDataRootHonorsExplicitConfiguration(t *testing.T) {
	t.Setenv("XCLOUD_CONTROL_DATA_ROOT", "/tmp/xcloud-control-test-data")
	if got := controlDataRoot(); got != "/tmp/xcloud-control-test-data" {
		t.Fatalf("controlDataRoot() = %q", got)
	}
}

func TestRuntimeReadinessCreatesMissingManagedNetwork(t *testing.T) {
	t.Setenv("XCLOUD_CONTROL_DATA_ROOT", t.TempDir())
	t.Setenv("XCLOUD_DOCKER_NETWORK", "xcloud_network")
	previous := controlDockerCommand
	t.Cleanup(func() { controlDockerCommand = previous })
	created := 0
	controlDockerCommand = func(args ...string) (string, error) {
		command := strings.Join(args, " ")
		switch {
		case strings.HasPrefix(command, "info "), strings.HasPrefix(command, "compose version"):
			return "ok", nil
		case strings.HasPrefix(command, "network inspect") && created == 0:
			return "network not found", errors.New("missing")
		case strings.HasPrefix(command, "network create"):
			created++
			return "network-id", nil
		case strings.HasPrefix(command, "network inspect"):
			return "network-id", nil
		default:
			return "", nil
		}
	}
	if got := checkRuntimeReadiness(); !got.Ready {
		t.Fatalf("runtime readiness = %#v", got)
	}
	if created != 1 {
		t.Fatalf("network create calls = %d, want 1", created)
	}
}

func TestRuntimeReadinessDoesNotCreateNetworkWithoutCompose(t *testing.T) {
	t.Setenv("XCLOUD_CONTROL_DATA_ROOT", t.TempDir())
	previous := controlDockerCommand
	t.Cleanup(func() { controlDockerCommand = previous })
	networkCall := false
	controlDockerCommand = func(args ...string) (string, error) {
		command := strings.Join(args, " ")
		if strings.HasPrefix(command, "network ") {
			networkCall = true
		}
		if strings.HasPrefix(command, "info ") {
			return "ok", nil
		}
		if strings.HasPrefix(command, "compose version") {
			return "", errors.New("compose missing")
		}
		return "", nil
	}
	got := checkRuntimeReadiness()
	if got.Ready || len(got.Issues) != 1 || got.Issues[0].Code != "compose_unavailable" {
		t.Fatalf("runtime readiness = %#v", got)
	}
	if networkCall {
		t.Fatal("network must not be created when compose is unavailable")
	}
}

func TestLocalAgentStatusDoesNotClaimInventoryWhenDockerListFails(t *testing.T) {
	t.Setenv("XCLOUD_CONTROL_DATA_ROOT", t.TempDir())
	previous := controlDockerCommand
	t.Cleanup(func() { controlDockerCommand = previous })
	controlDockerCommand = func(args ...string) (string, error) {
		command := strings.Join(args, " ")
		switch {
		case strings.HasPrefix(command, "info "), strings.HasPrefix(command, "compose version"), strings.HasPrefix(command, "network inspect"):
			return "ok", nil
		case strings.HasPrefix(command, "ps "):
			return "", errors.New("docker list timed out")
		default:
			return "", nil
		}
	}
	status := localAgentStatus()
	inventoryOK, ok := status["runtimeInventoryOK"].(bool)
	if !ok || inventoryOK {
		t.Fatalf("runtime inventory must be false after a failed Docker list: %#v", status)
	}
}

func TestManagedOperationPathStaysOutsideInstanceData(t *testing.T) {
	t.Setenv("XCLOUD_CONTROL_DATA_ROOT", t.TempDir())
	path, err := managedOperationPath(config{}, "xcloud-r0123456789abcdef", "aop_12345678")
	if err != nil {
		t.Fatalf("managedOperationPath: %v", err)
	}
	if strings.Contains(path, "/instances/") || !strings.Contains(path, "/operations/") {
		t.Fatalf("operation path must survive instance purge: %s", path)
	}
}

func TestDockerObjectNotFoundIsIdempotentCleanup(t *testing.T) {
	for _, output := range []string{"Error response from daemon: No such container: xcloud-01234567", "Error: No such object: xcloud-01234567"} {
		if !dockerObjectNotFound(output) {
			t.Fatalf("expected missing Docker object to be harmless: %q", output)
		}
	}
	if dockerObjectNotFound("permission denied") {
		t.Fatal("permission failure must not be treated as a successful cleanup")
	}
}
