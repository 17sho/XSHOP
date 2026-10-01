package integrationtest

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	resellergormstore "github.com/dujiao-next/internal/modules/reseller/infrastructure/gormstore"

	resellerdomain "github.com/dujiao-next/internal/modules/reseller/domain"

	resellermodule "github.com/dujiao-next/internal/modules/reseller/application"
	"github.com/dujiao-next/internal/shared/jsonmap"
)

func TestResellerSiteConfigServiceUserUpdateRequiresActiveProfile(t *testing.T) {
	db := openResellerManagementServiceTestDB(t)
	repo := resellergormstore.New(db)
	user := seedResellerManagementUser(t, db, "site-config-pending@example.test")
	profile := resellerdomain.Profile{UserID: user.ID, Status: resellerdomain.ProfileStatusPendingReview, SettlementStatus: resellerdomain.SettlementStatusNormal}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatalf("create profile failed: %v", err)
	}
	svc := NewResellerSiteConfigService(repo)
	_, err := svc.UpdateUserSiteConfig(context.Background(), user.ID, ResellerSiteConfigInput{SiteName: "Pending Store"})
	if !errors.Is(err, ErrResellerProfileInactive) {
		t.Fatalf("expected inactive profile error, got %v", err)
	}
}

func TestResellerSiteConfigServiceNormalizesAndStoresSafeFields(t *testing.T) {
	db := openResellerManagementServiceTestDB(t)
	repo := resellergormstore.New(db)
	user := seedResellerManagementUser(t, db, "site-config-active@example.test")
	profile := resellerdomain.Profile{UserID: user.ID, Status: resellerdomain.ProfileStatusActive, SettlementStatus: resellerdomain.SettlementStatusNormal}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatalf("create profile failed: %v", err)
	}
	svc := NewResellerSiteConfigService(repo)
	row, err := svc.UpdateUserSiteConfig(context.Background(), user.ID, ResellerSiteConfigInput{
		SiteName: "  Alice Store  ",
		Logo:     "/uploads/reseller/logo.png",
		Favicon:  "https://cdn.example.test/favicon.png",
		Announcement: ResellerAnnouncementInput{
			Enabled: true,
			Type:    "info",
			Title:   LocalizedTextInput{"zh-CN": "公告", "fr-FR": "drop me"},
			Content: LocalizedTextInput{"zh-CN": "欢迎"},
		},
		Support: ResellerSupportInput{
			Telegram:   "https://t.me/alice",
			Email:      "support@example.test",
			SupportURL: "mailto:support@example.test",
		},
		SEO: ResellerSEOInput{
			Title:       LocalizedTextInput{"zh-CN": "爱丽丝商店", "zh-TW": "愛麗絲商店", "en-US": "Alice Store"},
			Description: LocalizedTextInput{"zh-CN": "精选商品", "en-US": "Curated products"},
		},
	})
	if err != nil {
		t.Fatalf("update site config failed: %v", err)
	}
	if row.SiteName != "Alice Store" || row.Logo != "/uploads/reseller/logo.png" || row.Favicon != "https://cdn.example.test/favicon.png" {
		t.Fatalf("unexpected normalized row: %+v", row)
	}
	if row.SupportJSON["telegram"] != "https://t.me/alice" || row.SupportJSON["email"] != "support@example.test" {
		t.Fatalf("unexpected support json: %+v", row.SupportJSON)
	}
	announcementTitle := row.AnnouncementJSON["title"].(jsonmap.JSON)
	if _, exists := announcementTitle["fr-FR"]; exists {
		t.Fatalf("unexpected unsupported locale retained: %+v", announcementTitle)
	}
	if len(row.ThemeJSON) != 0 {
		t.Fatalf("expected theme config to be ignored, got: %+v", row.ThemeJSON)
	}
}

func TestResellerSiteConfigServiceRejectsUnsafeURLs(t *testing.T) {
	db := openResellerManagementServiceTestDB(t)
	repo := resellergormstore.New(db)
	user := seedResellerManagementUser(t, db, "site-config-unsafe@example.test")
	profile := resellerdomain.Profile{UserID: user.ID, Status: resellerdomain.ProfileStatusActive, SettlementStatus: resellerdomain.SettlementStatusNormal}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatalf("create profile failed: %v", err)
	}
	svc := NewResellerSiteConfigService(repo)
	_, err := svc.UpdateUserSiteConfig(context.Background(), user.ID, ResellerSiteConfigInput{
		SiteName: "Unsafe Store",
		Support:  ResellerSupportInput{SupportURL: "javascript:alert(1)"},
	})
	if !errors.Is(err, ErrResellerSiteConfigInvalid) {
		t.Fatalf("expected invalid site config error, got %v", err)
	}
}

