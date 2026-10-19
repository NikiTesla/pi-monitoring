package monitor

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/NikiTesla/pi-monitoring/internal/host"
)

func observationTime(at time.Time) string {
	return at.UTC().Format("02 Jan 2006 · 15:04:05 UTC")
}

func (s *checkState) summary(now time.Time) string {
	if s.result == nil {
		return "⚪ " + s.check.ID + " · Unavailable\nNo observation yet."
	}
	status := "🟢 " + s.check.ID + " · Healthy"
	if !s.result.Success {
		status = "🔴 " + s.check.ID + " · Failed"
	}
	if s.unknown || now.Sub(s.result.StartedAt) > s.check.Interval.Value()+s.check.Timeout.Value() {
		status = "🟠 " + s.check.ID + " · Stale / unknown"
		if s.result.Success {
			status += "\nLast result: healthy"
		} else {
			status += "\nLast result: failed"
		}
	}
	if !s.result.Success {
		status += "\nReason: " + s.result.Reason
	}
	return status + fmt.Sprintf("\nResponse time: %d ms\nConsecutive failures: %d\nObserved: %s", s.result.ElapsedMS, s.failures, observationTime(s.result.StartedAt))
}

func (r *runtime) checksSummary(now time.Time) string {
	if len(r.checks) == 0 {
		return "🌐 HTTP checks\nNo checks configured."
	}
	lines := []string{"🌐 HTTP checks"}
	for _, state := range r.checks {
		lines = append(lines, state.summary(now))
	}
	return strings.Join(lines, "\n\n")
}

func (r *runtime) incidentsSummary(now time.Time) string {
	var lines []string
	for _, state := range r.checks {
		if state.incident {
			lines = append(lines, "HTTP · "+state.check.ID+"\nOpened: "+observationTime(state.opened)+"\n"+state.summary(now))
		}
	}
	// Stable ordering keeps repeated status requests easy to compare.
	for _, id := range slices.Sorted(maps.Keys(r.hostIncidents)) {
		lines = append(lines, "Host · "+hostIncidentDetails(r.hostIncidents[id]))
	}
	if len(lines) == 0 {
		return "📋 Incidents\nNo active incidents.\nMissing or stale observations do not confirm health."
	}
	return fmt.Sprintf("🔴 Active incidents · %d\n\n", len(lines)) + strings.Join(lines, "\n\n")
}

func hostIncidentDetails(incident host.Transition) string {
	metric := incident.Metric
	value := fmt.Sprintf("%.1f", incident.Value)
	switch incident.Metric {
	case "host.memory.available_bytes", "host.disk.free_bytes":
		metric = "Memory available"
		if incident.Metric == "host.disk.free_bytes" {
			metric = "Disk space free"
		}
		value = fmt.Sprintf("%.0f MiB", incident.Value/(1<<20))
	case "host.cpu.usage_percent", "host.disk.free_percent":
		metric = "CPU usage"
		if incident.Metric == "host.disk.free_percent" {
			metric = "Disk space free"
		}
		value += "%"
	case "host.temperature.celsius":
		metric = "Temperature"
		value += " °C"
	}
	text := incident.ID + "\nMetric: " + metric + "\nValue: " + value
	if incident.Mount != "" {
		text += "\nMount: " + incident.Mount
	}
	// Ongoing incidents retain the value observed when they opened.
	return text + "\nObserved: " + observationTime(incident.At)
}

func (r *runtime) hostSummary(now time.Time) string {
	if !r.settings.Host.Enabled {
		return "💻 Host\nMonitoring disabled."
	}
	s := r.hostSnapshot
	if s.ObservedAt.IsZero() {
		return "💻 Host\n⚪ Metrics unavailable · No observation yet."
	}
	var status strings.Builder
	status.WriteString("💻 Host")
	if now.Sub(s.ObservedAt) > 2*r.settings.Host.Interval.Value() {
		status.WriteString("\n🟠 Stale / unknown · Showing last observation")
	}
	number := func(value *float64, unit string) string {
		if value == nil {
			return "unavailable"
		}
		return fmt.Sprintf("%.1f%s", *value, unit)
	}
	status.WriteString("\nCPU: " + number(s.CPUPercent, "%"))
	status.WriteString("\nLoad (1 min): " + number(s.Load1, ""))
	status.WriteString("\nTemperature: " + number(s.TemperatureCelsius, " °C"))
	if s.MemoryAvailableBytes == nil || *s.MemoryAvailableBytes > s.MemoryTotalBytes {
		status.WriteString("\nMemory used: unavailable")
	} else {
		used := s.MemoryTotalBytes - *s.MemoryAvailableBytes
		fmt.Fprintf(&status, "\nMemory used: %d / %d MiB", used/(1<<20), s.MemoryTotalBytes/(1<<20))
	}
	for _, disk := range s.Disks {
		status.WriteString("\nDisk " + disk.Mount + ": ")
		if disk.FreePercent == nil {
			status.WriteString("unavailable")
		} else {
			fmt.Fprintf(&status, "%.1f%% used", 100-*disk.FreePercent)
		}
	}
	status.WriteString("\nObserved: " + observationTime(s.ObservedAt))
	return status.String()
}
