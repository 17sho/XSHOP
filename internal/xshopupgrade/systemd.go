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
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
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

const previewCgroup = "/system.slice/" + PreviewService

type previewIdentity struct {
	PID   int    `json:"pid"`
	UID   uint32 `json:"uid"`
	GID   uint32 `json:"gid"`
	Start string `json:"start"`
}

func previewCredentials() (uint32, uint32, error) {
	u, err := user.Lookup("dujiao")
	if err != nil {
		return 0, 0, err
	}
	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil || uid == 0 {
		return 0, 0, errors.New("invalid preview UID")
	}
	gid, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil || gid == 0 {
		return 0, 0, errors.New("invalid preview GID")
	}
	return uint32(uid), uint32(gid), nil
}

// Only the fixed unit supplies the PID. Neither HTTP nor operator policy can
// supply a process, cgroup, account, executable, or command argument.
func fixedPreviewPID(ctx context.Context) (string, error) {
	raw, err := systemctl(ctx, "show", PreviewService, "-p", "MainPID", "-p", "ControlGroup")
	if err != nil {
		return "", err
	}
	fields := make(map[string]string)
	for _, line := range strings.Split(raw, "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok || (k != "MainPID" && k != "ControlGroup") {
			return "", errors.New("invalid service identity")
		}
		if _, exists := fields[k]; exists {
			return "", errors.New("duplicate service identity")
		}
		fields[k] = v
	}
	if fields["ControlGroup"] != previewCgroup {
		return "", errors.New("wrong preview cgroup")
	}
	return fields["MainPID"], nil
}

func readPreviewIdentity(pid string, uid, gid uint32) (previewIdentity, error) {
	n, err := strconv.Atoi(pid)
	if err != nil || n <= 1 || strconv.Itoa(n) != pid {
		return previewIdentity{}, errors.New("invalid preview PID")
	}
	base := "/proc/" + pid + "/"
	group, err := os.ReadFile(base + "cgroup")
	if err != nil || string(group) != "0::"+previewCgroup+"\n" {
		return previewIdentity{}, errors.New("wrong process cgroup")
	}
	status, err := os.ReadFile(base + "status")
	if err != nil {
		return previewIdentity{}, err
	}
	haveUID, haveGID := false, false
	for _, line := range strings.Split(string(status), "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || (f[0] != "Uid:" && f[0] != "Gid:") {
			continue
		}
		want := uid
		if f[0] == "Gid:" {
			want = gid
			haveGID = true
		} else {
			haveUID = true
		}
		if len(f) != 5 {
			return previewIdentity{}, errors.New("invalid process credentials")
		}
		for _, v := range f[1:] {
			if v != strconv.FormatUint(uint64(want), 10) {
				return previewIdentity{}, errors.New("wrong process credentials")
			}
		}
	}
	if !haveUID || !haveGID {
		return previewIdentity{}, errors.New("missing process credentials")
	}
	stat, err := os.ReadFile(base + "stat")
	if err != nil {
		return previewIdentity{}, err
	}
	// comm can contain spaces and ')'; field 22 follows the final ')'.
	end := strings.LastIndexByte(string(stat), ')')
	if end < 0 {
		return previewIdentity{}, errors.New("invalid process stat")
	}
	fields := strings.Fields(string(stat)[end+1:])
	if len(fields) < 20 {
		return previewIdentity{}, errors.New("invalid process stat")
	}
	start, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil || start == 0 {
		return previewIdentity{}, errors.New("invalid process start time")
	}
	return previewIdentity{n, uid, gid, strconv.FormatUint(start, 10)}, nil
}

