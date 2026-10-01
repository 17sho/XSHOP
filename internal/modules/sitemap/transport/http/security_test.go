package sitemaphttp

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

func TestSitemapIgnoresUntrustedForwardingHeaders(t *testing.T) {
	h := NewHandler(&fakeGenerator{}, fakeBrand{})
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "http://store.example/sitemap.xml", nil)
	c.Request.RemoteAddr = "203.0.113.10:1234"
	c.Request.Header.Set("X-Forwarded-Host", "attacker.example")
	c.Request.Header.Set("X-Forwarded-Proto", "javascript")
	if got := h.resolveBaseURL(c); got != "http://store.example" {
		t.Fatalf("untrusted base: %s", got)
	}
}
