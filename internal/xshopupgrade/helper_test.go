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
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestHelperProtocolRejectsUnknownAndNonemptyCheck(t *testing.T) {
	h := &Helper{}
	for _, x := range []struct{ method, path, body string }{{"POST", "/restart", ""}, {"POST", "/check", "{\"url\":\"https://evil.invalid\"}"}, {"POST", "/install", "{\"digest\":\"bad\",\"service\":\"dujiao-next\"}"}, {"GET", "/install", ""}} {
		r := httptest.NewRequest(x.method, x.path, strings.NewReader(x.body))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code < 400 {
			t.Fatalf("unsafe protocol accepted: %+v", x)
		}
	}
}
func TestPeerRequiresExactPreviewCgroup(t *testing.T) {
	other := "dujiao-next.service"
	if Production {
		other = "dujiao-preview.service"
	}
	for _, p := range []string{"0::/system.slice/" + other, "0::" + previewCgroup + "-evil", "0::/user.slice/" + PreviewService} {
		if PeerAllowed(1001, 1001, p) {
			t.Fatal("wrong peer accepted")
		}
	}
	if !PeerAllowed(1001, 1001, "0::"+previewCgroup+"\n") {
		t.Fatal("preview peer denied")
	}
	if PeerAllowed(0, 1001, "0::"+previewCgroup) {
		t.Fatal("wrong uid accepted")
	}
}
func TestStatusProjectionContainsNoInternalPaths(t *testing.T) {
	h := &Helper{status: Status{State: "idle", Version: "A"}}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/status", nil))
	var s map[string]any
	if json.Unmarshal(w.Body.Bytes(), &s) != nil || s["state"] != "idle" {
		t.Fatal(w.Body.String())
	}
	if strings.Contains(w.Body.String(), "/root") {
		t.Fatal("path leaked")
	}
	_ = context.Background()
}
func TestGitHubSourceRejectsUntrustedTagAndNames(t *testing.T) {
	for _, v := range []string{"../evil", "xshop-preview-b;sh", "https://evil.invalid", "v1.0.0"} {
		if ValidTag(v) {
			t.Fatal("tag accepted", v)
		}
	}
	if !ValidTag(TagPrefix + "b1") {
		t.Fatal("safe tag denied")
	}
	for _, v := range []string{"../x.tar.gz", "https://evil.invalid", "a/b"} {
		if ValidAsset(v) {
			t.Fatal("asset accepted")
		}
	}
}

func TestBoundedProductionReleaseList(t *testing.T) {
	calls := 0
	tags, err := productionCandidates(context.Background(), func(_ context.Context, endpoint string) ([]byte, error) {
		calls++
		if endpoint != fmt.Sprintf("repos/%s/releases?per_page=100&page=%d", Repository, calls) {
			t.Fatal(endpoint)
		}
		rs := []release{{Tag: "xshop-preview-v99"}, {Tag: "xshop-production-draft", Draft: true}, {Tag: "xshop-production-beta", Prerelease: true}, {Tag: "xshop-production-v2"}, {Tag: "xshop-production-v1"}}
		return json.Marshal(rs)
	})
	if err != nil || len(tags) != 2 || tags[0] != "xshop-production-v1" {
		t.Fatalf("%v %v", tags, err)
	}
	calls = 0
	_, err = productionCandidates(context.Background(), func(_ context.Context, _ string) ([]byte, error) {
		calls++
		rs := make([]release, 100)
		return json.Marshal(rs)
	})
	if err == nil || calls != 5 {
		t.Fatalf("unbounded/truncated list: %d %v", calls, err)
	}
}

func TestNewestSequenceRequiresVerificationAndDeterministicTie(t *testing.T) {
	tags := []string{"xshop-production-z", "xshop-production-unsigned", "xshop-production-old", "xshop-production-a"}
	tag, err := highestVerified(tags, func(tag string) (uint64, error) {
		switch tag {
		case "xshop-production-unsigned":
			return 999, errors.New("signature rejected")
		case "xshop-production-old":
			return 1, nil
		default:
			return 8, nil
		}
	})
	if err != nil || tag != "xshop-production-a" {
		t.Fatal(tag, err)
	}
	if _, err = highestVerified(tags, func(string) (uint64, error) { return 0, errors.New("profile rejected") }); err == nil {
		t.Fatal("invalid candidates selected")
	}
}

