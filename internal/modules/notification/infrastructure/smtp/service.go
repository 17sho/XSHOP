package smtp

import (
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"html"
	"mime"
	"net/mail"
	"net/smtp"
	"strings"
	"sync"
	"time"

	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/i18n"
	"github.com/dujiao-next/internal/logger"
	notificationcontract "github.com/dujiao-next/internal/modules/notification/contract"
	settingsmessaging "github.com/dujiao-next/internal/modules/settings/schema/messaging"
	"github.com/dujiao-next/internal/shared/mailbrand"
	"github.com/dujiao-next/internal/telegramidentity"
)

// writeStandardHeaders 写入 RFC 5322 要求的通用邮件头（Date、Message-ID、From、To、Subject、MIME-Version）。
func writeStandardHeaders(buf *bytes.Buffer, from, to, subject, replyTo string) {
	buf.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))
	buf.WriteString(fmt.Sprintf("Message-ID: %s\r\n", generateMessageID(from)))
	buf.WriteString(fmt.Sprintf("From: %s\r\n", from))
	buf.WriteString(fmt.Sprintf("To: %s\r\n", to))
	if normalized := normalizeReplyToHeader(replyTo); normalized != "" {
		buf.WriteString(fmt.Sprintf("Reply-To: %s\r\n", normalized))
	}
	buf.WriteString(fmt.Sprintf("Subject: %s\r\n", mime.QEncoding.Encode("UTF-8", subject)))
	buf.WriteString("MIME-Version: 1.0\r\n")
}

// generateMessageID 生成 RFC 5322 兼容的 Message-ID，域名取自 From 地址，失败则回退到 localhost。
func generateMessageID(from string) string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	domain := "localhost"
	if addr, err := mail.ParseAddress(from); err == nil {
		if i := strings.LastIndex(addr.Address, "@"); i >= 0 && i < len(addr.Address)-1 {
			domain = addr.Address[i+1:]
		}
	}
	return fmt.Sprintf("<%s@%s>", hex.EncodeToString(b[:]), domain)
}

// RegistrationEmailTemplateProvider keeps SMTP independent of settings storage.
type RegistrationEmailTemplateProvider interface {
	GetRegistrationEmailTemplateSetting() (settingsmessaging.RegistrationEmailTemplateSetting, error)
}

// Service 邮件发送服务
type Service struct {
	mu                    sync.RWMutex
	ctx                   context.Context
	cfg                   *config.EmailConfig
	registrationTemplates RegistrationEmailTemplateProvider
}

// New accepts an optional provider; only registration reads it, on each send.
func New(cfg *config.EmailConfig, providers ...RegistrationEmailTemplateProvider) *Service {
	s := &Service{}
	s.SetConfig(cfg)
	if len(providers) > 0 {
		s.registrationTemplates = providers[0]
	}
	return s
}

// SetConfig 更新运行时邮件配置
func (s *Service) SetConfig(cfg *config.EmailConfig) {
	if cfg == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	copyConfig := *cfg
	s.cfg = &copyConfig
}

// SendVerifyCode 发送邮箱验证码
func (s *Service) sendVerifyCode(toEmail, code, purpose, locale string, brand mailbrand.Brand) error {
	subject, body, markup := s.buildVerificationContent(code, purpose, locale, brand)
	return s.sendVerificationEmail(toEmail, subject, body, markup, brand)
}

var ErrEmailHeaderInvalid = fmt.Errorf("email header contains CR or LF")

func (s *Service) sendVerificationEmail(toEmail, subject, plain, htmlBody string, brand mailbrand.Brand) error {
	headers := []string{toEmail, subject, brand.FromName, brand.ReplyTo}
	if s.cfg != nil {
		headers = append(headers, s.cfg.From, s.cfg.FromName)
	}
	for _, value := range headers {
		if strings.ContainsAny(value, "\r\n") {
			return ErrEmailHeaderInvalid
		}
	}
	if telegramidentity.IsPlaceholderEmail(toEmail) {
		return nil
	}
	from, addr, err := s.prepareSMTPEnvelope(toEmail, brand.FromName)
	if err != nil {
		return err
	}
	msg := buildVerificationEmailMessage(from, toEmail, subject, plain, htmlBody, brand.ReplyTo)
	return s.sendSMTPMessage(addr, toEmail, []byte(msg))
}

//go:embed xshop-logo-v3-email.png
var inlineVerifyLogo []byte

const verifyLogoURL = "https://shop.example.com/xshop-logo-v3-email.png"
const verifyLogoCID = "xshop-logo@mail.shop.example.com"

