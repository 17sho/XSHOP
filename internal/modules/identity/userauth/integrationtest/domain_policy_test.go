package integrationtest

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	userstore "github.com/dujiao-next/internal/modules/identity/user/infrastructure/gormstore"

	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"

	settingsapp "github.com/dujiao-next/internal/modules/settings/application"
	settingsstore "github.com/dujiao-next/internal/modules/settings/infrastructure/gormstore"
	settingssecurity "github.com/dujiao-next/internal/modules/settings/schema/security"

	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/constants"
	emailverificationdomain "github.com/dujiao-next/internal/modules/identity/emailverification/domain"
	emailverificationstore "github.com/dujiao-next/internal/modules/identity/emailverification/infrastructure/gormstore"
	externalidentitydomain "github.com/dujiao-next/internal/modules/identity/externalidentity/domain"
	externalidentitystore "github.com/dujiao-next/internal/modules/identity/externalidentity/infrastructure/gormstore"
	userauthapp "github.com/dujiao-next/internal/modules/identity/userauth/application"
	usertotpapp "github.com/dujiao-next/internal/modules/identity/userauth/totp/application"
	resellercontract "github.com/dujiao-next/internal/modules/reseller/contract"
	"github.com/dujiao-next/internal/shared/mailbrand"
	"github.com/pquerna/otp/totp"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type verificationEmailSenderStub struct{}

func (verificationEmailSenderStub) SendVerifyCode(_, _, _, _ string, _ mailbrand.Brand) error {
	return nil
}

type capturingVerificationEmailSender struct {
	brand mailbrand.Brand
}

func (s *capturingVerificationEmailSender) SendVerifyCode(_, _, _, _ string, brand mailbrand.Brand) error {
	s.brand = brand
	return nil
}

type verificationEmailBrandResolverStub struct {
	brand       mailbrand.Brand
	resellerID  uint
	resolverRun bool
}

func (s *verificationEmailBrandResolverStub) ResolveEmailBrand(ctx context.Context, _ mailbrand.Scope) (mailbrand.Brand, error) {
	s.resolverRun = true
	tenant, ok := resellercontract.TenantFromContext(ctx)
	if ok && tenant.ResellerID != nil {
		s.resellerID = *tenant.ResellerID
	}
	return s.brand, nil
}

func newRegistrationDomainPolicyAuthService(t *testing.T) (*userauthapp.Service, *settingsapp.Service, *gorm.DB) {
	return newRegistrationDomainPolicyAuthServiceWithSender(t, verificationEmailSenderStub{})
}

func newRegistrationDomainPolicyAuthServiceWithSender(t *testing.T, sender userauthapp.VerificationEmailSender) (*userauthapp.Service, *settingsapp.Service, *gorm.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:user_auth_domain_policy_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&userdomain.User{}, &externalidentitydomain.Identity{}, &emailverificationdomain.Code{}, &settingsstore.SettingRecord{}); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	cfg := &config.Config{
		App:     config.AppConfig{SecretKey: "test-app-secret-domain-policy"},
		UserJWT: config.JWTConfig{SecretKey: "user-jwt-domain-policy-secret", ExpireHours: 24},
		Email:   config.EmailConfig{Enabled: false},
	}
	settingSvc := settingsapp.NewService(settingsstore.New(db))
	return userauthapp.NewService(
		cfg,
		userstore.New(db),
		externalidentitystore.New(db),
		emailverificationstore.New(db),
		settingSvc,
		sender,
		nil,
	), settingSvc, db
}

