package xshopupgrade

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dujiao-next/internal/customupgrade"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Disposable units only: never invoke the fixed preview service or helper socket.
type retentionSystemdController struct{ unit, target, db string }

func (c *retentionSystemdController) Stop(ctx context.Context) error {
	return exec.CommandContext(ctx, "systemctl", "stop", c.unit).Run()
}
func (c *retentionSystemdController) Start(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "systemd-run", "--quiet", "--collect", "--unit="+c.unit, "--service-type=simple", "--property=NoNewPrivileges=yes", "--property=CapabilityBoundingSet=", "--property=ProtectSystem=strict", "--property=PrivateTmp=yes", "--property=PrivateNetwork=yes", "--property=RestrictAddressFamilies=AF_UNIX", "--property=MemoryDenyWriteExecute=yes", c.target, "60")
	if raw, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("disposable systemd start: %w: %s", err, raw)
	}
	return exec.CommandContext(ctx, "/usr/bin/python3", "-c", `import sqlite3,sys;c=sqlite3.connect(sys.argv[1]);c.execute("insert into transactions(value) values('writer-start')");c.commit();c.close()`, c.db).Run()
}
func (c *retentionSystemdController) Healthy(ctx context.Context, want string) error {
	for {
		raw, err := exec.CommandContext(ctx, "systemctl", "show", c.unit, "--property=MainPID", "--value").Output()
		if err == nil {
			pid, e := strconv.Atoi(strings.TrimSpace(string(raw)))
			if e == nil && pid > 0 {
				hash, e := FileHash(fmt.Sprintf("/proc/%d/exe", pid))
				if e == nil && hash == want {
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}
func TestRetentionSignedDisposableSystemdManualRollback(t *testing.T) {
	if os.Getenv("XSHOP_RETENTION_SYSTEMD") != "1" {
		t.Skip("explicit disposable-systemd opt in required")
	}
	root := t.TempDir()
	target := filepath.Join(root, "app")
	db := filepath.Join(root, "copied-synthetic.db")
	state := filepath.Join(root, "state")
	os.Mkdir(state, 0700)
	raw, err := os.ReadFile("/usr/bin/sleep")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(target, raw, 0755)
	next := append(append([]byte(nil), raw...), []byte("synthetic-B")...)
	originalDB := filepath.Join(root, "synthetic-original.db")
	python(t, "-c", `import sqlite3,sys;c=sqlite3.connect(sys.argv[1]);c.execute("create table transactions(id integer primary key,value text)");c.execute("insert into transactions(value) values('original')");c.commit();c.close()`, originalDB)
	if err = atomicCopy(originalDB, db, 0600); err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, 8)
	rand.Read(nonce)
	ctl := &retentionSystemdController{unit: "xshop-retention-fixture-" + hex.EncodeToString(nonce) + ".service", target: target, db: db}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	defer ctl.Stop(context.Background())
	if err = ctl.Start(ctx); err != nil {
		t.Fatal(err)
	}
	old, _ := FileHash(target)
	sha := func(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
	newHash := sha(next)
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "dujiao-next", Size: int64(len(next)), Mode: 0755, Typeflag: tar.TypeReg})
	tw.Write(next)
	tw.Close()
	gz.Close()
	// The source archive is a synthetic test asset, not a published GPL claim.
	source := []byte("synthetic corresponding-source fixture")
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	schema := strings.Repeat("a", 64)
	m := customupgrade.Manifest{SchemaVersion: 1, Product: "XSHOP", Sequence: 2, Version: "xshop-production-systemd-fixture", SourceCommit: strings.Repeat("a", 40), Channel: "stable", Profile: "embedded-production", OS: "linux", Arch: "amd64", MinimumUpdater: 1, FromBinarySHA256: []string{old}, MigrationPolicy: "unchanged", SchemaFingerprint: schema, Archive: customupgrade.Artifact{Name: "binary.tar.gz", Size: uint64(archive.Len()), SHA256: sha(archive.Bytes())}, Source: customupgrade.Artifact{Name: "source.tar.gz", Size: uint64(len(source)), SHA256: sha(source)}, Files: []customupgrade.File{{Path: "dujiao-next", Size: uint64(len(next)), SHA256: newHash, Mode: 0755}}}
	assets := filepath.Join(root, "assets")
	os.Mkdir(assets, 0700)
	manifest, _ := json.Marshal(m)
	for name, data := range map[string][]byte{"manifest.json": manifest, "manifest.sig": ed25519.Sign(priv, manifest), "source.tar.gz": source, "binary.tar.gz": archive.Bytes()} {
		os.WriteFile(filepath.Join(assets, name), data, 0600)
	}
	e := &Engine{Target: target, StateDir: state, Control: ctl, Backup: func(_ context.Context, dir string) error {
		return atomicCopy(db, filepath.Join(dir, "database.sqlite3"), 0600)
	}}
	if err = e.Initialize(1); err != nil {
		t.Fatal(err)
	}
	h := &Helper{Engine: e, Source: localSource{assets}, PublicKey: pub, Schema: schema}
	h.check(ctx)
	if h.status.State != "available" {
		t.Fatal(h.status)
	}
	h.install(ctx, m.Version, h.status.Digest)
	if h.status.State != "prepared" {
		t.Fatal(h.status)
	}
	if err = ctl.Healthy(ctx, old); err != nil {
		t.Fatal("prepare lost old inode", err)
	}
	if err = e.Restart(ctx); err != nil {
		t.Fatal(err)
	}
	h.check(ctx)
	if h.status.State != "up_to_date" {
		t.Fatal(h.status)
	}
	before := python(t, "-c", dbDigest, db)
	if err = e.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err = ctl.Healthy(ctx, old); err != nil {
		t.Fatal(err)
	}
	after := python(t, "-c", dbDigest, db)
	if bytes.Equal(before, after) {
		t.Fatal("synthetic writer start not retained")
	}
	count := strings.TrimSpace(string(python(t, "-c", `import sqlite3,sys;c=sqlite3.connect(sys.argv[1]);print(c.execute("select count(*) from transactions").fetchone()[0])`, db)))
	if count != "4" {
		t.Fatal("synthetic database rewind", count)
	}
	s, err := e.ReadState()
	if err != nil || s.Pending || s.Result != "rolled_back" || s.HighWater != 2 {
		t.Fatal(s, err)
	}
	h.check(ctx)
	if h.status.State == "up_to_date" || h.status.State == "available" {
		t.Fatal("consumed release permitted", h.status)
	}
	t.Logf("disposable signed systemd A->prepare B->restart B->rollback A PASS A=%s B=%s highwater=%d synthetic_rows=%s unit=%s", old, newHash, s.HighWater, count, ctl.unit)
}

type localSource struct{ dir string }

func (s localSource) ProductionCandidates(ctx context.Context) ([]string, error) {
	tag, err := s.Latest(ctx)
	return []string{tag}, err
}
func (s localSource) Latest(context.Context) (string, error) {
	raw, err := os.ReadFile(filepath.Join(s.dir, "manifest.json"))
	if err != nil {
		return "", err
	}
	var m struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(raw, &m); err != nil || !ValidTag(m.Version) {
		return "", errors.New("local source identity")
	}
	return m.Version, nil
}
func (s localSource) Asset(_ context.Context, tag, name string, max uint64) ([]byte, error) {
	latest, err := s.Latest(context.Background())
	if err != nil || tag != latest || !ValidAsset(name) {
		return nil, errors.New("local source identity")
	}
	raw, err := os.ReadFile(filepath.Join(s.dir, name))
	if uint64(len(raw)) > max {
		return nil, errors.New("limit")
	}
	return raw, err
}

type processController struct {
	root, target, url string
	cmd               *exec.Cmd
	log               *os.File
	failNext          bool
	runningHash       string
	starts            int
}

func (c *processController) Stop(context.Context) error {
	if c.cmd != nil {
		if c.cmd.Process != nil {
			_ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGTERM)
		}
		done := make(chan error, 1)
		go func() { done <- c.cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGKILL)
			<-done
		}
		c.cmd = nil
	}
	return nil
}
func (c *processController) Start(context.Context) error {
	c.starts++
	c.cmd = exec.Command(c.target, "-mode", "api")
	c.cmd.Dir = c.root
	c.cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + c.root}
	c.cmd.Stdout = c.log
	c.cmd.Stderr = c.log
	c.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return c.cmd.Start()
}
func (c *processController) Healthy(ctx context.Context, hash string) error {
	if c.failNext {
		c.failNext = false
		return errors.New("injected unhealthy candidate")
	}
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	for {
		if c.cmd != nil && c.cmd.Process != nil {
			got, _ := FileHash("/proc/" + stringPID(c.cmd.Process.Pid) + "/exe")
			resp, err := http.Get(c.url + "/health")
			if err == nil {
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
				resp.Body.Close()
				if resp.StatusCode == 200 && strings.Contains(string(body), `"status":"ok"`) && got == hash {
					c.runningHash = hash
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return errors.New("actual binary unhealthy")
		case <-time.After(100 * time.Millisecond):
		}
	}
}
func stringPID(pid int) string { raw, _ := json.Marshal(pid); return string(raw) }
func python(t *testing.T, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("/usr/bin/python3", args...)
	raw, err := cmd.Output()
	if err != nil {
		t.Fatal("fixture python failed", err)
	}
	return raw
}

const dbDigest = `import sqlite3,sys,json,hashlib
c=sqlite3.connect('file:'+sys.argv[1]+'?mode=ro',uri=True)
out={}
for name, in c.execute("select name from sqlite_master where type='table' order by name"):
 rows=c.execute('select * from "'+name.replace('"','""')+'"').fetchall()
 serialized=sorted(json.dumps(r,default=lambda b:b.hex(),sort_keys=True) for r in rows)
 out[name]={'count':len(rows),'hash':hashlib.sha256(json.dumps(serialized).encode()).hexdigest()}
print(json.dumps(out,sort_keys=True))
`

// TestActualCandidateChain is opt-in and touches only a newly allocated directory.
// It runs the final A/B binaries, signed package verification, asynchronous HTTP
// helper protocol, data preservation and unhealthy-candidate executable rollback.
func TestActualCandidateChain(t *testing.T) {
	artifacts := os.Getenv("XSHOP_REHEARSAL_DIR")
	if artifacts == "" {
		t.Skip("run with final candidate artifacts")
	}
	root := t.TempDir()
	os.Chmod(root, 0700)
	for _, name := range []string{"db", "uploads", "logs", "state"} {
		os.Mkdir(filepath.Join(root, name), 0700)
	}
	target := filepath.Join(root, "dujiao-next")
	if err := atomicCopy(filepath.Join(artifacts, "A", "dujiao-next"), target, 0755); err != nil {
		t.Fatal(err)
	}
	aHash, _ := FileHash(target)
	bHash, _ := FileHash(filepath.Join(artifacts, "B", "dujiao-next"))
	manifestRaw, err := os.ReadFile(filepath.Join(artifacts, "B", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var release struct {
		Version  string `json:"version"`
		Sequence uint64 `json:"sequence"`
	}
	if json.Unmarshal(manifestRaw, &release) != nil || release.Sequence < 2 {
		t.Fatal("invalid rehearsal manifest")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	secret := strings.Repeat("a", 64)
	passwordBytes := make([]byte, 32)
	if _, err = rand.Read(passwordBytes); err != nil {
		t.Fatal(err)
	}
	fixturePassword := hex.EncodeToString(passwordBytes) + "A9!"
	cfg := map[string]any{"server": map[string]any{"host": "127.0.0.1", "port": port, "mode": "release"}, "database": map[string]any{"driver": "sqlite", "dsn": "./db/fixture.db"}, "app": map[string]any{"secret_key": secret}, "jwt": map[string]any{"secret": strings.Repeat("b", 64)}, "user_jwt": map[string]any{"secret": strings.Repeat("c", 64)}, "bootstrap": map[string]any{"default_admin_username": "upgrade-rehearsal-admin", "default_admin_password": fixturePassword}, "redis": map[string]any{"enabled": false}, "queue": map[string]any{"enabled": false}, "web": map[string]any{"admin_path": "/admin"}}
	cfgRaw, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(root, "config.yml"), cfgRaw, 0600)
	os.WriteFile(filepath.Join(root, "uploads", "synthetic.txt"), []byte("synthetic upload retained"), 0600)
	log, err := os.Create(filepath.Join(root, "process.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	if evidence := os.Getenv("XSHOP_REHEARSAL_EVIDENCE"); evidence != "" {
		defer func() {
			raw, _ := os.ReadFile(filepath.Join(root, "process.log"))
			_ = os.WriteFile(filepath.Join(evidence, "rehearsal-process.private.log"), raw, 0600)
		}()
	}
	ctl := &processController{root: root, target: target, url: "http://127.0.0.1:" + stringPID(port), log: log}
	defer ctl.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	if err = ctl.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err = ctl.Healthy(ctx, aHash); err != nil {
		t.Fatal(err)
	}
	// Real JWT middleware must reject anonymous requests to all registered actions.
	for _, x := range []struct{ m, p string }{{"GET", "status"}, {"POST", "check"}, {"POST", "install"}, {"POST", "restart"}} {
		req, _ := http.NewRequest(x.m, ctl.url+"/api/v1/admin/xshop-upgrade/"+x.p, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var envelope struct {
			Code int `json:"status_code"`
		}
		json.Unmarshal(raw, &envelope)
		if envelope.Code != 401 {
			t.Fatalf("real anonymous request accepted: %s %s", x.p, raw)
		}
	}
	db := filepath.Join(root, "db", "fixture.db")
	loginBody, _ := json.Marshal(map[string]string{"username": "upgrade-rehearsal-admin", "password": fixturePassword})
	loginResp, err := http.Post(ctl.url+"/api/v1/admin/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		t.Fatal(err)
	}
	var login struct {
		Code int `json:"status_code"`
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	json.NewDecoder(loginResp.Body).Decode(&login)
	loginResp.Body.Close()
	if login.Code != 0 || login.Data.Token == "" {
		t.Fatalf("synthetic admin login failed (code only): %d", login.Code)
	}
	authenticated := func() int {
		req, _ := http.NewRequest("GET", ctl.url+"/api/v1/admin/xshop-upgrade/status", nil)
		req.Header.Set("Authorization", "Bearer "+login.Data.Token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var envelope struct {
			Code int `json:"status_code"`
		}
		json.NewDecoder(resp.Body).Decode(&envelope)
		return envelope.Code
	}
	if authenticated() != 503 {
		t.Fatal("real super admin did not reach helper boundary")
	}
	python(t, "-c", `import sqlite3,sys;c=sqlite3.connect(sys.argv[1]);c.execute('update admins set is_super=0');c.commit()`, db)
	if authenticated() != 403 {
		t.Fatal("ordinary admin reached helper")
	}
	python(t, "-c", `import sqlite3,sys;c=sqlite3.connect(sys.argv[1]);c.execute('update admins set is_super=1,token_version=token_version+1');c.commit()`, db)
	if authenticated() != 401 {
		t.Fatal("revoked super token reached helper")
	}
	python(t, "-c", `import sqlite3,sys;c=sqlite3.connect(sys.argv[1]);c.execute('create table xshop_rehearsal_fixture(id integer primary key,value text)');c.execute("insert into xshop_rehearsal_fixture values(1,'synthetic business record')");c.commit()`, db)
	before := python(t, "-c", dbDigest, db)
	pubRaw, _ := os.ReadFile(filepath.Join(artifacts, "public-key.txt"))
	key, err := ParseKey(strings.TrimSpace(string(pubRaw)))
	if err != nil {
		t.Fatal(err)
	}
	schema, _ := os.ReadFile(filepath.Join(artifacts, "schema-fingerprint.txt"))
	engine := &Engine{Target: target, StateDir: filepath.Join(root, "state"), Control: ctl, Backup: func(ctx context.Context, dir string) error {
		cmd := exec.CommandContext(ctx, "/usr/bin/python3", "-c", `import sqlite3,sys;s=sqlite3.connect(sys.argv[1]);d=sqlite3.connect(sys.argv[2]);s.backup(d);d.close();s.close()`, db, filepath.Join(dir, "database.sqlite3"))
		return cmd.Run()
	}}
	if err = engine.Initialize(release.Sequence - 1); err != nil {
		t.Fatal(err)
	}
	h := &Helper{Engine: engine, Source: localSource{filepath.Join(artifacts, "B")}, PublicKey: key, Schema: strings.TrimSpace(string(schema))}
	srv := httptest.NewServer(h)
	defer srv.Close()
	call := func(action, body string) {
		resp, err := http.Post(srv.URL+"/"+action, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 202 {
			t.Fatalf("%s refused: %d", action, resp.StatusCode)
		}
	}
	wait := func(wanted string) Status {
		deadline := time.Now().Add(100 * time.Second)
		for time.Now().Before(deadline) {
			resp, err := http.Get(srv.URL + "/status")
			if err != nil {
				t.Fatal(err)
			}
			var s Status
			json.NewDecoder(resp.Body).Decode(&s)
			resp.Body.Close()
			if s.State == wanted {
				return s
			}
			if s.State == "failed" && wanted != "failed" {
				t.Fatal(s.Message)
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal("helper timeout")
		return Status{}
	}
	call("check", "")
	s := wait("available")
	payload, _ := json.Marshal(map[string]string{"digest": s.Digest})
	oldPID := ctl.cmd.Process.Pid
	oldStarts := ctl.starts
	call("install", string(payload))
	prepared := wait("prepared")
	actualStillA, err := FileHash("/proc/" + stringPID(ctl.cmd.Process.Pid) + "/exe")
	if err != nil || actualStillA != aHash || ctl.cmd.Process.Pid != oldPID || ctl.starts != oldStarts || !prepared.NeedRestart {
		t.Fatal("preparation restarted the serving process or did not request explicit restart")
	}
	if err = ctl.Healthy(ctx, aHash); err != nil {
		t.Fatal("old A stopped serving during preparation", err)
	}
	call("restart", "")
	wait("installed")
	if ctl.runningHash != bHash {
		t.Fatal("B is not running")
	}
	versionResp, err := http.Get(ctl.url + "/api/v1/public/config")
	if err != nil {
		t.Fatal(err)
	}
	var versionEnvelope struct {
		Data struct {
			Version string `json:"app_version"`
		} `json:"data"`
	}
	json.NewDecoder(versionResp.Body).Decode(&versionEnvelope)
	versionResp.Body.Close()
	if versionEnvelope.Data.Version != release.Version {
		t.Fatal("B version endpoint mismatch")
	}
	after := python(t, "-c", dbDigest, db)
	if !bytes.Equal(before, after) {
		t.Fatal("business rows changed across A->B")
	}
	gotCfg, _ := os.ReadFile(filepath.Join(root, "config.yml"))
	gotUpload, _ := os.ReadFile(filepath.Join(root, "uploads", "synthetic.txt"))
	if !bytes.Equal(cfgRaw, gotCfg) || string(gotUpload) != "synthetic upload retained" {
		t.Fatal("config/uploads changed")
	}
	// Same latest must authenticate as current, never take the replay install path.
	call("check", "")
	wait("up_to_date")
	call("rollback", "")
	wait("rolled_back")
	if ctl.runningHash != aHash {
		t.Fatal("manual rollback did not restore actual running A")
	}
	manualState, manualErr := engine.ReadState()
	if manualErr != nil || manualState.HighWater != release.Sequence || manualState.Pending {
		t.Fatal("manual rollback fence changed", manualState, manualErr)
	}
	manualData := python(t, "-c", dbDigest, db)
	if !bytes.Equal(before, manualData) {
		t.Fatal("manual rollback rewound synthetic data")
	}
	call("check", "")
	wait("failed") // Consumed B remains denied after rollback.
	// Rehearse a separate fresh trusted A state, not a rollback bypass on the channel.
	if err = ctl.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err = atomicCopy(filepath.Join(artifacts, "A", "dujiao-next"), target, 0755); err != nil {
		t.Fatal(err)
	}
	state2 := filepath.Join(root, "failure-state")
	os.Mkdir(state2, 0700)
	engine.StateDir = state2
	if err = engine.Initialize(release.Sequence - 1); err != nil {
		t.Fatal(err)
	}
	ctl.Start(ctx)
	if err = ctl.Healthy(ctx, aHash); err != nil {
		t.Fatal(err)
	}
	ctl.failNext = true
	call("check", "")
	s = wait("available")
	payload, _ = json.Marshal(map[string]string{"digest": s.Digest})
	call("install", string(payload))
	wait("prepared")
	call("restart", "")
	wait("failed")
	if ctl.runningHash != aHash {
		t.Fatal("A rollback not running")
	}
	state, _ := engine.ReadState()
	if state.Pending || state.HighWater != release.Sequence {
		t.Fatal("rollback replay fence wrong")
	}
	after = python(t, "-c", dbDigest, db)
	if !bytes.Equal(before, after) {
		t.Fatal("rollback rewound/changed business rows")
	}
	if evidence := os.Getenv("XSHOP_REHEARSAL_EVIDENCE"); evidence != "" {
		details, _ := json.MarshalIndent(map[string]any{"A_sha256": aHash, "B_sha256": bHash, "all_table_fingerprints_preserved": true, "config_preserved": true, "uploads_preserved": true, "successfully_running_B_observed": true, "failure_rollback_running_A_observed": true, "synthetic_super_admin_boundary": 503, "ordinary_admin_rejected": 403, "revoked_token_rejected": 401, "final_high_water": state.HighWater, "before_fingerprints": json.RawMessage(before)}, "", "  ")
		if err = os.WriteFile(filepath.Join(evidence, "rehearsal-details.json"), details, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("actual A->B PASS B=%s; health-failure->A PASS A=%s; all-table fingerprints/config/uploads unchanged", bHash, aHash)
}
