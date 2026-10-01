package xshopupgrade

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

type disposableController struct {
	target, db string
	cmd        *exec.Cmd
	unhealthy  bool
	starts     int
}

func (c *disposableController) Stop(context.Context) error {
	if c.cmd != nil {
		c.cmd.Process.Kill()
		c.cmd.Wait()
		c.cmd = nil
	}
	return nil
}
func (c *disposableController) Start(context.Context) error {
	c.starts++
	c.cmd = exec.Command(c.target, "30")
	if err := c.cmd.Start(); err != nil {
		return err
	}
	return exec.Command("/usr/bin/python3", "-c", `import sqlite3,sys;c=sqlite3.connect(sys.argv[1]);c.execute("insert into transactions(value) values(?)",("writer-start",));c.commit();c.close()`, c.db).Run()
}
func (c *disposableController) Healthy(_ context.Context, want string) error {
	if c.unhealthy {
		c.unhealthy = false
		return errors.New("unhealthy candidate")
	}
	got, err := FileHash("/proc/" + strconv.Itoa(c.cmd.Process.Pid) + "/exe")
	if err != nil || got != want {
		return errors.New("running identity mismatch")
	}
	return nil
}
func TestDisposableTwoPhaseProcessAndNoDataRewind(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "app")
	next := filepath.Join(root, "next")
	db := filepath.Join(root, "business.db")
	raw, err := os.ReadFile("/usr/bin/sleep")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(target, raw, 0755)
	os.WriteFile(next, append(raw, []byte("candidate")...), 0755)
	python(t, "-c", `import sqlite3,sys;c=sqlite3.connect(sys.argv[1]);c.execute("create table transactions(id integer primary key,value text)");c.execute("insert into transactions(value) values(?)",("initial",));c.commit();c.close()`, db)
	ctl := &disposableController{target: target, db: db}
	defer ctl.Stop(context.Background())
	ctl.Start(context.Background())
	pid := ctl.cmd.Process.Pid
	e := &Engine{Target: target, StateDir: root, Control: ctl, Backup: func(_ context.Context, dir string) error {
		return atomicCopy(db, filepath.Join(dir, "database.backup"), 0600)
	}}
	e.Initialize(1)
	old, _ := FileHash(target)
	candidate, _ := FileHash(next)
	if err := e.Prepare(context.Background(), next, old, candidate, 2, "v2", "d2"); err != nil {
		t.Fatal(err)
	}
	if ctl.cmd.Process.Pid != pid || ctl.starts != 1 {
		t.Fatal("prepare restarted old process")
	}
	if err := ctl.Healthy(context.Background(), old); err != nil {
		t.Fatal("old inode not serving", err)
	}
	if err := e.Restart(context.Background()); err != nil {
		t.Fatal(err)
	}
	s, err := e.ReadState()
	if err != nil || s.Result != "installed" {
		t.Fatal(s, err)
	}
	// Next candidate starts a writer then fails health. Rollback must retain its write.
	os.WriteFile(next, append(raw, []byte("unhealthy")...), 0755)
	bad, _ := FileHash(next)
	if err := e.Prepare(context.Background(), next, candidate, bad, 3, "v3", "d3"); err != nil {
		t.Fatal(err)
	}
	before := string(python(t, "-c", `import sqlite3,sys;c=sqlite3.connect(sys.argv[1]);print(c.execute("select count(*) from transactions").fetchone()[0])`, db))
	ctl.unhealthy = true
	if err := e.Restart(context.Background()); err == nil {
		t.Fatal("unhealthy accepted")
	}
	after := string(python(t, "-c", `import sqlite3,sys;c=sqlite3.connect(sys.argv[1]);print(c.execute("select count(*) from transactions").fetchone()[0])`, db))
	if after <= before {
		t.Fatal("candidate transactions rewound")
	}
	s, err = e.ReadState()
	if err != nil || s.HighWater != 3 || s.Pending || s.Result != "rolled_back" {
		t.Fatal(s, err)
	}
	if err := ctl.Healthy(context.Background(), candidate); err != nil {
		t.Fatal("rollback hash", err)
	}
}
