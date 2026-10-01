package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSensitiveResponseCachePolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sensitive := []string{
		"/orders", "/orders/:order_no", "/orders/:order_no/fulfillment/download",
		"/guest/orders", "/guest/orders/browser", "/guest/orders/:order_no/fulfillment/download",
		"/admin/orders", "/admin/orders/:id/fulfillment/download", "/admin/order-refunds/:id",
		"/admin/fulfillments", "/admin/card-secrets", "/admin/card-secrets/export", "/admin/card-secrets/export-available",
		"/admin/gift-cards/export", "/admin/procurement-orders/:id/upstream-payload/download",
		"/reseller/orders/:order_no", "/upstream/orders/:id", "/channel/orders/by-order-no/:order_no",
		"/gift-cards/redeem", "/channel/wallet/gift-card/redeem",
		"/payments/latest", "/guest/payments/latest", "/admin/payments/export",
		"/channel/payments", "/channel/payments/latest", "/channel/payments/:id",
	}
	for _, route := range sensitive {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodHead} {
			for _, status := range []int{http.StatusOK, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound} {
				t.Run(method+route+http.StatusText(status), func(t *testing.T) {
					r := gin.New()
					api := r.Group("/api/v1", SensitiveResponseCacheMiddleware())
					api.Handle(method, route, func(c *gin.Context) {
						// Simulates both direct downloads and auth failures before handlers.
						if status != http.StatusOK {
							c.AbortWithStatus(status)
							return
						}
						c.Header("Content-Disposition", "attachment; filename=fixture.txt")
						c.Data(status, "text/plain", []byte("FIXTURE-SECRET"))
					})
					url := "/api/v1" + strings.ReplaceAll(strings.ReplaceAll(route, ":id", "42"), ":order_no", "fixture-order")
					w := httptest.NewRecorder()
					r.ServeHTTP(w, httptest.NewRequest(method, url, nil))
					header := w.Result().Header // Snapshot at write time, not after c.Next.
					if header.Get("Cache-Control") != "private, no-store" {
						t.Errorf("cache=%q", header.Get("Cache-Control"))
					}
					if w.Code != status {
						t.Errorf("status=%d want=%d", w.Code, status)
					}
				})
			}
		}
	}
}

func TestSensitiveResponseCacheLeavesPublicAndUnrelatedRoutesAlone(t *testing.T) {
	for _, route := range []string{"/public/products", "/public/products/:slug", "/public/categories", "/public/posts", "/public/config", "/upstream/products", "/channel/products", "/orders-other", "/admin/products", "/admin/card-secrets-other"} {
		t.Run(route, func(t *testing.T) {
			r := gin.New()
			api := r.Group("/api/v1", SensitiveResponseCacheMiddleware())
			api.GET(route, func(c *gin.Context) {
				if c.Writer.Header().Get("Cache-Control") != "" {
					t.Error("unrelated route modified")
				}
				c.Header("Cache-Control", "public, max-age=300")
				c.String(http.StatusOK, "fixture catalog")
			})
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1"+strings.ReplaceAll(route, ":slug", "fixture"), nil))
			if w.Result().Header.Get("Cache-Control") != "public, max-age=300" {
				t.Error("public policy changed")
			}
		})
	}
}

func TestResolveAllowedOrigin(t *testing.T) {
	got := resolveAllowedOrigin("https://example.com", []string{"*"}, false)
	if got != "*" {
		t.Fatalf("wildcard without credentials should return *, got %s", got)
	}

	got = resolveAllowedOrigin("https://example.com", []string{"*"}, true)
	if got != "https://example.com" {
		t.Fatalf("wildcard with credentials should echo origin, got %s", got)
	}

	got = resolveAllowedOrigin("https://a.example.com", []string{"https://a.example.com", "https://b.example.com"}, false)
	if got != "https://a.example.com" {
		t.Fatalf("allow-list should return matched origin, got %s", got)
	}

	got = resolveAllowedOrigin("https://x.example.com", []string{"https://a.example.com"}, false)
	if got != "" {
		t.Fatalf("unmatched origin should be empty, got %s", got)
	}
}

func TestRequestIDMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(RequestIDMiddleware())
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"request_id": getRequestID(c)})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set(requestIDHeader, "req-123")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status want 200 got %d", w.Code)
	}
	if w.Header().Get(requestIDHeader) != "req-123" {
		t.Fatalf("response request id want req-123 got %s", w.Header().Get(requestIDHeader))
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if resp["request_id"] != "req-123" {
		t.Fatalf("context request id want req-123 got %s", resp["request_id"])
	}

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/ping", nil)
	r.ServeHTTP(w2, req2)
	generated := w2.Header().Get(requestIDHeader)
	if generated == "" {
		t.Fatalf("generated request id should not be empty")
	}
	if resp := strings.TrimSpace(generated); resp == "" {
		t.Fatalf("generated request id should not be blank")
	}
}

func TestJWTAuthMiddlewareMissingSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(JWTAuthMiddleware("", nil))
	r.GET("/admin/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/ping", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status want 200 got %d", w.Code)
	}
	var resp struct {
		StatusCode int `json:"status_code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if resp.StatusCode != 401 {
		t.Fatalf("status_code want 401 got %d", resp.StatusCode)
	}
}
