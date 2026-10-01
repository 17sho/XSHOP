package xshopupgrade

import (
	"context"
	"errors"
	"testing"
)

func TestPrepareJournalBoundaryFailuresRetainConsumedFence(t *testing.T) {
	for _, name := range []string{"state.json", recoveryJournal} {
		for _, boundary := range []string{"write", "file_sync", "rename", "dir_sync"} {
			t.Run(name+"/"+boundary, func(t *testing.T) {
				e, c, next := fixture(t)
				old, _ := FileHash(e.Target)
				candidate, _ := FileHash(next)
				hits := 0
				e.journalFault = func(n, b string) error {
					if n == name && b == boundary {
						hits++
						if hits == 2 {
							return errors.New("injected prepared journal failure")
						}
					}
					return nil
				}
				if err := e.Prepare(context.Background(), next, old, candidate, 2, "v", "d"); err == nil {
					t.Fatal("boundary failure hidden")
				}
				fresh := &Engine{Target: e.Target, StateDir: e.StateDir, Control: c, Backup: e.Backup}
				s, err := fresh.ReadState()
				if err != nil || s.HighWater != 2 || !s.Pending || c.started != 0 {
					t.Fatal("lost fence or restarted", s, err)
				}
				if err := fresh.Prepare(context.Background(), next, old, candidate, 2, "v", "d"); err == nil {
					t.Fatal("consumed sequence replayed")
				}
			})
		}
	}
}
