package publicconfighttp

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dujiao-next/internal/cache"
	"github.com/dujiao-next/internal/constants"
	settingsapp "github.com/dujiao-next/internal/modules/settings/application"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/gin-gonic/gin"
)

type noticeSettingsStub struct {
	nav         interface{}
	security    interface{}
	securityErr error
}

func (s noticeSettingsStub) GetConfig(d map[string]interface{}) (map[string]interface{}, error) {
	return d, nil
}
func (s noticeSettingsStub) GetWalletRechargeChannelIDs() []uint                     { return nil }
func (s noticeSettingsStub) GetWalletOnlyPayment() bool                              { return false }
func (s noticeSettingsStub) GetAffiliateSettingMap() (map[string]interface{}, error) { return nil, nil }
func (s noticeSettingsStub) GetSMTPEnabled() bool                                    { return false }
func (s noticeSettingsStub) GetRegistrationEnabled(v bool) (bool, error)             { return v, nil }
func (s noticeSettingsStub) GetEmailVerificationEnabled(v bool) (bool, error)        { return v, nil }
func (s noticeSettingsStub) GetRegistrationEmailDomainPolicy() (bool, []string, error) {
	return false, nil, nil
}
func (s noticeSettingsStub) GetByKey(k string) (interface{}, error) {
	if k == constants.SettingKeySecurityCenterConfig {
		return s.security, s.securityErr
	}
	if k == "nav_config" {
		return s.nav, nil
	}
	return nil, nil
}
func (s noticeSettingsStub) GetActiveHomeAnnouncement() (jsonmap.JSON, bool) { return nil, false }
func (s noticeSettingsStub) GetActiveHomepageAd() (jsonmap.JSON, bool)       { return nil, false }
func (s noticeSettingsStub) GetOrderPaymentChannels() ([]map[string]interface{}, error) {
	return nil, nil
}

type noticeCacheStub struct{ cached map[string]interface{} }

