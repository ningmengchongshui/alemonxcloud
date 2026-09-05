package cloud

import "testing"

func TestPromotionDiscountHonorsPercentCapAndList(t *testing.T) {
	p := promotion{DiscountType: "percent", DiscountValue: 2500, MaxDiscountFen: 120}
	if got := promotionDiscount(p, 1000); got != 120 {
		t.Fatalf("got %d, want 120", got)
	}
	p = promotion{DiscountType: "fixed", DiscountValue: 2000}
	if got := promotionDiscount(p, 999); got != 999 {
		t.Fatalf("got %d, want 999", got)
	}
	p = promotion{DiscountType: "percent", DiscountValue: 9500}
	if got := promotionDiscount(p, 1000); got != 50 {
		t.Fatalf("95 折 should discount 50, got %d", got)
	}
}

func TestCouponCodeNormalizationAndHash(t *testing.T) {
	if got := normalizeCouponCode(" xc-ab 12 "); got != "XC-AB12" {
		t.Fatalf("normalization: %q", got)
	}
	if couponHash("xc-ab12") != couponHash(" XC-AB12 ") {
		t.Fatal("equivalent coupon code must hash identically")
	}
}

func TestPromotionValidation(t *testing.T) {
	if err := validPromotion(promotion{Name: "新人", Kind: "newcomer", Scope: "purchase", DiscountType: "fixed", DiscountValue: 1}); err != nil {
		t.Fatal(err)
	}
	if err := validPromotion(promotion{Name: "套餐首购", Kind: "first_plan_purchase", Scope: "purchase", PlanIDs: []string{"plan-a"}, DiscountType: "fixed", DiscountValue: 1}); err != nil {
		t.Fatal(err)
	}
	if err := validPromotion(promotion{Name: "缺套餐", Kind: "first_plan_purchase", Scope: "purchase", DiscountType: "fixed", DiscountValue: 1}); err == nil {
		t.Fatal("expected first-plan promotion without plans to fail")
	}
	if err := validPromotion(promotion{Name: "新人续费", Kind: "newcomer", Scope: "renewal", DiscountType: "fixed", DiscountValue: 1}); err == nil {
		t.Fatal("expected newcomer renewal promotion to fail")
	}
	if err := validPromotion(promotion{Name: "bad", Kind: "campaign", Scope: "both", DiscountType: "percent", DiscountValue: 10001}); err == nil {
		t.Fatal("expected invalid percentage")
	}
}
