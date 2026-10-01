package procurement_test

import (
	"encoding/json"
	"errors"
	procurementapp "github.com/dujiao-next/internal/modules/procurement/application"
	procurementcontract "github.com/dujiao-next/internal/modules/procurement/contract"
	procurementgormstore "github.com/dujiao-next/internal/modules/procurement/infrastructure/gormstore"
	queueadapter "github.com/dujiao-next/internal/modules/procurement/infrastructure/queueadapter"
	siteconnectiondomain "github.com/dujiao-next/internal/modules/siteconnection/domain"
	"github.com/dujiao-next/internal/queue"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	orderdomain "github.com/dujiao-next/internal/modules/order/domain"

	mappingdomain "github.com/dujiao-next/internal/modules/catalog/mapping/domain"

	"github.com/dujiao-next/internal/constants"
	siteconnectionapp "github.com/dujiao-next/internal/modules/siteconnection/application"
)

func TestSubmitRevalidatesEligibility(t *testing.T) {
	for _, scenario := range []string{"canceled", "refunded", "pending_payment", "connection-disabled", "mapping-disabled", "sku-disabled", "order-changed", "mapping-changed", "sku-changed", "sku-identity-changed"} {
		t.Run(scenario, func(t *testing.T) {
			db := setupProcurementTestDB(t)
			state := constants.OrderStatusPaid
			if scenario == "canceled" || scenario == "refunded" || scenario == "pending_payment" {
				state = scenario
			}
			o := createProcTestOrder(t, db, "GUARD-"+scenario, state, constants.FulfillmentTypeUpstream)
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				json.NewEncoder(w).Encode(map[string]any{"ok": true, "order_id": 99})
			}))
			defer srv.Close()
			cs := newTestSiteConnectionService(db, "test-key", t.TempDir())
			conn, err := cs.Create(siteconnectionapp.CreateInput{Name: "guard", BaseURL: srv.URL, ApiKey: "test", ApiSecret: "secret", Protocol: constants.ConnectionProtocolDujiaoNext})
			if err != nil {
				t.Fatal(err)
			}
			status := "active"
			if scenario == "connection-disabled" {
				status = "disabled"
			}
			db.Model(&siteconnectiondomain.Connection{}).Where("id = ?", conn.ID).Update("status", status)
			pm := mappingdomain.Mapping{ConnectionID: conn.ID, LocalProductID: 1, UpstreamProductID: 1, IsActive: true}
			db.Create(&pm)
			sm := mappingdomain.SKUMapping{ProductMappingID: pm.ID, LocalSKUID: 1, UpstreamSKUID: 1, UpstreamIsActive: true}
			db.Create(&sm)
			if scenario == "mapping-disabled" {
				db.Model(&pm).Update("is_active", false)
			}
			if scenario == "sku-disabled" {
				db.Model(&sm).Update("upstream_is_active", false)
			}
			po := createTestProcurementOrder(t, db, conn.ID, o.ID, o.OrderNo, "pending")
			if scenario == "order-changed" || scenario == "mapping-changed" || scenario == "sku-changed" || scenario == "sku-identity-changed" {
				changed := false
				db.Callback().Query().After("gorm:query").Register("change-after-sku-read", func(tx *gorm.DB) {
					if !changed && tx.Statement.Table == "sku_mappings" {
						changed = true
						if scenario == "order-changed" {
							state = "refunded"
							db.Model(o).Update("status", state)
						} else if scenario == "sku-changed" {
							db.Model(&sm).Update("upstream_is_active", false)
						} else if scenario == "sku-identity-changed" {
							db.Model(&sm).Update("upstream_sku_id", 999)
						} else {
							db.Model(&pm).Update("is_active", false)
						}
					}
				})
			}
			_ = newTestProcurementService(db, cs).SubmitToUpstream(po.ID)
			if calls.Load() != 0 {
				t.Fatal("ineligible work purchased from supplier")
			}
			var after orderdomain.Order
			db.First(&after, o.ID)
			if after.Status != state {
				t.Fatal("order state overwritten")
			}
		})
	}
}

type submitOptionCapture struct{ options []asynq.Option }

func (c *submitOptionCapture) EnqueueProcurementSubmit(_ queue.ProcurementSubmitPayload, opts ...asynq.Option) error {
	c.options = opts
	return nil
}
func (c *submitOptionCapture) EnqueueProcurementPollStatus(queue.ProcurementPollStatusPayload, time.Duration) error {
	return nil
}
func TestSubmitQueueCarriesConfiguredDelay(t *testing.T) {
	c := &submitOptionCapture{}
	delay := 37 * time.Second
	if err := queueadapter.New(c).EnqueueSubmit(1, delay); err != nil {
		t.Fatal(err)
	}
	if len(c.options) != 1 || c.options[0].Type() != asynq.ProcessIn(delay).Type() || c.options[0].Value() != delay {
		t.Fatalf("missing ProcessIn delay: %v", c.options)
	}
}

