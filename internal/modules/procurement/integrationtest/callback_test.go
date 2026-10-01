package procurement_test

import (
	"encoding/json"
	"errors"
	mappingdomain "github.com/dujiao-next/internal/modules/catalog/mapping/domain"
	procurementgormstore "github.com/dujiao-next/internal/modules/procurement/infrastructure/gormstore"
	siteconnectionapp "github.com/dujiao-next/internal/modules/siteconnection/application"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	fulfillmentdomain "github.com/dujiao-next/internal/modules/fulfillment/domain"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"

	"github.com/dujiao-next/internal/constants"
	procurementcontract "github.com/dujiao-next/internal/modules/procurement/contract"
)

func TestDeliveryFinalizationRollsBackAndRetries(t *testing.T) {
	db := setupProcurementTestDB(t)
	o := createProcTestOrder(t, db, "ATOMIC", constants.OrderStatusFulfilling, constants.FulfillmentTypeUpstream)
	p := createTestProcurementOrder(t, db, 1, o.ID, o.OrderNo, "accepted")
	svc := newTestProcurementService(db, newTestSiteConnectionService(db, "test-key", t.TempDir()))
	db.Callback().Update().Before("gorm:update").Register("fail-order", func(tx *gorm.DB) {
		if tx.Statement.Table == "orders" {
			tx.AddError(errors.New("injected write failure"))
		}
	})
	f := &procurementcontract.Fulfillment{Payload: "synthetic", Status: "delivered"}
	if err := svc.HandleUpstreamCallback(p.ID, "delivered", f); err == nil {
		t.Fatal("order failure acknowledged")
	}
	var count int64
	db.Model(&fulfillmentdomain.Fulfillment{}).Count(&count)
	if count != 0 {
		t.Fatal("partial fulfillment persisted")
	}
	db.First(p, p.ID)
	if p.Status != "accepted" {
		t.Fatal("procurement finalized despite failed order write")
	}
	db.Callback().Update().Remove("fail-order")
	if err := svc.HandleUpstreamCallback(p.ID, "delivered", f); err != nil {
		t.Fatal(err)
	}
	db.First(&o, o.ID)
	if o.Status != constants.OrderStatusDelivered {
		t.Fatal("retry did not deliver")
	}
}

func TestParentFinalizationRollsBackAndRetries(t *testing.T) {
	for _, status := range []string{"delivered", "canceled"} {
		t.Run(status, func(t *testing.T) {
			db := setupProcurementTestDB(t)
			parent := createProcTestOrder(t, db, "PARENT", "fulfilling", "upstream")
			child := createProcTestOrder(t, db, "CHILD", "fulfilling", "upstream")
			if err := db.Model(child).Update("parent_id", parent.ID).Error; err != nil {
				t.Fatal(err)
			}
			p := createTestProcurementOrder(t, db, 1, child.ID, child.OrderNo, "accepted")
			svc := newTestProcurementService(db, newTestSiteConnectionService(db, "test-key", t.TempDir()))
			db.Callback().Update().Before("gorm:update").Register("parent-failure", func(tx *gorm.DB) {
				if tx.Statement.Table == "orders" {
					if _, ok := tx.Statement.Model.(map[string]interface{}); ok {
						tx.AddError(errors.New("injected parent write failure"))
					}
				}
			})
			f := &procurementcontract.Fulfillment{Type: "auto", Payload: "fixture-only"}
			if err := svc.HandleUpstreamCallback(p.ID, status, f); err == nil {
				t.Fatal("parent failure acknowledged")
			}
			db.First(child, child.ID)
			db.First(p, p.ID)
			db.First(parent, parent.ID)
			var count int64
			db.Model(&fulfillmentdomain.Fulfillment{}).Count(&count)
			if child.Status != "fulfilling" || parent.Status != "fulfilling" || p.Status != "accepted" || count != 0 {
				t.Fatalf("partial commit: parent=%s child=%s procurement=%s fulfillments=%d", parent.Status, child.Status, p.Status, count)
			}
			db.Callback().Update().Remove("parent-failure")
			for i := 0; i < 2; i++ {
				if err := svc.HandleUpstreamCallback(p.ID, status, f); err != nil {
					t.Fatal(err)
				}
			}
			db.First(parent, parent.ID)
			db.First(child, child.ID)
			db.First(p, p.ID)
			want, wantProc := "delivered", "fulfilled"
			if status == "canceled" {
				want, wantProc = "paid", "canceled"
			}
			if parent.Status != want || child.Status != want || p.Status != wantProc {
				t.Fatalf("retry inconsistent: parent=%s child=%s procurement=%s", parent.Status, child.Status, p.Status)
			}
			// Repair a historical terminal child whose parent predates atomic finalization.
			if err := db.Model(parent).Update("status", "fulfilling").Error; err != nil {
				t.Fatal(err)
			}
			if err := svc.HandleUpstreamCallback(p.ID, status, f); err != nil {
				t.Fatal(err)
			}
			db.First(parent, parent.ID)
			if parent.Status != want {
				t.Fatal("terminal retry suppressed historical parent repair")
			}
		})
	}
}

