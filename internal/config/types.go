package config

import (
	"time"

	"gopkg.in/yaml.v3"
)

// Duration uses Go duration syntax plus a whole or fractional number of days.
type Duration time.Duration

func (d Duration) Value() time.Duration { return time.Duration(d) }

type Config struct {
	SchemaVersion int         `yaml:"schema_version"`
	Server        Server      `yaml:"server"`
	Dashboard     Dashboard   `yaml:"dashboard"`
	Storage       Storage     `yaml:"storage"`
	Telegram      Telegram    `yaml:"telegram"`
	Host          Host        `yaml:"host"`
	Alerts        Alerts      `yaml:"alerts"`
	HostRules     []HostRule  `yaml:"host_rules"`
	Docker        Docker      `yaml:"docker"`
	Heartbeats    []Heartbeat `yaml:"heartbeats"`
	HTTPChecks    []HTTPCheck `yaml:"http_checks"`
}

type Server struct {
	Name   string `yaml:"name"`
	Listen string `yaml:"listen"`
}

type Dashboard struct {
	Enabled bool `yaml:"enabled"`
}

type Storage struct {
	Path          string   `yaml:"path"`
	Retention     Duration `yaml:"retention"`
	Budget        string   `yaml:"budget"`
	FlushInterval Duration `yaml:"flush_interval"`
}

type Telegram struct {
	Enabled        bool    `yaml:"enabled"`
	AllowReboot    bool    `yaml:"allow_reboot"`
	TokenEnvVar    string  `yaml:"token_env"`
	AllowedChatIDs []int64 `yaml:"allowed_chat_ids"`
	AllowedUserIDs []int64 `yaml:"allowed_user_ids"`
	AlertChatIDs   []int64 `yaml:"alert_chat_ids"`
}

type Host struct {
	Enabled  bool     `yaml:"enabled"`
	Interval Duration `yaml:"interval"`
	Mounts   []string `yaml:"mounts"`
}

type Alerts struct {
	StartupGrace           Duration `yaml:"startup_grace"`
	CriticalRepeatInterval Duration `yaml:"critical_repeat_interval"`
}

type HostRule struct {
	ID       string            `yaml:"id"`
	Metric   string            `yaml:"metric"`
	Operator string            `yaml:"operator"`
	Value    float64           `yaml:"value"`
	For      Duration          `yaml:"for"`
	Recovery RecoveryCondition `yaml:"recovery"`
}

type RecoveryCondition struct {
	Operator string   `yaml:"operator"`
	Value    float64  `yaml:"value"`
	For      Duration `yaml:"for"`
}

type Docker struct {
	Enabled          bool              `yaml:"enabled"`
	Socket           string            `yaml:"socket"`
	Interval         Duration          `yaml:"interval"`
	ExpectedServices []ExpectedService `yaml:"expected_services"`
}

type ExpectedService struct {
	ID             string   `yaml:"id"`
	ComposeProject string   `yaml:"compose_project"`
	ComposeService string   `yaml:"compose_service"`
	Replicas       int      `yaml:"replicas"`
	StartupGrace   Duration `yaml:"startup_grace"`
}

type Heartbeat struct {
	ID           string   `yaml:"id"`
	MaxAge       Duration `yaml:"max_age"`
	InitialGrace Duration `yaml:"initial_grace"`
	TokenEnvVar  string   `yaml:"token_env"`
}

type HTTPCheck struct {
	ID              string                   `yaml:"id"`
	URL             string                   `yaml:"url"`
	Interval        Duration                 `yaml:"interval"`
	Timeout         Duration                 `yaml:"timeout"`
	Method          string                   `yaml:"method"`
	FollowRedirects bool                     `yaml:"follow_redirects"`
	MaxBodyBytes    int64                    `yaml:"max_body_bytes"`
	Headers         map[string]RequestHeader `yaml:"headers"`
	Expectations    Expectations             `yaml:"expect"`
	AlertPolicy     AlertPolicy              `yaml:"alert"`
}

// A header is either a literal scalar or an explicit {env: NAME} mapping.
type RequestHeader struct {
	Value  string
	EnvVar string
}

type Expectations struct {
	Status int             `yaml:"status"`
	Text   []TextAssertion `yaml:"text"`
	JSON   []JSONAssertion `yaml:"json"`
}

type TextAssertion struct {
	Operator string `yaml:"operator"`
	Value    string `yaml:"value"`
}

// A node preserves scalar type, numeric precision and explicit null values.
type JSONAssertion struct {
	Pointer  string    `yaml:"pointer"`
	Operator string    `yaml:"operator"`
	Value    yaml.Node `yaml:"value"`
}

type AlertPolicy struct {
	Failures          int `yaml:"failures"`
	RecoverySuccesses int `yaml:"recovery_successes"`
}
