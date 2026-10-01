package orderreader

import (
	"testing"
	"time"

	fulfillmentdomain "github.com/dujiao-next/internal/modules/fulfillment/domain"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"

	"github.com/dujiao-next/internal/shared/jsonmap"
)

type sourceStub struct {
	order *orderdomain.Order
}

func (s sourceStub) GetByID(uint) (*orderdomain.Order, error) {
	return s.order, nil
}

func TestReaderProjectsParentAndChildFulfillment(t *testing.T) {
	parentID := uint(8)
	deliveredAt := time.Unix(1_700_000_000, 0).UTC()
	source := sourceStub{order: &orderdomain.Order{
		ID:      parentID,
		OrderNo: "DJ-8",
		Status:  "completed",
		Children: []orderdomain.Order{{
			ID:       9,
			ParentID: &parentID,
			Status:   "delivered",
			Fulfillment: &fulfillmentdomain.Fulfillment{
				Type:          "manual",
				Status:        "delivered",
				Payload:       "code-123",
				LogisticsJSON: jsonmap.JSON{"tracking_no": "TRACK-1"},
				DeliveredAt:   &deliveredAt,
			},
		}},
	}}

	projection, err := New(source).GetByID(parentID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if projection.ID != parentID || projection.OrderNo != "DJ-8" || len(projection.Children) != 1 {
		t.Fatalf("order projection mismatch: %#v", projection)
	}
	fulfillment := projection.Children[0].Fulfillment
	if fulfillment == nil || fulfillment.Payload != "code-123" || fulfillment.DeliveryData["tracking_no"] != "TRACK-1" {
		t.Fatalf("fulfillment projection mismatch: %#v", fulfillment)
	}
	if fulfillment.DeliveredAt == nil || !fulfillment.DeliveredAt.Equal(deliveredAt) {
		t.Fatalf("delivered time mismatch: %#v", fulfillment.DeliveredAt)
	}
}

func TestReaderOmitsUnreleasedParentAndChild(t *testing.T) {
	now := time.Unix(1700000000, 0)
	pending := orderdomain.Order{Status: "pending_payment", Fulfillment: &fulfillmentdomain.Fulfillment{Status: "delivered", Payload: "FIXTURE-BLOCKED"}}
	delivered := orderdomain.Order{Status: "refunded", PaidAt: &now, Fulfillment: &fulfillmentdomain.Fulfillment{Status: "delivered", DeliveredAt: &now, Payload: "FIXTURE-RELEASED"}}
	parent := pending
	parent.Children = []orderdomain.Order{pending, delivered}
	got := projectOrder(&parent)
	if got.Fulfillment != nil || got.Children[0].Fulfillment != nil {
		t.Fatal("unpaid delivery leaked")
	}
	if got.Children[1].Fulfillment == nil || got.Children[1].Fulfillment.Payload != "FIXTURE-RELEASED" {
		t.Fatal("historical delivery lost")
	}
}
