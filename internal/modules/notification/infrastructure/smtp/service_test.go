package smtp

// Synthetic Telegram fixture: ten zero digits, an invalid live account ID.
// Constructed solely to preserve placeholder-email, binding and search test relationships.
import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/i18n"
	notificationcontract "github.com/dujiao-next/internal/modules/notification/contract"
	settingsmessaging "github.com/dujiao-next/internal/modules/settings/schema/messaging"
	"github.com/dujiao-next/internal/shared/mailbrand"
	"github.com/dujiao-next/internal/shared/money"

	"github.com/shopspring/decimal"
)

func TestBuildOrderStatusContent(t *testing.T) {
	tests := []struct {
		name                string
		locale              string
		status              string
		payload             string
		wantSubjectContains []string
		wantBodyContains    []string
	}{
		{
			name:   "paid_zh",
			locale: i18n.LocaleZH,
			status: "paid",
			wantSubjectContains: []string{
				"订单状态更新",
				"已支付",
			},
			wantBodyContains: []string{
				"已收到您的付款",
				"订单号：DJ-PAID",
			},
		},
		{
			name:    "delivered_with_payload_tw",
			locale:  i18n.LocaleTW,
			status:  "delivered",
			payload: "CODE-A\nCODE-B",
			wantSubjectContains: []string{
				"訂單狀態更新",
				"已交付",
			},
			wantBodyContains: []string{
				"交付內容",
				"CODE-A",
			},
		},
		{
			name:    "delivered_no_payload_en",
			locale:  i18n.LocaleEN,
			status:  "delivered",
			payload: "",
			wantSubjectContains: []string{
				"Order status updated",
				"Delivered",
			},
			wantBodyContains: []string{
				"Delivery completed",
				"Order No: DJ-DELIVER",
			},
		},
		{
			name:    "completed_with_payload_zh",
			locale:  i18n.LocaleZH,
			status:  "completed",
			payload: "AUTO-CODE-001",
			wantSubjectContains: []string{
				"订单状态更新",
				"已完成",
			},
			wantBodyContains: []string{
				"交付内容",
				"AUTO-CODE-001",
			},
		},
		{
			name:   "refunded_zh",
			locale: i18n.LocaleZH,
			status: "refunded",
			wantSubjectContains: []string{
				"订单状态更新",
				"已退款",
			},
			wantBodyContains: []string{
				"退款金额：8.80 USD",
				"退款原因：manual refund",
				"示例站点 的网址：https://example.com",
			},
		},
		{
			name:   "partially_refunded_en",
			locale: i18n.LocaleEN,
			status: "partially_refunded",
			wantSubjectContains: []string{
				"Order status updated",
				"Partially refunded",
			},
			wantBodyContains: []string{
				"Refund Amount: 8.80 USD",
				"Reason for refund: manual refund",
				"Example Site's Site URL: https://example.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := notificationcontract.OrderStatusEmailInput{
				OrderNo:         pickOrderNo(tt.status),
				Status:          tt.status,
				Amount:          money.FromDecimal(decimal.NewFromFloat(19.8)),
				RefundAmount:    money.FromDecimal(decimal.NewFromFloat(8.8)),
				RefundReason:    "manual refund",
				Currency:        "USD",
				SiteName:        "Example Site",
				SiteURL:         "https://example.com",
				FulfillmentInfo: tt.payload,
			}
			if tt.locale == i18n.LocaleZH {
				input.SiteName = "示例站点"
			}
			subject, body := buildOrderStatusContent(input, tt.locale)
			for _, expected := range tt.wantSubjectContains {
				if !strings.Contains(subject, expected) {
					t.Fatalf("subject missing %q: %s", expected, subject)
				}
			}
			for _, expected := range tt.wantBodyContains {
				if !strings.Contains(body, expected) {
					t.Fatalf("body missing %q: %s", expected, body)
				}
			}
			if strings.Contains(body, "%!") {
				t.Fatalf("body contains fmt placeholder error marker: %s", body)
			}
		})
	}
}

func pickOrderNo(status string) string {
	switch status {
	case "paid":
		return "DJ-PAID"
	case "refunded":
		return "DJ-REFUND"
	case "partially_refunded":
		return "DJ-PART-REFUND"
	default:
		return "DJ-DELIVER"
	}
}

