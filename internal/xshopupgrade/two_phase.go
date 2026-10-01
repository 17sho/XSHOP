package xshopupgrade

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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
	previousVersion := ""
	if s.Result == "installed" {
		previousVersion = s.Version
	} else if s.Result == "rolled_back" {
		previousVersion = s.PreviousVersion
	}
	s = State{PreviousVersion: previousVersion, HighWater: sequence, Pending: true, Rollback: rollback, OldHash: old, NewHash: next, Result: "preparing", Version: version, Digest: digest}
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

// pruneLocked runs only under the engine operation lock, after durable terminal
// acknowledgement and a fresh disk/running/health match. Unknown trees fail closed.
func (e *Engine) pruneLocked(ctx context.Context, s State) error {
	if s.Pending || (s.Result != "installed" && s.Result != "rolled_back") {
		return errors.New("cleanup terminal required")
	}
	if err := e.retained(s); err != nil {
		return err
	}
	expected := s.NewHash
	if s.Result == "rolled_back" {
		expected = s.OldHash
	}
	hash, err := FileHash(e.Target)
	if err != nil || hash != expected {
		return errors.New("cleanup current identity rejected")
	}
	if err = e.Control.Healthy(ctx, expected); err != nil {
		return err
	}
	entries, err := os.ReadDir(e.StateDir)
	if err != nil {
		return err
	}
	var remove []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".journal-") || strings.HasPrefix(entry.Name(), ".xshop-switch-") {
			path := filepath.Join(e.StateDir, entry.Name())
			if err = regular(path); err != nil {
				return err
			}
			if err = safeCleanupTree(path); err != nil {
				return err
			}
			remove = append(remove, path)
		}
	}
	seen := map[string]bool{}
	retainedDownloads := map[string]string{}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "download-") {
			continue
		}
		path := filepath.Join(e.StateDir, entry.Name())
		if err = safeCleanupTree(path); err != nil {
			return err
		}
		stage := filepath.Join(path, "stage", "dujiao-next")
		var hash string
		var identity retentionIdentity
		if _, statErr := os.Lstat(stage); statErr == nil {
			if err = regular(stage); err != nil {
				return err
			}
			hash, err = FileHash(stage)
			if err != nil {
				return err
			}
			// New downloads already bind their source bytes before preparation.
			if f, openErr := os.Open(filepath.Join(path, "retention.json")); openErr == nil {
				raw, readErr := io.ReadAll(io.LimitReader(f, 1025))
				f.Close()
				if readErr != nil || len(raw) > 1024 || json.Unmarshal(raw, &identity) != nil || identity.BinaryHash != hash {
					return errors.New("staged source retention identity mismatch")
				}
			} else if !errors.Is(openErr, os.ErrNotExist) {
				return openErr
			}
		} else if errors.Is(statErr, os.ErrNotExist) {
			f, openErr := os.Open(filepath.Join(path, "retention.json"))
			if openErr != nil {
				return errors.New("unbound legacy source; cleanup refused")
			}
			raw, readErr := io.ReadAll(io.LimitReader(f, 1025))
			f.Close()
			if readErr != nil || len(raw) > 1024 || json.Unmarshal(raw, &identity) != nil || !digestPattern.MatchString(identity.BinaryHash) {
				return errors.New("invalid source retention identity")
			}
			hash = identity.BinaryHash
		} else {
			return statErr
		}
		files, fileErr := retainedFiles(path)
		if fileErr != nil {
			return fileErr
		}
		if identity.BinaryHash != "" {
			if len(identity.Files) == 0 || len(identity.Files) != len(files) {
				return errors.New("retained source metadata changed or unbound")
			}
			for name, want := range identity.Files {
				if !digestPattern.MatchString(want) || files[name] != want {
					return errors.New("retained source metadata hash mismatch")
				}
			}
		}
		if (hash == s.NewHash || hash == s.OldHash) && !seen[hash] {
			seen[hash] = true
			retainedDownloads[path] = hash
		} else {
			remove = append(remove, path)
		}
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "checkpoint-") || entry.Name() == filepath.Base(filepath.Dir(s.Rollback)) {
			continue
		}
		path := filepath.Join(e.StateDir, entry.Name())
		if err = e.cleanupCheckpoint(path); err != nil {
			return err
		}
		remove = append(remove, path)
	}
	checkpoint := filepath.Dir(s.Rollback)
	if err = e.cleanupCheckpoint(checkpoint); err != nil {
		return err
	}
	for _, name := range []string{"config.yml", "database.backup", "database.sqlite3", "database.sqlite3-wal", "database.sqlite3-shm"} {
		path := filepath.Join(checkpoint, name)
		if info, statErr := os.Lstat(path); statErr == nil {
			if !info.Mode().IsRegular() {
				return errors.New("unknown checkpoint backup")
			}
			remove = append(remove, path)
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
	}
	// Durable source/binary linkage precedes removal of its legacy stage copy.
	for path, hash := range retainedDownloads {
		marker := &Engine{StateDir: path}
		files, fileErr := retainedFiles(path)
		if fileErr != nil {
			return fileErr
		}
		if err = marker.saveJSON("retention.json", retentionIdentity{BinaryHash: hash, Files: files}); err != nil {
			return err
		}
		stage := filepath.Join(path, "stage")
		if _, statErr := os.Lstat(stage); statErr == nil {
			remove = append(remove, stage)
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
	}
	root, err := os.OpenRoot(e.StateDir)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, path := range remove {
		if err = e.boundary("cleanup", "remove"); err != nil {
			return err
		}
		if err = root.RemoveAll(strings.TrimPrefix(path, e.StateDir+string(os.PathSeparator))); err != nil {
			return err
		}
		if err = syncDir(e.StateDir); err != nil {
			return err
		}
	}
	if filepath.Dir(e.Target) != e.StateDir {
		return e.pruneSwitchTemps(filepath.Dir(e.Target))
	}
	return nil
}

// Only installer-created switch temp basenames may be removed from Target's directory.
func (e *Engine) pruneSwitchTemps(dir string) error {
	if !filepath.IsAbs(dir) || dir == "/" {
		return errors.New("unsafe target directory")
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil || real != dir {
		return errors.New("target directory symlink")
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != 0 || info.Mode().Perm()&0022 != 0 {
		return errors.New("unsafe target directory ownership")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var names []string
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".xshop-switch-") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if err = regular(path); err != nil {
			return err
		}
		if err = safeCleanupTree(path); err != nil {
			return err
		}
		names = append(names, entry.Name())
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, name := range names {
		if err = root.Remove(name); err != nil {
			return err
		}
	}
	return syncDir(dir)
}

type retentionIdentity struct {
	BinaryHash string            `json:"new_hash"`
	Files      map[string]string `json:"files,omitempty"`
}

func retainedFiles(path string) (map[string]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, entry := range entries {
		name := entry.Name()
		if name == "stage" || name == "retention.json" {
			continue
		}
		if !ValidAsset(name) || (name != "manifest.json" && name != "manifest.sig" && !strings.HasSuffix(name, ".tar.gz")) {
			return nil, errors.New("unknown retained metadata")
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() > 64<<20 || len(out) >= 8 {
			return nil, errors.New("source metadata bounds")
		}
		full := filepath.Join(path, name)
		hash, err := FileHash(full)
		if err != nil {
			return nil, err
		}
		out[name] = hash
		f, err := os.Open(full)
		if err != nil {
			return nil, err
		}
		err = f.Sync()
		f.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Checkpoint provenance is an exact flat artifact set, not merely root ownership.
func (e *Engine) cleanupCheckpoint(path string) error {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() {
		return errors.New("checkpoint directory required")
	}
	if err = safeCleanupTree(path); err != nil {
		return err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		switch entry.Name() {
		case "previous-binary", "config.yml", "database.backup", "database.sqlite3", "database.sqlite3-wal", "database.sqlite3-shm":
		default:
			return errors.New("unknown checkpoint artifact")
		}
		full := filepath.Join(path, entry.Name())
		info, err := os.Lstat(full)
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("checkpoint artifact type rejected")
		}
		// No alias of target, journals, config or database may be deleted. Reject
		// all hardlinks so opaque Backup policy identities need not be read here.
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || st.Nlink != 1 || full == filepath.Clean(e.Target) {
			return errors.New("protected checkpoint identity")
		}
	}
	return nil
}

func safeCleanupTree(path string) error {
	return filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || st.Uid != 0 || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0022 != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return errors.New("unsafe cleanup tree")
		}
		return nil
	})
}

// retained validates a fixed private checkpoint, never a caller path.
func (e *Engine) retained(s State) error {
	root := filepath.Clean(e.StateDir)
	dir := filepath.Dir(s.Rollback)
	if root == "/" || !filepath.IsAbs(root) || filepath.Dir(dir) != root || !strings.HasPrefix(filepath.Base(dir), "checkpoint-") || filepath.Base(s.Rollback) != "previous-binary" {
		return errors.New("rollback path rejected")
	}
	for _, p := range []string{root, dir, s.Rollback} {
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || st.Uid != 0 || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0022 != 0 {
			return errors.New("rollback ownership rejected")
		}
		real, err := filepath.EvalSymlinks(p)
		if err != nil || real != p {
			return errors.New("rollback symlink rejected")
		}
	}
	if err := regular(s.Rollback); err != nil {
		return err
	}
	hash, err := FileHash(s.Rollback)
	if err != nil || hash != s.OldHash {
		return errors.New("rollback identity rejected")
	}
	return nil
}

// Rollback is explicit binary-only recovery; high water is never reset.
func (e *Engine) Rollback(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	s, err := e.ReadState()
	if err != nil {
		return err
	}
	if s.Pending || s.Result != "installed" {
		return errors.New("rollback unavailable")
	}
	if err = e.retained(s); err != nil {
		return err
	}
	hash, err := FileHash(e.Target)
	if err != nil || hash != s.NewHash {
		return errors.New("current identity changed")
	}
	bounded, cancel := context.WithTimeout(ctx, RecoveryTimeout)
	defer cancel()
	if err = e.Control.Healthy(bounded, s.NewHash); err != nil {
		return err
	}
	return e.rollback(bounded, s)
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