func TestReleaseMetadataBindsRequestedTag(t *testing.T) {
	if err := validateReleaseIdentity(release{Tag: TagPrefix + "other"}, TagPrefix+"wanted"); err == nil {
		t.Fatal("metadata tag substitution accepted")
	}
	if err := validateReleaseIdentity(release{Tag: TagPrefix + "wanted"}, TagPrefix+"wanted"); err != nil {
		t.Fatal(err)
	}
}

type multiSignedSource struct{ assets map[string][2][]byte }

func (s multiSignedSource) Latest(context.Context) (string, error) { return "xshop-preview-beta", nil }
func (s multiSignedSource) ProductionCandidates(context.Context) ([]string, error) {
	return []string{"xshop-production-v1", "xshop-production-v2", "xshop-production-tampered", "xshop-preview-beta"}, nil
}
func (s multiSignedSource) Asset(_ context.Context, tag, name string, _ uint64) ([]byte, error) {
	a, ok := s.assets[tag]
	if !ok {
		return nil, errors.New("missing")
	}
	if name == "manifest.json" {
		return a[0], nil
	}
	return a[1], nil
}
func TestProductionSelectsAuthenticatedNewestDespitePreviewLatest(t *testing.T) {
	if !Production {
		t.Skip("production-only selector")
	}
	e, _, _ := fixture(t)
	old, _ := FileHash(e.Target)
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	schema := strings.Repeat("a", 64)
	assets := map[string][2][]byte{}
	for tag, seq := range map[string]uint64{"xshop-production-v1": 2, "xshop-production-v2": 3, "xshop-production-tampered": 999, "xshop-preview-beta": 1000} {
		m := customupgrade.Manifest{SchemaVersion: 1, Product: "XSHOP", Sequence: seq, Version: tag, SourceCommit: strings.Repeat("a", 40), Channel: "stable", Profile: "embedded-production", OS: "linux", Arch: "amd64", MinimumUpdater: 1, FromBinarySHA256: []string{old}, MigrationPolicy: "unchanged", SchemaFingerprint: schema, Archive: customupgrade.Artifact{Name: "binary.tar.gz", Size: 100, SHA256: schema}, Source: customupgrade.Artifact{Name: "source.tar.gz", Size: 100, SHA256: schema}, Files: []customupgrade.File{{Path: "dujiao-next", Size: 1, SHA256: schema, Mode: 0755}}}
		raw, _ := json.Marshal(m)
		sig := ed25519.Sign(key, raw)
		if tag == "xshop-production-tampered" {
			sig[0] ^= 1
		}
		assets[tag] = [2][]byte{raw, sig}
	}
	h := Helper{Engine: e, Source: multiSignedSource{assets}, PublicKey: pub, Schema: schema}
	h.check(context.Background())
	if h.status.State != "available" || h.status.Sequence != 3 || h.status.Version != "xshop-production-v2" {
		t.Fatal(h.status)
	}
}

func TestFixedWorkerRejectsOwnSameUIDWrongCgroup(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("credential transition requires root")
	}
	uid, gid, err := previewCredentials()
	if err != nil {
		t.Skip("dujiao account missing")
	}
	cmd := exec.Command("/usr/bin/sleep", "1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uid, Gid: gid, Groups: []uint32{}}}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Wait()
	if _, err = readPreviewIdentity(strconv.Itoa(cmd.Process.Pid), uid, gid); err == nil {
		t.Fatal("Go accepted wrong cgroup")
	}
	if _, err = hashPreviewIdentity(context.Background(), previewIdentity{PID: cmd.Process.Pid, UID: uid, GID: gid, Start: "1"}); err == nil {
		t.Fatal("Python accepted wrong cgroup")
	}
}

func TestUnreadableJournalOverridesCachedAvailableStatus(t *testing.T) {
	h := &Helper{Engine: &Engine{StateDir: t.TempDir()}, status: Status{State: "available", Digest: strings.Repeat("a", 64)}}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/status", nil))
	var s Status
	if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil {
		t.Fatal(err)
	}
	if s.State != "failed" || s.Digest != "" {
		t.Fatalf("unreadable trusted journal advertised installable state: %+v", s)
	}
	if strings.Contains(w.Body.String(), h.Engine.StateDir) {
		t.Fatal("internal path exposed")
	}
}

