// Package customupgrade verifies custom release packages. It does not install,
// execute, download, persist high-water state, or provide a rollback bypass.
package customupgrade

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
)

// ErrSignature means the detached signature or pinned key is invalid.
var ErrSignature = errors.New("customupgrade: invalid signature")

// Artifact describes exact bytes of a release asset, not a URL.
type Artifact struct {
	Name   string `json:"name"`
	Size   uint64 `json:"size"`
	SHA256 string `json:"sha256"`
}

// File describes an exact tar member. Mode is a JSON integer (0755 = 493).
type File struct {
	Path   string `json:"path"`
	Size   uint64 `json:"size"`
	SHA256 string `json:"sha256"`
	Mode   uint32 `json:"mode"`
}

// Manifest is the signed version-one wire schema. Sign its exact JSON bytes.
type Manifest struct {
	SchemaVersion     uint64   `json:"schema_version"`
	Product           string   `json:"product"`
	Sequence          uint64   `json:"sequence"`
	Version           string   `json:"version"`
	SourceCommit      string   `json:"source_commit"`
	Channel           string   `json:"channel"`
	Profile           string   `json:"profile"`
	OS                string   `json:"os"`
	Arch              string   `json:"arch"`
	MinimumUpdater    uint64   `json:"minimum_updater"`
	FromBinarySHA256  []string `json:"from_binary_sha256"`
	MigrationPolicy   string   `json:"migration_policy"`
	SchemaFingerprint string   `json:"schema_fingerprint"`
	Archive           Artifact `json:"archive"`
	Source            Artifact `json:"source"`
	Files             []File   `json:"files"`
}

// Policy is trusted local state, never values copied from an unverified manifest.
// The caller must serialize upgrades and durably preserve the high-water mark.
// A zero high-water mark is allowed only for an explicitly trusted bootstrap.
type Policy struct {
	PublicKey                              ed25519.PublicKey
	Channel, Profile, OS, Arch             string
	CurrentBinarySHA256, SchemaFingerprint string
	HighWaterSequence, UpdaterVersion      uint64
}

// VerifiedManifest is an immutable verification capability. Its zero value is
// invalid. Only VerifyManifest can create a usable value.
type VerifiedManifest struct {
	manifest Manifest
	digest   string
	verified bool
}

// Manifest returns a detached copy; edits cannot change verification authority.
func (v *VerifiedManifest) Manifest() Manifest {
	m := v.manifest
	m.FromBinarySHA256 = append([]string(nil), m.FromBinarySHA256...)
	m.Files = append([]File(nil), m.Files...)
	return m
}

// Digest returns the SHA-256 of the exact signed bytes.
func (v *VerifiedManifest) Digest() string { return v.digest }

// VerifyManifest verifies a detached signature and compatibility against policy.
// Length bounds precede authentication; all JSON interpretation follows it. raw,
// signature and PublicKey are copied; callers must not concurrently mutate inputs
// during the call. Subsequent mutation does not alter the returned capability.
// HighWaterSequence is a trusted durable per-channel/profile fence, not a version
// string. This function has no rollback flag and never accepts sequence <= fence.
func VerifyManifest(raw, signature []byte, policy Policy) (*VerifiedManifest, error) {
	return verifyManifest(raw, signature, policy, false)
}

// InstalledIdentity authenticates only an exact durable installed identity. It
// returns no staging capability and cannot authorize replay or installation.
func InstalledIdentity(raw, signature []byte, policy Policy, version, digest, targetHash string) bool {
	v, err := verifyManifest(raw, signature, policy, true)
	if err != nil {
		return false
	}
	m := v.Manifest()
	return m.Sequence == policy.HighWaterSequence && m.Version == version && v.Digest() == digest && m.Files[0].SHA256 == targetHash && targetHash == policy.CurrentBinarySHA256
}
func verifyManifest(raw, signature []byte, policy Policy, installed bool) (*VerifiedManifest, error) {
	if len(raw) == 0 || len(raw) > MaxManifestBytes {
		return nil, errors.New("customupgrade: manifest size")
	}
	if len(policy.PublicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize {
		return nil, ErrSignature
	}
	raw = append([]byte(nil), raw...)
	signature = append([]byte(nil), signature...)
	key := append(ed25519.PublicKey(nil), policy.PublicKey...)
	if !ed25519.Verify(key, raw, signature) {
		return nil, ErrSignature
	}
	if err := strictJSON(raw); err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	if err := validateManifestPolicy(m, policy, installed); err != nil {
		return nil, err
	}
	h := sha256.Sum256(raw)
	return &VerifiedManifest{manifest: m, digest: hex.EncodeToString(h[:]), verified: true}, nil
}