func TestIsEmailRecipientRejected(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "smtp_550_no_such_recipient",
			err:  errors.New("550 No such recipient here"),
			want: true,
		},
		{
			name: "smtp_user_unknown",
			err:  errors.New("SMTP 5.1.1 user unknown"),
			want: true,
		},
		{
			name: "smtp_550_mailbox_unavailable",
			err:  errors.New("550 mailbox unavailable"),
			want: true,
		},
		{
			name: "network_timeout",
			err:  errors.New("dial tcp timeout"),
			want: false,
		},
		{
			name: "nil_error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isEmailRecipientRejected(tt.err); got != tt.want {
				t.Fatalf("isEmailRecipientRejected() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizeEmailSendError(t *testing.T) {
	rejected := errors.New("550 No such recipient here")
	if got := normalizeEmailSendError(rejected); !errors.Is(got, notificationcontract.ErrEmailRecipientRejected) {
		t.Fatalf("normalizeEmailSendError() expected ErrEmailRecipientRejected, got %v", got)
	}

	networkErr := errors.New("dial tcp timeout")
	if got := normalizeEmailSendError(networkErr); !errors.Is(got, networkErr) {
		t.Fatalf("normalizeEmailSendError() should keep original error, got %v", got)
	}

	if got := normalizeEmailSendError(nil); got != nil {
		t.Fatalf("normalizeEmailSendError(nil) should be nil, got %v", got)
	}
}

func TestSendTextEmailSkipTelegramPlaceholder(t *testing.T) {
	service := &Service{}
	if err := service.sendTextEmail("telegram_0000000000@login.local", "subject", "body"); err != nil {
		t.Fatalf("sendTextEmail() should skip telegram placeholder email, got %v", err)
	}
}

func TestBuildOrderStatusContentFromTemplateIncludesSiteBrand(t *testing.T) {
	tmpl := settingsmessaging.DefaultOrderEmailTemplateSetting()
	tmpl.Templates.Paid.ZHCN.Subject = "订单通知 {{site_name}}"
	tmpl.Templates.Paid.ZHCN.Body = "订单号：{{order_no}}\n站点：{{site_name}} {{site_url}}"

	input := notificationcontract.OrderStatusEmailInput{
		OrderNo:         "DJ-SITE-001",
		Status:          "paid",
		Amount:          money.FromDecimal(decimal.NewFromInt(10)),
		Currency:        "CNY",
		SiteName:        " 示例站点 ",
		SiteURL:         " https://example.com/shop ",
		IsGuest:         false,
		FulfillmentInfo: "",
	}

	subject, body := buildOrderStatusContentFromTemplate(input, i18n.LocaleZH, tmpl)

	if !strings.Contains(subject, "示例站点") {
		t.Fatalf("subject should contain site_name, got: %s", subject)
	}
	if !strings.Contains(body, "站点：示例站点 https://example.com/shop") {
		t.Fatalf("body should contain site_name and site_url, got: %s", body)
	}
}

func TestOrderPaidEmailHasBrandedHTMLAndPlainFallback(t *testing.T) {
	input := notificationcontract.OrderStatusEmailInput{OrderNo: "DJ-SAMPLE", Status: "paid", Amount: money.FromDecimal(decimal.NewFromInt(8)), Currency: "CNY", SiteName: "XSHOP", SiteURL: "https://shop.example.com", MailBrand: mailbrand.Brand{SiteName: "XSHOP", SiteURL: "https://shop.example.com", SiteLogo: "https://shop.example.com/xshop-logo-v3.svg"}}
	subject, plain := buildOrderStatusContent(input, i18n.LocaleZH)
	htmlBody := buildOrderStatusHTML(input, i18n.LocaleZH)
	raw := buildOrderStatusEmailMessage("XSHOP <notify@mail.shop.example.com>", "buyer@example.com", subject, plain, htmlBody, "", "", "", input.MailBrand)
	message, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	typ, params, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
	if err != nil || typ != "multipart/alternative" {
		t.Fatalf("alternative missing: %s %v", typ, err)
	}
	parts := multipart.NewReader(message.Body, params["boundary"])
	first, err := parts.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(first.Header.Get("Content-Type"), "text/plain") {
		t.Fatal("plain fallback must come first")
	}
	second, err := parts.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(second.Header.Get("Content-Type"), "multipart/related") {
		t.Fatal("HTML and inline logo missing")
	}
	_, inner, _ := mime.ParseMediaType(second.Header.Get("Content-Type"))
	related := multipart.NewReader(second, inner["boundary"])
	part, err := related.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := io.ReadAll(part)
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encoded)))
	if err != nil {
		t.Fatal(err)
	}
	html := string(decoded)
	for _, s := range []string{"XSHOP", "DJ-SAMPLE", "订单信息", "商品信息", "温馨提示", "cid:" + verifyLogoCID} {
		if !strings.Contains(html, s) {
			t.Fatalf("HTML missing %s", s)
		}
	}
	if strings.Contains(html, "大橘") {
		t.Fatal("reference brand leaked")
	}
	logo, err := related.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	if logo.Header.Get("Content-ID") != "<"+verifyLogoCID+">" {
		t.Fatal("inline logo missing")
	}
}

func TestOrderPaidHTMLShowsActualItemsAndTimes(t *testing.T) {
	paid := time.Date(2026, 9, 26, 11, 30, 0, 0, time.UTC)
	input := notificationcontract.OrderStatusEmailInput{Status: "paid", CreatedAt: paid.Add(-time.Minute), PaidAt: &paid, Items: []notificationcontract.OrderEmailItem{{Title: "示例商品", Specification: "区域 HK", Quantity: 2, UnitPrice: money.FromDecimal(decimal.NewFromInt(8))}}, Currency: "CNY", MailBrand: mailbrand.Brand{SiteName: "XSHOP", SiteURL: "https://shop.example.com", SiteLogo: "https://shop.example.com/xshop-logo-v3.svg"}}
	html := buildOrderStatusHTML(input, i18n.LocaleZH)
	for _, want := range []string{"示例商品", "区域 HK", "购买数量", "2026-09-26 11:29", "2026-09-26 11:30", verifyLogoURL} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q", want)
		}
	}
}

func TestOrderMailHTMLDoesNotRepeatDeliveryButKeepsMerchantNote(t *testing.T) {
	input := notificationcontract.OrderStatusEmailInput{OrderNo: "DJ-TEMPLATE", Status: "delivered", FulfillmentInfo: "CODE-ONCE", Instructions: "使用前核对账号", MailBrand: mailbrand.Brand{SiteName: "Other Shop"}}
	plain := "订单号：DJ-TEMPLATE\n交付内容：CODE-ONCE\n使用说明：使用前核对账号\n自定义提醒：请自行备份\n订单号：如需帮助请联系客服"
	htmlBody := buildOrderStatusHTMLWithBody(input, i18n.LocaleZH, plain)
	for _, value := range []string{"CODE-ONCE", "使用前核对账号"} {
		if got := strings.Count(htmlBody, value); got != 1 {
			t.Errorf("%q repeated %d times", value, got)
		}
	}
	for _, value := range []string{"自定义提醒：请自行备份", "订单号：如需帮助请联系客服"} {
		if !strings.Contains(htmlBody, value) {
			t.Errorf("merchant note lost: %s", value)
		}
	}
}

