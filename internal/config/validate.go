package config

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Validate checks typed settings, including sections reserved for future runtime features.
func Validate(settings Config) error {
	if settings.SchemaVersion != 1 {
		return errors.New("schema_version must be 1")
	}
	if strings.TrimSpace(settings.Server.Name) == "" {
		return errors.New("server.name must not be empty")
	}
	if _, port, err := net.SplitHostPort(settings.Server.Listen); err != nil {
		return errors.New("server.listen must be host:port")
	} else {
		portNumber, err := strconv.Atoi(port)
		if err != nil || portNumber < 1 || portNumber > 65535 {
			return errors.New("server.listen has invalid port")
		}
	}
	if settings.Storage.Path == "" || settings.Storage.Retention <= 0 || settings.Storage.FlushInterval <= 0 {
		return errors.New("storage requires path and positive retention/flush_interval")
	}
	if !storageBudgetPattern.MatchString(settings.Storage.Budget) {
		return errors.New("storage.budget must be a positive integer with KiB, MiB or GiB suffix")
	}
	if settings.Host.Interval <= 0 {
		return errors.New("host.interval must be positive")
	}
	for _, mount := range settings.Host.Mounts {
		if !filepath.IsAbs(mount) {
			return errors.New("host.mounts must contain absolute paths")
		}
	}
	if settings.Host.Enabled && len(settings.Host.Mounts) == 0 {
		return errors.New("host.mounts must not be empty when host is enabled")
	}
	if settings.Alerts.StartupGrace < 0 || settings.Alerts.CriticalRepeatInterval <= 0 {
		return errors.New("alerts require nonnegative startup_grace and positive critical_repeat_interval")
	}
	if settings.Docker.Interval <= 0 || !filepath.IsAbs(settings.Docker.Socket) {
		return errors.New("docker requires positive interval and absolute socket path")
	}
	if settings.Telegram.Enabled {
		if !environmentVariableName.MatchString(settings.Telegram.TokenEnvVar) {
			return errors.New("telegram.token_env must be an environment variable name, such as PI_MONITOR_TELEGRAM_TOKEN; put the bot token in that variable, not in YAML")
		}
		if len(settings.Telegram.AllowedChatIDs) == 0 {
			return errors.New("telegram.allowed_chat_ids must contain at least one chat ID when telegram is enabled")
		}
		if len(settings.Telegram.AllowedUserIDs) == 0 {
			return errors.New("telegram.allowed_user_ids must contain at least one sender's user ID when telegram is enabled")
		}
		if len(settings.Telegram.AlertChatIDs) == 0 {
			return errors.New("telegram.alert_chat_ids must contain at least one notification destination when telegram is enabled")
		}
		for _, id := range settings.Telegram.AllowedUserIDs {
			if id <= 0 {
				return errors.New("telegram allowed_user_ids must be positive")
			}
		}
		for _, id := range append(append([]int64{}, settings.Telegram.AllowedChatIDs...), settings.Telegram.AlertChatIDs...) {
			if id == 0 {
				return errors.New("telegram chat IDs must not be zero")
			}
		}
	}
	seenIDs := map[string]bool{}
	registerID := func(id string) error {
		if !entityIDPattern.MatchString(id) {
			return fmt.Errorf("invalid ID %q: use letters, numbers, dot, underscore or hyphen", id)
		}
		if seenIDs[id] {
			return fmt.Errorf("duplicate ID %q", id)
		}
		seenIDs[id] = true
		return nil
	}
	for _, rule := range settings.HostRules {
		if err := registerID(rule.ID); err != nil {
			return err
		}
		if err := validateHostRule(rule); err != nil {
			return fmt.Errorf("host rule %q: %w", rule.ID, err)
		}
	}
	for _, service := range settings.Docker.ExpectedServices {
		if err := registerID(service.ID); err != nil {
			return err
		}
		if service.ComposeProject == "" || service.ComposeService == "" || service.Replicas <= 0 || service.StartupGrace < 0 {
			return fmt.Errorf("docker service %q requires compose labels, positive replicas and nonnegative startup_grace", service.ID)
		}
	}
	for _, heartbeat := range settings.Heartbeats {
		if err := registerID(heartbeat.ID); err != nil {
			return err
		}
		if heartbeat.MaxAge <= 0 || heartbeat.InitialGrace < 0 || !environmentVariableName.MatchString(heartbeat.TokenEnvVar) {
			return fmt.Errorf("heartbeat %q requires positive max_age, nonnegative initial_grace and valid token_env", heartbeat.ID)
		}
	}
	for _, check := range settings.HTTPChecks {
		if err := registerID(check.ID); err != nil {
			return err
		}
		if err := validateHTTPCheck(check); err != nil {
			return fmt.Errorf("HTTP check %q: %w", check.ID, err)
		}
	}
	return nil
}

func isNumericOperator(operator string) bool {
	return operator == "gt" || operator == "gte" || operator == "lt" || operator == "lte"
}

