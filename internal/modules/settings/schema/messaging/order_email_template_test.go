package settingsmessaging

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dujiao-next/internal/shared/jsonmap"
)

func TestOrderEmailTemplateSettingDoesNotExposeCanceledScene(t *testing.T) {
	encoded := EncodeOrderEmailTemplateSetting(DefaultOrderEmailTemplateSetting())
	templates, ok := encoded["templates"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected encoded templates map, got %T", encoded["templates"])
	}
	if _, exists := templates["canceled"]; exists {
		t.Fatalf("canceled email scene must not be exposed")
	}

	// 旧版本保存的 canceled 字段应被安全忽略，并在下一次序列化时自然清除。
	templates["canceled"] = map[string]interface{}{
		"zh-CN": map[string]interface{}{"subject": "legacy", "body": "legacy"},
	}
	roundTrip := EncodeOrderEmailTemplateSetting(DecodeOrderEmailTemplateSetting(encoded, DefaultOrderEmailTemplateSetting()))
	roundTripTemplates, ok := roundTrip["templates"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected round-trip templates map, got %T", roundTrip["templates"])
	}
	if _, exists := roundTripTemplates["canceled"]; exists {
		t.Fatalf("legacy canceled email scene must be discarded")
	}
}

func TestOrderEmailModulesDefaultTrueAndUseExactJSONContract(t *testing.T) {
	setting := DefaultOrderEmailTemplateSetting()
	modules := setting.Modules
	if !modules.Header || !modules.OrderDetails || !modules.Items || !modules.Delivery || !modules.Instructions || !modules.Message || !modules.Notice || !modules.Footer {
		t.Fatalf("all modules must default visible: %+v", modules)
	}
	raw, err := json.Marshal(EncodeOrderEmailTemplateSetting(setting)["modules"])
	if err != nil {
		t.Fatal(err)
	}
	want := `{"delivery":true,"footer":true,"header":true,"instructions":true,"items":true,"message":true,"notice":true,"order_details":true}`
	if string(raw) != want {
		t.Fatalf("unexpected modules contract: %s", raw)
	}
}

func TestOrderEmailModulesDecodeAndPatchUseSafeFallbacks(t *testing.T) {
	fallback := DefaultOrderEmailTemplateSetting()
	decoded := DecodeOrderEmailTemplateSetting(jsonmap.JSON{"modules": map[string]interface{}{
		"header": false, "order_details": "invalid", "items": nil,
	}}, fallback)
	if decoded.Modules.Header || !decoded.Modules.OrderDetails || !decoded.Modules.Items || !decoded.Modules.Footer {
		t.Fatalf("unsafe modules decode: %+v", decoded.Modules)
	}
	historical := DecodeOrderEmailTemplateSetting(jsonmap.JSON{"templates": map[string]interface{}{}}, fallback)
	if !historical.Modules.Header || !historical.Modules.Footer {
		t.Fatalf("historical modules must default true: %+v", historical.Modules)
	}
	value := false
	next, err := ApplyOrderEmailTemplateSettingPatch(fallback, OrderEmailTemplateSettingPatch{Modules: &OrderEmailModulesPatch{Delivery: &value, Message: &value}})
	if err != nil {
		t.Fatal(err)
	}
	if next.Modules.Delivery || next.Modules.Message || !next.Modules.Header || !next.Modules.Footer {
		t.Fatalf("module patch changed wrong values: %+v", next.Modules)
	}
}

func TestOrderEmailCustomHTMLDefaultsAndRoundTrips(t *testing.T) {
	setting := DefaultOrderEmailTemplateSetting()
	if setting.Templates.Paid.ENUS.CustomHTMLEnabled || setting.Templates.Paid.ENUS.CustomHTML != "" {
		t.Fatalf("custom HTML must default disabled and empty: %+v", setting.Templates.Paid.ENUS)
	}
	setting.Templates.Paid.ENUS.CustomHTMLEnabled = true
	setting.Templates.Paid.ENUS.CustomHTML = `<main>Order {{order_no}}</main>`
	decoded := DecodeOrderEmailTemplateSetting(EncodeOrderEmailTemplateSetting(setting), DefaultOrderEmailTemplateSetting())
	if !decoded.Templates.Paid.ENUS.CustomHTMLEnabled || decoded.Templates.Paid.ENUS.CustomHTML != setting.Templates.Paid.ENUS.CustomHTML {
		t.Fatalf("custom HTML did not round trip: %+v", decoded.Templates.Paid.ENUS)
	}
}

