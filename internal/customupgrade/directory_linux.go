//go:build linux

package customupgrade

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type directory struct {
	parent, root *os.File
	dest, base   string
}

// openParent walks every component with O_NOFOLLOW. A directory descriptor is
// held throughout extraction, so later path replacement cannot redirect writes.
func openParent(dest string) (*os.File, error) {
	if !filepath.IsAbs(dest) || filepath.Clean(dest) != dest || dest == "/" {
		return nil, fmt.Errorf("customupgrade: destination must be canonical absolute path")
	}
	fd, err := syscall.Open("/", syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	parent := filepath.Dir(dest)
	if parent != "/" {
		for _, part := range strings.Split(strings.TrimPrefix(parent, "/"), "/") {
			next, e := syscall.Openat(fd, part, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
			syscall.Close(fd)
			if e != nil {
				return nil, e
			}
			fd = next
		}
	}
	f := os.NewFile(uintptr(fd), parent)
	var st syscall.Stat_t
	if err = syscall.Fstat(fd, &st); err != nil {
		f.Close()
		return nil, err
	}
	if st.Uid != uint32(os.Geteuid()) || st.Mode&0022 != 0 {
		f.Close()
		return nil, fmt.Errorf("customupgrade: parent must be owned and not group/world writable")
	}
	return f, nil
}
func newStagingRoot(dest string) (stagingRoot, error) {
	parent, err := openParent(dest)
	if err != nil {
		return nil, err
	}
	base := filepath.Base(dest)
	if err = syscall.Mkdirat(int(parent.Fd()), base, 0700); err != nil {
		parent.Close()
		return nil, err
	}
	fd, err := syscall.Openat(int(parent.Fd()), base, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		parent.Close()
		return nil, err
	}
	d := &directory{parent: parent, root: os.NewFile(uintptr(fd), dest), dest: dest, base: base}
	if err = d.root.Chmod(0700); err != nil {
		d.cleanup()
		d.close()
		return nil, err
	}
	return d, nil
}
func (d *directory) create(name string) (*os.File, error) {
	fd, err := syscall.Openat(int(d.root.Fd()), name, syscall.O_RDWR|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}
func (d *directory) remove(name string) error { return syscall.Unlinkat(int(d.root.Fd()), name) }
func (d *directory) check() error {
	p, err := openParent(d.dest)
	if err != nil {
		return err
	}
	defer p.Close()
	before, err := d.parent.Stat()
	if err != nil {
		return err
	}
	after, err := p.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(before, after) {
		return fmt.Errorf("customupgrade: staging parent replaced")
	}
	fd, err := syscall.Openat(int(p.Fd()), d.base, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), d.base)
	defer f.Close()
	before, err = d.root.Stat()
	if err != nil {
		return err
	}
	after, err = f.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(before, after) {
		return fmt.Errorf("customupgrade: staging root replaced")
	}
	return nil
}
func (d *directory) sync() error { return errors.Join(d.root.Sync(), d.parent.Sync()) }
func (d *directory) cleanup() error {
	var errs []error
	for _, name := range []string{".archive", "dujiao-next"} {
		if err := d.remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	// Never recurse or remove a substituted path. Linux procfs keeps this removal
	// relative to the held parent fd. A renamed root is left empty for recovery.
	if err := d.check(); err != nil {
		return errors.Join(append(errs, err)...)
	}
	if err := os.Remove(fmt.Sprintf("/proc/self/fd/%d/%s", d.parent.Fd(), d.base)); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
func (d *directory) close() { d.root.Close(); d.parent.Close() }
