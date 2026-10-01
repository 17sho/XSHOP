package domain

import (
	"testing"
	"time"

	fulfillmentdomain "github.com/dujiao-next/internal/modules/fulfillment/domain"
	"github.com/shopspring/decimal"
)

func TestCalculatePaymentFeeRefundAmountAllocatesAndAbsorbsRounding(t *testing.T) {
	paymentAmount := decimal.RequireFromString("100.00")
	paymentFee := decimal.RequireFromString("3.01")

	first := CalculatePaymentFeeRefundAmount(
		paymentAmount,
		paymentFee,
		decimal.Zero,
		decimal.Zero,
		decimal.RequireFromString("33.33"),
	)
	if !first.Equal(decimal.RequireFromString("1.00")) {
		t.Fatalf("first fee refund = %s, want 1.00", first)
	}

	second := CalculatePaymentFeeRefundAmount(
		paymentAmount,
		paymentFee,
		decimal.RequireFromString("33.33"),
		first,
		decimal.RequireFromString("66.67"),
	)
	if !second.Equal(decimal.RequireFromString("2.01")) {
		t.Fatalf("final fee refund = %s, want 2.01", second)
	}
}

func TestCalculatePaymentFeeRefundAmountCapsAtOriginalFee(t *testing.T) {
	got := CalculatePaymentFeeRefundAmount(
		decimal.RequireFromString("80.00"),
		decimal.RequireFromString("2.40"),
		decimal.RequireFromString("60.00"),
		decimal.RequireFromString("1.80"),
		decimal.RequireFromString("50.00"),
	)
	if !got.Equal(decimal.RequireFromString("0.60")) {
		t.Fatalf("capped fee refund = %s, want 0.60", got)
	}
}

func TestFulfillmentReleaseBoundaryEvidence(t *testing.T) {
	now := time.Unix(1700000000, 0)
	zero := time.Time{}
	delivered := func() *fulfillmentdomain.Fulfillment {
		return &fulfillmentdomain.Fulfillment{Status: "delivered", DeliveredAt: &now}
	}
	for _, tc := range []struct {
		name  string
		order *Order
		want  bool
	}{
		{"nil", nil, false},
		{"missing fulfillment", &Order{Status: "delivered"}, false},
		{"deleted order", &Order{Status: "delivered", DeletedAt: &now, Fulfillment: delivered()}, false},
		{"deleted fulfillment", &Order{Status: "delivered", Fulfillment: &fulfillmentdomain.Fulfillment{Status: "delivered", DeletedAt: &now}}, false},
		{"zero paid", &Order{Status: "paid", PaidAt: &zero, Fulfillment: delivered()}, false},
		{"zero historic date", &Order{Status: "refunded", Fulfillment: &fulfillmentdomain.Fulfillment{Status: "delivered", DeliveredAt: &zero}}, false},
		{"unknown", &Order{Status: "future-state", PaidAt: &now, Fulfillment: delivered()}, false},
		{"inconsistent pending paid", &Order{Status: "pending_payment", PaidAt: &now, Fulfillment: delivered()}, false},
		{"partial delivered paid", &Order{Status: "partially_delivered", PaidAt: &now, Fulfillment: delivered()}, true},
		{"partial delivered unproven", &Order{Status: "partially_delivered", Fulfillment: delivered()}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.order.CanReleaseFulfillment(); got != tc.want {
				t.Errorf("release = %v, want %v", got, tc.want)
			}
		})
	}
}
