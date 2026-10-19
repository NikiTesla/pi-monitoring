package monitor

import (
	"strings"
	"testing"
	"time"

	"github.com/NikiTesla/pi-monitoring/internal/config"
	"github.com/NikiTesla/pi-monitoring/internal/host"
	"github.com/NikiTesla/pi-monitoring/internal/httpcheck"
)

func TestCheckMessagesDistinguishHealthAndFreshness(t *testing.T) {
	now := time.Date(2026, 1, 2, 12, 30, 0, 0, time.UTC)
	for _, test := range []struct {
		name    string
		result  *httpcheck.Result
		unknown bool
		want    []string
		absent  string
	}{
		{name: "missing", want: []string{"⚪ api · Unavailable", "No observation yet."}, absent: "Healthy"},
		{name: "healthy", result: &httpcheck.Result{Success: true, StartedAt: now, ElapsedMS: 42}, want: []string{"🟢 api · Healthy", "Response time: 42 ms", "Observed: 02 Jan 2026 · 12:30:00 UTC"}},
		{name: "failed", result: &httpcheck.Result{StartedAt: now, Reason: "unexpected HTTP status"}, want: []string{"🔴 api · Failed", "Reason: unexpected HTTP status", "Consecutive failures: 2"}, absent: "Healthy"},
		{name: "stale success", result: &httpcheck.Result{Success: true, StartedAt: now.Add(-2 * time.Minute)}, want: []string{"🟠 api · Stale / unknown", "Last result: healthy"}, absent: "· Healthy"},
		{name: "unknown failure", result: &httpcheck.Result{StartedAt: now, Reason: "timeout"}, unknown: true, want: []string{"🟠 api · Stale / unknown", "Last result: failed", "Reason: timeout"}, absent: "· Failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := checkState{check: config.HTTPCheck{ID: "api", Interval: config.Duration(time.Minute), Timeout: config.Duration(time.Second)}, result: test.result, unknown: test.unknown, failures: 2}
			text := state.summary(now)
			for _, want := range test.want {
				if !strings.Contains(text, want) {
					t.Errorf("message %q lacks %q", text, want)
				}
			}
			if test.absent != "" && strings.Contains(text, test.absent) {
				t.Errorf("message %q misleadingly contains %q", text, test.absent)
			}
		})
	}
}

func TestHostMessagesKeepUnavailableAndStaleMetricsExplicit(t *testing.T) {
	now := time.Date(2026, 1, 2, 12, 30, 0, 0, time.UTC)
	r := runtime{settings: config.Config{Host: config.Host{Enabled: true, Interval: config.Duration(time.Minute)}}}
	if text := r.hostSummary(now); !strings.Contains(text, "No observation yet.") {
		t.Fatalf("missing observation: %q", text)
	}
	available := uint64(256 << 20)
	free := 25.0
	r.hostSnapshot = host.Snapshot{ObservedAt: now.Add(-3 * time.Minute), MemoryAvailableBytes: &available, MemoryTotalBytes: 1 << 30, Disks: []host.Disk{{Mount: "/", FreePercent: &free}, {Mount: "/missing"}}}
	text := r.hostSummary(now)
	for _, want := range []string{"Stale / unknown", "CPU: unavailable", "Temperature: unavailable", "Memory used: 768 / 1024 MiB", "Disk /: 75.0% used", "Disk /missing: unavailable", "Observed: 02 Jan 2026 · 12:27:00 UTC"} {
		if !strings.Contains(text, want) {
			t.Errorf("message %q lacks %q", text, want)
		}
	}
}

func TestHostMemoryUsageUnavailable(t *testing.T) {
	now := time.Date(2026, 1, 2, 12, 30, 0, 0, time.UTC)
	invalid := uint64(2 << 30)
	for _, available := range []*uint64{nil, &invalid} {
		r := runtime{
			settings:     config.Config{Host: config.Host{Enabled: true, Interval: config.Duration(time.Minute)}},
			hostSnapshot: host.Snapshot{ObservedAt: now, MemoryTotalBytes: 1 << 30, MemoryAvailableBytes: available},
		}
		if text := r.hostSummary(now); !strings.Contains(text, "Memory used: unavailable") {
			t.Errorf("missing or invalid memory observation: %q", text)
		}
	}
}

func TestIncidentMessagesPreserveOpenIncidentAfterSuccessfulSample(t *testing.T) {
	now := time.Date(2026, 1, 2, 12, 30, 0, 0, time.UTC)
	r := runtime{
		checks: []*checkState{{check: config.HTTPCheck{ID: "api", Interval: config.Duration(time.Minute)}, incident: true, opened: now.Add(-time.Minute), result: &httpcheck.Result{Success: true, StartedAt: now}}},
		hostIncidents: map[string]host.Transition{
			"z-temperature": {ID: "z-temperature", Metric: "host.temperature.celsius", Value: 75, At: now},
			"a-memory":      {ID: "a-memory", Metric: "host.memory.available_bytes", Value: 256 << 20, At: now},
		},
	}
	text := r.incidentsSummary(now)
	for _, want := range []string{"Active incidents · 3", "HTTP · api", "Opened: 02 Jan 2026 · 12:29:00 UTC", "Value: 256 MiB", "Value: 75.0 °C"} {
		if !strings.Contains(text, want) {
			t.Errorf("message %q lacks %q", text, want)
		}
	}
	if strings.Index(text, "a-memory") > strings.Index(text, "z-temperature") {
		t.Fatalf("host incidents are not sorted: %q", text)
	}
	r.checks[0].incident = false
	r.hostIncidents = nil
	if text := r.incidentsSummary(now); !strings.Contains(text, "Missing or stale observations do not confirm health.") {
		t.Fatalf("empty incidents imply health: %q", text)
	}
}

func TestCommandMessagesUseReadableSectionsAndLiteralText(t *testing.T) {
	r := runtime{settings: config.Config{Server: config.Server{Name: "Example <server> & monitor"}}, mailboxes: map[int64]*mailbox{11: newMailbox(11)}}
	for _, test := range []struct {
		command string
		want    []string
	}{
		{"status", []string{"🖥 Example <server> & monitor\n\n💻 Host", "\n\n🌐 HTTP checks", "\n\n📋 Incidents"}},
		{"help", []string{"📖 Monitor commands\n\n", "/check <id>", "Example: /check example-api"}},
		{"check", []string{"❔ Check not found\n", "/checks", "/check <id>"}},
		{"unknown", []string{"❔ Unknown command\n", "/help"}},
	} {
		r.handleCommand(command{chat: 11, name: test.command}, nil, time.Time{})
		text := r.mailboxes[11].pending["reply"].text
		for _, want := range test.want {
			if !strings.Contains(text, want) {
				t.Errorf("/%s message %q lacks %q", test.command, text, want)
			}
		}
	}
}