func TestSecurityCenterEmailToggleAndPasswordRecoverySQLite(t *testing.T) {
	svc, settings, db := newRegistrationDomainPolicyAuthService(t)
	user, _, _, err := svc.Register("switch-old@example.com", "original-password", "", true, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SendChangeEmailCode(context.Background(), user.ID, "old", "", "en-US"); err != nil {
		t.Fatal(err)
	}
	if err := svc.SendChangeEmailCode(context.Background(), user.ID, "new", "switch-new@example.com", "en-US"); err != nil {
		t.Fatal(err)
	}
	codes := emailverificationstore.New(db)
	oldCode, err := codes.GetLatest(user.Email, constants.VerifyPurposeChangeEmailOld)
	if err != nil {
		t.Fatal(err)
	}
	newCode, err := codes.GetLatest("switch-new@example.com", constants.VerifyPurposeChangeEmailNew)
	if err != nil {
		t.Fatal(err)
	}
	if oldCode == nil || newCode == nil {
		t.Fatal("no verification codes")
	}
	if _, err := settings.Update(constants.SettingKeySecurityCenterConfig, map[string]interface{}{"email_change": false}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ChangeEmail(user.ID, "switch-new@example.com", oldCode.Code, newCode.Code); !errors.Is(err, settingssecurity.ErrSecurityCenterDisabled) {
		t.Fatalf("email toggle bypass: %v", err)
	}
	oldAfter, _ := codes.GetLatest(user.Email, constants.VerifyPurposeChangeEmailOld)
	newAfter, _ := codes.GetLatest("switch-new@example.com", constants.VerifyPurposeChangeEmailNew)
	unchanged, _ := userstore.New(db).GetByID(user.ID)
	if oldAfter.VerifiedAt != nil || newAfter.VerifiedAt != nil || unchanged.Email != user.Email || unchanged.TokenVersion != user.TokenVersion {
		t.Fatal("blocked email consumed code or mutated user")
	}
	if err := svc.ChangePassword(user.ID, "original-password", "next-password"); err != nil {
		t.Fatalf("independent password control: %v", err)
	}
	if _, err := settings.Update(constants.SettingKeySecurityCenterConfig, map[string]interface{}{"email_change": true, "password_change": false, "telegram_binding": false, "google_binding": false, "two_factor": false, "login_history": false}); err != nil {
		t.Fatal(err)
	}
	changed, err := svc.ChangeEmail(user.ID, "switch-new@example.com", oldCode.Code, newCode.Code)
	if err != nil || changed.Email != "switch-new@example.com" {
		t.Fatalf("isolated email positive: %+v %v", changed, err)
	}
	if err := svc.ChangePassword(user.ID, "next-password", "blocked-password"); !errors.Is(err, settingssecurity.ErrSecurityCenterDisabled) {
		t.Fatalf("password bypass: %v", err)
	}
	if err := svc.SendVerifyCode(context.Background(), changed.Email, constants.VerifyPurposeReset, "en-US"); err != nil {
		t.Fatalf("recovery code incorrectly gated: %v", err)
	}
	recovery, _ := codes.GetLatest(changed.Email, constants.VerifyPurposeReset)
	if err := svc.ResetPassword(changed.Email, recovery.Code, "recovered-password"); err != nil {
		t.Fatalf("forgot password incorrectly gated: %v", err)
	}
	if _, err := svc.LoginStep1(changed.Email, "recovered-password", false); err != nil {
		t.Fatalf("recovery login: %v", err)
	}
}

func TestSecurityCenterDisabledEnrollmentKeepsExistingLoginChallenge(t *testing.T) {
	auth, settings, db := newRegistrationDomainPolicyAuthService(t)
	user, _, _, err := auth.Register("two-factor-switch@example.com", "synthetic-password", "", true, false)
	if err != nil {
		t.Fatal(err)
	}
	service := usertotpapp.NewService(&config.Config{App: config.AppConfig{SecretKey: "test-app-secret-domain-policy"}}, userstore.New(db), nil)
	service.SetSecurityCenterPolicy(settings.CheckSecurityCenterFeature)
	setup, err := service.Setup(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	code, _ := totp.GenerateCode(setup.Secret, time.Now())
	if _, err := service.Enable(user.ID, code); err != nil {
		t.Fatal(err)
	}
	if _, err := settings.Update(constants.SettingKeySecurityCenterConfig, map[string]interface{}{"two_factor": false}); err != nil {
		t.Fatal(err)
	}
	login, err := auth.LoginStep1(user.Email, "synthetic-password", false)
	if err != nil || login == nil || !login.RequiresTOTP || login.Token != "" || login.ChallengeToken == "" {
		t.Fatalf("disabled enrollment bypassed existing challenge: %+v %v", login, err)
	}
	claims, err := auth.ParseUserChallengeToken(login.ChallengeToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.VerifyChallengeCode(user.ID, code); err != nil {
		t.Fatal(err)
	}
	if complete, err := auth.CompleteLoginAfter2FA(user.ID, false, *claims.TokenVersion); err != nil || complete.Token == "" {
		t.Fatalf("existing login locked out: %+v %v", complete, err)
	}
}

func TestRegisterRejectsEmailDomainNotAllowed(t *testing.T) {
	svc, settings, _ := newRegistrationDomainPolicyAuthService(t)
	if _, err := settings.Update(constants.SettingKeyRegistrationConfig, map[string]interface{}{
		constants.SettingFieldEmailDomainAllowlistEnabled: true,
		constants.SettingFieldAllowedEmailDomains:         []interface{}{"qq.com"},
	}); err != nil {
		t.Fatalf("update registration config failed: %v", err)
	}

	user, token, _, err := svc.Register("buyer@example.com", "secret123", "", true, false)
	if !errors.Is(err, settingsapp.ErrEmailDomainNotAllowed) {
		t.Fatalf("expected ErrEmailDomainNotAllowed, got user=%+v token=%q err=%v", user, token, err)
	}
}

func TestRegisterAllowsExactEmailDomain(t *testing.T) {
	svc, settings, _ := newRegistrationDomainPolicyAuthService(t)
	if _, err := settings.Update(constants.SettingKeyRegistrationConfig, map[string]interface{}{
		constants.SettingFieldEmailDomainAllowlistEnabled: true,
		constants.SettingFieldAllowedEmailDomains:         []interface{}{"qq.com"},
	}); err != nil {
		t.Fatalf("update registration config failed: %v", err)
	}

	user, token, _, err := svc.Register("buyer@qq.com", "secret123", "", true, false)
	if err != nil {
		t.Fatalf("register should allow qq.com: %v", err)
	}
	if user == nil || user.Email != "buyer@qq.com" || token == "" {
		t.Fatalf("unexpected register result user=%+v token=%q", user, token)
	}
}

func TestSendVerifyCodeRejectsEmailDomainBeforeEmailSend(t *testing.T) {
	svc, settings, _ := newRegistrationDomainPolicyAuthService(t)
	if _, err := settings.Update(constants.SettingKeyRegistrationConfig, map[string]interface{}{
		constants.SettingFieldEmailDomainAllowlistEnabled: true,
		constants.SettingFieldAllowedEmailDomains:         []interface{}{"qq.com"},
	}); err != nil {
		t.Fatalf("update registration config failed: %v", err)
	}

	err := svc.SendVerifyCode(context.Background(), "buyer@example.com", constants.VerifyPurposeRegister, constants.LocaleZhCN)
	if !errors.Is(err, settingsapp.ErrEmailDomainNotAllowed) {
		t.Fatalf("expected ErrEmailDomainNotAllowed, got %v", err)
	}
}

func TestSendVerifyCodePropagatesRequestTenantBrandToSender(t *testing.T) {
	sender := &capturingVerificationEmailSender{}
	svc, _, _ := newRegistrationDomainPolicyAuthServiceWithSender(t, sender)
	resolver := &verificationEmailBrandResolverStub{brand: mailbrand.Brand{
		SiteName: "White Label Store",
		SiteURL:  "https://shop.example.test",
		FromName: "White Label Store",
	}}
	svc.SetEmailBrandResolver(resolver)
	tenant := resellercontract.ResellerTenantContext("shop.example.test", 31, 310, "shop.example.test")
	ctx := resellercontract.WithTenantContext(context.Background(), tenant)

	if err := svc.SendVerifyCode(ctx, "brand-buyer@example.com", constants.VerifyPurposeRegister, constants.LocaleZhCN); err != nil {
		t.Fatalf("send verify code failed: %v", err)
	}
	if !resolver.resolverRun || resolver.resellerID != 31 {
		t.Fatalf("request tenant did not reach brand resolver: %+v", resolver)
	}
	if sender.brand.SiteName != "White Label Store" || sender.brand.SiteURL != "https://shop.example.test" || sender.brand.FromName != "White Label Store" {
		t.Fatalf("resolved brand did not reach sender: %+v", sender.brand)
	}
}
