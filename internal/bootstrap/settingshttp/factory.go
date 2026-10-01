package settingsbootstrap

import (
	"github.com/dujiao-next/internal/app/container"
	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/constants"
	notificationtransport "github.com/dujiao-next/internal/modules/notification/transport/http"
	settingstransport "github.com/dujiao-next/internal/modules/settings/transport/http"
)

// ConfigureAdminHandler connects generic keys to the existing typed contracts.
func ConfigureAdminHandler(h *settingstransport.AdminHandler, c *container.Container, cfg *config.Config, notifications *notificationtransport.AdminHandler) {
	smtp := NewSMTPHandler(c, cfg)
	captcha := NewCaptchaHandler(c, cfg)
	telegram := NewTelegramAuthHandler(c, cfg)
	google := NewGoogleAuthHandler(c, cfg)
	github := NewGitHubAuthHandler(c, cfg)
	registration := settingstransport.NewRegistrationEmailTemplateHandler(c.SettingService)
	order := settingstransport.NewOrderEmailTemplateHandler(c.SettingService)
	affiliate := settingstransport.NewAffiliateHandler(c.SettingService)
	h.ConfigureTypedRoutes(c.AuthzService, map[string]settingstransport.TypedSettingRoute{
		constants.SettingKeySMTPConfig:                      {Resource: "/admin/settings/smtp", Get: smtp.GetSMTP, Update: smtp.UpdateSMTP},
		constants.SettingKeyCaptchaConfig:                   {Resource: "/admin/settings/captcha", Get: captcha.GetCaptcha, Update: captcha.UpdateCaptcha},
		constants.SettingKeyTelegramAuthConfig:              {Resource: "/admin/settings/telegram-auth", Get: telegram.GetTelegramAuth, Update: telegram.UpdateTelegramAuth},
		constants.SettingKeyGoogleAuthConfig:                {Resource: "/admin/settings/google-auth", Get: google.GetGoogleAuth},
		constants.SettingKeyGitHubAuthConfig:                {Resource: "/admin/settings/github-auth", Get: github.GetGitHubAuth},
		constants.SettingKeyNotificationCenterConfig:        {Resource: "/admin/settings/notification-center", Get: notifications.GetNotificationCenterSettings, Update: notifications.UpdateNotificationCenterSettings},
		constants.SettingKeyRegistrationEmailTemplateConfig: {Resource: "/admin/settings/registration-email-template", Get: registration.GetRegistrationEmailTemplate, Update: registration.UpdateRegistrationEmailTemplate},
		constants.SettingKeyOrderEmailTemplateConfig:        {Resource: "/admin/settings/order-email-template", Get: order.GetOrderEmailTemplate, Update: order.UpdateOrderEmailTemplate},
		constants.SettingKeyAffiliateConfig:                 {Resource: "/admin/settings/affiliate", Get: affiliate.GetAffiliate, Update: affiliate.UpdateAffiliate},
	})
}

func NewSMTPHandler(c *container.Container, cfg *config.Config) *settingstransport.SMTPHandler {
	return settingstransport.NewSMTPHandler(settingsSMTPAdapter{
		settings: c.SettingService, cfg: cfg, email: c.EmailSender,
	})
}

func NewCaptchaHandler(c *container.Container, cfg *config.Config) *settingstransport.CaptchaHandler {
	return settingstransport.NewCaptchaHandler(settingsCaptchaAdapter{
		settings: c.SettingService, cfg: cfg, captcha: c.CaptchaService,
	})
}

func NewTelegramAuthHandler(c *container.Container, cfg *config.Config) *settingstransport.TelegramAuthHandler {
	return settingstransport.NewTelegramAuthHandler(settingsTelegramAuthAdapter{
		settings: c.SettingService, cfg: cfg, telegramAuth: c.TelegramAuthService,
	})
}

func NewGoogleAuthHandler(c *container.Container, cfg *config.Config) *settingstransport.GoogleAuthHandler {
	return settingstransport.NewGoogleAuthHandler(settingsGoogleAuthAdapter{
		settings: c.SettingService, cfg: cfg, googleAuth: c.GoogleAuthService,
	})
}

func NewGitHubAuthHandler(c *container.Container, cfg *config.Config) *settingstransport.GitHubAuthHandler {
	return settingstransport.NewGitHubAuthHandler(settingsGitHubAuthAdapter{
		settings: c.SettingService, cfg: cfg, gitHubAuth: c.GitHubAuthService,
	})
}
