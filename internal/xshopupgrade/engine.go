// Package xshopupgrade installs only preverified, schema-compatible preview binaries.
package xshopupgrade

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const RecoveryTimeout = 120 * time.Second

type Controller interface {
	Stop(context.Context) error
	Start(context.Context) error
	Healthy(context.Context, string) error
}
type State struct {
	PreviousVersion string `json:"previous_version,omitempty"`
	Version         string `json:"version,omitempty"`
	Digest          string `json:"digest,omitempty"`
	Prepared        bool   `json:"prepared,omitempty"`
	HighWater       uint64 `json:"high_water"`
	Pending         bool   `json:"pending"`
	Rollback        string `json:"rollback,omitempty"`
	OldHash         string `json:"old_hash,omitempty"`
	NewHash         string `json:"new_hash,omitempty"`
	Result          string `json:"result"`
}
type Engine struct {
	Target, StateDir string
	Control          Controller
	Backup           func(context.Context, string) error
	mu               sync.Mutex
	journalMu        sync.RWMutex
	journalErr       error
	terminalVerified uint64
	// Test-only fault seam at real persistence boundaries; nil in production.
	journalFault func(name, boundary string) error
}

func FileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	_, err = io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), err
}
func regular(path string) error {
	s, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !s.Mode().IsRegular() {
		return errors.New("regular file required")
	}
	return nil
}
func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
func atomicCopy(src, dest string, mode os.FileMode) error {
	if err := regular(src); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.CreateTemp(filepath.Dir(dest), ".xshop-switch-")
	if err != nil {
		return err
	}
	defer os.Remove(out.Name())
	defer out.Close()
	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	if err = out.Chmod(mode); err != nil {
		return err
	}
	if err = out.Sync(); err != nil {
		return err
	}
	if err = out.Close(); err != nil {
		return err
	}
	if err = os.Rename(out.Name(), dest); err != nil {
		return err
	}
	return syncDir(filepath.Dir(dest))
}

const recoveryJournal = "recovery.json"

