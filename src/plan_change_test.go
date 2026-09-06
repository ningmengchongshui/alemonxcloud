package cloud

import (
	"testing"
	"time"
)

func TestCalculatePlanDeltaUsesRemainingServiceTime(t *testing.T) {
	now := time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)
	seconds, delta := calculatePlanDelta(1000, 2000, now.Add(15*24*time.Hour), now)
	if seconds != 15*24*60*60 {
		t.Fatalf("remaining seconds = %d", seconds)
	}
	if delta != 500 {
		t.Fatalf("delta = %d, want 500 fen", delta)
	}
}

func TestCalculatePlanDeltaClampsExpiredService(t *testing.T) {
	now := time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)
	seconds, delta := calculatePlanDelta(2000, 1000, now.Add(-time.Hour), now)
	if seconds != 0 || delta != 0 {
		t.Fatalf("expired quote = (%d, %d), want (0, 0)", seconds, delta)
	}
}

func TestResizeIsLifecycleTask(t *testing.T) {
	if !lifecycleTask("resize") {
		t.Fatal("resize must hold the instance lifecycle lock")
	}
}
