package xshopupgrade

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUnreadableJournalOverridesCachedAvailableStatus(t *testing.T) {
	h := &Helper{Engine: &Engine{StateDir: t.TempDir()}, status: Status{State: "available", Digest: strings.Repeat("a", 64)}}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/status", nil))
	var s Status
	if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil {
		t.Fatal(err)
	}
	if s.State != "failed" || s.Digest != "" {
		t.Fatalf("unreadable trusted journal advertised installable state: %+v", s)
	}
	if strings.Contains(w.Body.String(), h.Engine.StateDir) {
		t.Fatal("internal path exposed")
	}
}

func TestActivationFailureClaimsRestorationOnlyAfterConfirmedRollback(t *testing.T) {
	e := &Engine{StateDir: t.TempDir()}
	if err := e.Initialize(1); err != nil {
		t.Fatal(err)
	}
	h := &Helper{Engine: e}
	if got := h.activationFailureMessage(); strings.Contains(got, "旧版本已恢复") {
		t.Fatal("idle journal misreported as rollback")
	}
	s, err := e.ReadState()
	if err != nil {
		t.Fatal(err)
	}
	s.Result = "installed"
	if err := e.saveState(s); err != nil {
		t.Fatal(err)
	}
	if got := h.activationFailureMessage(); strings.Contains(got, "旧版本已恢复") {
		t.Fatal("installed journal misreported as rollback")
	}
	s.Result = "rolled_back"
	if err := e.saveState(s); err != nil {
		t.Fatal(err)
	}
	if got := h.activationFailureMessage(); strings.Contains(got, "旧版本已恢复") {
		t.Fatal("label-only rollback misreported as confirmed recovery")
	}

	// A true result requires the real state machine to restore and verify A,
	// not merely a raw JSON label. The old positive fixture lacked this fence.
	e, ctl, next := fixture(t)
	ctl.failFirst = true
	h.Engine = e
	old, err := FileHash(e.Target)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := FileHash(next)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Activate(context.Background(), next, old, candidate, 2); err == nil {
		t.Fatal("fixture did not exercise health-failure rollback")
	}
	if actual, err := FileHash(e.Target); err != nil || actual != old || ctl.running != old {
		t.Fatal("fixture did not restore verified A")
	}
	if got := h.activationFailureMessage(); !strings.Contains(got, "旧版本已恢复") {
		t.Fatal("confirmed rollback not recognized")
	}
	fresh := &Engine{Target: e.Target, StateDir: e.StateDir, Control: ctl}
	h.Engine = fresh
	if got := h.activationFailureMessage(); strings.Contains(got, "旧版本已恢复") {
		t.Fatal("fresh engine trusted terminal label without recovery")
	}
	if err := fresh.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := h.activationFailureMessage(); !strings.Contains(got, "旧版本已恢复") {
		t.Fatal("startup identity-verified rollback not recognized")
	}
	s, err = fresh.ReadState()
	if err != nil {
		t.Fatal(err)
	}
	s.Pending = true
	if err := fresh.saveState(s); err != nil {
		t.Fatal(err)
	}
	if got := h.activationFailureMessage(); strings.Contains(got, "旧版本已恢复") {
		t.Fatal("pending journal misreported as rollback")
	}
}