func TestResellerSiteConfigServiceReturnsFieldErrorForInvalidSupport(t *testing.T) {
	db := openResellerManagementServiceTestDB(t)
	repo := resellergormstore.New(db)
	user := seedResellerManagementUser(t, db, "site-config-field@example.test")
	profile := resellerdomain.Profile{UserID: user.ID, Status: resellerdomain.ProfileStatusActive, SettlementStatus: resellerdomain.SettlementStatusNormal}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatalf("create profile failed: %v", err)
	}
	svc := NewResellerSiteConfigService(repo)
	_, err := svc.UpdateUserSiteConfig(context.Background(), user.ID, ResellerSiteConfigInput{
		SiteName: "Field Store",
		Support:  ResellerSupportInput{WhatsApp: "https://w.me/123"},
	})
	if !errors.Is(err, ErrResellerSiteConfigInvalid) {
		t.Fatalf("expected invalid site config error, got %v", err)
	}
	var fieldErr *ResellerSiteConfigFieldError
	if !errors.As(err, &fieldErr) {
		t.Fatalf("expected field error, got %v", err)
	}
	if fieldErr.Field != "support_whatsapp" {
		t.Fatalf("expected support_whatsapp field, got %q", fieldErr.Field)
	}
}

func TestNormalizeResellerSupportAcceptsTelegramMe(t *testing.T) {
	support, err := resellermodule.NormalizeResellerSupport(ResellerSupportInput{Telegram: "https://telegram.me/alice"})
	if err != nil {
		t.Fatalf("expected telegram.me link to be accepted, got %v", err)
	}
	if support["telegram"] != "https://telegram.me/alice" {
		t.Fatalf("unexpected support json: %+v", support)
	}
}

func TestResellerSiteConfigServiceApplyPublicConfigOverlay(t *testing.T) {
	db := openResellerManagementServiceTestDB(t)
	repo := resellergormstore.New(db)
	user := seedResellerManagementUser(t, db, "site-config-overlay@example.test")
	profile := resellerdomain.Profile{UserID: user.ID, Status: resellerdomain.ProfileStatusActive, SettlementStatus: resellerdomain.SettlementStatusNormal}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatalf("create profile failed: %v", err)
	}
	svc := NewResellerSiteConfigService(repo)
	if _, err := svc.UpdateUserSiteConfig(context.Background(), user.ID, ResellerSiteConfigInput{
		SiteName: "Overlay Store",
		Favicon:  "/uploads/reseller/favicon.png",
		SEO: ResellerSEOInput{
			Title: LocalizedTextInput{"zh-CN": "覆盖标题", "en-US": "Overlay Title"},
		},
	}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}
	resellerID := profile.ID
	base := map[string]interface{}{
		"brand": map[string]interface{}{
			"site_name": "Main Store",
			"site_icon": "/dj.svg",
			"site_url":  "https://main.example.test",
		},
		"currency": "CNY",
		"seo": map[string]interface{}{
			"title": map[string]interface{}{"zh-CN": "主站标题", "en-US": "Main Title"},
		},
		"announcement": map[string]interface{}{"enabled": true},
		"nav_config":   map[string]interface{}{"builtin": map[string]interface{}{"notice": false, "blog": true, "about": true}},
	}
	tenant := ResellerTenantContext("shop.example.test", resellerID, user.ID, "shop.example.test")
	out, err := svc.ApplyPublicConfigOverlay(context.Background(), tenant, base)
	if err != nil {
		t.Fatalf("apply overlay failed: %v", err)
	}
	brand := out["brand"].(map[string]interface{})
	if brand["site_name"] != "Overlay Store" || brand["site_icon"] != "/uploads/reseller/favicon.png" {
		t.Fatalf("unexpected brand overlay: %+v", brand)
	}
	if brand["site_url"] != "https://shop.example.test" {
		t.Fatalf("reseller brand must not inherit the main site URL: %+v", brand)
	}
	if out["currency"] != "CNY" {
		t.Fatalf("global inherited fields should remain, got currency=%v", out["currency"])
	}
	seo := resellerSiteConfigTestMap(out["seo"])
	title := resellerSiteConfigTestMap(seo["title"])
	if title["zh-CN"] != "覆盖标题" || title["en-US"] != "Overlay Title" {
		t.Fatalf("unexpected localized seo overlay: %+v", seo)
	}
	if _, exists := out["announcement"]; exists {
		t.Fatalf("disabled reseller announcement should remove the field, got %+v", out["announcement"])
	}
	nav := resellerSiteConfigTestMap(out["nav_config"])
	builtin := resellerSiteConfigTestMap(nav["builtin"])
	if len(builtin) != 1 || builtin["notice"] != true {
		t.Fatalf("saved reseller config should own only retained nav defaults, got %+v", nav)
	}
	tenantPayload := out["tenant"].(map[string]interface{})
	if tenantPayload["mode"] != "reseller" || tenantPayload["host"] != "shop.example.test" {
		t.Fatalf("unexpected tenant payload: %+v", tenantPayload)
	}
}

