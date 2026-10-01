package gormstore

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	downstreamcontract "github.com/dujiao-next/internal/modules/downstreamcallback/contract"
	downstreamdomain "github.com/dujiao-next/internal/modules/downstreamcallback/domain"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestCreateClaimsAreAtomicAndLegacySafe(t *testing.T) {
	store := openTestStore(t)
	// Historical duplicates are retained but must never be selected arbitrarily.
	for _, id := range []uint{11, 12} {
		if err := store.Create(&downstreamdomain.OrderRef{OrderID: id, ApiCredentialID: 5, DownstreamOrderNo: "legacy"}); err != nil {
			t.Fatal(err)
		}
	}
	claims, ok := any(store).(interface {
		PrepareCreateClaims() error
		ReserveCreate(uint, string) (bool, error)
	})
	if !ok {
		t.Fatal("durable atomic create claims missing")
	}
	if err := claims.PrepareCreateClaims(); err != nil {
		t.Fatal(err)
	}
	if err := claims.PrepareCreateClaims(); err != nil {
		t.Fatalf("repeat migration: %v", err)
	}
	if won, err := claims.ReserveCreate(5, "legacy"); err != nil || won {
		t.Fatal("legacy request not fenced")
	}
	if _, err := store.GetByCredentialAndDownstreamNo(5, "legacy"); err == nil {
		t.Fatal("ambiguous duplicate selected")
	}
	winners := 0
	for i := 0; i < 2; i++ {
		won, err := claims.ReserveCreate(5, "new")
		if err != nil {
			t.Fatal(err)
		}
		if won {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("claim winners=%d", winners)
	}
	var count int64
	store.db.Model(&downstreamdomain.OrderRef{}).Count(&count)
	if count != 2 {
		t.Fatal("migration rewrote legacy refs")
	}
}

func TestConcurrentCreateClaimsHaveOneWinner(t *testing.T) {
	store := openTestStore(t)
	if err := store.PrepareCreateClaims(); err != nil {
		t.Fatal(err)
	}
	// One SQLite writer, but independently concurrent callers contend on DB uniqueness.
	sqlDB, _ := store.db.DB()
	sqlDB.SetMaxOpenConns(1)
	var winners atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			won, err := store.ReserveCreate(7, "concurrent")
			if err != nil {
				t.Error(err)
			}
			if won {
				winners.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("winners=%d", winners.Load())
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := fmt.Sprintf("file:downstream_callback_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&downstreamdomain.OrderRef{}); err != nil {
		t.Fatalf("migrate order refs: %v", err)
	}
	return New(db)
}

func TestStorePersistsAndFindsOrderReference(t *testing.T) {
	store := openTestStore(t)
	ref := &downstreamdomain.OrderRef{
		OrderID:           8,
		ApiCredentialID:   5,
		DownstreamOrderNo: "remote-8",
		CallbackURL:       "https://callback.example.test",
		CallbackStatus:    downstreamdomain.StatusPending,
	}
	if err := store.Create(ref); err != nil {
		t.Fatalf("create ref: %v", err)
	}

	byOrder, err := store.GetByOrderID(ref.OrderID)
	if err != nil || byOrder == nil || byOrder.ID != ref.ID {
		t.Fatalf("get by order: ref=%#v err=%v", byOrder, err)
	}
	byRemote, err := store.GetByCredentialAndDownstreamNo(ref.ApiCredentialID, ref.DownstreamOrderNo)
	if err != nil || byRemote == nil || byRemote.ID != ref.ID {
		t.Fatalf("get by credential and downstream no: ref=%#v err=%v", byRemote, err)
	}
}

func TestStoreFiltersPendingAndCredentialLists(t *testing.T) {
	store := openTestStore(t)
	refs := []*downstreamdomain.OrderRef{
		{OrderID: 1, ApiCredentialID: 5, CallbackURL: "https://one.example", CallbackStatus: downstreamdomain.StatusPending},
		{OrderID: 2, ApiCredentialID: 5, CallbackURL: "", CallbackStatus: downstreamdomain.StatusPending},
		{OrderID: 3, ApiCredentialID: 5, CallbackURL: "https://three.example", CallbackStatus: downstreamdomain.StatusSent},
		{OrderID: 4, ApiCredentialID: 6, CallbackURL: "https://four.example", CallbackStatus: downstreamdomain.StatusPending},
	}
	for _, ref := range refs {
		if err := store.Create(ref); err != nil {
			t.Fatalf("create ref for order %d: %v", ref.OrderID, err)
		}
	}

	pending, err := store.ListPendingCallbacks(10)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 2 || pending[0].OrderID != 1 || pending[1].OrderID != 4 {
		t.Fatalf("pending refs mismatch: %#v", pending)
	}

	listed, total, err := store.ListByCredentialID(5, downstreamcontract.RefListFilter{
		CallbackStatus: downstreamdomain.StatusPending,
		Page:           1,
		PageSize:       1,
	})
	if err != nil {
		t.Fatalf("list by credential: %v", err)
	}
	if total != 2 || len(listed) != 1 || listed[0].OrderID != 2 {
		t.Fatalf("credential list mismatch: total=%d refs=%#v", total, listed)
	}
}
