package config

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

const checkYAML = `http_checks:
  - id: "health"
    url: "http://localhost:8080/health"
    interval: "30s"
    expect:
      json:
        - pointer: "/ready"
          operator: "eq"
          value: true
`

func TestParseDefaultsAndOfflineSecrets(t *testing.T) {
	c, err := Parse([]byte(checkYAML))
	if err != nil {
		t.Fatal(err)
	}
	h := c.HTTPChecks[0]
	if h.Method != "GET" || h.Timeout.Value() != 5*time.Second || h.MaxBodyBytes != 1<<20 || h.Expectations.Status != 200 || h.AlertPolicy.Failures != 3 {
		t.Fatalf("incorrect defaults: %+v", h)
	}
	t.Setenv("TEST_PI_MONITOR_ABSENT_TOKEN", "")
	_, err = Parse([]byte(checkYAML + `telegram:
  enabled: true
  token_env: "TEST_PI_MONITOR_ABSENT_TOKEN"
  allowed_chat_ids: [12]
  allowed_user_ids: [34]
  alert_chat_ids: [12]
`))
	if err != nil {
		t.Fatalf("offline parse unexpectedly requires secret: %v", err)
	}
}

func TestRebootPermissionRequiresExplicitBoolean(t *testing.T) {
	settings, err := Parse([]byte("schema_version: 1\n"))
	if err != nil || settings.Telegram.AllowReboot {
		t.Fatalf("reboot must default off: settings=%+v error=%v", settings.Telegram, err)
	}
	for _, value := range []string{"true", "false"} {
		settings, err := Parse([]byte("telegram:\n  allow_reboot: " + value + "\n"))
		if err != nil || settings.Telegram.AllowReboot != (value == "true") {
			t.Fatalf("allow_reboot %s: settings=%+v error=%v", value, settings.Telegram, err)
		}
	}
	for _, value := range []string{"null", "yes", "1", `"true"`} {
		if _, err := Parse([]byte("telegram:\n  allow_reboot: " + value + "\n")); err == nil {
			t.Fatalf("non-boolean allow_reboot %s was accepted", value)
		}
	}
	generated, err := Generate(GenerationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	settings, err = Parse(generated)
	if err != nil || settings.Telegram.AllowReboot || !strings.Contains(string(generated), "allow_reboot: false") {
		t.Fatalf("generated config must keep reboot off: error=%v", err)
	}
}
func TestRejectInvalidConfiguration(t *testing.T) {
	cases := map[string]string{
		"unknown root":             "typo: true",
		"unknown nested":           "host:\n  typo: true",
		"duplicate key":            "schema_version: 1\nschema_version: 1",
		"alias":                    "host: &host {enabled: true}\ndashboard: *host",
		"merge":                    "host:\n  <<: {enabled: true}",
		"multiple documents":       "schema_version: 1\n---\nschema_version: 1",
		"duration":                 "host:\n  interval: 10",
		"zero duration":            "host:\n  interval: \"0s\"",
		"overflow days":            "storage:\n  retention: \"9999999999999999999999999999999999999999999999d\"",
		"duplicate IDs":            checkYAML + "heartbeats:\n  - id: health\n    max_age: \"1h\"\n    token_env: TOKEN",
		"zero timeout":             strings.Replace(checkYAML, "interval: \"30s\"", "interval: \"30s\"\n    timeout: \"0s\"", 1),
		"zero status":              strings.Replace(checkYAML, "expect:", "expect:\n      status: 0", 1),
		"unknown header":           strings.Replace(checkYAML, "expect:", "headers:\n      Authorization: {env: TOKEN, other: bad}\n    expect:", 1),
		"bad pointer":              strings.Replace(checkYAML, "/ready", "/a~2", 1),
		"missing JSON value":       strings.Replace(checkYAML, "          value: true\n", "", 1),
		"numeric boolean":          strings.Replace(checkYAML, "operator: \"eq\"", "operator: \"gt\"", 1),
		"HEAD body":                strings.Replace(checkYAML, "interval:", "method: HEAD\n    interval:", 1),
		"bad header":               strings.Replace(checkYAML, "expect:", "headers:\n      'bad name': value\n    expect:", 1),
		"bad scheme":               strings.Replace(checkYAML, "http://localhost:8080/health", "file:///tmp/health", 1),
		"null host section":        "host: null",
		"null duration":            "host:\n  interval: null",
		"null schema":              "schema_version: null",
		"null enabled":             "dashboard:\n  enabled: null",
		"legacy boolean":           "dashboard:\n  enabled: yes",
		"quoted boolean":           "dashboard:\n  enabled: \"true\"",
		"explicit invalid bool":    strings.Replace(checkYAML, "value: true", "value: !!bool yes", 1),
		"explicit invalid null":    strings.Replace(checkYAML, "value: true", "value: !!null bogus", 1),
		"explicit invalid integer": strings.Replace(checkYAML, "value: true", "value: !!int 1.0", 1),
		"header case collision":    strings.Replace(checkYAML, "expect:", "headers:\n      Authorization: a\n      authorization: b\n    expect:", 1),
		"nested unknown assertion": strings.Replace(checkYAML, "value: true", "value: true\n          surprise: 1", 1),
	}
	for name, s := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(s)); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}
func TestJSONScalarPrecisionAndNull(t *testing.T) {
	for _, v := range []string{"9007199254740993", "1.0", "null", "\"true\"", "false"} {
		t.Run(v, func(t *testing.T) {
			c, err := Parse([]byte(strings.Replace(checkYAML, "value: true", "value: "+v, 1)))
			if err != nil {
				t.Fatal(err)
			}
			if c.HTTPChecks[0].Expectations.JSON[0].Value.Value != v && v != "\"true\"" {
				t.Fatal("scalar precision changed")
			}
		})
	}
}

