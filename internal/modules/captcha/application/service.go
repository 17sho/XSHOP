package application

import (
	"container/list"
	"crypto/rand"
	"encoding/hex"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/modules/captcha/contract"
	settingssecurity "github.com/dujiao-next/internal/modules/settings/schema/security"
	"github.com/dujiao-next/internal/shared/jsonmap"

	"github.com/mojocn/base64Captcha"
)

// Service 验证码服务
// 负责统一读取配置、生成挑战与执行校验
// 按场景开关决定是否需要验证码
// 对图片验证码与 Turnstile 进行统一封装
// 外部仅需要调用 Verify(scene, payload, clientIP)
// 以及图片模式下调用 GenerateImageChallenge
//
//nolint:govet
type Service struct {
	settingService contract.SettingReader
	defaultConfig  config.CaptchaConfig
	turnstile      contract.TurnstileVerifier
	cacheTTL       time.Duration

	mu            sync.RWMutex
	cachedSetting settingssecurity.CaptchaSetting
	cachedAt      time.Time

	imageStore          base64Captcha.Store
	imageStoreMaxStore  int
	imageStoreExpireSec int
}

// NewService 创建验证码服务。
func NewService(settingService contract.SettingReader, defaultConfig config.CaptchaConfig, turnstile contract.TurnstileVerifier) *Service {
	return &Service{
		settingService: settingService,
		defaultConfig:  defaultConfig,
		turnstile:      turnstile,
		cacheTTL:       30 * time.Second,
	}
}

// SetDefaultConfig 更新默认配置（通常在后台保存后调用）
func (s *Service) SetDefaultConfig(defaultConfig config.CaptchaConfig) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.defaultConfig = defaultConfig
	s.cachedAt = time.Time{}
}

// InvalidateCache 失效本地缓存配置
func (s *Service) InvalidateCache() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cachedAt = time.Time{}
}

// GetPublicSetting 获取公开可下发配置
func (s *Service) GetPublicSetting() (jsonmap.JSON, error) {
	setting, err := s.getSetting()
	if err != nil {
		return nil, err
	}
	return settingssecurity.PublicCaptchaSetting(setting), nil
}

// GenerateImageChallenge 生成图片验证码
func (s *Service) GenerateImageChallenge() (*contract.ImageChallenge, error) {
	setting, err := s.getSetting()
	if err != nil {
		return nil, err
	}
	if setting.Provider != constants.CaptchaProviderImage {
		return nil, contract.ErrConfigInvalid
	}

	store := s.ensureImageStore(setting)
	characterSource := "0123456789"
	if setting.Image.CharacterType == "alphanumeric" {
		characterSource = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	}
	driver := base64Captcha.NewDriverString(
		setting.Image.Height,
		setting.Image.Width,
		setting.Image.NoiseCount,
		setting.Image.ShowLine,
		setting.Image.Length,
		characterSource,
		nil,
		base64Captcha.DefaultEmbeddedFonts,
		nil,
	)
	// Generate identifiers and answers with crypto/rand; the dependency uses
	// math/rand for these. Keep its renderer only, without replacing dependency.
	idBytes := make([]byte, 24)
	if _, err := rand.Read(idBytes); err != nil {
		return nil, err
	}
	id := hex.EncodeToString(idBytes)
	answer := make([]byte, setting.Image.Length)
	for i := range answer {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(characterSource))))
		if err != nil {
			return nil, err
		}
		answer[i] = characterSource[n.Int64()]
	}
	item, genErr := driver.DrawCaptcha(string(answer))
	if genErr != nil {
		return nil, genErr
	}
	if err := store.Set(id, string(answer)); err != nil {
		return nil, err
	}
	b64s := item.EncodeB64string()

	return &contract.ImageChallenge{
		CaptchaID:   strings.TrimSpace(id),
		ImageBase64: strings.TrimSpace(b64s),
	}, nil
}

