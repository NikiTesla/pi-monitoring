// Package config loads and validates YAML without resolving credentials or contacting services.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Response body limits are measured in bytes, after HTTP content decoding.
const (
	DefaultHTTPBodyLimitBytes = 1 << 20
	MaxHTTPBodyLimitBytes     = 16 << 20
)

// Defaults returns fresh settings for schema version 1.
func Defaults() Config {
	return Config{
		SchemaVersion: 1,
		Server:        Server{Name: "personal-server", Listen: "127.0.0.1:8088"},
		Dashboard:     Dashboard{Enabled: true},
		Storage: Storage{
			Path: "./monitor.db", Retention: Duration(7 * 24 * time.Hour),
			Budget: "1GiB", FlushInterval: Duration(time.Minute),
		},
		Telegram: Telegram{TokenEnvVar: "PI_MONITOR_TELEGRAM_TOKEN"},
		Host:     Host{Enabled: true, Interval: Duration(10 * time.Second), Mounts: []string{"/"}},
		Alerts:   Alerts{StartupGrace: Duration(2 * time.Minute), CriticalRepeatInterval: Duration(time.Hour)},
		Docker:   Docker{Socket: "/var/run/docker.sock", Interval: Duration(30 * time.Second)},
	}
}

// Load reads and validates one YAML file without requiring credential values.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	return Parse(data)
}

// Parse rejects unsupported YAML constructs before applying defaults and validation.
func Parse(data []byte) (Config, error) {
	var document yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&document); err != nil {
		return Config{}, fmt.Errorf("YAML: %w", err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return Config{}, errors.New("configuration must be a YAML mapping")
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return Config{}, err
		}
		return Config{}, errors.New("multiple YAML documents are unsupported")
	}
	if err := validateYAMLNode(document.Content[0], ""); err != nil {
		return Config{}, err
	}
	settings := Defaults()
	typedDecoder := yaml.NewDecoder(bytes.NewReader(data))
	typedDecoder.KnownFields(true)
	if err := typedDecoder.Decode(&settings); err != nil {
		return Config{}, fmt.Errorf("configuration: %w", err)
	}
	checks := mappingValue(document.Content[0], "http_checks")
	if checks != nil && checks.Kind == yaml.SequenceNode {
		for index, checkNode := range checks.Content {
			check := &settings.HTTPChecks[index]
			if err := validateAssertionFields(check.ID, mappingValue(checkNode, "expect")); err != nil {
				return Config{}, err
			}
			applyHTTPCheckDefaults(check, checkNode)
		}
	}
	if err := Validate(settings); err != nil {
		return Config{}, err
	}
	return settings, nil
}

func applyHTTPCheckDefaults(check *HTTPCheck, checkNode *yaml.Node) {
	expectation := mappingValue(checkNode, "expect")
	if mappingValue(checkNode, "method") == nil {
		check.Method = http.MethodGet
	}
	if mappingValue(checkNode, "timeout") == nil {
		check.Timeout = Duration(5 * time.Second)
	}
	if mappingValue(checkNode, "max_body_bytes") == nil {
		check.MaxBodyBytes = DefaultHTTPBodyLimitBytes
	}
	if mappingValue(expectation, "status") == nil {
		check.Expectations.Status = 200
	}
	alert := mappingValue(checkNode, "alert")
	if mappingValue(alert, "failures") == nil {
		check.AlertPolicy.Failures = 3
	}
	if mappingValue(alert, "recovery_successes") == nil {
		check.AlertPolicy.RecoverySuccesses = 2
	}
}
