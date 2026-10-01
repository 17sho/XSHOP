package xshopupgrade

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The engine switches two real ELF test executables, not a fake running-hash
// string. Appended tags give distinct identities without requiring a compiler.
func TestJournalProcessChild(t *testing.T) {
	if os.Getenv("SEC_B02_CHILD") != "1" {
		t.Skip("fixture child only")
	}
	f, err := os.OpenFile(os.Getenv("SEC_B02_CHILD_DB"), os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		os.Exit(31)
	}
	if _, err = f.WriteString("writer-started\n"); err != nil {
		os.Exit(32)
	}
	if err = f.Close(); err != nil {
		os.Exit(33)
	}
	fmt.Println("READY")
	for {
		time.Sleep(time.Hour)
	}
}

type journalProcessController struct {
	*journalController
	cmd      *exec.Cmd
	database string
}

func (c *journalProcessController) forceStop() {
	if c.cmd != nil {
		_ = c.cmd.Process.Kill()
		_ = c.cmd.Wait()
		c.cmd = nil
	}
}
func (c *journalProcessController) Stop(ctx context.Context) error {
	if err := c.journalController.Stop(ctx); err != nil {
		return err
	}
	c.forceStop()
	return nil
}
func (c *journalProcessController) Start(ctx context.Context) error {
	if err := c.journalController.Start(ctx); err != nil {
		return err
	}
	cmd := exec.Command(c.target, "-test.run=^TestJournalProcessChild$")
	cmd.Env = append(os.Environ(), "SEC_B02_CHILD=1", "SEC_B02_CHILD_DB="+c.database)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return err
	}
	c.cmd = cmd
	ready := make(chan bool, 1)
	go func() { scanner := bufio.NewScanner(stdout); ready <- scanner.Scan() && scanner.Text() == "READY" }()
	select {
	case ok := <-ready:
		if !ok {
			c.forceStop()
			return errors.New("fixture did not start")
		}
	case <-ctx.Done():
		c.forceStop()
		return ctx.Err()
	case <-time.After(10 * time.Second):
		c.forceStop()
		return errors.New("fixture start timeout")
	}
	return nil
}
func (c *journalProcessController) Healthy(ctx context.Context, expected string) error {
	if err := c.journalController.Healthy(ctx, expected); err != nil {
		return err
	}
	if c.cmd == nil {
		return errors.New("fixture not running")
	}
	got, err := FileHash(fmt.Sprintf("/proc/%d/exe", c.cmd.Process.Pid))
	if err != nil {
		return err
	}
	if got != expected {
		return errors.New("real running ELF identity mismatch")
	}
	return nil
}

func TestJournalRealProcessTerminalFailureRecovery(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	for _, failure := range []string{"stop", "start", "healthy", "save"} {
		t.Run(failure, func(t *testing.T) {
			root := t.TempDir()
			target, next := filepath.Join(root, "app"), filepath.Join(root, "next")
			for path, tag := range map[string]string{target: "A", next: "B"} {
				image := append(append([]byte(nil), raw...), []byte(tag)...)
				if err := os.WriteFile(path, image, 0755); err != nil {
					t.Fatal(err)
				}
			}
			database := filepath.Join(root, "fixture-database")
			if err := os.WriteFile(database, []byte("business-data\n"), 0600); err != nil {
				t.Fatal(err)
			}
			c := &journalProcessController{journalController: &journalController{target: target, fail: failure}, database: database}
			t.Cleanup(c.forceStop)
			e := &Engine{Target: target, StateDir: root, Control: c, Backup: func(context.Context, string) error { return nil }}
			if err := e.Initialize(1); err != nil {
				t.Fatal(err)
			}
			old, _ := FileHash(target)
			newHash, _ := FileHash(next)
			installedHit, rollbackHit := false, false
			e.journalFault = func(name, boundary string) error {
				if name == "state.json" && boundary == "dir_sync" && c.healths == 1 && !installedHit {
					installedHit = true
					return errors.New("installed post-rename EIO")
				}
				if name == "state.json" && boundary == "dir_sync" && c.healths > 1 && failure == "save" {
					rollbackHit = true
					return errors.New("rollback post-rename EIO")
				}
				return nil
			}
			if err := e.Activate(context.Background(), next, old, newHash, 2); err == nil || !installedHit {
				t.Fatal("real installation failure path not exercised")
			}
			if failure == "save" && !rollbackHit {
				t.Fatal("real rollback terminal save not exercised")
			}
			s, err := e.ReadState()
			if err != nil || !s.Pending || s.HighWater != 2 {
				t.Fatalf("real process failure lost fence: %+v %v", s, err)
			}
			c.fail = ""
			fresh := &Engine{Target: target, StateDir: root, Control: c, Backup: e.Backup}
			if err := fresh.Recover(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := c.Healthy(context.Background(), old); err != nil {
				t.Fatal(err)
			}
			s, err = fresh.ReadState()
			if err != nil || s.Pending || s.Result != "rolled_back" || s.HighWater != 2 {
				t.Fatalf("real recovery did not commit: %+v %v", s, err)
			}
			data, err := os.ReadFile(database)
			if err != nil || !strings.HasPrefix(string(data), "business-data\nwriter-started\nwriter-started\n") {
				t.Fatal("writer data was restored/lost")
			}
		})
	}
}