func TestActivationFailureClaimsRestorationOnlyAfterConfirmedRollback(t *testing.T) {
	e := &Engine{StateDir: t.TempDir()}
	if err := e.Initialize(1); err != nil {
		t.Fatal(err)
	}
	h := &Helper{Engine: e}
	if got := h.activationFailureMessage(); strings.Contains(got, "旧版本已恢复") {
		t.Fatal("idle journal misreported as rollback")
	}
	s, err := e.ReadState()
	if err != nil {
		t.Fatal(err)
	}
	s.Result = "installed"
	if err := e.saveState(s); err != nil {
		t.Fatal(err)
	}
	if got := h.activationFailureMessage(); strings.Contains(got, "旧版本已恢复") {
		t.Fatal("installed journal misreported as rollback")
	}
	s.Result = "rolled_back"
	if err := e.saveState(s); err != nil {
		t.Fatal(err)
	}
	if got := h.activationFailureMessage(); strings.Contains(got, "旧版本已恢复") {
		t.Fatal("label-only rollback misreported as confirmed recovery")
	}

	// A true result requires the real state machine to restore and verify A,
	// not merely a raw JSON label. The old positive fixture lacked this fence.
	e, ctl, next := fixture(t)
	ctl.failFirst = true
	h.Engine = e
	old, err := FileHash(e.Target)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := FileHash(next)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Activate(context.Background(), next, old, candidate, 2); err == nil {
		t.Fatal("fixture did not exercise health-failure rollback")
	}
	if actual, err := FileHash(e.Target); err != nil || actual != old || ctl.running != old {
		t.Fatal("fixture did not restore verified A")
	}
	if got := h.activationFailureMessage(); !strings.Contains(got, "旧版本已恢复") {
		t.Fatal("confirmed rollback not recognized")
	}
	fresh := &Engine{Target: e.Target, StateDir: e.StateDir, Control: ctl}
	h.Engine = fresh
	if got := h.activationFailureMessage(); strings.Contains(got, "旧版本已恢复") {
		t.Fatal("fresh engine trusted terminal label without recovery")
	}
	if err := fresh.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := h.activationFailureMessage(); !strings.Contains(got, "旧版本已恢复") {
		t.Fatal("startup identity-verified rollback not recognized")
	}
	s, err = fresh.ReadState()
	if err != nil {
		t.Fatal(err)
	}
	s.Pending = true
	if err := fresh.saveState(s); err != nil {
		t.Fatal(err)
	}
	if got := h.activationFailureMessage(); strings.Contains(got, "旧版本已恢复") {
		t.Fatal("pending journal misreported as rollback")
	}
}

func TestGitHubCommandPinsHostAndRejectsInheritedAuthorityEnvironment(t *testing.T) {
	t.Setenv("GH_HOST", "evil.invalid")
	t.Setenv("GH_REPO", "other/other")
	t.Setenv("GH_TOKEN", "synthetic-no-network-fixture")
	t.Setenv("GH_CONFIG_DIR", "/tmp/untrusted-fixture")
	cmd := githubCommand(context.Background(), "api", "repos/"+Repository+"/releases/latest")
	if cmd.Path != "/usr/bin/gh" || !slices.Equal(cmd.Args[1:4], []string{"api", "--hostname", "github.com"}) {
		t.Fatalf("host/binary not pinned: %v", cmd.Args)
	}
	env := strings.Join(cmd.Env, "\n")
	for _, bad := range []string{"evil.invalid", "other/other", "synthetic-no-network-fixture", "/tmp/untrusted-fixture"} {
		if strings.Contains(env, bad) {
			t.Fatal("inherited host/credential authority")
		}
	}
	for _, good := range []string{"HOME=/root", "GH_CONFIG_DIR=/root/.config/gh", "GH_PROMPT_DISABLED=1"} {
		if !strings.Contains(env, good) {
			t.Fatalf("required fixed backend setting missing: %s", good)
		}
	}
}

func TestHelperPreparedStatusAndExplicitRestart(t *testing.T) {
	e, _, next := fixture(t)
	old, _ := FileHash(e.Target)
	candidate, _ := FileHash(next)
	if err := e.Prepare(context.Background(), next, old, candidate, 2, "xshop-preview-b", strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	h := &Helper{Engine: e}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/status", nil))
	var s map[string]any
	json.Unmarshal(w.Body.Bytes(), &s)
	if s["state"] != "prepared" || s["need_restart"] != true {
		t.Fatal(w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/restart", nil))
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		h.mu.Lock()
		busy := h.busy
		state := h.status.State
		h.mu.Unlock()
		if !busy {
			if state != "installed" {
				t.Fatal(state)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("restart did not finish")
}
