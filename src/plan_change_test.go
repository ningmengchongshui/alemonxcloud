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

func TestReplacementChargeUsesTargetThirtyDayRate(t *testing.T) {
	start := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
	if got := prorateMonthlyFen(2400, start, start.Add(15*24*time.Hour)); got != 1200 {
		t.Fatalf("replacement charge = %d, want 1200", got)
	}
}

func TestPlanChangeTierRateAppliesToEveryRemainingDay(t *testing.T) {
	start := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
	end := start.Add(100 * 24 * time.Hour)
	if tier := planChangeTierMonths(start, end); tier != 3 {
		t.Fatalf("100 remaining days should use the 3-month tier, got %d", tier)
	}
	// 3,000 fen monthly at an 8-discount tier is 2,400 fen/month. Every one
	// of the remaining 100 days uses that same 80 fen/day rate.
	if charge := prorateMonthlyFen(2400, start, end); charge != 8000 {
		t.Fatalf("100 days at the selected tier rate = %d, want 8000", charge)
	}
}

func TestProrationDoesNotOverflowForLongServiceWindows(t *testing.T) {
	start := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
	end := start.Add(12 * 30 * 24 * time.Hour)
	if charge := prorateMonthlyFen(240000, start, end); charge != 2880000 {
		t.Fatalf("12 months of target price = %d, want 2880000", charge)
	}
	if credit := prorateFen(2880000, start, end, start.Add(6*30*24*time.Hour)); credit != 1440000 {
		t.Fatalf("half of a long real-paid order = %d, want 1440000", credit)
	}
}

func TestReplacementChargeNeverAcceptsNegativeMonthlyPrice(t *testing.T) {
	start := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
	if got := prorateMonthlyFen(-2400, start, start.Add(15*24*time.Hour)); got != 0 {
		t.Fatalf("negative monthly price must not create a negative charge: %d", got)
	}
}

func TestPlanChangeUsesRealPaidValueForUnusedTime(t *testing.T) {
	start := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	if got := planChangeOrderRefundFen(100, start, start.Add(12*time.Hour), start); got != 100 {
		t.Fatalf("short order refund = %d, want 100", got)
	}
	if got := planChangeOrderRefundFen(0, start, start.Add(30*24*time.Hour), start); got != 0 {
		t.Fatalf("free order refund = %d, want 0", got)
	}
	if got := planChangeOrderRefundFen(3000, start, start.Add(30*24*time.Hour), start); got != 3000 {
		t.Fatalf("30-day order refund = %d, want 3000", got)
	}
	if got := planChangeOrderRefundFen(3000, start, start.Add(30*24*time.Hour), start.Add(10*24*time.Hour)); got != 2000 {
		t.Fatalf("20 days of real paid value = %d, want 2000", got)
	}
}
