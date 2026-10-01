package customupgrade

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tarBytes(t *testing.T, headers []*tar.Header, contents [][]byte) []byte {
	t.Helper()
	var b bytes.Buffer
	g := gzip.NewWriter(&b)
	w := tar.NewWriter(g)
	for i, h := range headers {
		if e := w.WriteHeader(h); e != nil {
			t.Fatal(e)
		}
		if len(contents[i]) > 0 {
			if _, e := w.Write(contents[i]); e != nil {
				t.Fatal(e)
			}
		}
	}
	if e := w.Close(); e != nil {
		t.Fatal(e)
	}
	if e := g.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func regular(data []byte) *tar.Header {
	return &tar.Header{Name: "dujiao-next", Size: int64(len(data)), Mode: 0755, Typeflag: tar.TypeReg, Format: tar.FormatUSTAR}
}
func verifiedArchive(t *testing.T, archive, data []byte) *VerifiedManifest {
	t.Helper()
	m, p, k := fixture(t)
	m["archive"].(map[string]any)["size"] = len(archive)
	m["archive"].(map[string]any)["sha256"] = digest(archive)
	f := m["files"].([]any)[0].(map[string]any)
	f["size"] = len(data)
	f["sha256"] = digest(data)
	b, s := signed(t, m, k)
	v, e := VerifyManifest(b, s, p)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func TestStageCancellationAndInvalidAuthority(t *testing.T) {
	data := []byte("fixture binary")
	archive := tarBytes(t, []*tar.Header{regular(data)}, [][]byte{data})
	v := verifiedArchive(t, archive, data)
	for name, authority := range map[string]*VerifiedManifest{"nil": nil, "zero": {}} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("invalid authority panicked: %v", r)
				}
			}()
			rejectStage(t, archive, authority)
		})
	}
	t.Run("pre-cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		r := &countReader{Reader: bytes.NewReader(archive)}
		dest := filepath.Join(t.TempDir(), "stage")
		_, e := VerifyAndStage(ctx, r, dest, v)
		if !errors.Is(e, context.Canceled) || r.n != 0 {
			t.Fatalf("not canceled before read: %v bytes=%d", e, r.n)
		}
		if _, e = os.Stat(dest); !errors.Is(e, os.ErrNotExist) {
			t.Fatal("canceled operation created root")
		}
	})
	t.Run("mid-read", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		r := &onRead{Reader: bytes.NewReader(archive), fn: cancel}
		dest := filepath.Join(t.TempDir(), "stage")
		_, e := VerifyAndStage(ctx, r, dest, v)
		if !errors.Is(e, context.Canceled) {
			t.Fatalf("not canceled: %v", e)
		}
		if _, e = os.Stat(dest); !errors.Is(e, os.ErrNotExist) {
			t.Fatal("cancellation leaked staging")
		}
	})
	t.Run("nonprogress", func(t *testing.T) {
		r := &zeroReader{}
		_, e := VerifyAndStage(context.Background(), r, filepath.Join(t.TempDir(), "stage"), v)
		if !errors.Is(e, io.ErrNoProgress) || r.n > 100 {
			t.Fatalf("unbounded empty reads: %v %d", e, r.n)
		}
	})
	t.Run("bounded-download", func(t *testing.T) {
		r := &countReader{Reader: io.MultiReader(bytes.NewReader(archive), strings.NewReader(strings.Repeat("x", 10000)))}
		_, e := VerifyAndStage(context.Background(), r, filepath.Join(t.TempDir(), "stage"), v)
		if e == nil || r.n > len(archive)+1 {
			t.Fatalf("unbounded download: %v %d", e, r.n)
		}
	})
}

type countReader struct {
	io.Reader
	n int
}

func (r *countReader) Read(p []byte) (int, error) { n, e := r.Reader.Read(p); r.n += n; return n, e }

type zeroReader struct{ n int }

