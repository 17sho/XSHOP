package settingsapp

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/constants"
	settingsmessaging "github.com/dujiao-next/internal/modules/settings/schema/messaging"
	"github.com/dujiao-next/internal/shared/jsonmap"
)

type mockSettingRepo struct {
	store map[string]jsonmap.JSON
}

func newMockSettingRepo() *mockSettingRepo {
	return &mockSettingRepo{store: map[string]jsonmap.JSON{}}
}

func (m *mockSettingRepo) GetByKey(key string) (jsonmap.JSON, bool, error) {
	value, ok := m.store[key]
	if !ok {
		return nil, false, nil
	}
	return value, true, nil
}

func (m *mockSettingRepo) Upsert(key string, value jsonmap.JSON) (jsonmap.JSON, error) {
	m.store[key] = value
	return value, nil
}

func TestNormalizeSMTPSetting(t *testing.T) {
	setting := settingsmessaging.NormalizeSMTPSetting(settingsmessaging.SMTPSetting{})
	if setting.Port != 587 {
		t.Fatalf("expected default port 587, got %d", setting.Port)
	}
	if setting.VerifyCode.Length != 6 {
		t.Fatalf("expected default verify length 6, got %d", setting.VerifyCode.Length)
	}
	if setting.VerifyCode.ExpireMinutes != 10 {
		t.Fatalf("expected default expire minutes 10, got %d", setting.VerifyCode.ExpireMinutes)
	}
}

func TestValidateSMTPSetting(t *testing.T) {
	invalid := settingsmessaging.NormalizeSMTPSetting(settingsmessaging.SMTPSetting{
		Enabled: true,
		Host:    "smtp.example.com",
		From:    "notify@example.com",
		UseTLS:  true,
		UseSSL:  true,
	})
	if err := settingsmessaging.ValidateSMTPSetting(invalid); err == nil {
		t.Fatal("expected tls/ssl conflict validation error")
	}

	valid := settingsmessaging.NormalizeSMTPSetting(settingsmessaging.SMTPSetting{
		Enabled:  true,
		Host:     "smtp.example.com",
		Port:     587,
		From:     "notify@example.com",
		UseTLS:   true,
		UseSSL:   false,
		Password: "secret",
		VerifyCode: settingsmessaging.SMTPVerifyCodeSetting{
			ExpireMinutes:       10,
			SendIntervalSeconds: 60,
			MaxAttempts:         5,
			Length:              6,
		},
	})
	if err := settingsmessaging.ValidateSMTPSetting(valid); err != nil {
		t.Fatalf("expected valid smtp config, got error: %v", err)
	}
}

func TestPatchSMTPSettingKeepsPasswordWhenEmpty(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewService(repo)

	defaultCfg := config.EmailConfig{
		Enabled:  true,
		Host:     "smtp.default.com",
		Port:     587,
		Username: "default-user",
		Password: "default-secret",
		From:     "default@example.com",
		FromName: "Default",
		UseTLS:   true,
		UseSSL:   false,
		VerifyCode: config.VerifyCodeConfig{
			ExpireMinutes:       10,
			SendIntervalSeconds: 60,
			MaxAttempts:         5,
			Length:              6,
		},
	}

	updated, err := svc.PatchSMTPSetting(defaultCfg, settingsmessaging.SMTPSettingPatch{
		Host:     ptrString("smtp.custom.com"),
		Password: ptrString(""),
	})
	if err != nil {
		t.Fatalf("patch smtp setting failed: %v", err)
	}
	if updated.Password != "default-secret" {
		t.Fatalf("expected password keep default-secret, got %q", updated.Password)
	}

	saved, ok := repo.store[constants.SettingKeySMTPConfig]
	if !ok {
		t.Fatalf("smtp setting was not saved")
	}
	if saved["password"] != "default-secret" {
		t.Fatalf("expected saved password keep old value, got %v", saved["password"])
	}
}

func TestRegistrationEmailTemplatePersistence(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewService(repo)
	defaults, err := svc.GetRegistrationEmailTemplateSetting()
	if err != nil || !reflect.DeepEqual(defaults, settingsmessaging.DefaultRegistrationEmailTemplateSetting()) || len(repo.store) != 0 {
		t.Fatalf("defaults: %+v %v", defaults, err)
	}
	repo.store[constants.SettingKeyOrderEmailTemplateConfig] = jsonmap.JSON{"untouched": true}
	var patch settingsmessaging.RegistrationEmailTemplateSettingPatch
	if err := json.Unmarshal([]byte(`{"templates":{"zh-CN":{"subject":"欢迎 {{site_name}}"}}}`), &patch); err != nil {
		t.Fatal(err)
	}
	saved, err := svc.PatchRegistrationEmailTemplateSetting(patch)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := svc.GetRegistrationEmailTemplateSetting()
	if err != nil || !reflect.DeepEqual(loaded, saved) || loaded.Templates.ENUS != defaults.Templates.ENUS {
		t.Fatalf("readback: %+v %v", loaded, err)
	}
	if repo.store[constants.SettingKeyRegistrationEmailTemplateConfig] == nil || repo.store[constants.SettingKeyOrderEmailTemplateConfig]["untouched"] != true {
		t.Fatal("incorrect persistence scope")
	}
	bad := "missing placeholders"
	patch.Templates.ZHCN.Body = &bad
	if _, err := svc.PatchRegistrationEmailTemplateSetting(patch); err == nil {
		t.Fatal("accepted invalid patch")
	}
	loaded, err = svc.GetRegistrationEmailTemplateSetting()
	if err != nil || !reflect.DeepEqual(loaded, saved) {
		t.Fatal("failed patch mutated stored configuration")
	}
	repo.store[constants.SettingKeyRegistrationEmailTemplateConfig] = jsonmap.JSON{"templates": "bad"}
	if _, err := svc.GetRegistrationEmailTemplateSetting(); err == nil {
		t.Fatal("corrupt stored value accepted")
	}
}

func ptrString(value string) *string {
	return &value
}
