package xshopupgrade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const PreviewService = "dujiao-preview.service"
const PreviewRoot = "/opt/dujiao-preview"
const StateRoot = "/var/lib/xshop-preview-upgrader"

type Systemd struct{ HealthURL string }

func systemctl(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 100*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/systemctl", args...)
	raw, err := cmd.Output()
	return strings.TrimSpace(string(raw)), err
}
func (Systemd) Stop(ctx context.Context) error {
	_, err := systemctl(ctx, "stop", PreviewService)
	if err != nil {
		return err
	}
	pid, err := systemctl(ctx, "show", PreviewService, "-p", "MainPID", "--value")
	if err != nil || pid != "0" {
		return errors.New("writers not stopped")
	}
	group, err := systemctl(ctx, "show", PreviewService, "-p", "ControlGroup", "--value")
	if err != nil {
		return err
	}
	if group != "" {
		return filepath.WalkDir(filepath.Join("/sys/fs/cgroup", group), func(p string, d os.DirEntry, err error) error {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			if err != nil {
				return err
			}
			if d.Name() == "cgroup.procs" {
				raw, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				if strings.TrimSpace(string(raw)) != "" {
					return errors.New("writers remain")
				}
			}
			return nil
		})
	}
	return nil
}
func (Systemd) Start(ctx context.Context) error {
	_, err := systemctl(ctx, "start", PreviewService)
	return err
}
func (s Systemd) Healthy(ctx context.Context, expected string) error {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 2 * time.Second}
	for {
		pid, err := systemctl(ctx, "show", PreviewService, "-p", "MainPID", "--value")
		if err == nil && pid != "0" {
			hash, err := FileHash("/proc/" + pid + "/exe")
			if err == nil && hash == expected {
				req, _ := http.NewRequestWithContext(ctx, "GET", s.HealthURL, nil)
				resp, err := client.Do(req)
				if err == nil {
					raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 4097))
					resp.Body.Close()
					var health struct {
						Status string `json:"status"`
					}
					if readErr == nil && len(raw) <= 4096 && resp.StatusCode == 200 && json.Unmarshal(raw, &health) == nil && health.Status == "ok" {
						return nil
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return errors.New("running binary/JSON health failed")
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// Backup factory is only constructed from root-owned operator policy, never HTTP.
func PreviewBackup(database string) func(context.Context, string) error {
	return func(ctx context.Context, dir string) error {
		if err := regular(database); err != nil {
			return err
		}
		if err := atomicCopy(filepath.Join(PreviewRoot, "config.yml"), filepath.Join(dir, "config.yml"), 0600); err != nil {
			return err
		}
		// SQLite backup is a coherent checkpoint; it is never used for automatic rollback.
		script := `import sqlite3,sys,os
src=sqlite3.connect('file:'+sys.argv[1]+'?mode=ro',uri=True)
dst=sqlite3.connect(sys.argv[2]);src.backup(dst);dst.close();src.close();os.chmod(sys.argv[2],0o600)
`
		cmd := exec.CommandContext(ctx, "/usr/bin/python3", "-c", script, database, filepath.Join(dir, "database.sqlite3"))
		cmd.Stderr = io.Discard
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("SQLite backup failed: %w", err)
		}
		f, err := os.Open(filepath.Join(dir, "database.sqlite3"))
		if err != nil {
			return err
		}
		defer f.Close()
		return f.Sync()
	}
}

type OperatorPolicy struct {
	PublicKey         string `json:"public_key"`
	Schema            string `json:"schema_fingerprint"`
	Database          string `json:"database"`
	Port              uint16 `json:"port"`
	BootstrapSequence uint64 `json:"bootstrap_sequence"`
}

func ValidateOperatorPolicy(p OperatorPolicy) error {
	if _, err := ParseKey(p.PublicKey); err != nil {
		return err
	}
	if !digestPattern.MatchString(p.Schema) || p.BootstrapSequence != 1 || p.Port == 0 {
		return errors.New("policy rejected")
	}
	if filepath.Dir(p.Database) != filepath.Join(PreviewRoot, "db") || filepath.Clean(p.Database) != p.Database || !ValidAsset(filepath.Base(p.Database)) {
		return errors.New("database must be exact preview db file")
	}
	return nil
}
func PolicyHealthURL(p OperatorPolicy) string {
	return "http://127.0.0.1:" + strconv.Itoa(int(p.Port)) + "/health"
}
