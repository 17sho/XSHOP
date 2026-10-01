package settingsmessaging

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestVerificationScenesSparsePatchPreservesLegacyRegistration(t *testing.T) {
	current := DefaultRegistrationEmailTemplateSetting()
	current.Templates.ENUS.Subject = "Merchant registration"
	var patch RegistrationEmailTemplateSettingPatch
	if err := json.Unmarshal([]byte(`{"scenes":{"reset":{"en-US":{"subject":"Merchant reset"}}}}`), &patch); err != nil {
		t.Fatalf("valid scene patch rejected: %v", err)
	}
	next, err := ApplyRegistrationEmailTemplateSettingPatch(current, patch)
	if err != nil {
		t.Fatal(err)
	}
	encoded := EncodeRegistrationEmailTemplateSetting(next)
	data, _ := json.Marshal(encoded)
	var raw map[string]any
	_ = json.Unmarshal(data, &raw)
	scenes := raw["scenes"].(map[string]any)
	if len(scenes) != 4 || scenes["reset"].(map[string]any)["en-US"].(map[string]any)["subject"] != "Merchant reset" || next.Templates != current.Templates {
		t.Fatalf("scene patch lost legacy/siblings: %s", data)
	}
	var legacy RegistrationEmailTemplateSettingPatch
	_ = json.Unmarshal([]byte(`{"templates":{"zh-CN":{"subject":"Legacy update"}}}`), &legacy)
	updated, err := ApplyRegistrationEmailTemplateSettingPatch(next, legacy)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(EncodeRegistrationEmailTemplateSetting(updated)["scenes"], encoded["scenes"]) {
		t.Fatal("legacy client erased scenes")
	}
}

func TestRegistrationTemplateRejectsUnsafePatches(t *testing.T) {
	tests := []string{
		`{"subject":"Bad\r\nBcc: victim@example.test"}`, `{"subject":"Safe\n"}`, `{"subject":"{{unknown}}"}`,
		`{"subject":"{{ code }}"}`, `{"body":"{{code}}"}`, `{"body":"{{expire_minutes}}"}`,
		`{"body":"{{code}} {{expire_minutes}} {{other}}"}`, `{"custom_html_enabled":true}`,
		`{"custom_html_enabled":true,"custom_html":"<p>{{code}}</p>"}`,
		`{"custom_html":"<script>alert(1)</script>"}`, `{"custom_html":"<p onclick=\"bad()\">{{code}}</p>"}`,
		`{"custom_html":"<a href=\"{{site_url}}\">link</a>"}`,
		`{"custom_html_enabled":true,"custom_html":"<!-- {{code}} {{expire_minutes}} -->"}`,
		`{"custom_html_enabled":true,"custom_html":"<title>{{code}} {{expire_minutes}}</title>"}`,
		`{"custom_html":"<img src=\"javascript:alert(1)\">"}`,
		`{"custom_html":"<div style=\"background:url(https://evil.test)\">bad</div>"}`,
	}
	large, _ := json.Marshal(map[string]any{"custom_html": strings.Repeat("x", 200*1024+1)})
	tests = append(tests, string(large))
	for _, fields := range tests {
		t.Run(fields[:min(len(fields), 70)], func(t *testing.T) {
			for _, scene := range []string{"register", "reset", "telegram_bind", "change_email_old", "change_email_new"} {
				input := `{"templates":{"en-US":` + fields + `}}`
				if scene != "register" {
					input = `{"scenes":{"` + scene + `":{"en-US":` + fields + `}}}`
				}
				var patch RegistrationEmailTemplateSettingPatch
				if err := json.Unmarshal([]byte(input), &patch); err != nil {
					t.Fatal(err)
				}
				if _, err := ApplyRegistrationEmailTemplateSettingPatch(DefaultRegistrationEmailTemplateSetting(), patch); err == nil {
					t.Fatalf("accepted unsafe %s fields: %.90s", scene, fields)
				}
			}
		})
	}
	var valid RegistrationEmailTemplateSettingPatch
	if err := json.Unmarshal([]byte(`{"templates":{"en-US":{"custom_html_enabled":true,"custom_html":"<p>{{code}} — {{expire_minutes}} {{site_name}} {{site_url}}</p>"}}}`), &valid); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyRegistrationEmailTemplateSettingPatch(DefaultRegistrationEmailTemplateSetting(), valid); err != nil {
		t.Fatal(err)
	}
}