func TestOrderMailHTMLDoesNotRepeatMultilineFulfillment(t *testing.T) {
	input := notificationcontract.OrderStatusEmailInput{Status: "delivered", FulfillmentInfo: "[CHILD-A]\nCODE-A\n\n[CHILD-B]\nCODE-B", Instructions: "第一步\n第二步"}
	plain := "交付内容：\n[CHILD-A]\nCODE-A\n\n[CHILD-B]\nCODE-B\n\n使用说明：\n第一步\n第二步\n\n仅此一次：请备份"
	htmlBody := buildOrderStatusHTMLWithBody(input, i18n.LocaleZH, plain)
	for _, value := range []string{"CODE-A", "CODE-B", "第一步", "第二步"} {
		if got := strings.Count(htmlBody, value); got != 1 {
			t.Errorf("%q repeated %d times", value, got)
		}
	}
	if !strings.Contains(htmlBody, "仅此一次：请备份") {
		t.Fatal("merchant note lost")
	}
}

func TestOrderMailHTMLKeepsProseContainingDeliveryValue(t *testing.T) {
	input := notificationcontract.OrderStatusEmailInput{Status: "delivered", FulfillmentInfo: "CODE-A"}
	htmlBody := buildOrderStatusHTMLWithBody(input, i18n.LocaleZH, "交付内容：CODE-A\n如需帮助请引用 CODE-A 进行查询")
	if !strings.Contains(htmlBody, "如需帮助请引用 CODE-A 进行查询") {
		t.Fatal("merchant prose was removed")
	}
	if got := strings.Count(htmlBody, "CODE-A"); got != 2 {
		t.Fatalf("wanted delivery plus prose, got %d", got)
	}
}

func TestOrderMailChildDeliverySeparatesIdentifierFromCopyableContent(t *testing.T) {
	input := notificationcontract.OrderStatusEmailInput{Status: "delivered", FulfillmentInfo: "[CHILD-ONE]\nCODE-ONE\n\n[CHILD-TWO]\nCODE-TWO", ChildDeliveries: []notificationcontract.OrderEmailDelivery{{OrderNo: "CHILD-ONE", Content: "CODE-ONE"}, {OrderNo: "CHILD-TWO", Content: "CODE-TWO"}}, Items: []notificationcontract.OrderEmailItem{{Title: "第一件", ChildOrderNo: "CHILD-ONE", Quantity: 1}, {Title: "第二件", ChildOrderNo: "CHILD-TWO", Quantity: 1}}}
	htmlBody := buildOrderStatusHTML(input, i18n.LocaleZH)
	for _, pair := range [][2]string{{"CHILD-ONE", "CODE-ONE"}, {"CHILD-TWO", "CODE-TWO"}} {
		if !strings.Contains(htmlBody, pair[0]) || strings.Count(htmlBody, pair[1]) != 1 {
			t.Errorf("missing child section or duplicate content for %s", pair[0])
		}
		if strings.Contains(htmlBody, "["+pair[0]+"]\n"+pair[1]) {
			t.Errorf("identifier copied into fulfillment for %s", pair[0])
		}
	}
	if strings.Index(htmlBody, "CHILD-ONE") > strings.Index(htmlBody, "第一件") || strings.Index(htmlBody, "CHILD-TWO") > strings.Index(htmlBody, "第二件") {
		t.Fatal("product is not labeled with its child order")
	}
}

func TestOrderMailHTMLIncludesConfiguredBody(t *testing.T) {
	input := notificationcontract.OrderStatusEmailInput{OrderNo: "DJ-TEMPLATE", Status: "paid", SiteName: "Other Shop", MailBrand: mailbrand.Brand{SiteName: "Other Shop"}}
	tmpl := settingsmessaging.DefaultOrderEmailTemplateSetting()
	tmpl.Templates.Paid.ZHCN.Body = "专属提醒：请核对收件地址 <buyer>"
	_, plain := buildOrderStatusContentFromTemplate(input, i18n.LocaleZH, tmpl)
	rendered := buildOrderStatusHTMLWithBody(input, i18n.LocaleZH, plain)
	if !strings.Contains(rendered, "专属提醒：请核对收件地址 &lt;buyer&gt;") {
		t.Fatal("configured body missing from HTML")
	}
	if strings.Contains(rendered, "<buyer>") {
		t.Fatal("configured body not escaped")
	}
}

func TestOrderMailHTMLRespectsPersistedModuleVisibility(t *testing.T) {
	input := notificationcontract.OrderStatusEmailInput{OrderNo: "ORDER-MARKER", Status: "delivered", FulfillmentInfo: "DELIVERY-MARKER", Instructions: "INSTRUCTIONS-MARKER", Items: []notificationcontract.OrderEmailItem{{Title: "ITEM-MARKER", Quantity: 1}}, MailBrand: mailbrand.Brand{SiteName: "BRAND-MARKER"}}
	modules := settingsmessaging.DefaultOrderEmailTemplateSetting().Modules
	modules.Header, modules.OrderDetails, modules.Items, modules.Delivery = false, false, false, false
	modules.Instructions, modules.Message, modules.Notice, modules.Footer = false, false, false, false
	rendered := buildOrderStatusHTMLWithBodyAndModules(input, i18n.LocaleZH, "MESSAGE-MARKER", modules)
	for _, hidden := range []string{"background:#1766ba", "ORDER-MARKER", "ITEM-MARKER", "DELIVERY-MARKER", "INSTRUCTIONS-MARKER", "MESSAGE-MARKER", "温馨提示", "background:#17395f;color:#cbdcf0"} {
		if strings.Contains(rendered, hidden) {
			t.Errorf("hidden HTML module leaked %q", hidden)
		}
	}
	if !strings.HasPrefix(rendered, "<!doctype html><html><body") || !strings.HasSuffix(rendered, "</body></html>") {
		t.Fatalf("invalid HTML envelope: %s", rendered)
	}
}

