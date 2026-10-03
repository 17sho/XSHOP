package xshopupgrade

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dujiao-next/internal/customupgrade"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Synthetic signed releases only; no service configuration or data reads.
type retentionSource struct{ raw, sig []byte }

func (s retentionSource) ProductionCandidates(ctx context.Context) ([]string, error) {
	tag, err := s.Latest(ctx)
	return []string{tag}, err
}
func (s retentionSource) Latest(context.Context) (string, error) { return "xshop-production-b", nil }
func (s retentionSource) Asset(_ context.Context, _, name string, _ uint64) ([]byte, error) {
	if name == "manifest.json" {
		return s.raw, nil
	}
	return s.sig, nil
}
func installedHelper(t *testing.T) (*Helper, *testController) {
	t.Helper()
	e, c, next := fixture(t)
	old, _ := FileHash(e.Target)
	n, _ := FileHash(next)
	key, priv, _ := ed25519.GenerateKey(rand.Reader)
	hash := strings.Repeat("a", 64)
	m := customupgrade.Manifest{SchemaVersion: 1, Product: "XSHOP", Sequence: 2, Version: "xshop-production-b", SourceCommit: strings.Repeat("a", 40), Channel: "stable", Profile: "embedded-production", OS: "linux", Arch: "amd64", MinimumUpdater: 1, FromBinarySHA256: []string{old}, MigrationPolicy: "unchanged", SchemaFingerprint: hash, Archive: customupgrade.Artifact{Name: "binary.tar.gz", Size: 100, SHA256: hash}, Source: customupgrade.Artifact{Name: "source.tar.gz", Size: 100, SHA256: hash}, Files: []customupgrade.File{{Path: "dujiao-next", Size: 1, SHA256: n, Mode: 0755}}}
	raw, _ := json.Marshal(m)
	sig := ed25519.Sign(priv, raw)
	h := &Helper{Engine: e, Source: retentionSource{raw, sig}, PublicKey: key, Schema: hash}
	h.check(context.Background())
	if h.status.State != "available" {
		t.Fatal(h.status)
	}
	digest := h.status.Digest
	if err := e.Prepare(context.Background(), next, old, n, 2, m.Version, digest); err != nil {
		t.Fatal(err)
	}
	if err := e.Restart(context.Background()); err != nil {
		t.Fatal(err)
	}
	return h, c
}
func TestRetentionInstalledLatestIsAuthenticatedUpToDate(t *testing.T) {
	h, _ := installedHelper(t)
	h.check(context.Background())
	if h.status.State != "up_to_date" {
		t.Fatalf("installed latest misclassified: %+v", h.status)
	}
}