func buildVerificationEmailMessage(from, to, subject, plain, htmlBody, replyTo string) string {
	// Only the verified main-site asset may be embedded. Other tenants keep
	// their own remote image or plain-text fallback, never the main site's logo.
	if !strings.Contains(htmlBody, `src="`+verifyLogoURL+`"`) {
		return buildAlternativeEmailMessage(from, to, subject, plain, htmlBody, replyTo)
	}
	htmlBody = strings.Replace(htmlBody, `src="`+verifyLogoURL+`"`, `src="cid:`+verifyLogoCID+`"`, 1)
	var buf bytes.Buffer
	writeStandardHeaders(&buf, from, to, subject, replyTo)
	outer := "=_verify_" + hex.EncodeToString(randomBoundaryBytes())
	inner := "=_related_" + hex.EncodeToString(randomBoundaryBytes())
	buf.WriteString("Content-Type: multipart/alternative; boundary=\"" + outer + "\"\r\n\r\n")
	buf.WriteString("--" + outer + "\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: base64\r\n\r\n")
	writeMailBase64(&buf, []byte(plain))
	buf.WriteString("--" + outer + "\r\nContent-Type: multipart/related; boundary=\"" + inner + "\"\r\n\r\n")
	buf.WriteString("--" + inner + "\r\nContent-Type: text/html; charset=UTF-8\r\nContent-Transfer-Encoding: base64\r\n\r\n")
	writeMailBase64(&buf, []byte(htmlBody))
	buf.WriteString("--" + inner + "\r\nContent-Type: image/png\r\nContent-Transfer-Encoding: base64\r\nContent-ID: <" + verifyLogoCID + ">\r\nContent-Disposition: inline\r\n\r\n")
	writeMailBase64(&buf, inlineVerifyLogo)
	buf.WriteString("--" + inner + "--\r\n--" + outer + "--\r\n")
	return buf.String()
}

func writeMailBase64(buf *bytes.Buffer, data []byte) {
	encoded := base64.StdEncoding.EncodeToString(data)
	for len(encoded) > 76 {
		buf.WriteString(encoded[:76] + "\r\n")
		encoded = encoded[76:]
	}
	buf.WriteString(encoded + "\r\n")
}

func buildAlternativeEmailMessage(from, to, subject, plain, htmlBody, replyTo string) string {
	var buf bytes.Buffer
	writeStandardHeaders(&buf, from, to, subject, replyTo)
	boundary := "=_verify_" + strings.Trim(hex.EncodeToString(randomBoundaryBytes()), " ")
	buf.WriteString("Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n\r\n")
	for _, part := range []struct{ contentType, body string }{{"text/plain", plain}, {"text/html", htmlBody}} {
		buf.WriteString("--" + boundary + "\r\n")
		buf.WriteString("Content-Type: " + part.contentType + "; charset=UTF-8\r\n")
		buf.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
		encoded := base64.StdEncoding.EncodeToString([]byte(part.body))
		for len(encoded) > 76 {
			buf.WriteString(encoded[:76] + "\r\n")
			encoded = encoded[76:]
		}
		buf.WriteString(encoded + "\r\n")
	}
	buf.WriteString("--" + boundary + "--\r\n")
	return buf.String()
}

func randomBoundaryBytes() []byte {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return b[:]
}