func TestSubmitHonorsPersistedRetryDeadline(t *testing.T) {
	db := setupProcurementTestDB(t)
	o := createProcTestOrder(t, db, "RETRY-DEADLINE", constants.OrderStatusPaid, constants.FulfillmentTypeUpstream)
	p := createTestProcurementOrder(t, db, 1, o.ID, o.OrderNo, "failed")
	due := time.Now().Add(time.Hour)
	db.Model(p).Update("next_retry_at", due)
	svc := newTestProcurementService(db, newTestSiteConnectionService(db, "test-key", t.TempDir()))
	_ = svc.SubmitToUpstream(p.ID)
	db.First(p, p.ID)
	if p.Status != "failed" || p.RetryCount != 0 {
		t.Fatal("early retry executed instead of honoring deadline")
	}
}

func TestEarlyDeliveryCannotBeRegressedBySubmitResponse(t *testing.T) {
	db := setupProcurementTestDB(t)
	o := createProcTestOrder(t, db, "EARLY-DELIVERY", constants.OrderStatusPaid, constants.FulfillmentTypeUpstream)
	var svc *procurementapp.Service
	var procID uint
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := svc.HandleUpstreamCallback(procID, "delivered", &procurementcontract.Fulfillment{Payload: "synthetic", Status: "delivered"}); err != nil {
			t.Error(err)
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "order_id": 99, "amount": "10.00"})
	}))
	defer srv.Close()
	cs := newTestSiteConnectionService(db, "test-key", t.TempDir())
	conn, err := cs.Create(siteconnectionapp.CreateInput{Name: "early", BaseURL: srv.URL, ApiKey: "test", ApiSecret: "secret", Protocol: constants.ConnectionProtocolDujiaoNext})
	if err != nil {
		t.Fatal(err)
	}
	db.Model(&siteconnectiondomain.Connection{}).Where("id = ?", conn.ID).Update("status", "active")
	pm := mappingdomain.Mapping{ConnectionID: conn.ID, LocalProductID: 1, UpstreamProductID: 1, IsActive: true}
	db.Create(&pm)
	db.Create(&mappingdomain.SKUMapping{ProductMappingID: pm.ID, LocalSKUID: 1, UpstreamSKUID: 1, UpstreamIsActive: true})
	p := createTestProcurementOrder(t, db, conn.ID, o.ID, o.OrderNo, "pending")
	procID = p.ID
	svc = newTestProcurementService(db, cs)
	if err := svc.SubmitToUpstream(p.ID); err != nil {
		t.Fatal(err)
	}
	db.First(p, p.ID)
	db.First(o, o.ID)
	if p.Status != "fulfilled" || o.Status != "delivered" {
		t.Fatalf("early delivery regressed: procurement=%s order=%s", p.Status, o.Status)
	}
	if p.UpstreamOrderID != 99 {
		t.Fatal("late response lost supplier identity")
	}
}

