package integrationtest

import (
	"context"
	apicredentialdomain "github.com/dujiao-next/internal/modules/apicredential/domain"
	apicredentialstore "github.com/dujiao-next/internal/modules/apicredential/infrastructure/gormstore"
	downstreamcontract "github.com/dujiao-next/internal/modules/downstreamcallback/contract"
	downstreamdomain "github.com/dujiao-next/internal/modules/downstreamcallback/domain"
	"github.com/dujiao-next/internal/modules/downstreamcallback/infrastructure/credentialreader"
	callbackstore "github.com/dujiao-next/internal/modules/downstreamcallback/infrastructure/gormstore"
	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"testing"
	"time"
)

func TestRevokedCallbackPausesAndCanResume(t *testing.T) {
	for _, state := range []string{"disabled", "unapproved", "deleted credential", "disabled owner", "deleted owner", "missing owner", "missing credential"} {
		t.Run(state, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, _ := db.DB()
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { _ = sqlDB.Close() })
			if err := db.AutoMigrate(&userdomain.User{}, &apicredentialdomain.ApiCredential{}, &downstreamdomain.OrderRef{}); err != nil {
				t.Fatal(err)
			}
			owner := userdomain.User{ID: 4, Email: "fixture@example.test", Status: "active"}
			cred := apicredentialdomain.ApiCredential{ID: 5, UserID: owner.ID, ApiKey: "fixture-key", ApiSecret: "fixture-secret", Status: "approved", IsActive: true}
			if err := db.Create(&owner).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&cred).Error; err != nil {
				t.Fatal(err)
			}
			now := time.Unix(1700000000, 0)
			switch state {
			case "disabled":
				db.Model(&cred).Update("is_active", false)
			case "unapproved":
				db.Model(&cred).Update("status", "pending_review")
			case "deleted credential":
				db.Model(&cred).Update("deleted_at", now)
			case "disabled owner":
				db.Model(&owner).Update("status", "disabled")
			case "deleted owner":
				db.Model(&owner).Update("deleted_at", now)
			case "missing owner":
				db.Delete(&owner)
			case "missing credential":
				db.Delete(&cred)
			}
			ref := &downstreamdomain.OrderRef{ID: 3, OrderID: 8, ApiCredentialID: 5, CallbackURL: "https://callback.example.test?token=FIXTURE-URL-SECRET", CallbackStatus: "pending"}
			refs := callbackstore.New(db)
			if err := refs.Create(ref); err != nil {
				t.Fatal(err)
			}
			orders := orderReaderStub{orders: map[uint]*downstreamcontract.OrderSnapshot{8: {ID: 8, Status: "delivered", Fulfillment: &downstreamcontract.Fulfillment{Status: "delivered", Payload: "FIXTURE-CARD", DeliveredAt: &now}}}}
			queue := &callbackQueueStub{}
			deliverer := &delivererStub{}
			svc := newService(refs, orders, credentialreader.New(apicredentialstore.New(db)), queue, deliverer)
			if err := svc.SendCallback(context.Background(), ref.ID); err != nil {
				t.Errorf("revocation must not cause busy retry: %v", err)
			}
			if deliverer.calls != 0 {
				t.Fatal("revoked endpoint received callback")
			}
			ref, err = refs.GetByID(ref.ID)
			if err != nil || ref == nil {
				t.Fatalf("read paused ref: %v", err)
			}
			pending, err := refs.ListPendingCallbacks(10)
			if err != nil || len(pending) != 0 {
				t.Fatalf("paused ref remains in pending sweep: %v", err)
			}
			if ref.CallbackStatus != "paused" || ref.CallbackRetryCount != 0 || ref.LastCallbackAt != nil || len(queue.callbacks) != 0 {
				t.Fatalf("not recoverably paused: %#v", ref)
			}
			if orders.orders[8].Fulfillment.Payload != "FIXTURE-CARD" || ref.CallbackURL == "" {
				t.Fatal("order/ref was destroyed")
			}
			// Restore the same owner/credential; resumption is an explicit legitimate event.
			if state == "missing owner" {
				db.Create(&owner)
			} else {
				db.Model(&owner).Updates(map[string]interface{}{"status": "active", "deleted_at": nil})
			}
			if state == "missing credential" {
				db.Create(&cred)
			} else {
				db.Model(&cred).Updates(map[string]interface{}{"is_active": true, "status": "approved", "deleted_at": nil})
			}
			svc.EnqueueCallback(8)
			ref, err = refs.GetByID(ref.ID)
			if err != nil || ref == nil {
				t.Fatalf("read resumed ref: %v", err)
			}
			if len(queue.callbacks) != 1 || ref.CallbackStatus != "pending" {
				t.Fatal("explicit event did not resume paused ref")
			}
			if err := svc.SendCallback(context.Background(), ref.ID); err != nil {
				t.Fatal(err)
			}
			ref, err = refs.GetByID(ref.ID)
			if err != nil || ref == nil {
				t.Fatalf("read sent ref: %v", err)
			}
			if deliverer.calls != 1 || deliverer.request.Payload.Fulfillment == nil || deliverer.request.Payload.Fulfillment.Payload != "FIXTURE-CARD" || ref.CallbackStatus != "sent" {
				t.Fatal("legitimate delivery did not recover")
			}
		})
	}
}
