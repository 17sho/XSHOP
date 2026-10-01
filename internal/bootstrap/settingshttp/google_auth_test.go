package settingsbootstrap

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dujiao-next/internal/app/container"
	"github.com/dujiao-next/internal/app/httpserver/middleware"
	"github.com/dujiao-next/internal/authz"
	notificationapp "github.com/dujiao-next/internal/modules/notification/application"
	notificationcontract "github.com/dujiao-next/internal/modules/notification/contract"
	notificationsmtp "github.com/dujiao-next/internal/modules/notification/infrastructure/smtp"
	notificationtransport "github.com/dujiao-next/internal/modules/notification/transport/http"
	settingsmessaging "github.com/dujiao-next/internal/modules/settings/schema/messaging"
	settingstransport "github.com/dujiao-next/internal/modules/settings/transport/http"
	"github.com/dujiao-next/internal/shared/mailbrand"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/constants"
	settingsapp "github.com/dujiao-next/internal/modules/settings/application"
	settingssecurity "github.com/dujiao-next/internal/modules/settings/schema/security"
	"github.com/dujiao-next/internal/shared/jsonmap"
)

type googleAuthSettingStore struct {
	mu    sync.RWMutex
	store map[string]jsonmap.JSON
}

func (s *googleAuthSettingStore) GetByKey(key string) (jsonmap.JSON, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.store[key]
	if !ok {
		return nil, false, nil
	}
	copyValue := make(jsonmap.JSON, len(value))
	for field, item := range value {
		copyValue[field] = item
	}
	return copyValue, true, nil
}

func (s *googleAuthSettingStore) Upsert(key string, value jsonmap.JSON) (jsonmap.JSON, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	copyValue := make(jsonmap.JSON, len(value))
	for field, item := range value {
		copyValue[field] = item
	}
	s.store[key] = copyValue
	return copyValue, nil
}

type googleAuthRuntimeStub struct {
	mu  sync.RWMutex
	cfg config.GoogleAuthConfig
}

func (s *googleAuthRuntimeStub) SetConfig(cfg config.GoogleAuthConfig) {
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
}

func (s *googleAuthRuntimeStub) snapshot() config.GoogleAuthConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

type genericSettingsFixture struct {
	handler *settingstransport.AdminHandler
	service *settingsapp.Service
	store   *googleAuthSettingStore
	auth    *authz.Service
	cfg     *config.Config
	email   *notificationsmtp.Service
}

func newGenericSettingsFixture(t *testing.T) genericSettingsFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	auth, err := authz.NewService(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.BootstrapBuiltinRoles(); err != nil {
		t.Fatal(err)
	}
	if err := auth.SetAdminRoles(1, []string{"system_admin"}); err != nil {
		t.Fatal(err)
	}
	if err := auth.GrantRolePolicy("generic_only", "/admin/settings", "*"); err != nil {
		t.Fatal(err)
	}
	if err := auth.SetAdminRoles(2, []string{"generic_only"}); err != nil {
		t.Fatal(err)
	}
	store := &googleAuthSettingStore{store: map[string]jsonmap.JSON{}}
	service := settingsapp.NewService(store)
	cfg := &config.Config{Email: config.EmailConfig{Enabled: true, Host: "smtp.invalid", Port: 587, From: "shop@example.invalid", Password: "smtp-fixture-secret"}}
	email := notificationsmtp.New(&cfg.Email)
	c := &container.Container{SettingService: service, AuthzService: auth, EmailSender: email}
	notifications := notificationtransport.NewAdminHandler(service, &notificationapp.LogService{}, &notificationapp.Service{})
	handler := settingstransport.NewAdminHandler(service)
	ConfigureAdminHandler(handler, c, cfg, notifications)
	return genericSettingsFixture{handler: handler, service: service, store: store, auth: auth, cfg: cfg, email: email}
}

