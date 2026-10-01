package customupgrade

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"unicode/utf8"
)

// MaxManifestBytes bounds authentication and parsing work before any allocation.
const MaxManifestBytes = 1 << 20

// strictJSON checks exact canonical field names (Go's case folding is unsafe
// here), recursively rejects duplicates/null/missing fields, and bounds depth.
func strictJSON(raw []byte) error {
	if !utf8.Valid(raw) {
		return fmt.Errorf("customupgrade: malformed UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := jsonValue(d, reflect.TypeFor[Manifest](), 0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return fmt.Errorf("customupgrade: trailing JSON")
	}
	return nil
}
func jsonValue(d *json.Decoder, typ reflect.Type, depth int) error {
	if depth > 16 {
		return fmt.Errorf("customupgrade: JSON depth")
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	invalid := func() error { return fmt.Errorf("customupgrade: invalid JSON %s", typ) }
	switch typ.Kind() {
	case reflect.Struct:
		if token != json.Delim('{') {
			return invalid()
		}
		fields := make(map[string]reflect.Type, typ.NumField())
		seen := make(map[string]bool)
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			fields[f.Tag.Get("json")] = f.Type
		}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok {
				return invalid()
			}
			field, ok := fields[name]
			if !ok || seen[name] {
				return fmt.Errorf("customupgrade: unknown or duplicate JSON field %q", name)
			}
			seen[name] = true
			if err := jsonValue(d, field, depth+1); err != nil {
				return err
			}
		}
		if len(seen) != len(fields) {
			return fmt.Errorf("customupgrade: missing JSON field")
		}
		end, err := d.Token()
		if err != nil {
			return err
		}
		if end != json.Delim('}') {
			return invalid()
		}
	case reflect.Slice:
		if token != json.Delim('[') {
			return invalid()
		}
		for d.More() {
			if err := jsonValue(d, typ.Elem(), depth+1); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil {
			return err
		}
		if end != json.Delim(']') {
			return invalid()
		}
	case reflect.String:
		if _, ok := token.(string); !ok {
			return invalid()
		}
	case reflect.Uint32, reflect.Uint64:
		num, ok := token.(json.Number)
		if !ok {
			return invalid()
		}
		if _, err := strconv.ParseUint(string(num), 10, typ.Bits()); err != nil {
			return invalid()
		}
	default:
		return invalid()
	}
	return nil
}
