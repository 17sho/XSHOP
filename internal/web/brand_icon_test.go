package web

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestFirstPaintConfiguredIcon(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	icon := "/uploads/brand.png"
	err := RegisterUser(r, fstest.MapFS{"index.html": {Data: []byte("<html><head></head></html>")}, "favicon.ico": {Data: []byte("DEFAULT")}}, func(c *gin.Context) (string, error) { return icon, nil })
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"/", "/guest/orders", "/index.html"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", target, nil))
		if !strings.Contains(w.Body.String(), `href="/uploads/brand.png"`) {
			t.Fatalf("missing first paint icon: %s", w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/favicon.ico", nil))
	if w.Code != 302 || w.Header().Get("Location") != icon {
		t.Fatalf("default leaked: %d %s", w.Code, w.Body.String())
	}
	icon = "/uploads/new.png"
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if !strings.Contains(w.Body.String(), `href="/uploads/new.png"`) {
		t.Fatal("stale icon")
	}
}
