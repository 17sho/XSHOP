//go:build fullstack

package web

import (
	"github.com/gin-gonic/gin"
	"io/fs"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuiltEmbeddedFrontendsDeliverLogoutAndAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if err := RegisterAdmin(r, "/ops", AdminFS()); err != nil {
		t.Fatal(err)
	}
	if err := RegisterUser(r, UserFS()); err != nil {
		t.Fatal(err)
	}
	found := false
	fs.WalkDir(UserFS(), "assets", func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !d.IsDir() && strings.HasPrefix(d.Name(), "PersonalCenter-") {
			b, _ := fs.ReadFile(UserFS(), p)
			if strings.Contains(string(b), "personal-logout") {
				w := httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest("GET", "/"+p, nil))
				if w.Code != 200 || !strings.Contains(w.Body.String(), "personal-logout") {
					t.Fatal("embedded logout asset unavailable")
				}
				found = true
			}
		}
		return nil
	})
	if !found {
		t.Fatal("built user frontend lacks logout")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/ops/login", nil))
	if w.Code != 200 || strings.Contains(w.Body.String(), "__DJ_ADMIN_BASE__") {
		t.Fatal("admin base not resolved")
	}
}
