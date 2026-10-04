package transport_test

import (
	"encoding/base64"
	"github.com/dujiao-next/internal/config"
	captchaapp "github.com/dujiao-next/internal/modules/captcha/application"
	captcha "github.com/dujiao-next/internal/modules/captcha/contract"
	captchahttp "github.com/dujiao-next/internal/modules/captcha/transport/http"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	orderhttp "github.com/dujiao-next/internal/modules/order/transport/http"
	reseller "github.com/dujiao-next/internal/modules/reseller/contract"
	security "github.com/dujiao-next/internal/modules/settings/schema/security"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

type lookupQuerySpy struct {
	browserOrderQueryStub
	calls           int
	email, password string
}

func (s *lookupQuerySpy) ListOrdersByGuestForTenant(_ reseller.TenantContext, email, password string, _, _ int) ([]orderdomain.Order, int64, error) {
	s.calls++
	s.email = email
	s.password = password
	return nil, 0, nil
}
func (s *lookupQuerySpy) GetOrderByGuestOrderNoForTenant(_ reseller.TenantContext, _, email, password string) (*orderdomain.Order, error) {
	s.calls++
	s.email = email
	s.password = password
	return &orderdomain.Order{}, nil
}

type lookupSettings struct {
	provider string
	enabled  bool
}

func (s lookupSettings) GetCaptchaSetting(config.CaptchaConfig) (security.CaptchaSetting, error) {
	return security.NormalizeCaptchaSetting(security.CaptchaSetting{Provider: s.provider, Scenes: security.CaptchaSceneSetting{GuestCreateOrder: s.enabled}}), nil
}

type lookupTurnstile struct{}

func (lookupTurnstile) Verify(_ security.CaptchaTurnstileSetting, token, _ string) error {
	if token != "valid-fixture-token" {
		return captcha.ErrInvalid
	}
	return nil
}
func requestLookup(t *testing.T, email, query, provider, token string, enabled bool) (*lookupQuerySpy, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	spy := &lookupQuerySpy{}
	service := captchaapp.NewService(lookupSettings{provider, enabled}, config.CaptchaConfig{}, lookupTurnstile{})
	r := gin.New()
	orderhttp.RegisterGuestReadRoutes(r, orderhttp.NewGuestHandler(spy, nil, nil, captchahttp.NewVerifier(service)))
	req := httptest.NewRequest("GET", "/orders"+query, nil)
	req.Header.Set("Authorization", "Guest "+base64.RawURLEncoding.EncodeToString([]byte(email+"\npassword")))
	req.Header.Set("X-Turnstile-Token", token)
	if provider == "image" && token != "" {
		req.Header.Set("X-Captcha-ID", "fake-id")
		req.Header.Set("X-Captcha-Code", token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return spy, w.Body.String()
}
func TestGuestLookupFailsClosedWithoutCaptchaVerifier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	spy := &lookupQuerySpy{}
	orderhttp.RegisterGuestReadRoutes(r, orderhttp.NewGuestHandler(spy, nil, nil))
	req := httptest.NewRequest("GET", "/orders", nil)
	req.Header.Set("Authorization", "Guest "+base64.RawURLEncoding.EncodeToString([]byte("fixture@example.invalid\npassword")))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), `"status_code":500`) || spy.calls != 0 {
		t.Fatalf("lookup must fail closed: %s calls=%d", w.Body.String(), spy.calls)
	}
}
func TestGuestLookupInvalidEmailRejectedBeforeCaptcha(t *testing.T) {
	for _, email := range []string{"123", "fixture@", "   "} {
		spy, body := requestLookup(t, email, "", "turnstile", "", true)
		if !strings.Contains(body, `"status_code":400`) || spy.calls != 0 {
			t.Fatalf("invalid email %q queried: %s", email, body)
		}
		if email != "   " && !strings.Contains(body, "邮箱格式不正确") {
			t.Fatalf("email must be checked before missing captcha: %s", body)
		}
	}
}
func TestGuestLookupCaptchaBeforeEveryQueryBranch(t *testing.T) {
	for _, query := range []string{"", "?order_no=FIXTURE"} {
		for _, provider := range []string{"image", "turnstile"} {
			for _, token := range []string{"", "wrong"} {
				spy, body := requestLookup(t, "fixture@example.invalid", query, provider, token, true)
				if !strings.Contains(body, `"status_code":400`) || spy.calls != 0 {
					t.Fatalf("captcha bypass %s %s %q: %s calls=%d", query, provider, token, body, spy.calls)
				}
			}
		}
		spy, body := requestLookup(t, "  Fixture@Example.invalid  ", query, "turnstile", "valid-fixture-token", true)
		if !strings.Contains(body, `"status_code":0`) || spy.calls != 1 || spy.email != "fixture@example.invalid" || spy.password != "password" {
			t.Fatalf("valid lookup failed: %s spy=%+v", body, spy)
		}
	}
}
func TestGuestLookupDisabledGuestScenePreservesCredentials(t *testing.T) {
	spy, body := requestLookup(t, "fixture@example.invalid", "", "image", "", false)
	if !strings.Contains(body, `"status_code":0`) || spy.calls != 1 {
		t.Fatalf("disabled scene failed: %s", body)
	}
}
