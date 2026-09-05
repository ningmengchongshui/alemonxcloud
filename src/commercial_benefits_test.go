package cloud

import "testing"

func TestBenefitDiscountAndDays(t *testing.T) {
	if discount, days := benefitDiscount(benefitProgram{BenefitType: "fixed_discount", BenefitValue: 1200}, 1000); discount != 1000 || days != 0 {
		t.Fatalf("fixed discount = %d days=%d", discount, days)
	}
	if discount, days := benefitDiscount(benefitProgram{BenefitType: "percent_discount", BenefitValue: 1500}, 2000); discount != 300 || days != 0 {
		t.Fatalf("percent discount = %d days=%d", discount, days)
	}
	if discount, days := benefitDiscount(benefitProgram{BenefitType: "bonus_days", BenefitValue: 7}, 2000); discount != 0 || days != 7 {
		t.Fatalf("bonus benefit = %d days=%d", discount, days)
	}
}

func TestBenefitProgramValidationAndCodeNormalization(t *testing.T) {
	valid := benefitProgram{Name: "首购立减", Goal: "first_purchase", Status: "draft", TriggerType: "automatic", OrderScope: "purchase", BenefitType: "fixed_discount", BenefitValue: 1, AudienceType: "all"}
	if err := validBenefitProgram(valid); err != nil {
		t.Fatal(err)
	}
	valid.BenefitType = "percent_discount"
	valid.BenefitValue = 10001
	if err := validBenefitProgram(valid); err == nil {
		t.Fatal("expected invalid percentage")
	}
	if benefitCodeHash(" xc-ab 12 ") != benefitCodeHash("XC-AB12") {
		t.Fatal("equivalent codes need stable hashing")
	}
}

func TestRenewalAudienceRequiresCurrentInstance(t *testing.T) {
	p := benefitProgram{AudienceType: "expiring", Status: "active", OrderScope: "renewal", TriggerType: "automatic"}
	if p.AudienceType != "expiring" || p.OrderScope != "renewal" {
		t.Fatal("test fixture invalid")
	}
	// programEligible returns false before querying when no renewal instance is supplied.
	ok, err := programEligible(nil, nil, p, "owner", "renewal", "plan", "", 1, 100, new(string), false)
	if err != nil || ok {
		t.Fatalf("missing current instance must not qualify: ok=%v err=%v", ok, err)
	}
}

func TestSubscriptionMonthsExposeEveryTierChoice(t *testing.T) {
	for _, months := range []int{1, 3, 6, 12} {
		if !validSubscriptionMonths(months) {
			t.Fatalf("%d months must be purchasable", months)
		}
	}
	for _, months := range []int{0, 2, 4, 24} {
		if validSubscriptionMonths(months) {
			t.Fatalf("%d months must not bypass the product choices", months)
		}
	}
}