func TestResellerSiteConfigServiceOverlayFiltersPersistedLegacyNavDestinations(t *testing.T) {
	db := openResellerManagementServiceTestDB(t)
	repo := resellergormstore.New(db)
	user := seedResellerManagementUser(t, db, "legacy-nav-overlay@example.test")
	profile := resellerdomain.Profile{UserID: user.ID, Status: resellerdomain.ProfileStatusActive, SettlementStatus: resellerdomain.SettlementStatusNormal}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatalf("create profile failed: %v", err)
	}
	row := resellerdomain.SiteConfig{ResellerID: profile.ID, NavConfigJSON: jsonmap.JSON{
		"builtin":      map[string]interface{}{"notice": false, "blog": true, "about": true},
		"custom_items": []interface{}{map[string]interface{}{"url": "/custom"}},
	}}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("create legacy config failed: %v", err)
	}

	svc := NewResellerSiteConfigService(repo)
	out, err := svc.ApplyPublicConfigOverlay(context.Background(), ResellerTenantContext("legacy.example.test", profile.ID, user.ID, "legacy.example.test"), map[string]interface{}{})
	if err != nil {
		t.Fatalf("apply overlay failed: %v", err)
	}
	nav := resellerSiteConfigTestMap(out["nav_config"])
	builtin := resellerSiteConfigTestMap(nav["builtin"])
	if len(builtin) != 1 || builtin["notice"] != false {
		t.Fatalf("legacy destinations must not reach public overlay: %+v", nav)
	}
}

func TestResellerSiteConfigServiceOverlayEmitsActiveAnnouncement(t *testing.T) {
	db := openResellerManagementServiceTestDB(t)
	repo := resellergormstore.New(db)
	user := seedResellerManagementUser(t, db, "site-config-announcement@example.test")
	profile := resellerdomain.Profile{UserID: user.ID, Status: resellerdomain.ProfileStatusActive, SettlementStatus: resellerdomain.SettlementStatusNormal}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatalf("create profile failed: %v", err)
	}
	svc := NewResellerSiteConfigService(repo)
	if _, err := svc.UpdateUserSiteConfig(context.Background(), user.ID, ResellerSiteConfigInput{
		SiteName: "Announcement Store",
		Announcement: ResellerAnnouncementInput{
			Enabled: true,
			Type:    "success",
			Title:   LocalizedTextInput{"zh-CN": "测试"},
			Content: LocalizedTextInput{"zh-CN": "<p>测试测试</p>"},
		},
	}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}
	tenant := ResellerTenantContext("shop.example.test", profile.ID, user.ID, "shop.example.test")
	out, err := svc.ApplyPublicConfigOverlay(context.Background(), tenant, map[string]interface{}{
		"announcement": map[string]interface{}{"type": "info", "version": "main0000"},
	})
	if err != nil {
		t.Fatalf("apply overlay failed: %v", err)
	}
	announcement := resellerSiteConfigTestMap(out["announcement"])
	if announcement == nil {
		t.Fatalf("enabled reseller announcement should be emitted, got %+v", out["announcement"])
	}
	if announcement["type"] != "success" {
		t.Fatalf("unexpected announcement type: %+v", announcement)
	}
	if _, exists := announcement["enabled"]; exists {
		t.Fatalf("public announcement should not expose enabled flag, got %+v", announcement)
	}
	version, _ := announcement["version"].(string)
	if len(version) != 8 || version == "main0000" {
		t.Fatalf("expected reseller-derived version fingerprint, got %q", version)
	}
	content := resellerSiteConfigTestMap(announcement["content"])
	if content["zh-CN"] != "<p>测试测试</p>" {
		t.Fatalf("unexpected announcement content: %+v", content)
	}
}

