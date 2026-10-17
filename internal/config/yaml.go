package config

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var dayDurationPattern = regexp.MustCompile(`^[+-]?[0-9]+(\.[0-9]+)?$`)
var storageBudgetPattern = regexp.MustCompile(`^[1-9][0-9]*(KiB|MiB|GiB)$`)
var environmentVariableName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var entityIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
var decimalNumberPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)
var decimalIntegerPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
		return errors.New("duration must be a quoted duration string")
	}
	text := node.Value
	if len(text) > 64 {
		return errors.New("duration is too long")
	}
	if strings.HasSuffix(text, "d") {
		dayText := strings.TrimSuffix(text, "d")
		if !dayDurationPattern.MatchString(dayText) {
			return fmt.Errorf("invalid duration %q", text)
		}
		days, ok := new(big.Rat).SetString(dayText)
		if !ok {
			return fmt.Errorf("invalid duration %q", text)
		}
		days.Mul(days, new(big.Rat).SetInt64(int64(24*time.Hour)))
		if !days.IsInt() || !days.Num().IsInt64() {
			return fmt.Errorf("duration %q exceeds supported precision or range", text)
		}
		*d = Duration(days.Num().Int64())
		return nil
	}
	duration, err := time.ParseDuration(text)
	if err != nil {
		return err
	}
	*d = Duration(duration)
	return nil
}

func (d Duration) MarshalYAML() (any, error) { return d.Value().String(), nil }

func (header *RequestHeader) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode && node.Tag == "!!str" {
		header.Value = node.Value
		return nil
	}
	if node.Kind != yaml.MappingNode || len(node.Content) != 2 || node.Content[0].Value != "env" || node.Content[1].Tag != "!!str" {
		return errors.New("header must be a string or {env: NAME}")
	}
	header.EnvVar = node.Content[1].Value
	if !environmentVariableName.MatchString(header.EnvVar) {
		return errors.New("invalid header environment variable name")
	}
	return nil
}

func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func validateYAMLNode(node *yaml.Node, path string) error {
	if node.Kind == yaml.AliasNode || node.Anchor != "" {
		return fmt.Errorf("line %d: YAML aliases and anchors are unsupported", node.Line)
	}
	if node.Kind == yaml.MappingNode {
		seen := map[string]bool{}
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || key.Value == "<<" {
				return fmt.Errorf("line %d: mapping keys must be strings; merge keys are unsupported", key.Line)
			}
			if seen[key.Value] {
				return fmt.Errorf("line %d: duplicate key %q", key.Line, key.Value)
			}
			seen[key.Value] = true
			childPath := key.Value
			if path != "" {
				childPath = path + "." + key.Value
			}
			if err := validateYAMLNode(node.Content[i+1], childPath); err != nil {
				return err
			}
		}
	} else if node.Kind == yaml.SequenceNode {
		for _, child := range node.Content {
			if err := validateYAMLNode(child, path+"[]"); err != nil {
				return err
			}
		}
	} else if node.Kind == yaml.ScalarNode {
		switch path {
		case "dashboard.enabled", "telegram.enabled", "telegram.allow_reboot", "host.enabled", "docker.enabled", "http_checks[].follow_redirects":
			if node.Tag != "!!bool" {
				return fmt.Errorf("line %d: boolean setting requires true or false", node.Line)
			}
		case "schema_version", "telegram.allowed_chat_ids[]", "telegram.allowed_user_ids[]", "telegram.alert_chat_ids[]",
			"docker.expected_services[].replicas", "http_checks[].max_body_bytes", "http_checks[].expect.status",
			"http_checks[].alert.failures", "http_checks[].alert.recovery_successes":
			// yaml.v3 otherwise silently truncates floats when decoding into integers.
			if node.Tag != "!!int" {
				return fmt.Errorf("line %d: %s requires an integer", node.Line, path)
			}
		}
		switch node.Tag {
		case "!!bool":
			switch node.Value {
			case "true", "True", "TRUE", "false", "False", "FALSE":
			default:
				return fmt.Errorf("line %d: invalid boolean scalar", node.Line)
			}
		case "!!null":
			if path != "http_checks[].expect.json[].value" {
				return fmt.Errorf("line %d: null is only supported as a JSON assertion value", node.Line)
			}
			switch node.Value {
			case "", "null", "Null", "NULL", "~":
			default:
				return fmt.Errorf("line %d: invalid null scalar", node.Line)
			}
		}
	}
	return nil
}

func validateAssertionFields(checkID string, expectation *yaml.Node) error {
	for _, kind := range []string{"text", "json"} {
		assertions := mappingValue(expectation, kind)
		if assertions == nil {
			continue
		}
		for _, assertion := range assertions.Content {
			value := mappingValue(assertion, "value")
			if value == nil {
				return fmt.Errorf("HTTP check %q: %s assertion requires an explicit value", checkID, kind)
			}
			if kind == "text" && value.Tag != "!!str" {
				return fmt.Errorf("HTTP check %q: text assertion requires a string value", checkID)
			}
			if kind == "json" {
				pointer := mappingValue(assertion, "pointer")
				if pointer == nil || pointer.Tag != "!!str" {
					return fmt.Errorf("HTTP check %q: JSON assertion requires an explicit string pointer", checkID)
				}
			}
		}
	}
	return nil
}
