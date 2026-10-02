package web

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

func TestEmbeddedIngressDeniesMissingAndSensitivePaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if err := RegisterAdmin(r, "/ops", newAdminFS("<html>admin</html>")); err != nil {
		t.Fatal(err)
	}
	if err := RegisterUser(r, newUserFS()); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/assets/missing.js", "/.env", "/.git/config", "/config.yaml", "/assets/../.env", "/%2eenv", "//.env", "/ops/assets/missing.js", "/ops/.env", "/ops/%2eenv", "/ops//.env"} {
		t.Run(p, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", p, nil))
			if w.Code != 404 {
				t.Fatalf("%s status=%d want 404", p, w.Code)
			}
		})
	}
}