func TestRejectedProcurementPropagatesParentFailure(t *testing.T) {
	db := setupProcurementTestDB(t)
	parent := createProcTestOrder(t, db, "REJECT-PARENT", "fulfilling", "upstream")
	child := createProcTestOrder(t, db, "REJECT-CHILD", "fulfilling", "upstream")
	if err := db.Model(child).Update("parent_id", parent.ID).Error; err != nil {
		t.Fatal(err)
	}
	p := createTestProcurementOrder(t, db, 1, child.ID, child.OrderNo, "pending")
	svc := newTestProcurementService(db, newTestSiteConnectionService(db, "test-key", t.TempDir()))
	db.Callback().Update().Before("gorm:update").Register("reject-parent-failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "orders" {
			if _, ok := tx.Statement.Model.(map[string]interface{}); ok {
				tx.AddError(errors.New("injected parent failure"))
			}
		}
	})
	if err := svc.SubmitToUpstream(p.ID); err == nil {
		t.Fatal("rejection swallowed parent transaction failure")
	}
	db.Callback().Update().Remove("reject-parent-failure")
	db.First(p, p.ID)
	db.First(child, child.ID)
	if p.Status != "pending" || child.Status != "fulfilling" {
		t.Fatal("rejection failed to roll back")
	}
	if err := svc.SubmitToUpstream(p.ID); err != nil {
		t.Fatal(err)
	}
	db.First(p, p.ID)
	db.First(parent, parent.ID)
	db.First(child, child.ID)
	if p.Status != "rejected" || parent.Status != "paid" || child.Status != "paid" {
		t.Fatal("rejection retry did not converge")
	}
}

func TestCancelManualPreservesUncertainSupplierOrder(t *testing.T) {
	db := setupProcurementTestDB(t)
	o := createProcTestOrder(t, db, "CANCEL-UNCERTAIN", constants.OrderStatusFulfilling, constants.FulfillmentTypeUpstream)
	p := createTestProcurementOrder(t, db, 1, o.ID, o.OrderNo, "accepted")
	db.Model(p).Update("upstream_order_id", 99)
	svc := newTestProcurementService(db, newTestSiteConnectionService(db, "test-key", t.TempDir()))
	if err := svc.CancelManual(p.ID); err == nil {
		t.Fatal("missing supplier reported cancellation success")
	}
	db.First(p, p.ID)
	if p.Status != "accepted" {
		t.Fatal("uncertain procurement became terminal")
	}
}

func TestConfirmedManualCancellationRollsBackLocalOrder(t *testing.T) {
	db := setupProcurementTestDB(t)
	o := createProcTestOrder(t, db, "CANCEL-CONFIRMED", constants.OrderStatusFulfilling, constants.FulfillmentTypeUpstream)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(map[string]any{"ok": true}) }))
	defer srv.Close()
	cs := newTestSiteConnectionService(db, "test-key", t.TempDir())
	conn, err := cs.Create(siteconnectionapp.CreateInput{Name: "cancel", BaseURL: srv.URL, ApiKey: "test", ApiSecret: "synthetic", Protocol: constants.ConnectionProtocolDujiaoNext})
	if err != nil {
		t.Fatal(err)
	}
	p := createTestProcurementOrder(t, db, conn.ID, o.ID, o.OrderNo, "accepted")
	db.Model(p).Update("upstream_order_id", 99)
	if err := newTestProcurementService(db, cs).CancelManual(p.ID); err != nil {
		t.Fatal(err)
	}
	db.First(o, o.ID)
	db.First(p, p.ID)
	if o.Status != "paid" || p.Status != "canceled" {
		t.Fatalf("inconsistent cancellation order=%s procurement=%s", o.Status, p.Status)
	}
}

