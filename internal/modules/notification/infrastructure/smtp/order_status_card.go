package smtp

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"html"
	"mime"
	"net/url"
	"strings"

	notificationcontract "github.com/dujiao-next/internal/modules/notification/contract"
	settingsmessaging "github.com/dujiao-next/internal/modules/settings/schema/messaging"
	"github.com/dujiao-next/internal/shared/mailbrand"
)

func orderHTMLNote(body string, input notificationcontract.OrderStatusEmailInput) string {
	// Drop only exact rendered field/value lines. Do not discard merchant prose
	// merely because it starts with a familiar label.
	values := []string{strings.TrimSpace(input.OrderNo), strings.TrimSpace(input.RefundReason)}
	// A structured multiline delivery block is already displayed above. Remove
	// only an exact line-for-line copy, with its known label when present.
	for _, block := range []string{input.FulfillmentInfo, input.Instructions} {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
		blockLines := strings.Split(strings.ReplaceAll(block, "\r\n", "\n"), "\n")
		for start := 0; start+len(blockLines) <= len(lines); {
			match := true
			for offset, expected := range blockLines {
				candidate := strings.TrimSpace(lines[start+offset])
				expected = strings.TrimSpace(expected)
				if len(blockLines) == 1 && offset == 0 {
					for _, prefix := range []string{"交付内容", "交付內容", "Delivery content", "使用说明", "使用說明", "Usage instructions"} {
						candidate = strings.TrimPrefix(candidate, prefix+"：")
						candidate = strings.TrimPrefix(candidate, prefix+": ")
						candidate = strings.TrimPrefix(candidate, prefix+":")
					}
				}
				if candidate != expected {
					match = false
					break
				}
			}
			if !match {
				start++
				continue
			}
			lines = append(lines[:start], lines[start+len(blockLines):]...)
			if start > 0 {
				label := strings.TrimSpace(lines[start-1])
				for _, prefix := range []string{"交付内容", "交付內容", "Delivery content", "使用说明", "使用說明", "Usage instructions"} {
					if label == prefix+"：" || label == prefix+":" {
						lines = append(lines[:start-1], lines[start:]...)
						start--
						break
					}
				}
			}
		}
		body = strings.Join(lines, "\n")
	}
	var kept []string
	for _, raw := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			kept = append(kept, "")
			continue
		}
		duplicate := false
		for _, value := range values {
			if value == "" {
				continue
			}
			if line == value {
				duplicate = true
				break
			}
			for _, label := range []string{"订单号", "订单编号", "Order No", "Order number", "交付内容", "交付內容", "Delivery content", "使用说明", "使用說明", "Usage instructions", "退款原因", "Refund reason"} {
				if line == label+"："+value || line == label+":"+value || line == label+": "+value {
					duplicate = true
					break
				}
			}
			if duplicate {
				break
			}
		}
		if duplicate {
			continue
		}
		kept = append(kept, raw)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

// Only order-status mail uses this card. The plain-text body and the existing
// subject still come from the configured order-mail templates.
func buildOrderStatusHTML(input notificationcontract.OrderStatusEmailInput, locale string) string {
	return buildOrderStatusHTMLWithBody(input, locale, "")
}

func buildOrderStatusHTMLWithBody(input notificationcontract.OrderStatusEmailInput, locale, configuredBody string) string {
	return buildOrderStatusHTMLWithBodyAndModules(input, locale, configuredBody, settingsmessaging.DefaultOrderEmailTemplateSetting().Modules)
}

