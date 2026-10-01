package application

import (
	"regexp"
	"testing"

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

func TestVerifySkipsDisabledScene(t *testing.T) {
	setting := settingssecurity.CaptchaSetting{Provider: constants.CaptchaProviderTurnstile}
	service := NewService(settingReaderStub{setting: setting}, config.CaptchaConfig{}, nil)
	if err := service.Verify(constants.CaptchaSceneLogin, contract.VerifyPayload{}, ""); err != nil {
		t.Fatalf("disabled scene must skip captcha, got %v", err)
	}
}
