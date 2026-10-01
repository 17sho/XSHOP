package xshopupgrade

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestPrepareDoesNotRestartAndExplicitRestartInstalls(t *testing.T) {
	e, c, next := fixture(t)
	old, _ := FileHash(e.Target)
	candidate, _ := FileHash(next)
	if err := e.Prepare(context.Background(), next, old, candidate, 2, "xshop-preview-b", "manifest-digest"); err != nil {
		t.Fatal(err)
	}
	s, err := e.ReadState()
	if err != nil || s.Result != "prepared" || !s.Pending || c.started != 0 {
		t.Fatalf("prepare restarted or not durable: %+v %v starts=%d", s, err, c.started)
	}
	got, _ := FileHash(e.Target)
	if got != candidate {
		t.Fatal("candidate not swapped")
	}
	// A helper restart must preserve the prepared state without restarting writers.
	fresh := &Engine{Target: e.Target, StateDir: e.StateDir, Control: c, Backup: e.Backup}
	if err := fresh.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.started != 0 {
		t.Fatal("recovery restarted prepared service")
	}
	if err := fresh.Restart(context.Background()); err != nil {
		t.Fatal(err)
	}
	s, err = fresh.ReadState()
	if err != nil || s.Pending || s.Result != "installed" || c.started != 1 {
		t.Fatalf("restart not installed: %+v %v", s, err)
	}
}

func TestPrepareFailureKeepsFenceWithoutRestart(t *testing.T) {
	e, c, next := fixture(t)
	old, _ := FileHash(e.Target)
	candidate, _ := FileHash(next)
	e.Backup = func(context.Context, string) error { return os.ErrPermission }
	if err := e.Prepare(context.Background(), next, old, candidate, 2, "v", "d"); err == nil {
		t.Fatal("failure hidden")
	}
	s, err := e.ReadState()
	if err != nil || s.HighWater != 2 || !s.Pending || c.started != 0 {
		t.Fatalf("lost fence/restarted: %+v %v", s, err)
	}
	got, _ := FileHash(e.Target)
	if got != old {
		t.Fatal("failed backup swapped binary")
	}
	if _, err := os.Stat(filepath.Join(e.StateDir, recoveryJournal)); err != nil {
		t.Fatal("lost durable fence")
	}
}

func TestSanitizedFailureRetainsOperationPhase(t *testing.T) {
	h := &Helper{status: Status{State: "installing", Phase: "prepare"}}
	h.finish(Status{State: "failed", Message: "安装失败"})
	if h.status.Phase != "prepare" {
		t.Fatal("failure lost operation phase", h.status)
	}
}

func TestPermissionHashFailuresAreNotRetried(t *testing.T) {
	for _, err := range []error{syscall.EPERM, fmt.Errorf("fork: %w", syscall.EACCES), errHashWorker} {
		if !permanentHashError(err) {
			t.Fatal("permanent worker error would loop silently", err)
		}
	}
	if permanentHashError(errors.New("PID not ready")) {
		t.Fatal("startup transient rejected")
	}
}

func TestWorkerClearsInheritedCapabilitiesBeforeAssertion(t *testing.T) {
	clear := strings.Index(previewHashWorker, "libc.capset")
	check := strings.Index(previewHashWorker, "worker privileges rejected")
	if clear < 0 || clear > check {
		t.Fatal("ambient SETUID leaves inherited capability in credential child; clear all capability sets before asserting zero")
	}
}

func TestPreparedUncertainSyncFailsClosedInCurrentHelper(t *testing.T) {
	e, _, next := fixture(t)
	old, _ := FileHash(e.Target)
	candidate, _ := FileHash(next)
	hits := 0
	e.journalFault = func(name, b string) error {
		if name == "state.json" && b == "dir_sync" {
			hits++
			if hits == 2 {
				return errors.New("uncertain prepared sync")
			}
		}
		return nil
	}
	if err := e.Prepare(context.Background(), next, old, candidate, 2, "v", "d"); err == nil {
		t.Fatal("failure hidden")
	}
	if s, err := e.ReadState(); err == nil {
		t.Fatal("uncertain prepared durability advertised", s)
	}
}

func TestPreparedRecoveryResynchronizesJournal(t *testing.T) {
	e, c, next := fixture(t)
	old, _ := FileHash(e.Target)
	candidate, _ := FileHash(next)
	if err := e.Prepare(context.Background(), next, old, candidate, 2, "v", "d"); err != nil {
		t.Fatal(err)
	}
	fresh := &Engine{Target: e.Target, StateDir: e.StateDir, Control: c}
	fresh.journalFault = func(n, b string) error {
		if n == "state.json" && b == "recover_sync" {
			return errors.New("sync unavailable")
		}
		return nil
	}
	if err := fresh.Recover(context.Background()); err == nil {
		t.Fatal("prepared recovery trusted unsynchronized state")
	}
	if c.started != 0 {
		t.Fatal("recovery restarted")
	}
}
