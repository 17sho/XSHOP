package settingsapp

import (
	"time"

	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/constants"
	settingsintegration "github.com/dujiao-next/internal/modules/settings/schema/integration"
	settingsmessaging "github.com/dujiao-next/internal/modules/settings/schema/messaging"
	settingssecurity "github.com/dujiao-next/internal/modules/settings/schema/security"
	settingsstorefront "github.com/dujiao-next/internal/modules/settings/schema/storefront"
)

// GetDashboardSetting 获取仪表盘设置（优先 settings，空时回退默认）。
func (s *Service) GetDashboardSetting() (settingsstorefront.DashboardSetting, error) {
	fallback := settingsstorefront.DefaultDashboardSetting()
	if s == nil {
		return fallback, nil
	}
	value, err := s.GetByKey(constants.SettingKeyDashboardConfig)
	if err != nil {
		return fallback, err
	}
	if value == nil {
		return fallback, nil
	}
	return settingsstorefront.DecodeDashboardSetting(value, fallback), nil
}

// GetDashboardLowStockThreshold 获取低库存阈值（读取失败回退默认值）。
func (s *Service) GetDashboardLowStockThreshold() int {
	defaultThreshold := int(settingsstorefront.DefaultDashboardSetting().Alert.LowStockThreshold)
	if s == nil {
		return defaultThreshold
	}
	setting, err := s.GetDashboardSetting()
	if err != nil {
		return defaultThreshold
	}
	return int(setting.Alert.LowStockThreshold)
}

// GetAffiliateSetting 获取推广返利设置（优先 settings，空时回退默认）。
func (s *Service) GetAffiliateSetting() (settingsintegration.AffiliateSetting, error) {
	fallback := settingsintegration.DefaultAffiliateSetting()
	if s == nil {
		return fallback, nil
	}
	value, err := s.GetByKey(constants.SettingKeyAffiliateConfig)
	if err != nil {
		return fallback, err
	}
	if value == nil {
		return fallback, nil
	}
	return settingsintegration.DecodeAffiliateSetting(value, fallback), nil
}

// UpdateAffiliateSetting 更新推广返利设置。
func (s *Service) UpdateAffiliateSetting(setting settingsintegration.AffiliateSetting) (settingsintegration.AffiliateSetting, error) {
	normalized := settingsintegration.NormalizeAffiliateSetting(setting)
	if err := settingsintegration.ValidateAffiliateSetting(normalized); err != nil {
		return settingsintegration.DefaultAffiliateSetting(), err
	}
	if _, err := s.Update(constants.SettingKeyAffiliateConfig, map[string]interface{}(settingsintegration.EncodeAffiliateSetting(normalized))); err != nil {
		return settingsintegration.DefaultAffiliateSetting(), err
	}
	return normalized, nil
}

// GetUpstreamSyncConfig 获取上游同步配置。
// fallbackInterval 来自 config.yml，仅在数据库没有覆盖值时使用。
func (s *Service) GetUpstreamSyncConfig(fallbackInterval string) (settingsintegration.UpstreamSyncConfig, error) {
	fallback := settingsintegration.UpstreamSyncFallback(fallbackInterval)
	if s == nil {
		return fallback, nil
	}
	value, err := s.GetByKey(constants.SettingKeyUpstreamSyncConfig)
	if err != nil {
		return fallback, err
	}
	return settingsintegration.DecodeUpstreamSyncConfig(value, fallback), nil
}

// GetUpstreamSyncInterval 返回归一化后的同步间隔。
func (s *Service) GetUpstreamSyncInterval(fallbackInterval string) (time.Duration, error) {
	config, err := s.GetUpstreamSyncConfig(fallbackInterval)
	if err != nil {
		return time.Duration(config.IntervalMinutes) * time.Minute, err
	}
	return time.Duration(config.IntervalMinutes) * time.Minute, nil
}

// GetNotificationCenterSetting 获取通知中心配置（优先 settings，空时回退默认）。
func (s *Service) GetNotificationCenterSetting() (settingsmessaging.NotificationCenterSetting, error) {
	fallback := settingsmessaging.NotificationCenterDefaultSetting()
	if s == nil {
		return fallback, nil
	}
	value, err := s.GetByKey(constants.SettingKeyNotificationCenterConfig)
	if err != nil {
		return fallback, err
	}
	if value == nil {
		return fallback, nil
	}
	return settingsmessaging.NormalizeNotificationCenterSetting(settingsmessaging.DecodeNotificationCenterSetting(value, fallback)), nil
}