func TestOrderEmailCustomHTMLValidationRejectsDangerousOrOversizedMarkup(t *testing.T) {
	tests := []string{
		`<script>alert(1)</script>`, `<iframe src="https://evil.test"></iframe>`, `<object></object>`, `<embed>`, `<form></form>`,
		`<meta http-equiv="refresh" content="0;url=https://evil.test">`, `<base href="https://evil.test">`, `<svg><foreignObject>x</foreignObject></svg>`,
		`<math><mtext>x</mtext></math>`, `<img src=x onerror="alert(1)">`, `<a href="javascript:alert(1)">x</a>`,
		`<a href="vbscript:msgbox(1)">x</a>`, `<a href="data:text/html;base64,WA==">x</a>`, `<img src="data:image/svg+xml,<svg onload=alert(1)>">`,
		`<img src="file:///etc/passwd">`, `<img src="blob:https://safe.test/id">`, `<a href="custom:payload">x</a>`, `<img src="//evil.test/x.png">`,
		`<style>body{background:url(https://evil.test/x)}</style>`, `<div style="background:url(https://evil.test/x)">x</div>`,
		`<div style="background:u\72l(https://evil.test/x)">x</div>`, `<div style="@import 'https://evil.test/x'">x</div>`,
		`<div style="width:expression(alert(1))">x</div>`, `<div style="behavior:url(x.htc)">x</div>`, `<div style="-moz-binding:url(x.xml)">x</div>`,
		`<link rel="stylesheet" href="https://evil.test/style.css">`, `<div style="background-image:image-set('https://evil.test/a.png' 1x)">x</div>`,
		`<div style="background-image:-webkit-image-set('https://evil.test/a.png' 1x)">x</div>`,
		strings.Repeat("x", 200*1024+1),
	}
	for _, customHTML := range tests {
		setting := DefaultOrderEmailTemplateSetting()
		setting.Templates.Paid.ENUS.CustomHTMLEnabled = true
		setting.Templates.Paid.ENUS.CustomHTML = customHTML
		if err := ValidateOrderEmailTemplateSetting(setting); err == nil {
			t.Errorf("dangerous custom HTML accepted: %.80q", customHTML)
		}
	}
}

func TestOrderEmailCustomHTMLValidationAllowsOnlyTextNodePlaceholders(t *testing.T) {
	unsafe := []string{
		`<a href="{{fulfillment_info}}">delivery</a>`,
		`<div style="{{instructions}}">instructions</div>`,
		`<{{site_name}}>content</{{site_name}}>`,
		`<div {{instructions}}>content</div>`,
		`<!-- {{order_no}} -->`,
		`<textarea>{{instructions}}</textarea>`,
	}
	for _, customHTML := range unsafe {
		setting := DefaultOrderEmailTemplateSetting()
		setting.Templates.Paid.ENUS.CustomHTMLEnabled = true
		setting.Templates.Paid.ENUS.CustomHTML = customHTML
		if err := ValidateOrderEmailTemplateSetting(setting); err == nil {
			t.Errorf("placeholder outside a normal text node accepted: %q", customHTML)
		}
	}

	setting := DefaultOrderEmailTemplateSetting()
	setting.Templates.Paid.ENUS.CustomHTMLEnabled = true
	setting.Templates.Paid.ENUS.CustomHTML = `<main style="color:#123; padding: 8px"><p>Order {{order_no}}</p><a href="https://example.com/order">View</a><img src="cid:brand-logo"><a href="/help">Help</a><a href="mailto:support@example.com">Email</a></main>`
	if err := ValidateOrderEmailTemplateSetting(setting); err != nil {
		t.Fatalf("safe HTML with text-node placeholder rejected: %v", err)
	}
}

func TestOrderEmailCustomHTMLPatchRejectsUnsafeMarkup(t *testing.T) {
	enabled := true
	unsafe := `<img src="javascript:alert(1)">`
	_, err := ApplyOrderEmailTemplateSettingPatch(DefaultOrderEmailTemplateSetting(), OrderEmailTemplateSettingPatch{Templates: &OrderEmailTemplatesPatch{
		Paid: &OrderEmailSceneTemplatePatch{ENUS: &OrderEmailLocalizedTemplatePatch{CustomHTMLEnabled: &enabled, CustomHTML: &unsafe}},
	}})
	if err != ErrOrderEmailTemplateConfigInvalid {
		t.Fatalf("expected config invalid, got %v", err)
	}
}
