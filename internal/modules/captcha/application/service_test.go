package application

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/modules/captcha/contract"
	settingssecurity "github.com/dujiao-next/internal/modules/settings/schema/security"
)

type settingReaderStub struct {
	setting settingssecurity.CaptchaSetting
}

func (s settingReaderStub) GetCaptchaSetting(config.CaptchaConfig) (settingssecurity.CaptchaSetting, error) {
	return s.setting, nil
}

type turnstileStub struct {
	cfg      settingssecurity.CaptchaTurnstileSetting
	token    string
	clientIP string
}

func (s *turnstileStub) Verify(cfg settingssecurity.CaptchaTurnstileSetting, token, clientIP string) error {
	s.cfg = cfg
	s.token = token
	s.clientIP = clientIP
	return nil
}

func TestVerifyTurnstileDelegatesToVerifier(t *testing.T) {
	setting := settingssecurity.CaptchaSetting{
		Provider: constants.CaptchaProviderTurnstile,
		Scenes: settingssecurity.CaptchaSceneSetting{
			Login: true,
		},
		Turnstile: settingssecurity.CaptchaTurnstileSetting{
			SecretKey: "secret",
			VerifyURL: "https://captcha.example/verify",
			TimeoutMS: 2000,
		},
	}
	verifier := &turnstileStub{}
	service := NewService(settingReaderStub{setting: setting}, config.CaptchaConfig{}, verifier)

	err := service.Verify(constants.CaptchaSceneLogin, contract.VerifyPayload{TurnstileToken: " token-1 "}, " 127.0.0.1 ")
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if verifier.token != "token-1" || verifier.clientIP != "127.0.0.1" || verifier.cfg.SecretKey != "secret" {
		t.Fatalf("turnstile input mismatch: token=%q ip=%q cfg=%#v", verifier.token, verifier.clientIP, verifier.cfg)
	}
}

func TestGenerateImageChallengeUsesDigitsOnly(t *testing.T) {
	setting := settingssecurity.CaptchaSetting{
		Provider: constants.CaptchaProviderImage,
		Image: settingssecurity.CaptchaImageSetting{
			CharacterType: "digits",
			Length:        6,
			Width:         240,
			Height:        80,
			NoiseCount:    2,
			ShowLine:      2,
			ExpireSeconds: 300,
			MaxStore:      100,
		},
	}
	service := NewService(settingReaderStub{setting: setting}, config.CaptchaConfig{}, nil)

	for i := 0; i < 100; i++ {
		challenge, err := service.GenerateImageChallenge()
		if err != nil {
			t.Fatalf("generate image captcha: %v", err)
		}
		answer := service.imageStore.Get(challenge.CaptchaID, false)
		if !regexp.MustCompile(`^[0-9]{6}$`).MatchString(answer) {
			t.Fatalf("image captcha answer %q contains non-digit characters", answer)
		}
	}
}

func TestGenerateImageChallengeUsesAlphanumericWhenSelected(t *testing.T) {
	setting := settingssecurity.CaptchaSetting{
		Provider: constants.CaptchaProviderImage,
		Image: settingssecurity.CaptchaImageSetting{
			CharacterType: "alphanumeric",
			Length:        6,
			Width:         240,
			Height:        80,
			NoiseCount:    2,
			ShowLine:      2,
			ExpireSeconds: 300,
			MaxStore:      100,
		},
	}
	service := NewService(settingReaderStub{setting: setting}, config.CaptchaConfig{}, nil)
	foundLetter := false
	for i := 0; i < 100; i++ {
		challenge, err := service.GenerateImageChallenge()
		if err != nil {
			t.Fatalf("generate image captcha: %v", err)
		}
		answer := service.imageStore.Get(challenge.CaptchaID, false)
		if !regexp.MustCompile(`^[0-9A-Za-z]{6}$`).MatchString(answer) {
			t.Fatalf("image captcha answer %q is not alphanumeric", answer)
		}
		if regexp.MustCompile(`[A-Za-z]`).MatchString(answer) {
			foundLetter = true
		}
	}
	if !foundLetter {
		t.Fatal("alphanumeric mode generated no letters in 100 challenges")
	}
}