func TestCustomOrderHTMLReplacesModulesEscapesVariablesAndKeepsPlainText(t *testing.T) {
	input := notificationcontract.OrderStatusEmailInput{OrderNo: `<img src=x onerror=alert(1)>`, Status: "paid", SiteName: "Shop"}
	tmpl := settingsmessaging.DefaultOrderEmailTemplateSetting()
	tmpl.Templates.Paid.ENUS.Body = "Plain order {{order_no}}"
	tmpl.Templates.Paid.ENUS.CustomHTMLEnabled = true
	tmpl.Templates.Paid.ENUS.CustomHTML = `<html><body><main id="custom">Order {{order_no}}</main></body></html>`
	subject, plain := buildOrderStatusContentFromTemplate(input, i18n.LocaleEN, tmpl)
	htmlBody := buildOrderStatusHTMLFromTemplate(input, i18n.LocaleEN, plain, tmpl)
	if !strings.Contains(htmlBody, `id="custom"`) || strings.Contains(htmlBody, "Order details") || strings.Contains(htmlBody, input.OrderNo) || !strings.Contains(htmlBody, "&lt;img src=x onerror=alert(1)&gt;") {
		t.Fatalf("custom HTML selection or escaping failed: %s", htmlBody)
	}
	raw := buildOrderStatusEmailMessage("sender@example.com", "buyer@example.com", subject, plain, htmlBody, "", "", "", input.MailBrand)
	message, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	_, params, _ := mime.ParseMediaType(message.Header.Get("Content-Type"))
	part, err := multipart.NewReader(message.Body, params["boundary"]).NextPart()
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(part)
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || !strings.Contains(string(decoded), input.OrderNo) {
		t.Fatalf("plain-text alternative was altered: %v %s", err, decoded)
	}
}

func TestDeliveredAttachmentSelectsContentSceneForPlainAndCustomHTML(t *testing.T) {
	for _, input := range []notificationcontract.OrderStatusEmailInput{
		{Status: "delivered", AttachmentName: "delivery.txt"},
		{Status: "completed", AttachmentContent: "SECRET"},
	} {
		tmpl := settingsmessaging.DefaultOrderEmailTemplateSetting()
		tmpl.Templates.Delivered.ENUS.Subject = "without-content"
		tmpl.Templates.DeliveredWithContent.ENUS.Subject = "with-content"
		tmpl.Templates.Delivered.ENUS.CustomHTMLEnabled = true
		tmpl.Templates.Delivered.ENUS.CustomHTML = `<p>without-content-html</p>`
		tmpl.Templates.DeliveredWithContent.ENUS.CustomHTMLEnabled = true
		tmpl.Templates.DeliveredWithContent.ENUS.CustomHTML = `<p>with-content-html</p>`
		subject, plain := buildOrderStatusContentFromTemplate(input, i18n.LocaleEN, tmpl)
		htmlBody := buildOrderStatusHTMLFromTemplate(input, i18n.LocaleEN, plain, tmpl)
		if subject != "with-content" || !strings.Contains(htmlBody, "with-content-html") {
			t.Errorf("attachment-backed delivery selected wrong scene: subject=%q html=%q", subject, htmlBody)
		}
	}
}

func TestCustomOrderHTMLUsesLocalizedStatusLabel(t *testing.T) {
	input := notificationcontract.OrderStatusEmailInput{Status: "paid"}
	tmpl := settingsmessaging.DefaultOrderEmailTemplateSetting()
	tmpl.Templates.Paid.ZHCN.CustomHTMLEnabled = true
	tmpl.Templates.Paid.ZHCN.CustomHTML = `<p>{{status}}</p>`
	htmlBody := buildOrderStatusHTMLFromTemplate(input, i18n.LocaleZH, "", tmpl)
	if !strings.Contains(htmlBody, "已支付") || strings.Contains(htmlBody, ">paid<") {
		t.Fatalf("custom HTML status was not localized: %s", htmlBody)
	}
}

func TestOrderMailWithConfiguredTemplatePreservesSubjectAndBody(t *testing.T) {
	input := notificationcontract.OrderStatusEmailInput{OrderNo: "DJ-TEMPLATE", Status: "paid", SiteName: "Other Shop", MailBrand: mailbrand.Brand{SiteName: "Other Shop"}}
	tmpl := settingsmessaging.DefaultOrderEmailTemplateSetting()
	tmpl.Templates.Paid.ZHCN.Subject = "自定义主题 {{order_no}}"
	tmpl.Templates.Paid.ZHCN.Body = "自定义正文 {{order_no}}"
	subject, plain := buildOrderStatusContentFromTemplate(input, i18n.LocaleZH, tmpl)
	raw := buildOrderStatusEmailMessage("sender@example.com", "buyer@example.com", subject, plain, buildOrderStatusHTML(input, i18n.LocaleZH), "", "", "", input.MailBrand)
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	decodedSubject, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if err != nil || decodedSubject != "自定义主题 DJ-TEMPLATE" {
		t.Fatalf("configured subject lost: %s %v", decodedSubject, err)
	}
	typ, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || typ != "multipart/alternative" {
		t.Fatalf("missing alternative: %v", err)
	}
	part, err := multipart.NewReader(msg.Body, params["boundary"]).NextPart()
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(part)
	if err != nil {
		t.Fatal(err)
	}
	text, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || !strings.Contains(string(text), "自定义正文 DJ-TEMPLATE") {
		t.Fatalf("configured plain text lost: %v", err)
	}
}

