package host

import (
	"math"
	"time"

	"pi-monitoring/internal/config"
)

type Transition struct {
	ID     string
	Metric string
	Mount  string
	Open   bool
	At     time.Time
	Value  float64
}

type ruleState struct {
	open         bool
	pendingSince time.Time
	lastObserved time.Time
}

// RuleSet evaluates sequential snapshots; callers own notification and persistence.
type RuleSet struct {
	rules  []config.HostRule
	states map[string]ruleState
}

func NewRules(rules []config.HostRule) *RuleSet {
	return &RuleSet{rules: append([]config.HostRule(nil), rules...), states: make(map[string]ruleState)}
}

func (r *RuleSet) Evaluate(snapshot Snapshot) []Transition {
	var transitions []Transition
	for _, rule := range r.rules {
		if rule.Metric == "host.disk.free_bytes" || rule.Metric == "host.disk.free_percent" {
			seen := make(map[string]bool)
			for _, disk := range snapshot.Disks {
				id := rule.ID + ":" + disk.Mount
				if seen[id] {
					continue
				}
				seen[id] = true
				value := disk.FreePercent
				if rule.Metric == "host.disk.free_bytes" {
					value = bytesValue(disk.FreeBytes)
				}
				if transition := r.evaluate(rule, id, disk.Mount, snapshot.ObservedAt, value); transition != nil {
					transitions = append(transitions, *transition)
				}
			}
			// A disappeared mount is unknown, rather than continuous failure/recovery.
			for id, state := range r.states {
				if len(id) > len(rule.ID) && id[:len(rule.ID)+1] == rule.ID+":" && !seen[id] {
					state.pendingSince = time.Time{}
					r.states[id] = state
				}
			}
			continue
		}
		var value *float64
		switch rule.Metric {
		case "host.memory.available_bytes":
			value = bytesValue(snapshot.MemoryAvailableBytes)
		case "host.cpu.usage_percent":
			value = snapshot.CPUPercent
		case "host.temperature.celsius":
			value = snapshot.TemperatureCelsius
		}
		if transition := r.evaluate(rule, rule.ID, "", snapshot.ObservedAt, value); transition != nil {
			transitions = append(transitions, *transition)
		}
	}
	return transitions
}

func bytesValue(value *uint64) *float64 {
	if value == nil {
		return nil
	}
	converted := float64(*value)
	return &converted
}

func (r *RuleSet) evaluate(rule config.HostRule, id, mount string, observedAt time.Time, value *float64) *Transition {
	state := r.states[id]
	if observedAt.IsZero() || (!state.lastObserved.IsZero() && !observedAt.After(state.lastObserved)) {
		state.pendingSince = time.Time{}
		r.states[id] = state
		return nil
	}
	state.lastObserved = observedAt
	if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) {
		state.pendingSince = time.Time{}
		r.states[id] = state
		return nil
	}
	operator, threshold, duration := rule.Operator, rule.Value, rule.For.Value()
	if state.open {
		operator, threshold, duration = rule.Recovery.Operator, rule.Recovery.Value, rule.Recovery.For.Value()
	}
	if !compare(*value, operator, threshold) {
		state.pendingSince = time.Time{}
		r.states[id] = state
		return nil
	}
	if state.pendingSince.IsZero() {
		state.pendingSince = observedAt
	}
	if observedAt.Sub(state.pendingSince) < duration {
		r.states[id] = state
		return nil
	}
	state.open = !state.open
	state.pendingSince = time.Time{}
	r.states[id] = state
	return &Transition{ID: id, Metric: rule.Metric, Mount: mount, Open: state.open, At: observedAt, Value: *value}
}

func compare(value float64, operator string, threshold float64) bool {
	switch operator {
	case "gt":
		return value > threshold
	case "gte":
		return value >= threshold
	case "lt":
		return value < threshold
	case "lte":
		return value <= threshold
	default:
		return false
	}
}
