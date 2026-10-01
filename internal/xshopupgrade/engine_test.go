package xshopupgrade

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type testController struct {
	started   int
	failFirst bool
	running   string
}

func (c *testController) Stop(context.Context) error  { return nil }
func (c *testController) Start(context.Context) error { c.started++; return nil }
func (c *testController) Healthy(_ context.Context, hash string) error {
	if c.failFirst && c.started == 1 {
		return errors.New("unhealthy")
	}
	c.running = hash
	return nil
}
func fixture(t *testing.T) (*Engine, *testController, string) {
	t.Helper()
	root := t.TempDir()
	target := filepath.Join(root, "app")
	os.WriteFile(target, []byte("A"), 0755)
	next := filepath.Join(root, "next")
	os.WriteFile(next, []byte("B"), 0755)
	ctl := &testController{}
	e := &Engine{Target: target, StateDir: root, Control: ctl, Backup: func(_ context.Context, dir string) error {
		return os.WriteFile(filepath.Join(dir, "database.backup"), []byte("preserved"), 0600)
	}}
	if err := e.Initialize(1); err != nil {
		t.Fatal(err)
	}
	return e, ctl, next
}
func TestInstallAtomicAndReplayFence(t *testing.T) {
	e, c, next := fixture(t)
	before, _ := FileHash(e.Target)
	after, _ := FileHash(next)
	if err := e.Activate(context.Background(), next, before, after, 2); err != nil {
		t.Fatal(err)
	}
	got, _ := FileHash(e.Target)
	if got != after || c.running != after {
		t.Fatal("candidate not active")
	}
	if err := e.Activate(context.Background(), next, after, after, 2); err == nil {
		t.Fatal("replay accepted")
	}
}
func TestHealthFailureRestoresOnlyBinary(t *testing.T) {
	e, c, next := fixture(t)
	c.failFirst = true
	before, _ := FileHash(e.Target)
	after, _ := FileHash(next)
	if err := e.Activate(context.Background(), next, before, after, 2); err == nil {
		t.Fatal("failure hidden")
	}
	got, _ := FileHash(e.Target)
	if got != before || c.running != before {
		t.Fatal("rollback did not restore A")
	}
	state, err := e.ReadState()
	if err != nil || state.HighWater != 2 || state.Pending {
		t.Fatalf("invalid rollback state: %+v %v", state, err)
	}
}
func TestTamperDoesNotStopOrConsumeSequence(t *testing.T) {
	e, c, next := fixture(t)
	before, _ := FileHash(e.Target)
	if err := e.Activate(context.Background(), next, before, before, 2); err == nil {
		t.Fatal("tamper accepted")
	}
	s, _ := e.ReadState()
	if c.started != 0 || s.HighWater != 1 {
		t.Fatal("tamper had effects")
	}
}
func TestBackupFailureRestartsOldBinary(t *testing.T) {
	e, c, next := fixture(t)
	e.Backup = func(context.Context, string) error { return errors.New("disk full") }
	before, _ := FileHash(e.Target)
	after, _ := FileHash(next)
	if err := e.Activate(context.Background(), next, before, after, 2); err == nil {
		t.Fatal("backup error hidden")
	}
	got, _ := FileHash(e.Target)
	if got != before || c.running != before {
		t.Fatal("backup failure left service down")
	}
}
func TestRecoveryUsesJournalAndNeverDatabaseRestore(t *testing.T) {
	e, c, next := fixture(t)
	before, _ := FileHash(e.Target)
	rollback := filepath.Join(e.StateDir, "rollback")
	os.WriteFile(rollback, []byte("A"), 0755)
	os.WriteFile(e.Target, []byte("B"), 0755)
	if err := e.saveState(State{HighWater: 2, Pending: true, Rollback: rollback, OldHash: before}); err != nil {
		t.Fatal(err)
	}
	if err := e.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, _ := FileHash(e.Target)
	if got != before || c.running != before {
		t.Fatal("journal recovery failed")
	}
	_ = next
}
func TestTargetSymlinkRejected(t *testing.T) {
	e, _, next := fixture(t)
	h, _ := FileHash(e.Target)
	os.Remove(e.Target)
	os.Symlink(next, e.Target)
	if err := e.Activate(context.Background(), next, h, h, 2); err == nil {
		t.Fatal("symlink accepted")
	}
}
