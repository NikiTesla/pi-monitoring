// Package monitor runs bounded, in-memory host and HTTP monitoring.
package monitor

import (
	"time"

	"pi-monitoring/internal/config"
	"pi-monitoring/internal/httpcheck"
)

type checkState struct {
	check               config.HTTPCheck
	next                time.Time
	running             bool
	result              *httpcheck.Result
	unknown             bool
	failures, successes int
	firstFailure        time.Time
	incident            bool
	opened              time.Time
	waitingChat         int64
}

func (s *checkState) missed() {
	s.unknown = true
	s.failures, s.successes = 0, 0
	s.firstFailure = time.Time{}
}

// Only completed evaluations advance streaks. Unknown observations cannot recover.
func (s *checkState) observe(result httpcheck.Result, now, graceEnd time.Time) string {
	if s.result != nil && result.StartedAt.Sub(s.result.StartedAt) > s.check.Interval.Value()+s.check.Timeout.Value() {
		s.missed()
	}
	s.result = &result
	s.running = false
	s.unknown = false
	if now.Sub(result.StartedAt) > s.check.Interval.Value()+s.check.Timeout.Value() {
		s.missed()
		return ""
	}
	if result.Category == "canceled" {
		s.missed()
		return ""
	}
	if result.Success {
		s.failures = 0
		s.firstFailure = time.Time{}
		s.successes++
		if s.incident && s.successes >= s.check.AlertPolicy.RecoverySuccesses {
			s.incident = false
			return "✅ Recovered · HTTP\n" + s.summary(now)
		}
		return ""
	}
	s.successes = 0
	if s.failures == 0 {
		s.firstFailure = now
	}
	s.failures++
	if !s.incident && !now.Before(graceEnd) && s.failures >= s.check.AlertPolicy.Failures {
		s.incident, s.opened = true, s.firstFailure
		return "🔴 Alert · HTTP\n" + s.summary(now) + "\nFirst failure: " + observationTime(s.firstFailure)
	}
	return ""
}
