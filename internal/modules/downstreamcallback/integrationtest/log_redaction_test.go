package integrationtest

import (
	"context"
	"errors"
	"fmt"
	"github.com/dujiao-next/internal/logger"
	downstreamcontract "github.com/dujiao-next/internal/modules/downstreamcallback/contract"
	downstreamdomain "github.com/dujiao-next/internal/modules/downstreamcallback/domain"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"strings"
	"testing"
)

func TestCallbackLogsDoNotExposeURLOrResponseSecrets(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	previous := logger.L
	logger.L = zap.New(core)
	t.Cleanup(func() { logger.L = previous })
	ref := &downstreamdomain.OrderRef{ID: 3, OrderID: 8, ApiCredentialID: 5, CallbackURL: "https://fixture.example.test/callback?token=FIXTURE-QUERY", CallbackRetryCount: 4}
	refs := &refRepositoryStub{byID: map[uint]*downstreamdomain.OrderRef{3: ref}, byOrderID: map[uint]*downstreamdomain.OrderRef{8: ref}}
	svc := newService(refs, orderReaderStub{orders: map[uint]*downstreamcontract.OrderSnapshot{8: {ID: 8, Status: "paid"}}}, credentialReaderStub{credentials: map[uint]*downstreamcontract.Credential{5: {ID: 5, DeliveryAllowed: true}}}, &callbackQueueStub{}, &delivererStub{err: errors.New("FIXTURE-REFLECTED-CARD")})
	svc.EnqueueCallback(8)
	ref.CallbackRetryCount = 4
	if err := svc.SendCallback(context.Background(), 3); err != nil {
		t.Fatal(err)
	}
	if logs.Len() == 0 {
		t.Fatal("expected diagnostic logs")
	}
	for _, entry := range logs.All() {
		if strings.Contains(fmt.Sprint(entry.ContextMap()), "FIXTURE-") {
			t.Fatalf("sensitive log: %v", entry.ContextMap())
		}
	}
}
