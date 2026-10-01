package settingshttp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/dujiao-next/internal/constants"
	settingsapp "github.com/dujiao-next/internal/modules/settings/application"
	"github.com/dujiao-next/internal/shared/jsonmap"
)

type adminSettingsStub struct {
	updateCalls int
	getCalls    int
}

func (s *adminSettingsStub) GetByKey(string) (jsonmap.JSON, error) {
	s.getCalls++
	return jsonmap.JSON{"password": "persisted-secret"}, nil
}

func (s *adminSettingsStub) UpdateWithEffects(string, map[string]interface{}) (settingsapp.UpdateResult, error) {
	s.updateCalls++
	return settingsapp.UpdateResult{}, nil
}

func (s *adminSettingsStub) InvalidateCallbackRoutesCache() {}

type settingsAuthorizerStub struct {
	allowed          bool
	err              error
	resource, method string
	calls            int
}

func (a *settingsAuthorizerStub) EnforceAdmin(_ uint, resource, method string) (bool, error) {
	a.calls++
	a.resource, a.method = resource, method
	return a.allowed, a.err
}

func TestGenericSettingsTypedDispatchAndPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		for _, allowed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/allowed=%t", method, allowed), func(t *testing.T) {
				stub := &adminSettingsStub{}
				auth := &settingsAuthorizerStub{allowed: allowed}
				handler := NewAdminHandler(stub)
				typedCalls := 0
				typed := func(c *gin.Context) {
					typedCalls++
					if method == http.MethodPut {
						var value struct {
							Enabled bool `json:"enabled"`
						}
						if err := c.ShouldBindJSON(&value); err != nil || !value.Enabled {
							t.Errorf("typed payload was not unwrapped: %+v err=%v", value, err)
						}
					}
					c.JSON(200, gin.H{"password": "", "password_configured": true})
				}
				handler.ConfigureTypedRoutes(auth, map[string]TypedSettingRoute{"smtp_config": {Resource: "/admin/settings/smtp", Get: typed, Update: typed}})
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Set("admin_id", uint(5))
				c.Request = httptest.NewRequest(method, "/api/v1/admin/settings?key=smtp_config", strings.NewReader(`{"key":" smtp_config ","value":{"enabled":true}}`))
				c.Request.Header.Set("Content-Type", "application/json")
				if method == http.MethodGet {
					handler.Get(c)
				} else {
					handler.Update(c)
				}
				wantCalls := 0
				if allowed {
					wantCalls = 1
				}
				if typedCalls != wantCalls || auth.calls != 1 || auth.resource != "/admin/settings/smtp" || auth.method != method || stub.getCalls != 0 || stub.updateCalls != 0 {
					t.Fatalf("typed calls=%d auth=%+v raw=%+v body=%s", typedCalls, auth, stub, recorder.Body.String())
				}
			})
		}
	}
}

func TestGenericSettingsPreserveTypedTemplateValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, value := range []string{
		`{"templates":{"en-US":{"subject":"First","subject":"Second"}}}`,
		`{"templates":{"en-US":{"subject":"Valid"}}}` + strings.Repeat(" ", 1024*1024),
		`{"templates":{"en-US":{"subject":"Valid"}}}} {"value":{}`,
	} {
		store := &registrationSettingsStore{values: map[string]jsonmap.JSON{}}
		svc := settingsapp.NewService(store)
		h := NewAdminHandler(svc)
		typed := NewRegistrationEmailTemplateHandler(svc)
		h.ConfigureTypedRoutes(&settingsAuthorizerStub{allowed: true}, map[string]TypedSettingRoute{
			"registration_email_template_config": {Resource: "/admin/settings/registration-email-template", Update: typed.UpdateRegistrationEmailTemplate},
		})
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Set("admin_id", uint(1))
		c.Request = httptest.NewRequest("PUT", "/api/v1/admin/settings", strings.NewReader(`{"key":"registration_email_template_config","value":`+value+`}`))
		c.Request.Header.Set("Content-Type", "application/json")
		h.Update(c)
		var result struct {
			Status int `json:"status_code"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil || result.Status != 400 || len(store.values) != 0 {
			t.Fatalf("typed validation bypass: status=%d writes=%d body=%.200s", result.Status, len(store.values), recorder.Body.String())
		}
	}
}

func TestGenericSettingsFailClosedWithoutTypedWiring(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, key := range []string{"smtp_config", "captcha_config", "telegram_auth_config", "github_auth_config", "google_auth_config", "notification_center_config", "affiliate_config", "registration_email_template_config", "order_email_template_config", "unknown_secret_config"} {
		for _, method := range []string{http.MethodGet, http.MethodPut} {
			t.Run(key+"/"+method, func(t *testing.T) {
				stub := &adminSettingsStub{}
				handler := NewAdminHandler(stub)
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(method, "/api/v1/admin/settings?key="+key, strings.NewReader(`{"key":"`+key+`","value":{"enabled":true}}`))
				c.Request.Header.Set("Content-Type", "application/json")
				if method == http.MethodGet {
					handler.Get(c)
				} else {
					handler.Update(c)
				}
				if stub.getCalls != 0 || stub.updateCalls != 0 || strings.Contains(recorder.Body.String(), "persisted-secret") {
					t.Fatalf("generic path accessed protected/unknown setting: reads=%d writes=%d body=%s", stub.getCalls, stub.updateCalls, recorder.Body.String())
				}
			})
		}
	}
}

func TestSecurityCenterStrictGenericSaveAndReadback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &registrationSettingsStore{values: map[string]jsonmap.JSON{}}
	service := settingsapp.NewService(store)
	h := NewAdminHandler(service)
	auth := &settingsAuthorizerStub{allowed: true}
	h.ConfigureTypedRoutes(auth, nil)
	request := func(method, body string) (int, jsonmap.JSON) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("admin_id", uint(1))
		c.Request = httptest.NewRequest(method, "/api/v1/admin/settings?key=security_center_config", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		if method == "PUT" {
			h.Update(c)
		} else {
			h.Get(c)
		}
		var result struct {
			Status int          `json:"status_code"`
			Data   jsonmap.JSON `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result.Status, result.Data
	}
	if status, defaults := request("GET", ""); status != 0 || len(defaults) != 6 || defaults["email_change"] != true || defaults["google_binding"] != true || len(store.values) != 0 {
		t.Fatalf("absent-row defaults=%d %+v persisted=%v", status, defaults, store.values)
	}
	for _, value := range []string{`{"email_change":"false"}`, `{"two_factor":null}`, `{"login_history":1}`, `{"unknown":true}`, `{"email_change":true,"email_change":false}`} {
		if status, _ := request("PUT", `{"key":"security_center_config","value":`+value+`}`); status != 400 || len(store.values) != 0 {
			t.Fatalf("invalid config accepted: %s status=%d", value, status)
		}
	}
	if status, _ := request("PUT", `{"key":"security_center_config","value":{},"value":{"email_change":false}}`); status != 400 || len(store.values) != 0 {
		t.Fatalf("duplicate envelope accepted: %d", status)
	}
	auth.allowed = false
	if status, _ := request("PUT", `{"key":"security_center_config","value":{"email_change":false}}`); status != 403 || len(store.values) != 0 {
		t.Fatal("raw API permission bypass")
	}
	auth.allowed = true
	status, saved := request("PUT", `{"key":"security_center_config","value":{"email_change":false}}`)
	if status != 0 || len(saved) != 6 || saved["email_change"] != false || saved["google_binding"] != true {
		t.Fatalf("save=%d %+v", status, saved)
	}
	if status, read := request("GET", ""); status != 0 || fmt.Sprint(read) != fmt.Sprint(saved) {
		t.Fatalf("readback=%d %+v", status, read)
	}
}

func TestGenericSecurityCenterRequiresTypedAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &adminSettingsStub{}
	h := NewAdminHandler(stub)
	auth := &settingsAuthorizerStub{allowed: true}
	// The security config is protected and uses the same settings resource permissions.
	h.ConfigureTypedRoutes(auth, nil)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("admin_id", uint(1))
	c.Request = httptest.NewRequest("GET", "/api/v1/admin/settings?key=security_center_config", nil)
	h.Get(c)
	if !strings.Contains(recorder.Body.String(), `"telegram_binding":true`) {
		t.Fatalf("missing normalized config: %s", recorder.Body.String())
	}
	if auth.calls != 1 || auth.resource != "/admin/settings" {
		t.Fatalf("authorization=%+v", auth)
	}
}

func TestSecurityCenterEnvelopeAliasesAndRawValue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, body := range []string{
		`{"key":"site_config","Key":"security_center_config","value":{"email_change":false}}`,
		`{"KEY":"site_config","key":"security_center_config","value":{"email_change":false}}`,
		`{"key":"site_config","Key":"security_center_config","value":{"email_change":false}}`,
		`{"key":"security_center_config","value":{},"Value":{"email_change":false}}`,
		`{"key":"security_center_config","VALUE":{},"value":{"email_change":false}}`,
		`{"key":"security_center_config","value":{"email_change":true,"email_change":false}}`,
		`{"key":"security_center_config","value":{` + strings.Repeat(" ", 4096) + `"email_change":false}}`,
	} {
		store := &registrationSettingsStore{values: map[string]jsonmap.JSON{}}
		h := NewAdminHandler(settingsapp.NewService(store))
		h.ConfigureTypedRoutes(&settingsAuthorizerStub{allowed: true}, nil)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("admin_id", uint(1))
		c.Request = httptest.NewRequest("PUT", "/api/v1/admin/settings", strings.NewReader(body))
		h.Update(c)
		var response struct {
			Status int `json:"status_code"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || response.Status != 400 || len(store.values) != 0 {
			t.Fatalf("ambiguous/raw input accepted status=%d values=%v body=%s", response.Status, store.values, body)
		}
	}
	for _, envelope := range []string{`{"key":"site_config","value":{"enabled":true}}`, `{"Key":"site_config","VALUE":{"enabled":true}}`, `{"Key":"site_config","Value":{"enabled":true}}`} {
		stub := &adminSettingsStub{}
		h := NewAdminHandler(stub)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("PUT", "/api/v1/admin/settings", strings.NewReader(envelope))
		h.Update(c)
		if stub.updateCalls != 1 {
			t.Fatalf("unprotected compatibility lost: %s response=%s", envelope, w.Body.String())
		}
	}
	var req updateRequest
	raw := `{ "email_change": true, "email_change": false }`
	if err := decodeUpdateRequest([]byte(`{"key":"security_center_config","value":`+raw+`}`), &req); err != nil || string(req.Value) != raw {
		t.Fatalf("raw value changed %s err=%v", req.Value, err)
	}
}

func TestAdminHandlerRejectsGoogleAuthOnGenericUpdate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &adminSettingsStub{}
	handler := NewAdminHandler(stub)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(
		http.MethodPut,
		"/api/v1/admin/settings",
		strings.NewReader(`{"key":"`+constants.SettingKeyGoogleAuthConfig+`","value":{"enabled":true,"client_id":""}}`),
	)
	context.Request.Header.Set("Content-Type", "application/json")

	handler.Update(context)

	if stub.updateCalls != 0 {
		t.Fatalf("generic update persisted protected Google auth config")
	}
	var body struct {
		StatusCode int    `json:"status_code"`
		Msg        string `json:"msg"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response %q: %v", recorder.Body.String(), err)
	}
	if recorder.Code != http.StatusOK || body.StatusCode != 400 {
		t.Fatalf("http=%d status_code=%d body=%s", recorder.Code, body.StatusCode, recorder.Body.String())
	}
	if !strings.Contains(body.Msg, "/admin/settings/google-auth") {
		t.Fatalf("unexpected rejection message: %q", body.Msg)
	}
}
