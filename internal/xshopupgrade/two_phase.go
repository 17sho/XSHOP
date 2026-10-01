package xshopupgrade

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// Prepare leaves the existing executable inode serving. The replacement is
// copied and renamed in Target's own directory, never across filesystems.
func (e *Engine) Prepare(ctx context.Context, candidate, expectedOld, expectedNew string, sequence uint64, version, digest string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	s, err := e.ReadState()
	if err != nil {
		return err
	}
	if s.Pending || sequence <= s.HighWater {
		return errors.New("pending recovery or sequence replay")
	}
	if err = regular(e.Target); err != nil {
		return err
	}
	if err = regular(candidate); err != nil {
		return err
	}
	old, err := FileHash(e.Target)
	if err != nil || old != expectedOld {
		return errors.New("baseline changed")
	}
	next, err := FileHash(candidate)
	if err != nil || next != expectedNew {
		return errors.New("candidate changed")
	}
	dir, err := os.MkdirTemp(e.StateDir, "checkpoint-")
	if err != nil {
		return err
	}
	rollback := filepath.Join(dir, "previous-binary")
	if err = atomicCopy(e.Target, rollback, 0755); err != nil {
		return err
	}
	if err = syncDir(e.StateDir); err != nil {
		return err
	}
	s = State{HighWater: sequence, Pending: true, Rollback: rollback, OldHash: old, NewHash: next, Result: "preparing", Version: version, Digest: digest}
	if err = e.fence(s); err != nil {
		return err
	}
	if err = e.saveState(s); err != nil {
		return err
	}
	if err = e.Backup(ctx, dir); err != nil {
		return err
	}
	next, err = FileHash(candidate)
	if err != nil || next != expectedNew {
		return errors.New("staged candidate changed")
	}
	if err = atomicCopy(candidate, e.Target, 0755); err != nil {
		return err
	}
	s.Prepared = true
	s.Result = "prepared"
	if err = e.fence(s); err != nil {
		return err
	}
	if err := e.saveState(s); err != nil {
		return e.failClosed(err)
	}
	return nil
}

// Restart consumes only the durable prepared identity, never caller authority.
func (e *Engine) Restart(ctx context.Context) (err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	s, err := e.ReadState()
	if err != nil {
		return err
	}
	if !s.Pending || s.Result != "prepared" {
		return errors.New("no prepared upgrade")
	}
	hash, err := FileHash(e.Target)
	if err != nil || hash != s.NewHash {
		return errors.New("prepared binary changed")
	}
	s.Prepared = false
	s.Result = "restarting"
	if err = e.fence(s); err != nil {
		return err
	}
	if err = e.saveState(s); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			recovery, cancel := context.WithTimeout(context.Background(), RecoveryTimeout)
			defer cancel()
			err = errors.Join(err, e.rollback(recovery, s))
		}
	}()
	if err = e.Control.Stop(ctx); err != nil {
		return err
	}
	if err = e.Control.Start(ctx); err != nil {
		return err
	}
	if err = e.Control.Healthy(ctx, s.NewHash); err != nil {
		return err
	}
	return e.finish(s, "installed")
}
