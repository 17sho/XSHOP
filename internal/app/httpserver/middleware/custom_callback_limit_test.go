package middleware

import (
	settingsintegration "github.com/dujiao-next/internal/modules/settings/schema/integration"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"testing"
)

type customCallbackSettingsStub struct {
	routes *settingsintegration.CallbackRoutesSetting
}

func (s customCallbackSettingsStub) GetCallbackRoutesCached() *settingsintegration.CallbackRoutesSetting {
	return s.routes
}
func TestCustomCallbacksReceiveLimitWithoutLimitingOtherRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	settings := customCallbackSettingsStub{&settingsintegration.CallbackRoutesSetting{PaymentCallback: "/api/custom/payment", PaypalWebhook: "/api/custom/paypal", UpstreamCallback: "/api/custom/upstream"}}
	for _, path := range []string{"/api/custom/payment", "/api/custom/paypal", "/api/custom/upstream"} {
		r := gin.New()
		r.Use(CustomCallbackRateLimitMiddleware(settings, nil, RateLimitRule{WindowSeconds: 60, MaxRequests: 1, BlockSeconds: 60}))
		r.Any("/*path", func(c *gin.Context) { c.Status(http.StatusNoContent) })
		request := func(p string) *httptest.ResponseRecorder {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, p, nil))
			return w
		}
		if w := request(path); w.Code != 204 {
			t.Fatalf("first callback = %d", w.Code)
		}
		if w := request(path); w.Code == 204 {
			t.Fatal("custom callback bypassed limit")
		}
		for i := 0; i < 3; i++ {
			if w := request("/api/v1/public/config"); w.Code != 204 {
				t.Fatalf("public request limited: %d", w.Code)
			}
		}
	}
}
func TestNoCustomCallbackConfigIsPassThrough(t *testing.T) {
	r := gin.New()
	r.Use(CustomCallbackRateLimitMiddleware(customCallbackSettingsStub{}, nil, RateLimitRule{WindowSeconds: 60, MaxRequests: 1}))
	r.GET("/api/v1/payments/callback", func(c *gin.Context) { c.Status(204) })
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/payments/callback", nil))
		if w.Code != 204 {
			t.Fatal(w.Code)
		}
	}
}
