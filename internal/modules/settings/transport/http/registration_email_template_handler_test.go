package settingshttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	settingsapp "github.com/dujiao-next/internal/modules/settings/application"
	settingsmessaging "github.com/dujiao-next/internal/modules/settings/schema/messaging"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/gin-gonic/gin"
)

type registrationSettingsStore struct{ values map[string]jsonmap.JSON }

func (s *registrationSettingsStore) GetByKey(key string) (jsonmap.JSON, bool, error) {
	v, ok := s.values[key]
	return v, ok, nil
}
func (s *registrationSettingsStore) Upsert(key string, v jsonmap.JSON) (jsonmap.JSON, error) {
	s.values[key] = v
	return v, nil
}

func TestRegistrationEmailTemplateAdminRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &registrationSettingsStore{values: map[string]jsonmap.JSON{}}
	service := settingsapp.NewService(store)
	r := gin.New()
	admin := r.Group("/api/v1/admin", func(c *gin.Context) {
		if c.GetHeader("X-Test-Admin") != "yes" {
			c.AbortWithStatus(http.StatusUnauthorized)
		}
	})
	RegisterAdminRegistrationEmailTemplateRoutes(admin, NewRegistrationEmailTemplateHandler(service))
	path := "/api/v1/admin/settings/registration-email-template"
	request := func(method, url, body string, authorized bool) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, url, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if authorized {
			req.Header.Set("X-Test-Admin", "yes")
		}
		r.ServeHTTP(w, req)
		return w
	}
	for _, method := range []string{"GET", "PUT"} {
		if w := request(method, path, `{}`, false); w.Code != 401 {
			t.Fatalf("unprotected %s: %d", method, w.Code)
		}
	}
	decode := func(w *httptest.ResponseRecorder) settingsmessaging.RegistrationEmailTemplateSetting {
		t.Helper()
		if w.Code != 200 {
			t.Fatalf("request: %d %s", w.Code, w.Body.String())
		}
		var result struct {
			Data settingsmessaging.RegistrationEmailTemplateSetting `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result.Data
	}
	defaults := decode(request("GET", path, "", true))
	if !reflect.DeepEqual(defaults, settingsmessaging.DefaultRegistrationEmailTemplateSetting()) || len(store.values) != 0 {
		t.Fatal("GET must return absent defaults without writing")
	}
	saved := decode(request("PUT", path, `{"templates":{"en-US":{"subject":"Custom {{site_name}}"}}}`, true))
	if saved.Templates.ENUS.Subject != "Custom {{site_name}}" || saved.Templates.ZHCN != defaults.Templates.ZHCN {
		t.Fatal("PUT lost sparse fields")
	}
	saved = decode(request("PUT", path, `{"scenes":{"reset":{"en-US":{"subject":"Reset {{site_name}}"}}}}`, true))
	if saved.Templates.ENUS.Subject != "Custom {{site_name}}" || saved.Scenes["reset"].ENUS.Subject != "Reset {{site_name}}" {
		t.Fatal("scene PUT lost registration or scene values")
	}
	saved = decode(request("PUT", path, `{"templates":{"zh-TW":{"subject":"Legacy registration update"}}}`, true))
	if saved.Scenes["reset"].ENUS.Subject != "Reset {{site_name}}" {
		t.Fatal("legacy PUT erased scene settings")
	}
	if read := decode(request("GET", path, "", true)); !reflect.DeepEqual(read, saved) {
		t.Fatal("GET differs from saved")
	}
	if read := decode(request("GET", path+"/defaults", "", true)); !reflect.DeepEqual(read, defaults) {
		t.Fatal("defaults endpoint modified by persisted settings")
	}
	for _, body := range []string{`{"templates":{"en-US":{"subject":"bad\r\nBcc: evil"}}}`, `{"templates":null}`, `{"templates":{"en-US":{"body":"no code"}}}`, `{"unknown":true}`, `{} {}`, strings.Repeat(" ", 1024*1024+1) + `{}`} {
		w := request("PUT", path, body, true)
		var envelope struct {
			StatusCode int `json:"status_code"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil || envelope.StatusCode != 400 {
			t.Fatalf("accepted invalid update %.100s: %d %s", body, w.Code, w.Body.String())
		}
	}
	if read := decode(request("GET", path, "", true)); !reflect.DeepEqual(read, saved) {
		t.Fatal("invalid PUT mutated settings")
	}
	if w := request("GET", "/api/v1/settings/registration-email-template", "", true); w.Code != 404 {
		t.Fatal("public exposure")
	}
	if w := request("POST", path+"/reset", "", true); w.Code != 404 {
		t.Fatal("unexpected reset route")
	}
}