func (f genericSettingsFixture) request(t *testing.T, method, key, value string, admin uint) (int, string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	r := gin.New()
	group := r.Group("/api/v1/admin", func(c *gin.Context) { c.Set("admin_id", admin) }, middleware.AdminRBACMiddleware(f.auth))
	settingstransport.RegisterAdminRoutes(group, f.handler)
	request := httptest.NewRequest(method, "/api/v1/admin/settings?key="+key, strings.NewReader(`{"key":"`+key+`","value":`+value+`}`))
	request.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(recorder, request)
	var envelope struct {
		Status int `json:"status_code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope.Status, recorder.Body.String()
}

func TestGenericSettingsBootstrapTypedContracts(t *testing.T) {
	f := newGenericSettingsFixture(t)
	fixtures := []struct {
		key, resource, valid, invalid string
		seed                          jsonmap.JSON
		secrets                       []string
	}{
		{"smtp_config", "smtp", `{"enabled":false,"password":""}`, `{"use_tls":true,"use_ssl":true}`, nil, []string{"smtp-fixture-secret"}},
		{"captcha_config", "captcha", `{"provider":"none","turnstile":{"secret_key":""}}`, `{"provider":"turnstile","turnstile":{"site_key":""}}`, jsonmap.JSON{"turnstile": map[string]interface{}{"secret_key": "captcha-fixture-secret"}}, []string{"captcha-fixture-secret"}},
		{"telegram_auth_config", "telegram-auth", `{"enabled":false,"bot_token":"","client_secret":""}`, `{"enabled":true,"bot_username":""}`, jsonmap.JSON{"bot_token": "12345:telegram-fixture-secret", "client_secret": "telegram-oidc-fixture-secret"}, []string{"telegram-fixture-secret", "telegram-oidc-fixture-secret"}},
		{"notification_center_config", "notification-center", `{"channels":{"feishu":{"app_secret":""}}}`, `{"channels":{"feishu":{"enabled":true,"app_id":""}}}`, jsonmap.JSON{"channels": map[string]interface{}{"feishu": map[string]interface{}{"app_secret": "feishu-fixture-secret"}}}, []string{"feishu-fixture-secret"}},
		{"registration_email_template_config", "registration-email-template", `{"templates":{"en-US":{"subject":"Welcome {{site_name}}"}}}`, `{"templates":{"en-US":{"body":"missing required variables"}}}`, nil, nil},
		{"order_email_template_config", "order-email-template", `{"modules":{"footer":false}}`, `{"templates":{"paid":{"en-US":{"custom_html":"<script>bad()</script>","custom_html_enabled":true}}}}`, nil, nil},
		{"affiliate_config", "affiliate", `{"enabled":true,"commission_rate":5}`, `{"commission_rate":"bad"}`, nil, nil},
		{"github_auth_config", "github-auth", "", "", jsonmap.JSON{"client_secret": "github-fixture-secret"}, []string{"github-fixture-secret"}},
		{"google_auth_config", "google-auth", "", "", nil, nil},
	}
	for _, item := range fixtures {
		t.Run(item.key, func(t *testing.T) {
			if item.seed != nil {
				if _, err := f.store.Upsert(item.key, item.seed); err != nil {
					t.Fatal(err)
				}
			}
			before, _, _ := f.store.GetByKey(item.key)
			for _, method := range []string{"GET", "PUT"} {
				status, body := f.request(t, method, item.key, `{}`, 2)
				want := 403
				if method == "PUT" && (item.key == "google_auth_config" || item.key == "github_auth_config") {
					want = 400
				}
				if status != want {
					t.Fatalf("limited generic role status=%d want=%d body=%s", status, want, body)
				}
			}
			after, _, _ := f.store.GetByKey(item.key)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("limited-role request changed settings")
			}
			status, body := f.request(t, "GET", item.key, `{}`, 1)
			if status != 0 {
				t.Fatalf("authorized read rejected: %s", body)
			}
			for _, secret := range item.secrets {
				if strings.Contains(body, secret) {
					t.Fatalf("secret leaked: %s", body)
				}
			}
			// A grant for this typed GET must not imply its PUT authority.
			if err := f.auth.GrantRolePolicy("typed_reader", "/admin/settings", "GET"); err != nil {
				t.Fatal(err)
			}
			if err := f.auth.GrantRolePolicy("typed_reader", "/admin/settings", "PUT"); err != nil {
				t.Fatal(err)
			}
			if err := f.auth.GrantRolePolicy("typed_reader", "/admin/settings/"+item.resource, "GET"); err != nil {
				t.Fatal(err)
			}
			if err := f.auth.SetAdminRoles(3, []string{"typed_reader"}); err != nil {
				t.Fatal(err)
			}
			if status, body := f.request(t, "GET", item.key, `{}`, 3); status != 0 {
				t.Fatalf("key-specific reader denied: %s", body)
			}
			if status, body := f.request(t, "PUT", item.key, `{}`, 3); status != 403 && status != 400 {
				t.Fatalf("reader wrote protected config: %s", body)
			}
			if item.valid == "" {
				if status, body := f.request(t, "PUT", item.key, `{}`, 1); status != 400 {
					t.Fatalf("OAuth generic write contract changed: %s", body)
				}
				return
			}
			status, body = f.request(t, "PUT", item.key, item.valid, 1)
			if status != 0 {
				t.Fatalf("authorized valid update rejected: %s", body)
			}
			for _, secret := range item.secrets {
				if strings.Contains(body, secret) {
					t.Fatalf("write response secret leaked: %s", body)
				}
			}
			persisted, found, err := f.store.GetByKey(item.key)
			if err != nil || !found {
				t.Fatalf("not persisted: found=%t err=%v", found, err)
			}
			encoded, err := json.Marshal(persisted)
			if err != nil {
				t.Fatal(err)
			}
			for _, secret := range item.secrets {
				if !strings.Contains(string(encoded), secret) {
					t.Fatal("blank masked secret input erased persisted credential")
				}
			}
			status, body = f.request(t, "PUT", item.key, item.invalid, 1)
			if status != 400 {
				t.Fatalf("typed invalid update not rejected: status=%d body=%s", status, body)
			}
			after, _, _ = f.store.GetByKey(item.key)
			if !reflect.DeepEqual(persisted, after) {
				t.Fatal("invalid update changed persisted setting")
			}
			// Read back through the generic path, not only the internal store.
			if status, body := f.request(t, "GET", item.key, `{}`, 1); status != 0 {
				t.Fatalf("readback failed: %s", body)
			}
		})
	}
	if cfg := f.email.ConfigSnapshot(); cfg.Enabled || cfg.Password != "smtp-fixture-secret" {
		t.Fatalf("SMTP runtime not applied / blank secret lost: %+v", cfg)
	}
	if f.cfg.Captcha.Provider != "none" || f.cfg.Captcha.Turnstile.SecretKey != "captcha-fixture-secret" {
		t.Fatal("captcha runtime or secret not preserved")
	}
	if f.cfg.TelegramAuth.Enabled || f.cfg.TelegramAuth.BotToken != "12345:telegram-fixture-secret" || f.cfg.TelegramAuth.ClientSecret != "telegram-oidc-fixture-secret" {
		t.Fatal("Telegram runtime or secrets not preserved")
	}
}

func TestGenericSettingsOrdinaryFrontendOperations(t *testing.T) {
	f := newGenericSettingsFixture(t)
	cases := []struct{ key, value string }{
		{"site_config", `{"brand":{"site_name":"Preview Shop","site_logo":"/uploads/logo.png"},"currency":"USD"}`},
		{"order_config", `{"payment_expire_minutes":45,"max_refund_days":20}`},
		{"nav_config", `{"homepage_notice_enabled":false,"builtin":{"notice":false},"custom_items":[{"id":1,"title":{"en-US":"Help"},"link_type":"external","url":"https://example.invalid/help","target":"_blank","sort_order":0,"enabled":true,"icon":"link"}]}`},
		{"dashboard_config", `{"alert":{"low_stock_threshold":8}}`},
		{"registration_config", `{"registration_enabled":false,"email_verification_enabled":true,"email_domain_allowlist_enabled":true,"allowed_email_domains":["example.invalid"]}`},
		{"wallet_config", `{"enabled":true,"recharge_enabled":false}`},
		{"payment_config", `{"fee_mode":"merchant"}`},
		{"personal_center_visibility_config", `{"wallet":true}`},
		{"order_risk_control_config", `{"enabled":false}`},
		{"upstream_sync_config", `{"enabled":true,"interval_minutes":30}`},
		{"callback_routes_config", `{"enabled":false}`},
		{"home_announcement", `{"enabled":false}`},
		{"homepage_ad", `{"enabled":false}`},
	}
	for _, item := range cases {
		t.Run(item.key, func(t *testing.T) {
			status, written := f.request(t, "PUT", item.key, item.value, 2)
			if status != 0 {
				t.Fatalf("ordinary generic write denied: %s", written)
			}
			status, read := f.request(t, "GET", item.key, `{}`, 2)
			if status != 0 || read != written {
				t.Fatalf("ordinary generic readback mismatch: write=%s read=%s", written, read)
			}
			stored, found, err := f.store.GetByKey(item.key)
			if !found || err != nil {
				t.Fatalf("not stored: found=%t err=%v", found, err)
			}
			// Check actual frontend fields, not just a successful normalized no-op.
			switch item.key {
			case "site_config":
				encoded, _ := json.Marshal(stored)
				if !strings.Contains(string(encoded), "Preview Shop") || !strings.Contains(string(encoded), "/uploads/logo.png") || stored["currency"] != "USD" {
					t.Fatalf("branding lost: %s", encoded)
				}
			case "nav_config":
				encoded, _ := json.Marshal(stored)
				if stored["homepage_notice_enabled"] != false || !strings.Contains(string(encoded), "https://example.invalid/help") || !strings.Contains(string(encoded), `"notice":false`) {
					t.Fatalf("navigation lost: %s", encoded)
				}
			case "order_config":
				encoded, _ := json.Marshal(stored)
				if !strings.Contains(string(encoded), `"payment_expire_minutes":45`) || !strings.Contains(string(encoded), `"max_refund_days":20`) {
					t.Fatalf("order policy lost: %s", encoded)
				}
			case "registration_config":
				if stored["registration_enabled"] != false || stored["email_verification_enabled"] != true {
					t.Fatalf("registration policy lost: %+v", stored)
				}
			}
		})
	}
	// No privileged endpoint grant is required for branding, but the ordinary
	// route permission itself is still enforced by the real RBAC middleware.
	if status, body := f.request(t, "GET", "site_config", `{}`, 99); status != 403 {
		t.Fatalf("ungranted admin read branding: %s", body)
	}
	for _, method := range []string{"GET", "PUT"} {
		if status, body := f.request(t, method, "unknown_secret_config", `{}`, 1); status != 403 {
			t.Fatalf("unknown key accepted: %s", body)
		}
	}
}

func TestGenericSettingsMasksStartupDefaultsWithoutPersistedRows(t *testing.T) {
	f := newGenericSettingsFixture(t)
	f.cfg.Captcha.Turnstile.SecretKey = "startup-captcha-secret"
	f.cfg.TelegramAuth.BotToken = "12345:startup-telegram-secret"
	f.cfg.TelegramAuth.ClientSecret = "startup-oidc-secret"
	f.cfg.GitHubAuth.ClientSecret = "startup-github-secret"
	for _, item := range []struct {
		key        string
		secrets    []string
		indicators []string
	}{
		{"smtp_config", []string{"smtp-fixture-secret"}, []string{`"has_password":true`, `"password":""`}},
		{"captcha_config", []string{"startup-captcha-secret"}, []string{`"has_secret":true`, `"secret_key":""`}},
		{"telegram_auth_config", []string{"startup-telegram-secret", "startup-oidc-secret"}, []string{`"has_bot_token":true`, `"has_client_secret":true`}},
		{"github_auth_config", []string{"startup-github-secret"}, []string{`"client_secret_configured":true`, `"client_secret":""`}},
	} {
		t.Run(item.key, func(t *testing.T) {
			status, body := f.request(t, "GET", item.key, `{}`, 1)
			if status != 0 {
				t.Fatalf("default read rejected: %s", body)
			}
			for _, secret := range item.secrets {
				if strings.Contains(body, secret) {
					t.Fatalf("default secret leaked: %s", body)
				}
			}
			for _, indicator := range item.indicators {
				if !strings.Contains(body, indicator) {
					t.Fatalf("masked default contract lost %s: %s", indicator, body)
				}
			}
			if _, found, _ := f.store.GetByKey(item.key); found {
				t.Fatal("GET persisted a default")
			}
		})
	}
}

func TestSMTPRuntimeUpdatesKeepStartupFallbackImmutable(t *testing.T) {
	f := newGenericSettingsFixture(t)
	initial := f.cfg.Email
	adapter := settingsSMTPAdapter{settings: f.service, cfg: f.cfg, email: f.email}
	var wait sync.WaitGroup
	for i := 0; i < 16; i++ {
		wait.Add(2)
		go func() {
			defer wait.Done()
			adapter.ApplyRuntime(settingsmessaging.DefaultSMTPSetting(config.EmailConfig{Enabled: false, Host: "runtime.example.invalid"}))
		}()
		go func() {
			defer wait.Done()
			if _, err := adapter.GetSMTPSetting(); err != nil {
				t.Error(err)
			}
			_ = f.email.ConfigSnapshot()
		}()
	}
	wait.Wait()
	if f.cfg.Email != initial {
		t.Fatal("SMTP update mutated startup shared config")
	}
	if status, body := f.request(t, "PUT", "smtp_config", `{"enabled":false,"verify_code":{"length":8,"expire_minutes":3,"send_interval_seconds":15,"max_attempts":2}}`, 1); status != 0 {
		t.Fatal(body)
	}
	snapshot := f.email.ConfigSnapshot()
	if snapshot.Enabled || snapshot.VerifyCode.Length != 8 || snapshot.VerifyCode.MaxAttempts != 2 || snapshot.VerifyCode.ExpireMinutes != 3 || snapshot.VerifyCode.SendIntervalSeconds != 15 {
		t.Fatalf("runtime verification policy lost: %+v", snapshot.VerifyCode)
	}
	// Every production mail entry point must observe the generic disable without restart.
	for _, send := range []func() error{
		func() error { return f.email.SendCustomEmail("to@example.test", "subject", "body") },
		func() error {
			return f.email.SendVerifyCode("to@example.test", "12345678", "register", "en-US", mailbrand.Brand{})
		},
		func() error {
			return f.email.SendOrderStatusEmail("to@example.test", notificationcontract.OrderStatusEmailInput{}, "en-US")
		},
		func() error {
			return f.email.SendOrderStatusEmailWithTemplate("to@example.test", notificationcontract.OrderStatusEmailInput{}, "en-US", nil)
		},
	} {
		if err := send(); !errors.Is(err, notificationcontract.ErrEmailServiceDisabled) {
			t.Fatalf("disabled runtime bypassed: %v", err)
		}
	}
}

func TestSettingsGoogleAuthAdapterKeepsStartupFallbackImmutable(t *testing.T) {
	const fallbackClientID = "startup.apps.googleusercontent.com"
	cfg := &config.Config{GoogleAuth: config.GoogleAuthConfig{
		Enabled:  false,
		ClientID: fallbackClientID,
	}}
	store := &googleAuthSettingStore{store: make(map[string]jsonmap.JSON)}
	runtime := &googleAuthRuntimeStub{}
	adapter := settingsGoogleAuthAdapter{
		settings:   settingsapp.NewService(store),
		cfg:        cfg,
		googleAuth: runtime,
	}

	var wait sync.WaitGroup
	for index := 0; index < 64; index++ {
		index := index
		wait.Add(3)
		go func() {
			defer wait.Done()
			if _, err := adapter.GetGoogleAuthSetting(); err != nil {
				t.Errorf("get setting: %v", err)
			}
		}()
		go func() {
			defer wait.Done()
			enabled := true
			clientID := fmt.Sprintf("client-%d.apps.googleusercontent.com", index)
			if _, err := adapter.PatchGoogleAuthSetting(settingssecurity.GoogleAuthSettingPatch{
				Enabled:  &enabled,
				ClientID: &clientID,
			}); err != nil {
				t.Errorf("patch setting: %v", err)
			}
		}()
		go func() {
			defer wait.Done()
			adapter.ApplyRuntime(settingssecurity.GoogleAuthSetting{
				Enabled:  true,
				ClientID: fmt.Sprintf("runtime-%d.apps.googleusercontent.com", index),
			})
		}()
	}
	wait.Wait()

	if cfg.GoogleAuth.Enabled || cfg.GoogleAuth.ClientID != fallbackClientID {
		t.Fatalf("startup fallback was mutated: %#v", cfg.GoogleAuth)
	}
	if _, ok, err := store.GetByKey(constants.SettingKeyGoogleAuthConfig); err != nil || !ok {
		t.Fatalf("database setting missing: ok=%v err=%v", ok, err)
	}

	final := settingssecurity.GoogleAuthSetting{Enabled: true, ClientID: "final.apps.googleusercontent.com"}
	adapter.ApplyRuntime(final)
	if got := runtime.snapshot(); !got.Enabled || got.ClientID != final.ClientID {
		t.Fatalf("runtime config = %#v", got)
	}
}
