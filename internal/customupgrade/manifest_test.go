package customupgrade

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func fixture(t *testing.T) (map[string]any, Policy, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	hash := digest([]byte("current"))
	schema := digest([]byte("schema"))
	p := Policy{PublicKey: pub, Channel: "preview", Profile: "embedded-preview", OS: "linux", Arch: "amd64", CurrentBinarySHA256: hash, SchemaFingerprint: schema, HighWaterSequence: 1, UpdaterVersion: 1}
	m := map[string]any{"schema_version": 1, "product": "XSHOP", "sequence": 2, "version": "v1.2.3", "source_commit": "0123456789012345678901234567890123456789", "channel": "preview", "profile": "embedded-preview", "os": "linux", "arch": "amd64", "minimum_updater": 1, "from_binary_sha256": []string{hash}, "migration_policy": "unchanged", "schema_fingerprint": schema, "archive": map[string]any{"name": "dujiao-preview.tar.gz", "size": 100, "sha256": digest([]byte("archive"))}, "source": map[string]any{"name": "source.tar.gz", "size": 100, "sha256": digest([]byte("source"))}, "files": []any{map[string]any{"path": "dujiao-next", "size": 100, "sha256": digest([]byte("binary")), "mode": 493}}}
	return m, p, priv
}
func signed(t *testing.T, m map[string]any, key ed25519.PrivateKey) ([]byte, []byte) {
	t.Helper()
	b, e := json.Marshal(m)
	if e != nil {
		t.Fatal(e)
	}
	return b, ed25519.Sign(key, b)
}
func TestManifestMembers(t *testing.T) {
	cases := map[string]func(map[string]any){
		"empty":                    func(m map[string]any) { m["files"] = []any{} },
		"duplicate":                func(m map[string]any) { f := m["files"].([]any); m["files"] = append(f, f[0]) },
		"archive-source-collision": func(m map[string]any) { m["source"].(map[string]any)["name"] = m["archive"].(map[string]any)["name"] },
	}
	for _, path := range []string{"../dujiao-next", "./dujiao-next", "/dujiao-next", "a/../dujiao-next", "a//dujiao-next", "dujiao-next/", "config.yml", "uploads/x", "db/data.db", "dujiao-next\\x"} {
		p := path
		cases["path-"+p] = func(m map[string]any) { m["files"].([]any)[0].(map[string]any)["path"] = p }
	}
	for n, value := range map[string]any{"size-zero": uint64(0), "size-over": uint64(1<<30) + 1, "ratio": uint64(100000)} {
		v := value
		cases[n] = func(m map[string]any) { m["files"].([]any)[0].(map[string]any)["size"] = v }
	}
	cases["mode"] = func(m map[string]any) { m["files"].([]any)[0].(map[string]any)["mode"] = 0777 }
	cases["hash"] = func(m map[string]any) { m["files"].([]any)[0].(map[string]any)["sha256"] = strings.Repeat("A", 64) }
	for n, mut := range cases {
		t.Run(n, func(t *testing.T) {
			m, p, k := fixture(t)
			mut(m)
			raw, sig := signed(t, m, k)
			if _, e := VerifyManifest(raw, sig, p); e == nil {
				t.Fatal("unsafe member accepted")
			}
		})
	}
}
func TestManifestPolicy(t *testing.T) {
	mutations := map[string]func(map[string]any, *Policy){
		"schema-version":      func(m map[string]any, p *Policy) { m["schema_version"] = 2 },
		"product":             func(m map[string]any, p *Policy) { m["product"] = "official" },
		"sequence-zero":       func(m map[string]any, p *Policy) { m["sequence"] = 0 },
		"replay":              func(m map[string]any, p *Policy) { m["sequence"] = 1 },
		"rollback":            func(m map[string]any, p *Policy) { p.HighWaterSequence = 3 },
		"max-highwater":       func(m map[string]any, p *Policy) { p.HighWaterSequence = ^uint64(0) },
		"empty-version":       func(m map[string]any, p *Policy) { m["version"] = "" },
		"url-version":         func(m map[string]any, p *Policy) { m["version"] = "https://evil/a" },
		"commit":              func(m map[string]any, p *Policy) { m["source_commit"] = strings.Repeat("A", 40) },
		"channel-mismatch":    func(m map[string]any, p *Policy) { m["channel"] = "stable" },
		"unknown-channel":     func(m map[string]any, p *Policy) { m["channel"] = "nightly"; p.Channel = "nightly" },
		"split-production":    func(m map[string]any, p *Policy) { m["profile"] = "split-production"; p.Profile = "split-production" },
		"profile-mismatch":    func(m map[string]any, p *Policy) { p.Profile = "split-production" },
		"os":                  func(m map[string]any, p *Policy) { m["os"] = "darwin"; p.OS = "darwin" },
		"arch":                func(m map[string]any, p *Policy) { m["arch"] = "arm64"; p.Arch = "arm64" },
		"updater-zero":        func(m map[string]any, p *Policy) { m["minimum_updater"] = 0 },
		"updater-old":         func(m map[string]any, p *Policy) { m["minimum_updater"] = 2 },
		"policy-updater-zero": func(m map[string]any, p *Policy) { p.UpdaterVersion = 0 },
		"from-empty":          func(m map[string]any, p *Policy) { m["from_binary_sha256"] = []string{} },
		"from-mismatch":       func(m map[string]any, p *Policy) { p.CurrentBinarySHA256 = digest([]byte("other")) },
		"from-invalid":        func(m map[string]any, p *Policy) { m["from_binary_sha256"] = []string{p.CurrentBinarySHA256, "bad"} },
		"from-duplicate": func(m map[string]any, p *Policy) {
			m["from_binary_sha256"] = []string{p.CurrentBinarySHA256, p.CurrentBinarySHA256}
		},
		"from-too-many":   func(m map[string]any, p *Policy) { m["from_binary_sha256"] = make([]string, 65) },
		"migration":       func(m map[string]any, p *Policy) { m["migration_policy"] = "destructive" },
		"schema-mismatch": func(m map[string]any, p *Policy) { p.SchemaFingerprint = digest([]byte("other")) },
		"schema-format": func(m map[string]any, p *Policy) {
			m["schema_fingerprint"] = strings.Repeat("A", 64)
			p.SchemaFingerprint = strings.Repeat("A", 64)
		},
	}
	for _, section := range []string{"archive", "source"} {
		for _, field := range []string{"name", "size", "sha256"} {
			s, f := section, field
			mutations[s+"-"+f] = func(m map[string]any, p *Policy) {
				a := m[s].(map[string]any)
				switch f {
				case "name":
					a[f] = "../evil.tar.gz"
				case "size":
					a[f] = 0
				case "sha256":
					a[f] = strings.Repeat("A", 64)
				}
			}
		}
		s := section
		mutations[s+"-huge"] = func(m map[string]any, p *Policy) { m[s].(map[string]any)["size"] = uint64(2 << 30) }
	}
	for name, mut := range mutations {
		t.Run(name, func(t *testing.T) {
			m, p, k := fixture(t)
			mut(m, &p)
			raw, sig := signed(t, m, k)
			if _, e := VerifyManifest(raw, sig, p); e == nil {
				t.Fatal("policy violation accepted")
			}
		})
	}
	t.Run("stable-valid", func(t *testing.T) {
		m, p, k := fixture(t)
		m["channel"] = "stable"
		p.Channel = "stable"
		m["profile"] = "embedded-production"
		p.Profile = "embedded-production"
		raw, sig := signed(t, m, k)
		if _, e := VerifyManifest(raw, sig, p); e != nil {
			t.Fatal(e)
		}
	})
}
func TestStrictJSON(t *testing.T) {
	m, p, k := fixture(t)
	raw, _ := signed(t, m, k)
	cases := map[string][]byte{
		"duplicate-root":   bytes.Replace(raw, []byte(`"sequence":2`), []byte(`"sequence":2,"sequence":3`), 1),
		"duplicate-nested": bytes.Replace(raw, []byte(`"name":"source.tar.gz"`), []byte(`"name":"source.tar.gz","name":"evil.tar.gz"`), 1),
		"duplicate-file":   bytes.Replace(raw, []byte(`"mode":493`), []byte(`"mode":493,"mode":493`), 1),
		"unknown-root":     append([]byte(`{"extra":1,`), raw[1:]...),
		"unknown-nested":   bytes.Replace(raw, []byte(`"mode":493`), []byte(`"mode":493,"extra":0`), 1),
		"case-alias":       bytes.Replace(raw, []byte(`"sequence"`), []byte(`"Sequence"`), 1),
		"malformed-utf8":   bytes.Replace(raw, []byte("v1.2.3"), []byte{0xff}, 1),
		"trailing":         append(append([]byte(nil), raw...), []byte(` {}`)...),
		"null":             bytes.Replace(raw, []byte(`"minimum_updater":1`), []byte(`"minimum_updater":null`), 1),
		"missing":          bytes.Replace(raw, []byte(`"minimum_updater":1,`), nil, 1),
		"overflow":         bytes.Replace(raw, []byte(`"sequence":2`), []byte(`"sequence":18446744073709551616`), 1),
		"negative":         bytes.Replace(raw, []byte(`"sequence":2`), []byte(`"sequence":-1`), 1),
		"fractional":       bytes.Replace(raw, []byte(`"sequence":2`), []byte(`"sequence":2.0`), 1),
		"oversize":         append(append([]byte(nil), raw...), []byte(strings.Repeat(" ", 1<<20))...),
	}
	for n, b := range cases {
		t.Run(n, func(t *testing.T) {
			if _, e := VerifyManifest(b, ed25519.Sign(k, b), p); e == nil {
				t.Fatal("invalid JSON accepted")
			}
		})
	}
}
func TestSignatureGate(t *testing.T) {
	m, p, k := fixture(t)
	raw, sig := signed(t, m, k)
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	for name, s := range map[string][]byte{"missing": nil, "short": sig[:63], "wrong-key": ed25519.Sign(other, raw), "tampered": ed25519.Sign(k, append(append([]byte(nil), raw...), ' '))} {
		t.Run(name, func(t *testing.T) {
			if _, e := VerifyManifest(raw, s, p); e == nil {
				t.Fatal("unauthenticated bytes accepted")
			}
		})
	}
	p.PublicKey = nil
	if _, e := VerifyManifest(raw, sig, p); e == nil {
		t.Fatal("missing pin accepted")
	}
	// Signature errors must precede JSON parsing, even for malformed input.
	p.PublicKey = k.Public().(ed25519.PublicKey)
	if _, e := VerifyManifest([]byte("{"), sig, p); e != ErrSignature {
		t.Fatalf("parsed before authentication: %v", e)
	}
}
func TestVerifyManifestValid(t *testing.T) {
	m, p, k := fixture(t)
	raw, sig := signed(t, m, k)
	v, e := VerifyManifest(raw, sig, p)
	if e != nil {
		t.Fatal(e)
	}
	if v.Manifest().Sequence != 2 || v.Digest() != digest(raw) {
		t.Fatal("wrong verified identity")
	}
}
