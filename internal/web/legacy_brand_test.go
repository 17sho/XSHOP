package web

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLegacyRootBrandAsset(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	os.Mkdir("storefront-current", 0700)
	os.WriteFile(filepath.Join("storefront-current", "custom-logo.svg"), []byte("<svg>brand</svg>"), 0600)
	os.WriteFile(filepath.Join("storefront-current", "private.txt"), []byte("private"), 0600)
	os.Symlink(filepath.Join(dir, "outside.svg"), filepath.Join("storefront-current", "escape.svg"))
	os.WriteFile("outside.svg", []byte("secret"), 0600)
	r := gin.New()
	RegisterUser(r, fstest.MapFS{"index.html": {Data: []byte("SPA")}})
	for _, tc := range []struct {
		path string
		want int
		body string
	}{{"/custom-logo.svg", 200, "<svg>brand</svg>"}, {"/escape.svg", 404, ""}, {"/private.txt", 200, "SPA"}} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != tc.want || tc.body != "" && !strings.Contains(w.Body.String(), tc.body) {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
	}
}
