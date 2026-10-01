package settingsmessaging

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/dujiao-next/internal/shared/jsonmap"
	"golang.org/x/net/html"
)

// RegistrationEmailTemplateSetting is intentionally separate from order templates.
type RegistrationEmailTemplateSetting struct {
	Templates OrderEmailSceneTemplate            `json:"templates"`
	Scenes    map[string]OrderEmailSceneTemplate `json:"scenes,omitempty"`
}

type RegistrationEmailTemplateSettingPatch struct {
	Templates *OrderEmailSceneTemplatePatch            `json:"templates"`
	Scenes    map[string]*OrderEmailSceneTemplatePatch `json:"scenes,omitempty"`
}

// UnmarshalJSON rejects unknown keys, nulls, duplicates and wrong types rather
// than silently turning a malformed update into a successful no-op.
func (p *RegistrationEmailTemplateSettingPatch) UnmarshalJSON(data []byte) error {
	d := json.NewDecoder(bytes.NewReader(data))
	var object func(kind string) error
	object = func(kind string) error {
		token, err := d.Token()
		if err != nil || token != json.Delim('{') {
			return ErrRegistrationEmailTemplateConfigInvalid
		}
		seen := map[string]bool{}
		for d.More() {
			token, err := d.Token()
			if err != nil {
				return ErrRegistrationEmailTemplateConfigInvalid
			}
			key, ok := token.(string)
			if !ok || seen[key] {
				return ErrRegistrationEmailTemplateConfigInvalid
			}
			seen[key] = true
			child := ""
			switch kind {
			case "root":
				if key == "templates" {
					child = "locales"
				} else if key == "scenes" {
					child = "scenes"
				} else {
					return ErrRegistrationEmailTemplateConfigInvalid
				}
			case "scenes":
				if !verificationSceneKnown(key) {
					return ErrRegistrationEmailTemplateConfigInvalid
				}
				child = "locales"
			case "locales":
				if key != "zh-CN" && key != "zh-TW" && key != "en-US" {
					return ErrRegistrationEmailTemplateConfigInvalid
				}
				child = "fields"
			}
			if child != "" {
				if err := object(child); err != nil {
					return err
				}
				continue
			}
			value, err := d.Token()
			if err != nil {
				return ErrRegistrationEmailTemplateConfigInvalid
			}
			switch key {
			case "subject", "body", "custom_html":
				if _, ok := value.(string); !ok {
					return ErrRegistrationEmailTemplateConfigInvalid
				}
			case "custom_html_enabled":
				if _, ok := value.(bool); !ok {
					return ErrRegistrationEmailTemplateConfigInvalid
				}
			default:
				return ErrRegistrationEmailTemplateConfigInvalid
			}
		}
		token, err = d.Token()
		if err != nil || token != json.Delim('}') {
			return ErrRegistrationEmailTemplateConfigInvalid
		}
		return nil
	}
	if err := object("root"); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrRegistrationEmailTemplateConfigInvalid
	}
	type plainPatch RegistrationEmailTemplateSettingPatch
	var next plainPatch
	if err := json.Unmarshal(data, &next); err != nil {
		return ErrRegistrationEmailTemplateConfigInvalid
	}
	*p = RegistrationEmailTemplateSettingPatch(next)
	return nil
}

func DefaultRegistrationEmailTemplateSetting() RegistrationEmailTemplateSetting {
	return RegistrationEmailTemplateSetting{Scenes: DefaultVerificationEmailScenes(), Templates: OrderEmailSceneTemplate{
		ZHCN: OrderEmailLocalizedTemplate{Subject: "{{site_name}} - 注册验证码", Body: "您的验证码是：{{code}}\n\n请使用此验证码完成账号注册，{{expire_minutes}} 分钟内有效。\n\n站点：{{site_name}}\n网址：{{site_url}}"},
		ZHTW: OrderEmailLocalizedTemplate{Subject: "{{site_name}} - 註冊驗證碼", Body: "您的驗證碼是：{{code}}\n\n請使用此驗證碼完成帳號註冊，{{expire_minutes}} 分鐘內有效。\n\n站點：{{site_name}}\n網址：{{site_url}}"},
		ENUS: OrderEmailLocalizedTemplate{Subject: "{{site_name}} - Registration Code", Body: "Your verification code is: {{code}}\n\nUse this code to finish creating your account. This code expires in {{expire_minutes}} minutes.\n\nSite: {{site_name}}\nURL: {{site_url}}"},
	}}
}