func buildOrderStatusHTMLWithBodyAndModules(input notificationcontract.OrderStatusEmailInput, locale, configuredBody string, modules settingsmessaging.OrderEmailModulesSetting) string {
	zh := !strings.HasPrefix(strings.ToLower(locale), "en")
	label := func(cn, en string) string {
		if zh {
			return cn
		}
		return en
	}
	escape := html.EscapeString
	brand := input.MailBrand
	name := strings.TrimSpace(brand.SiteName)
	if name == "" {
		name = strings.TrimSpace(input.SiteName)
	}
	if name == "" {
		name = label("订单通知", "Order notification")
	}
	logo := strings.TrimSpace(brand.SiteLogo)
	// The storefront SVG may not render in mail clients; use its verified PNG
	// only for the main storefront, never for reseller domains.
	if logo == "https://shop.example.com/xshop-logo-v3.svg" && strings.TrimSpace(brand.SiteURL) == "https://shop.example.com" {
		logo = verifyLogoURL
	}
	if logo == "" {
		logo = strings.TrimSpace(brand.SiteIcon)
	}
	if logo != "" {
		u, err := url.Parse(logo)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			logo = ""
		}
	}
	site := strings.TrimSpace(brand.SiteURL)
	if site == "" {
		site = strings.TrimSpace(input.SiteURL)
	}
	if u, err := url.Parse(site); err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		site = ""
	}
	status := strings.ToLower(strings.TrimSpace(input.Status))
	title := label("订单状态更新", "Order update")
	description := label("订单状态已更新，请查看以下订单信息。", "Your order status has changed. Review the details below.")
	switch status {
	case "paid":
		title = label("支付成功", "Payment received")
		description = label("我们已收到付款，将尽快处理您的订单。", "We received your payment and will process your order soon.")
	case "delivered", "completed":
		title = label("订单已交付", "Order delivered")
		description = label("您的订单已完成交付，请查看交付内容。", "Your order has been delivered. Please check the delivery details.")
	case "refunded", "partially_refunded":
		title = label("退款状态更新", "Refund update")
		description = label("订单退款状态已更新。", "Your refund status has been updated.")
	}
	var b strings.Builder
	b.WriteString(`<!doctype html><html><body style="margin:0;padding:24px 12px;background:#f1f6fc;font-family:Arial,'Microsoft YaHei',sans-serif;color:#162b47"><table role="presentation" cellpadding="0" cellspacing="0" style="width:100%;max-width:620px;margin:0 auto;background:#ffffff;border-radius:18px;overflow:hidden">`)
	if modules.Header {
		b.WriteString(`<tr><td style="padding:34px 28px;background:#1766ba;color:#ffffff;text-align:center">`)
		if logo != "" {
			b.WriteString(`<img alt="" width="54" height="54" src="` + escape(logo) + `" style="display:block;width:54px;height:54px;object-fit:contain;margin:0 auto 12px">`)
		}
		b.WriteString(`<div style="font-size:17px;font-weight:700">` + escape(name) + `</div><div style="margin:14px auto 8px;font-size:12px;letter-spacing:2px">` + escape(label("订单通知", "ORDER UPDATE")) + `</div><h1 style="margin:0;font-size:27px;line-height:1.4;color:#ffffff">` + escape(title) + `</h1><p style="margin:10px 0 0;color:#e5f1ff;font-size:14px">` + escape(description) + `</p></td></tr>`)
	}
	b.WriteString(`<tr><td style="padding:26px 28px"><p style="font-weight:700;margin:0 0 8px">` + escape(label("您好！", "Hello!")) + `</p><p style="margin:0 0 24px;color:#52657d;font-size:14px;line-height:1.7">` + escape(description) + `</p>`)
	section := func(s string) {
		b.WriteString(`<p style="display:inline-block;margin:18px 0 10px;padding:6px 13px;border-radius:16px;background:#e9f3ff;color:#145caa;font-size:13px;font-weight:700">` + escape(s) + `</p>`)
	}
	row := func(k, v string) {
		if strings.TrimSpace(v) == "" {
			return
		}
		b.WriteString(`<tr><td style="padding:10px 12px;color:#59708c;border-bottom:1px solid #dfebf7;font-size:13px;width:110px;vertical-align:top">` + escape(k) + `</td><td style="padding:10px 12px;border-bottom:1px solid #dfebf7;font-size:14px;overflow-wrap:anywhere">` + escape(v) + `</td></tr>`)
	}
	tableStart := `<table role="presentation" cellpadding="0" cellspacing="0" style="width:100%;border-radius:12px;background:#f5f9fe;border:1px solid #dfebf7;border-collapse:separate">`
	if modules.OrderDetails {
		section(label("订单信息", "Order details"))
		b.WriteString(tableStart)
		row(label("订单编号", "Order number"), input.OrderNo)
		row(label("订单状态", "Status"), title)
		if !input.CreatedAt.IsZero() {
			row(label("下单时间", "Created"), input.CreatedAt.Format("2006-01-02 15:04"))
		}
		if input.PaidAt != nil && !input.PaidAt.IsZero() {
			row(label("支付时间", "Paid"), input.PaidAt.Format("2006-01-02 15:04"))
		}
		row(label("订单金额", "Amount"), strings.TrimSpace(input.Amount.String()+" "+input.Currency))
		if status == "refunded" || status == "partially_refunded" {
			row(label("退款金额", "Refund amount"), strings.TrimSpace(input.RefundAmount.String()+" "+input.Currency))
			row(label("退款原因", "Refund reason"), input.RefundReason)
		}
		b.WriteString(`</table>`)
	}
	if modules.Items {
		section(label("商品信息", "Items"))
		if len(input.Items) > 0 {
			b.WriteString(tableStart)
			for _, item := range input.Items {
				row(label("子订单编号", "Child order"), item.ChildOrderNo)
				row(label("商品名称", "Product"), item.Title)
				row(label("商品规格", "Specification"), item.Specification)
				row(label("商品单价", "Unit price"), strings.TrimSpace(item.UnitPrice.String()+" "+input.Currency))
				row(label("购买数量", "Quantity"), fmt.Sprintf("%d", item.Quantity))
			}
			b.WriteString(`</table>`)
		} else {
			b.WriteString(`<div style="padding:13px;background:#f5f9fe;border:1px solid #dfebf7;border-radius:12px;color:#52657d;font-size:13px;line-height:1.7">` + escape(label("商品明细请前往订单详情查看。", "View your order for item details.")) + `</div>`)
		}
	}
	if modules.Delivery && (input.FulfillmentInfo != "" || input.AttachmentName != "") {
		section(label("发货内容", "Delivery"))
		if input.FulfillmentInfo != "" {
			// Child fulfillment is already labeled in the plain-text alternative.
			// In HTML keep each order number outside the copyable payload.
			deliveries := make([]struct{ orderNo, content string }, 0, len(input.ChildDeliveries))
			for _, child := range input.ChildDeliveries {
				if strings.TrimSpace(child.OrderNo) != "" && strings.TrimSpace(child.Content) != "" {
					deliveries = append(deliveries, struct{ orderNo, content string }{child.OrderNo, child.Content})
				}
			}
			if len(deliveries) > 0 {
				for _, child := range deliveries {
					b.WriteString(`<p style="font-size:13px;font-weight:700">` + escape(label("子订单编号：", "Child order: ")) + escape(child.orderNo) + `</p>`)
					b.WriteString(`<div style="padding:16px;background:#17395f;color:#ffffff;border-radius:12px;font-family:monospace;font-size:13px;white-space:pre-wrap;overflow-wrap:anywhere">` + escape(child.content) + `</div>`)
				}
			} else {
				b.WriteString(`<div style="padding:16px;background:#17395f;color:#ffffff;border-radius:12px;font-family:monospace;font-size:13px;white-space:pre-wrap;overflow-wrap:anywhere">` + escape(input.FulfillmentInfo) + `</div>`)
			}
		} else {
			b.WriteString(`<div style="padding:13px;background:#f5f9fe;border-radius:12px">` + escape(label("交付内容较多，请查看邮件附件。", "See the attached delivery file.")) + `</div>`)
		}
	}
	if modules.Instructions && input.Instructions != "" {
		section(label("使用说明", "Instructions"))
		b.WriteString(`<div style="white-space:pre-wrap;overflow-wrap:anywhere;font-size:13px;line-height:1.7">` + escape(input.Instructions) + `</div>`)
	}
	if note := orderHTMLNote(configuredBody, input); modules.Message && note != "" {
		section(label("邮件说明", "Message"))
		b.WriteString(`<div style="white-space:pre-wrap;overflow-wrap:anywhere;font-size:13px;line-height:1.7;color:#465c75">` + escape(note) + `</div>`)
	}
	if modules.Notice {
		section(label("温馨提示", "Please note"))
		b.WriteString(`<div style="padding:14px;border:1px solid #b7d6f6;background:#f5f9fe;border-radius:12px;color:#465c75;font-size:13px;line-height:1.7">`)
		if input.IsGuest {
			b.WriteString(escape(label("游客订单可凭下单邮箱和订单密码查询，请妥善保管。", "Guest orders can be looked up with your email and order password.")) + `<br>`)
		}
		b.WriteString(escape(label("请妥善保管订单及交付信息；如有疑问，请通过网站联系支持。", "Keep your order and delivery details safe. Contact support through the site if needed.")) + `</div>`)
	}
	b.WriteString(`<p style="border-top:1px dashed #b7d6f6;margin:26px 0 0;padding-top:20px;font-size:14px;line-height:1.8">` + escape(label("祝好", "Best regards")) + `<br><strong>` + escape(name) + ` ` + escape(label("团队", "Team")) + `</strong></p>`)
	if site != "" {
		b.WriteString(`<p style="margin:10px 0 0;font-size:12px"><a href="` + escape(site) + `" style="color:#2367a9;text-decoration:none">` + escape(site) + `</a></p>`)
	}
	b.WriteString(`</td></tr>`)
	if modules.Footer {
		b.WriteString(`<tr><td style="padding:22px;text-align:center;background:#17395f;color:#cbdcf0;font-size:12px">` + escape(name) + `<br>` + escape(label("本邮件为订单通知，请勿向任何人透露密码或验证码。", "Order notification. Never share passwords or verification codes.")) + `</td></tr>`)
	}
	b.WriteString(`</table></body></html>`)
	return b.String()
}

