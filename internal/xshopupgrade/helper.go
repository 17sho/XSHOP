package xshopupgrade

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dujiao-next/internal/customupgrade"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const Repository = "17sho/XSHOP"

var token = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func ValidTag(s string) bool   { return strings.HasPrefix(s, TagPrefix) && ValidAsset(s) }
func ValidAsset(s string) bool { return token.MatchString(s) && !strings.Contains(s, "..") }
func PeerAllowed(uid, wanted uint32, cgroup string) bool {
	if uid != wanted {
		return false
	}
	for _, line := range strings.Split(cgroup, "\n") {
		if line == "0::"+previewCgroup {
			return true
		}
	}
	return false
}

// Source never accepts a URL from an HTTP caller or an unsigned manifest.
type Source interface {
	Latest(context.Context) (string, error)
	Asset(context.Context, string, string, uint64) ([]byte, error)
}
type Status struct {
	NeedRestart       bool   `json:"need_restart"`
	Phase             string `json:"phase,omitempty"`
	CurrentVersion    string `json:"current_version,omitempty"`
	PreviousVersion   string `json:"previous_version,omitempty"`
	RollbackAvailable bool   `json:"rollback_available"`
	State             string `json:"state"`
	Version           string `json:"version,omitempty"`
	Sequence          uint64 `json:"sequence,omitempty"`
	Digest            string `json:"digest,omitempty"`
	Message           string `json:"message,omitempty"`
	Repository        string `json:"repository"`
}
type Helper struct {
	Engine    *Engine
	Source    Source
	PublicKey ed25519.PublicKey
	Schema    string
	mu        sync.Mutex
	status    Status
	busy      bool
	tag       string
}

func (h *Helper) policy() (customupgrade.Policy, error) {
	s, err := h.Engine.ReadState()
	if err != nil {
		return customupgrade.Policy{}, err
	}
	if s.Pending {
		return customupgrade.Policy{}, errors.New("recovery pending")
	}
	hash, err := FileHash(h.Engine.Target)
	return customupgrade.Policy{PublicKey: h.PublicKey, Channel: Channel, Profile: Profile, OS: "linux", Arch: "amd64", CurrentBinarySHA256: hash, SchemaFingerprint: h.Schema, HighWaterSequence: s.HighWater, UpdaterVersion: 1}, err
}
func (h *Helper) verify(ctx context.Context, tag string) (*customupgrade.VerifiedManifest, error) {
	if !ValidTag(tag) {
		return nil, errors.New("tag rejected")
	}
	raw, err := h.Source.Asset(ctx, tag, "manifest.json", customupgrade.MaxManifestBytes)
	if err != nil {
		return nil, err
	}
	sig, err := h.Source.Asset(ctx, tag, "manifest.sig", 64)
	if err != nil {
		return nil, err
	}
	p, err := h.policy()
	if err != nil {
		return nil, err
	}
	v, err := customupgrade.VerifyManifest(raw, sig, p)
	if err != nil {
		return nil, err
	}
	m := v.Manifest()
	if m.Archive.Size > 128<<20 || m.Source.Size > 64<<20 || m.Files[0].Size > 256<<20 {
		return nil, errors.New("local package limit rejected")
	}
	if m.Version != tag {
		return nil, errors.New("tag/manifest mismatch")
	}
	return v, nil
}
func (h *Helper) finish(s Status) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if s.Phase == "" {
		if s.State == "failed" {
			s.Phase = h.status.Phase
			if s.Phase == "" {
				s.Phase = h.status.State
			}
		} else {
			s.Phase = s.State
		}
	}
	s.Repository = Repository
	h.status = s
	h.busy = false
}
func (h *Helper) check(ctx context.Context) {
	var tag string
	var err error
	if Production {
		tag, err = h.productionLatest(ctx)
	} else {
		tag, err = h.Source.Latest(ctx)
	}
	if err != nil {
		h.finish(Status{State: "failed", Message: "检查失败；请由服务器管理员查看受限日志"})
		return
	}
	if h.installedLatest(ctx, tag) {
		return
	}
	v, err := h.verify(ctx, tag)
	if err != nil {
		h.finish(Status{State: "failed", Message: "更新签名、兼容性或防回退校验失败"})
		return
	}
	m := v.Manifest()
	h.mu.Lock()
	h.tag = tag
	h.mu.Unlock()
	h.finish(Status{State: "available", Version: m.Version, Sequence: m.Sequence, Digest: v.Digest(), Message: "已验证 XSHOP 自定义更新；数据库结构不变"})
}
func (h *Helper) installedLatest(ctx context.Context, tag string) bool {
	if !ValidTag(tag) {
		return false
	}
	h.Engine.mu.Lock()
	defer h.Engine.mu.Unlock()
	s, err := h.Engine.ReadState()
	if err != nil || s.Pending || s.Result != "installed" || s.Version != tag {
		return false
	}
	raw, err := h.Source.Asset(ctx, tag, "manifest.json", customupgrade.MaxManifestBytes)
	if err != nil {
		return false
	}
	sig, err := h.Source.Asset(ctx, tag, "manifest.sig", 64)
	if err != nil {
		return false
	}
	p, err := h.policy()
	if err != nil || !customupgrade.InstalledIdentity(raw, sig, p, s.Version, s.Digest, s.NewHash) {
		return false
	}
	bounded, cancel := context.WithTimeout(ctx, RecoveryTimeout)
	defer cancel()
	if h.Engine.Control.Healthy(bounded, s.NewHash) != nil {
		return false
	}
	h.finish(Status{State: "up_to_date", CurrentVersion: s.Version, Version: s.Version, Sequence: s.HighWater, Digest: s.Digest, Message: "当前已是最新版本"})
	return true
}
func (h *Helper) activationFailureMessage() string {
	state, err := h.Engine.ReadState()
	if err == nil && !state.Pending && state.Result == "rolled_back" {
		return "安装失败，旧版本已恢复"
	}
	return "安装或恢复失败，需要服务器管理员处理；请勿继续升级"
}

