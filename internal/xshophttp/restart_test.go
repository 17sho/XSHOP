package xshophttp

import (
	"bytes"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRestartUsesAuthenticatedEmptyBodyProxy(t *testing.T) {
	for _, action := range []string{"restart", "rollback"} {
		for _, super := range []bool{false, true} {
			r := gin.New()
			r.Use(func(c *gin.Context) { c.Set("admin_id", uint(1)); c.Set("admin_is_super", super) })
			calls := 0
			Register(r.Group("/admin"), &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.URL.Path != "/"+action {
					t.Fatal(req.URL.Path)
				}
				return &http.Response{StatusCode: 202, Body: io.NopCloser(bytes.NewBufferString(`{"accepted":true}`))}, nil
			})})
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("POST", "/admin/xshop-upgrade/"+action, nil))
			if super && (w.Code != 202 || calls != 1) {
				t.Fatal(w.Code, calls)
			}
			if !super && (w.Code != 403 || calls != 0) {
				t.Fatal("unauthorized restart")
			}
			w = httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("POST", "/admin/xshop-upgrade/"+action, bytes.NewBufferString(`{}`)))
			if w.Code < 400 {
				t.Fatal("nonempty restart accepted")
			}
		}
	}
}