func TestOrderMailAttachmentRetainsContentAndEscapesHTML(t *testing.T) {
	input := notificationcontract.OrderStatusEmailInput{OrderNo: "<script>alert(1)</script>", Status: "delivered", SiteName: "Other Shop", SiteURL: "https://other.example", FulfillmentInfo: "<secret>&", MailBrand: mailbrand.Brand{SiteName: "Other Shop", SiteLogo: "https://other.example/logo.png"}}
	html := buildOrderStatusHTML(input, i18n.LocaleZH)
	if strings.Contains(html, "<script>") || strings.Contains(html, "<secret>") || !strings.Contains(html, "&lt;secret&gt;&amp;") {
		t.Fatalf("HTML escaping failed: %s", html)
	}
	if strings.Contains(html, verifyLogoCID) || strings.Contains(html, "XSHOP") {
		t.Fatal("other tenant leaked main brand")
	}
	raw := buildOrderStatusEmailMessage("sender@example.com", "buyer@example.com", "subject", "plain body", html, "", "delivery.txt", "CODE-123", input.MailBrand)
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	typ, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || typ != "multipart/mixed" {
		t.Fatalf("attachment wrapper missing: %s %v", typ, err)
	}
	parts := multipart.NewReader(msg.Body, params["boundary"])
	first, err := parts.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(first.Header.Get("Content-Type"), "multipart/alternative") {
		t.Fatal("alternative body missing")
	}
	attachment, err := parts.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(attachment)
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || string(decoded) != "CODE-123" {
		t.Fatalf("attachment lost: %v", err)
	}
}

func TestBuildVerifyCodeContentUsesResolvedWhiteLabelBrand(t *testing.T) {
	subject, body := buildVerifyCodeContent("123456", "register", i18n.LocaleZH, mailbrand.Brand{
		SiteName: "白标商店",
		SiteURL:  "https://shop.example.test",
	})

	if !strings.Contains(subject, "白标商店") {
		t.Fatalf("subject should contain white-label site name, got: %s", subject)
	}
	if !strings.Contains(body, "白标商店") || !strings.Contains(body, "https://shop.example.test") {
		t.Fatalf("body should contain white-label brand only, got: %s", body)
	}
	if strings.Contains(body, "Dujiao-Next") || strings.Contains(body, "main.example.test") {
		t.Fatalf("body leaked main-site brand: %s", body)
	}
}

func TestVerificationEmailHasBrandedHTMLAndPlainFallback(t *testing.T) {
	brand := mailbrand.Brand{SiteName: "XSHOP", SiteURL: "https://shop.example.com", ReplyTo: "support@shop.example.com"}
	subject, plain := buildVerifyCodeContent("654321", "register", i18n.LocaleZH, brand)
	html := buildVerifyCodeHTML("654321", "register", i18n.LocaleZH, brand, 10)
	raw := buildAlternativeEmailMessage("XSHOP <notify@mail.shop.example.com>", "buyer@example.com", subject, plain, html, brand.ReplyTo)
	message, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if message.Header.Get("Reply-To") != brand.ReplyTo {
		t.Fatalf("reply-to lost: %q", message.Header.Get("Reply-To"))
	}
	mediaType, params, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/alternative" {
		t.Fatalf("expected multipart/alternative, got %q: %v", mediaType, err)
	}
	parts := multipart.NewReader(message.Body, params["boundary"])
	for i, want := range []string{"text/plain", "text/html"} {
		part, err := parts.NextPart()
		if err != nil {
			t.Fatal(err)
		}
		typ, _, err := mime.ParseMediaType(part.Header.Get("Content-Type"))
		if err != nil || typ != want {
			t.Fatalf("part %d: %q, %v", i, typ, err)
		}
		body, err := io.ReadAll(part)
		if err != nil {
			t.Fatal(err)
		}
		if part.Header.Get("Content-Transfer-Encoding") != "base64" {
			t.Fatal("parts must be encoded for SMTP safety")
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(body)))
		if err != nil {
			t.Fatal(err)
		}
		for _, text := range []string{"654321", "XSHOP"} {
			if !strings.Contains(string(decoded), text) {
				t.Fatalf("part %d missing %q", i, text)
			}
		}
		if i == 1 && (!strings.Contains(string(decoded), "10 分钟") || !strings.Contains(string(decoded), "max-width:560px")) {
			t.Fatal("HTML missing expiry or responsive width")
		}
	}
	if _, err := parts.NextPart(); err != io.EOF {
		t.Fatalf("unexpected extra MIME part: %v", err)
	}
}

func TestVerifyCodeHTMLEscapesUntrustedBrandAndCode(t *testing.T) {
	html := buildVerifyCodeHTML("<123&>", "reset", i18n.LocaleZH, mailbrand.Brand{SiteName: "<script>alert(1)</script>", SiteURL: "javascript:alert(1)"}, 15)
	if strings.Contains(html, "<script>") || strings.Contains(html, "href=\"javascript:") || strings.Contains(html, "<123&>") {
		t.Fatal("unsafe HTML interpolated")
	}
	for _, want := range []string{"&lt;123&amp;&gt;", "重置密码", "15 分钟"} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q", want)
		}
	}
}

func TestVerifyCodeHTMLReferenceInspiredLayoutAndSafeSiteLink(t *testing.T) {
	html := buildVerifyCodeHTML("654321", "register", i18n.LocaleZH, mailbrand.Brand{SiteName: "XSHOP", SiteURL: "https://shop.example.com"}, 10)
	for _, want := range []string{"注册验证", "654321", "10 分钟", "role=\"presentation\"", "linear-gradient", "border-top:1px dashed", "https://shop.example.com", "我们不会索要您的密码", "XSHOP 团队"} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(html, "大橘") || strings.Contains(html, "145192") || strings.Contains(html, "@xiiyioozzz") {
		t.Fatal("reference brand or live code copied")
	}
	unsafe := buildVerifyCodeHTML("123456", "register", i18n.LocaleZH, mailbrand.Brand{SiteName: "<店铺>", SiteURL: "javascript:alert(1)"}, 10)
	if strings.Contains(unsafe, "href=\"javascript:") || strings.Contains(unsafe, "<店铺>") {
		t.Fatal("unsafe brand markup or URL")
	}
	if !strings.Contains(unsafe, "&lt;店铺&gt;") {
		t.Fatal("brand name should be escaped")
	}
}