var ErrRegistrationEmailTemplateConfigInvalid = errors.New("registration email template config invalid")

func verificationSceneKnown(key string) bool {
	switch key {
	case "reset", "telegram_bind", "change_email_old", "change_email_new":
		return true
	}
	return false
}

// Add defaults only for absent scenes, never replacing merchant registration.
func DefaultVerificationEmailScenes() map[string]OrderEmailSceneTemplate {
	result := map[string]OrderEmailSceneTemplate{}
	for purpose, labels := range map[string][3]string{
		"reset":            {"重置密码", "重置密碼", "Password reset"},
		"telegram_bind":    {"绑定 Telegram", "綁定 Telegram", "Telegram binding"},
		"change_email_old": {"更换邮箱（原邮箱验证）", "更換郵箱（原郵箱驗證）", "Change email (verify current email)"},
		"change_email_new": {"更换邮箱（新邮箱验证）", "更換郵箱（新郵箱驗證）", "Change email (verify new email)"},
	} {
		result[purpose] = OrderEmailSceneTemplate{
			ZHCN: OrderEmailLocalizedTemplate{Subject: "{{site_name}} - " + labels[0] + "验证码", Body: "您的验证码是：{{code}}\n\n请使用此验证码完成" + labels[0] + "，{{expire_minutes}} 分钟内有效。\n\n站点：{{site_name}}\n网址：{{site_url}}"},
			ZHTW: OrderEmailLocalizedTemplate{Subject: "{{site_name}} - " + labels[1] + "驗證碼", Body: "您的驗證碼是：{{code}}\n\n請使用此驗證碼完成" + labels[1] + "，{{expire_minutes}} 分鐘內有效。\n\n站點：{{site_name}}\n網址：{{site_url}}"},
			ENUS: OrderEmailLocalizedTemplate{Subject: "{{site_name}} - " + labels[2] + " Code", Body: "Your verification code is: {{code}}\n\nUse this code for " + labels[2] + ". This code expires in {{expire_minutes}} minutes.\n\nSite: {{site_name}}\nURL: {{site_url}}"},
		}
	}
	return result
}

func ValidateRegistrationEmailTemplateSetting(setting RegistrationEmailTemplateSetting) error {
	if err := validateVerificationScene(setting.Templates); err != nil {
		return err
	}
	for key, scene := range setting.Scenes {
		if !verificationSceneKnown(key) {
			return ErrRegistrationEmailTemplateConfigInvalid
		}
		if err := validateVerificationScene(scene); err != nil {
			return err
		}
	}
	return nil
}

func validateVerificationScene(scene OrderEmailSceneTemplate) error {
	for _, lt := range []OrderEmailLocalizedTemplate{scene.ZHCN, scene.ZHTW, scene.ENUS} {
		if strings.TrimSpace(lt.Subject) == "" || strings.ContainsAny(lt.Subject, "\r\n") || !registrationVariablesValid(lt.Subject) || !registrationVariablesValid(lt.Body) || !registrationVariablesValid(lt.CustomHTML) {
			return ErrRegistrationEmailTemplateConfigInvalid
		}
		required := func(text string) bool {
			return strings.Contains(text, "{{code}}") && strings.Contains(text, "{{expire_minutes}}")
		}
		if !required(lt.Body) || !IsSafeEmailCustomHTML(lt.CustomHTML) || lt.CustomHTMLEnabled && !registrationHTMLHasRequiredText(lt.CustomHTML) {
			return ErrRegistrationEmailTemplateConfigInvalid
		}
	}
	return nil
}

