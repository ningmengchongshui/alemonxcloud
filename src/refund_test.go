package cloud

import (
	"testing"
	"time"
)

func refundTime(day int) time.Time {
	return time.Date(2026, time.January, day, 12, 0, 0, 0, time.UTC)
}

func TestQuoteRefundKeepsThreeFullDaysAndMovesRenewals(t *testing.T) {
	segments := []refundSegment{
		{ID: "first", Status: orderActive, AmountFen: 3100, Start: refundTime(1), End: time.Date(2026, time.February, 1, 12, 0, 0, 0, time.UTC), Source: "wallet"},
		{ID: "renewal", Status: orderActive, AmountFen: 2800, Start: time.Date(2026, time.February, 1, 12, 0, 0, 0, time.UTC), End: time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC), Source: "wallet"},
	}
	quote, shift, index, err := quoteRefund(segments, "first", refundTime(11))
	if err != nil {
		t.Fatalf("quote refund: %v", err)
	}
	if index != 0 || quote.TotalDays != 31 || quote.RemainingDays != 21 || quote.RefundableDays != 18 {
		t.Fatalf("unexpected quote: %#v, index=%d", quote, index)
	}
	if shift != 18*refundDay || quote.RefundAmountFen != 1800 {
		t.Fatalf("unexpected refund calculation: shift=%s, amount=%d", shift, quote.RefundAmountFen)
	}
	if want := time.Date(2026, time.February, 11, 12, 0, 0, 0, time.UTC); !quote.ServiceEndsAt.Equal(want) {
		t.Fatalf("final service end = %s, want %s", quote.ServiceEndsAt, want)
	}
}

func TestQuoteRefundRejectsThreeDaysOrLess(t *testing.T) {
	segments := []refundSegment{{ID: "order", Status: orderActive, AmountFen: 1000, Start: refundTime(1), End: refundTime(10), Source: "wallet"}}
	if _, _, _, err := quoteRefund(segments, "order", refundTime(7)); err == nil {
		t.Fatal("expected remaining three days to be non-refundable")
	}
}

func TestQuoteRefundUsesWholeTwentyFourHourDays(t *testing.T) {
	start := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	end := start.Add(10*refundDay + 23*time.Hour)
	segments := []refundSegment{{ID: "order", Status: orderActive, AmountFen: 1000, Start: start, End: end, Source: "wallet"}}
	quote, _, _, err := quoteRefund(segments, "order", start.Add(4*refundDay+23*time.Hour))
	if err != nil {
		t.Fatalf("quote refund: %v", err)
	}
	if quote.TotalDays != 10 || quote.RemainingDays != 6 || quote.RefundableDays != 3 || quote.RefundAmountFen != 300 {
		t.Fatalf("must use complete 24-hour periods: %#v", quote)
	}
}
