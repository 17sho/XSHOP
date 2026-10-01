package procurement_test

import (
	"errors"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	"testing"
	"time"

	mappingdomain "github.com/dujiao-next/internal/modules/catalog/mapping/domain"

	"github.com/dujiao-next/internal/constants"
)

type recoveringQueue struct {
	fail  bool
	calls int
}

func (q *recoveringQueue) EnqueueSubmit(uint, ...time.Duration) error {
	q.calls++
	if q.fail {
		return errors.New("queue offline")
	}
	return nil
}
func (q *recoveringQueue) EnqueuePoll(uint, time.Duration) error { return nil }
func TestCreateRecoversAfterLostEnqueue(t *testing.T) {
	db := setupProcurementTestDB(t)
	o := createProcTestOrder(t, db, "LOST-QUEUE", constants.OrderStatusPaid, constants.FulfillmentTypeUpstream)
	db.Create(&mappingdomain.Mapping{ConnectionID: 1, LocalProductID: 1, UpstreamProductID: 101, IsActive: true})
	q := &recoveringQueue{fail: true}
	svc := newTestProcurementService(db, newTestSiteConnectionService(db, "test-key", t.TempDir()), q)
	if err := svc.CreateForOrder(o.ID); err == nil {
		t.Fatal("queue failure swallowed")
	}
	q.fail = false
	_ = svc.CreateForOrder(o.ID)
	if q.calls != 2 {
		t.Fatal("existing pending procurement not re-enqueued")
	}
	var count int64
	db.Model(&ProcurementOrder{}).Count(&count)
	if count != 1 {
		t.Fatal("duplicate procurement")
	}
}

func TestPeriodicRecoveryCreatesMissingSiblingsAndEnqueuesPending(t *testing.T) {
	db := setupProcurementTestDB(t)
	parent := createProcTestOrder(t, db, "RECOVERY-PARENT", constants.OrderStatusPaid, constants.FulfillmentTypeUpstream)
	first := createProcTestOrder(t, db, "RECOVERY-FIRST", constants.OrderStatusPaid, constants.FulfillmentTypeUpstream)
	second := createProcTestOrder(t, db, "RECOVERY-SECOND", constants.OrderStatusPaid, constants.FulfillmentTypeUpstream)
	for _, o := range []*orderdomain.Order{first, second} {
		if err := db.Model(o).Update("parent_id", parent.ID).Error; err != nil {
			t.Fatal(err)
		}
	}
	db.Create(&mappingdomain.Mapping{ConnectionID: 1, LocalProductID: 1, UpstreamProductID: 101, IsActive: true})
	createTestProcurementOrder(t, db, 1, first.ID, first.OrderNo, "pending")
	q := &recoveringQueue{}
	svc := newTestProcurementService(db, newTestSiteConnectionService(db, "test-key", t.TempDir()), q)
	svc.SyncAcceptedOrders()
	var n int64
	db.Model(&ProcurementOrder{}).Count(&n)
	if n != 2 || q.calls < 2 {
		t.Fatalf("recovery stranded work: procurements=%d enqueue=%d", n, q.calls)
	}
	if err := svc.CreateForOrder(parent.ID); err != nil {
		t.Fatalf("existing sibling blocked resume: %v", err)
	}
}

// ── CreateForOrder tests ──

func TestCreateForOrder_SkipsNonUpstreamItems(t *testing.T) {
	db := setupProcurementTestDB(t)

	order := createProcTestOrder(t, db, "PROC-SKIP-001", constants.OrderStatusPaid, constants.FulfillmentTypeAuto)

	connSvc := newTestSiteConnectionService(db, "test-key", t.TempDir())
	svc := newTestProcurementService(db, connSvc)

	if err := svc.CreateForOrder(order.ID); err != nil {
		t.Fatalf("CreateForOrder: %v", err)
	}

	// 验证没有创建采购单
	var count int64
	db.Model(&ProcurementOrder{}).Count(&count)
	if count != 0 {
		t.Errorf("expected no procurement orders for auto fulfillment, got %d", count)
	}
}

func TestCreateForOrder_IdempotentSkipsDuplicate(t *testing.T) {
	db := setupProcurementTestDB(t)

	order := createProcTestOrder(t, db, "PROC-DUP-001", constants.OrderStatusPaid, constants.FulfillmentTypeUpstream)
	pm := &mappingdomain.Mapping{ConnectionID: 1, LocalProductID: 1, UpstreamProductID: 101, IsActive: true}
	db.Create(pm)

	connSvc := newTestSiteConnectionService(db, "test-key", t.TempDir())
	svc := newTestProcurementService(db, connSvc)

	// 第一次创建成功
	if err := svc.CreateForOrder(order.ID); err != nil {
		t.Fatalf("first CreateForOrder: %v", err)
	}

	// 第二次应该返回 ErrExists
	err := svc.CreateForOrder(order.ID)
	if err != ErrExists {
		t.Errorf("expected ErrExists on duplicate, got: %v", err)
	}
}