func TestIntegerSettingsRejectCoercion(t *testing.T) {
	settings := []struct {
		path, document string
	}{
		{"schema_version", "schema_version: %s\n"},
		{"telegram.allowed_chat_ids[]", "telegram:\n  allowed_chat_ids: [%s]\n"},
		{"telegram.allowed_user_ids[]", "telegram:\n  allowed_user_ids: [%s]\n"},
		{"telegram.alert_chat_ids[]", "telegram:\n  alert_chat_ids: [%s]\n"},
		{"docker.expected_services[].replicas", "docker:\n  expected_services:\n    - id: bot\n      compose_project: example\n      compose_service: bot\n      replicas: %s\n"},
		{"http_checks[].max_body_bytes", checkYAML + "    max_body_bytes: %s\n"},
		{"http_checks[].expect.status", strings.Replace(checkYAML, "    expect:\n", "    expect:\n      status: %s\n", 1)},
		{"http_checks[].alert.failures", checkYAML + "    alert:\n      failures: %s\n"},
		{"http_checks[].alert.recovery_successes", checkYAML + "    alert:\n      recovery_successes: %s\n"},
	}
	for _, setting := range settings {
		t.Run(setting.path, func(t *testing.T) {
			for _, value := range []string{"1.9", "1.0", "1e0", `"1"`} {
				_, err := Parse([]byte(fmt.Sprintf(setting.document, value)))
				if err == nil || !strings.Contains(err.Error(), setting.path+" requires an integer") {
					t.Fatalf("%s: expected integer error for %s, got %v", setting.path, value, err)
				}
			}
			valid := "1"
			if setting.path == "http_checks[].expect.status" {
				valid = "200"
			}
			if _, err := Parse([]byte(fmt.Sprintf(setting.document, valid))); err != nil {
				t.Fatalf("integer rejected: %v", err)
			}
		})
	}
}

func TestTelegramAlertDestinationsAreIndependent(t *testing.T) {
	const document = `telegram:
  enabled: true
  allowed_chat_ids: [12]
  allowed_user_ids: [34]
  alert_chat_ids: [-56]
`
	c, err := Parse([]byte(document))
	if err != nil {
		t.Fatalf("alert-only destination rejected: %v", err)
	}
	if len(c.Telegram.AllowedChatIDs) != 1 || c.Telegram.AllowedChatIDs[0] != 12 || c.Telegram.AlertChatIDs[0] != -56 {
		t.Fatalf("alert destination changed authorization: %+v", c.Telegram)
	}
	for _, field := range []string{"allowed_chat_ids", "allowed_user_ids", "alert_chat_ids"} {
		invalid := document
		switch field {
		case "allowed_chat_ids":
			invalid = strings.Replace(invalid, "[12]", "[0]", 1)
		case "allowed_user_ids":
			invalid = strings.Replace(invalid, "[34]", "[0]", 1)
		case "alert_chat_ids":
			invalid = strings.Replace(invalid, "[-56]", "[0]", 1)
		}
		if _, err := Parse([]byte(invalid)); err == nil {
			t.Errorf("zero %s accepted", field)
		}
	}
}

func TestTelegramValidationIdentifiesMissingSetting(t *testing.T) {
	for _, field := range []string{"token_env", "allowed_chat_ids", "allowed_user_ids", "alert_chat_ids"} {
		t.Run(field, func(t *testing.T) {
			settings := Defaults()
			settings.Telegram = Telegram{
				Enabled: true, TokenEnvVar: "PI_MONITOR_TELEGRAM_TOKEN",
				AllowedChatIDs: []int64{12}, AllowedUserIDs: []int64{34}, AlertChatIDs: []int64{12},
			}
			switch field {
			case "token_env":
				settings.Telegram.TokenEnvVar = "123:fixture-token"
			case "allowed_chat_ids":
				settings.Telegram.AllowedChatIDs = nil
			case "allowed_user_ids":
				settings.Telegram.AllowedUserIDs = nil
			case "alert_chat_ids":
				settings.Telegram.AlertChatIDs = nil
			}
			err := Validate(settings)
			if err == nil || !strings.Contains(err.Error(), "telegram."+field) {
				t.Fatalf("expected field-specific error, got %v", err)
			}
			if strings.Contains(err.Error(), "fixture-token") {
				t.Fatal("validation error exposed token")
			}
		})
	}
}

func TestGenerateRoundTrip(t *testing.T) {
	for _, installed := range []bool{false, true} {
		for _, docker := range []bool{false, true} {
			for _, dashboard := range []bool{false, true} {
				b, err := Generate(GenerationOptions{DockerEnabled: docker, DashboardEnabled: dashboard, SystemPaths: installed})
				if err != nil {
					t.Fatal(err)
				}
				c, err := Parse(b)
				if err != nil {
					t.Fatal(err)
				}
				if c.Docker.Enabled != docker || c.Dashboard.Enabled != dashboard || len(c.HostRules) != 4 || len(c.HTTPChecks) != 0 {
					t.Fatalf("incorrect generated configuration: %+v", c)
				}
				if installed && c.Storage.Path != "/var/lib/pi-monitor/monitor.db" {
					t.Fatal("wrong installation path")
				}
			}
		}
	}
}
func TestDurationDays(t *testing.T) {
	c, err := Parse([]byte("storage:\n  retention: \"1.5d\""))
	if err != nil {
		t.Fatal(err)
	}
	if c.Storage.Retention.Value() != 36*time.Hour {
		t.Fatal("incorrect day conversion")
	}
}