func TestVerifyImageRequiresChallengePayload(t *testing.T) {
	setting := settingssecurity.CaptchaSetting{
		Provider: constants.CaptchaProviderImage,
		Scenes: settingssecurity.CaptchaSceneSetting{
			Login: true,
		},
	}
	service := NewService(settingReaderStub{setting: setting}, config.CaptchaConfig{}, &turnstileStub{})
	if err := service.Verify(constants.CaptchaSceneLogin, contract.VerifyPayload{}, ""); err != contract.ErrRequired {
		t.Fatalf("empty image captcha error got %v want ErrRequired", err)
	}
}

func TestVerifyGuestImageConsumesCorrectChallengeAndRejectsReplay(t *testing.T) {
	setting := settingssecurity.NormalizeCaptchaSetting(settingssecurity.CaptchaSetting{
		Provider: constants.CaptchaProviderImage,
		Scenes:   settingssecurity.CaptchaSceneSetting{GuestCreateOrder: true},
	})
	service := NewService(settingReaderStub{setting: setting}, config.CaptchaConfig{}, nil)
	challenge, err := service.GenerateImageChallenge()
	if err != nil {
		t.Fatal(err)
	}
	answer := service.imageStore.Get(challenge.CaptchaID, false)
	payload := contract.VerifyPayload{CaptchaID: challenge.CaptchaID, CaptchaCode: answer}
	if err := service.Verify(constants.CaptchaSceneGuestCreateOrder, payload, ""); err != nil {
		t.Fatalf("correct guest captcha: %v", err)
	}
	if err := service.Verify(constants.CaptchaSceneGuestCreateOrder, payload, ""); err != contract.ErrInvalid {
		t.Fatalf("captcha replay accepted: %v", err)
	}
}

func TestVerifySkipsDisabledScene(t *testing.T) {
	setting := settingssecurity.CaptchaSetting{Provider: constants.CaptchaProviderTurnstile}
	service := NewService(settingReaderStub{setting: setting}, config.CaptchaConfig{}, nil)
	if err := service.Verify(constants.CaptchaSceneLogin, contract.VerifyPayload{}, ""); err != nil {
		t.Fatalf("disabled scene must skip captcha, got %v", err)
	}
}

func TestImageStoreStrictTTLBoundedCapacityAndConsumption(t *testing.T) {
	now := time.Unix(100, 0)
	s := newImageStore(2, time.Minute, func() time.Time { return now })
	if err := s.Set("a", "1234"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	s.Set("b", "5678")
	s.Set("c", "abcd")
	if s.Get("a", false) != "" || len(s.entries) != 2 {
		t.Fatal("capacity must evict oldest")
	}
	now = now.Add(time.Minute)
	if s.Verify("b", "5678", true) || s.Get("c", false) != "" {
		t.Fatal("expired entry accepted without GC")
	}
	s.Set("wrong", "1234")
	if s.Verify("wrong", "9999", true) || s.Verify("wrong", "1234", true) {
		t.Fatal("wrong attempt must consume")
	}
	s.Set("right", "Ab12")
	if !s.Verify("right", "aB12", true) || s.Verify("right", "Ab12", true) {
		t.Fatal("case-insensitive single use violated")
	}
	if s.Verify("missing", "", true) {
		t.Fatal("empty missing match")
	}
}

func TestImageStoreConcurrentSingleUse(t *testing.T) {
	s := newImageStore(100, time.Minute, time.Now)
	s.Set("one", "1234")
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.Verify("one", "1234", true) {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("wins %d", wins.Load())
	}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Set("shared", "1234")
			s.Get("shared", false)
			s.Verify("shared", "wrong", true)
		}()
	}
	wg.Wait()
}