func TestRetentionExplicitRollbackOnlyBinary(t *testing.T) {
	h, _ := installedHelper(t)
	s, _ := h.Engine.ReadState()
	db := filepath.Join(h.Engine.StateDir, "synthetic.db")
	os.WriteFile(db, []byte("new writes"), 0600)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/rollback", nil))
	if w.Code != 202 {
		t.Fatalf("rollback unavailable: %d", w.Code)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		h.mu.Lock()
		busy := h.busy
		h.mu.Unlock()
		if !busy {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("rollback timeout")
		}
		time.Sleep(time.Millisecond)
	}
	state, err := h.Engine.ReadState()
	got, _ := FileHash(h.Engine.Target)
	if err != nil || state.Pending || state.Result != "rolled_back" || state.HighWater != s.HighWater || got != s.OldHash {
		t.Fatal(state, err, got)
	}
	data, _ := os.ReadFile(db)
	if string(data) != "new writes" {
		t.Fatal("database rewound")
	}
	h.check(context.Background())
	if h.status.State == "available" || h.status.State == "up_to_date" {
		t.Fatal("consumed release authorized after rollback", h.status)
	}
}
func TestRetentionUnknownCheckpointPreflightPreservesEverything(t *testing.T) {
	for _, kind := range []string{"unknown", "nested", "retained", "wrong_type", "protected_target", "protected_hardlink"} {
		t.Run(kind, func(t *testing.T) {
			h, _ := installedHelper(t)
			e := h.Engine
			s, _ := e.ReadState()
			stale, err := os.MkdirTemp(e.StateDir, "checkpoint-")
			if err != nil {
				t.Fatal(err)
			}
			keep := filepath.Join(stale, "previous-binary")
			if err = os.WriteFile(keep, []byte("stale"), 0755); err != nil {
				t.Fatal(err)
			}
			bad := stale
			if kind == "retained" {
				bad = filepath.Dir(s.Rollback)
			}
			sentinel := filepath.Join(bad, "operator-data-not-an-updater-artifact")
			switch kind {
			case "nested":
				if err = os.Mkdir(sentinel, 0700); err == nil {
					err = os.WriteFile(filepath.Join(sentinel, "data"), []byte("keep"), 0600)
				}
			case "wrong_type":
				sentinel = filepath.Join(bad, "database.sqlite3")
				err = os.Mkdir(sentinel, 0700)
			case "protected_target":
				sentinel = filepath.Join(bad, "database.sqlite3")
				err = os.Rename(e.Target, sentinel)
				e.Target = sentinel
			case "protected_hardlink":
				sentinel = filepath.Join(bad, "database.sqlite3")
				err = os.Link(e.Target, sentinel)
			default:
				err = os.WriteFile(sentinel, []byte("keep"), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = e.Recover(context.Background()); err == nil {
				t.Fatal("unknown/protected checkpoint accepted")
			}
			for _, path := range []string{keep, sentinel, s.Rollback} {
				if _, err = os.Lstat(path); err != nil {
					t.Fatal("preflight partially deleted", path, err)
				}
			}
		})
	}
}

func TestRetentionLegacyCheckpointArtifactMigration(t *testing.T) {
	h, _ := installedHelper(t)
	e := h.Engine
	s, _ := e.ReadState()
	stale, err := os.MkdirTemp(e.StateDir, "checkpoint-")
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{stale, filepath.Dir(s.Rollback)} {
		for _, name := range []string{"config.yml", "database.backup", "database.sqlite3", "database.sqlite3-wal", "database.sqlite3-shm"} {
			if err = os.WriteFile(filepath.Join(dir, name), []byte("legacy"), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = e.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("obsolete known checkpoint retained", err)
	}
	entries, err := os.ReadDir(filepath.Dir(s.Rollback))
	if err != nil || len(entries) != 1 || entries[0].Name() != "previous-binary" {
		t.Fatal("checkpoint not exact binary-only", entries, err)
	}
}

func TestRetentionOneCheckpointAfterVerifiedTerminal(t *testing.T) {
	h, _ := installedHelper(t)
	e := h.Engine
	oldState, _ := e.ReadState()
	for i := 0; i < 3; i++ {
		dir, _ := os.MkdirTemp(e.StateDir, "checkpoint-")
		os.WriteFile(filepath.Join(dir, "previous-binary"), []byte("stale"), 0755)
	}
	// Recovery is the natural retry point for interrupted terminal cleanup.
	if err := e.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(e.StateDir)
	count := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "checkpoint-") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("want exactly one checkpoint, got %d", count)
	}
	if _, err := os.Stat(oldState.Rollback); err != nil {
		t.Fatal("usable rollback removed", err)
	}
}
func TestRetentionLegacyDownloadsKeepOnlyCurrentAndPreviousSource(t *testing.T) {
	h, _ := installedHelper(t)
	e := h.Engine
	s, _ := e.ReadState()
	for _, content := range []string{"A", "B", "obsolete"} {
		dir, _ := os.MkdirTemp(e.StateDir, "download-")
		os.Mkdir(filepath.Join(dir, "stage"), 0700)
		os.WriteFile(filepath.Join(dir, "stage", "dujiao-next"), []byte(content), 0755)
		os.WriteFile(filepath.Join(dir, "source.tar.gz"), []byte("source-"+content), 0600)
	}
	if err := e.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(e.StateDir)
	count := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "download-") {
			count++
			path := filepath.Join(e.StateDir, entry.Name())
			if _, err := os.Stat(filepath.Join(path, "stage")); !os.IsNotExist(err) {
				t.Fatal("staged binary retained", err)
			}
			if _, err := os.Stat(filepath.Join(path, "source.tar.gz")); err != nil {
				t.Fatal("source lost", err)
			}
		}
	}
	if count != 2 {
		t.Fatalf("want current+previous source only, got %d", count)
	}
	if err := e.Recover(context.Background()); err != nil {
		t.Fatal("cleanup not idempotent", err)
	}
	after, _ := e.ReadState()
	if after.HighWater != s.HighWater {
		t.Fatal("highwater changed")
	}
}
func TestRetentionStatusProjectsSlotWithoutLegacyVersionGuess(t *testing.T) {
	h, _ := installedHelper(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/status", nil))
	var s Status
	json.Unmarshal(w.Body.Bytes(), &s)
	if s.CurrentVersion != "xshop-production-b" || !s.RollbackAvailable || s.PreviousVersion != "" {
		t.Fatal("status slot missing or legacy version invented", s)
	}
}
func TestRetentionCheckpointContainsOnlyRollbackBinary(t *testing.T) {
	h, _ := installedHelper(t)
	s, _ := h.Engine.ReadState()
	entries, _ := os.ReadDir(filepath.Dir(s.Rollback))
	if len(entries) != 1 || entries[0].Name() != "previous-binary" {
		t.Fatal("obsolete backup retained", entries)
	}
}
func TestRetentionInterruptedCleanupRetriesWithoutLosingSlot(t *testing.T) {
	h, _ := installedHelper(t)
	e := h.Engine
	s, _ := e.ReadState()
	for i := 0; i < 3; i++ {
		dir, _ := os.MkdirTemp(e.StateDir, "checkpoint-")
		os.WriteFile(filepath.Join(dir, "previous-binary"), []byte("stale"), 0755)
	}
	hits := 0
	e.journalFault = func(n, b string) error {
		if n == "cleanup" && b == "remove" {
			hits++
			if hits == 2 {
				return os.ErrPermission
			}
		}
		return nil
	}
	if err := e.Recover(context.Background()); err == nil {
		t.Fatal("cleanup interruption hidden")
	}
	if err := e.retained(s); err != nil {
		t.Fatal("rollback lost", err)
	}
	e.journalFault = nil
	if err := e.Recover(context.Background()); err != nil {
		t.Fatal("retry failed", err)
	}
	entries, _ := os.ReadDir(e.StateDir)
	count := 0
	for _, v := range entries {
		if strings.HasPrefix(v.Name(), "checkpoint-") {
			count++
		}
	}
	if count != 1 {
		t.Fatal("retry left extra checkpoints", count)
	}
}
func TestRetentionRejectsUnknownSourceAndSymlinkBeforeDeletion(t *testing.T) {
	for _, kind := range []string{"source", "symlink", "pending"} {
		t.Run(kind, func(t *testing.T) {
			h, _ := installedHelper(t)
			e := h.Engine
			s, _ := e.ReadState()
			stale, _ := os.MkdirTemp(e.StateDir, "checkpoint-")
			os.WriteFile(filepath.Join(stale, "previous-binary"), []byte("stale"), 0755)
			switch kind {
			case "source":
				dir, _ := os.MkdirTemp(e.StateDir, "download-")
				os.WriteFile(filepath.Join(dir, "source.tar.gz"), []byte("unknown"), 0600)
			case "symlink":
				dir, _ := os.MkdirTemp(e.StateDir, "download-")
				os.Symlink("/", filepath.Join(dir, "escape"))
			case "pending":
				s.Pending = true
			}
			e.mu.Lock()
			err := e.pruneLocked(context.Background(), s)
			e.mu.Unlock()
			if err == nil {
				t.Fatal("unknown cleanup accepted")
			}
			if _, err := os.Stat(stale); err != nil {
				t.Fatal("preflight partially deleted", err)
			}
			if err := e.retained(s); err != nil {
				t.Fatal("retained binary lost", err)
			}
		})
	}
}
func TestRetentionInstalledCheckTamperAndRevokedKeyDenied(t *testing.T) {
	for _, kind := range []string{"signature", "key", "target", "digest", "version", "health"} {
		t.Run(kind, func(t *testing.T) {
			h, c := installedHelper(t)
			s, _ := h.Engine.ReadState()
			switch kind {
			case "signature":
				src := h.Source.(retentionSource)
				src.sig[0] ^= 1
				h.Source = src
			case "key":
				key, _, _ := ed25519.GenerateKey(rand.Reader)
				h.PublicKey = key
			case "target":
				os.WriteFile(h.Engine.Target, []byte("tamper"), 0755)
			case "digest":
				s.Digest = strings.Repeat("b", 64)
				h.Engine.saveState(s)
			case "version":
				s.Version = "xshop-production-other"
				h.Engine.saveState(s)
			case "health":
				c.failFirst = true
				c.started = 1
			}
			h.check(context.Background())
			if h.status.State == "up_to_date" || h.status.State == "available" {
				t.Fatal("unbound identity accepted", kind, h.status)
			}
		})
	}
}

type retentionFaultController struct {
	base                         Controller
	stopErr, startErr, healthErr error
	healthAfterStop              bool
	stops                        int
}

func (c *retentionFaultController) Stop(ctx context.Context) error {
	c.stops++
	if c.stopErr != nil {
		return c.stopErr
	}
	return c.base.Stop(ctx)
}
func (c *retentionFaultController) Start(ctx context.Context) error {
	if c.startErr != nil {
		return c.startErr
	}
	return c.base.Start(ctx)
}
func (c *retentionFaultController) Healthy(ctx context.Context, h string) error {
	if c.healthErr != nil && (!c.healthAfterStop || c.stops > 0) {
		return c.healthErr
	}
	return c.base.Healthy(ctx, h)
}
func TestRetentionManualRollbackFailuresFenceAndRecover(t *testing.T) {
	for _, phase := range []string{"fence", "stop", "start", "health", "health_running", "tamper", "escape"} {
		t.Run(phase, func(t *testing.T) {
			h, _ := installedHelper(t)
			e := h.Engine
			s, _ := e.ReadState()
			ctl := &retentionFaultController{base: e.Control}
			e.Control = ctl
			switch phase {
			case "fence":
				e.journalFault = func(n, b string) error {
					if n == recoveryJournal && b == "write" {
						return os.ErrPermission
					}
					return nil
				}
			case "stop":
				ctl.stopErr = os.ErrPermission
			case "start":
				ctl.startErr = os.ErrPermission
			case "health":
				ctl.healthErr = os.ErrPermission
			case "health_running":
				ctl.healthErr = os.ErrPermission
				ctl.healthAfterStop = true
			case "tamper":
				os.WriteFile(s.Rollback, []byte("tampered"), 0755)
			case "escape":
				s.Rollback = filepath.Join(e.StateDir, "..", "outside")
				e.saveState(s)
			}
			if err := e.Rollback(context.Background()); err == nil {
				t.Fatal("failure accepted", phase)
			}
			if (phase == "fence" || phase == "health" || phase == "tamper" || phase == "escape") && ctl.stops != 0 {
				t.Fatal("unsafe stop", phase, ctl.stops)
			}
			if phase == "stop" || phase == "start" || phase == "health_running" {
				state, err := e.ReadState()
				if err != nil || !state.Pending || state.HighWater != s.HighWater {
					t.Fatal("recovery fence lost", state, err)
				}
				ctl.stopErr = nil
				ctl.startErr = nil
				ctl.healthErr = nil
				if err = e.Recover(context.Background()); err != nil {
					t.Fatal("recovery failed", err)
				}
			}
		})
	}
}
func TestRetentionStageLessRequiresExactFileBindings(t *testing.T) {
	for _, kind := range []string{"missing", "empty", "omitted", "extra", "malformed", "empty_hash", "missing_file", "no_files"} {
		t.Run(kind, func(t *testing.T) {
			h, _ := installedHelper(t)
			e := h.Engine
			s, _ := e.ReadState()
			dir, err := os.MkdirTemp(e.StateDir, "download-")
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"source.tar.gz", "manifest.json"} {
				if err = os.WriteFile(filepath.Join(dir, name), []byte(name), 0600); err != nil {
					t.Fatal(err)
				}
			}
			files, err := retainedFiles(dir)
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "missing":
				files = nil
			case "empty":
				files = map[string]string{}
			case "omitted":
				delete(files, "source.tar.gz")
			case "extra":
				files["other.tar.gz"] = strings.Repeat("a", 64)
			case "malformed":
				files["source.tar.gz"] = "not-a-hash"
			case "empty_hash":
				files["source.tar.gz"] = ""
			case "missing_file":
				err = os.Remove(filepath.Join(dir, "source.tar.gz"))
			case "no_files":
				for name := range files {
					if err = os.Remove(filepath.Join(dir, name)); err != nil {
						t.Fatal(err)
					}
				}
				files = nil
			}
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(retentionIdentity{BinaryHash: s.NewHash, Files: files})
			if err != nil {
				t.Fatal(err)
			}
			if kind == "empty" {
				raw = []byte(fmt.Sprintf(`{"new_hash":%q,"files":{}}`, s.NewHash))
			}
			marker := filepath.Join(dir, "retention.json")
			if err = os.WriteFile(marker, raw, 0600); err != nil {
				t.Fatal(err)
			}
			stale, err := os.MkdirTemp(e.StateDir, "checkpoint-")
			if err != nil {
				t.Fatal(err)
			}
			if err = e.Recover(context.Background()); err == nil {
				t.Fatal("unbound stage-less source accepted")
			}
			after, err := os.ReadFile(marker)
			if err != nil || string(after) != string(raw) {
				t.Fatal("rejected marker rewritten", err)
			}
			if _, err = os.Stat(stale); err != nil {
				t.Fatal("partial cleanup", err)
			}
		})
	}
}

