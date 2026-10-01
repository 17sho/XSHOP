package customupgrade

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// Staged describes verified files, not authority to activate them. The helper
// must recheck policy/high-water/current binary and rehash before activation.
type Staged struct {
	Directory, BinaryPath, BinarySHA256, ManifestSHA256 string
	Sequence                                            uint64
}
type stagingRoot interface {
	create(string) (*os.File, error)
	remove(string) error
	check() error
	sync() error
	cleanup() error
	close()
}

// VerifyAndStage creates a NEW 0700 directory under an existing caller-owned,
// non-group/world-writable parent. dest must be canonical and absolute, with
// no symlink components. Linux procfs is needed for anchored failure cleanup.
// Only dujiao-next is created (0755 after all checks). No existing directory or
// file is reused. The manifest authority is snapshotted before reading r.
// Cancellation is checked between bounded reads and before finalization; r must
// itself honor cancellation or an I/O deadline if it can block. Filesystem calls
// are synchronous. No goroutines are used to abandon blocked reads.
// The caller must exclusively own staging ancestors and serialize operations;
// this does not defend against a malicious process with the same UID or root.
// Errors include cleanup failures. A renamed/replaced root is not recursively
// removed; recovery may need to delete the now-empty original directory.
func VerifyAndStage(ctx context.Context, r io.Reader, dest string, v *VerifiedManifest) (result Staged, err error) {
	if ctx == nil || r == nil || v == nil || !v.verified {
		return result, errors.New("customupgrade: invalid staging arguments")
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	snapshot := *v
	m := snapshot.Manifest()
	root, err := newStagingRoot(dest)
	if err != nil {
		return result, err
	}
	defer root.close()
	defer func() {
		if err != nil {
			err = errors.Join(err, root.cleanup())
		}
	}()
	a, err := root.create(".archive")
	if err != nil {
		return result, err
	}
	defer a.Close()
	// An unlinked, open descriptor is the only archive source for extraction.
	if err = root.remove(".archive"); err != nil {
		return result, err
	}
	if err = snapshotArchive(&contextReader{ctx: ctx, r: r}, a, m.Archive); err != nil {
		return result, err
	}
	if _, err = a.Seek(0, io.SeekStart); err != nil {
		return result, err
	}
	f, err := root.create("dujiao-next")
	if err != nil {
		return result, err
	}
	defer f.Close()
	if err = extractBinary(ctx, a, f, m); err != nil {
		return result, err
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	if err = f.Chmod(0755); err != nil {
		return result, err
	}
	if err = f.Sync(); err != nil {
		return result, err
	}
	if err = f.Close(); err != nil {
		return result, err
	}
	if err = root.check(); err != nil {
		return result, err
	}
	if err = root.sync(); err != nil {
		return result, err
	}
	return Staged{Directory: dest, BinaryPath: filepath.Join(dest, "dujiao-next"), BinarySHA256: m.Files[0].SHA256, ManifestSHA256: snapshot.digest, Sequence: m.Sequence}, nil
}
