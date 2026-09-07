package cloud

import (
	"testing"
	"time"
)

func refundTime(day int) time.Time {
	return time.Date(2026, time.January, day, 12, 0, 0, 0, time.UTC)
}

func TestInstanceRefundStartsOnFourthShanghaiCalendarDay(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Date(2026, time.September, 7, 23, 59, 0, 0, loc)
	start := time.Date(2026, time.September, 1, 0, 0, 0, 0, loc)
	end := time.Date(2026, time.September, 20, 0, 0, 0, 0, loc)
	quote, err := quoteInstanceRefund([]refundSegment{{ID: "current", Status: orderActive, AmountFen: 1900, Start: start, End: end, Source: "wallet"}}, "current", now)
	if err != nil {
		t.Fatalf("instance quote: %v", err)
	}
	want := time.Date(2026, time.September, 10, 0, 0, 0, 0, loc)
	if !quote.ServiceEndsAt.Equal(want) || quote.RefundAmountFen != 1000 {
		t.Fatalf("quote = %#v, want cutoff %s and 1000", quote, want)
	}
}

func TestQuoteInstanceRefundRejectsDiscontinuousServiceChain(t *testing.T) {
	segments := []refundSegment{
		{ID: "first", Status: orderActive, AmountFen: 1000, Start: refundTime(1), End: refundTime(20), Source: "wallet"},
		{ID: "renewal", Status: orderActive, AmountFen: 1000, Start: refundTime(21), End: refundTime(31), Source: "wallet"},
	}
	if _, err := quoteInstanceRefund(segments, "first", refundTime(5)); err == nil {
		t.Fatal("discontinuous order service chain must require manual handling")
	}
}

func TestRefundAmountForSegmentUsesItsOwnActualPayment(t *testing.T) {
	segment := refundSegment{ID: "order", Status: orderActive, AmountFen: 3000, Start: refundTime(1), End: refundTime(31), Source: "wallet"}
	if got := refundAmountForSegment(segment, refundTime(16)); got != 1500 {
		t.Fatalf("refund amount = %d, want 1500", got)
	}
	free := segment
	free.AmountFen = 0
	if got := refundAmountForSegment(free, refundTime(16)); got != 0 {
		t.Fatalf("free order refund = %d, want 0", got)
	}
}