// PatchNotificationCenterSetting 基于补丁更新通知中心配置。
func (s *Service) PatchNotificationCenterSetting(patch settingsmessaging.NotificationCenterSettingPatch) (settingsmessaging.NotificationCenterSetting, error) {
	current, err := s.GetNotificationCenterSetting()
	if err != nil {
		return settingsmessaging.NotificationCenterSetting{}, err
	}
	next, err := settingsmessaging.ApplyNotificationCenterSettingPatch(current, patch)
	if err != nil {
		return settingsmessaging.NotificationCenterSetting{}, err
	}
	if _, err := s.Update(constants.SettingKeyNotificationCenterConfig, settingsmessaging.NotificationCenterSettingToMap(next)); err != nil {
		return settingsmessaging.NotificationCenterSetting{}, err
	}
	return next, nil
}

// GetSMTPSetting 获取 SMTP 设置（优先 settings，空时回退默认配置）。
func (s *Service) GetSMTPSetting(defaultCfg config.EmailConfig) (settingsmessaging.SMTPSetting, error) {
	fallback := settingsmessaging.DefaultSMTPSetting(defaultCfg)
	if s == nil {
		return fallback, nil
	}
	value, err := s.GetByKey(constants.SettingKeySMTPConfig)
	if err != nil {
		return fallback, err
	}
	if value == nil {
		return fallback, nil
	}
	return settingsmessaging.NormalizeSMTPSetting(settingsmessaging.DecodeSMTPSetting(value, fallback)), nil
}

// PatchSMTPSetting 基于补丁更新 SMTP 设置。
func (s *Service) PatchSMTPSetting(defaultCfg config.EmailConfig, patch settingsmessaging.SMTPSettingPatch) (settingsmessaging.SMTPSetting, error) {
	current, err := s.GetSMTPSetting(defaultCfg)
	if err != nil {
		return settingsmessaging.SMTPSetting{}, err
	}
	next, err := settingsmessaging.ApplySMTPSettingPatch(current, patch)
	if err != nil {
		return settingsmessaging.SMTPSetting{}, err
	}
	if _, err := s.Update(constants.SettingKeySMTPConfig, map[string]interface{}(settingsmessaging.EncodeSMTPSetting(next))); err != nil {
		return settingsmessaging.SMTPSetting{}, err
	}
	return next, nil
}

// GetCaptchaSetting 获取验证码设置（优先 settings，空时回退 config.yml）。
func (s *Service) GetCaptchaSetting(defaultCfg config.CaptchaConfig) (settingssecurity.CaptchaSetting, error) {
	fallback := settingssecurity.DefaultCaptchaSetting(defaultCfg)
	if s == nil {
		return fallback, nil
	}
	value, err := s.GetByKey(constants.SettingKeyCaptchaConfig)
	if err != nil {
		return fallback, err
	}
	if value == nil {
		return fallback, nil
	}
	return settingssecurity.NormalizeCaptchaSetting(settingssecurity.DecodeCaptchaSetting(value, fallback)), nil
}

// PatchCaptchaSetting 基于补丁更新验证码设置。
func (s *Service) PatchCaptchaSetting(defaultCfg config.CaptchaConfig, patch settingssecurity.CaptchaSettingPatch) (settingssecurity.CaptchaSetting, error) {
	current, err := s.GetCaptchaSetting(defaultCfg)
	if err != nil {
		return settingssecurity.CaptchaSetting{}, err
	}
	next, err := settingssecurity.ApplyCaptchaSettingPatch(current, patch)
	if err != nil {
		return settingssecurity.CaptchaSetting{}, err
	}
	if _, err := s.Update(constants.SettingKeyCaptchaConfig, map[string]interface{}(settingssecurity.EncodeCaptchaSetting(next))); err != nil {
		return settingssecurity.CaptchaSetting{}, err
	}
	return next, nil
}