func TestRegistrationTemplateRejectsMalformedJSON(t *testing.T) {
	for _, input := range []string{
		`null`, `[]`, `{"templates":null}`, `{"templates":[]}`, `{"templates":{"en-US":null}}`,
		`{"templates":{"fr-FR":{}}}`, `{"templates":{"en-US":{"subject":null}}}`,
		`{"templates":{"en-US":{"subject":123}}}`, `{"templates":{"en-US":{"custom_html_enabled":"true"}}}`,
		`{"templates":{"en-US":{"other":"x"}}}`, `{"modules":{}}`, `{"Templates":{}}`,
		`{"templates":{"en-US":{"subject":"one","subject":"two"}}}`,
	} {
		var patch RegistrationEmailTemplateSettingPatch
		if err := json.Unmarshal([]byte(input), &patch); err == nil {
			t.Errorf("accepted malformed JSON: %s", input)
		}
	}
}

func TestRegistrationEnabledHTMLRequiresBodyTextNodes(t *testing.T) {
	for _, markup := range []string{
		`<template>{{code}} {{expire_minutes}}</template>`,
		`<noscript>{{code}} {{expire_minutes}}</noscript>`,
		`<head><noscript>{{code}} {{expire_minutes}}</noscript></head>`,
		`<p>&#123;&#123;code&#125;&#125; &#123;&#123;expire_minutes&#125;&#125;</p>`,
		`<template>{{code}} {{expire_minutes}}</template><p>&#123;&#123;code&#125;&#125; &#123;&#123;expire_minutes&#125;&#125;</p>`,
	} {
		cfg := DefaultRegistrationEmailTemplateSetting()
		cfg.Templates.ENUS.CustomHTML = markup
		cfg.Templates.ENUS.CustomHTMLEnabled = true
		if err := ValidateRegistrationEmailTemplateSetting(cfg); err == nil {
			t.Errorf("accepted non-body/inert code text: %s", markup)
		}
	}
}

func TestRegistrationTemplateDefaultsAndSparsePatch(t *testing.T) {
	current := DefaultRegistrationEmailTemplateSetting()
	for _, lt := range []OrderEmailLocalizedTemplate{current.Templates.ZHCN, current.Templates.ZHTW, current.Templates.ENUS} {
		if lt.Subject == "" || !strings.Contains(lt.Body, "{{code}}") || !strings.Contains(lt.Body, "{{expire_minutes}}") || lt.CustomHTML != "" || lt.CustomHTMLEnabled {
			t.Fatalf("invalid default: %+v", lt)
		}
	}
	var patch RegistrationEmailTemplateSettingPatch
	if err := json.Unmarshal([]byte(`{"templates":{"en-US":{"subject":"Welcome {{site_name}}"}}}`), &patch); err != nil {
		t.Fatal(err)
	}
	next, err := ApplyRegistrationEmailTemplateSettingPatch(current, patch)
	if err != nil {
		t.Fatal(err)
	}
	if next.Templates.ENUS.Subject != "Welcome {{site_name}}" || next.Templates.ENUS.Body != current.Templates.ENUS.Body || next.Templates.ZHCN != current.Templates.ZHCN || next.Templates.ZHTW != current.Templates.ZHTW {
		t.Fatalf("sparse patch lost values: %+v", next)
	}
	decoded, err := DecodeRegistrationEmailTemplateSetting(EncodeRegistrationEmailTemplateSetting(next))
	if err != nil || !reflect.DeepEqual(decoded, next) {
		t.Fatalf("roundtrip: %+v %v", decoded, err)
	}
	absent, err := DecodeRegistrationEmailTemplateSetting(nil)
	if err != nil || !reflect.DeepEqual(absent, current) {
		t.Fatalf("absent defaults: %+v %v", absent, err)
	}
}

