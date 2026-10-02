package xshopupgrade

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// GitHub uses only root's existing gh credential backend. No token is exported,
// copied into packages, accepted from callers, or returned to the application.
type GitHub struct{}

func githubCommand(ctx context.Context, args ...string) *exec.Cmd {
	// All callers are fixed internal GitHub API operations. Use only the existing
	// root backend, never inherited caller host/config/token or interactive tools.
	pinned := []string{"api", "--hostname", "github.com"}
	pinned = append(pinned, args[1:]...)
	cmd := exec.CommandContext(ctx, "/usr/bin/gh", pinned...)
	cmd.Env = []string{"HOME=/root", "PATH=/usr/bin:/bin", "GH_HOST=github.com", "GH_CONFIG_DIR=/root/.config/gh", "GH_PROMPT_DISABLED=1", "GH_PAGER=cat", "GIT_TERMINAL_PROMPT=0", "LANG=C.UTF-8"}
	return cmd
}

func boundedCommand(ctx context.Context, max uint64, args ...string) ([]byte, error) {
	cmd := githubCommand(ctx, args...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	} // Discard stderr: upstream errors can contain privileged response details.
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	raw, readErr := io.ReadAll(io.LimitReader(out, int64(max)+1))
	if readErr != nil || uint64(len(raw)) > max {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, errors.New("download limit")
	}
	if err = cmd.Wait(); err != nil {
		return nil, errors.New("private GitHub download failed")
	}
	return raw, nil
}

type release struct {
	Tag        string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		ID   uint64 `json:"id"`
		Name string `json:"name"`
		Size uint64 `json:"size"`
	} `json:"assets"`
}

func validateReleaseIdentity(r release, tag string) error {
	if r.Draft || !ValidTag(r.Tag) || (Production && r.Prerelease) || (tag != "" && r.Tag != tag) {
		return errors.New("release identity rejected")
	}
	return nil
}

func getRelease(ctx context.Context, endpoint string) (release, error) {
	var r release
	raw, err := boundedCommand(ctx, 128<<10, "api", "repos/"+Repository+"/releases/"+endpoint)
	if err != nil {
		return r, err
	}
	err = json.Unmarshal(raw, &r)
	if err == nil {
		tag := ""
		if strings.HasPrefix(endpoint, "tags/") {
			tag = strings.TrimPrefix(endpoint, "tags/")
		}
		err = validateReleaseIdentity(r, tag)
	}
	return r, err
}
func (GitHub) Latest(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if Production {
		return "", errors.New("production requires authenticated bounded selection")
	}
	r, err := getRelease(ctx, "latest")
	return r.Tag, err
}
func (GitHub) Asset(ctx context.Context, tag, name string, max uint64) ([]byte, error) {
	if !ValidTag(tag) || !ValidAsset(name) || max == 0 || max > 128<<20 {
		return nil, errors.New("asset request rejected")
	}
	ctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	r, err := getRelease(ctx, "tags/"+tag)
	if err != nil {
		return nil, err
	}
	id := uint64(0)
	for _, a := range r.Assets {
		if a.Name == name {
			if id != 0 || a.ID == 0 || a.Size == 0 || a.Size > max {
				return nil, errors.New("asset size/identity rejected")
			}
			id = a.ID
		}
	}
	if id == 0 {
		return nil, errors.New("asset missing")
	}
	return boundedCommand(ctx, max, "api", "repos/"+Repository+"/releases/assets/"+strconv.FormatUint(id, 10), "-H", "Accept: application/octet-stream")
}

// No package may redirect the helper to a different repository or command.
var _ Source = GitHub{}
var _ = strings.TrimSpace