func (e *Engine) boundary(name, stage string) error {
	if e.journalFault != nil {
		return e.journalFault(name, stage)
	}
	return nil
}
func (e *Engine) failClosed(err error) error {
	e.journalMu.Lock()
	e.journalErr = errors.Join(e.journalErr, err)
	e.journalMu.Unlock()
	return err
}
func (e *Engine) saveState(s State) error                { return e.saveJournal("state.json", s) }
func (e *Engine) saveJournal(name string, s State) error { return e.saveJSON(name, s) }
func (e *Engine) saveJSON(name string, s any) error {
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(e.StateDir, ".journal-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = e.boundary(name, "write"); err != nil {
		return err
	}
	if _, err = f.Write(raw); err != nil {
		return err
	}
	if err = e.boundary(name, "file_sync"); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = e.boundary(name, "rename"); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), filepath.Join(e.StateDir, name)); err != nil {
		return err
	}
	if err = e.boundary(name, "dir_sync"); err != nil {
		return err
	}
	return syncDir(e.StateDir)
}
func (e *Engine) ReadState() (State, error) {
	s, err := e.readState()
	if err != nil {
		return s, err
	}
	e.journalMu.RLock()
	verified := e.terminalVerified == s.HighWater
	e.journalMu.RUnlock()
	if !s.Pending && s.Result != "idle" && !verified {
		s.Pending = true
		s.Result = "recovery_required"
	}
	return s, nil
}
func (e *Engine) verified(sequence uint64) {
	e.journalMu.Lock()
	e.terminalVerified = sequence
	e.journalMu.Unlock()
}
func (e *Engine) readState() (State, error) {
	e.journalMu.RLock()
	blocked := e.journalErr
	e.journalMu.RUnlock()
	if blocked != nil {
		return State{}, fmt.Errorf("journal durability unknown: %w", blocked)
	}
	var s State
	raw, err := os.ReadFile(filepath.Join(e.StateDir, "state.json"))
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(raw, &s)
	if err == nil && s.HighWater == 0 {
		err = errors.New("invalid high water")
	}
	if err != nil {
		return s, err
	}
	// The separately durable intent outlives any uncertain terminal rename.
	raw, fenceErr := os.ReadFile(filepath.Join(e.StateDir, recoveryJournal))
	if fenceErr == nil {
		var intent State
		if err = json.Unmarshal(raw, &intent); err != nil {
			return s, err
		}
		if intent.HighWater == 0 || !intent.Pending || intent.Rollback == "" || intent.OldHash == "" {
			return s, errors.New("invalid recovery fence")
		}
		if intent.HighWater < s.HighWater {
			return s, errors.New("stale recovery fence")
		}
		// The state label is the acknowledgement boundary. An interrupted
		// preparing/restarting write still uses the conservative recovery fence.
		if s.Result == "prepared" {
			if !s.Prepared || !intent.Prepared || !s.Pending || !intent.Pending ||
				s.HighWater != intent.HighWater || s.Version != intent.Version ||
				s.Digest != intent.Digest || s.Rollback != intent.Rollback ||
				s.OldHash != intent.OldHash || s.NewHash != intent.NewHash ||
				s.NewHash == "" || intent.Result != "recovery_required" {
				return s, errors.New("inconsistent prepared journals")
			}
			intent.Result = "prepared"
		} else {
			intent.Result = "recovery_required"
		}
		intent.Pending = true
		return intent, nil
	}
	if !errors.Is(fenceErr, os.ErrNotExist) {
		return s, fenceErr
	}
	if s.Prepared || s.Result == "prepared" {
		return s, errors.New("prepared recovery fence missing")
	}
	// Legacy installed journals do not contain the installed identity. Recover
	// them conservatively via the retained old binary, never trust the label.
	if !s.Pending && s.Result == "installed" && s.NewHash == "" {
		s.Pending = true
		s.Result = "recovery_required"
	}
	if !s.Pending && s.Result != "idle" && s.Result != "installed" && s.Result != "rolled_back" {
		return s, errors.New("unknown journal terminal state")
	}
	return s, nil
}

func (e *Engine) fence(s State) error {
	s.Pending = true
	s.Result = "recovery_required"
	if err := e.saveJournal(recoveryJournal, s); err != nil {
		return e.failClosed(err)
	}
	return nil
}

// A terminal result is exposed only after its state is durable. If fence
// removal durability is unknown, re-establish it before returning failure.
func (e *Engine) finish(s State, result string) error {
	terminal := s
	terminal.Pending = false
	terminal.Result = result
	if err := e.saveState(terminal); err != nil {
		return err
	}
	err := e.boundary(recoveryJournal, "remove")
	if err == nil {
		err = os.Remove(filepath.Join(e.StateDir, recoveryJournal))
	}
	if err == nil {
		err = e.boundary(recoveryJournal, "remove_sync")
	}
	if err == nil {
		err = syncDir(e.StateDir)
	}
	if err != nil {
		return errors.Join(err, e.fence(s))
	}
	e.verified(s.HighWater)
	// Cleanup failure must not turn a verified install into an automatic rollback.
	ctx, cancel := context.WithTimeout(context.Background(), RecoveryTimeout)
	defer cancel()
	_ = e.pruneLocked(ctx, terminal)
	return nil
}

