package xshopupgrade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestPreparedJournalIdentityMustAgree(t *testing.T) {
	for _, journal := range []string{"state.json", recoveryJournal} {
		for _, field := range []string{"missing", "version", "digest", "rollback", "old", "new", "highwater", "prepared", "pending"} {
			t.Run(journal+"/"+field, func(t *testing.T) {
				e, c, next := fixture(t)
				old, _ := FileHash(e.Target)
				n, _ := FileHash(next)
				if err := e.Prepare(context.Background(), next, old, n, 2, "v", "d"); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(e.StateDir, journal)
				if field == "missing" {
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				} else {
					raw, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					var s State
					if err := json.Unmarshal(raw, &s); err != nil {
						t.Fatal(err)
					}
					switch field {
					case "version":
						s.Version = "other"
					case "digest":
						s.Digest = "other"
					case "rollback":
						s.Rollback = "other"
					case "old":
						s.OldHash = "other"
					case "new":
						s.NewHash = "other"
					case "highwater":
						s.HighWater++
					case "prepared":
						s.Prepared = false
					case "pending":
						s.Pending = false
					}
					raw, err = json.Marshal(s)
					if err != nil {
						t.Fatal(err)
					}
					if err = os.WriteFile(path, raw, 0600); err != nil {
						t.Fatal(err)
					}
				}
				fresh := &Engine{Target: e.Target, StateDir: e.StateDir, Control: c}
				if s, err := fresh.ReadState(); err == nil {
					t.Fatalf("inconsistent journals accepted: %+v", s)
				}
				if err := fresh.Restart(context.Background()); err == nil {
					t.Fatal("inconsistent restart accepted")
				}
				if c.started != 0 {
					t.Fatal("writers restarted")
				}
			})
		}
	}
}

func TestAbsentWorkerTargetIsRetryable(t *testing.T) {
	uid, gid, err := previewCredentials()
	if err != nil {
		t.Fatal(err)
	}
	_, err = hashPreviewIdentity(context.Background(), previewIdentity{PID: 2147483647, UID: uid, GID: gid, Start: "1"})
	if err == nil || permanentHashError(err) {
		t.Fatalf("absent target must fail transiently: %v", err)
	}
}

func TestWorkerTargetChurnProtocol(t *testing.T) {
	// Exercise the actual isolated worker with synthetic target exceptions only;
	// no service or real target process is touched.
	for _, failure := range []string{"FileNotFoundError()", "ProcessLookupError()", "TargetChanged()", "PermissionError()", "RuntimeError('target credentials rejected')"} {
		t.Run(failure, func(t *testing.T) {
			uid, gid, err := previewCredentials()
			if err != nil {
				t.Fatal(err)
			}
			script := strings.Replace(previewHashWorker, "d=os.open('/proc/'+str(i['pid']),os.O_RDONLY|os.O_DIRECTORY|os.O_CLOEXEC)", "raise "+failure, 1)
			cmd := exec.Command("/usr/bin/python3", "-I", "-c", script)
			cmd.Stdin = strings.NewReader(fmt.Sprintf(`{"pid":2147483647,"uid":%d,"gid":%d,"start":"1"}`, uid, gid))
			cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uid, Gid: gid, Groups: []uint32{}}}
			err = cmd.Run()
			var exit *exec.ExitError
			if !errors.As(err, &exit) {
				t.Fatalf("worker did not reject: %v", err)
			}
			transient := failure == "FileNotFoundError()" || failure == "ProcessLookupError()" || failure == "TargetChanged()"
			if (exit.ExitCode() == 75) != transient {
				t.Fatalf("wrong protocol exit %d for %s", exit.ExitCode(), failure)
			}
		})
	}
}

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
