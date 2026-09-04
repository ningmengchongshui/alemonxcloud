package main

import (
	"strings"
	"testing"
)

func TestInstanceComposeCarriesPlanLimitsAndRuntimeTuning(t *testing.T) {
	compose := instanceCompose(createRequest{Name: "xcloud-12345678", Image: "registry.example/alemonx:latest", CPU: 4, MemoryMB: 8192, Route: "r0123456789abcdef"}, "/data/xcloud-12345678", "/data/xcloud-12345678/workspace")
	for _, expected := range []string{"container_name: \"xcloud-12345678\"", "cpus: \"4\"", "mem_limit: \"8192m\"", "memswap_limit: \"8192m\"", "pids_limit: 512", "cgroup: private", "GOMAXPROCS: \"4\"", "NODE_OPTIONS: \"--max-old-space-size=6144\"", "OMP_NUM_THREADS: \"4\"", "xcloud.route: \"r0123456789abcdef\""} {
		if !strings.Contains(compose, expected) {
			t.Fatalf("compose missing %q:\n%s", expected, compose)
		}
	}
	if strings.Contains(compose, "ports:") {
		t.Fatal("user container must not publish host ports")
	}
}
