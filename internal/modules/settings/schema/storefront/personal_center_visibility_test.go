package settingsstorefront

import (
	"reflect"
	"testing"

	"github.com/dujiao-next/internal/shared/jsonmap"
)

func TestNormalizePersonalCenterVisibilityDefaultsAndCanonicalKeys(t *testing.T) {
	got := NormalizePersonalCenterVisibilityJSON(jsonmap.JSON{
		"orders":  false,
		"profile": "invalid",
		"unknown": false,
	})
	want := jsonmap.JSON{
		"overview":   true,
		"orders":     false,
		"wallet":     true,
		"gift_cards": true,
		"security":   true,
		"profile":    true,
		"affiliate":  true,
		"reseller":   true,
		"api":        true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized visibility = %#v, want %#v", got, want)
	}
}

func TestResolvePersonalCenterVisibilityAppliesSparseOverrides(t *testing.T) {
	global := NormalizePersonalCenterVisibilityJSON(jsonmap.JSON{"wallet": false, "orders": false})
	got := ResolvePersonalCenterVisibility(global, jsonmap.JSON{"orders": true})
	want := jsonmap.JSON{
		"overview":   true,
		"orders":     true,
		"wallet":     false,
		"gift_cards": true,
		"security":   true,
		"profile":    true,
		"affiliate":  true,
		"reseller":   true,
		"api":        true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("effective visibility = %#v, want %#v", got, want)
	}
}

func TestNormalizePersonalCenterVisibilityOverridesRejectsUnknownAndNonBoolean(t *testing.T) {
	if _, err := NormalizePersonalCenterVisibilityOverrides(jsonmap.JSON{"unknown": true}); err == nil {
		t.Fatal("expected unknown key to be rejected")
	}
	if _, err := NormalizePersonalCenterVisibilityOverrides(jsonmap.JSON{"orders": "false"}); err == nil {
		t.Fatal("expected non-boolean value to be rejected")
	}
	got, err := NormalizePersonalCenterVisibilityOverrides(jsonmap.JSON{"orders": false})
	if err != nil {
		t.Fatalf("normalize valid overrides: %v", err)
	}
	if !reflect.DeepEqual(got, jsonmap.JSON{"orders": false}) {
		t.Fatalf("sparse overrides changed: %#v", got)
	}
	advanced, err := NormalizePersonalCenterVisibilityOverrides(jsonmap.JSON{
		"affiliate": false,
		"reseller":  false,
		"api":       false,
	})
	if err != nil {
		t.Fatalf("normalize advanced overrides: %v", err)
	}
	if !reflect.DeepEqual(advanced, jsonmap.JSON{"affiliate": false, "reseller": false, "api": false}) {
		t.Fatalf("advanced overrides changed: %#v", advanced)
	}
}