func buildVerifyCodeHTML(code, purpose, locale string, brand mailbrand.Brand, minutes int, configuredBody ...string) string {
	name := strings.TrimSpace(brand.SiteName)
	if name == "" {
		name = "商城"
	}
	lang := normalizeLocale(locale)
	label, title, intro, expiry, warning, ignore, signature := "安全验证", "邮箱验证码", "请使用下方验证码完成操作。", fmt.Sprintf("验证码 %d 分钟内有效", minutes), "请勿向任何人透露验证码。", "如果不是您本人操作，请忽略此邮件。", "团队"
	switch lang {
	case i18n.LocaleTW:
		label, title, intro, expiry, warning, ignore, signature = "安全驗證", "郵箱驗證碼", "請使用下方驗證碼完成操作。", fmt.Sprintf("驗證碼 %d 分鐘內有效", minutes), "請勿向任何人透露驗證碼。", "如果不是您本人操作，請忽略此郵件。", "團隊"
	case i18n.LocaleEN:
		label, title, intro, expiry, warning, ignore, signature = "Security verification", "Email verification code", "Use the code below to complete your request.", fmt.Sprintf("This code expires in %d minutes.", minutes), "Do not share this code with anyone.", "If you didn't request this, you can ignore this email.", "Team"
	}
	purposeKey := strings.ToLower(strings.TrimSpace(purpose))
	if lang == i18n.LocaleEN {
		switch purposeKey {
		case constants.VerifyPurposeRegister:
			label, intro = "Registration", "Use this code to finish creating your account."
		case constants.VerifyPurposeReset:
			label, intro = "Password reset", "Use this code to reset your password."
		case constants.VerifyPurposeTelegramBind:
			label, intro = "Telegram binding", "Use this code to bind Telegram."
		case constants.VerifyPurposeChangeEmailOld, constants.VerifyPurposeChangeEmailNew:
			label, intro = "Change email", "Use this code to change your email."
		}
	} else if lang == i18n.LocaleTW {
		switch purposeKey {
		case constants.VerifyPurposeRegister:
			label, intro = "註冊驗證", "請使用下方驗證碼完成帳號註冊。"
		case constants.VerifyPurposeReset:
			label, intro = "重置密碼", "請使用下方驗證碼重置密碼。"
		case constants.VerifyPurposeTelegramBind:
			label, intro = "綁定 Telegram", "請使用下方驗證碼綁定 Telegram。"
		case constants.VerifyPurposeChangeEmailOld, constants.VerifyPurposeChangeEmailNew:
			label, intro = "更換郵箱", "請使用下方驗證碼更換郵箱。"
		}
	} else {
		switch purposeKey {
		case constants.VerifyPurposeRegister:
			label, intro = "注册验证", "请使用下方验证码完成账号注册。"
		case constants.VerifyPurposeReset:
			label, intro = "重置密码", "请使用下方验证码重置密码。"
		case constants.VerifyPurposeTelegramBind:
			label, intro = "绑定 Telegram", "请使用下方验证码绑定 Telegram。"
		case constants.VerifyPurposeChangeEmailOld, constants.VerifyPurposeChangeEmailNew:
			label, intro = "更换邮箱", "请使用下方验证码更换邮箱。"
		}
	}
	clean := func(value string) string { return html.EscapeString(value) }
	introHTML := clean(intro)
	if len(configuredBody) > 0 {
		introHTML = strings.ReplaceAll(clean(configuredBody[0]), "\n", "<br>")
	}
	return renderVerifyCodeCard(verifyCodeCard{
		Lang: clean(lang), Name: clean(name), Label: clean(label), Title: clean(title), Intro: introHTML,
		Code: clean(code), Expiry: clean(expiry), Warning: clean(warning), Ignore: clean(ignore),
		Signature: clean(signature), SiteURL: brand.SiteURL, SiteLogo: brand.SiteLogo, SiteIcon: brand.SiteIcon,
	})
}

// SendOrderStatusEmail 发送订单状态通知
func (s *Service) sendOrderStatusEmailDefault(toEmail string, input notificationcontract.OrderStatusEmailInput, locale string) error {
	subject, body := buildOrderStatusContent(input, locale)
	return s.sendOrderStatusEmail(toEmail, subject, body, input, locale)
}

// SendOrderStatusEmailWithTemplate 使用可配置模板发送订单状态通知
func (s *Service) sendOrderStatusEmailWithTemplate(toEmail string, input notificationcontract.OrderStatusEmailInput, locale string, tmplSetting *settingsmessaging.OrderEmailTemplateSetting) error {
	if tmplSetting == nil {
		return s.sendOrderStatusEmailDefault(toEmail, input, locale)
	}
	if err := settingsmessaging.ValidateOrderEmailTemplateSetting(settingsmessaging.NormalizeOrderEmailTemplateSetting(*tmplSetting)); err != nil {
		return err
	}
	subject, body := buildOrderStatusContentFromTemplate(input, locale, *tmplSetting)
	return s.sendOrderStatusEmailWithHTML(toEmail, subject, body, buildOrderStatusHTMLFromTemplate(input, locale, body, *tmplSetting), input)
}

func (s *Service) sendOrderStatusEmail(toEmail, subject, plain string, input notificationcontract.OrderStatusEmailInput, locale string) error {
	return s.sendOrderStatusEmailWithHTML(toEmail, subject, plain, buildOrderStatusHTMLWithBody(input, locale, plain), input)
}

func (s *Service) sendOrderStatusEmailWithHTML(toEmail, subject, plain, htmlBody string, input notificationcontract.OrderStatusEmailInput) error {
	if telegramidentity.IsPlaceholderEmail(toEmail) {
		return nil
	}
	from, addr, err := s.prepareSMTPEnvelope(toEmail, input.MailBrand.FromName)
	if err != nil {
		return err
	}
	msg := buildOrderStatusEmailMessage(from, toEmail, subject, plain, htmlBody, input.MailBrand.ReplyTo, input.AttachmentName, input.AttachmentContent, input.MailBrand)
	return s.sendSMTPMessage(addr, toEmail, []byte(msg))
}