// Verify 按场景校验验证码
func (s *Service) Verify(scene string, payload contract.VerifyPayload, clientIP string) error {
	setting, err := s.getSetting()
	if err != nil {
		return err
	}

	if !setting.IsSceneEnabled(scene) {
		return nil
	}

	switch setting.Provider {
	case constants.CaptchaProviderImage:
		captchaID := strings.TrimSpace(payload.CaptchaID)
		captchaCode := strings.TrimSpace(payload.CaptchaCode)
		if captchaID == "" || captchaCode == "" {
			return contract.ErrRequired
		}
		store := s.ensureImageStore(setting)
		if !store.Verify(captchaID, captchaCode, true) {
			return contract.ErrInvalid
		}
		return nil
	case constants.CaptchaProviderTurnstile:
		token := strings.TrimSpace(payload.TurnstileToken)
		if token == "" {
			return contract.ErrRequired
		}
		if s.turnstile == nil {
			return contract.ErrVerifyFailed
		}
		return s.turnstile.Verify(setting.Turnstile, token, strings.TrimSpace(clientIP))
	case constants.CaptchaProviderNone:
		return contract.ErrConfigInvalid
	default:
		return contract.ErrConfigInvalid
	}
}

func (s *Service) ensureImageStore(setting settingssecurity.CaptchaSetting) base64Captcha.Store {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.imageStore != nil && s.imageStoreMaxStore == setting.Image.MaxStore && s.imageStoreExpireSec == setting.Image.ExpireSeconds {
		return s.imageStore
	}
	s.imageStore = newImageStore(setting.Image.MaxStore, time.Duration(setting.Image.ExpireSeconds)*time.Second, time.Now)
	s.imageStoreMaxStore = setting.Image.MaxStore
	s.imageStoreExpireSec = setting.Image.ExpireSeconds
	return s.imageStore
}

type imageEntry struct {
	id, answer string
	expires    time.Time
}

// imageStore enforces strict TTL and evicts oldest entries at true capacity.
// TTL/capacity updates replace this store, invalidating outstanding challenges;
// presentation/scene changes preserve them. Wrong and correct attempts consume.
type imageStore struct {
	mu       sync.Mutex
	entries  map[string]*list.Element
	order    *list.List
	capacity int
	ttl      time.Duration
	now      func() time.Time
}

func newImageStore(capacity int, ttl time.Duration, now func() time.Time) *imageStore {
	return &imageStore{entries: make(map[string]*list.Element), order: list.New(), capacity: capacity, ttl: ttl, now: now}
}
func (s *imageStore) remove(e *list.Element) {
	delete(s.entries, e.Value.(imageEntry).id)
	s.order.Remove(e)
}
func (s *imageStore) Set(id, answer string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for e := s.order.Front(); e != nil; e = s.order.Front() {
		if now.Before(e.Value.(imageEntry).expires) {
			break
		}
		s.remove(e)
	}
	if e := s.entries[id]; e != nil {
		s.remove(e)
	}
	for len(s.entries) >= s.capacity {
		s.remove(s.order.Front())
	}
	s.entries[id] = s.order.PushBack(imageEntry{id: id, answer: answer, expires: now.Add(s.ttl)})
	return nil
}
func (s *imageStore) Get(id string, clear bool) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.entries[id]
	if e == nil {
		return ""
	}
	entry := e.Value.(imageEntry)
	expired := !s.now().Before(entry.expires)
	if clear || expired {
		s.remove(e)
	}
	if expired {
		return ""
	}
	return entry.answer
}
func (s *imageStore) Verify(id, answer string, clear bool) bool {
	stored := s.Get(id, clear)
	return stored != "" && strings.EqualFold(stored, strings.TrimSpace(answer))
}

func (s *Service) getSetting() (settingssecurity.CaptchaSetting, error) {
	if s == nil {
		return settingssecurity.DefaultCaptchaSetting(config.CaptchaConfig{}), nil
	}

	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.cachedAt.IsZero() && now.Sub(s.cachedAt) <= s.cacheTTL {
		return s.cachedSetting, nil
	}

	fallback := s.defaultConfig
	if s.settingService == nil {
		setting := settingssecurity.DefaultCaptchaSetting(fallback)
		s.cachedSetting = setting
		s.cachedAt = now
		return setting, nil
	}

	setting, err := s.settingService.GetCaptchaSetting(fallback)
	if err != nil {
		return settingssecurity.CaptchaSetting{}, err
	}
	setting = settingssecurity.NormalizeCaptchaSetting(setting)

	s.cachedSetting = setting
	s.cachedAt = now
	return setting, nil
}
