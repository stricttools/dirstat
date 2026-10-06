// Package scanconfig loads and validates the optional TOML scan-config file
// selected with --config (spec §11, R41–R44). The file supplies scan-semantic
// defaults only; it is never auto-discovered and rendering/output options are
// deliberately rejected.
package scanconfig

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/stricttools/dirstat/internal/scan"
	tomledit "github.com/stricttools/go-toml-edit"
)

// AllowedKeys is the exact set of keys a scan config file may contain
// (flag names with underscores), in canonical order (R42).
var AllowedKeys = []string{
	"exclude", "method", "formats", "depth", "ignored", "hidden",
	"type", "stats", "sort_by", "sort_order",
}

// renderingKeys are flags that exist on the scan command but are rendering
// or output options, deliberately not allowed in a scan config file (R42).
var renderingKeys = map[string]bool{
	"colors": true, "style": true, "output": true, "top": true,
	"show": true, "combined": true, "singletons": true,
	"list_no_ext": true, "legend": true, "human": true,
}

// choices holds the valid values of each choice-typed key, mirroring the
// flag registrations in scan.go (R8).
var choices = map[string][]string{
	"method":     {"ext", "type", "hybrid"},
	"formats":    {"raw", "canonical"},
	"ignored":    {"include", "exclude", "only"},
	"hidden":     {"include", "exclude"},
	"type":       {"text", "binary", "both"},
	"sort_order": {"asc", "desc"},
}

// flagToKey maps the CLI flag names of the R42 set to config key names.
var flagToKey = map[string]string{
	"exclude": "exclude", "method": "method", "formats": "formats", "depth": "depth",
	"ignored": "ignored", "hidden": "hidden", "type": "type",
	"stats": "stats", "sort-by": "sort_by", "sort-order": "sort_order",
}

// Config holds the validated values from a scan config file. Only keys
// present in the file are set. Values are kwargs-ready: strings, ints, and
// []interface{} of string for the repeatable keys.
type Config struct {
	values map[string]interface{}
}

// Has reports whether the file set the given key.
func (c *Config) Has(key string) bool {
	_, ok := c.values[key]
	return ok
}

// Overlay copies every file-set value into kwargs. Keys absent from the
// file are left untouched (R44).
func (c *Config) Overlay(kwargs map[string]interface{}) {
	for key, val := range c.values {
		kwargs[key] = val
	}
}

// Load reads and fully validates a scan config file. Any missing or
// unreadable file, malformed TOML, unknown key, rendering/output key, wrong
// type, invalid value, or duplicate array value is an error (R41, R42).
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		// Strip the "open <path>:" prefix of *os.PathError so the path is
		// not repeated in the message.
		var pathErr *os.PathError
		if errors.As(err, &pathErr) {
			err = pathErr.Err
		}
		return nil, fmt.Errorf("config file %s: %v", path, err)
	}

	raw, err := tomledit.Unmarshal[map[string]interface{}](data)
	if err != nil {
		return nil, fmt.Errorf("config file %s: %v", path, err)
	}

	cfg := &Config{values: make(map[string]interface{}, len(*raw))}
	for key, val := range *raw {
		if err := cfg.setKey(key, val); err != nil {
			return nil, fmt.Errorf("config file %s: %v", path, err)
		}
	}
	return cfg, nil
}

// setKey validates one key/value pair and stores the kwargs-ready value.
func (c *Config) setKey(key string, val interface{}) error {
	if key == "where" {
		return fmt.Errorf(`key "where" cannot be set from a config file; pass the directory as a positional argument`)
	}
	if renderingKeys[key] {
		return fmt.Errorf("key %q is a rendering/output option and cannot be set from a config file; allowed keys: %s",
			key, strings.Join(AllowedKeys, ", "))
	}

	switch key {
	case "exclude":
		items, err := stringArray(key, val)
		if err != nil {
			return err
		}
		c.values[key] = items
	case "method", "formats", "ignored", "hidden", "type", "sort_order":
		s, ok := val.(string)
		if !ok {
			return wrongType(key, "a string", val)
		}
		if !contains(choices[key], s) {
			return fmt.Errorf("invalid value %q for key %q; valid: %s",
				s, key, strings.Join(choices[key], ", "))
		}
		c.values[key] = s
	case "depth":
		n, ok := val.(int64)
		if !ok {
			return wrongType(key, "an integer", val)
		}
		c.values[key] = int(n)
	case "stats":
		items, err := stringArray(key, val)
		if err != nil {
			return err
		}
		for _, item := range items {
			if !contains(scan.StatNames, item.(string)) {
				return fmt.Errorf("invalid stat %q in key %q; valid: %s",
					item, key, strings.Join(scan.StatNames, ", "))
			}
		}
		c.values[key] = items
	case "sort_by":
		items, err := stringArray(key, val)
		if err != nil {
			return err
		}
		for _, item := range items {
			if item != "format" && !contains(scan.StatNames, item.(string)) {
				return fmt.Errorf("invalid sort column %q in key %q; valid: format, %s",
					item, key, strings.Join(scan.StatNames, ", "))
			}
		}
		c.values[key] = items
	default:
		return fmt.Errorf("unknown key %q; allowed keys: %s",
			key, strings.Join(AllowedKeys, ", "))
	}
	return nil
}

// stringArray validates that val is an array of unique strings and returns
// it as a kwargs-ready []interface{}.
func stringArray(key string, val interface{}) ([]interface{}, error) {
	arr, ok := val.([]interface{})
	if !ok {
		return nil, wrongType(key, "an array of strings", val)
	}
	seen := make(map[string]bool, len(arr))
	out := make([]interface{}, 0, len(arr))
	for _, elem := range arr {
		s, ok := elem.(string)
		if !ok {
			return nil, wrongType(key, "an array of strings", val)
		}
		if seen[s] {
			return nil, fmt.Errorf("duplicate value %q in key %q", s, key)
		}
		seen[s] = true
		out = append(out, elem)
	}
	return out, nil
}

func wrongType(key, want string, got interface{}) error {
	return fmt.Errorf("key %q must be %s, got %s", key, want, tomlTypeName(got))
}

// tomlTypeName names a decoded TOML value's type in user-facing terms.
func tomlTypeName(v interface{}) string {
	switch v.(type) {
	case string:
		return "a string"
	case int64:
		return "an integer"
	case float64:
		return "a float"
	case bool:
		return "a boolean"
	case []interface{}:
		return "an array"
	case map[string]interface{}:
		return "a table"
	default:
		return fmt.Sprintf("%T", v)
	}
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

// ExplicitKeys scans raw argv for the R42 flag set and reports which config
// keys were explicitly passed on the command line, matching the exact token
// "--<flag>" or the prefix "--<flag>=" (R43). None of these flags are
// booleans, so no --no-* forms exist.
func ExplicitKeys(argv []string) map[string]bool {
	out := map[string]bool{}
	for _, tok := range argv {
		if !strings.HasPrefix(tok, "--") {
			continue
		}
		name := strings.TrimPrefix(tok, "--")
		if i := strings.IndexByte(name, '='); i >= 0 {
			name = name[:i]
		}
		if key, ok := flagToKey[name]; ok {
			out[key] = true
		}
	}
	return out
}
