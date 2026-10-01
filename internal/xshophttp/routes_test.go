package xshophttp

import (
	"bytes"
	"context"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestUpgradeRequiresAuthenticatedSuperAdministrator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, super := range []any{nil, false, "true", 1} {
		calls := 0
		r := gin.New()
		r.Use(func(c *gin.Context) {
			if super != nil {
				c.Set("admin_is_super", super)
				c.Set("admin_id", uint(1))
			}
		})
		Register(r.Group("/admin"), &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, nil })})
		for _, x := range []struct{ m, p string }{{"GET", "status"}, {"POST", "check"}, {"POST", "install"}} {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(x.m, "/admin/xshop-upgrade/"+x.p, nil))
			if w.Code != 403 || calls != 0 {
				t.Fatal("unauthorized helper access")
			}
		}
	}
}
func TestUpgradeProxyCanonicalPayloadAndNoURLAuthority(t *testing.T) {
	gin.SetMode(gin.TestMode)
	calls := 0
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("admin_id", uint(1)); c.Set("admin_is_super", true) })
	Register(r.Group("/admin"), &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Host != "helper" || req.URL.Path != "/check" || req.Method != "POST" {
			t.Fatal("wrong helper target")
		}
		return &http.Response{StatusCode: 202, Body: io.NopCloser(bytes.NewBufferString(`{"accepted":true}`))}, nil
	})})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/admin/xshop-upgrade/check", nil))
	if w.Code != 200 || calls != 1 {
		t.Fatal(w.Body.String())
	}
	for _, body := range []string{`{"url":"https://evil.invalid"}`, `{"digest":"abc"}`, `{"Digest":"abc"}`, `{"digest":"abc","digest":"abc"}`} {
		w = httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("POST", "/admin/xshop-upgrade/install", bytes.NewBufferString(body)))
		if w.Code != 400 || calls != 1 {
			t.Fatal("unsafe authority forwarded")
		}
	}
	_ = context.Background()
}
