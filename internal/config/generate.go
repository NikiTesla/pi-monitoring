package config

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// GenerationOptions controls the initial configuration; it starts no runtime features.
type GenerationOptions struct {
	DockerEnabled    bool
	DashboardEnabled bool
	SystemPaths      bool
}

// Generate produces a complete editable configuration, with inactive examples.
func Generate(options GenerationOptions) ([]byte, error) {
	settings := Defaults()
	settings.Docker.Enabled = options.DockerEnabled
	settings.Dashboard.Enabled = options.DashboardEnabled
	if options.SystemPaths {
		settings.Storage.Path = "/var/lib/pi-monitor/monitor.db"
	}
	settings.HostRules = []HostRule{
		{
			ID: "low-memory", Metric: "host.memory.available_bytes",
			Operator: "lt", Value: 500 << 20, For: Duration(5 * time.Minute),
			Recovery: RecoveryCondition{Operator: "gte", Value: 600 << 20, For: Duration(2 * time.Minute)},
		},
		{
			ID: "low-disk-space", Metric: "host.disk.free_percent",
			Operator: "lt", Value: 15, For: Duration(5 * time.Minute),
			Recovery: RecoveryCondition{Operator: "gte", Value: 20, For: Duration(2 * time.Minute)},
		},
		{
			ID: "high-cpu", Metric: "host.cpu.usage_percent",
			Operator: "gt", Value: 90, For: Duration(10 * time.Minute),
			Recovery: RecoveryCondition{Operator: "lt", Value: 80, For: Duration(2 * time.Minute)},
		},
		{
			ID: "high-temperature", Metric: "host.temperature.celsius",
			Operator: "gt", Value: 75, For: Duration(5 * time.Minute),
			Recovery: RecoveryCondition{Operator: "lt", Value: 70, For: Duration(2 * time.Minute)},
		},
	}
	if err := Validate(settings); err != nil {
		return nil, err
	}
	var out strings.Builder
	out.WriteString("# pi-monitor configuration, schema version 1.\n# Run supports HTTP checks, Linux host metrics and Telegram alerts/commands.\n# Docker, dashboard, storage and heartbeats remain future settings.\n# Supply credentials in the environment or private secrets.env beside this file.\n# Run and check load secrets.env automatically; never put tokens in YAML.\n")
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(settings); err != nil {
		return nil, fmt.Errorf("encode configuration: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	out.WriteString(`
# Optional HTTP check: replace http_checks: [] above with a list like this.
# http_checks:
#   - id: "example-api"
#     url: "http://localhost:9000/health"
#     interval: "30s"
#     timeout: "5s"
#     expect:
#       status: 200
#       json:
#         - pointer: "/status"
#           operator: "eq"
#           value: "healthy"
#     alert:
#       failures: 3
#       recovery_successes: 2
#
# Future heartbeat configuration (not yet implemented):
# heartbeats:
#   - id: "daily-utility"
#     max_age: "25h"
#     initial_grace: "25h"
#     token_env: "PI_MONITOR_DAILY_UTILITY_TOKEN"
#
# Enable Telegram after filling allowed_chat_ids, allowed_user_ids and alert_chat_ids.
# token_env is the variable NAME, not the token. Set its value in the environment
# or secrets.env beside this file (chmod 600 secrets.env).
# Keep telegram.allow_reboot false unless host reboot access is deliberately set up;
# see docs/usage.md for the optional systemd/polkit setup and confirmation command.
# Docker socket access grants substantial host privileges; enable deliberately.
# Dashboard is not implemented; run does not open an HTTP listener.
`)
	return []byte(out.String()), nil
}
