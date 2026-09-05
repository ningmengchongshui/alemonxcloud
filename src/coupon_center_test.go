package cloud

import "testing"

func TestCouponBatchValidation(t *testing.T) {
	base := couponBatch{Name: "公开券", Status: "active", DistributionMode: "public", DiscountType: "fixed", DiscountValue: 100, Scope: "purchase", PerUserLimit: 1}
	if err := validCouponBatch(base); err != nil {
		t.Fatal(err)
	}
	base.PerUserLimit = 0
	if err := validCouponBatch(base); err == nil {
		t.Fatal("expected per-user limit validation")
	}
	base.PerUserLimit = 1
	base.DistributionMode = "code"
	if err := validCouponBatch(base); err == nil {
		t.Fatal("expected unsupported distribution mode")
	}
}
