package xshopupgrade

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSignedPackageInstallActuallyPrepares(t *testing.T) {
	artifacts := os.Getenv("XSHOP_REHEARSAL_DIR")
	if artifacts == "" {
		t.Skip("requires signed local candidate artifacts")
	}
	root := t.TempDir()
	target := filepath.Join(root, "app")
	if err := atomicCopy(filepath.Join(artifacts, "A", "dujiao-next"), target, 0755); err != nil {
		t.Fatal(err)
	}
	ctl := &testController{}
	e := &Engine{Target: target, StateDir: root, Control: ctl, Backup: func(context.Context, string) error { return nil }}
	if err := e.Initialize(1); err != nil {
		t.Fatal(err)
	}
	pub, _ := os.ReadFile(filepath.Join(artifacts, "public-key.txt"))
	key, err := ParseKey(strings.TrimSpace(string(pub)))
	if err != nil {
		t.Fatal(err)
	}
	schema, _ := os.ReadFile(filepath.Join(artifacts, "schema-fingerprint.txt"))
	h := &Helper{Engine: e, Source: localSource{filepath.Join(artifacts, "B")}, PublicKey: key, Schema: strings.TrimSpace(string(schema))}
	h.check(context.Background())
	if h.status.State != "available" {
		t.Fatal(h.status)
	}
	digest := h.status.Digest
	h.install(context.Background(), "xshop-preview-b1", digest)
	if h.status.State != "prepared" || !h.status.NeedRestart || ctl.started != 0 {
		t.Fatal("signed install did not prepare without restart", h.status, ctl.started)
	}
	if err := e.Restart(context.Background()); err != nil {
		t.Fatal(err)
	}
	s, err := e.ReadState()
	if err != nil || s.Result != "installed" {
		t.Fatal(s, err)
	}
}