// Fixed isolated Python invocation is a read-only hash worker, not a roothelper
// endpoint. Go's credential transition clears supplementary groups and drops
// active capabilities at non-root exec; the worker verifies that invariant and
// sets no_new_privs itself, also protecting direct non-systemd callers.
const previewHashWorker = `import ctypes,hashlib,json,os,sys
libc=ctypes.CDLL(None,use_errno=True)
if libc.prctl(38,1,0,0,0)!=0: raise RuntimeError('no_new_privs failed')
i=json.loads(sys.stdin.read(4096))
if set(i)!={'pid','uid','gid','start'} or type(i['pid']) is not int or i['pid']<=1: raise RuntimeError('identity rejected')
if os.getresuid()!=(i['uid'],)*3 or os.getresgid()!=(i['gid'],)*3 or os.getgroups(): raise RuntimeError('worker credentials rejected')
# UID transition clears effective/permitted/ambient, but not inheritable.
# Drop all sets explicitly; failure is fatal, never relax the zero-cap check.
class Header(ctypes.Structure): _fields_=[('version',ctypes.c_uint32),('pid',ctypes.c_int)]
class Data(ctypes.Structure): _fields_=[('effective',ctypes.c_uint32),('permitted',ctypes.c_uint32),('inheritable',ctypes.c_uint32)]
hdr=Header(0x20080522,0); caps=(Data*2)()
if libc.capset(ctypes.byref(hdr),ctypes.byref(caps))!=0: raise RuntimeError('capability clear failed')
if libc.prctl(47,4,0,0,0)!=0: raise RuntimeError('ambient clear failed')
s=dict(line.split(':',1) for line in open('/proc/self/status') if ':' in line)
if any(int(s[k].strip(),16) for k in ('CapEff','CapPrm','CapInh','CapAmb')) or s['NoNewPrivs'].strip()!='1': raise RuntimeError('worker privileges rejected')
d=os.open('/proc/'+str(i['pid']),os.O_RDONLY|os.O_DIRECTORY|os.O_CLOEXEC)
def read(name):
 f=os.open(name,os.O_RDONLY|os.O_CLOEXEC,dir_fd=d)
 with os.fdopen(f,'rb') as r: return r.read(65536)
def check():
 if read('cgroup')!=b'0::/system.slice/dujiao-preview.service\n': raise RuntimeError('cgroup rejected')
 s=dict(line.split(':',1) for line in read('status').decode().splitlines() if ':' in line)
 if s['Uid'].split()!=[str(i['uid'])]*4 or s['Gid'].split()!=[str(i['gid'])]*4: raise RuntimeError('target credentials rejected')
 if read('stat').decode().rsplit(')',1)[1].split()[19]!=i['start']: raise RuntimeError('PID replaced')
check()
f=os.open('exe',os.O_RDONLY|os.O_CLOEXEC,dir_fd=d)
with os.fdopen(f,'rb') as r:
 before=os.fstat(r.fileno())
 h=hashlib.file_digest(r,'sha256').hexdigest()
 after=os.fstat(r.fileno())
 current=os.stat('exe',dir_fd=d)
 if (before.st_dev,before.st_ino,before.st_size,before.st_mtime_ns,before.st_ctime_ns)!=(after.st_dev,after.st_ino,after.st_size,after.st_mtime_ns,after.st_ctime_ns) or (after.st_dev,after.st_ino)!=(current.st_dev,current.st_ino): raise RuntimeError('executable changed')
check()
os.close(d)
print(h)
`

var errHashWorker = errors.New("fixed hash worker failed")

func permanentHashError(err error) bool {
	return errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) || errors.Is(err, errHashWorker)
}

func hashPreviewIdentity(ctx context.Context, identity previewIdentity) (string, error) {
	uid, gid, credentialErr := previewCredentials()
	start, startErr := strconv.ParseUint(identity.Start, 10, 64)
	if credentialErr != nil || identity.UID != uid || identity.GID != gid || identity.UID == 0 || identity.GID == 0 || identity.PID <= 1 || startErr != nil || start == 0 || strconv.FormatUint(start, 10) != identity.Start {
		return "", errors.New("invalid hash worker identity")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	input, err := json.Marshal(identity)
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/python3", "-I", "-c", previewHashWorker)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C"}
	cmd.Stdin = strings.NewReader(string(input))
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: identity.UID, Gid: identity.GID, Groups: []uint32{}}}
	raw, err := cmd.Output()
	if err != nil {
		return "", errors.Join(errHashWorker, fmt.Errorf("preview hash worker failed: %w", err))
	}
	hash := strings.TrimSpace(string(raw))
	if !digestPattern.MatchString(hash) {
		return "", errors.New("invalid running hash")
	}
	return hash, nil
}

func runningPreviewHash(ctx context.Context) (string, previewIdentity, error) {
	uid, gid, err := previewCredentials()
	if err != nil {
		return "", previewIdentity{}, err
	}
	pid, err := fixedPreviewPID(ctx)
	if err != nil {
		return "", previewIdentity{}, err
	}
	identity, err := readPreviewIdentity(pid, uid, gid)
	if err != nil {
		return "", previewIdentity{}, err
	}
	hash, err := hashPreviewIdentity(ctx, identity)
	if err != nil {
		return "", previewIdentity{}, err
	}
	finalPID, err := fixedPreviewPID(ctx)
	if err != nil || finalPID != pid {
		return "", previewIdentity{}, errors.New("preview PID changed")
	}
	final, err := readPreviewIdentity(finalPID, uid, gid)
	if err != nil || final != identity {
		return "", previewIdentity{}, errors.New("preview identity changed")
	}
	return hash, identity, nil
}

func (s Systemd) Healthy(ctx context.Context, expected string) error {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 2 * time.Second}
	for {
		hash, identity, err := runningPreviewHash(ctx)
		if permanentHashError(err) {
			return errors.New("running identity verification unavailable; helper permissions or worker failed")
		}
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
					finalHash, finalIdentity, finalErr := runningPreviewHash(ctx)
					if finalErr == nil && finalHash == expected && finalIdentity == identity {
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