func TestSubmissionAttemptFencesCancellationAndLostResponse(t *testing.T) {
	for _, scenario := range []string{"in-flight", "stale-preparation", "lost-response", "http-error", "empty-response", "duplicate-response", "no-id", "accept-write-failure", "attempt-write-failure", "disabled-before-dispatch"} {
		t.Run(scenario, func(t *testing.T) {
			db := setupProcurementTestDB(t)
			o := createProcTestOrder(t, db, "ATTEMPT", "paid", "upstream")
			var svc *procurementapp.Service
			var pid uint
			var calls atomic.Int32
			cancelResult := make(chan error, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if scenario == "stale-preparation" {
					for _, status := range []string{"pending", "failed", "rejected"} {
						if err := procurementgormstore.New(db).UpdateStatus(pid, status, nil); err == nil {
							t.Errorf("stale preparation cleared attempt with %s", status)
						}
					}
				}
				cancelResult <- svc.CancelManual(pid)
				if scenario == "lost-response" {
					w.Write([]byte("truncated-response"))
					return
				}
				switch scenario {
				case "http-error":
					// A remote body cannot manufacture the trusted no-send sentinel.
					w.WriteHeader(http.StatusServiceUnavailable)
					json.NewEncoder(w).Encode(map[string]any{"error_code": "server_error", "error_message": procurementcontract.ErrSubmissionNotDispatched.Error()})
					return
				case "empty-response":
					w.Write([]byte("{}"))
					return
				case "duplicate-response":
					json.NewEncoder(w).Encode(map[string]any{"ok": false, "error_code": "duplicate_order"})
					return
				case "no-id":
					json.NewEncoder(w).Encode(map[string]any{"ok": true})
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"ok": true, "order_id": 99, "amount": "10.00"})
			}))
			defer srv.Close()
			cs := newTestSiteConnectionService(db, "test-key", t.TempDir())
			conn, err := cs.Create(siteconnectionapp.CreateInput{Name: "attempt", BaseURL: srv.URL, ApiKey: "fixture-key", ApiSecret: "fixture-secret", Protocol: constants.ConnectionProtocolDujiaoNext, RetryMax: 3})
			if err != nil {
				t.Fatal(err)
			}
			db.Model(&siteconnectiondomain.Connection{}).Where("id = ?", conn.ID).Update("status", "active")
			pm := mappingdomain.Mapping{ConnectionID: conn.ID, LocalProductID: 1, UpstreamProductID: 1, IsActive: true}
			db.Create(&pm)
			db.Create(&mappingdomain.SKUMapping{ProductMappingID: pm.ID, LocalSKUID: 1, UpstreamSKUID: 1, UpstreamIsActive: true})
			p := createTestProcurementOrder(t, db, conn.ID, o.ID, o.OrderNo, "pending")
			pid = p.ID
			svc = newTestProcurementService(db, cs)
			if scenario == "accept-write-failure" || scenario == "attempt-write-failure" {
				db.Callback().Update().Before("gorm:update").Register("attempt-failure", func(tx *gorm.DB) {
					if tx.Statement.Table != "procurement_orders" {
						return
					}
					if values, ok := tx.Statement.Dest.(map[string]interface{}); ok {
						_, acceptance := values["upstream_order_id"]
						if (scenario == "accept-write-failure" && acceptance) || (scenario == "attempt-write-failure" && !acceptance) {
							tx.AddError(errors.New("injected attempt persistence failure"))
						}
					}
				})
			}
			if scenario == "disabled-before-dispatch" {
				db.Model(conn).Update("status", "disabled")
			}
			err = svc.SubmitToUpstream(pid)
			if scenario == "disabled-before-dispatch" {
				if err != nil {
					t.Fatalf("known unsent work became uncertain: %v", err)
				}
				db.First(p, pid)
				if p.Status != "failed" || p.NextRetryAt == nil || calls.Load() != 0 {
					t.Fatal("safe failure did not schedule retry")
				}
				db.Model(conn).Update("status", "active")
				if err := svc.SubmitToUpstream(pid); err != nil {
					t.Fatal(err)
				}
				if calls.Load() != 0 {
					t.Fatal("persisted delay ignored")
				}
				db.Model(p).Update("next_retry_at", time.Now().Add(-time.Second))
				if err := svc.SubmitToUpstream(pid); err != nil {
					t.Fatal(err)
				}
				db.First(p, pid)
				if p.Status != "accepted" || p.UpstreamOrderID != 99 || calls.Load() != 1 {
					t.Fatal("known unsent retry did not converge")
				}
				return
			}
			if scenario == "in-flight" && err != nil {
				t.Fatal(err)
			}
			if scenario != "in-flight" && scenario != "stale-preparation" && err == nil {
				t.Fatal("uncertainty/persistence failure acknowledged")
			}
			if scenario == "attempt-write-failure" {
				if calls.Load() != 0 {
					t.Fatal("purchase dispatched without durable fence")
				}
				return
			}
			if cancelErr := <-cancelResult; cancelErr == nil {
				t.Fatal("in-flight purchase canceled as unsent")
			}
			db.Callback().Update().Remove("attempt-failure")
			if scenario != "in-flight" && scenario != "stale-preparation" {
				if err := svc.CancelManual(pid); err == nil {
					t.Fatal("uncertain lost response canceled")
				}
				if err := svc.RetryManual(pid); err == nil {
					t.Fatal("uncertain lost response reset for purchase")
				}
				_ = svc.SubmitToUpstream(pid)
				if calls.Load() != 1 {
					t.Fatal("uncertain purchase spent again")
				}
			}
			if err := svc.HandleUpstreamCallback(pid, "delivered", &procurementcontract.Fulfillment{Type: "auto", Payload: "fixture-only"}); err != nil {
				t.Fatal(err)
			}
			db.First(p, pid)
			db.First(o, o.ID)
			if p.Status != "fulfilled" || o.Status != "delivered" {
				t.Fatalf("callback recovery failed: procurement=%s local=%s", p.Status, o.Status)
			}
		})
	}
}

