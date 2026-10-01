package settingssecurity

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/dujiao-next/internal/shared/jsonmap"
)

var ErrSecurityCenterConfigInvalid = errors.New("security center config invalid")
var ErrSecurityCenterDisabled = errors.New("security center feature disabled")

// SecurityCenterKeys are independent account-management controls, not provider login controls.
func SecurityCenterKeys() []string {
	return []string{"telegram_binding", "google_binding", "email_change", "password_change", "two_factor", "login_history"}
}

// NormalizeSecurityCenterConfigJSON preserves legacy availability for absent fields.
// Explicit malformed persisted fields fail closed; unknown fields are never exposed.
func NormalizeSecurityCenterConfigJSON(raw jsonmap.JSON) jsonmap.JSON {
	result := make(jsonmap.JSON, 6)
	for _, key := range SecurityCenterKeys() {
		value, exists := raw[key]
		enabled := true
		if exists {
			enabled, _ = value.(bool)
		}
		result[key] = enabled
	}
	return result
}

func ValidateSecurityCenterConfig(raw jsonmap.JSON) error {
	allowed := NormalizeSecurityCenterConfigJSON(nil)
	for key, value := range raw {
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("%w: unknown field %q", ErrSecurityCenterConfigInvalid, key)
		}
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%w: %q must be boolean", ErrSecurityCenterConfigInvalid, key)
		}
	}
	return nil
}

// DecodeSecurityCenterConfig rejects duplicates before decoding into a map.
func DecodeSecurityCenterConfig(data []byte) (jsonmap.JSON, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, ErrSecurityCenterConfigInvalid
	}
	raw := jsonmap.JSON{}
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return nil, ErrSecurityCenterConfigInvalid
		}
		key, ok := token.(string)
		if !ok {
			return nil, ErrSecurityCenterConfigInvalid
		}
		if _, exists := raw[key]; exists {
			return nil, ErrSecurityCenterConfigInvalid
		}
		var value interface{}
		if err = decoder.Decode(&value); err != nil {
			return nil, ErrSecurityCenterConfigInvalid
		}
		raw[key] = value
	}
	if _, err = decoder.Token(); err != nil {
		return nil, ErrSecurityCenterConfigInvalid
	}
	if err = decoder.Decode(new(interface{})); err != io.EOF {
		return nil, ErrSecurityCenterConfigInvalid
	}
	if err = ValidateSecurityCenterConfig(raw); err != nil {
		return nil, err
	}
	return NormalizeSecurityCenterConfigJSON(raw), nil
}
