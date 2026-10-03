package xshopupgrade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dujiao-next/internal/customupgrade"
	"sort"
	"strings"
)

// Five bounded pages; fail closed on truncation rather than silently select stale releases.
func productionCandidates(ctx context.Context, fetch func(context.Context, string) ([]byte, error)) ([]string, error) {
	tags := map[string]bool{}
	for page := 1; page <= 5; page++ {
		raw, err := fetch(ctx, fmt.Sprintf("repos/%s/releases?per_page=100&page=%d", Repository, page))
		if err != nil {
			return nil, err
		}
		if len(raw) > 2<<20 {
			return nil, errors.New("release list limit")
		}
		var rs []release
		if err = json.Unmarshal(raw, &rs); err != nil {
			return nil, err
		}
		if len(rs) > 100 {
			return nil, errors.New("release page limit")
		}
		for _, r := range rs {
			if !r.Draft && !r.Prerelease && strings.HasPrefix(r.Tag, "xshop-production-") && ValidAsset(r.Tag) {
				tags[r.Tag] = true
			}
		}
		if len(rs) < 100 {
			out := make([]string, 0, len(tags))
			for tag := range tags {
				out = append(out, tag)
			}
			sort.Strings(out)
			return out, nil
		}
	}
	return nil, errors.New("release pagination cap reached")
}
func (GitHub) ProductionCandidates(ctx context.Context) ([]string, error) {
	return productionCandidates(ctx, func(ctx context.Context, endpoint string) ([]byte, error) {
		return boundedCommand(ctx, 2<<20, "api", endpoint)
	})
}

// Sequence is trusted only after signature/profile/platform/schema/from-hash validation.
func highestVerified(tags []string, verify func(string) (uint64, error)) (string, error) {
	best := ""
	var high uint64
	for _, tag := range tags {
		n, err := verify(tag)
		if err != nil || n == 0 {
			continue
		}
		if n > high || (n == high && (best == "" || tag < best)) {
			best = tag
			high = n
		}
	}
	if best == "" {
		return "", errors.New("no compatible authenticated production release")
	}
	return best, nil
}
func (h *Helper) productionLatest(ctx context.Context) (string, error) {
	source, ok := h.Source.(interface {
		ProductionCandidates(context.Context) ([]string, error)
	})
	if !ok {
		return "", errors.New("production candidate source required")
	}
	tags, err := source.ProductionCandidates(ctx)
	if err != nil {
		return "", err
	}
	return highestVerified(tags, func(tag string) (uint64, error) {
		if !ValidTag(tag) {
			return 0, errors.New("tag rejected")
		}
		if v, err := h.verify(ctx, tag); err == nil {
			return v.Manifest().Sequence, nil
		}
		// Current installed identity may be selected as up-to-date, but not replayed.
		s, err := h.Engine.ReadState()
		if err != nil || s.Pending || s.Result != "installed" || s.Version != tag {
			return 0, errors.New("not installed")
		}
		raw, err := h.Source.Asset(ctx, tag, "manifest.json", customupgrade.MaxManifestBytes)
		if err != nil {
			return 0, err
		}
		sig, err := h.Source.Asset(ctx, tag, "manifest.sig", 64)
		if err != nil {
			return 0, err
		}
		p, err := h.policy()
		if err != nil || !customupgrade.InstalledIdentity(raw, sig, p, s.Version, s.Digest, s.NewHash) {
			return 0, errors.New("installed identity rejected")
		}
		return s.HighWater, nil
	})
}