func (r *zeroReader) Read([]byte) (int, error) {
	r.n++
	if r.n > 100 {
		return 0, errors.New("test safety limit")
	}
	return 0, nil
}
func TestStageFilesystemBoundary(t *testing.T) {
	data := []byte("fixture binary")
	archive := tarBytes(t, []*tar.Header{regular(data)}, [][]byte{data})
	v := verifiedArchive(t, archive, data)
	t.Run("symlink-parent", func(t *testing.T) {
		base := t.TempDir()
		outside := t.TempDir()
		link := filepath.Join(base, "link")
		if e := os.Symlink(outside, link); e != nil {
			t.Fatal(e)
		}
		if _, e := VerifyAndStage(context.Background(), bytes.NewReader(archive), filepath.Join(link, "stage"), v); e == nil {
			t.Fatal("symlink parent accepted")
		}
		entries, _ := os.ReadDir(outside)
		if len(entries) != 0 {
			t.Fatal("wrote through parent symlink")
		}
	})
	t.Run("existing", func(t *testing.T) {
		dest := t.TempDir()
		sentinel := filepath.Join(dest, "sentinel")
		os.WriteFile(sentinel, []byte("keep"), 0600)
		if _, e := VerifyAndStage(context.Background(), bytes.NewReader(archive), dest, v); e == nil {
			t.Fatal("existing directory accepted")
		}
		b, e := os.ReadFile(sentinel)
		if e != nil || string(b) != "keep" {
			t.Fatal("existing data changed")
		}
	})
	t.Run("unsafe-parent-permissions", func(t *testing.T) {
		base := t.TempDir()
		os.Chmod(base, 0777)
		if _, e := VerifyAndStage(context.Background(), bytes.NewReader(archive), filepath.Join(base, "stage"), v); e == nil {
			t.Fatal("writable parent accepted")
		}
	})
	t.Run("unclean-destination", func(t *testing.T) {
		base := t.TempDir()
		if _, e := VerifyAndStage(context.Background(), bytes.NewReader(archive), base+"/./stage", v); e == nil {
			t.Fatal("noncanonical destination accepted")
		}
	})
	t.Run("swapped-destination", func(t *testing.T) {
		base := t.TempDir()
		dest := filepath.Join(base, "stage")
		outside := t.TempDir()
		r := &onRead{Reader: bytes.NewReader(archive), fn: func() {
			if e := os.Rename(dest, dest+"-moved"); e != nil {
				t.Fatal(e)
			}
			if e := os.Symlink(outside, dest); e != nil {
				t.Fatal(e)
			}
		}}
		if _, e := VerifyAndStage(context.Background(), r, dest, v); e == nil {
			t.Fatal("replaced destination accepted")
		}
		entries, _ := os.ReadDir(outside)
		if len(entries) != 0 {
			t.Fatal("wrote outside owned root")
		}
	})
}

type onRead struct {
	io.Reader
	fn func()
}

