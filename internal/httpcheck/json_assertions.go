package httpcheck

import (
	"encoding/json"
	"math/big"
	"strconv"
	"strings"

	"pi-monitoring/internal/config"
)

func evaluateJSONAssertion(root any, assertion config.JSONAssertion) (string, string) {
	actual, present, valid := resolveJSONPointer(root, assertion.Pointer)
	if !valid {
		return "configuration", "invalid JSON pointer"
	}
	if assertion.Operator == "exists" {
		if assertion.Value.Tag != "!!bool" {
			return "configuration", "exists requires a boolean"
		}
		expected, err := strconv.ParseBool(assertion.Value.Value)
		if err != nil {
			return "configuration", "exists requires a boolean"
		}
		if present == expected {
			return "ok", "assertion passed"
		}
		return "missing", "field existence did not match"
	}
	if !present {
		return "missing", "JSON field is missing"
	}
	var expected any
	switch assertion.Value.Tag {
	case "!!null":
		expected = nil
	case "!!str":
		expected = assertion.Value.Value
	case "!!bool":
		boolean, err := strconv.ParseBool(assertion.Value.Value)
		if err != nil {
			return "configuration", "invalid boolean assertion"
		}
		expected = boolean
	case "!!int", "!!float":
		number, ok := parseYAMLNumber(assertion.Value.Value, assertion.Value.Tag == "!!int")
		if !ok {
			return "configuration", "unsupported numeric assertion"
		}
		expected = number
	default:
		return "configuration", "assertion requires a scalar value"
	}
	comparison := 0
	switch actualValue := actual.(type) {
	case nil:
		if expected != nil {
			return "type", "JSON value has an unexpected type"
		}
	case string:
		expectedValue, ok := expected.(string)
		if !ok {
			return "type", "JSON value has an unexpected type"
		}
		comparison = strings.Compare(actualValue, expectedValue)
	case bool:
		expectedValue, ok := expected.(bool)
		if !ok {
			return "type", "JSON value has an unexpected type"
		}
		if actualValue != expectedValue {
			comparison = 1
		}
	case json.Number:
		expectedValue, ok := expected.(*big.Rat)
		if !ok {
			return "type", "JSON value has an unexpected type"
		}
		number, ok := parseDecimalNumber(string(actualValue))
		if !ok {
			return "body", "JSON number exceeds supported precision limits"
		}
		comparison = number.Cmp(expectedValue)
	default:
		return "type", "JSON value must be a scalar"
	}
	pass := false
	switch assertion.Operator {
	case "eq":
		pass = comparison == 0
	case "ne":
		pass = comparison != 0
	case "gt", "gte", "lt", "lte":
		if _, ok := actual.(json.Number); !ok {
			return "type", "ordering requires a numeric value"
		}
		switch assertion.Operator {
		case "gt":
			pass = comparison > 0
		case "gte":
			pass = comparison >= 0
		case "lt":
			pass = comparison < 0
		case "lte":
			pass = comparison <= 0
		}
	default:
		return "configuration", "unsupported assertion operator"
	}
	if pass {
		return "ok", "assertion passed"
	}
	return "value", "JSON value did not match"
}

func resolveJSONPointer(root any, path string) (any, bool, bool) {
	if path == "" {
		return root, true, true
	}
	if !strings.HasPrefix(path, "/") {
		return nil, false, false
	}
	current := root
	for _, part := range strings.Split(path[1:], "/") {
		// Only ~0 and ~1 escapes are permitted. Decode after validating so ~01
		// becomes the literal ~1 rather than being decoded twice.
		for i := 0; i < len(part); i++ {
			if part[i] == '~' {
				if i+1 >= len(part) || (part[i+1] != '0' && part[i+1] != '1') {
					return nil, false, false
				}
				i++
			}
		}
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch value := current.(type) {
		case map[string]any:
			var found bool
			current, found = value[part]
			if !found {
				return nil, false, true
			}
		case []any:
			if part == "" || (len(part) > 1 && part[0] == '0') {
				return nil, false, true
			}
			for _, r := range part {
				if r < '0' || r > '9' {
					return nil, false, true
				}
			}
			index, err := strconv.Atoi(part)
			if err != nil || index >= len(value) {
				return nil, false, true
			}
			current = value[index]
		default:
			return nil, false, true
		}
	}
	return current, true, true
}

func parseYAMLNumber(value string, integer bool) (*big.Rat, bool) {
	value = strings.ReplaceAll(value, "_", "")
	if integer {
		if len(value) > 1024 {
			return nil, false
		}
		number, ok := new(big.Int).SetString(value, 10)
		if !ok {
			return nil, false
		}
		return new(big.Rat).SetInt(number), true
	}
	return parseDecimalNumber(value)
}

// Bound both input length and exponent before constructing arbitrary-precision
// numbers. This preserves exact ordinary decimals without unbounded allocation.
func parseDecimalNumber(value string) (*big.Rat, bool) {
	if len(value) > 1024 {
		return nil, false
	}
	if i := strings.IndexAny(value, "eE"); i >= 0 {
		exponent, err := strconv.Atoi(value[i+1:])
		if err != nil || exponent < -10000 || exponent > 10000 {
			return nil, false
		}
	}
	number, ok := new(big.Rat).SetString(value)
	return number, ok
}