func buildOrderStatusContentFromTemplate(input notificationcontract.OrderStatusEmailInput, locale string, tmplSetting settingsmessaging.OrderEmailTemplateSetting) (string, string) {
	normalized := normalizeLocale(locale)

	status := strings.ToLower(strings.TrimSpace(input.Status))
	sceneTmpl := selectOrderEmailScene(tmplSetting, input)

	localeTmpl := settingsmessaging.ResolveOrderEmailLocaleTemplate(sceneTmpl, normalized)

	statusLabel := localizedOrderStatusLabel(normalized, input.Status)

	variables := map[string]interface{}{
		"order_no":         input.OrderNo,
		"status":           statusLabel,
		"amount":           input.Amount.String(),
		"refund_amount":    "",
		"refund_reason":    "",
		"currency":         strings.TrimSpace(input.Currency),
		"site_name":        strings.TrimSpace(input.SiteName),
		"site_url":         strings.TrimSpace(input.SiteURL),
		"fulfillment_info": strings.TrimSpace(input.FulfillmentInfo),
		"instructions":     strings.TrimSpace(input.Instructions),
	}
	if status == constants.OrderStatusRefunded || status == constants.OrderStatusPartiallyRefunded {
		variables["refund_amount"] = input.RefundAmount.String()
		variables["refund_reason"] = strings.TrimSpace(input.RefundReason)
	}

	subject := renderTemplate(localeTmpl.Subject, variables)
	body := renderTemplate(localeTmpl.Body, variables)

	// 存量兼容：历史自定义模板未引用 {{instructions}} 时自动在末尾追加使用说明，避免因占位符缺失导致说明丢失。
	// 只在交付含内容场景生效（此时 input.Instructions 才会被填充）。
	if strings.TrimSpace(input.Instructions) != "" && !strings.Contains(localeTmpl.Body, "{{instructions}}") {
		body = strings.TrimRight(body, "\n") + "\n\n" + strings.TrimSpace(input.Instructions)
	}

	// 交付内容以附件形式发送时追加提示
	if input.AttachmentName != "" {
		tip := strings.TrimSpace(settingsmessaging.ResolveOrderEmailFulfillmentAttachmentTip(tmplSetting.FulfillmentAttachmentTip, normalized))
		if tip != "" {
			body = body + "\n\n" + tip
		}
	}

	// 游客订单追加提示
	if input.IsGuest {
		tip := strings.TrimSpace(settingsmessaging.ResolveOrderEmailGuestTip(tmplSetting.GuestTip, normalized))
		if tip != "" {
			body = body + "\n\n" + tip
		}
	}

	return subject, body
}

func orderEmailHasDeliveryContent(input notificationcontract.OrderStatusEmailInput) bool {
	return strings.TrimSpace(input.FulfillmentInfo) != "" || strings.TrimSpace(input.AttachmentName) != "" || strings.TrimSpace(input.AttachmentContent) != ""
}

func selectOrderEmailScene(setting settingsmessaging.OrderEmailTemplateSetting, input notificationcontract.OrderStatusEmailInput) settingsmessaging.OrderEmailSceneTemplate {
	switch strings.ToLower(strings.TrimSpace(input.Status)) {
	case constants.OrderStatusPaid:
		return setting.Templates.Paid
	case constants.OrderStatusDelivered, constants.OrderStatusCompleted:
		if orderEmailHasDeliveryContent(input) {
			return setting.Templates.DeliveredWithContent
		}
		return setting.Templates.Delivered
	case constants.OrderStatusRefunded:
		return setting.Templates.Refunded
	case constants.OrderStatusPartiallyRefunded:
		return setting.Templates.PartiallyRefunded
	default:
		return setting.Templates.Default
	}
}

func localizedOrderStatusLabel(locale, status string) string {
	normalizedStatus := strings.ToLower(strings.TrimSpace(status))
	statusKey := "order.status." + normalizedStatus
	label := i18n.T(locale, statusKey)
	if label == statusKey {
		return status
	}
	return label
}

func buildOrderStatusHTMLFromTemplate(input notificationcontract.OrderStatusEmailInput, locale, plain string, setting settingsmessaging.OrderEmailTemplateSetting) string {
	normalizedLocale := normalizeLocale(locale)
	localized := settingsmessaging.ResolveOrderEmailLocaleTemplate(selectOrderEmailScene(setting, input), normalizedLocale)
	if !localized.CustomHTMLEnabled || strings.TrimSpace(localized.CustomHTML) == "" {
		return buildOrderStatusHTMLWithBodyAndModules(input, locale, plain, setting.Modules)
	}
	variables := map[string]interface{}{"order_no": input.OrderNo, "status": localizedOrderStatusLabel(normalizedLocale, input.Status), "amount": input.Amount.String(), "currency": input.Currency, "refund_amount": input.RefundAmount.String(), "refund_reason": input.RefundReason, "site_name": input.SiteName, "site_url": input.SiteURL, "fulfillment_info": input.FulfillmentInfo, "instructions": input.Instructions}
	for key, value := range variables {
		variables[key] = html.EscapeString(strings.TrimSpace(fmt.Sprintf("%v", value)))
	}
	return renderTemplate(localized.CustomHTML, variables)
}