func buildOrderStatusEmailMessage(from, to, subject, plain, htmlBody, replyTo, attachName, attachContent string, brand mailbrand.Brand) string {
	logo := strings.TrimSpace(brand.SiteLogo)
	inline := strings.TrimSpace(brand.SiteURL) == "https://shop.example.com" && (logo == verifyLogoURL || logo == "https://shop.example.com/xshop-logo-v3.svg") && strings.Contains(htmlBody, `src="`+verifyLogoURL+`"`)
	if inline {
		htmlBody = strings.Replace(htmlBody, `src="`+verifyLogoURL+`"`, `src="cid:`+verifyLogoCID+`"`, 1)
	}
	var b bytes.Buffer
	writeStandardHeaders(&b, from, to, subject, replyTo)
	mixed := attachName != "" && attachContent != ""
	outer := "=_order_" + hex.EncodeToString(randomBoundaryBytes())
	alt := outer
	if mixed {
		b.WriteString(`Content-Type: multipart/mixed; boundary="` + outer + `"` + "\r\n\r\n--" + outer + "\r\n")
		alt = "=_alt_" + hex.EncodeToString(randomBoundaryBytes())
	}
	b.WriteString(`Content-Type: multipart/alternative; boundary="` + alt + `"` + "\r\n\r\n--" + alt + "\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: base64\r\n\r\n")
	writeMailBase64(&b, []byte(plain))
	b.WriteString("--" + alt + "\r\n")
	if inline {
		related := "=_related_" + hex.EncodeToString(randomBoundaryBytes())
		b.WriteString(`Content-Type: multipart/related; boundary="` + related + `"` + "\r\n\r\n--" + related + "\r\n")
		b.WriteString("Content-Type: text/html; charset=UTF-8\r\nContent-Transfer-Encoding: base64\r\n\r\n")
		writeMailBase64(&b, []byte(htmlBody))
		b.WriteString("--" + related + "\r\nContent-Type: image/png\r\nContent-Transfer-Encoding: base64\r\nContent-ID: <" + verifyLogoCID + ">\r\nContent-Disposition: inline\r\n\r\n")
		writeMailBase64(&b, inlineVerifyLogo)
		b.WriteString("--" + related + "--\r\n")
	} else {
		b.WriteString("Content-Type: text/html; charset=UTF-8\r\nContent-Transfer-Encoding: base64\r\n\r\n")
		writeMailBase64(&b, []byte(htmlBody))
	}
	b.WriteString("--" + alt + "--\r\n")
	if mixed {
		b.WriteString("--" + outer + "\r\nContent-Type: text/plain; charset=UTF-8\r\n")
		b.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=%q\r\n", mime.QEncoding.Encode("UTF-8", attachName)))
		b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
		writeMailBase64(&b, []byte(attachContent))
		b.WriteString("--" + outer + "--\r\n")
	}
	return b.String()
}