func (r *onRead) Read(p []byte) (int, error) {
	if r.fn != nil {
		fn := r.fn
		r.fn = nil
		fn()
	}
	return r.Reader.Read(p)
}
func TestStageSnapshot(t *testing.T) {
	data := []byte("fixture binary")
	archive := tarBytes(t, []*tar.Header{regular(data)}, [][]byte{data})
	v := verifiedArchive(t, archive, data)
	clone := v.Manifest()
	clone.Files[0].SHA256 = digest(nil)
	clone.FromBinarySHA256[0] = digest(nil)
	if v.Manifest().Files[0].SHA256 != digest(data) {
		t.Fatal("manifest aliases authority")
	}
	originalDigest := v.Digest()
	dest := filepath.Join(t.TempDir(), "stage")
	r := &onRead{Reader: bytes.NewReader(archive), fn: func() { *v = VerifiedManifest{} }}
	result, e := VerifyAndStage(context.Background(), r, dest, v)
	if e != nil {
		t.Fatal(e)
	}
	if result.ManifestSHA256 != originalDigest {
		t.Fatal("authority changed after read began")
	}
}
func TestArchiveRejection(t *testing.T) {
	data := []byte("fixture embedded binary")
	good := tarBytes(t, []*tar.Header{regular(data)}, [][]byte{data})
	cases := map[string][]byte{"extra-download-byte": append(append([]byte(nil), good...), 0), "short-download": good[:len(good)-1], "bad-download-hash": append([]byte(nil), good...)}
	cases["bad-download-hash"][20] ^= 1
	for n, b := range cases {
		t.Run(n, func(t *testing.T) { rejectStage(t, b, verifiedArchive(t, good, data)) })
	}
	for _, path := range []string{"../dujiao-next", "./dujiao-next", "/dujiao-next", "x/../dujiao-next", "//dujiao-next", "config.yml", "uploads/x", "db/x", "dujiao-next\\x"} {
		t.Run("path-"+path, func(t *testing.T) {
			h := regular(data)
			h.Name = path
			b := tarBytes(t, []*tar.Header{h}, [][]byte{data})
			rejectStage(t, b, verifiedArchive(t, b, data))
		})
	}
	for _, typ := range []byte{tar.TypeSymlink, tar.TypeLink, tar.TypeDir, tar.TypeChar, tar.TypeBlock, tar.TypeFifo} {
		t.Run("type-"+string(typ), func(t *testing.T) {
			h := regular(nil)
			h.Typeflag = typ
			if typ == tar.TypeSymlink || typ == tar.TypeLink {
				h.Linkname = "outside"
			}
			b := tarBytes(t, []*tar.Header{h}, [][]byte{nil})
			rejectStage(t, b, verifiedArchive(t, b, data))
		})
	}
	t.Run("duplicate", func(t *testing.T) {
		b := tarBytes(t, []*tar.Header{regular(data), regular(data)}, [][]byte{data, data})
		rejectStage(t, b, verifiedArchive(t, b, data))
	})
	t.Run("extra", func(t *testing.T) {
		h := regular(data)
		h.Name = "config.yml"
		b := tarBytes(t, []*tar.Header{regular(data), h}, [][]byte{data, data})
		rejectStage(t, b, verifiedArchive(t, b, data))
	})
	t.Run("missing", func(t *testing.T) { b := tarBytes(t, nil, nil); rejectStage(t, b, verifiedArchive(t, b, data)) })
	t.Run("mode", func(t *testing.T) {
		h := regular(data)
		h.Mode = 04755
		b := tarBytes(t, []*tar.Header{h}, [][]byte{data})
		rejectStage(t, b, verifiedArchive(t, b, data))
	})
	t.Run("size", func(t *testing.T) {
		b := tarBytes(t, []*tar.Header{regular(data[:5])}, [][]byte{data[:5]})
		rejectStage(t, b, verifiedArchive(t, b, data))
	})
	t.Run("member-hash", func(t *testing.T) {
		bad := bytes.Repeat([]byte("x"), len(data))
		b := tarBytes(t, []*tar.Header{regular(bad)}, [][]byte{bad})
		rejectStage(t, b, verifiedArchive(t, b, data))
	})
	t.Run("pax", func(t *testing.T) {
		h := regular(data)
		h.Format = tar.FormatPAX
		h.PAXRecords = map[string]string{"comment": "extra"}
		b := tarBytes(t, []*tar.Header{h}, [][]byte{data})
		rejectStage(t, b, verifiedArchive(t, b, data))
	})
	t.Run("multistream", func(t *testing.T) {
		b := append(append([]byte(nil), good...), good...)
		rejectStage(t, b, verifiedArchive(t, b, data))
	})
	t.Run("gzip-checksum", func(t *testing.T) {
		b := append([]byte(nil), good...)
		b[len(b)-8] ^= 1
		rejectStage(t, b, verifiedArchive(t, b, data))
	})
	t.Run("decompression-bomb", func(t *testing.T) {
		g, e := gzip.NewReader(bytes.NewReader(good))
		if e != nil {
			t.Fatal(e)
		}
		plain, e := io.ReadAll(g)
		if e != nil {
			t.Fatal(e)
		}
		var b bytes.Buffer
		w := gzip.NewWriter(&b)
		w.Write(plain)
		w.Write([]byte(strings.Repeat("\x00", 1<<20)))
		w.Close()
		rejectStage(t, b.Bytes(), verifiedArchive(t, b.Bytes(), data))
	})
}
func rejectStage(t *testing.T, b []byte, v *VerifiedManifest) {
	t.Helper()
	dest := filepath.Join(t.TempDir(), "stage")
	if _, e := VerifyAndStage(context.Background(), bytes.NewReader(b), dest, v); e == nil {
		t.Fatal("unsafe archive accepted")
	}
	if _, e := os.Stat(dest); !errors.Is(e, os.ErrNotExist) {
		t.Fatalf("failed staging leaked files: %v", e)
	}
}
func TestVerifyAndStageValid(t *testing.T) {
	data := []byte("fixture embedded binary")
	archive := tarBytes(t, []*tar.Header{regular(data)}, [][]byte{data})
	v := verifiedArchive(t, archive, data)
	dest := filepath.Join(t.TempDir(), "new-stage")
	s, e := VerifyAndStage(context.Background(), bytes.NewReader(archive), dest, v)
	if e != nil {
		t.Fatal(e)
	}
	got, e := os.ReadFile(filepath.Join(dest, "dujiao-next"))
	if e != nil {
		t.Fatal(e)
	}
	st, e := os.Stat(s.BinaryPath)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(got, data) || st.Mode().Perm() != 0755 || s.Directory != dest || s.ManifestSHA256 != v.Digest() || s.BinarySHA256 != digest(data) || s.Sequence != 2 {
		t.Fatal("staged result mismatch")
	}
	entries, e := os.ReadDir(dest)
	if e != nil || len(entries) != 1 {
		t.Fatalf("unexpected staging contents: %v %v", entries, e)
	}
}
