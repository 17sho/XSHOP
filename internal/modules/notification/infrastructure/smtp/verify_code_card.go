package smtp

import (
	"fmt"
	"html"
	"net/url"
	"strings"
	"time"
)

type verifyCodeCard struct {
	Lang, Name, Label, Title, Intro, Code, Expiry, Warning, Ignore, Signature, SiteURL, SiteLogo, SiteIcon string
}

// verifiedSiteLink accepts only an absolute web URL without embedded credentials.
func verifiedSiteLink(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil {
		return ""
	}
	return html.EscapeString(u.String())
}

// renderVerifyCodeCard uses nested tables and inline styles for mobile mail clients.
func renderVerifyCodeCard(c verifyCodeCard) string {
	logo := ""
	// Mail clients often reject SVG. Use the transparent raster of this site's
	// configured SVG logo rather than its opaque square touch icon.
	if site, err := url.Parse(c.SiteURL); err == nil && site.Scheme == "https" && site.Hostname() == "shop.example.com" {
		if configured, err := url.Parse(c.SiteLogo); err == nil && configured.Scheme == "https" && configured.Host == site.Host && configured.Path == "/xshop-logo-v3.svg" {
			logo = verifiedSiteLink(site.Scheme + "://" + site.Host + "/xshop-logo-v3-email.png")
		}
	}
	for _, candidate := range []string{c.SiteLogo, c.SiteIcon} {
		if logo != "" {
			break
		}
		path := strings.ToLower(strings.SplitN(candidate, "?", 2)[0])
		if strings.HasPrefix(strings.ToLower(candidate), "https://") && !strings.HasSuffix(path, ".svg") {
			logo = verifiedSiteLink(candidate)
			if logo != "" {
				break
			}
		}
	}
	mark := ""
	if logo != "" {
		mark = fmt.Sprintf(`<img src="%s" alt="%s" width="46" height="46" style="display:block;width:46px;height:46px;border:0;object-fit:contain">`, logo, c.Name)
	}
	greeting, siteLabel := "祝好", "站点地址"
	security := "本邮件由系统自动发送。我们不会索要您的密码、支付信息或验证码，请谨防冒充客服的消息。"
	slogan := "安心选购 · 即时交付"
	if c.Lang == "zh-TW" {
		greeting, siteLabel = "祝好", "站點地址"
		security = "本郵件由系統自動發送。我們不會索取您的密碼、付款資訊或驗證碼，請留意冒充客服的訊息。"
		slogan = "安心選購 · 即時交付"
	} else if c.Lang == "en-US" {
		greeting, siteLabel = "Best regards,", "Store"
		security = "This is an automated email. We will never ask for your password, payment details, or verification code."
		slogan = "Shop with confidence"
	}
	link := verifiedSiteLink(c.SiteURL)
	siteRow := ""
	if link != "" {
		siteRow = fmt.Sprintf(`<p style="margin:10px 0 0;color:#64748b;font-size:12px;line-height:1.6">%s：<a href="%s" style="color:#2563eb;text-decoration:none;font-weight:500">%s</a></p>`, siteLabel, link, link)
	}
	return fmt.Sprintf(`<!doctype html><html lang="%s"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%s</title></head><body style="margin:0;padding:24px 12px;background:#f4f7fc;font-family:Arial,'Microsoft YaHei',sans-serif;color:#18243b"><table role="presentation" cellpadding="0" cellspacing="0" style="width:100%%;max-width:560px;margin:0 auto;background:#ffffff;border-collapse:separate;border-spacing:0;border-radius:20px;overflow:hidden;box-shadow:0 12px 30px #dce5f4"><tr><td style="padding:32px 24px 38px;text-align:center;background:#2563eb;background-image:linear-gradient(125deg,#3b82f6,#1d4ed8);color:#ffffff"><div style="width:60px;height:60px;border-radius:999px;background:#ffffff;margin:0 auto 17px;text-align:center"><table role="presentation" cellpadding="0" cellspacing="0" style="width:100%%;height:60px"><tr><td align="center" valign="middle">%s</td></tr></table></div><div style="color:#ffffff;font-size:20px;font-weight:700;line-height:1.5">%s</div><div style="display:inline-block;margin:15px 0 22px;padding:5px 14px;border-radius:999px;border:1px solid #bfdbfe;color:#ffffff;font-size:12px;letter-spacing:1px">%s</div><h1 style="margin:0;color:#ffffff;font-size:27px;line-height:1.35">%s</h1><p style="margin:13px 0 0;color:#dbeafe;font-size:14px;line-height:1.7">%s</p></td></tr><tr><td style="padding:31px 27px 34px;background:#ffffff"><table role="presentation" cellpadding="0" cellspacing="0" style="width:100%%;background:#172554;border-collapse:separate;border-spacing:0;border-radius:14px"><tr><td style="padding:24px 12px;text-align:center"><div style="color:#bfdbfe;font-size:12px">%s</div><div style="margin:11px 0;color:#dbeafe;font-size:34px;font-weight:700;letter-spacing:6px;word-break:break-all">%s</div><div style="color:#bfdbfe;font-size:12px;line-height:1.7">%s · %s</div></td></tr></table><p style="margin:27px 0 0;color:#475569;font-size:13px;line-height:1.8">%s</p><div style="margin:20px 0 17px;border-top:1px dashed #cbd5e1"></div><p style="margin:0 0 4px;color:#64748b;font-size:12px;line-height:1.5">%s</p><p style="margin:0;color:#1e293b;font-size:14px;font-weight:600;line-height:1.5">%s %s</p>%s</td></tr><tr><td style="padding:29px 25px 25px;text-align:center;background:#172554;color:#cbd5e1"><div style="color:#93c5fd;font-size:15px;font-weight:700">%s</div><div style="margin:8px 0 17px;color:#93c5fd;font-size:13px">%s</div><p style="margin:0;color:#cbd5e1;font-size:11px;line-height:1.8">%s</p><p style="margin:15px 0 0;color:#94a3b8;font-size:11px">© %d %s</p></td></tr></table></body></html>`, c.Lang, c.Title, mark, c.Name, c.Label, c.Title, c.Intro, c.Title, c.Code, c.Expiry, c.Warning, c.Ignore, greeting, c.Name, c.Signature, siteRow, c.Name, slogan, security, time.Now().Year(), c.Name)
}
