package main

import "testing"

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