// GetTelegramAuthSetting 获取 Telegram 登录配置。
func (s *Service) GetTelegramAuthSetting(defaultCfg config.TelegramAuthConfig) (settingssecurity.TelegramAuthSetting, error) {
	fallback := settingssecurity.DefaultTelegramAuthSetting(defaultCfg)
	if s == nil {
		return fallback, nil
	}
	value, err := s.GetByKey(constants.SettingKeyTelegramAuthConfig)
	if err != nil {
		return fallback, err
	}
	if value == nil {
		return fallback, nil
	}
	return settingssecurity.NormalizeTelegramAuthSetting(settingssecurity.DecodeTelegramAuthSetting(value, fallback)), nil
}

// PatchTelegramAuthSetting 基于补丁更新 Telegram 登录配置。
func (s *Service) PatchTelegramAuthSetting(defaultCfg config.TelegramAuthConfig, patch settingssecurity.TelegramAuthSettingPatch) (settingssecurity.TelegramAuthSetting, error) {
	current, err := s.GetTelegramAuthSetting(defaultCfg)
	if err != nil {
		return settingssecurity.TelegramAuthSetting{}, err
	}
	next := settingssecurity.NormalizeTelegramAuthSetting(settingssecurity.ApplyTelegramAuthSettingPatch(current, patch))
	if err := settingssecurity.ValidateTelegramAuthSetting(next); err != nil {
		return settingssecurity.TelegramAuthSetting{}, err
	}
	if _, err := s.Update(constants.SettingKeyTelegramAuthConfig, map[string]interface{}(settingssecurity.EncodeTelegramAuthSetting(next))); err != nil {
		return settingssecurity.TelegramAuthSetting{}, err
	}
	return next, nil
}

// GetGoogleAuthSetting 获取 Google Identity Services 登录配置。
func (s *Service) GetGoogleAuthSetting(defaultCfg config.GoogleAuthConfig) (settingssecurity.GoogleAuthSetting, error) {
	fallback := settingssecurity.DefaultGoogleAuthSetting(defaultCfg)
	if s == nil {
		return fallback, nil
	}
	value, err := s.GetByKey(constants.SettingKeyGoogleAuthConfig)
	if err != nil {
		return fallback, err
	}
	if value == nil {
		return fallback, nil
	}
	return settingssecurity.DecodeGoogleAuthSetting(value, fallback), nil
}

// PatchGoogleAuthSetting 基于补丁更新 Google Identity Services 登录配置。
func (s *Service) PatchGoogleAuthSetting(defaultCfg config.GoogleAuthConfig, patch settingssecurity.GoogleAuthSettingPatch) (settingssecurity.GoogleAuthSetting, error) {
	current, err := s.GetGoogleAuthSetting(defaultCfg)
	if err != nil {
		return settingssecurity.GoogleAuthSetting{}, err
	}
	next := settingssecurity.NormalizeGoogleAuthSetting(settingssecurity.ApplyGoogleAuthSettingPatch(current, patch))
	if err := settingssecurity.ValidateGoogleAuthSetting(next); err != nil {
		return settingssecurity.GoogleAuthSetting{}, err
	}
	if _, err := s.Update(constants.SettingKeyGoogleAuthConfig, map[string]interface{}(settingssecurity.EncodeGoogleAuthSetting(next))); err != nil {
		return settingssecurity.GoogleAuthSetting{}, err
	}
	return next, nil
}

// GetGitHubAuthSetting 获取 GitHub OAuth 登录配置。
func (s *Service) GetGitHubAuthSetting(defaultCfg config.GitHubAuthConfig) (settingssecurity.GitHubAuthSetting, error) {
	fallback := settingssecurity.DefaultGitHubAuthSetting(defaultCfg)
	if s == nil {
		return fallback, nil
	}
	value, err := s.GetByKey(constants.SettingKeyGitHubAuthConfig)
	if err != nil {
		return fallback, err
	}
	if value == nil {
		return fallback, nil
	}
	return settingssecurity.DecodeGitHubAuthSetting(value, fallback), nil
}

// PatchGitHubAuthSetting 基于补丁更新 GitHub OAuth 登录配置。
func (s *Service) PatchGitHubAuthSetting(defaultCfg config.GitHubAuthConfig, patch settingssecurity.GitHubAuthSettingPatch) (settingssecurity.GitHubAuthSetting, error) {
	current, err := s.GetGitHubAuthSetting(defaultCfg)
	if err != nil {
		return settingssecurity.GitHubAuthSetting{}, err
	}
	next := settingssecurity.NormalizeGitHubAuthSetting(settingssecurity.ApplyGitHubAuthSettingPatch(current, patch))
	if err := settingssecurity.ValidateGitHubAuthSetting(next); err != nil {
		return settingssecurity.GitHubAuthSetting{}, err
	}
	if _, err := s.Update(constants.SettingKeyGitHubAuthConfig, map[string]interface{}(settingssecurity.EncodeGitHubAuthSetting(next))); err != nil {
		return settingssecurity.GitHubAuthSetting{}, err
	}
	return next, nil
}