// Initialize is a one-time trusted bootstrap, never an HTTP operation.
func (e *Engine) Initialize(sequence uint64) error {
	if sequence == 0 {
		return errors.New("zero sequence")
	}
	for _, name := range []string{"state.json", recoveryJournal} {
		if _, err := os.Lstat(filepath.Join(e.StateDir, name)); !errors.Is(err, os.ErrNotExist) {
			return errors.New("state or recovery fence exists or cannot be inspected")
		}
	}
	if err := e.saveState(State{HighWater: sequence, Result: "idle"}); err != nil {
		return e.failClosed(err)
	}
	return nil
}
func (e *Engine) rollback(ctx context.Context, s State) error { // Never rewind database/config/uploads after a writer-start attempt.
	if err := e.fence(s); err != nil {
		return fmt.Errorf("rollback fence: %w", err)
	}
	if err := e.Control.Stop(ctx); err != nil {
		return fmt.Errorf("rollback stop: %w", err)
	}
	if err := regular(s.Rollback); err != nil {
		return err
	}
	h, err := FileHash(s.Rollback)
	if err != nil || h != s.OldHash {
		return errors.New("rollback hash mismatch")
	}
	if err = atomicCopy(s.Rollback, e.Target, 0755); err != nil {
		return err
	}
	if err = e.Control.Start(ctx); err != nil {
		return err
	}
	if err = e.Control.Healthy(ctx, s.OldHash); err != nil {
		return err
	}
	return e.finish(s, "rolled_back")
}
func (e *Engine) Recover(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	s, err := e.readState()
	if err != nil {
		return err
	}
	if s.Pending && s.Result == "prepared" {
		hash, err := FileHash(e.Target)
		if err != nil || hash != s.NewHash {
			return errors.New("prepared binary identity mismatch")
		}
		if err := e.boundary("state.json", "recover_sync"); err != nil {
			return e.failClosed(err)
		}
		if err := syncDir(e.StateDir); err != nil {
			return e.failClosed(err)
		}
		return nil // Explicit restart remains required after helper restart.
	}
	if !s.Pending {
		var expected string
		switch s.Result {
		case "idle":
			return nil
		case "installed":
			expected = s.NewHash
		case "rolled_back":
			expected = s.OldHash
		default:
			return errors.New("unknown journal terminal state")
		}
		if expected == "" {
			return errors.New("terminal identity unavailable; administrator recovery required")
		}
		hash, hashErr := FileHash(e.Target)
		if hashErr != nil || hash != expected {
			return errors.Join(errors.New("terminal binary identity mismatch"), hashErr, e.fence(s))
		}
		if err := e.Control.Healthy(ctx, expected); err != nil {
			return errors.Join(err, e.fence(s))
		}
		// Re-sync a visible terminal journal before treating it as durable on
		// restart (including an earlier uncertain fence-removal boundary).
		if err := e.boundary("state.json", "recover_sync"); err != nil {
			return e.failClosed(err)
		}
		if err := syncDir(e.StateDir); err != nil {
			return e.failClosed(err)
		}
		e.verified(s.HighWater)
		return e.pruneLocked(ctx, s)
	}
	return e.rollback(ctx, s)
}
func (e *Engine) Activate(ctx context.Context, candidate, expectedOld, expectedNew string, sequence uint64) (err error) {
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
	if err = atomicCopy(e.Target, filepath.Join(dir, "previous-binary"), 0755); err != nil {
		return err
	}
	s = State{HighWater: sequence, Pending: true, Rollback: filepath.Join(dir, "previous-binary"), OldHash: old, NewHash: next, Result: "installing"}
	if err = e.fence(s); err != nil {
		return err
	}
	if err = e.saveState(s); err != nil {
		return err
	}
	// Recovery gets an independent bounded context, not the failed operation's context.
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
	if err = e.Backup(ctx, dir); err != nil {
		return err
	}
	// Rehash immediately before switching; staging and its ancestors are root-owned.
	next, err = FileHash(candidate)
	if err != nil || next != expectedNew {
		return errors.New("staged candidate changed")
	}
	if err = atomicCopy(candidate, e.Target, 0755); err != nil {
		return err
	}
	if err = e.Control.Start(ctx); err != nil {
		return err
	}
	if err = e.Control.Healthy(ctx, next); err != nil {
		return err
	}
	return e.finish(s, "installed")
}