// ── SubmitToUpstream tests ──

func TestSubmitToUpstream_Success(t *testing.T) {
	db := setupProcurementTestDB(t)

	order := createProcTestOrder(t, db, "PROC-SUBMIT-001", constants.OrderStatusPaid, constants.FulfillmentTypeUpstream)
	// 创建 product mapping 和 sku mapping
	pm := &mappingdomain.Mapping{
		ConnectionID:      1,
		LocalProductID:    1,
		UpstreamProductID: 101,
		IsActive:          true,
	}
	db.Create(pm)
	sm := &mappingdomain.SKUMapping{
		ProductMappingID: pm.ID,
		LocalSKUID:       1,
		UpstreamSKUID:    201,
		UpstreamIsActive: true,
	}
	db.Create(sm)

	// mock upstream server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"ok":       true,
			"order_id": 999,
			"order_no": "UP-999",
			"status":   "accepted",
			"amount":   "50.00",
			"currency": "CNY",
		})
	}))
	defer server.Close()

	connSvc := newTestSiteConnectionService(db, "test-key", t.TempDir())
	conn, err := connSvc.Create(siteconnectionapp.CreateInput{
		Name:      "test-upstream",
		BaseURL:   server.URL,
		ApiKey:    "key",
		ApiSecret: "secret",
		Protocol:  constants.ConnectionProtocolDujiaoNext,
	})
	if err != nil {
		t.Fatalf("create connection: %v", err)
	}

	db.Model(&siteconnectiondomain.Connection{}).Where("id = ?", conn.ID).Update("status", "active")
	proc := createTestProcurementOrder(t, db, conn.ID, order.ID, order.OrderNo, "pending")

	svc := newTestProcurementService(db, connSvc)

	if err := svc.SubmitToUpstream(proc.ID); err != nil {
		t.Fatalf("SubmitToUpstream: %v", err)
	}

	// 验证采购单状态 = accepted
	var updatedProc ProcurementOrder
	db.First(&updatedProc, proc.ID)
	if updatedProc.Status != "accepted" {
		t.Errorf("expected procurement status 'accepted', got %q", updatedProc.Status)
	}
	if updatedProc.UpstreamOrderID != 999 {
		t.Errorf("expected upstream_order_id=999, got %d", updatedProc.UpstreamOrderID)
	}

	// 验证本地订单状态 = fulfilling
	var updatedOrder orderdomain.Order
	db.First(&updatedOrder, order.ID)
	if updatedOrder.Status != constants.OrderStatusFulfilling {
		t.Errorf("expected order status %q, got %q", constants.OrderStatusFulfilling, updatedOrder.Status)
	}
}

func TestSubmitToUpstream_NonRetryableError_Rejects(t *testing.T) {
	db := setupProcurementTestDB(t)

	order := createProcTestOrder(t, db, "PROC-NONRETRY-001", constants.OrderStatusFulfilling, constants.FulfillmentTypeUpstream)
	pm := &mappingdomain.Mapping{ConnectionID: 1, LocalProductID: 1, UpstreamProductID: 101, IsActive: true}
	db.Create(pm)
	sm := &mappingdomain.SKUMapping{ProductMappingID: pm.ID, LocalSKUID: 1, UpstreamSKUID: 201, UpstreamIsActive: true}
	db.Create(sm)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"ok":            false,
			"error_code":    "product_out_of_stock",
			"error_message": "product out of stock",
		})
	}))
	defer server.Close()

	connSvc := newTestSiteConnectionService(db, "test-key", t.TempDir())
	conn, _ := connSvc.Create(siteconnectionapp.CreateInput{
		Name: "test-upstream", BaseURL: server.URL,
		ApiKey: "key", ApiSecret: "secret", Protocol: constants.ConnectionProtocolDujiaoNext,
	})

	db.Model(&siteconnectiondomain.Connection{}).Where("id = ?", conn.ID).Update("status", "active")
	proc := createTestProcurementOrder(t, db, conn.ID, order.ID, order.OrderNo, "pending")
	svc := newTestProcurementService(db, connSvc)

	// 不可重试错误应返回 error
	_ = svc.SubmitToUpstream(proc.ID)

	// 验证采购单状态 = rejected
	var updatedProc ProcurementOrder
	db.First(&updatedProc, proc.ID)
	if updatedProc.Status != "rejected" {
		t.Errorf("expected procurement status 'rejected', got %q", updatedProc.Status)
	}

	// 验证本地订单状态回退到 paid
	var updatedOrder orderdomain.Order
	db.First(&updatedOrder, order.ID)
	if updatedOrder.Status != constants.OrderStatusPaid {
		t.Errorf("expected order status %q after rejection, got %q", constants.OrderStatusPaid, updatedOrder.Status)
	}
}