func (h *Helper) install(ctx context.Context, tag, digest string) {
	v, err := h.verify(ctx, tag)
	if err != nil || v.Digest() != digest {
		h.finish(Status{State: "failed", Message: "更新身份已变化，请重新检查"})
		return
	}
	m := v.Manifest()
	src, err := h.Source.Asset(ctx, tag, m.Source.Name, m.Source.Size)
	if err == nil {
		err = customupgrade.VerifySource(ctx, bytes.NewReader(src), v)
	}
	if err != nil {
		h.finish(Status{State: "failed", Message: "对应源码校验失败"})
		return
	}
	archive, err := h.Source.Asset(ctx, tag, m.Archive.Name, m.Archive.Size)
	if err != nil {
		h.finish(Status{State: "failed", Message: "升级包下载失败"})
		return
	}
	dir, err := os.MkdirTemp(h.Engine.StateDir, "download-")
	if err != nil {
		h.finish(Status{State: "failed", Message: "暂存失败"})
		return
	}
	// Retain exact Corresponding Source and manifest in private helper storage.
	if err = os.WriteFile(filepath.Join(dir, m.Source.Name), src, 0600); err != nil {
		h.finish(Status{State: "failed", Message: "源码保存失败"})
		return
	}
	staged, err := customupgrade.VerifyAndStage(ctx, bytes.NewReader(archive), filepath.Join(dir, "stage"), v)
	if err != nil {
		h.finish(Status{State: "failed", Message: "升级包校验失败"})
		return
	}
	// Retain authenticated metadata beside Corresponding Source before preparing.
	raw, metaErr := h.Source.Asset(ctx, tag, "manifest.json", customupgrade.MaxManifestBytes)
	sig, sigErr := h.Source.Asset(ctx, tag, "manifest.sig", 64)
	pMeta, policyErr := h.policy()
	metadata, verifyErr := customupgrade.VerifyManifest(raw, sig, pMeta)
	if metaErr != nil || sigErr != nil || policyErr != nil || verifyErr != nil || metadata.Digest() != digest {
		h.finish(Status{State: "failed", Message: "源码元数据身份已变化"})
		return
	}
	for name, data := range map[string][]byte{"manifest.json": raw, "manifest.sig": sig} {
		if err = os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			h.finish(Status{State: "failed", Message: "元数据保存失败"})
			return
		}
	}
	sourceFiles, fileErr := retainedFiles(dir)
	if fileErr != nil {
		h.finish(Status{State: "failed", Message: "源码身份保存失败"})
		return
	}
	marker := &Engine{StateDir: dir}
	if err = marker.saveJSON("retention.json", retentionIdentity{BinaryHash: staged.BinarySHA256, Files: sourceFiles}); err != nil {
		h.finish(Status{State: "failed", Message: "源码身份保存失败"})
		return
	}
	old, err := FileHash(h.Engine.Target)
	if err != nil {
		h.finish(Status{State: "failed", Message: "基线读取失败"})
		return
	}
	// Verification policy must still match immediately before activation.
	p, err := h.policy()
	if err != nil || p.CurrentBinarySHA256 != old {
		h.finish(Status{State: "failed", Message: "安装基线已变化"})
		return
	}
	found := false
	for _, hash := range m.FromBinarySHA256 {
		found = found || hash == old
	}
	if !found {
		h.finish(Status{State: "failed", Message: "安装基线不兼容"})
		return
	}
	err = h.Engine.Prepare(ctx, staged.BinaryPath, old, staged.BinarySHA256, m.Sequence, m.Version, digest)
	if err != nil {
		h.finish(Status{State: "failed", Message: h.activationFailureMessage()})
		return
	}
	h.finish(Status{State: "prepared", NeedRestart: true, Phase: "prepared", Version: m.Version, Sequence: m.Sequence, Digest: digest, Message: "更新已准备；旧程序继续服务，请显式重启完成升级"})
}
func (h *Helper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	if r.URL.RawQuery != "" {
		http.Error(w, "query rejected", 400)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/status" {
		h.mu.Lock()
		s := h.status
		busy := h.busy
		h.mu.Unlock()
		// Cached availability is not authority when durable state cannot be read.
		if h.Engine != nil {
			journal, err := h.Engine.ReadState()
			if err != nil || (journal.Pending && journal.Result != "prepared" && !busy) {
				s = Status{State: "failed", Message: "升级状态不可用，需要服务器管理员恢复；请勿继续升级"}
			}
		}
		if s.State == "" {
			s.State = "idle"
			if h.Engine != nil {
				if journal, err := h.Engine.ReadState(); err == nil {
					s.Sequence = journal.HighWater
					if journal.Pending {
						s.State = "failed"
						s.Message = "需要服务器管理员恢复"
					} else if journal.Result == "installed" {
						s.State = "installed"
					} else if journal.Result == "rolled_back" {
						s.State = "rolled_back"
						s.Message = "旧程序已恢复；失败序号已禁用"
					}
				}
			}
		}
		if h.Engine != nil && !busy {
			if journal, err := h.Engine.ReadState(); err == nil && journal.Result == "prepared" {
				s = Status{State: "prepared", NeedRestart: true, Phase: "prepared", Version: journal.Version, Sequence: journal.HighWater, Digest: journal.Digest}
			}
		}
		if h.Engine != nil && !busy {
			if journal, err := h.Engine.ReadState(); err == nil && !journal.Pending {
				switch journal.Result {
				case "installed":
					s.CurrentVersion = journal.Version
					s.PreviousVersion = journal.PreviousVersion
					s.RollbackAvailable = h.Engine.retained(journal) == nil
				case "rolled_back":
					s.CurrentVersion = journal.PreviousVersion
					s.PreviousVersion = ""
					s.RollbackAvailable = false
					if s.State == "idle" || s.State == "installed" {
						s.State = "rolled_back"
					}
				}
			}
		}
		s.Repository = Repository
		json.NewEncoder(w).Encode(s)
		return
	}
	if r.Method != "POST" || (r.URL.Path != "/check" && r.URL.Path != "/install" && r.URL.Path != "/restart" && r.URL.Path != "/rollback") {
		http.Error(w, "not found", 404)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 257))
	if err != nil || len(body) > 256 {
		http.Error(w, "size rejected", 400)
		return
	}
	digest := ""
	if r.URL.Path == "/check" || r.URL.Path == "/restart" || r.URL.Path == "/rollback" {
		if len(body) != 0 {
			http.Error(w, "empty body required", 400)
			return
		}
	} else { // Canonical one-field JSON prevents duplicate/unknown keys and aliases.
		var payload struct {
			Digest string `json:"digest"`
		}
		if json.Unmarshal(body, &payload) != nil || !digestPattern.MatchString(payload.Digest) {
			http.Error(w, "digest rejected", 400)
			return
		}
		canonical, _ := json.Marshal(payload)
		if string(body) != string(canonical) {
			http.Error(w, "canonical payload required", 400)
			return
		}
		digest = payload.Digest
	}
	h.mu.Lock()
	if h.busy {
		h.mu.Unlock()
		http.Error(w, "busy", 409)
		return
	}
	if digest != "" && (h.status.State != "available" || h.status.Digest != digest) {
		h.mu.Unlock()
		http.Error(w, "check first", 409)
		return
	}
	if r.URL.Path == "/restart" {
		if h.Engine == nil {
			h.mu.Unlock()
			http.Error(w, "not prepared", 409)
			return
		}
		s, err := h.Engine.ReadState()
		if err != nil || s.Result != "prepared" {
			h.mu.Unlock()
			http.Error(w, "not prepared", 409)
			return
		}
	}
	if r.URL.Path == "/rollback" {
		if h.Engine == nil {
			h.mu.Unlock()
			http.Error(w, "rollback unavailable", 409)
			return
		}
		s, err := h.Engine.ReadState()
		if err != nil || s.Pending || s.Result != "installed" || h.Engine.retained(s) != nil {
			h.mu.Unlock()
			http.Error(w, "rollback unavailable", 409)
			return
		}
	}
	h.busy = true
	tag := h.tag
	h.status = Status{State: "checking", Repository: Repository}
	if digest != "" {
		h.status.State = "installing"
	}
	if r.URL.Path == "/restart" {
		h.status = Status{State: "restarting", Phase: "restarting", Repository: Repository}
	}
	if r.URL.Path == "/rollback" {
		h.status = Status{State: "rolling_back", Phase: "rollback", Repository: Repository}
	}
	h.mu.Unlock()
	w.WriteHeader(202)
	fmt.Fprint(w, `{"accepted":true}`)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if r.URL.Path == "/rollback" {
			if err := h.Engine.Rollback(ctx); err != nil {
				h.finish(Status{State: "failed", Phase: "rollback", Message: "回退或恢复失败，需要服务器管理员处理"})
			} else {
				h.finish(Status{State: "rolled_back", Message: "上一版本已恢复；数据库未回退"})
			}
		} else if r.URL.Path == "/restart" {
			s, _ := h.Engine.ReadState()
			if err := h.Engine.Restart(ctx); err != nil {
				h.finish(Status{State: "failed", Phase: "restart", Message: h.activationFailureMessage()})
			} else {
				h.finish(Status{State: "installed", Version: s.Version, Sequence: s.HighWater, Digest: s.Digest, Message: "升级完成；数据库未回退"})
			}
		} else if digest == "" {
			h.check(ctx)
		} else {
			h.install(ctx, tag, digest)
		}
	}()
}
func ParseKey(s string) (ed25519.PublicKey, error) {
	raw, err := hex.DecodeString(s)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, errors.New("key rejected")
	}
	return raw, nil
}
