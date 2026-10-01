package paypal

import (
	"github.com/dujiao-next/internal/constants"
	"testing"
)

func TestPaymentSuccessRequiresPaymentEvent(t *testing.T) {
	for _, tc := range []struct {
		event, state string
		success      bool
	}{
		{"PAYMENT.CAPTURE.COMPLETED", "COMPLETED", true},
		{"CHECKOUT.ORDER.COMPLETED", "COMPLETED", true},
		{"PAYMENT.CAPTURE.COMPLETED", "", true}, // Existing event-only contract.
		{"PAYMENT.CAPTURE.COMPLETED", "PENDING", false},
		{"CHECKOUT.ORDER.COMPLETED", "REFUNDED", false},
		{"PAYMENT.CAPTURE.REFUNDED", "COMPLETED", false},
		{"PAYMENT.CAPTURE.REVERSED", "COMPLETED", false},
		{"PAYMENT.AUTHORIZATION.CREATED", "COMPLETED", false},
		{"CHECKOUT.ORDER.APPROVED", "COMPLETED", false},
		{"UNKNOWN", "COMPLETED", false},
		{"", "COMPLETED", false},
	} {
		t.Run(tc.event+"/"+tc.state, func(t *testing.T) {
			status, ok := ToPaymentStatus(tc.event, tc.state)
			if got := ok && status == constants.PaymentStatusSuccess; got != tc.success {
				t.Fatalf("status=%q ok=%v want success=%v", status, ok, tc.success)
			}
		})
	}
}
