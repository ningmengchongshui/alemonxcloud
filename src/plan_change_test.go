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

func TestPlanChangeRetainsOneCompleteOldPackageDay(t *testing.T) {
	now := time.Date(2026, 9, 7, 18, 30, 0, 0, time.UTC)
	if got := planChangeRefundAfter(now); !got.Equal(now.Add(24 * time.Hour)) {
		t.Fatalf("refund cutoff = %s, want %s", got, now.Add(24*time.Hour))
	}
}

func TestReplacementChargeUsesTargetThirtyDayRate(t *testing.T) {
	start := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
	if got := prorateMonthlyFen(2400, start, start.Add(15*24*time.Hour)); got != 1200 {
		t.Fatalf("replacement charge = %d, want 1200", got)
	}
}

func TestPlanChangeTierMonthsUsesCurrentSupportedTier(t *testing.T) {
	start := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	if got := planChangeTierMonths(start, start.Add(100*24*time.Hour)); got != 3 {
		t.Fatalf("100-day replacement tier = %d, want 3", got)
	}
	if got := planChangeTierMonths(start, start.Add(89*24*time.Hour)); got != 1 {
		t.Fatalf("short replacement tier = %d, want 1", got)
	}
}

func TestTierDiscountBpsIsPayableRateNotAmountOff(t *testing.T) {
	if bps := tierDiscountBps([]byte(`{"tierDiscountBps":8000}`)); bps != 8000 {
		t.Fatalf("tier bps = %d, want 8000", bps)
	}
	if monthly := 3000 * tierDiscountBps([]byte(`{"tierDiscountBps":8000}`)) / 10000; monthly != 2400 {
		t.Fatalf("8-discount monthly price = %d, want 2400", monthly)
	}
}

func TestPlanChangeRefundNeverTurnsOneDayRetentionIntoDebt(t *testing.T) {
	start := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	if got := planChangeOrderRefundFen(100, start, start.Add(12*time.Hour), start); got != 0 {
		t.Fatalf("short order refund = %d, want 0", got)
	}
	if got := planChangeOrderRefundFen(0, start, start.Add(30*24*time.Hour), start); got != 0 {
		t.Fatalf("free order refund = %d, want 0", got)
	}
	if got := planChangeOrderRefundFen(3000, start, start.Add(30*24*time.Hour), start); got != 2900 {
		t.Fatalf("30-day order refund = %d, want 2900", got)
	}
}
