package settingsstorefront

import (
	"fmt"

	"github.com/dujiao-next/internal/shared/jsonmap"
)

var PersonalCenterVisibilityKeys = []string{
	"overview",
	"orders",
	"wallet",
	"gift_cards",
	"security",
	"profile",
	"affiliate",
	"reseller",
	"api",
}

func DefaultPersonalCenterVisibility() jsonmap.JSON {
	result := make(jsonmap.JSON, len(PersonalCenterVisibilityKeys))
	for _, key := range PersonalCenterVisibilityKeys {
		result[key] = true
	}
	return result
}

func NormalizePersonalCenterVisibilityJSON(raw jsonmap.JSON) jsonmap.JSON {
	result := DefaultPersonalCenterVisibility()
	for _, key := range PersonalCenterVisibilityKeys {
		if value, ok := raw[key].(bool); ok {
			result[key] = value
		}
	}
	return result
}

func NormalizePersonalCenterVisibilityOverrides(raw jsonmap.JSON) (jsonmap.JSON, error) {
	result := make(jsonmap.JSON, len(raw))
	for key, rawValue := range raw {
		if !isPersonalCenterVisibilityKey(key) {
			return nil, fmt.Errorf("unknown personal center visibility key %q", key)
		}
		value, ok := rawValue.(bool)
		if !ok {
			return nil, fmt.Errorf("personal center visibility %q must be boolean", key)
		}
		result[key] = value
	}
	return result, nil
}

func ResolvePersonalCenterVisibility(global, overrides jsonmap.JSON) jsonmap.JSON {
	result := NormalizePersonalCenterVisibilityJSON(global)
	for _, key := range PersonalCenterVisibilityKeys {
		if value, ok := overrides[key].(bool); ok {
			result[key] = value
		}
	}
	return result
}

func isPersonalCenterVisibilityKey(key string) bool {
	for _, candidate := range PersonalCenterVisibilityKeys {
		if key == candidate {
			return true
		}
	}
	return false
}