// SendCustomEmail 发送测试邮件或自定义邮件
func (s *Service) sendCustomEmail(toEmail, subject, body string) error {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		subject = "SMTP 配置测试邮件"
	}
	body = strings.TrimSpace(body)
	if body == "" {
		body = "这是一封来自 Dujiao-Next 的 SMTP 测试邮件，说明当前配置可正常发送。"
	}
	return s.sendTextEmail(toEmail, subject, body)
}

func (s *Service) sendTextEmail(toEmail, subject, body string, brands ...mailbrand.Brand) error {
	if telegramidentity.IsPlaceholderEmail(toEmail) {
		return nil
	}
	brand := firstMailBrand(brands)
	from, addr, err := s.prepareSMTPEnvelope(toEmail, brand.FromName)
	if err != nil {
		return err
	}
	msg := buildEmailMessage(from, toEmail, subject, body, brand.ReplyTo)
	return s.sendSMTPMessage(addr, toEmail, []byte(msg))
}

// prepareSMTPEnvelope 校验配置与收件人，并返回发件地址与 SMTP 服务器地址。
func (s *Service) prepareSMTPEnvelope(toEmail, fromNameOverride string) (string, string, error) {
	if s.cfg == nil || !s.cfg.Enabled {
		return "", "", notificationcontract.ErrEmailServiceDisabled
	}
	if s.cfg.Host == "" || s.cfg.Port == 0 || s.cfg.From == "" {
		return "", "", notificationcontract.ErrEmailNotConfigured
	}
	if _, err := mail.ParseAddress(toEmail); err != nil {
		return "", "", notificationcontract.ErrInvalidEmail
	}
	fromName := strings.TrimSpace(fromNameOverride)
	if fromName == "" {
		fromName = s.cfg.FromName
	}
	from := buildFromAddress(s.cfg.From, fromName)
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	return from, addr, nil
}

// sendSMTPMessage 根据配置选择 SSL/STARTTLS/明文通道发送邮件。
func (s *Service) sendSMTPMessage(addr, toEmail string, msg []byte) error {
	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	mode := "plain"
	if s.cfg.UseSSL {
		mode = "ssl"
	} else if s.cfg.UseTLS {
		mode = "starttls"
	}
	return normalizeEmailSendError(sendMailContext(ctx, mode, addr, s.cfg.Host, s.cfg.From, []string{toEmail}, msg, s.cfg.Username, s.cfg.Password))
}

func buildVerifyCodeContent(code, purpose, locale string, brands ...mailbrand.Brand) (string, string) {
	normalized := normalizeLocale(locale)
	purposeKey := strings.ToLower(strings.TrimSpace(purpose))
	brand := firstMailBrand(brands)
	switch normalized {
	case i18n.LocaleTW:
		subject := "郵箱驗證碼"
		purposeText := "郵箱驗證"
		switch purposeKey {
		case constants.VerifyPurposeRegister:
			subject = "註冊驗證碼"
			purposeText = "註冊"
		case constants.VerifyPurposeReset:
			subject = "重置密碼驗證碼"
			purposeText = "重置密碼"
		case constants.VerifyPurposeTelegramBind:
			subject = "Telegram 綁定驗證碼"
			purposeText = "綁定 Telegram"
		case constants.VerifyPurposeChangeEmailOld, constants.VerifyPurposeChangeEmailNew:
			subject = "更換郵箱驗證碼"
			purposeText = "更換郵箱"
		}
		body := fmt.Sprintf("您的驗證碼是：%s\n\n該驗證碼用於 %s，請勿洩露。", code, purposeText)
		return applyVerifyCodeBrand(normalized, subject, body, brand)
	case i18n.LocaleEN:
		subject := "Email Verification Code"
		purposeText := "email verification"
		switch purposeKey {
		case constants.VerifyPurposeRegister:
			subject = "Registration Code"
			purposeText = "registration"
		case constants.VerifyPurposeReset:
			subject = "Password Reset Code"
			purposeText = "password reset"
		case constants.VerifyPurposeTelegramBind:
			subject = "Telegram Binding Code"
			purposeText = "binding Telegram"
		case constants.VerifyPurposeChangeEmailOld, constants.VerifyPurposeChangeEmailNew:
			subject = "Change Email Code"
			purposeText = "change email"
		}
		body := fmt.Sprintf("Your verification code is: %s\n\nThis code is for %s. Do not share it.", code, purposeText)
		return applyVerifyCodeBrand(normalized, subject, body, brand)
	default:
		subject := "邮箱验证码"
		purposeText := "邮箱验证"
		switch purposeKey {
		case constants.VerifyPurposeRegister:
			subject = "注册验证码"
			purposeText = "注册"
		case constants.VerifyPurposeReset:
			subject = "重置密码验证码"
			purposeText = "重置密码"
		case constants.VerifyPurposeTelegramBind:
			subject = "Telegram 绑定验证码"
			purposeText = "绑定 Telegram"
		case constants.VerifyPurposeChangeEmailOld, constants.VerifyPurposeChangeEmailNew:
			subject = "更换邮箱验证码"
			purposeText = "更换邮箱"
		}
		body := fmt.Sprintf("您的验证码是：%s\n\n该验证码用于 %s，请勿泄露。", code, purposeText)
		return applyVerifyCodeBrand(normalized, subject, body, brand)
	}
}

