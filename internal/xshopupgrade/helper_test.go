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
	for _, p := range []string{"0::/system.slice/dujiao-next.service", "0::/system.slice/dujiao-preview.service-evil", "0::/user.slice/dujiao-preview.service"} {
		if PeerAllowed(1001, 1001, p) {
			t.Fatal("wrong peer accepted")
		}
	}
	if !PeerAllowed(1001, 1001, "0::/system.slice/dujiao-preview.service\n") {
		t.Fatal("preview peer denied")
	}
	if PeerAllowed(0, 1001, "0::/system.slice/dujiao-preview.service") {
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
	if !ValidTag("xshop-preview-b1") {
		t.Fatal("safe tag denied")
	}
	for _, v := range []string{"../x.tar.gz", "https://evil.invalid", "a/b"} {
		if ValidAsset(v) {
			t.Fatal("asset accepted")
		}
	}
}
