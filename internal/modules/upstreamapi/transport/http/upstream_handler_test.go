package upstreamhttp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dujiao-next/internal/logger"
	productdomain "github.com/dujiao-next/internal/modules/catalog/product/domain"
	downstreamdomain "github.com/dujiao-next/internal/modules/downstreamcallback/domain"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	fulfillmentdomain "github.com/dujiao-next/internal/modules/fulfillment/domain"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"time"

	procurementcontract "github.com/dujiao-next/internal/modules/procurement/contract"
	procurementdomain "github.com/dujiao-next/internal/modules/procurement/domain"
	siteconnectiondomain "github.com/dujiao-next/internal/modules/siteconnection/domain"
	upstreamadapter "github.com/dujiao-next/internal/upstream"

	"github.com/gin-gonic/gin"
)

type createCaptureOrders struct {
	Orders
	input CreateOrderInput
}

func (s *createCaptureOrders) CreateOrder(i CreateOrderInput) (*orderdomain.Order, error) {
	s.input = i
	return nil, errors.New("stop after form capture")
}

type formSKU struct{}

func (formSKU) GetByID(uint) (*productdomain.ProductSKU, error) {
	return &productdomain.ProductSKU{ID: 1, ProductID: 2, IsActive: true}, nil
}

type formProduct struct{}