func TestVerificationMailDoesNotEmbedMainLogoForOtherTenant(t *testing.T) {
	brand := mailbrand.Brand{SiteName: "其他商店", SiteURL: "https://other.example", SiteIcon: "https://other.example/icon.png"}
	htmlBody := buildVerifyCodeHTML("123456", "register", i18n.LocaleZH, brand, 10)
	raw := buildVerificationEmailMessage("notify@mail.shop.example.com", "buyer@example.test", "验证码", "正文", htmlBody, "")
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	mt, _, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/alternative" {
		t.Fatalf("bad MIME type: %s: %v", mt, err)
	}
	if strings.Contains(raw, "Content-ID: <"+verifyLogoCID+">") {
		t.Fatal("main logo leaked into another tenant's mail")
	}
}

func TestVerificationMailEmbedsMainLogoInline(t *testing.T) {
	brand := mailbrand.Brand{SiteName: "XSHOP", SiteURL: "https://shop.example.com", SiteLogo: "https://shop.example.com/xshop-logo-v3.svg"}
	htmlBody := buildVerifyCodeHTML("123456", "register", i18n.LocaleZH, brand, 10)
	raw := buildVerificationEmailMessage("notify@mail.shop.example.com", "buyer@example.test", "验证码", "正文", htmlBody, "")
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	mt, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/alternative" {
		t.Fatalf("outer MIME: %s: %v", mt, err)
	}
	outer := multipart.NewReader(msg.Body, params["boundary"])
	plain, err := outer.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	if plain.Header.Get("Content-Type") != "text/plain; charset=UTF-8" {
		t.Fatal("plain fallback missing")
	}
	related, err := outer.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	mt, params, err = mime.ParseMediaType(related.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/related" {
		t.Fatalf("HTML and inline logo must be related: %s: %v", mt, err)
	}
	inner := multipart.NewReader(related, params["boundary"])
	bodyPart, err := inner.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	encodedHTML, err := io.ReadAll(bodyPart)
	if err != nil {
		t.Fatal(err)
	}
	decodedHTML, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encodedHTML)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(decodedHTML), `src="cid:xshop-logo@mail.shop.example.com"`) {
		t.Fatal("HTML logo must use embedded CID")
	}
	logoPart, err := inner.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	if logoPart.Header.Get("Content-ID") != "<xshop-logo@mail.shop.example.com>" || logoPart.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("bad logo MIME headers: %v", logoPart.Header)
	}
	encodedPNG, err := io.ReadAll(logoPart)
	if err != nil {
		t.Fatal(err)
	}
	png, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encodedPNG)))
	if err != nil {
		t.Fatal(err)
	}
	if len(png) < 100 || !bytes.Equal(png[:8], []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatal("inline logo is not a PNG")
	}
	if _, err := inner.NextPart(); err != io.EOF {
		t.Fatalf("unexpected related part: %v", err)
	}
}

func TestVerifyCodeHTMLUsesTransparentBrandLogoWithoutOpaqueIcon(t *testing.T) {
	card := buildVerifyCodeHTML("123456", "register", i18n.LocaleZH, mailbrand.Brand{SiteName: "XSHOP", SiteURL: "https://shop.example.com", SiteLogo: "https://shop.example.com/xshop-logo-v3.svg", SiteIcon: "https://shop.example.com/uploads/library-import/brand/apple-touch-icon-v4.png"}, 10)
	if !strings.Contains(card, `src="https://shop.example.com/xshop-logo-v3-email.png"`) {
		t.Fatal("email must use transparent raster of the site logo rather than opaque square favicon")
	}
	if strings.Contains(card, `src="https://shop.example.com/uploads/library-import/brand/apple-touch-icon-v4.png"`) {
		t.Fatal("opaque icon should not be used in the round email logo")
	}
}

func TestVerifyCodeHTMLSignatureKeepsCompactHierarchy(t *testing.T) {
	card := buildVerifyCodeHTML("123456", "register", i18n.LocaleZH, mailbrand.Brand{SiteName: "XSHOP", SiteURL: "https://shop.example.com"}, 10)
	for _, want := range []string{`margin:20px 0 17px;border-top:1px dashed`, `margin:0 0 4px;color:#64748b`, `margin:0;color:#1e293b;font-size:14px`, `margin:10px 0 0;color:#64748b`, `font-weight:500`, `href="https://shop.example.com"`} {
		if !strings.Contains(card, want) {
			t.Errorf("signature missing %q", want)
		}
	}
	if strings.Contains(card, `margin:14px 0 0;color:#64748b;font-size:13px`) {
		t.Fatal("site address must not have a large separate gap")
	}
}

func TestVerifyCodeHTMLMatchesReferenceStructureWithOwnBrand(t *testing.T) {
	card := buildVerifyCodeHTML("123456", "register", i18n.LocaleZH, mailbrand.Brand{SiteName: "XSHOP", SiteURL: "https://shop.example.com"}, 10)
	for _, want := range []string{`background:#f4f7fc`, `text-align:center`, `border-radius:999px`, `border-top:1px dashed`, `XSHOP 团队`, `站点地址`, `href="https://shop.example.com"`, `background:#172554`, `©`, `我们不会索要您的密码`} {
		if !strings.Contains(card, want) {
			t.Errorf("reference-inspired structure missing %q", want)
		}
	}
	if strings.Count(card, "请忽略此邮件") != 1 || strings.Contains(card, "大橘AI") {
		t.Fatal("duplicate guidance or copied third-party identity")
	}
}