// Check the parsed body as well as the hardened tokenizer validator: inert
// template/noscript/head text must not satisfy the required code/expiry fields.
func registrationHTMLHasRequiredText(markup string) bool {
	// Substitute source tokens before parsing, just as runtime does. Encoded
	// braces look like tokens after parsing but would never receive a real code.
	marker := "registration-template-required-"
	decoded := html.UnescapeString(markup)
	for strings.Contains(decoded, marker) {
		// Bound collision probing even for deliberately adversarial admin input.
		if len(marker) >= 128 {
			return false
		}
		marker += "x"
	}
	codeMarker, expiryMarker := marker+"code", marker+"expiry"
	markup = strings.NewReplacer("{{code}}", codeMarker, "{{expire_minutes}}", expiryMarker).Replace(markup)
	doc, err := html.Parse(strings.NewReader(markup))
	if err != nil {
		return false
	}
	code, expiry := false, false
	var walk func(*html.Node, bool)
	walk = func(n *html.Node, inBody bool) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "head", "template", "noscript":
				return
			case "body":
				inBody = true
			}
		}
		if inBody && n.Type == html.TextNode {
			code = code || strings.Contains(n.Data, codeMarker)
			expiry = expiry || strings.Contains(n.Data, expiryMarker)
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child, inBody)
		}
	}
	walk(doc, false)
	return code && expiry
}

func registrationVariablesValid(text string) bool {
	for _, key := range []string{"code", "expire_minutes", "site_name", "site_url"} {
		text = strings.ReplaceAll(text, "{{"+key+"}}", "")
	}
	return !strings.Contains(text, "{{") && !strings.Contains(text, "}}")
}

func ApplyRegistrationEmailTemplateSettingPatch(current RegistrationEmailTemplateSetting, patch RegistrationEmailTemplateSettingPatch) (RegistrationEmailTemplateSetting, error) {
	next := current
	next.Scenes = DefaultVerificationEmailScenes()
	for key, scene := range current.Scenes {
		next.Scenes[key] = scene
	}
	apply := func(dst *OrderEmailSceneTemplate, src *OrderEmailSceneTemplatePatch) {
		if src == nil {
			return
		}
		localized := func(dst *OrderEmailLocalizedTemplate, src *OrderEmailLocalizedTemplatePatch) {
			if src == nil {
				return
			}
			if src.Subject != nil {
				dst.Subject = *src.Subject
			}
			if src.Body != nil {
				dst.Body = *src.Body
			}
			if src.CustomHTML != nil {
				dst.CustomHTML = *src.CustomHTML
			}
			if src.CustomHTMLEnabled != nil {
				dst.CustomHTMLEnabled = *src.CustomHTMLEnabled
			}
		}
		localized(&dst.ZHCN, src.ZHCN)
		localized(&dst.ZHTW, src.ZHTW)
		localized(&dst.ENUS, src.ENUS)
	}
	apply(&next.Templates, patch.Templates)
	for key, src := range patch.Scenes {
		if !verificationSceneKnown(key) || src == nil {
			return RegistrationEmailTemplateSetting{}, ErrRegistrationEmailTemplateConfigInvalid
		}
		scene := next.Scenes[key]
		apply(&scene, src)
		next.Scenes[key] = scene
	}
	// Validate before trimming, including CR/LF and the raw HTML size limit.
	if err := ValidateRegistrationEmailTemplateSetting(next); err != nil {
		return RegistrationEmailTemplateSetting{}, err
	}
	next.Templates = normalizeOrderEmailSceneTemplate(next.Templates)
	for key, scene := range next.Scenes {
		next.Scenes[key] = normalizeOrderEmailSceneTemplate(scene)
	}
	return next, nil
}

func EncodeRegistrationEmailTemplateSetting(setting RegistrationEmailTemplateSetting) jsonmap.JSON {
	result := jsonmap.JSON{"templates": orderEmailSceneTemplateToMap(setting.Templates)}
	if len(setting.Scenes) > 0 {
		scenes := map[string]interface{}{}
		for key, scene := range setting.Scenes {
			scenes[key] = orderEmailSceneTemplateToMap(scene)
		}
		result["scenes"] = scenes
	}
	return result
}

func DecodeRegistrationEmailTemplateSetting(raw jsonmap.JSON) (RegistrationEmailTemplateSetting, error) {
	fallback := DefaultRegistrationEmailTemplateSetting()
	if raw == nil {
		return fallback, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return fallback, err
	}
	var patch RegistrationEmailTemplateSettingPatch
	if err := json.Unmarshal(data, &patch); err != nil {
		return fallback, err
	}
	return ApplyRegistrationEmailTemplateSettingPatch(fallback, patch)
}
