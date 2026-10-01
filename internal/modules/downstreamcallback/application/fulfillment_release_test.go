package application

import (
	downstreamcontract "github.com/dujiao-next/internal/modules/downstreamcallback/contract"
	"testing"
	"time"
)

func TestDeliveredFulfillmentUsesPerOrderReleaseEvidence(t *testing.T) {
	now := time.Unix(1700000000, 0)
	blocked := downstreamcontract.OrderSnapshot{Status: "pending_payment", Fulfillment: &downstreamcontract.Fulfillment{Status: "delivered", DeliveredAt: &now, Payload: "FIXTURE-BLOCKED"}}
	released := downstreamcontract.OrderSnapshot{Status: "refunded", Fulfillment: &downstreamcontract.Fulfillment{Status: "delivered", DeliveredAt: &now, Payload: "FIXTURE-RELEASED"}}
	if got := deliveredFulfillment(&blocked); got != nil {
		t.Fatal("unpaid callback leaked")
	}
	parent := blocked
	parent.Children = []downstreamcontract.OrderSnapshot{blocked, released}
	got := deliveredFulfillment(&parent)
	if got == nil || got.Payload != "FIXTURE-RELEASED" {
		t.Fatalf("expected first releasable child, got %#v", got)
	}
	if got == released.Fulfillment {
		t.Fatal("must retain copied payload contract")
	}
}
