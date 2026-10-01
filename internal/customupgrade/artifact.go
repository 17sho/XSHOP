package customupgrade

import (
	"context"
	"errors"
	"io"
)

// ReleaseAssetURL constructs a URL in the fixed public distribution repository.
// tag and asset are ASCII basenames, never URLs or paths. This performs no I/O
// and grants no verification authority. For a verified release use its Version
// as tag, and Archive.Name / Source.Name as asset. Redirect/network policy must
// be enforced separately by a future fetcher.
func ReleaseAssetURL(tag, asset string) (string, error) {
	if !safeName(tag) || !safeName(asset) {
		return "", errors.New("customupgrade: invalid release tag/asset")
	}
	return "https://github.com/17sho/XSHOP/releases/download/" + tag + "/" + asset, nil
}

// VerifySource checks exact source asset size and SHA-256 without extracting it.
// It does not audit GPL completeness or source contents. Reader deadline and
// cancellation requirements are the same as VerifyAndStage.
func VerifySource(ctx context.Context, r io.Reader, v *VerifiedManifest) error {
	if ctx == nil || r == nil || v == nil || !v.verified {
		return errors.New("customupgrade: invalid source arguments")
	}
	a := v.manifest.Source
	return snapshotArchive(&contextReader{ctx: ctx, r: r}, io.Discard, a)
}