func TestVerifyCodeHTMLUsesConfiguredSiteLogo(t *testing.T) {
	brand := mailbrand.Brand{SiteName: "XSHOP", SiteURL: "https://shop.example.com", SiteLogo: "https://shop.example.com/xshop-logo-v3.svg", SiteIcon: "https://shop.example.com/uploads/library-import/brand/apple-touch-icon-v4.png"}
	html := buildVerifyCodeHTML("123456", "register", i18n.LocaleZH, brand, 10)
	if !strings.Contains(html, `src="https://shop.example.com/xshop-logo-v3-email.png"`) || !strings.Contains(html, `alt="XSHOP"`) {
		t.Fatal("transparent raster of configured site logo not shown")
	}
	if strings.Contains(html, `>✦</div>`) {
		t.Fatal("placeholder icon still shown")
	}
	unsafe := buildVerifyCodeHTML("123456", "register", i18n.LocaleZH, mailbrand.Brand{SiteName: "<Brand>", SiteLogo: "javascript:alert(1)"}, 10)
	if strings.Contains(unsafe, `<img`) || strings.Contains(unsafe, `src="javascript:`) {
		t.Fatal("unsafe image URL")
	}
	if !strings.Contains(unsafe, `&lt;Brand&gt;`) {
		t.Fatal("missing safe text fallback")
	}
}

func TestSMTPEnvelopeUsesAddressNotDisplayName(t *testing.T) {
	from := buildFromAddress("notify@mail.shop.example.com", "XSHOP")
	if got := smtpEnvelopeSender(from); got != "notify@mail.shop.example.com" {
		t.Fatalf("SMTP MAIL FROM must be bare address, got %q", got)
	}
	message := buildEmailMessage(from, "buyer@example.com", "subject", "body")
	parsed, err := mail.ReadMessage(strings.NewReader(message))
	if err != nil {
		t.Fatal(err)
	}
	address, err := mail.ParseAddress(parsed.Header.Get("From"))
	if err != nil || address.Name != "XSHOP" || address.Address != "notify@mail.shop.example.com" {
		t.Fatalf("display name must remain only in From header: %v %v", address, err)
	}
}

func TestBuildEmailMessageSupportsPerMessageFromNameAndReplyTo(t *testing.T) {
	from := buildFromAddress("mailer@service.example", "白标商店")
	parsedFrom, err := mail.ParseAddress(from)
	if err != nil {
		t.Fatalf("parse from address failed: %v", err)
	}
	if parsedFrom.Name != "白标商店" {
		t.Fatalf("from display name was encoded incorrectly: %q", parsedFrom.Name)
	}
	message := buildEmailMessage(from, "buyer@example.com", "subject", "body", "support@shop.example.test")

	if !strings.Contains(message, "From: ") || !strings.Contains(message, "<mailer@service.example>") {
		t.Fatalf("message should retain the authenticated shared sender address: %s", message)
	}
	if !strings.Contains(message, "Reply-To: support@shop.example.test\r\n") {
		t.Fatalf("message should contain tenant reply-to: %s", message)
	}
}