func TestHomepageNoticeResellerTypedCustomProjection(t *testing.T) {
	for _, items := range []interface{}{
		[]jsonmap.JSON{{"name": jsonmap.JSON{"en-US": "Help"}, "url": "https://example.test/help"}},
		[]map[string]interface{}{{"name": map[string]string{"en-US": "Help"}, "url": "https://example.test/help"}},
	} {
		got := resellermodule.NormalizeResellerNavConfigJSON(jsonmap.JSON{"homepage_notice_enabled": false, "custom_items": items})
		if len(got["custom_items"].([]interface{})) != 1 {
			t.Fatalf("typed custom projection lost links: %+v", got)
		}
	}
	if resellermodule.NormalizeResellerNavConfigJSON(nil)["homepage_notice_enabled"] != true {
		t.Fatal("missing reseller flag defaults on")
	}
}

func TestHomepageNoticeResellerRoundTripProjection(t *testing.T) {
	db := openResellerManagementServiceTestDB(t)
	repo := resellergormstore.New(db)
	user := seedResellerManagementUser(t, db, "notice-switch@example.test")
	profile := resellerdomain.Profile{UserID: user.ID, Status: resellerdomain.ProfileStatusActive, SettlementStatus: resellerdomain.SettlementStatusNormal}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewResellerSiteConfigService(repo)
	for _, home := range []interface{}{false, true, nil} {
		for _, nav := range []bool{false, true} {
			navJSON := map[string]interface{}{"builtin": map[string]bool{"notice": nav}, "custom_items": []interface{}{map[string]interface{}{"name": map[string]string{"en-US": "Help"}, "url": "https://example.test/help"}}}
			if home != nil {
				navJSON["homepage_notice_enabled"] = home
			}
			encoded, _ := json.Marshal(map[string]interface{}{"nav_config": navJSON})
			var input ResellerSiteConfigInput
			if err := json.Unmarshal(encoded, &input); err != nil {
				t.Fatal(err)
			}
			row, err := svc.UpdateUserSiteConfig(context.Background(), user.ID, input)
			if err != nil {
				t.Fatal(err)
			}
			_, reloaded, _, err := svc.GetUserSiteConfig(user.ID)
			if err != nil {
				t.Fatal(err)
			}
			overlay, err := svc.ApplyPublicConfigOverlay(context.Background(), ResellerTenantContext("shop.example.test", profile.ID, user.ID, "shop.example.test"), map[string]interface{}{})
			if err != nil {
				t.Fatal(err)
			}
			want := home != false
			for _, value := range []jsonmap.JSON{row.NavConfigJSON, reloaded.NavConfigJSON, resellermodule.NormalizeResellerNavConfigJSON(row.NavConfigJSON), resellermodule.NormalizeResellerNavConfigJSON(reloaded.NavConfigJSON), jsonmap.JSON(resellerSiteConfigTestMap(overlay["nav_config"]))} {
				if value["homepage_notice_enabled"] != want || resellerSiteConfigTestMap(value["builtin"])["notice"] != nav {
					t.Fatalf("reseller flags lost: %+v", value)
				}
				data, _ := json.Marshal(value)
				var decoded map[string]interface{}
				_ = json.Unmarshal(data, &decoded)
				if decoded["custom_items"].([]interface{})[0].(map[string]interface{})["url"] != "https://example.test/help" {
					t.Fatal("custom link lost")
				}
			}
		}
	}
	for _, builtin := range []interface{}{map[string]bool{"notice": false}, jsonmap.JSON{"notice": false}, map[string]interface{}{"notice": false}} {
		got := resellermodule.NormalizeResellerNavConfigJSON(jsonmap.JSON{"homepage_notice_enabled": false, "builtin": builtin})
		if got["homepage_notice_enabled"] != false || resellerSiteConfigTestMap(got["builtin"])["notice"] != false {
			t.Fatalf("typed projection lost false: %+v", got)
		}
	}
}

func resellerSiteConfigTestMap(value interface{}) map[string]interface{} {
	switch typed := value.(type) {
	case jsonmap.JSON:
		return map[string]interface{}(typed)
	case map[string]interface{}:
		return typed
	default:
		return nil
	}
}