func firstMailBrand(brands []mailbrand.Brand) mailbrand.Brand {
	if len(brands) == 0 {
		return mailbrand.Brand{}
	}
	return brands[0]
}

func applyVerifyCodeBrand(locale, subject, body string, brand mailbrand.Brand) (string, string) {
	siteName := strings.TrimSpace(brand.SiteName)
	siteURL := strings.TrimRight(strings.TrimSpace(brand.SiteURL), "/")
	if siteName == "" && siteURL == "" {
		return subject, body
	}
	if siteName != "" {
		subject = siteName + " - " + subject
	}
	switch locale {
	case i18n.LocaleTW:
		if siteName != "" {
			body += "\n\n站點：" + siteName
		}
		if siteURL != "" {
			body += "\n網址：" + siteURL
		}
	case i18n.LocaleEN:
		if siteName != "" {
			body += "\n\nSite: " + siteName
		}
		if siteURL != "" {
			body += "\nURL: " + siteURL
		}
	default:
		if siteName != "" {
			body += "\n\n站点：" + siteName
		}
		if siteURL != "" {
			body += "\n网址：" + siteURL
		}
	}
	return subject, body
}

func buildOrderStatusContent(input notificationcontract.OrderStatusEmailInput, locale string) (string, string) {
	normalized := normalizeLocale(locale)
	statusKey := "order.status." + strings.ToLower(strings.TrimSpace(input.Status))
	statusLabel := i18n.T(normalized, statusKey)
	if statusLabel == statusKey {
		statusLabel = input.Status
	}
	amount := input.Amount.String()
	refundAmount := input.RefundAmount.String()
	refundReason := strings.TrimSpace(input.RefundReason)
	currency := strings.TrimSpace(input.Currency)
	siteName := strings.TrimSpace(input.SiteName)
	siteURL := strings.TrimSpace(input.SiteURL)
	subject := i18n.Sprintf(normalized, "email.order_status.subject", statusLabel)
	payload := strings.TrimSpace(input.FulfillmentInfo)
	status := strings.ToLower(strings.TrimSpace(input.Status))
	switch status {
	case constants.OrderStatusDelivered, constants.OrderStatusCompleted:
		if payload != "" {
			body := i18n.Sprintf(normalized, "email.order_status.body_delivered", input.OrderNo, statusLabel, amount, currency, payload, siteName, siteURL)
			return subject, appendGuestTip(normalized, input, appendFulfillmentAttachmentTip(normalized, input, body))
		}
		body := i18n.Sprintf(normalized, "email.order_status.body_delivered_simple", input.OrderNo, statusLabel, amount, currency, siteName, siteURL)
		return subject, appendGuestTip(normalized, input, appendFulfillmentAttachmentTip(normalized, input, body))
	case constants.OrderStatusPaid:
		body := i18n.Sprintf(normalized, "email.order_status.body_paid", input.OrderNo, statusLabel, amount, currency, siteName, siteURL)
		return subject, appendGuestTip(normalized, input, appendFulfillmentAttachmentTip(normalized, input, body))
	case constants.OrderStatusRefunded:
		body := i18n.Sprintf(normalized, "email.order_status.body_refunded", input.OrderNo, statusLabel, refundAmount, currency, refundReason, siteName, siteURL)
		return subject, appendGuestTip(normalized, input, appendFulfillmentAttachmentTip(normalized, input, body))
	case constants.OrderStatusPartiallyRefunded:
		body := i18n.Sprintf(normalized, "email.order_status.body_partially_refunded", input.OrderNo, statusLabel, refundAmount, currency, refundReason, siteName, siteURL)
		return subject, appendGuestTip(normalized, input, appendFulfillmentAttachmentTip(normalized, input, body))
	default:
		body := i18n.Sprintf(normalized, "email.order_status.body", input.OrderNo, statusLabel, amount, currency, siteName, siteURL)
		return subject, appendGuestTip(normalized, input, appendFulfillmentAttachmentTip(normalized, input, body))
	}
}

