package presenter

import (
	fulfillmentdomain "github.com/dujiao-next/internal/modules/fulfillment/domain"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	"testing"
	"time"
)

func TestFulfillmentReleaseRequiresPaymentAndDeliveryEvidence(t *testing.T) {
	now := time.Unix(1700000000, 0)
	for _, tc := range []struct {
		name, status, fulfillmentStatus string
		paid, delivered                 bool
		want                            bool
	}{
		{"unpaid pending", "pending_payment", "pending", false, false, false},
		{"unpaid inconsistent delivered", "pending_payment", "delivered", false, true, false},
		{"paid pending", "paid", "pending", true, false, false},
		{"paid delivered", "paid", "delivered", true, true, true},
		{"status alone paid", "paid", "delivered", false, false, false},
		{"legacy delivered", "delivered", "delivered", false, false, true},
		{"legacy completed", "completed", "delivered", false, false, true},
		{"refunded delivered", "refunded", "delivered", true, true, true},
		{"historical refund", "refunded", "delivered", false, true, true},
		{"partial refund delivered", "partially_refunded", "delivered", true, true, true},
		{"refunded without proof", "refunded", "delivered", false, false, false},
		{"refund before delivery", "refunded", "pending", true, false, false},
		{"canceled unpaid", "canceled", "delivered", false, false, false},
		{"canceled paid not delivered", "canceled", "delivered", true, false, false},
		{"canceled historical delivery", "canceled", "delivered", true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			order := orderdomain.Order{Status: tc.status, Fulfillment: &fulfillmentdomain.Fulfillment{Status: tc.fulfillmentStatus, Payload: "FIXTURE-CARD"}}
			if tc.paid {
				order.PaidAt = &now
			}
			if tc.delivered {
				order.Fulfillment.DeliveredAt = &now
			}
			got := NewOrderDetail(&order)
			if (got.Fulfillment != nil) != tc.want {
				t.Errorf("fulfillment released = %v, want %v", got.Fulfillment != nil, tc.want)
			}
			// A parent's payment/delivery must never authorize a different child.
			parent := orderdomain.Order{Status: "delivered", PaidAt: &now, Children: []orderdomain.Order{order}}
			child := NewOrderDetail(&parent).Children[0]
			if (child.Fulfillment != nil) != tc.want {
				t.Errorf("child released = %v, want %v", child.Fulfillment != nil, tc.want)
			}
		})
	}
}
