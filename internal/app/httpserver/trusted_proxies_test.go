package httpserver

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestStaticUploadsConfineActiveContentAndPreserveImages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, mime string
		data       []byte
		attachment bool
	}{
		{"normal.png", "image/png", imageData.Bytes(), false},
		{"old.html", "application/octet-stream", []byte("<html><script>fixture</script></html>"), true},
		{"old.xhtml", "application/octet-stream", []byte(`<html xmlns="http://www.w3.org/1999/xhtml"/>`), true},
		{"old.js", "application/octet-stream", []byte("fixture()"), true},
		{"image.png.html", "application/octet-stream", []byte("<html>fixture</html>"), true},
		{"fixture.zip", "application/octet-stream", []byte("PK\x03\x04fixture"), true},
		{"fixture.pdf", "application/octet-stream", []byte("%PDF-1.4 fixture"), true},
		{"fixture.svg", "image/svg+xml", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect width="1" height="1"/></svg>`), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(dir, tc.name), tc.data, 0600); err != nil {
				t.Fatal(err)
			}
			r := gin.New()
			registerUploadRoutes(r, dir)
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				w := httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest(method, "/uploads/"+tc.name, nil))
				result := w.Result()
				defer result.Body.Close()
				if w.Code != http.StatusOK {
					t.Fatalf("status=%d", w.Code)
				}
				if result.Header.Get("X-Content-Type-Options") != "nosniff" {
					t.Error("missing nosniff")
				}
				csp := result.Header.Get("Content-Security-Policy")
				for _, directive := range []string{"sandbox", "default-src 'none'", "script-src 'none'", "object-src 'none'", "base-uri 'none'", "form-action 'none'"} {
					if !strings.Contains(csp, directive) {
						t.Errorf("missing CSP %s: %s", directive, csp)
					}
				}
				if result.Header.Get("Content-Type") != tc.mime {
					t.Errorf("MIME=%q want %q", result.Header.Get("Content-Type"), tc.mime)
				}
				if (result.Header.Get("Content-Disposition") == "attachment") != tc.attachment {
					t.Errorf("disposition=%q", result.Header.Get("Content-Disposition"))
				}
				if method == http.MethodGet && !bytes.Equal(w.Body.Bytes(), tc.data) {
					t.Fatal("changed bytes")
				}
			}
		})
	}
}

func TestConfigureTrustedProxiesRejectsSpoofedForwardedIPFromUntrustedPeer(t *testing.T) {
	engine := gin.New()
	if err := configureTrustedProxies(engine, nil); err != nil {
		t.Fatal(err)
	}
	engine.GET("/ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })

	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "203.0.113.9:1234"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	if resp.Body.String() != "203.0.113.9" {
		t.Fatalf("untrusted peer spoofed client IP: %q", resp.Body.String())
	}
}

func TestConfigureTrustedProxiesAcceptsForwardedIPFromConfiguredProxy(t *testing.T) {
	engine := gin.New()
	if err := configureTrustedProxies(engine, []string{"127.0.0.1/32"}); err != nil {
		t.Fatal(err)
	}
	engine.GET("/ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })

	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	if resp.Body.String() != "1.2.3.4" {
		t.Fatalf("trusted proxy client IP=%q, want 1.2.3.4", resp.Body.String())
	}
}

func TestConfigureTrustedProxiesRejectsInvalidCIDR(t *testing.T) {
	if err := configureTrustedProxies(gin.New(), []string{"not-a-cidr"}); err == nil {
		t.Fatal("expected invalid trusted proxy configuration to fail")
	}
}

func TestConfigureTrustedProxiesRejectsTrustAllNetworks(t *testing.T) {
	for _, network := range []string{"0.0.0.0/0", "::/0"} {
		t.Run(network, func(t *testing.T) {
			if err := configureTrustedProxies(gin.New(), []string{network}); err == nil {
				t.Fatalf("expected %s to be rejected", network)
			}
		})
	}
}
