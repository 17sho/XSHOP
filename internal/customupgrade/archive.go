package customupgrade

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
)

// extractBinary accepts one USTAR regular member, zero padding, exactly two
// end blocks and one gzip stream. PAX/GNU metadata and appended data are not
// interpreted; they are rejected before archive/tar can hide extra entries.
func extractBinary(ctx context.Context, r io.Reader, w io.Writer, m Manifest) error {
	br := bufio.NewReader(&contextReader{ctx: ctx, r: r})
	gz, err := gzip.NewReader(br)
	if err != nil {
		return err
	}
	defer gz.Close()
	gz.Multistream(false)
	g := &contextReader{ctx: ctx, r: gz}
	f := m.Files[0]
	padding := (512 - f.Size%512) % 512
	total := uint64(512) + f.Size + padding + 1024
	if total > m.Archive.Size*MaxExpansionRatio {
		return fmt.Errorf("customupgrade: expansion ratio")
	}
	header := make([]byte, 512)
	if _, err = io.ReadFull(g, header); err != nil {
		return err
	}
	if header[156] != tar.TypeReg && header[156] != tar.TypeRegA {
		return fmt.Errorf("customupgrade: nonregular tar header")
	}
	h, err := tar.NewReader(bytes.NewReader(header)).Next()
	if err != nil {
		return err
	}
	if h.Format != tar.FormatUSTAR || h.Name != f.Path || h.Linkname != "" || h.Mode != 0755 || h.Size != int64(f.Size) || h.Devmajor != 0 || h.Devminor != 0 {
		return fmt.Errorf("customupgrade: tar metadata mismatch")
	}
	hash := sha256.New()
	if _, err = io.CopyN(io.MultiWriter(w, hash), g, int64(f.Size)); err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != f.SHA256 {
		return fmt.Errorf("customupgrade: member digest mismatch")
	}
	trailer := make([]byte, int(padding)+1024)
	if _, err = io.ReadFull(g, trailer); err != nil {
		return err
	}
	for _, b := range trailer {
		if b != 0 {
			return fmt.Errorf("customupgrade: extra member or nonzero tar padding")
		}
	}
	var extra [1]byte
	if n, err := g.Read(extra[:]); n != 0 || err != io.EOF {
		return fmt.Errorf("customupgrade: gzip ending: %w", err)
	}
	if _, err := br.ReadByte(); err != io.EOF {
		return fmt.Errorf("customupgrade: trailing compressed bytes")
	}
	return nil
}

func snapshotArchive(r io.Reader, w io.Writer, a Artifact) error {
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(w, hash), io.LimitReader(r, int64(a.Size)+1))
	if err != nil {
		return err
	}
	if uint64(n) != a.Size || hex.EncodeToString(hash.Sum(nil)) != a.SHA256 {
		return fmt.Errorf("customupgrade: archive size/digest mismatch")
	}
	return nil
}
