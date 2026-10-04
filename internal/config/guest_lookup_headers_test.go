package config

import "testing"

func TestGuestLookupCaptchaHeadersAllowedByDefault(t *testing.T) {
	for _, wanted := range []string{"X-Captcha-ID", "X-Captcha-Code", "X-Turnstile-Token"} {
		found := false
		for _, header := range DefaultCORSAllowedHeaders() {
			if header == wanted {
				found = true
			}
		}
		if !found {
			t.Errorf("guest lookup CAPTCHA header %s missing from default CORS", wanted)
		}
	}
}
