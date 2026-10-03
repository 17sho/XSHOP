package xshopupgrade

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
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
	for _, p := range []string{"0::/system.slice/dujiao-preview.service", "0::/system.slice/dujiao-next.service-evil", "0::/user.slice/dujiao-next.service"} {
		if PeerAllowed(1001, 1001, p) {
			t.Fatal("wrong peer accepted")
		}
	}
	if !PeerAllowed(1001, 1001, "0::/system.slice/dujiao-next.service\n") {
		t.Fatal("preview peer denied")
	}
	if PeerAllowed(0, 1001, "0::/system.slice/dujiao-next.service") {
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
	for _, v := range []string{"../evil", "xshop-production-b;sh", "https://evil.invalid", "v1.0.0"} {
		if ValidTag(v) {
			t.Fatal("tag accepted", v)
		}
	}
	if !ValidTag("xshop-production-b1") {
		t.Fatal("safe tag denied")
	}
	for _, v := range []string{"../x.tar.gz", "https://evil.invalid", "a/b"} {
		if ValidAsset(v) {
			t.Fatal("asset accepted")
		}
	}
}

func TestProductionCompileBoundary(t *testing.T) {
	if !Production {
		t.Skip("production-only contract")
	}
	if PreviewService != "dujiao-next.service" || PreviewRoot != "/opt/dujiao-next" || StateRoot != "/var/lib/xshop-production-upgrader" || SocketPath != "/run/xshop-production-upgrader/control.sock" {
		t.Fatal("production namespace not fixed")
	}
	if !ValidTag("xshop-production-v1") || ValidTag("xshop-preview-v1") {
		t.Fatal("cross tag namespace")
	}
	if !PeerAllowed(996, 996, "0::/system.slice/dujiao-next.service\n") || PeerAllowed(996, 996, "0::/system.slice/dujiao-preview.service\n") {
		t.Fatal("cross peer")
	}
	if !strings.Contains(previewHashWorker, "0::/system.slice/dujiao-next.service") || strings.Contains(previewHashWorker, "dujiao-preview.service") {
		t.Fatal("worker cross scope")
	}
}

func TestProductionOperatorPolicyFixedDatabasePort(t *testing.T) {
	if !Production {
		t.Skip("production-only contract")
	}
	p := OperatorPolicy{PublicKey: strings.Repeat("01", 32), Schema: strings.Repeat("a", 64), Database: "/opt/dujiao-next/db/dujiao.db", Port: 18080, BootstrapSequence: 1}
	if err := ValidateOperatorPolicy(p); err != nil {
		t.Fatal(err)
	}
	for _, db := range []string{"/opt/dujiao-preview/db/dujiao.db", "/opt/dujiao-next/db/other.db", "/opt/dujiao-next/db/../db/dujiao.db"} {
		q := p
		q.Database = db
		if ValidateOperatorPolicy(q) == nil {
			t.Fatal("wrong database", db)
		}
	}
	q := p
	q.Port = 18083
	if ValidateOperatorPolicy(q) == nil {
		t.Fatal("preview port accepted")
	}
}
