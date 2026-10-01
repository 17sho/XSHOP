package smtp

import (
	"context"
	"github.com/dujiao-next/internal/config"
	notificationcontract "github.com/dujiao-next/internal/modules/notification/contract"
	settingsmessaging "github.com/dujiao-next/internal/modules/settings/schema/messaging"
	"github.com/dujiao-next/internal/shared/mailbrand"
)

func (s *Service) ConfigSnapshot() config.EmailConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg == nil {
		return config.EmailConfig{}
	}
	return *s.cfg
}
func (s *Service) forSend(ctx context.Context) *Service {
	cfg := s.ConfigSnapshot()
	return &Service{cfg: &cfg, ctx: ctx, registrationTemplates: s.registrationTemplates}
}

func (s *Service) SendVerifyCode(toEmail, code, purpose, locale string, brand mailbrand.Brand) error {
	return s.SendVerifyCodeContext(context.Background(), toEmail, code, purpose, locale, brand)
}
func (s *Service) SendVerifyCodeContext(ctx context.Context, toEmail, code, purpose, locale string, brand mailbrand.Brand) error {
	return s.forSend(ctx).sendVerifyCode(toEmail, code, purpose, locale, brand)
}

func (s *Service) SendOrderStatusEmail(toEmail string, input notificationcontract.OrderStatusEmailInput, locale string) error {
	return s.SendOrderStatusEmailContext(context.Background(), toEmail, input, locale)
}
func (s *Service) SendOrderStatusEmailContext(ctx context.Context, toEmail string, input notificationcontract.OrderStatusEmailInput, locale string) error {
	return s.forSend(ctx).sendOrderStatusEmailDefault(toEmail, input, locale)
}

func (s *Service) SendOrderStatusEmailWithTemplate(toEmail string, input notificationcontract.OrderStatusEmailInput, locale string, tmplSetting *settingsmessaging.OrderEmailTemplateSetting) error {
	return s.SendOrderStatusEmailWithTemplateContext(context.Background(), toEmail, input, locale, tmplSetting)
}
func (s *Service) SendOrderStatusEmailWithTemplateContext(ctx context.Context, toEmail string, input notificationcontract.OrderStatusEmailInput, locale string, tmplSetting *settingsmessaging.OrderEmailTemplateSetting) error {
	return s.forSend(ctx).sendOrderStatusEmailWithTemplate(toEmail, input, locale, tmplSetting)
}

func (s *Service) SendCustomEmail(toEmail, subject, body string) error {
	return s.SendCustomEmailContext(context.Background(), toEmail, subject, body)
}
func (s *Service) SendCustomEmailContext(ctx context.Context, toEmail, subject, body string) error {
	return s.forSend(ctx).sendCustomEmail(toEmail, subject, body)
}