func (formProduct) GetByID(string) (*productdomain.Product, error) {
	return &productdomain.Product{ID: 2, IsActive: true, FulfillmentType: "upstream"}, nil
}
func TestCreateMappedOrderPreservesManualForm(t *testing.T) {
	capture := &createCaptureOrders{}
	h := &Handler{Dependencies: Dependencies{Orders: capture, SKUs: formSKU{}, ProductRepository: formProduct{}}}
	r := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(r)
	c.Set(upstreamUserIDKey, uint(1))
	c.Set(upstreamCredentialIDKey, uint(2))
	c.Request = httptest.NewRequest("POST", "/api/v1/upstream/orders", bytes.NewBufferString(`{"sku_id":1,"quantity":1,"manual_form_data":{"account":"synthetic"}}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.CreateOrder(c)
	if capture.input.ManualFormData["2"]["account"] != "synthetic" {
		t.Fatal("mapped upstream form discarded")
	}
}

type createWorkingOrders struct {
	Orders
	canceled bool
}

func (s *createWorkingOrders) CreateOrder(CreateOrderInput) (*orderdomain.Order, error) {
	return &orderdomain.Order{ID: 1, OrderNo: "SYNTHETIC", Status: "pending_payment"}, nil
}
func (s *createWorkingOrders) CancelOrder(uint, uint) (*orderdomain.Order, error) {
	s.canceled = true
	return &orderdomain.Order{ID: 1, Status: "canceled"}, nil
}

type createRefsFailure struct{ DownstreamOrderReferences }

func (createRefsFailure) Create(*downstreamdomain.OrderRef) error {
	return errors.New("injected ref failure")
}

type paymentCapture struct{ calls int }

func (s *paymentCapture) CreatePayment(CreatePaymentInput) (*CreatePaymentResult, error) {
	s.calls++
	return nil, errors.New("synthetic payment failure")
}
func TestCreateDoesNotPayWithoutReference(t *testing.T) {
	orders := &createWorkingOrders{}
	pay := &paymentCapture{}
	h := &Handler{Dependencies: Dependencies{Orders: orders, Payments: pay, SKUs: formSKU{}, ProductRepository: formProduct{}, DownstreamRefs: createRefsFailure{}}}
	r := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(r)
	c.Set(upstreamUserIDKey, uint(1))
	c.Set(upstreamCredentialIDKey, uint(2))
	c.Request = httptest.NewRequest("POST", "/api/v1/upstream/orders", bytes.NewBufferString(`{"sku_id":1,"quantity":1}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.CreateOrder(c)
	if pay.calls != 0 {
		t.Fatal("charged without durable downstream reference")
	}
	if !orders.canceled {
		t.Fatal("unreferenced order not canceled")
	}
}

type claimedRefs struct{ DownstreamOrderReferences }

func (claimedRefs) GetByCredentialAndDownstreamNo(uint, string) (*downstreamdomain.OrderRef, error) {
	return nil, nil
}
func (claimedRefs) ReserveCreate(uint, string) (bool, error) { return false, nil }
func TestConcurrentCreateLoserHasNoSideEffects(t *testing.T) {
	capture := &createCaptureOrders{}
	h := &Handler{Dependencies: Dependencies{Orders: capture, SKUs: formSKU{}, ProductRepository: formProduct{}, DownstreamRefs: claimedRefs{}}}
	r := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(r)
	c.Set(upstreamUserIDKey, uint(1))
	c.Set(upstreamCredentialIDKey, uint(2))
	c.Request = httptest.NewRequest("POST", "/api/v1/upstream/orders", bytes.NewBufferString(`{"sku_id":1,"quantity":1,"downstream_order_no":"same"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.CreateOrder(c)
	if capture.input.UserID != 0 {
		t.Fatal("lost create claim still created order")
	}
}

func TestCreateLogsHideCallbackURL(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	previous := logger.L
	logger.L = zap.New(core)
	defer func() { logger.L = previous }()
	h := &Handler{Dependencies: Dependencies{Orders: &createWorkingOrders{}, Payments: &paymentCapture{}, SKUs: formSKU{}, ProductRepository: formProduct{}, DownstreamRefs: createRefsFailure{}}}
	r := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(r)
	c.Set(upstreamUserIDKey, uint(1))
	c.Set(upstreamCredentialIDKey, uint(2))
	c.Request = httptest.NewRequest("POST", "/api/v1/upstream/orders", bytes.NewBufferString(`{"sku_id":1,"quantity":1,"callback_url":"https://example.com/callback?token=PRIVATE-MARKER"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.CreateOrder(c)
	if logs.Len() == 0 {
		t.Fatal("no create diagnostic exercised")
	}
	for _, e := range logs.All() {
		if strings.Contains(fmt.Sprint(e.ContextMap()), "PRIVATE-MARKER") {
			t.Fatal("callback query logged")
		}
	}
}

type failedPaymentRefs struct{ DownstreamOrderReferences }

func (failedPaymentRefs) GetByCredentialAndDownstreamNo(uint, string) (*downstreamdomain.OrderRef, error) {
	return &downstreamdomain.OrderRef{OrderID: 7}, nil
}
func TestCanceledRetryDoesNotReportPaidSuccess(t *testing.T) {
	h := &Handler{Dependencies: Dependencies{Orders: releaseOrdersStub{order: &orderdomain.Order{ID: 7, UserID: 1, Status: "canceled"}}, DownstreamRefs: failedPaymentRefs{}}}
	r := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(r)
	c.Set(upstreamUserIDKey, uint(1))
	c.Set(upstreamCredentialIDKey, uint(2))
	c.Request = httptest.NewRequest("POST", "/api/v1/upstream/orders", bytes.NewBufferString(`{"sku_id":1,"quantity":1,"downstream_order_no":"failed"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	// A nil Settings catches the old success path before it can serialize ok:true.
	defer func() {
		if recover() != nil {
			t.Fatal("canceled retry entered success path")
		}
	}()
	h.CreateOrder(c)
	var response map[string]interface{}
	json.Unmarshal(r.Body.Bytes(), &response)
	if response["ok"] != false || response["error_code"] != "payment_failed" {
		t.Fatalf("unexpected retry response %s", r.Body.String())
	}
}

type releaseOrdersStub struct {
	Orders
	order *orderdomain.Order
}

func (s releaseOrdersStub) GetOrderByUser(orderID, userID uint) (*orderdomain.Order, error) {
	if orderID != s.order.ID || userID != s.order.UserID {
		return nil, ErrOrderNotFound
	}
	return s.order, nil
}

func (s releaseOrdersStub) BuildLocalRefundRecordsForOrder(*orderdomain.Order) ([]jsonmap.JSON, error) {
	return []jsonmap.JSON{}, nil
}

func assertGetOrderRelease(t *testing.T, order *orderdomain.Order, wantFulfillment *fulfillmentdomain.Fulfillment) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	order.ID, order.UserID, order.OrderNo, order.Currency = 7, 42, "SYNTHETIC-ORDER", "CNY"
	order.Items = []orderdomain.OrderItem{{ProductID: 11, SKUID: 12, Quantity: 1, TitleJSON: jsonmap.JSON{"en-US": "Synthetic item"}, FulfillmentType: "auto"}}
	before, err := json.Marshal(order)
	if err != nil {
		t.Fatal(err)
	}
	handler := &Handler{Dependencies: Dependencies{Orders: releaseOrdersStub{order: order}}}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/upstream/orders/7", nil)
	c.Params = gin.Params{{Key: "id", Value: "7"}}
	c.Set(upstreamUserIDKey, uint(42))
	handler.GetOrder(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
	}
	want := map[string]interface{}{
		"ok": true, "order_id": float64(7), "order_no": "SYNTHETIC-ORDER", "status": order.Status,
		"amount": "0.00", "refunded_amount": "0.00", "currency": "CNY", "refund_records": []interface{}{},
		"items": []interface{}{map[string]interface{}{"product_id": float64(11), "sku_id": float64(12), "quantity": float64(1),
			"title": map[string]interface{}{"en-US": "Synthetic item"}, "original_unit_price": "0.00", "unit_price": "0.00",
			"original_total_price": "0.00", "total_price": "0.00", "fulfillment_type": "auto"}},
	}
	if wantFulfillment != nil {
		var deliveredAt interface{}
		if wantFulfillment.DeliveredAt != nil {
			deliveredAt = wantFulfillment.DeliveredAt.Format(time.RFC3339Nano)
		}
		want["fulfillment"] = map[string]interface{}{"type": wantFulfillment.Type, "status": wantFulfillment.Status,
			"payload": wantFulfillment.Payload, "delivery_data": map[string]interface{}(wantFulfillment.LogisticsJSON), "delivered_at": deliveredAt}
	}
	var got map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("response differs from unchanged wire schema:\ngot: %#v\nwant: %#v", got, want)
	}
	after, err := json.Marshal(order)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("read mutated order history")
	}
}

func TestGetOrderFulfillmentReleaseEvidence(t *testing.T) {
	stamp := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	cases := []struct {
		name, status, deliveryStatus                         string
		paidAt, deliveredAt, deletedAt, fulfillmentDeletedAt *time.Time
		want                                                 bool
	}{
		{name: "paid delivered", status: "paid", deliveryStatus: "delivered", paidAt: &stamp, want: true},
		{name: "fulfilling", status: "fulfilling", deliveryStatus: "delivered", paidAt: &stamp, want: true},
		{name: "partially delivered", status: "partially_delivered", deliveryStatus: "delivered", paidAt: &stamp, want: true},
		{name: "historical delivered", status: "delivered", deliveryStatus: "delivered", want: true},
		{name: "historical completed", status: "completed", deliveryStatus: "delivered", want: true},
		{name: "refunded paid", status: "refunded", deliveryStatus: "delivered", paidAt: &stamp, want: true},
		{name: "partial refund history", status: "partially_refunded", deliveryStatus: "delivered", deliveredAt: &stamp, want: true},
		{name: "canceled history", status: "canceled", deliveryStatus: "delivered", paidAt: &stamp, deliveredAt: &stamp, want: true},
		{name: "pending inconsistent", status: "pending_payment", deliveryStatus: "delivered", paidAt: &stamp, deliveredAt: &stamp},
		{name: "paid missing proof", status: "paid", deliveryStatus: "delivered"},
		{name: "paid reserved", status: "paid", deliveryStatus: "pending", paidAt: &stamp},
		{name: "refunded unproven", status: "refunded", deliveryStatus: "delivered"},
		{name: "refund before delivery", status: "refunded", deliveryStatus: "pending", paidAt: &stamp},
		{name: "canceled undated", status: "canceled", deliveryStatus: "delivered", paidAt: &stamp},
		{name: "unknown", status: "unknown", deliveryStatus: "delivered", paidAt: &stamp},
		{name: "deleted order", status: "delivered", deliveryStatus: "delivered", deletedAt: &stamp},
		{name: "deleted fulfillment", status: "delivered", deliveryStatus: "delivered", fulfillmentDeletedAt: &stamp},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			order := orderdomain.Order{Status: tc.status, PaidAt: tc.paidAt, DeletedAt: tc.deletedAt,
				Fulfillment: &fulfillmentdomain.Fulfillment{Type: "auto", Status: tc.deliveryStatus, DeliveredAt: tc.deliveredAt,
					DeletedAt: tc.fulfillmentDeletedAt, Payload: " SYNTHETIC-CARD\n", LogisticsJSON: jsonmap.JSON{"code": "SYNTHETIC-STRUCTURED"}}}
			var want *fulfillmentdomain.Fulfillment
			if tc.want {
				want = order.Fulfillment
			}
			assertGetOrderRelease(t, &order, want)
			assertGetOrderRelease(t, &orderdomain.Order{Status: "paid", PaidAt: &stamp, Children: []orderdomain.Order{order}}, want)
		})
	}
}

func TestGetOrderFulfillmentFallbackUsesFirstReleasableOrder(t *testing.T) {
	stamp := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	snapshot := func(code string) *fulfillmentdomain.Fulfillment {
		return &fulfillmentdomain.Fulfillment{Type: "auto", Status: "delivered", Payload: code, LogisticsJSON: jsonmap.JSON{"code": code}}
	}
	valid := orderdomain.Order{Status: "paid", PaidAt: &stamp, Fulfillment: snapshot("SYNTHETIC-VALID")}
	for _, parentStatus := range []string{"pending_payment", "delivered"} {
		t.Run(parentStatus, func(t *testing.T) {
			parent := &orderdomain.Order{Status: parentStatus, Fulfillment: snapshot("SYNTHETIC-PARENT"), Children: []orderdomain.Order{
				{Status: "pending_payment", PaidAt: &stamp, Fulfillment: snapshot("SYNTHETIC-HIDDEN")}, valid,
				{Status: "completed", Fulfillment: snapshot("SYNTHETIC-LATER")},
			}}
			want := valid.Fulfillment
			if parentStatus == "delivered" {
				want = parent.Fulfillment
			}
			assertGetOrderRelease(t, parent, want)
			parent.Fulfillment = nil
			assertGetOrderRelease(t, parent, valid.Fulfillment)
			parent.Fulfillment = snapshot("SYNTHETIC-RESERVED")
			parent.Fulfillment.Status = "pending"
			assertGetOrderRelease(t, parent, valid.Fulfillment)
		})
	}
}

func TestMapCallbackStatus(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{name: "delivered keep delivered", input: "delivered", expect: "delivered"},
		{name: "completed map delivered", input: "completed", expect: "delivered"},
		{name: "fulfilled map delivered", input: "fulfilled", expect: "delivered"},
		{name: "canceled keep canceled", input: "canceled", expect: "canceled"},
		{name: "cancelled map canceled", input: "cancelled", expect: "canceled"},
		{name: "refunded keep refunded", input: "refunded", expect: "refunded"},
		{name: "partially refunded keep value", input: "partially_refunded", expect: "partially_refunded"},
		{name: "trim and lower", input: "  ReFuNdEd  ", expect: "refunded"},
		{name: "fallback normalized raw", input: "PROCESSING", expect: "processing"},
	}

	for _, tc := range tests {
		got := mapCallbackStatus(tc.input)
		if got != tc.expect {
			t.Fatalf("%s: want %q got %q", tc.name, tc.expect, got)
		}
	}
}

type stubConnections struct {
	conn *siteconnectiondomain.Connection
}

func (s stubConnections) GetByApiKey(string) (*siteconnectiondomain.Connection, error) {
	return s.conn, nil
}

type stubSecrets struct{}

func (stubSecrets) DecryptSecret(encrypted string) (string, error) { return encrypted, nil }

type stubProcurements struct {
	order   *procurementdomain.Order
	handled bool
}

func (s *stubProcurements) GetByLocalOrderNo(string) (*procurementdomain.Order, error) {
	return s.order, nil
}

func (s *stubProcurements) HandleUpstreamCallback(uint, string, *procurementcontract.Fulfillment) error {
	s.handled = true
	return nil
}

// TestHandleCallbackOwnership 保证回调只能由采购单所属连接、且上游订单号一致时才被受理。
func TestHandleCallbackOwnership(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name         string
		connectionID uint
		upstreamID   uint
		payloadID    uint
		wantHandled  bool
	}{
		{name: "同连接同上游单号放行", connectionID: 1, upstreamID: 88, payloadID: 88, wantHandled: true},
		{name: "上游单号未落库时只校验连接", connectionID: 1, upstreamID: 0, payloadID: 88, wantHandled: true},
		{name: "其它连接冒用本地订单号", connectionID: 2, upstreamID: 88, payloadID: 88, wantHandled: false},
		{name: "上游订单号不匹配", connectionID: 1, upstreamID: 88, payloadID: 99, wantHandled: false},
	}

	for _, tc := range cases {
		procurements := &stubProcurements{order: &procurementdomain.Order{
			ID:              7,
			ConnectionID:    tc.connectionID,
			LocalOrderNo:    "LOCAL-1",
			UpstreamOrderID: tc.upstreamID,
		}}
		handler := &Handler{Dependencies: Dependencies{
			Connections: stubConnections{conn: &siteconnectiondomain.Connection{
				ID: 1, ApiKey: "key-1", ApiSecret: "secret-1", Status: "active",
			}},
			ConnectionSecrets: stubSecrets{},
			Procurements:      procurements,
		}}

		timestamp := time.Now().Unix()
		body, _ := json.Marshal(callbackPayload{
			Event:             "order.fulfilled",
			OrderID:           tc.payloadID,
			DownstreamOrderNo: "LOCAL-1",
			Status:            "delivered",
			Timestamp:         timestamp,
		})

		request := httptest.NewRequest(http.MethodPost, "/api/v1/upstream/callback", bytes.NewReader(body))
		request.Header.Set(upstreamadapter.HeaderApiKey, "key-1")
		request.Header.Set(upstreamadapter.HeaderTimestamp, fmt.Sprintf("%d", timestamp))
		request.Header.Set(upstreamadapter.HeaderSignature,
			upstreamadapter.Sign("secret-1", http.MethodPost, "/api/v1/upstream/callback", timestamp, body))

		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = request
		handler.HandleCallback(c)

		if procurements.handled != tc.wantHandled {
			t.Fatalf("%s: handled=%v want %v (body %s)", tc.name, procurements.handled, tc.wantHandled, recorder.Body.String())
		}
	}
}
