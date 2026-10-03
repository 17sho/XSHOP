package customupgrade

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const (
	MaxArchiveBytes   uint64 = 512 << 20
	MaxSourceBytes    uint64 = 1 << 30
	MaxMemberBytes    uint64 = 1 << 30
	MaxMembers               = 1
	MaxExpansionRatio uint64 = 200
)

// ErrUnsupportedProfile explicitly includes the reserved split-production profile.
var ErrUnsupportedProfile = errors.New("customupgrade: unsupported profile")
var safeToken = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
var lowerHash = regexp.MustCompile(`^[0-9a-f]{64}$`)
var lowerCommit = regexp.MustCompile(`^[0-9a-f]{40}$`)

func safeName(s string) bool { return safeToken.MatchString(s) && !strings.Contains(s, "..") }
func validateArtifact(a Artifact, max uint64) bool {
	return safeName(a.Name) && strings.HasSuffix(a.Name, ".tar.gz") && a.Size > 0 && a.Size <= max && lowerHash.MatchString(a.SHA256)
}
func validatePolicy(m Manifest, p Policy) error { return validateManifestPolicy(m, p, false) }
func validateManifestPolicy(m Manifest, p Policy, installed bool) error {
	if m.Profile != p.Profile || !((p.Profile == "embedded-preview" && p.Channel == "preview") || (p.Profile == "embedded-production" && p.Channel == "stable")) {
		return ErrUnsupportedProfile
	}
	if m.SchemaVersion != 1 || m.Product != "XSHOP" || m.Sequence == 0 || (!installed && m.Sequence <= p.HighWaterSequence) {
		return fmt.Errorf("customupgrade: schema/product/sequence rejected")
	}
	if !safeName(m.Version) || !lowerCommit.MatchString(m.SourceCommit) {
		return fmt.Errorf("customupgrade: release identity rejected")
	}
	if (p.Channel != "preview" && p.Channel != "stable") || m.Channel != p.Channel || m.OS != "linux" || p.OS != "linux" || m.Arch != "amd64" || p.Arch != "amd64" {
		return fmt.Errorf("customupgrade: channel/platform rejected")
	}
	if m.MinimumUpdater == 0 || p.UpdaterVersion == 0 || m.MinimumUpdater > p.UpdaterVersion {
		return fmt.Errorf("customupgrade: updater version rejected")
	}
	if m.MigrationPolicy != "unchanged" || !lowerHash.MatchString(m.SchemaFingerprint) || m.SchemaFingerprint != p.SchemaFingerprint || !lowerHash.MatchString(p.CurrentBinarySHA256) {
		return fmt.Errorf("customupgrade: compatibility rejected")
	}
	if len(m.FromBinarySHA256) == 0 || len(m.FromBinarySHA256) > 64 {
		return fmt.Errorf("customupgrade: baseline list rejected")
	}
	seen := make(map[string]bool)
	matched := false
	for _, h := range m.FromBinarySHA256 {
		if !lowerHash.MatchString(h) || seen[h] {
			return fmt.Errorf("customupgrade: baseline hash rejected")
		}
		seen[h] = true
		matched = matched || h == p.CurrentBinarySHA256
	}
	if !matched && !installed {
		return fmt.Errorf("customupgrade: current binary not compatible")
	}
	if !validateArtifact(m.Archive, MaxArchiveBytes) || !validateArtifact(m.Source, MaxSourceBytes) {
		return fmt.Errorf("customupgrade: artifact rejected")
	}
	if m.Archive.Name == m.Source.Name || len(m.Files) != MaxMembers {
		return fmt.Errorf("customupgrade: member set rejected")
	}
	f := m.Files[0]
	// Multiplication is safe: archive size is already capped at 512 MiB.
	if f.Path != "dujiao-next" || f.Mode != 0755 || f.Size == 0 || f.Size > MaxMemberBytes || f.Size > m.Archive.Size*MaxExpansionRatio || !lowerHash.MatchString(f.SHA256) {
		return fmt.Errorf("customupgrade: binary member rejected")
	}
	return nil
}