func TestVerificationScenesRejectMalformedJSON(t *testing.T) {
	for _, input := range []string{
		`{"scenes":null}`, `{"scenes":[]}`, `{"scenes":{"register":{}}}`, `{"scenes":{"other":{}}}`,
		`{"scenes":{"reset":null}}`, `{"scenes":{"reset":[]}}`, `{"scenes":{"reset":{"fr-FR":{}}}}`,
		`{"scenes":{"reset":{"en-US":null}}}`, `{"scenes":{"reset":{"en-US":{"subject":null}}}}`,
		`{"scenes":{"reset":{"en-US":{"body":23}}}}`, `{"scenes":{"reset":{"en-US":{"custom_html_enabled":"true"}}}}`,
		`{"scenes":{"reset":{"en-US":{"other":"x"}}}}`, `{"scenes":{"reset":{},"reset":{}}}`,
		`{"scenes":{"reset":{"en-US":{},"en-US":{}}}}`, `{"scenes":{"reset":{"en-US":{"subject":"one","subject":"two"}}}}`,
		`{"scenes":{},"scenes":{}}`, `{"Scenes":{}}`,
	} {
		var patch RegistrationEmailTemplateSettingPatch
		if err := json.Unmarshal([]byte(input), &patch); err == nil {
			t.Errorf("accepted malformed scenes: %s", input)
		}
	}
}

func TestVerificationScenesPatchDoesNotMutateCurrentOrReplaceStoredLegacy(t *testing.T) {
	current := DefaultRegistrationEmailTemplateSetting()
	current.Templates.ENUS.Subject = "Merchant registration"
	current.Templates.ENUS.Body = "Merchant {{code}} valid {{expire_minutes}}"
	current.Templates.ENUS.CustomHTML = "<p>Merchant {{code}} {{expire_minutes}}</p>"
	current.Templates.ENUS.CustomHTMLEnabled = true
	legacy := EncodeRegistrationEmailTemplateSetting(current)
	delete(legacy, "scenes")
	decoded, err := DecodeRegistrationEmailTemplateSetting(legacy)
	if err != nil || decoded.Templates != current.Templates || len(decoded.Scenes) != 4 {
		t.Fatal("legacy merchant registration overwritten")
	}
	before, _ := json.Marshal(current)
	for _, key := range []string{"reset", "telegram_bind", "change_email_old", "change_email_new"} {
		var patch RegistrationEmailTemplateSettingPatch
		_ = json.Unmarshal([]byte(`{"scenes":{"`+key+`":{"zh-TW":{"subject":"Changed"}}}}`), &patch)
		next, err := ApplyRegistrationEmailTemplateSettingPatch(current, patch)
		if err != nil {
			t.Fatal(err)
		}
		if next.Templates != current.Templates {
			t.Fatal("registration overwritten")
		}
		for other, scene := range current.Scenes {
			expected := scene
			if other == key {
				expected.ZHTW.Subject = "Changed"
			}
			if next.Scenes[other] != expected {
				t.Fatalf("sibling/locale lost: %s", other)
			}
		}
		after, _ := json.Marshal(current)
		if string(after) != string(before) {
			t.Fatal("patch mutated input map")
		}
		_ = json.Unmarshal([]byte(`{"scenes":{"`+key+`":{"zh-TW":{"body":"bad"}}}}`), &patch)
		if _, err := ApplyRegistrationEmailTemplateSettingPatch(current, patch); err == nil {
			t.Fatal("invalid independent scene accepted")
		}
		after, _ = json.Marshal(current)
		if string(after) != string(before) {
			t.Fatal("invalid patch mutated input map")
		}
	}
}