func appendFulfillmentAttachmentTip(locale string, input notificationcontract.OrderStatusEmailInput, body string) string {
	if input.AttachmentName == "" {
		return body
	}
	tipKey := "email.order_status.fulfillment_attachment_tip"
	tip := i18n.T(locale, tipKey)
	if tip == tipKey {
		return body
	}
	return body + "\n\n" + tip
}

func appendGuestTip(locale string, input notificationcontract.OrderStatusEmailInput, body string) string {
	if !input.IsGuest {
		return body
	}
	tipKey := "email.order_status.guest_tip"
	tip := i18n.T(locale, tipKey)
	if tip == tipKey {
		return body
	}
	return body + "\n\n" + tip
}

func normalizeLocale(locale string) string {
	l := strings.ToLower(strings.TrimSpace(locale))
	switch {
	case strings.HasPrefix(l, "zh-tw"), strings.HasPrefix(l, "zh-hk"), strings.HasPrefix(l, "zh-mo"):
		return i18n.LocaleTW
	case strings.HasPrefix(l, "en"):
		return i18n.LocaleEN
	default:
		return i18n.LocaleZH
	}
}

func buildFromAddress(from, name string) string {
	name = strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ").Replace(name))
	if name == "" {
		return from
	}
	// mail.Address.String performs the required RFC 2047 encoding itself.
	// Pre-encoding here would make clients display the encoded-word literally.
	return (&mail.Address{Name: name, Address: from}).String()
}

func normalizeReplyToHeader(raw string) string {
	addr, err := mail.ParseAddress(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return addr.Address
}

func buildEmailMessage(from, to, subject, body string, replyTo ...string) string {
	var buf bytes.Buffer
	resolvedReplyTo := ""
	if len(replyTo) > 0 {
		resolvedReplyTo = replyTo[0]
	}
	writeStandardHeaders(&buf, from, to, subject, resolvedReplyTo)
	buf.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(body)
	return buf.String()
}

func sendMailWithSSL(addr, host, from string, to []string, msg []byte, username, password string) error {
	return sendMailContext(context.Background(), "ssl", addr, host, from, to, msg, username, password)
}

func sendMailWithStartTLS(addr, host, from string, to []string, msg []byte, username, password string) error {
	return sendMailContext(context.Background(), "starttls", addr, host, from, to, msg, username, password)
}

func sendMailPlain(addr, host, from string, to []string, msg []byte, username, password string) error {
	return sendMailContext(context.Background(), "plain", addr, host, from, to, msg, username, password)
}

const (
	smtpAuthMechanismPlain = "PLAIN"
	smtpAuthMechanismLogin = "LOGIN"
	// 还有 XOAUTH2 等机制，当前实现暂不处理。
)

// authenticateSMTPClient 根据服务端 AUTH 能力选择并执行认证。
// Service 认证策略：优先 LOGIN，回退 PLAIN。
// 对 smtp.office365.com 的 SMTP Basic/LOGIN 场景，通常需要开启 MFA 并使用应用密码。
func authenticateSMTPClient(client *smtp.Client, host, username, password string) error {
	if client == nil {
		return nil
	}
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if username == "" && password == "" {
		return nil
	}

	ok, advertised := client.Extension("AUTH")
	if !ok {
		return nil
	}

	switch pickSMTPAuthMechanism(advertised) {
	case smtpAuthMechanismLogin:
		return client.Auth(newLoginAuth(username, password, host))
	case smtpAuthMechanismPlain:
		return client.Auth(smtp.PlainAuth("", username, password, host))
	default:
		return fmt.Errorf("smtp auth mechanism not supported (server AUTH=%q)", advertised)
	}
}

// pickSMTPAuthMechanism 根据服务端 AUTH 能力选择机制，优先 LOGIN，再回退 PLAIN。
func pickSMTPAuthMechanism(advertised string) string {
	if hasSMTPAuthMechanism(advertised, smtpAuthMechanismLogin) {
		return smtpAuthMechanismLogin
	}
	if hasSMTPAuthMechanism(advertised, smtpAuthMechanismPlain) {
		return smtpAuthMechanismPlain
	}
	return ""
}

// hasSMTPAuthMechanism 判断服务端 AUTH 扩展是否包含指定机制。
func hasSMTPAuthMechanism(advertised, mechanism string) bool {
	if strings.TrimSpace(mechanism) == "" {
		return false
	}
	tokens := strings.Fields(strings.ToUpper(strings.TrimSpace(advertised)))
	needle := strings.ToUpper(strings.TrimSpace(mechanism))
	for _, token := range tokens {
		if token == needle {
			return true
		}
	}
	return false
}

type loginAuth struct {
	username string
	password string
	host     string
	userSent bool
}

// newLoginAuth 构造 AUTH LOGIN 认证器。
func newLoginAuth(username, password, host string) smtp.Auth {
	return &loginAuth{username: username, password: password, host: host}
}

// Start 校验连接安全性并声明 LOGIN 机制。
func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if server == nil {
		return "", nil, fmt.Errorf("smtp server info is required")
	}
	if server.Name != a.host {
		return "", nil, fmt.Errorf("wrong host name")
	}
	if !server.TLS {
		return "", nil, fmt.Errorf("unencrypted connection")
	}
	a.userSent = false
	return smtpAuthMechanismLogin, nil, nil
}

