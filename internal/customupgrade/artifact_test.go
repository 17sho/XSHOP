package customupgrade

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestReleaseAssetURL(t *testing.T) {
	got, e := ReleaseAssetURL("v1.2.3-preview.2", "dujiao-preview.tar.gz")
	if e != nil || got != "https://github.com/17sho/XSHOP/releases/download/v1.2.3-preview.2/dujiao-preview.tar.gz" {
		t.Fatalf("wrong fixed URL: %s %v", got, e)
	}
	for _, s := range []string{"", "../evil", "a/b", "a\\b", "https://evil", "%2f", "..", "a?x", "x#y", "@evil", strings.Repeat("a", 129), "a\n", "a b"} {
		t.Run(s, func(t *testing.T) {
			if _, e := ReleaseAssetURL(s, "manifest.json"); e == nil {
				t.Fatal("unsafe tag accepted")
			}
			if _, e := ReleaseAssetURL("v1", s); e == nil {
				t.Fatal("unsafe asset accepted")
			}
		})
	}
}
func TestVerifySource(t *testing.T) {
	m, p, k := fixture(t)
	source := []byte("corresponding source archive bytes")
	m["source"].(map[string]any)["size"] = len(source)
	m["source"].(map[string]any)["sha256"] = digest(source)
	b, s := signed(t, m, k)
	v, e := VerifyManifest(b, s, p)
	if e != nil {
		t.Fatal(e)
	}
	if e = VerifySource(context.Background(), bytes.NewReader(source), v); e != nil {
		t.Fatal(e)
	}
	for n, b := range map[string][]byte{"truncated": source[:len(source)-1], "extra": append(append([]byte(nil), source...), 0), "hash": bytes.Repeat([]byte("x"), len(source))} {
		t.Run(n, func(t *testing.T) {
			if e := VerifySource(context.Background(), bytes.NewReader(b), v); e == nil {
				t.Fatal("bad source accepted")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := VerifySource(ctx, bytes.NewReader(source), v); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	for _, v := range []*VerifiedManifest{nil, {}} {
		if e := VerifySource(context.Background(), bytes.NewReader(source), v); e == nil {
			t.Fatal("unverified source accepted")
		}
	}
}
