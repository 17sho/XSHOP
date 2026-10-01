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
	HighWater uint64 `json:"high_water"`
	Pending   bool   `json:"pending"`
	Rollback  string `json:"rollback,omitempty"`
	OldHash   string `json:"old_hash,omitempty"`
	Result    string `json:"result"`
}
type Engine struct {
	Target, StateDir string
	Control          Controller
	Backup           func(context.Context, string) error
	mu               sync.Mutex
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
func (e *Engine) saveState(s State) error {
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
	if _, err = f.Write(raw); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), filepath.Join(e.StateDir, "state.json")); err != nil {
		return err
	}
	return syncDir(e.StateDir)
}
func (e *Engine) ReadState() (State, error) {
	var s State
	raw, err := os.ReadFile(filepath.Join(e.StateDir, "state.json"))
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(raw, &s)
	if err == nil && s.HighWater == 0 {
		err = errors.New("invalid high water")
	}
	return s, err
}

// Initialize is a one-time trusted bootstrap, never an HTTP operation.
func (e *Engine) Initialize(sequence uint64) error {
	if sequence == 0 {
		return errors.New("zero sequence")
	}
	if _, err := os.Lstat(filepath.Join(e.StateDir, "state.json")); !errors.Is(err, os.ErrNotExist) {
		return errors.New("state exists or cannot be inspected")
	}
	return e.saveState(State{HighWater: sequence, Result: "idle"})
}
func (e *Engine) rollback(ctx context.Context, s State) error { // Never rewind database/config/uploads after a writer-start attempt.
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
	s.Pending = false
	s.Result = "rolled_back"
	return e.saveState(s)
}
func (e *Engine) Recover(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	s, err := e.ReadState()
	if err != nil {
		return err
	}
	if !s.Pending {
		return nil
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
	s = State{HighWater: sequence, Pending: true, Rollback: filepath.Join(dir, "previous-binary"), OldHash: old, Result: "installing"}
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
	s.Pending = false
	s.Result = "installed"
	return e.saveState(s)
}