// Next 按服务端 challenge 顺序回送用户名与密码。
func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	challenge := strings.ToLower(strings.TrimSpace(string(fromServer)))
	if strings.Contains(challenge, "password") {
		return []byte(a.password), nil
	}
	if strings.Contains(challenge, "username") || strings.Contains(challenge, "user name") {
		a.userSent = true
		return []byte(a.username), nil
	}
	if !a.userSent {
		a.userSent = true
		return []byte(a.username), nil
	}
	return []byte(a.password), nil
}

type smtpSessionCloser interface {
	Quit() error
	Close() error
}

func smtpEnvelopeSender(from string) string {
	address, err := mail.ParseAddress(from)
	if err != nil {
		return from
	}
	return address.Address
}

// sendSMTPData 发送 SMTP Envelope 与邮件正文。
func sendSMTPData(client *smtp.Client, host, addr, from string, to []string, msg []byte) error {
	if err := client.Mail(smtpEnvelopeSender(from)); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return quitSMTPClient(client, host, addr)
}

// quitSMTPClient 优先执行 SMTP QUIT；仅在 QUIT 失败时补偿 Close 回收连接。
func quitSMTPClient(client smtpSessionCloser, host, addr string) error {
	if client == nil {
		return nil
	}
	if err := client.Quit(); err != nil {
		if closeErr := client.Close(); closeErr != nil && !isSMTPAlreadyClosedError(closeErr) {
			logger.Debugw("smtp_close_after_quit_failed", "host", host, "addr", addr, "error", closeErr)
		}
		return err
	}
	return nil
}

// closeSMTPClientOnError 仅在发送流程异常时兜底 Close，避免成功路径重复关闭噪音。
func closeSMTPClientOnError(client *smtp.Client, sendErr *error, host, addr string) {
	if client == nil || sendErr == nil || *sendErr == nil {
		return
	}
	if err := client.Close(); err != nil && !isSMTPAlreadyClosedError(err) {
		logger.Debugw("smtp_close_failed", "host", host, "addr", addr, "error", err)
	}
}

// isSMTPAlreadyClosedError 识别连接已关闭类错误，避免重复记录无效噪音。
func isSMTPAlreadyClosedError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	if message == "" {
		return false
	}
	return strings.Contains(message, "use of closed network connection") ||
		strings.Contains(message, "closed connection") ||
		strings.Contains(message, "connection is closed")
}

// normalizeEmailSendError 将可识别的收件人拒绝错误归一化为业务错误码。
func normalizeEmailSendError(err error) error {
	if err == nil {
		return nil
	}
	if isEmailRecipientRejected(err) {
		return notificationcontract.ErrEmailRecipientRejected
	}
	return err
}

// isEmailRecipientRejected 识别常见 SMTP 收件人不存在/被拒绝错误。
func isEmailRecipientRejected(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	if message == "" {
		return false
	}
	directKeywords := []string{
		"no such recipient",
		"no such user",
		"recipient not found",
		"recipient address rejected",
		"invalid recipient",
		"user unknown",
		"unknown user",
		"unknown mailbox",
		"mailbox unavailable",
	}
	for _, keyword := range directKeywords {
		if strings.Contains(message, keyword) {
			return true
		}
	}
	if strings.Contains(message, "550") {
		hints := []string{"recipient", "user", "mailbox", "address", "rcpt"}
		for _, hint := range hints {
			if strings.Contains(message, hint) {
				return true
			}
		}
	}
	return false
}