func TestSubmitToUpstream_RetryableError_Retries(t *testing.T) {
	db := setupProcurementTestDB(t)

	order := createProcTestOrder(t, db, "PROC-RETRY-001", constants.OrderStatusFulfilling, constants.FulfillmentTypeUpstream)
	pm := &mappingdomain.Mapping{ConnectionID: 1, LocalProductID: 1, UpstreamProductID: 101, IsActive: true}
	db.Create(pm)
	sm := &mappingdomain.SKUMapping{ProductMappingID: pm.ID, LocalSKUID: 1, UpstreamSKUID: 201, UpstreamIsActive: true}
	db.Create(sm)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"ok":            false,
			"error_code":    "server_error",
			"error_message": "temporary failure",
		})
	}))
	defer server.Close()

	connSvc := newTestSiteConnectionService(db, "test-key", t.TempDir())
	conn, _ := connSvc.Create(siteconnectionapp.CreateInput{
		Name: "test-upstream", BaseURL: server.URL,
		ApiKey: "key", ApiSecret: "secret", Protocol: constants.ConnectionProtocolDujiaoNext,
		RetryMax: 3,
	})

	db.Model(&siteconnectiondomain.Connection{}).Where("id = ?", conn.ID).Update("status", "active")
	proc := createTestProcurementOrder(t, db, conn.ID, order.ID, order.OrderNo, "pending")
	svc := newTestProcurementService(db, connSvc)

	// 可重试错误不应返回 error（已入队重试）
	if err := svc.SubmitToUpstream(proc.ID); err != nil {
		t.Fatalf("expected no error for retryable failure, got: %v", err)
	}

	// 验证采购单状态 = failed（而非 rejected）
	var updatedProc ProcurementOrder
	db.First(&updatedProc, proc.ID)
	if updatedProc.Status != "failed" {
		t.Errorf("expected procurement status 'failed', got %q", updatedProc.Status)
	}
	if updatedProc.RetryCount != 1 {
		t.Errorf("expected retry_count=1, got %d", updatedProc.RetryCount)
	}
}

func TestHandleSubmitFailure_MaxRetriesExhausted(t *testing.T) {
	db := setupProcurementTestDB(t)

	order := createProcTestOrder(t, db, "PROC-MAXRETRY-001", constants.OrderStatusFulfilling, constants.FulfillmentTypeUpstream)
	productMapping := &mappingdomain.Mapping{ConnectionID: 1, LocalProductID: 1, UpstreamProductID: 101, IsActive: true}
	db.Create(productMapping)
	db.Create(&mappingdomain.SKUMapping{
		ProductMappingID: productMapping.ID,
		LocalSKUID:       1,
		UpstreamSKUID:    201,
		UpstreamIsActive: true,
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":            false,
			"error_code":    "server_error",
			"error_message": "timeout after retries",
		})
	}))
	defer server.Close()

	connSvc := newTestSiteConnectionService(db, "test-key", t.TempDir())
	conn, err := connSvc.Create(siteconnectionapp.CreateInput{
		Name: "test-upstream", BaseURL: server.URL,
		ApiKey: "key", ApiSecret: "secret", Protocol: constants.ConnectionProtocolDujiaoNext,
		RetryMax: 2, RetryIntervals: "[30,60]",
	})
	if err != nil {
		t.Fatalf("create connection: %v", err)
	}

	db.Model(&siteconnectiondomain.Connection{}).Where("id = ?", conn.ID).Update("status", "active")
	proc := createTestProcurementOrder(t, db, conn.ID, order.ID, order.OrderNo, "failed")
	// 设置 retry_count 已达上限
	db.Model(proc).Update("retry_count", 2)

	svc := newTestProcurementService(db, connSvc)

	// 通过公开提交入口验证：可重试错误在次数耗尽后仍必须转为 rejected。
	_ = svc.SubmitToUpstream(proc.ID)

	// 验证采购单状态 = rejected
	var updatedProc ProcurementOrder
	db.First(&updatedProc, proc.ID)
	if updatedProc.Status != "rejected" {
		t.Errorf("expected procurement status 'rejected', got %q", updatedProc.Status)
	}

	// 验证本地订单回退到 paid
	var updatedOrder orderdomain.Order
	db.First(&updatedOrder, order.ID)
	if updatedOrder.Status != constants.OrderStatusPaid {
		t.Errorf("expected order status %q, got %q", constants.OrderStatusPaid, updatedOrder.Status)
	}
}
