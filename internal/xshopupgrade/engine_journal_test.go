package xshopupgrade

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type journalController struct {
	target                 string
	stops, starts, healths int
	fail                   string
}

func (c *journalController) Stop(context.Context) error {
	c.stops++
	if c.stops > 1 && c.fail == "stop" {
		return errors.New("rollback stop injected")
	}
	return nil
}
func (c *journalController) Start(context.Context) error {
	c.starts++
	if c.starts > 1 && c.fail == "start" {
		return errors.New("rollback start injected")
	}
	return nil
}
func (c *journalController) Healthy(_ context.Context, want string) error {
	c.healths++
	if c.healths > 1 && c.fail == "healthy" {
		return errors.New("rollback health injected")
	}
	got, err := FileHash(c.target)
	if err != nil {
		return err
	}
	if got != want {
		return errors.New("running fixture identity mismatch")
	}
	return nil
}

func TestJournalBootstrapCannotEraseOrphanFence(t *testing.T) {
	e, _, _ := fixture(t)
	old, _ := FileHash(e.Target)
	rollback := filepath.Join(e.StateDir, "previous")
	if err := os.WriteFile(rollback, []byte("A"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := e.saveJournal(recoveryJournal, State{HighWater: 9, Pending: true, Rollback: rollback, OldHash: old}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(e.StateDir, "state.json")); err != nil {
		t.Fatal(err)
	}
	if err := e.Initialize(1); err == nil {
		t.Fatal("orphan recovery fence allowed bootstrap high-water rewind")
	}
}

func TestJournalLegacyInstalledCannotSkipIdentityRecovery(t *testing.T) {
	e, c, _ := fixture(t)
	old, _ := FileHash(e.Target)
	rollback := filepath.Join(e.StateDir, "previous")
	if err := os.WriteFile(rollback, []byte("A"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(e.Target, []byte("B"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := e.saveState(State{HighWater: 2, Rollback: rollback, OldHash: old, Result: "installed"}); err != nil {
		t.Fatal(err)
	}
	s, err := e.ReadState()
	if err != nil || !s.Pending {
		t.Fatalf("legacy installed identity unknown must require recovery: %+v %v", s, err)
	}
	if err := e.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.running != old {
		t.Fatal("legacy recovery did not verify old binary")
	}
}

func TestJournalTerminalBoundaryRollbackMatrix(t *testing.T) {
	for _, boundary := range []string{"write", "file_sync", "rename", "dir_sync", "remove", "remove_sync"} {
		for _, rollbackFailure := range []string{"stop", "start", "healthy", "save_write", "save_file_sync", "save_rename", "save_dir_sync", "fence"} {
			t.Run(boundary+"/"+rollbackFailure, func(t *testing.T) {
				e, _, next := fixture(t)
				c := &journalController{target: e.Target, fail: rollbackFailure}
				e.Control = c
				old, _ := FileHash(e.Target)
				newHash, _ := FileHash(next)
				database := filepath.Join(e.StateDir, "database-live")
				if err := os.WriteFile(database, []byte("writer data must survive"), 0600); err != nil {
					t.Fatal(err)
				}
				terminalHit, rollbackSaveHit, rollbackFenceHit := false, false, false
				injected := errors.New("journal boundary EIO")
				e.journalFault = func(name, stage string) error {
					terminalName := "state.json"
					if boundary == "remove" || boundary == "remove_sync" {
						terminalName = recoveryJournal
					}
					if !terminalHit && c.healths == 1 && name == terminalName && stage == boundary {
						terminalHit = true
						if boundary == "dir_sync" {
							raw, err := os.ReadFile(filepath.Join(e.StateDir, "state.json"))
							if err != nil || !bytes.Contains(raw, []byte(`"result":"installed"`)) {
								t.Fatal("post-rename path did not execute")
							}
						}
						return injected
					}
					if terminalHit && strings.HasPrefix(rollbackFailure, "save_") && c.healths > 1 && name == "state.json" && stage == strings.TrimPrefix(rollbackFailure, "save_") {
						rollbackSaveHit = true
						return injected
					}
					if terminalHit && rollbackFailure == "fence" && name == recoveryJournal && stage == "write" {
						rollbackFenceHit = true
						return injected
					}
					return nil
				}
				if err := e.Activate(context.Background(), next, old, newHash, 2); err == nil {
					t.Fatal("combined error hidden")
				}
				if !terminalHit || c.starts < 1 || c.healths < 1 {
					t.Fatal("terminal path was not executed")
				}
				if strings.HasPrefix(rollbackFailure, "save_") && !rollbackSaveHit {
					t.Fatal("rollback terminal save path not executed")
				}
				if rollbackFailure == "fence" && !rollbackFenceHit {
					t.Fatal("rollback fence path not executed")
				}
				if rollbackFailure == "stop" && c.stops != 2 {
					t.Fatal("rollback Stop not executed")
				}
				if rollbackFailure == "start" && c.starts != 2 {
					t.Fatal("rollback Start not executed")
				}
				if rollbackFailure == "healthy" && c.healths != 2 {
					t.Fatal("rollback Healthy not executed")
				}
				s, err := e.ReadState()
				if err == nil && (!s.Pending || s.HighWater != 2) {
					t.Fatalf("fence lost: %+v", s)
				}
				// Simulate daemon restart with no injected runtime latch. Durable fence
				// must preserve both recovery and high water even if state says terminal.
				freshControl := &journalController{target: e.Target, stops: 1, fail: "stop"}
				// If cleanup unlinked the fence and every repair write failed,
				// only the already-durable terminal remains. Restart must still
				// gate success on real identity/health verification, not its label.
				if boundary == "remove_sync" && rollbackFailure == "fence" {
					freshControl.fail = "healthy"
					freshControl.healths = 1
				}
				fresh := &Engine{Target: e.Target, StateDir: e.StateDir, Control: freshControl, Backup: e.Backup}
				s, err = fresh.ReadState()
				if err != nil || !s.Pending || s.HighWater != 2 {
					t.Fatalf("restart lost durable fence: %+v %v", s, err)
				}
				if err := fresh.Recover(context.Background()); err == nil {
					t.Fatal("failed restart recovery skipped")
				}
				if err := fresh.Activate(context.Background(), next, old, newHash, 3); err == nil {
					t.Fatal("newer install crossed fence")
				}
				freshControl.fail = ""
				if err := fresh.Recover(context.Background()); err != nil {
					t.Fatal(err)
				}
				s, err = fresh.ReadState()
				got, _ := FileHash(e.Target)
				if err != nil || s.Pending || s.Result != "rolled_back" || s.HighWater != 2 || got != old {
					t.Fatalf("recovery did not finish durably: %+v %v", s, err)
				}
				data, err := os.ReadFile(database)
				if err != nil || string(data) != "writer data must survive" {
					t.Fatal("database was rewound")
				}
			})
		}
	}
}

func TestJournalInitialBoundaryFailuresFailClosed(t *testing.T) {
	for _, name := range []string{recoveryJournal, "state.json"} {
		for _, boundary := range []string{"write", "file_sync", "rename", "dir_sync"} {
			t.Run(name+"/"+boundary, func(t *testing.T) {
				e, c, next := fixture(t)
				old, _ := FileHash(e.Target)
				newHash, _ := FileHash(next)
				hit := false
				e.journalFault = func(n, b string) error {
					if n == name && b == boundary {
						hit = true
						return errors.New("initial boundary EIO")
					}
					return nil
				}
				if err := e.Activate(context.Background(), next, old, newHash, 2); err == nil || !hit {
					t.Fatal("initial fault not executed")
				}
				got, _ := FileHash(e.Target)
				if c.started != 0 || got != old {
					t.Fatal("service changed before durable intent")
				}
				if err := e.Activate(context.Background(), next, old, newHash, 3); err == nil {
					t.Fatal("unknown persistence allowed next install")
				}
				if name == "state.json" || boundary == "dir_sync" {
					fresh := &Engine{Target: e.Target, StateDir: e.StateDir, Control: c, Backup: e.Backup}
					s, err := fresh.ReadState()
					if err != nil || !s.Pending || s.HighWater != 2 {
						t.Fatalf("durable high water lost: %+v %v", s, err)
					}
				}
			})
		}
	}
}

func TestJournalTotalWriteLossFailsClosedWithoutDatabaseRestore(t *testing.T) {
	e, _, next := fixture(t)
	c := &journalController{target: e.Target}
	e.Control = c
	old, _ := FileHash(e.Target)
	newHash, _ := FileHash(next)
	database := filepath.Join(e.StateDir, "database-live")
	if err := os.WriteFile(database, []byte("advanced writer state"), 0600); err != nil {
		t.Fatal(err)
	}
	e.journalFault = func(_, _ string) error {
		if c.healths > 0 {
			return errors.New("all journal storage writes unavailable")
		}
		return nil
	}
	if err := e.Activate(context.Background(), next, old, newHash, 2); err == nil {
		t.Fatal("total write loss hidden")
	}
	if c.stops != 1 || c.starts != 1 {
		t.Fatal("rollback modified service without persisting its fence")
	}
	if _, err := e.ReadState(); err == nil {
		t.Fatal("unknown durability exposed terminal state")
	}
	if err := e.Recover(context.Background()); err == nil {
		t.Fatal("runtime latch did not fail closed")
	}
	if err := e.Activate(context.Background(), next, old, newHash, 3); err == nil {
		t.Fatal("runtime latch allowed upgrade")
	}
	freshControl := &journalController{target: e.Target}
	fresh := &Engine{Target: e.Target, StateDir: e.StateDir, Control: freshControl, Backup: e.Backup, journalFault: func(_, _ string) error { return errors.New("storage still unwritable") }}
	s, err := fresh.ReadState()
	if err != nil || !s.Pending || s.HighWater != 2 {
		t.Fatalf("durable intent lost: %+v %v", s, err)
	}
	if err := fresh.Recover(context.Background()); err == nil {
		t.Fatal("unwritable restart recovery succeeded")
	}
	if freshControl.stops != 0 || freshControl.starts != 0 {
		t.Fatal("recovery changed service before durable fence")
	}
	data, err := os.ReadFile(database)
	if err != nil || string(data) != "advanced writer state" {
		t.Fatal("database restored")
	}
	// No engine can manufacture a durable write on failed media. Recovery only
	// resumes after storage is repaired and a fresh engine is constructed.
	repaired := &Engine{Target: e.Target, StateDir: e.StateDir, Control: freshControl, Backup: e.Backup}
	if err := repaired.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	s, err = repaired.ReadState()
	if err != nil || s.Pending || s.HighWater != 2 || s.Result != "rolled_back" {
		t.Fatalf("repaired recovery failed: %+v %v", s, err)
	}
	data, err = os.ReadFile(database)
	if err != nil || string(data) != "advanced writer state" {
		t.Fatal("repaired recovery restored database")
	}
}

func TestJournalRestartTerminalRequiresVerification(t *testing.T) {
	e, _, next := fixture(t)
	old, _ := FileHash(e.Target)
	newHash, _ := FileHash(next)
	if err := e.Activate(context.Background(), next, old, newHash, 2); err != nil {
		t.Fatal(err)
	}
	fresh := &Engine{Target: e.Target, StateDir: e.StateDir, Control: e.Control, Backup: e.Backup}
	s, err := fresh.ReadState()
	if err != nil || !s.Pending {
		t.Fatalf("unverified restart terminal exposed success: %+v %v", s, err)
	}
	if err := fresh.Activate(context.Background(), next, newHash, newHash, 3); err == nil {
		t.Fatal("install before startup verification accepted")
	}
	if err := fresh.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	s, err = fresh.ReadState()
	if err != nil || s.Pending || s.Result != "installed" {
		t.Fatalf("verified terminal unavailable: %+v %v", s, err)
	}
}

func TestJournalTerminalRecoverySyncFailClosed(t *testing.T) {
	e, _, next := fixture(t)
	old, _ := FileHash(e.Target)
	newHash, _ := FileHash(next)
	if err := e.Activate(context.Background(), next, old, newHash, 2); err != nil {
		t.Fatal(err)
	}
	hit := false
	fresh := &Engine{Target: e.Target, StateDir: e.StateDir, Control: e.Control, Backup: e.Backup}
	fresh.journalFault = func(name, boundary string) error {
		if name == "state.json" && boundary == "recover_sync" {
			hit = true
			return errors.New("recovery directory sync EIO")
		}
		return nil
	}
	if err := fresh.Recover(context.Background()); err == nil || !hit {
		t.Fatal("recovery directory durability failure was skipped")
	}
	if _, err := fresh.ReadState(); err == nil {
		t.Fatal("unknown recovery durability exposed success")
	}
	if err := fresh.Activate(context.Background(), next, newHash, newHash, 3); err == nil {
		t.Fatal("unknown recovery durability accepted install")
	}
}

func TestJournalTerminalRecoveryRevalidatesIdentity(t *testing.T) {
	for _, result := range []string{"installed", "rolled_back"} {
		for _, failure := range []string{"disk", "health"} {
			t.Run(result+"/"+failure, func(t *testing.T) {
				e, _, next := fixture(t)
				c := &journalController{target: e.Target}
				e.Control = c
				old, _ := FileHash(e.Target)
				newHash, _ := FileHash(next)
				if err := e.Activate(context.Background(), next, old, newHash, 2); err != nil {
					t.Fatal(err)
				}
				if result == "rolled_back" {
					s, err := e.ReadState()
					if err != nil {
						t.Fatal(err)
					}
					if err := e.rollback(context.Background(), s); err != nil {
						t.Fatal(err)
					}
				}
				fresh := &Engine{Target: e.Target, StateDir: e.StateDir, Control: c, Backup: e.Backup}
				if failure == "disk" {
					if err := os.WriteFile(e.Target, []byte("wrong executable"), 0755); err != nil {
						t.Fatal(err)
					}
				} else {
					c.fail = "healthy"
				}
				if err := fresh.Recover(context.Background()); err == nil {
					t.Fatal("terminal label skipped identity/health validation")
				}
				s, err := fresh.ReadState()
				if err != nil || !s.Pending || s.HighWater != 2 {
					t.Fatalf("terminal recovery failure not fenced: %+v %v", s, err)
				}
				if err := fresh.Activate(context.Background(), next, old, newHash, 3); err == nil {
					t.Fatal("terminal recovery failure allowed upgrade")
				}
			})
		}
	}
}

// Executed under the evidence ptrace harness: a real StateDir fsync syscall
// is replaced with EIO only after the installed journal rename is visible.
func TestJournalTerminalSyncRollbackFailure(t *testing.T) {
	if os.Getenv("SEC_B02_PTRACE") == "" {
		t.Skip("requires syscall fault harness")
	}
	e, _, next := fixture(t)
	c := &journalController{target: e.Target, fail: os.Getenv("SEC_B02_ROLLBACK")}
	e.Control = c
	old, _ := FileHash(e.Target)
	newHash, _ := FileHash(next)
	marker := os.Getenv("SEC_B02_MARKER")
	if err := os.WriteFile(marker, []byte(e.StateDir), 0600); err != nil {
		t.Fatal(err)
	}
	err := e.Activate(context.Background(), next, old, newHash, 2)
	if err == nil {
		t.Fatal("terminal durability failure hidden")
	}
	s, stateErr := e.ReadState()
	if stateErr == nil && (!s.Pending || s.HighWater != 2) {
		t.Fatalf("SEC-B02: failed commit/rollback lost recovery fence: %+v; activate=%v", s, err)
	}
	// A fresh engine must not silently skip a failed recovery.
	fresh := &Engine{Target: e.Target, StateDir: e.StateDir, Control: c, Backup: e.Backup}
	c.fail = "stop"
	if err := fresh.Recover(context.Background()); err == nil {
		t.Fatal("SEC-B02: restart recovery falsely succeeded")
	}
	if err := fresh.Activate(context.Background(), next, old, newHash, 3); err == nil {
		t.Fatal("unresolved operation accepted a newer upgrade")
	}
	if _, err := os.Stat(filepath.Join(e.StateDir, "state.json")); err != nil {
		t.Fatal(err)
	}
}