// GetRegistrationEmailTemplateSetting reads fresh settings; absent records use
// defaults, while corrupt records and storage failures remain explicit errors.
func (s *Service) GetRegistrationEmailTemplateSetting() (settingsmessaging.RegistrationEmailTemplateSetting, error) {
	value, err := s.GetByKey(constants.SettingKeyRegistrationEmailTemplateConfig)
	if err != nil {
		return settingsmessaging.DefaultRegistrationEmailTemplateSetting(), err
	}
	return settingsmessaging.DecodeRegistrationEmailTemplateSetting(value)
}

func (s *Service) PatchRegistrationEmailTemplateSetting(patch settingsmessaging.RegistrationEmailTemplateSettingPatch) (settingsmessaging.RegistrationEmailTemplateSetting, error) {
	current, err := s.GetRegistrationEmailTemplateSetting()
	if err != nil {
		return settingsmessaging.RegistrationEmailTemplateSetting{}, err
	}
	next, err := settingsmessaging.ApplyRegistrationEmailTemplateSettingPatch(current, patch)
	if err != nil {
		return settingsmessaging.RegistrationEmailTemplateSetting{}, err
	}
	if _, err := s.Update(constants.SettingKeyRegistrationEmailTemplateConfig, map[string]interface{}(settingsmessaging.EncodeRegistrationEmailTemplateSetting(next))); err != nil {
		return settingsmessaging.RegistrationEmailTemplateSetting{}, err
	}
	return next, nil
}

// GetOrderEmailTemplateSetting 获取订单邮件模板配置（优先 settings，空时回退默认）。
func (s *Service) GetOrderEmailTemplateSetting() (settingsmessaging.OrderEmailTemplateSetting, error) {
	fallback := settingsmessaging.DefaultOrderEmailTemplateSetting()
	if s == nil {
		return fallback, nil
	}
	value, err := s.GetByKey(constants.SettingKeyOrderEmailTemplateConfig)
	if err != nil {
		return fallback, err
	}
	if value == nil {
		return fallback, nil
	}
	return settingsmessaging.NormalizeOrderEmailTemplateSetting(settingsmessaging.DecodeOrderEmailTemplateSetting(value, fallback)), nil
}

// PatchOrderEmailTemplateSetting 基于补丁更新订单邮件模板配置。
func (s *Service) PatchOrderEmailTemplateSetting(patch settingsmessaging.OrderEmailTemplateSettingPatch) (settingsmessaging.OrderEmailTemplateSetting, error) {
	current, err := s.GetOrderEmailTemplateSetting()
	if err != nil {
		return settingsmessaging.OrderEmailTemplateSetting{}, err
	}
	next, err := settingsmessaging.ApplyOrderEmailTemplateSettingPatch(current, patch)
	if err != nil {
		return settingsmessaging.OrderEmailTemplateSetting{}, err
	}
	if _, err := s.Update(constants.SettingKeyOrderEmailTemplateConfig, map[string]interface{}(settingsmessaging.EncodeOrderEmailTemplateSetting(next))); err != nil {
		return settingsmessaging.OrderEmailTemplateSetting{}, err
	}
	return next, nil
}

// ResetOrderEmailTemplateSetting 重置订单邮件模板为默认。
func (s *Service) ResetOrderEmailTemplateSetting() (settingsmessaging.OrderEmailTemplateSetting, error) {
	defaultSetting := settingsmessaging.DefaultOrderEmailTemplateSetting()
	if s == nil {
		return defaultSetting, nil
	}
	if _, err := s.Update(constants.SettingKeyOrderEmailTemplateConfig, map[string]interface{}(settingsmessaging.EncodeOrderEmailTemplateSetting(defaultSetting))); err != nil {
		return settingsmessaging.OrderEmailTemplateSetting{}, err
	}
	return defaultSetting, nil
}
