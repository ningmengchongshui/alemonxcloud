package cloud

import (
	"testing"
	"time"
)

func TestCalculatePlanDeltaUsesRemainingThirtyDayBillingWindow(t *testing.T) {
	now := time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)
	seconds, delta := calculatePlanDelta(1000, 2000, now.Add(15*24*time.Hour), now)
	if seconds != int64(15*24*time.Hour/time.Second) {
		t.Fatalf("remaining seconds = %d", seconds)
	}
	if delta != 500 {
		t.Fatalf("half-month upgrade delta = %d fen, want 500", delta)
	}
}

func TestCalculatePlanDeltaClampsExpiredInstances(t *testing.T) {
	now := time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)
	seconds, delta := calculatePlanDelta(2000, 1000, now.Add(-time.Hour), now)
	if seconds != 0 || delta != 0 {
		t.Fatalf("expired plan change = (%d, %d), want (0, 0)", seconds, delta)
	}
}

func TestPlanChangeQuoteExpiryCannotBeExtendedByClient(t *testing.T) {
	now := time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)
	if !validPlanChangeQuoteExpiry(now.Add(5*time.Minute), now) {
		t.Fatal("a fresh five-minute quote should be accepted")
	}
	if validPlanChangeQuoteExpiry(now.Add(5*time.Minute+time.Second), now) {
		t.Fatal("client must not extend the quote window")
	}
	if validPlanChangeQuoteExpiry(time.Time{}, now) {
		t.Fatal("missing quote expiry must be rejected")
	}
}