func (s *noticeCacheStub) CacheKey(id *uint) string { return cache.PublicConfigCacheKey(id) }
func (s *noticeCacheStub) GetJSON(_ context.Context, _ string, dest *map[string]interface{}) (bool, error) {
	*dest = s.cached
	return s.cached != nil, nil
}
func (s *noticeCacheStub) SetJSON(_ context.Context, _ string, value interface{}, _ time.Duration) error {
	s.cached = value.(map[string]interface{})
	return nil
}
func TestHomepageNoticePublicPayloadAndNoRowDefaults(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []jsonmap.JSON{nil}
	for _, nav := range []bool{false, true} {
		for _, home := range []bool{false, true} {
			cases = append(cases, jsonmap.JSON{"builtin": map[string]interface{}{"notice": nav}, "homepage_notice_enabled": home, "custom_items": []interface{}{map[string]interface{}{"url": "/help"}}})
		}
	}
	for _, nav := range cases {
		cache := &noticeCacheStub{}
		settings := noticeSettingsStub{}
		wantHome, wantNav := true, true
		if nav != nil {
			settings.nav = nav
			wantHome = nav["homepage_notice_enabled"].(bool)
			wantNav = nav["builtin"].(map[string]interface{})["notice"].(bool)
		}
		h := NewHandler(cache, settings, settings, nil, nil, TelegramAuthFallback{}, nil, GoogleAuthFallback{}, nil, GitHubAuthFallback{}, nil)
		for i := 0; i < 2; i++ {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/api/v1/config", nil)
			h.GetConfig(c)
			var envelope struct {
				Code int                    `json:"code"`
				Data map[string]interface{} `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if w.Code != 200 || envelope.Code != 0 {
				t.Fatalf("public config failed: %s", w.Body.String())
			}
			if _, ok := envelope.Data["security_center_config"].(map[string]interface{}); !ok {
				t.Fatalf("missing public security flags: %s", w.Body.String())
			}
			got := envelope.Data["nav_config"].(map[string]interface{})
			if got["homepage_notice_enabled"] != wantHome {
				t.Fatalf("public flag/default lost (cache pass %d): %+v", i, got)
			}
			if got["builtin"].(map[string]interface{})["notice"] != wantNav {
				t.Fatal("homepage flag changed navigation")
			}
			if nav != nil && got["custom_items"].([]interface{})[0].(map[string]interface{})["url"] != "/help" {
				t.Fatal("public links lost")
			}
		}
	}
}

type securityPublicStore struct{ value jsonmap.JSON }

func (s *securityPublicStore) GetByKey(string) (jsonmap.JSON, bool, error) {
	return s.value, s.value != nil, nil
}
func (s *securityPublicStore) Upsert(_ string, value jsonmap.JSON) (jsonmap.JSON, error) {
	s.value = value
	return value, nil
}

func TestSecurityCenterPublicConfigCachePersistReadbackAndReadError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &securityPublicStore{}
	service := settingsapp.NewService(store)
	cached := &noticeCacheStub{}
	if cached.CacheKey(nil) != "public:config:main" {
		t.Fatal("wrong main cache key")
	}
	id := uint(7)
	if cached.CacheKey(&id) != "public:config:reseller:7" {
		t.Fatal("wrong reseller cache key")
	}
	fetch := func(settings noticeSettingsStub) (int, jsonmap.JSON) {
		h := NewHandler(cached, settings, settings, nil, nil, TelegramAuthFallback{}, nil, GoogleAuthFallback{}, nil, GitHubAuthFallback{}, nil)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/v1/config", nil)
		h.GetConfig(c)
		var body struct {
			Status int `json:"status_code"`
			Data   struct {
				Security jsonmap.JSON `json:"security_center_config"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Status, body.Data.Security
	}
	cached.cached = map[string]interface{}{"legacy": true}
	if status, got := fetch(noticeSettingsStub{security: jsonmap.JSON{"email_change": false}}); status != 0 || got["email_change"] != false {
		t.Fatalf("legacy cache omitted current flags: %d %+v", status, got)
	}
	cached.cached = nil
	if status, got := fetch(noticeSettingsStub{}); status != 0 || len(got) != 6 || got["email_change"] != true {
		t.Fatalf("defaults=%d %+v", status, got)
	}
	result, err := service.UpdateWithEffects(constants.SettingKeySecurityCenterConfig, map[string]interface{}{"email_change": false})
	if err != nil || !result.HasEffect(settingsapp.EffectInvalidatePublicConfigCache) {
		t.Fatalf("effect=%+v %v", result, err)
	}
	// Synthetic cache adapter applies the same declared effect as the HTTP pipeline.
	cached.cached = nil
	persisted, err := service.GetByKey(constants.SettingKeySecurityCenterConfig)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if status, got := fetch(noticeSettingsStub{security: persisted}); status != 0 || got["email_change"] != false || got["google_binding"] != true {
			t.Fatalf("read/cache pass=%d status=%d %+v", i, status, got)
		}
	}
	cached.cached = nil
	if status, got := fetch(noticeSettingsStub{securityErr: errors.New("fixture read failure")}); status != 500 || got != nil || cached.cached != nil {
		t.Fatalf("read error returned/cached defaults: %d %+v", status, got)
	}
}

type googleAuthPublicStub map[string]interface{}

func (s googleAuthPublicStub) PublicConfig() map[string]interface{} {
	return map[string]interface{}(s)
}

func TestResolveGoogleAuthPublicConfig(t *testing.T) {
	tests := []struct {
		name     string
		source   GoogleAuthPublic
		fallback GoogleAuthFallback
		want     map[string]interface{}
	}{
		{
			name:     "fallback enabled",
			fallback: GoogleAuthFallback{Enabled: true, ClientID: " fallback-client "},
			want:     map[string]interface{}{"enabled": true, "client_id": "fallback-client"},
		},
		{
			name:     "service overrides fallback",
			source:   googleAuthPublicStub{"enabled": true, "client_id": " runtime-client ", "unexpected": "private"},
			fallback: GoogleAuthFallback{Enabled: false, ClientID: "fallback-client"},
			want:     map[string]interface{}{"enabled": true, "client_id": "runtime-client"},
		},
		{
			name:     "missing client id fails closed",
			source:   googleAuthPublicStub{"enabled": true, "client_id": ""},
			fallback: GoogleAuthFallback{Enabled: true, ClientID: "fallback-client"},
			want:     map[string]interface{}{"enabled": false, "client_id": ""},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := resolveGoogleAuthPublicConfig(test.source, test.fallback); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("public config = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestResolvePersonalCenterVisibilityPublicConfigUsesDefaults(t *testing.T) {
	got := resolvePersonalCenterVisibilityPublicConfig(jsonmap.JSON{"orders": false, "unknown": true})
	want := jsonmap.JSON{"overview": true, "orders": false, "wallet": true,
		"gift_cards": true, "security": true, "profile": true,
		"affiliate": true, "reseller": true, "api": true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("public visibility = %#v, want %#v", got, want)
	}
}

func TestFooterTextPublicNoRowDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := &noticeCacheStub{}
	settings := noticeSettingsStub{}
	h := NewHandler(cache, settings, settings, nil, nil, TelegramAuthFallback{}, nil, GoogleAuthFallback{}, nil, GitHubAuthFallback{}, nil)
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/v1/config", nil)
		h.GetConfig(c)
		var envelope struct {
			Code int                    `json:"code"`
			Data map[string]interface{} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if w.Code != 200 || envelope.Code != 0 || envelope.Data["footer_text"] != "" {
			t.Fatalf("missing empty footer_text default: %s", w.Body.String())
		}
	}
}
