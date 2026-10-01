package settingsapp

import (
	"reflect"
	"testing"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/shared/jsonmap"
)

func TestDefaultSettingRegistryCoversLegacyNormalizedKeys(t *testing.T) {
	want := []string{
		constants.SettingKeyAffiliateConfig,
		constants.SettingKeyCallbackRoutesConfig,
		constants.SettingKeyDashboardConfig,
		constants.SettingKeyGitHubAuthConfig,
		constants.SettingKeyGoogleAuthConfig,
		constants.SettingKeyHomeAnnouncement,
		constants.SettingKeyHomepageAd,
		constants.SettingKeyNavConfig,
		constants.SettingKeyNotificationCenterConfig,
		constants.SettingKeyOrderConfig,
		constants.SettingKeyOrderRiskControlConfig,
		constants.SettingKeyPaymentConfig,
		constants.SettingKeyPersonalCenterVisibilityConfig,
		constants.SettingKeyRegistrationConfig,
		constants.SettingKeySecurityCenterConfig,
		constants.SettingKeySiteConfig,
		constants.SettingKeyTelegramAuthConfig,

		constants.SettingKeyUpstreamSyncConfig,
		constants.SettingKeyWalletConfig,
	}

	if got := defaultSettingRegistry.Keys(); !reflect.DeepEqual(got, want) {
		t.Fatalf("default registry keys mismatch\nwant: %#v\n got: %#v", want, got)
	}
}

func TestPersonalCenterVisibilitySettingNormalizesAndInvalidatesPublicCache(t *testing.T) {
	repo := newMockSettingRepo()
	service := NewService(repo)
	result, err := service.UpdateWithEffects(constants.SettingKeyPersonalCenterVisibilityConfig, map[string]interface{}{
		"orders":  false,
		"unknown": false,
	})
	if err != nil {
		t.Fatalf("update visibility setting: %v", err)
	}
	if result.Value["orders"] != false ||
		result.Value["overview"] != true ||
		result.Value["affiliate"] != true ||
		result.Value["reseller"] != true ||
		result.Value["api"] != true ||
		len(result.Value) != 9 {
		t.Fatalf("normalized visibility setting = %#v", result.Value)
	}
	if !result.HasEffect(EffectInvalidatePublicConfigCache) {
		t.Fatal("visibility setting must invalidate public config cache")
	}
}

func TestSettingServiceUpdateWithEffectsDescribesCacheImpact(t *testing.T) {
	tests := []struct {
		name       string
		key        string
		value      map[string]interface{}
		wantEffect Effect
	}{
		{
			name:       "google auth config invalidates public cache",
			key:        constants.SettingKeyGoogleAuthConfig,
			value:      map[string]interface{}{"enabled": true, "client_id": "client.apps.googleusercontent.com"},
			wantEffect: EffectInvalidatePublicConfigCache,
		},
		{
			name:       "site config invalidates public cache",
			key:        constants.SettingKeySiteConfig,
			value:      map[string]interface{}{},
			wantEffect: EffectInvalidatePublicConfigCache,
		},
		{
			name:       "wallet config is effect only",
			key:        constants.SettingKeyWalletConfig,
			value:      map[string]interface{}{constants.SettingFieldWalletOnlyPayment: true},
			wantEffect: EffectInvalidatePublicConfigCache,
		},
		{
			name:       "callback routes invalidate route cache",
			key:        constants.SettingKeyCallbackRoutesConfig,
			value:      map[string]interface{}{constants.SettingFieldPaymentCallback: "/api/custom/callback"},
			wantEffect: EffectInvalidateCallbackRoutesCache,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newMockSettingRepo()
			service := NewService(repo)
			result, err := service.UpdateWithEffects(test.key, test.value)
			if err != nil {
				t.Fatalf("update setting: %v", err)
			}
			if !result.HasEffect(test.wantEffect) {
				t.Fatalf("expected effect %q, got %#v", test.wantEffect, result.Effects)
			}
		})
	}

	repo := newMockSettingRepo()
	service := NewService(repo)
	result, err := service.UpdateWithEffects("custom_extension_config", map[string]interface{}{"enabled": true})
	if err != nil {
		t.Fatalf("update unknown setting: %v", err)
	}
	if len(result.Effects) != 0 {
		t.Fatalf("unknown setting emitted effects: %#v", result.Effects)
	}
}

func TestSettingServiceUpdateKeepsUnknownKeyPassThroughBehavior(t *testing.T) {
	repo := newMockSettingRepo()
	service := NewService(repo)
	input := map[string]interface{}{
		"nested": map[string]interface{}{"enabled": true},
		"count":  float64(3),
	}

	got, err := service.Update("custom_extension_config", input)
	if err != nil {
		t.Fatalf("update unknown setting: %v", err)
	}
	if !reflect.DeepEqual(got, jsonmap.JSON(input)) {
		t.Fatalf("unknown setting changed during pass-through: %#v", got)
	}
	if saved := repo.store["custom_extension_config"]; !reflect.DeepEqual(saved, jsonmap.JSON(input)) {
		t.Fatalf("unknown setting persisted with a different shape: %#v", saved)
	}
}

func TestSecurityCenterNormalizationAndStrictSave(t *testing.T) {
	repo := newMockSettingRepo()
	service := NewService(repo)
	result, err := service.UpdateWithEffects("security_center_config", map[string]interface{}{"email_change": false})
	if err != nil || len(result.Value) != 6 || result.Value["email_change"] != false || result.Value["two_factor"] != true || !result.HasEffect(EffectInvalidatePublicConfigCache) {
		t.Fatalf("security config = %+v err=%v", result, err)
	}
	for _, bad := range []map[string]interface{}{{"email_change": "false"}, {"two_factor": nil}, {"unknown": true}} {
		if _, err := service.Update("security_center_config", bad); err == nil {
			t.Fatalf("accepted invalid config: %#v", bad)
		}
	}
}

func TestPaymentFeeConfigDefaultsSafeAndNormalizesSwitches(t *testing.T) {
	repo := newMockSettingRepo()
	service := NewService(repo)

	if got := service.GetPaymentFeeConfig(); got != (PaymentFeeConfig{}) {
		t.Fatalf("default payment fee config must be safe: %#v", got)
	}

	stored, err := service.Update(constants.SettingKeyPaymentConfig, map[string]interface{}{
		constants.SettingFieldCustomerFeeEnabled:         "true",
		constants.SettingFieldReuseLegacyOrderFeePayment: 1,
		"unexpected": true,
	})
	if err != nil {
		t.Fatalf("update payment fee config: %v", err)
	}
	want := jsonmap.JSON{
		constants.SettingFieldCustomerFeeEnabled:         true,
		constants.SettingFieldReuseLegacyOrderFeePayment: true,
	}
	if !reflect.DeepEqual(stored, want) {
		t.Fatalf("normalized payment config mismatch: %#v", stored)
	}
	if got := service.GetPaymentFeeConfig(); got != (PaymentFeeConfig{CustomerFeeEnabled: true, ReuseLegacyOrderFeePayment: true}) {
		t.Fatalf("payment fee config mismatch: %#v", got)
	}
}