func TestRetentionSourceMetadataTamperFailsClosed(t *testing.T) {
	h, _ := installedHelper(t)
	e := h.Engine
	dir, _ := os.MkdirTemp(e.StateDir, "download-")
	os.Mkdir(filepath.Join(dir, "stage"), 0700)
	os.WriteFile(filepath.Join(dir, "stage", "dujiao-next"), []byte("B"), 0755)
	source := filepath.Join(dir, "source.tar.gz")
	os.WriteFile(source, []byte("verified-source"), 0600)
	if err := e.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(source, []byte("tampered-source"), 0600)
	stale, _ := os.MkdirTemp(e.StateDir, "checkpoint-")
	os.WriteFile(filepath.Join(stale, "previous-binary"), []byte("stale"), 0755)
	if err := e.Recover(context.Background()); err == nil {
		t.Fatal("retained source tamper accepted")
	}
	if _, err := os.Stat(stale); err != nil {
		t.Fatal("tamper cleanup did not fail closed", err)
	}
}
func TestRetentionRollbackStatusSurvivesHelperRestart(t *testing.T) {
	h, _ := installedHelper(t)
	if err := h.Engine.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	fresh := &Helper{Engine: h.Engine}
	w := httptest.NewRecorder()
	fresh.ServeHTTP(w, httptest.NewRequest("GET", "/status", nil))
	var s Status
	json.Unmarshal(w.Body.Bytes(), &s)
	if s.State != "rolled_back" || s.RollbackAvailable {
		t.Fatal("rollback terminal misprojected", s)
	}
}
func TestRetentionTerminalRemovesOnlyKnownOrphanTemps(t *testing.T) {
	h, _ := installedHelper(t)
	e := h.Engine
	for _, name := range []string{".journal-orphan", ".xshop-switch-orphan", "config.yml", "uploads", "highwater.keep"} {
		os.WriteFile(filepath.Join(e.StateDir, name), []byte("keep-or-orphan"), 0600)
	}
	if err := e.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".journal-orphan", ".xshop-switch-orphan"} {
		if _, err := os.Stat(filepath.Join(e.StateDir, name)); !os.IsNotExist(err) {
			t.Fatal("orphan retained", name, err)
		}
	}
	for _, name := range []string{"config.yml", "uploads", "highwater.keep", "state.json"} {
		if _, err := os.Stat(filepath.Join(e.StateDir, name)); err != nil {
			t.Fatal("non-temp touched", name, err)
		}
	}
}
func TestRetentionOrphanSwitchInTargetDirectoryRemoved(t *testing.T) {
	h, _ := installedHelper(t)
	e := h.Engine
	dir := t.TempDir()
	target := filepath.Join(dir, "app")
	if err := os.Rename(e.Target, target); err != nil {
		t.Fatal(err)
	}
	e.Target = target
	orphan := filepath.Join(dir, ".xshop-switch-orphan")
	os.WriteFile(orphan, []byte("orphan"), 0600)
	if err := e.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatal("target orphan retained", err)
	}
}
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
	if err := e.Prepare(context.Background(), next, old, candidate, 2, "xshop-production-b", "manifest-digest"); err != nil {
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