func TestPickSMTPAuthMechanism(t *testing.T) {
	tests := []struct {
		name       string
		advertised string
		want       string
	}{
		{name: "prefer_login_when_both_exist", advertised: "PLAIN LOGIN XOAUTH2", want: smtpAuthMechanismLogin},
		{name: "plain_only", advertised: "PLAIN", want: smtpAuthMechanismPlain},
		{name: "case_and_space", advertised: "  login   xoauth2 ", want: smtpAuthMechanismLogin},
		{name: "unsupported", advertised: "CRAM-MD5", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pickSMTPAuthMechanism(tt.advertised); got != tt.want {
				t.Fatalf("pickSMTPAuthMechanism() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoginAuth(t *testing.T) {
	auth := newLoginAuth("user@example.com", "pwd", "smtp.office365.com")
	login, ok := auth.(*loginAuth)
	if !ok {
		t.Fatal("newLoginAuth() should return *loginAuth")
	}

	if _, _, err := login.Start(&smtp.ServerInfo{Name: "smtp.office365.com", TLS: true}); err != nil {
		t.Fatalf("Start() returned unexpected error: %v", err)
	}

	resp, err := login.Next([]byte("Username:"), true)
	if err != nil || string(resp) != "user@example.com" {
		t.Fatalf("Next(username) = %q, %v", string(resp), err)
	}

	resp, err = login.Next([]byte("Password:"), true)
	if err != nil || string(resp) != "pwd" {
		t.Fatalf("Next(password) = %q, %v", string(resp), err)
	}

	resp, err = login.Next(nil, false)
	if err != nil || resp != nil {
		t.Fatalf("Next(done) = %v, %v; want nil, nil", resp, err)
	}
}

func TestLoginAuthRejectsInsecureRemoteConnection(t *testing.T) {
	auth := newLoginAuth("user@example.com", "pwd", "smtp.office365.com")
	login := auth.(*loginAuth)

	if _, _, err := login.Start(&smtp.ServerInfo{Name: "smtp.office365.com", TLS: false}); err == nil {
		t.Fatal("Start() should reject insecure remote connection")
	}
}

func TestSMTPServerAuthExtensions(t *testing.T) {
	host := strings.TrimSpace(os.Getenv("TEST_SMTP_HOST"))
	if host == "" {
		t.Skip("set TEST_SMTP_HOST to check SMTP server AUTH extensions")
	}

	port := 587
	if rawPort := strings.TrimSpace(os.Getenv("TEST_SMTP_PORT")); rawPort != "" {
		if parsed, err := strconv.Atoi(rawPort); err == nil && parsed > 0 {
			port = parsed
		}
	}

	useStartTLS := true
	if rawStartTLS := strings.TrimSpace(os.Getenv("TEST_SMTP_USE_STARTTLS")); rawStartTLS != "" {
		if parsed, err := strconv.ParseBool(rawStartTLS); err == nil {
			useStartTLS = parsed
		}
	}

	insecureSkipVerify := false
	if rawInsecure := strings.TrimSpace(os.Getenv("TEST_SMTP_INSECURE_SKIP_VERIFY")); rawInsecure != "" {
		if parsed, err := strconv.ParseBool(rawInsecure); err == nil {
			insecureSkipVerify = parsed
		}
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	client, closeFn, err := newSMTPTestClient(addr, host, port, useStartTLS, insecureSkipVerify)
	if err != nil {
		t.Fatalf("connect smtp server failed: %v", err)
	}
	t.Cleanup(closeFn)

	ok, authLine := client.Extension("AUTH")
	if !ok {
		t.Logf("smtp server %s does not advertise AUTH", addr)
		return
	}

	mechanisms := strings.Fields(strings.ToUpper(strings.TrimSpace(authLine)))
	if len(mechanisms) == 0 {
		t.Fatalf("smtp AUTH extension is empty: %q", authLine)
	}

	t.Logf("smtp server %s supports AUTH: %s", addr, strings.Join(mechanisms, ", "))
}

func newSMTPTestClient(addr, host string, port int, useStartTLS, insecureSkipVerify bool) (*smtp.Client, func(), error) {
	if port == 465 {
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host, InsecureSkipVerify: insecureSkipVerify})
		if err != nil {
			return nil, nil, err
		}
		client, err := smtp.NewClient(conn, host)
		if err != nil {
			_ = conn.Close()
			return nil, nil, err
		}
		return client, func() {
			if err := client.Quit(); err != nil {
				_ = client.Close()
			}
		}, nil
	}

	client, err := smtp.Dial(addr)
	if err != nil {
		return nil, nil, err
	}
	if useStartTLS {
		if err := client.StartTLS(&tls.Config{ServerName: host, InsecureSkipVerify: insecureSkipVerify}); err != nil {
			_ = client.Close()
			return nil, nil, err
		}
	}
	return client, func() {
		if err := client.Quit(); err != nil {
			_ = client.Close()
		}
	}, nil
}

// 仅是针对 Office365 SMTP 服务器的集成测试，确保 Service 能够成功发送邮件并正确处理服务器的响应。
// 需要在环境变量中设置 TEST_OFFICE365_SEND=1 和有效的 TEST_OFFICE365_PASSWORD 来运行此测试。
func TestEmailServiceSendOffice365Integration(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_OFFICE365_SEND")) != "1" {
		t.Skip("set TEST_OFFICE365_SEND=1 to send a real email via Office365")
	}

	username := strings.TrimSpace(os.Getenv("TEST_OFFICE365_USERNAME"))
	if username == "" {
		t.Skip("set TEST_OFFICE365_USERNAME")
	}

	password := strings.TrimSpace(os.Getenv("TEST_OFFICE365_PASSWORD"))
	if password == "" {
		t.Skip("set TEST_OFFICE365_PASSWORD")
	}

	to := strings.TrimSpace(os.Getenv("TEST_OFFICE365_TO"))
	if to == "" {
		t.Skip("set TEST_OFFICE365_TO")
	}

	from := strings.TrimSpace(os.Getenv("TEST_OFFICE365_FROM"))
	if from == "" {
		from = username
	}

	svc := New(&config.EmailConfig{
		Enabled:  true,
		Host:     "smtp.office365.com",
		Port:     587,
		Username: username,
		Password: password,
		From:     from,
		FromName: "Dujiao Next",
		UseTLS:   true,
		UseSSL:   false,
	})

	if err := svc.SendCustomEmail(to, "Office365 SMTP integration test", "This is a real email sent by Service integration test."); err != nil {
		t.Fatalf("SendCustomEmail() failed: %v", err)
	}

	t.Logf("email sent via smtp.office365.com:587 to %s", to)
}

type fakeSMTPSessionCloser struct {
	quitErr  error
	closeErr error
	quitCnt  int
	closeCnt int
}

func (f *fakeSMTPSessionCloser) Quit() error {
	f.quitCnt++
	return f.quitErr
}

func (f *fakeSMTPSessionCloser) Close() error {
	f.closeCnt++
	return f.closeErr
}

func TestQuitSMTPClient(t *testing.T) {
	t.Run("quit_success_no_close", func(t *testing.T) {
		fake := &fakeSMTPSessionCloser{}
		if err := quitSMTPClient(fake, "smtp.office365.com", "smtp.office365.com:587"); err != nil {
			t.Fatalf("quitSMTPClient() returned error: %v", err)
		}
		if fake.quitCnt != 1 {
			t.Fatalf("Quit() called %d times, want 1", fake.quitCnt)
		}
		if fake.closeCnt != 0 {
			t.Fatalf("Close() called %d times, want 0", fake.closeCnt)
		}
	})

	t.Run("quit_failed_fallback_close", func(t *testing.T) {
		quitErr := errors.New("421 service not available")
		fake := &fakeSMTPSessionCloser{quitErr: quitErr}
		if err := quitSMTPClient(fake, "smtp.office365.com", "smtp.office365.com:587"); !errors.Is(err, quitErr) {
			t.Fatalf("quitSMTPClient() error = %v, want %v", err, quitErr)
		}
		if fake.quitCnt != 1 {
			t.Fatalf("Quit() called %d times, want 1", fake.quitCnt)
		}
		if fake.closeCnt != 1 {
			t.Fatalf("Close() called %d times, want 1", fake.closeCnt)
		}
	})
}

func TestIsSMTPAlreadyClosedError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "closed_network_connection", err: errors.New("use of closed network connection"), want: true},
		{name: "connection_is_closed", err: errors.New("connection is closed"), want: true},
		{name: "other_error", err: errors.New("dial tcp timeout"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSMTPAlreadyClosedError(tt.err); got != tt.want {
				t.Fatalf("isSMTPAlreadyClosedError() = %v, want %v", got, tt.want)
			}
		})
	}
}
