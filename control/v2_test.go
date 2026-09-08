package main

import (
	"errors"
	"strings"
	"testing"
)

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