func TestLateFailureCannotRegressDeliveredProcurement(t *testing.T) {
	db := setupProcurementTestDB(t)
	o := createProcTestOrder(t, db, "LATE-FAILURE", constants.OrderStatusDelivered, constants.FulfillmentTypeUpstream)
	p := createTestProcurementOrder(t, db, 1, o.ID, o.OrderNo, "fulfilled")
	repo := procurementgormstore.New(db)
	for _, state := range []string{"failed", "pending", "rejected", "accepted", "canceled"} {
		_ = repo.UpdateStatus(p.ID, state, map[string]interface{}{"retry_count": 99})
		db.First(p, p.ID)
		if p.Status != "fulfilled" {
			t.Fatalf("late %s overwrote fulfillment", state)
		}
	}
}

func TestMissingDeliveryRemainsRecoverableAndManualMayBeEmpty(t *testing.T) {
	for _, kind := range []string{"auto", "manual"} {
		t.Run(kind, func(t *testing.T) {
			db := setupProcurementTestDB(t)
			o := createProcTestOrder(t, db, "MISSING-"+kind, constants.OrderStatusFulfilling, constants.FulfillmentTypeUpstream)
			p := createTestProcurementOrder(t, db, 1, o.ID, o.OrderNo, "accepted")
			if err := db.Create(&mappingdomain.Mapping{ConnectionID: 1, LocalProductID: 1, UpstreamProductID: 101, UpstreamFulfillmentType: kind, IsActive: true}).Error; err != nil {
				t.Fatal(err)
			}
			svc := newTestProcurementService(db, newTestSiteConnectionService(db, "test-key", t.TempDir()))
			err := svc.HandleUpstreamCallback(p.ID, "delivered", nil)
			if kind == "manual" {
				if err != nil {
					t.Fatalf("manual status-only delivery rejected: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("missing automatic delivery not retryable")
				}
				if err := svc.HandleUpstreamCallback(p.ID, "delivered", &procurementcontract.Fulfillment{Type: "auto", Status: "delivered"}); err == nil {
					t.Fatal("empty automatic delivery not retryable")
				}
				db.First(p, p.ID)
				if p.Status != "accepted" {
					t.Fatal("missing payload became terminal")
				}
				if err := svc.HandleUpstreamCallback(p.ID, "delivered", &procurementcontract.Fulfillment{Type: "auto", Status: "delivered", Payload: "synthetic"}); err != nil {
					t.Fatal(err)
				}
			}
			db.First(o, o.ID)
			db.First(p, p.ID)
			if o.Status != "delivered" || p.Status != "fulfilled" {
				t.Fatal("delivery never finalized")
			}
		})
	}
}

type procurementCallbackStatusFixture struct {
	orderNo                   string
	initialOrderStatus        string
	initialProcurementStatus  string
	callbackStatus            string
	expectedProcurementStatus string
	expectedOrderStatus       string
}

func assertProcurementCallbackStatus(t *testing.T, fixture procurementCallbackStatusFixture) {
	t.Helper()
	db := setupProcurementTestDB(t)

	order := createProcTestOrder(t, db, fixture.orderNo, fixture.initialOrderStatus, constants.FulfillmentTypeUpstream)
	proc := createTestProcurementOrder(t, db, 1, order.ID, order.OrderNo, fixture.initialProcurementStatus)

	connSvc := newTestSiteConnectionService(db, "test-key", t.TempDir())
	svc := newTestProcurementService(db, connSvc)

	if err := svc.HandleUpstreamCallback(proc.ID, fixture.callbackStatus, nil); err != nil {
		t.Fatalf("HandleUpstreamCallback: %v", err)
	}

	var updatedProc ProcurementOrder
	if err := db.First(&updatedProc, proc.ID).Error; err != nil {
		t.Fatalf("load procurement: %v", err)
	}
	if updatedProc.Status != fixture.expectedProcurementStatus {
		t.Errorf("expected procurement status %q, got %q", fixture.expectedProcurementStatus, updatedProc.Status)
	}

	var updatedOrder orderdomain.Order
	if err := db.First(&updatedOrder, order.ID).Error; err != nil {
		t.Fatalf("load order: %v", err)
	}
	if updatedOrder.Status != fixture.expectedOrderStatus {
		t.Errorf("expected order status %q, got %q", fixture.expectedOrderStatus, updatedOrder.Status)
	}
}

// ── Phase 1 tests: order rollback on procurement failure ──

func TestRejectProcurement_RollsBackOrderStatus(t *testing.T) {
	db := setupProcurementTestDB(t)

	order := createProcTestOrder(t, db, "PROC-REJECT-001", constants.OrderStatusFulfilling, constants.FulfillmentTypeUpstream)
	proc := createTestProcurementOrder(t, db, 1, order.ID, order.OrderNo, "pending")

	connSvc := newTestSiteConnectionService(db, "test-key", t.TempDir())
	svc := newTestProcurementService(db, connSvc)

	if err := svc.SubmitToUpstream(proc.ID); err != nil {
		t.Fatalf("SubmitToUpstream with missing connection: %v", err)
	}

	// 验证采购单状态 = rejected
	var updatedProc ProcurementOrder
	if err := db.First(&updatedProc, proc.ID).Error; err != nil {
		t.Fatalf("load procurement: %v", err)
	}
	if updatedProc.Status != "rejected" {
		t.Errorf("expected procurement status 'rejected', got %q", updatedProc.Status)
	}

	// 验证本地订单状态从 fulfilling 回退到 paid
	var updatedOrder orderdomain.Order
	if err := db.First(&updatedOrder, order.ID).Error; err != nil {
		t.Fatalf("load order: %v", err)
	}
	if updatedOrder.Status != constants.OrderStatusPaid {
		t.Errorf("expected order status %q, got %q", constants.OrderStatusPaid, updatedOrder.Status)
	}
}

func TestHandleUpstreamCallback_Canceled_RollsBackOrder(t *testing.T) {
	db := setupProcurementTestDB(t)

	order := createProcTestOrder(t, db, "PROC-CANCEL-001", constants.OrderStatusFulfilling, constants.FulfillmentTypeUpstream)
	proc := createTestProcurementOrder(t, db, 1, order.ID, order.OrderNo, "accepted")

	connSvc := newTestSiteConnectionService(db, "test-key", t.TempDir())
	svc := newTestProcurementService(db, connSvc)

	if err := svc.HandleUpstreamCallback(proc.ID, "canceled", nil); err != nil {
		t.Fatalf("HandleUpstreamCallback: %v", err)
	}

	// 验证采购单状态 = canceled
	var updatedProc ProcurementOrder
	if err := db.First(&updatedProc, proc.ID).Error; err != nil {
		t.Fatalf("load procurement: %v", err)
	}
	if updatedProc.Status != "canceled" {
		t.Errorf("expected procurement status 'canceled', got %q", updatedProc.Status)
	}

	// 验证本地订单状态从 fulfilling 回退到 paid
	var updatedOrder orderdomain.Order
	if err := db.First(&updatedOrder, order.ID).Error; err != nil {
		t.Fatalf("load order: %v", err)
	}
	if updatedOrder.Status != constants.OrderStatusPaid {
		t.Errorf("expected order status %q, got %q", constants.OrderStatusPaid, updatedOrder.Status)
	}
}

func TestHandleUpstreamCallback_Delivered_CreatesFulfillment(t *testing.T) {
	db := setupProcurementTestDB(t)

	order := createProcTestOrder(t, db, "PROC-DELIVER-001", constants.OrderStatusFulfilling, constants.FulfillmentTypeUpstream)
	proc := createTestProcurementOrder(t, db, 1, order.ID, order.OrderNo, "accepted")

	connSvc := newTestSiteConnectionService(db, "test-key", t.TempDir())
	svc := newTestProcurementService(db, connSvc)

	now := time.Now()
	fulfillment := &procurementcontract.Fulfillment{
		Type:        constants.FulfillmentTypeUpstream,
		Status:      constants.FulfillmentStatusDelivered,
		Payload:     "CDK-001\nCDK-002",
		DeliveredAt: &now,
	}

	if err := svc.HandleUpstreamCallback(proc.ID, "delivered", fulfillment); err != nil {
		t.Fatalf("HandleUpstreamCallback: %v", err)
	}

	// 验证采购单状态 = fulfilled
	var updatedProc ProcurementOrder
	if err := db.First(&updatedProc, proc.ID).Error; err != nil {
		t.Fatalf("load procurement: %v", err)
	}
	if updatedProc.Status != "fulfilled" {
		t.Errorf("expected procurement status 'fulfilled', got %q", updatedProc.Status)
	}

	// 验证本地订单状态 = delivered
	var updatedOrder orderdomain.Order
	if err := db.First(&updatedOrder, order.ID).Error; err != nil {
		t.Fatalf("load order: %v", err)
	}
	if updatedOrder.Status != constants.OrderStatusDelivered {
		t.Errorf("expected order status %q, got %q", constants.OrderStatusDelivered, updatedOrder.Status)
	}

	// 验证 Fulfillment 记录已创建
	var ff fulfillmentdomain.Fulfillment
	if err := db.Where("order_id = ?", order.ID).First(&ff).Error; err != nil {
		t.Fatalf("expected fulfillment record to exist: %v", err)
	}
	if ff.Payload != "CDK-001\nCDK-002" {
		t.Errorf("unexpected fulfillment payload: %q", ff.Payload)
	}
	if ff.Type != constants.FulfillmentTypeUpstream {
		t.Errorf("expected fulfillment type %q, got %q", constants.FulfillmentTypeUpstream, ff.Type)
	}
}

func TestHandleUpstreamCallback_Delivered_SynchronizesParentStatus(t *testing.T) {
	db := setupProcurementTestDB(t)
	parent := createProcTestOrder(t, db, "PROC-PARENT-DELIVERED", constants.OrderStatusFulfilling, constants.FulfillmentTypeUpstream)
	child := createProcTestOrder(t, db, "PROC-CHILD-DELIVERED", constants.OrderStatusFulfilling, constants.FulfillmentTypeUpstream)
	if err := db.Model(&child).Update("parent_id", parent.ID).Error; err != nil {
		t.Fatalf("set child parent: %v", err)
	}
	proc := createTestProcurementOrder(t, db, 1, child.ID, child.OrderNo, constants.ProcurementStatusAccepted)

	svc := newTestProcurementService(db, newTestSiteConnectionService(db, "test-key", t.TempDir()))
	// A real status-only manual delivery has a mapped manual product.
	if err := db.Create(&mappingdomain.Mapping{ConnectionID: 1, LocalProductID: 1, UpstreamProductID: 101, UpstreamFulfillmentType: constants.FulfillmentTypeManual, IsActive: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.HandleUpstreamCallback(proc.ID, "delivered", nil); err != nil {
		t.Fatalf("HandleUpstreamCallback: %v", err)
	}

	var updatedParent orderdomain.Order
	if err := db.First(&updatedParent, parent.ID).Error; err != nil {
		t.Fatalf("load parent order: %v", err)
	}
	if updatedParent.Status != constants.OrderStatusDelivered {
		t.Fatalf("parent status = %q, want %q", updatedParent.Status, constants.OrderStatusDelivered)
	}
}

func TestHandleUpstreamCallback_PartiallyRefunded_AfterFulfilledUpdatesProcurementStatus(t *testing.T) {
	assertProcurementCallbackStatus(t, procurementCallbackStatusFixture{
		orderNo:                   "PROC-REFUND-KEEP-001",
		initialOrderStatus:        constants.OrderStatusDelivered,
		initialProcurementStatus:  constants.ProcurementStatusFulfilled,
		callbackStatus:            "partially_refunded",
		expectedProcurementStatus: constants.ProcurementStatusPartiallyRefunded,
		expectedOrderStatus:       constants.OrderStatusDelivered,
	})
}

func TestHandleUpstreamCallback_PartiallyRefunded_WhileFulfillingKeepsOrderStatus(t *testing.T) {
	assertProcurementCallbackStatus(t, procurementCallbackStatusFixture{
		orderNo:                   "PROC-REFUND-FULFILLING-001",
		initialOrderStatus:        constants.OrderStatusFulfilling,
		initialProcurementStatus:  constants.ProcurementStatusAccepted,
		callbackStatus:            "partially_refunded",
		expectedProcurementStatus: constants.ProcurementStatusPartiallyRefunded,
		expectedOrderStatus:       constants.OrderStatusFulfilling,
	})
}

func TestHandleUpstreamCallback_Refunded_AfterCompletedKeepsOrderStatus(t *testing.T) {
	assertProcurementCallbackStatus(t, procurementCallbackStatusFixture{
		orderNo:                   "PROC-REFUND-COMPLETED-001",
		initialOrderStatus:        constants.OrderStatusCompleted,
		initialProcurementStatus:  constants.ProcurementStatusFulfilled,
		callbackStatus:            "refunded",
		expectedProcurementStatus: constants.ProcurementStatusRefunded,
		expectedOrderStatus:       constants.OrderStatusCompleted,
	})
}
