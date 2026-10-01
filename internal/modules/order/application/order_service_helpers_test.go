package application

import (
	"testing"

	orderdomain "github.com/dujiao-next/internal/modules/order/domain"

	coupondomain "github.com/dujiao-next/internal/modules/coupon/domain"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/shared/jsonmap"

	"github.com/shopspring/decimal"
)

func TestMergeCreateOrderItems(t *testing.T) {
	items := []CreateOrderItem{
		{ProductID: 1, SKUID: 10, Quantity: 1, FulfillmentType: "auto"},
		{ProductID: 1, SKUID: 10, Quantity: 2, FulfillmentType: "auto"},
		{ProductID: 1, SKUID: 11, Quantity: 1, FulfillmentType: "auto"},
		{ProductID: 2, SKUID: 20, Quantity: 1, FulfillmentType: ""},
	}
	merged, err := mergeCreateOrderItems(items)
	if err != nil {
		t.Fatalf("mergeCreateOrderItems error: %v", err)
	}
	if len(merged) != 3 {
		t.Fatalf("expected 3 items, got %d", len(merged))
	}
	if merged[0].ProductID != 1 || merged[0].SKUID != 10 || merged[0].Quantity != 3 {
		t.Fatalf("unexpected merged item: %+v", merged[0])
	}
	if merged[0].FulfillmentType != "" {
		t.Fatalf("expected empty fulfillment type, got: %s", merged[0].FulfillmentType)
	}
}

func TestMergeCreateOrderItemsConflict(t *testing.T) {
	items := []CreateOrderItem{
		{ProductID: 1, SKUID: 10, Quantity: 1, FulfillmentType: "auto"},
		{ProductID: 1, SKUID: 11, Quantity: 1, FulfillmentType: "manual"},
	}
	merged, err := mergeCreateOrderItems(items)
	if err != nil {
		t.Fatalf("expected no error for conflicting fulfillment type input, got: %v", err)
	}
	if len(merged) != 2 {
		t.Fatalf("unexpected merged result: %+v", merged)
	}
}

func TestApplyCouponDiscountToItems(t *testing.T) {
	t.Run("CentConservationAcrossSmallCarts", func(t *testing.T) {
		coupon := &coupondomain.Coupon{ScopeType: constants.ScopeTypeProduct, ScopeRefIDs: "[1]"}
		for size := 1; size <= 25; size++ {
			for cents := 1; cents <= 50; cents++ {
				plans := make([]childOrderPlan, size)
				total := decimal.Zero
				for i := range plans {
					value := decimal.New(int64((i%5)+1), -2)
					plans[i] = childOrderPlan{Item: orderdomain.OrderItem{ProductID: 1, SKUID: uint(i + 1), Quantity: 1}, TotalAmount: value}
					total = total.Add(value)
				}
				requested := decimal.New(int64(cents), -2)
				if err := applyCouponDiscountToItems(plans, coupon, requested); err != nil {
					t.Fatal(err)
				}
				allocated := decimal.Zero
				for _, plan := range plans {
					if plan.CouponDiscount.IsNegative() || plan.CouponDiscount.GreaterThan(plan.TotalAmount) || !plan.CouponDiscount.Equal(plan.CouponDiscount.Round(2)) {
						t.Fatalf("invalid cent allocation: %s", plan.CouponDiscount)
					}
					allocated = allocated.Add(plan.CouponDiscount)
				}
				if !allocated.Equal(decimal.Min(requested, total)) {
					t.Fatalf("size=%d request=%s allocation=%s eligible=%s", size, requested, allocated, total)
				}
			}
		}
	})

	t.Run("TestAuditCouponAllocationDropsRemainder", func(t *testing.T) {
		plans := make([]childOrderPlan, 10)
		for i := range plans {
			plans[i] = childOrderPlan{Item: orderdomain.OrderItem{ProductID: 1, SKUID: uint(i + 1), Quantity: 1}, TotalAmount: decimal.RequireFromString("0.01")}
		}
		coupon := &coupondomain.Coupon{ScopeType: constants.ScopeTypeProduct, ScopeRefIDs: "[1]"}
		requested := decimal.RequireFromString("0.04")
		if err := applyCouponDiscountToItems(plans, coupon, requested); err != nil {
			t.Fatal(err)
		}
		actual := decimal.Zero
		for _, plan := range plans {
			actual = actual.Add(plan.CouponDiscount)
		}
		if !actual.Equal(requested) {
			t.Fatalf("unexpected allocation=%s", actual)
		}
		t.Logf("CONFIRMED: eligible=0.10 requested=%s allocated=%s dropped=%s", requested.StringFixed(2), actual.StringFixed(2), requested.Sub(actual).StringFixed(2))

	})

	plans := []childOrderPlan{
		{Item: orderdomain.OrderItem{ProductID: 1}, TotalAmount: decimal.NewFromInt(100)},
		{Item: orderdomain.OrderItem{ProductID: 2}, TotalAmount: decimal.NewFromInt(50)},
		{Item: orderdomain.OrderItem{ProductID: 3}, TotalAmount: decimal.NewFromInt(50)},
	}
	coupon := &coupondomain.Coupon{
		ScopeType:   constants.ScopeTypeProduct,
		ScopeRefIDs: "[1,2]",
	}
	if err := applyCouponDiscountToItems(plans, coupon, decimal.NewFromInt(30)); err != nil {
		t.Fatalf("applyCouponDiscountToItems error: %v", err)
	}
	if !plans[0].CouponDiscount.Equal(decimal.NewFromInt(20)) {
		t.Fatalf("expected 20, got %s", plans[0].CouponDiscount.String())
	}
	if !plans[1].CouponDiscount.Equal(decimal.NewFromInt(10)) {
		t.Fatalf("expected 10, got %s", plans[1].CouponDiscount.String())
	}
	if !plans[2].CouponDiscount.Equal(decimal.Zero) {
		t.Fatalf("expected 0, got %s", plans[2].CouponDiscount.String())
	}
}

func TestResolveManualFormSubmissionPreferOrderItemKey(t *testing.T) {
	data := map[string]jsonmap.JSON{
		"1":    {"legacy": "legacy"},
		"1:10": {"current": "current"},
	}
	got := resolveManualFormSubmission(data, 1, 10)
	if got["current"] != "current" {
		t.Fatalf("expected order item key value, got: %+v", got)
	}
}

func TestResolveManualFormSubmissionFallbackLegacyProductKey(t *testing.T) {
	data := map[string]jsonmap.JSON{
		"1": {"legacy": "legacy"},
	}
	got := resolveManualFormSubmission(data, 1, 99)
	if got["legacy"] != "legacy" {
		t.Fatalf("expected legacy product key value, got: %+v", got)
	}
}