func validateHostRule(rule HostRule) error {
	switch rule.Metric {
	case "host.memory.available_bytes", "host.disk.free_bytes", "host.disk.free_percent", "host.cpu.usage_percent", "host.temperature.celsius":
	default:
		return errors.New("unsupported metric")
	}
	if !isNumericOperator(rule.Operator) || !isNumericOperator(rule.Recovery.Operator) ||
		rule.For <= 0 || rule.Recovery.For <= 0 ||
		math.IsInf(rule.Value, 0) || math.IsNaN(rule.Value) ||
		math.IsInf(rule.Recovery.Value, 0) || math.IsNaN(rule.Recovery.Value) {
		return errors.New("requires numeric operators, finite values and positive for durations")
	}
	triggersAbove := rule.Operator == "gt" || rule.Operator == "gte"
	recoversBelow := rule.Recovery.Operator == "lt" || rule.Recovery.Operator == "lte"
	if triggersAbove != recoversBelow || (triggersAbove && rule.Recovery.Value >= rule.Value) || (!triggersAbove && rule.Recovery.Value <= rule.Value) {
		return errors.New("recovery must use the opposite direction and a hysteresis margin")
	}
	return nil
}

func validateHTTPCheck(check HTTPCheck) error {
	endpoint, err := url.Parse(check.URL)
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.Fragment != "" {
		return errors.New("url must be absolute HTTP(S), without user info or fragment")
	}
	if check.Interval <= 0 || check.Timeout <= 0 || check.Timeout >= check.Interval {
		return errors.New("timeout and interval must be positive; timeout must be shorter than interval")
	}
	if check.Method != "GET" && check.Method != "HEAD" {
		return errors.New("method must be GET or HEAD")
	}
	if check.MaxBodyBytes <= 0 || check.MaxBodyBytes > MaxHTTPBodyLimitBytes {
		return errors.New("max_body_bytes must be between 1 and 16777216")
	}
	if check.Expectations.Status < 100 || check.Expectations.Status > 599 {
		return errors.New("expect.status must be between 100 and 599")
	}
	if check.AlertPolicy.Failures <= 0 || check.AlertPolicy.RecoverySuccesses <= 0 {
		return errors.New("alert counts must be positive")
	}
	if len(check.Expectations.Text) > 0 && len(check.Expectations.JSON) > 0 {
		return errors.New("expect must use either text or json assertions")
	}
	if check.Method == "HEAD" && (len(check.Expectations.Text) > 0 || len(check.Expectations.JSON) > 0) {
		return errors.New("HEAD cannot use body assertions")
	}
	seenHeaders := map[string]bool{}
	for name, header := range check.Headers {
		canonical := strings.ToLower(name)
		if seenHeaders[canonical] {
			return errors.New("duplicate case-insensitive request header name")
		}
		seenHeaders[canonical] = true
		if !validHeaderName(name) {
			return errors.New("invalid request header name")
		}
		if strings.EqualFold(name, "Host") || strings.EqualFold(name, "Content-Length") || strings.EqualFold(name, "Transfer-Encoding") {
			return errors.New("Host, Content-Length and Transfer-Encoding headers are unsupported")
		}
		if strings.ContainsAny(header.Value, "\r\n") || (header.EnvVar != "" && !environmentVariableName.MatchString(header.EnvVar)) {
			return errors.New("invalid request header value or environment reference")
		}
	}
	for _, assertion := range check.Expectations.Text {
		if assertion.Operator != "eq" && assertion.Operator != "contains" {
			return errors.New("text operator must be eq or contains")
		}
	}
	for _, assertion := range check.Expectations.JSON {
		if err := validateJSONAssertion(assertion); err != nil {
			return err
		}
	}
	return nil
}

func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, character := range name {
		if character > 127 || !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", character)) {
			return false
		}
	}
	return true
}

func validateJSONAssertion(assertion JSONAssertion) error {
	if assertion.Pointer != "" && !strings.HasPrefix(assertion.Pointer, "/") {
		return errors.New("JSON pointer must be empty or start with /")
	}
	for i := 0; i < len(assertion.Pointer); i++ {
		if assertion.Pointer[i] == '~' {
			if i+1 >= len(assertion.Pointer) || assertion.Pointer[i+1] != '0' && assertion.Pointer[i+1] != '1' {
				return errors.New("JSON pointer has invalid ~ escape")
			}
			i++
		}
	}
	value := assertion.Value
	if value.Kind != yaml.ScalarNode {
		return errors.New("JSON assertion value must be present and scalar")
	}
	if assertion.Operator == "exists" {
		if value.Tag != "!!bool" {
			return errors.New("exists requires a boolean value")
		}
		return nil
	}
	if assertion.Operator != "eq" && assertion.Operator != "ne" && !isNumericOperator(assertion.Operator) {
		return errors.New("unsupported JSON operator")
	}
	switch value.Tag {
	case "!!str", "!!bool", "!!null":
		if isNumericOperator(assertion.Operator) {
			return errors.New("numeric JSON operator requires a number")
		}
	case "!!int", "!!float":
		if value.Tag == "!!int" && !decimalIntegerPattern.MatchString(value.Value) {
			return errors.New("JSON assertion integer must use decimal integer notation")
		}
		if len(value.Value) > 256 || !decimalNumberPattern.MatchString(value.Value) {
			return errors.New("JSON assertion number must use finite decimal notation (at most 256 characters)")
		}
		if i := strings.IndexAny(value.Value, "eE"); i >= 0 {
			exponent, err := strconv.Atoi(value.Value[i+1:])
			if err != nil || exponent < -1000 || exponent > 1000 {
				return errors.New("JSON assertion number exponent must be between -1000 and 1000")
			}
		}
		if _, ok := new(big.Rat).SetString(value.Value); !ok {
			return errors.New("invalid JSON assertion number")
		}
	default:
		return errors.New("JSON assertion requires string, boolean, null or decimal number")
	}
	return nil
}
