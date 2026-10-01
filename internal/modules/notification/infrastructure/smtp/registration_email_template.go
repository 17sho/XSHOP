package smtp

import (
	"bytes"
	"html"
	"strconv"
	"strings"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/i18n"
	"github.com/dujiao-next/internal/logger"
	settingsmessaging "github.com/dujiao-next/internal/modules/settings/schema/messaging"
	"github.com/dujiao-next/internal/shared/mailbrand"
	xhtml "golang.org/x/net/html"
)

func (s *Service) buildVerificationContent(code, purpose, locale string, brand mailbrand.Brand) (subject, plain, markup string) {
	minutes := 10
	if s.cfg != nil && s.cfg.VerifyCode.ExpireMinutes > 0 {
		minutes = s.cfg.VerifyCode.ExpireMinutes
	}
	purposeKey := strings.ToLower(strings.TrimSpace(purpose))
	switch purposeKey {
	case constants.VerifyPurposeRegister, constants.VerifyPurposeReset, constants.VerifyPurposeTelegramBind, constants.VerifyPurposeChangeEmailOld, constants.VerifyPurposeChangeEmailNew:
	default:
		subject, plain = buildVerifyCodeContent(code, purpose, locale, brand)
		return subject, plain, buildVerifyCodeHTML(code, purpose, locale, brand, minutes)
	}
	setting := settingsmessaging.DefaultRegistrationEmailTemplateSetting()
	if s.registrationTemplates != nil {
		candidate, err := s.registrationTemplates.GetRegistrationEmailTemplateSetting()
		if err == nil {
			err = settingsmessaging.ValidateRegistrationEmailTemplateSetting(candidate)
		}
		if err == nil {
			setting = candidate
		} else {
			// Template availability must not cause a valid verification code to be lost.
			// Never log recipient, code, or template contents.
			logger.Warnw("registration_email_template_fallback")
		}
	}
	scene := setting.Templates
	if purposeKey != constants.VerifyPurposeRegister {
		scene = settingsmessaging.DefaultVerificationEmailScenes()[purposeKey]
		if configured, ok := setting.Scenes[purposeKey]; ok {
			scene = configured
		}
	}
	lt := settingsmessaging.ResolveOrderEmailLocaleTemplate(scene, normalizeLocale(locale))
	render := func(text string, escape bool) string {
		pairs := []string{}
		for _, pair := range [][2]string{{"code", code}, {"expire_minutes", strconv.Itoa(minutes)}, {"site_name", brand.SiteName}, {"site_url", brand.SiteURL}} {
			value := pair[1]
			if escape {
				value = html.EscapeString(value)
			}
			pairs = append(pairs, "{{"+pair[0]+"}}", value)
		}
		return strings.NewReplacer(pairs...).Replace(text)
	}
	subject = render(lt.Subject, false)
	plain = render(lt.Body, false) + "\n\n" + registrationSecurityFooter(locale)
	if lt.CustomHTMLEnabled {
		markup = registrationHTMLWithFooter(render(lt.CustomHTML, true), locale)
	} else {
		defaultTemplate := settingsmessaging.ResolveOrderEmailLocaleTemplate(settingsmessaging.DefaultRegistrationEmailTemplateSetting().Templates, normalizeLocale(locale))
		if lt.Body == defaultTemplate.Body {
			markup = buildVerifyCodeHTML(code, purpose, locale, brand, minutes)
		} else {
			markup = buildVerifyCodeHTML(code, purpose, locale, brand, minutes, render(lt.Body, false))
		}
	}
	return
}

// These warnings are deliberately not part of the configurable template.
func registrationSecurityFooter(locale string) string {
	switch normalizeLocale(locale) {
	case i18n.LocaleTW:
		return "請勿向任何人透露驗證碼。\n如果不是您本人操作，請忽略此郵件。\n本郵件由系統自動發送。我們不會索取您的密碼、付款資訊或驗證碼，請留意冒充客服的訊息。"
	case i18n.LocaleEN:
		return "Do not share this code with anyone.\nIf you didn't request this, you can ignore this email.\nThis is an automated email. We will never ask for your password, payment details, or verification code."
	default:
		return "请勿向任何人透露验证码。\n如果不是您本人操作，请忽略此邮件。\n本邮件由系统自动发送。我们不会索要您的密码、支付信息或验证码，请谨防冒充客服的消息。"
	}
}

func registrationHTMLWithFooter(markup, locale string) string {
	// Parse and re-render the body so an unclosed table/comment cannot swallow
	// the fixed footer. Document/head/body attributes cannot style that footer.
	doc, err := xhtml.Parse(strings.NewReader(markup))
	var content bytes.Buffer
	if err == nil {
		var walk func(*xhtml.Node)
		walk = func(n *xhtml.Node) {
			if n.Type == xhtml.ElementNode && n.Data == "body" {
				for child := n.FirstChild; child != nil; child = child.NextSibling {
					_ = xhtml.Render(&content, child)
				}
				return
			}
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
		}
		walk(doc)
	}
	footer := strings.ReplaceAll(html.EscapeString(registrationSecurityFooter(locale)), "\n", "<br>")
	return `<!doctype html><html><body><div>` + content.String() + `</div><p style="display:block;color:#475569;font:13px Arial;line-height:1.8">` + footer + `</p></body></html>`
}
