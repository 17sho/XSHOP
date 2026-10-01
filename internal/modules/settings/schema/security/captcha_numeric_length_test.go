package settingssecurity

import (
	"testing"

	"github.com/dujiao-next/internal/config"
)

func TestNormalizeNumericCaptchaLengthUsesSecureMinimum(t *testing.T) {
	for _, length := range []int{0, 4, 5} {
		setting := NormalizeCaptchaSetting(CaptchaSetting{
			Image: CaptchaImageSetting{Length: length},
		})
		if setting.Image.Length != 6 {
			t.Fatalf("length %d normalized to %d, want 6", length, setting.Image.Length)
		}
	}

	for _, length := range []int{6, 7, 8} {
		setting := NormalizeCaptchaSetting(CaptchaSetting{
			Image: CaptchaImageSetting{Length: length},
		})
		if setting.Image.Length != length {
			t.Fatalf("valid length %d normalized to %d", length, setting.Image.Length)
		}
	}
}

func TestDefaultNumericCaptchaLengthIsSix(t *testing.T) {
	setting := DefaultCaptchaSetting(config.CaptchaConfig{})
	if setting.Image.Length != 6 {
		t.Fatalf("default numeric captcha length = %d, want 6", setting.Image.Length)
	}
	if setting.Image.CharacterType != "digits" {
		t.Fatalf("default image captcha character type = %q, want digits", setting.Image.CharacterType)
	}
}

func TestCaptchaCharacterTypeRoundTripsAndRejectsUnknownValues(t *testing.T) {
	setting := NormalizeCaptchaSetting(CaptchaSetting{Image: CaptchaImageSetting{
		CharacterType: "alphanumeric",
		Length:        6,
	}})
	encoded := EncodeCaptchaSetting(setting)
	decoded := DecodeCaptchaSetting(encoded, CaptchaSetting{})
	if decoded.Image.CharacterType != "alphanumeric" {
		t.Fatalf("character type round trip = %q", decoded.Image.CharacterType)
	}

	invalid := NormalizeCaptchaSetting(CaptchaSetting{Image: CaptchaImageSetting{CharacterType: "unknown"}})
	if invalid.Image.CharacterType != "digits" {
		t.Fatalf("unknown character type normalized to %q, want digits", invalid.Image.CharacterType)
	}
}
