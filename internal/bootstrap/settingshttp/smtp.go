package settingsbootstrap

import (
	"context"
	"errors"

	notificationcontract "github.com/dujiao-next/internal/modules/notification/contract"
	notificationsmtp "github.com/dujiao-next/internal/modules/notification/infrastructure/smtp"
	settingsapp "github.com/dujiao-next/internal/modules/settings/application"
	settingsmessaging "github.com/dujiao-next/internal/modules/settings/schema/messaging"

	"github.com/dujiao-next/internal/config"
	settingstransport "github.com/dujiao-next/internal/modules/settings/transport/http"
)

type settingsSMTPAdapter struct {
	settings *settingsapp.Service
	cfg      *config.Config
	email    *notificationsmtp.Service
}

func (a settingsSMTPAdapter) GetSMTPSetting() (settingsmessaging.SMTPSetting, error) {
	return a.settings.GetSMTPSetting(a.emailConfig())
}

func (a settingsSMTPAdapter) PatchSMTPSetting(patch settingsmessaging.SMTPSettingPatch) (settingsmessaging.SMTPSetting, error) {
	return a.settings.PatchSMTPSetting(a.emailConfig(), patch)
}

func (a settingsSMTPAdapter) ApplyRuntime(setting settingsmessaging.SMTPSetting) {
	cfg := settingsmessaging.SMTPSettingToConfig(setting)
	if a.email != nil {
		a.email.SetConfig(&cfg)
	}
}

func (a settingsSMTPAdapter) SendTest(setting settingsmessaging.SMTPSetting, toEmail, subject, body string) error {
	return a.SendTestContext(context.Background(), setting, toEmail, subject, body)
}

func (a settingsSMTPAdapter) emailConfig() config.EmailConfig {
	if a.email != nil {
		return a.email.ConfigSnapshot()
	}
	return a.cfg.Email
}

func (a settingsSMTPAdapter) SendTestContext(ctx context.Context, setting settingsmessaging.SMTPSetting, toEmail, subject, body string) error {
	configForSend := settingsmessaging.SMTPSettingToConfig(setting)
	configForSend.Enabled = true
	err := notificationsmtp.New(&configForSend).SendCustomEmailContext(ctx, toEmail, subject, body)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, notificationcontract.ErrInvalidEmail):
		return settingstransport.ErrSMTPTestInvalidEmail
	case errors.Is(err, notificationcontract.ErrEmailRecipientRejected):
		return settingstransport.ErrSMTPTestRecipientRejected
	case errors.Is(err, notificationcontract.ErrEmailServiceDisabled):
		return settingstransport.ErrSMTPTestServiceDisabled
	case errors.Is(err, notificationcontract.ErrEmailNotConfigured):
		return settingstransport.ErrSMTPTestServiceNotConfigured
	default:
		return err
	}
}