func TestImageParametersRealPNGAndHotUpdates(t *testing.T) {
	cfg := config.CaptchaConfig{Provider: "image", Image: config.CaptchaImageConfig{Length: 4, Width: 321, Height: 99, MaxStore: 100, ExpireSeconds: 30}}
	s := NewService(nil, cfg, nil)
	ids := map[string]bool{}
	for _, typ := range []string{"digits", "alphanumeric"} {
		for _, mask := range []int{0, 2, 4, 6, 8, 10, 12, 14} {
			for _, noise := range []int{0, 2, 20} {
				cfg.Image.CharacterType = typ
				cfg.Image.ShowLine = mask
				cfg.Image.NoiseCount = noise
				s.SetDefaultConfig(cfg)
				c, err := s.GenerateImageChallenge()
				if err != nil {
					t.Fatal(err)
				}
				if len(c.CaptchaID) != 48 || ids[c.CaptchaID] {
					t.Fatal("invalid/duplicate secure ID")
				}
				ids[c.CaptchaID] = true
				raw, err := base64.StdEncoding.DecodeString(strings.SplitN(c.ImageBase64, ",", 2)[1])
				if err != nil {
					t.Fatal(err)
				}
				img, err := png.Decode(bytes.NewReader(raw))
				if err != nil {
					t.Fatal(err)
				}
				if img.Bounds().Dx() != 321 || img.Bounds().Dy() != 99 {
					t.Fatal("wrong PNG dimensions")
				}
				answer := s.imageStore.Get(c.CaptchaID, false)
				if len(answer) != 4 {
					t.Fatalf("length %d", len(answer))
				}
				if typ == "digits" && strings.Trim(answer, "0123456789") != "" {
					t.Fatal("not digits")
				}
			}
		}
	}
	c, _ := s.GenerateImageChallenge()
	a := s.imageStore.Get(c.CaptchaID, false)
	p := contract.VerifyPayload{CaptchaID: c.CaptchaID, CaptchaCode: a}
	cfg.Scenes.Login = true
	s.SetDefaultConfig(cfg)
	if err := s.Verify(constants.CaptchaSceneLogin, contract.VerifyPayload{}, ""); err != contract.ErrRequired {
		t.Fatal(err)
	}
	// Presentation and scene changes retain challenges.
	cfg.Image.Length = 8
	s.SetDefaultConfig(cfg)
	if err := s.Verify(constants.CaptchaSceneLogin, p, ""); err != nil {
		t.Fatal(err)
	}
	c, _ = s.GenerateImageChallenge()
	a = s.imageStore.Get(c.CaptchaID, false)
	// Changing TTL/capacity resets outstanding challenges, never extends old TTL.
	cfg.Image.ExpireSeconds = 31
	s.SetDefaultConfig(cfg)
	if err := s.Verify(constants.CaptchaSceneLogin, contract.VerifyPayload{CaptchaID: c.CaptchaID, CaptchaCode: a}, ""); err != contract.ErrInvalid {
		t.Fatal("TTL reset", err)
	}
	c, _ = s.GenerateImageChallenge()
	a = s.imageStore.Get(c.CaptchaID, false)
	cfg.Image.MaxStore = 101
	s.SetDefaultConfig(cfg)
	if err := s.Verify(constants.CaptchaSceneLogin, contract.VerifyPayload{CaptchaID: c.CaptchaID, CaptchaCode: a}, ""); err != contract.ErrInvalid {
		t.Fatal("capacity reset", err)
	}
	cfg.Scenes.Login = false
	s.SetDefaultConfig(cfg)
	if err := s.Verify(constants.CaptchaSceneLogin, contract.VerifyPayload{}, ""); err != nil {
		t.Fatal(err)
	}
}

func TestServiceConcurrentDefaultsAndGeneration(t *testing.T) {
	cfg := config.CaptchaConfig{Provider: "image"}
	s := NewService(nil, cfg, nil)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				s.SetDefaultConfig(cfg)
				s.InvalidateCache()
				if _, err := s.GenerateImageChallenge(); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
}
