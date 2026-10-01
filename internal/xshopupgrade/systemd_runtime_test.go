package xshopupgrade

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestSystemdUnitRestrictedHashPolicy(t *testing.T) {
	raw, err := os.ReadFile("../../deploy/xshop-preview-upgrader.service")
	if err != nil {
		t.Fatal(err)
	}
	unit := string(raw)
	for _, line := range []string{"CapabilityBoundingSet=CAP_DAC_OVERRIDE CAP_CHOWN CAP_SETUID CAP_SETGID\n", "RestartPreventExitStatus=78\n", "NoNewPrivileges=yes\n", "ProtectControlGroups=yes\n", "RestrictNamespaces=yes\n"} {
		if !strings.Contains(unit, line) {
			t.Errorf("missing required policy: %s", line)
		}
	}
	if strings.Contains(unit, "CAP_SYS_PTRACE") {
		t.Fatal("broad ptrace capability forbidden")
	}
}

func TestSystemdHashRejectsNonPreviewWorkerIdentity(t *testing.T) {
	uid, gid, err := previewCredentials()
	if err != nil {
		t.Skip("dujiao account required")
	}
	for _, identity := range []previewIdentity{{PID: 0, UID: uid, GID: gid, Start: "1"}, {PID: 2, UID: 0, GID: gid, Start: "1"}, {PID: 2, UID: uid, GID: 0, Start: "1"}, {PID: 2, UID: uid + 1, GID: gid, Start: "1"}, {PID: 2, UID: uid, GID: gid, Start: "not-a-start"}} {
		_, err := hashPreviewIdentity(context.Background(), identity)
		if err == nil || !strings.Contains(err.Error(), "invalid hash worker identity") {
			t.Errorf("must reject before executing credential transition: identity=%+v err=%v", identity, err)
		}
	}
}

func TestSystemdRejectsNoncanonicalPID(t *testing.T) {
	for _, pid := range []string{"0", "1", "-1", "+2", "02", "2/../1", "99999999999999999999999999", ""} {
		if _, err := readPreviewIdentity(pid, 996, 989); err == nil {
			t.Errorf("accepted PID %q", pid)
		}
	}
}

func TestSystemdLiveIdentityFailures(t *testing.T) {
	if os.Getenv("XSHOP_RUNTIME_PROBE") != "1" {
		t.Skip("explicit same-host read-only probe required")
	}
	ctx := context.Background()
	hash, id, err := runningPreviewHash(ctx)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := FileHash(PreviewRoot + "/dujiao-next")
	if err != nil || hash != expected {
		t.Fatalf("live hash mismatch %s %s %v", hash, expected, err)
	}
	pid := strconv.Itoa(id.PID)
	for _, wrong := range [][2]uint32{{id.UID + 1, id.GID}, {id.UID, id.GID + 1}} {
		if _, err := readPreviewIdentity(pid, wrong[0], wrong[1]); err == nil {
			t.Fatal("accepted wrong UID/GID")
		}
	}
	changed := id
	changed.Start = "1"
	if _, err := hashPreviewIdentity(ctx, changed); err == nil {
		t.Fatal("accepted replaced PID/starttime")
	}
	changed = id
	changed.PID = 2147483647
	if _, err := hashPreviewIdentity(ctx, changed); err == nil {
		t.Fatal("accepted absent PID")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := hashPreviewIdentity(canceled, id); err == nil {
		t.Fatal("canceled worker did not fail closed")
	}
	// Same UID real process, outside preview cgroup. It exits by itself; no
	// CAP_KILL, service mutation, updater, or production interaction needed.
	fixture := exec.Command("/usr/bin/sleep", "0.5")
	fixture.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: id.UID, Gid: id.GID, Groups: []uint32{}}}
	if err := fixture.Start(); err != nil {
		t.Fatal(err)
	}
	defer fixture.Wait()
	if _, err := readPreviewIdentity(strconv.Itoa(fixture.Process.Pid), id.UID, id.GID); err == nil {
		t.Fatal("accepted wrong cgroup")
	}
	changed = id
	changed.PID = fixture.Process.Pid
	if _, err := hashPreviewIdentity(ctx, changed); err == nil {
		t.Fatal("worker accepted wrong cgroup")
	}
}

func TestSystemdLiveHealthFailures(t *testing.T) {
	if os.Getenv("XSHOP_RUNTIME_PROBE") != "1" {
		t.Skip("explicit same-host read-only probe required")
	}
	expected, err := FileHash(PreviewRoot + "/dujiao-next")
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{"status":"bad"}`, `not-json`, strings.Repeat("x", 4097)} {
		t.Run(body[:3], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := (Systemd{HealthURL: server.URL}).Healthy(ctx, expected); err == nil {
				t.Fatal("accepted failed JSON health")
			}
		})
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.Write([]byte(`{"status":"ok"}`)) }))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := (Systemd{HealthURL: server.URL}).Healthy(ctx, strings.Repeat("0", 64)); err == nil {
		t.Fatal("accepted incorrect hash")
	}
	if calls != 0 {
		t.Fatal("health requested before hash authorization")
	}
}

func TestSystemdLivePreviewHTTPHealth(t *testing.T) {
	if os.Getenv("XSHOP_RUNTIME_PROBE") != "1" {
		t.Skip("explicit same-host read-only probe required")
	}
	expected, err := FileHash(PreviewRoot + "/dujiao-next")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// Fixed host preview listener discovered read-only, not an updater endpoint.
	if err := (Systemd{HealthURL: "http://127.0.0.1:18083/health"}).Healthy(ctx, expected); err != nil {
		t.Fatal(err)
	}
}

// Opt-in read-only probe: does not install/start an updater or mutate any service.
func TestSystemdLiveRestrictedHealthy(t *testing.T) {
	if os.Getenv("XSHOP_RUNTIME_PROBE") != "1" {
		t.Skip("explicit same-host read-only probe required")
	}
	expected, err := FileHash(PreviewRoot + "/dujiao-next")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"status":"ok"}`)) }))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := (Systemd{HealthURL: server.URL}).Healthy(ctx, expected); err != nil {
		t.Fatalf("legitimate fixed preview running hash/health must work with proposed caps: %v", err)
	}
}
