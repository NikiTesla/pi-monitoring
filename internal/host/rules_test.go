package host

import (
	"testing"
	"time"

	"pi-monitoring/internal/config"
)

func TestRulesRequireContinuousFailureAndRecovery(t *testing.T) {
	rules := NewRules([]config.HostRule{{ID: "hot", Metric: "host.temperature.celsius", Operator: "gt", Value: 75, For: config.Duration(time.Minute), Recovery: config.RecoveryCondition{Operator: "lt", Value: 70, For: config.Duration(time.Minute)}}})
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		seconds    int
		value      *float64
		transition bool
		open       bool
	}{
		{0, number(80), false, false}, {30, nil, false, false},
		{60, number(80), false, false}, {90, number(80), false, false},
		{120, number(80), true, true}, {150, number(80), false, false},
		{180, number(65), false, false}, {210, number(72), false, false},
		{240, number(65), false, false}, {270, nil, false, false},
		{300, number(65), false, false}, {360, number(65), true, false},
	}
	for _, test := range tests {
		transitions := rules.Evaluate(Snapshot{ObservedAt: start.Add(time.Duration(test.seconds) * time.Second), TemperatureCelsius: test.value})
		if (len(transitions) != 0) != test.transition {
			t.Fatalf("at %ds transitions = %+v", test.seconds, transitions)
		}
		if test.transition && (len(transitions) != 1 || transitions[0].Open != test.open || transitions[0].ID != "hot") {
			t.Fatalf("at %ds unexpected transition %+v", test.seconds, transitions)
		}
	}
}

func TestDiskRulesKeepMountIncidentsIndependent(t *testing.T) {
	rules := NewRules([]config.HostRule{{ID: "space", Metric: "host.disk.free_percent", Operator: "lt", Value: 15, For: config.Duration(time.Minute), Recovery: config.RecoveryCondition{Operator: "gte", Value: 20, For: config.Duration(time.Minute)}}})
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	collect := func(seconds int, disks []Disk) []Transition {
		return rules.Evaluate(Snapshot{ObservedAt: start.Add(time.Duration(seconds) * time.Second), Disks: disks})
	}
	disks := []Disk{{Mount: "/", FreePercent: number(10)}, {Mount: "/data", FreePercent: number(50)}}
	collect(0, disks)
	opened := collect(60, disks)
	if len(opened) != 1 || opened[0].ID != "space:/" || !opened[0].Open {
		t.Fatalf("unexpected opening: %+v", opened)
	}
	collect(90, []Disk{{Mount: "/", FreePercent: number(25)}})
	collect(120, nil)
	if recovered := collect(150, []Disk{{Mount: "/", FreePercent: number(25)}}); len(recovered) != 0 {
		t.Fatalf("missing mount must break recovery: %+v", recovered)
	}
	recovered := collect(210, []Disk{{Mount: "/", FreePercent: number(25)}})
	if len(recovered) != 1 || recovered[0].Open {
		t.Fatalf("unexpected recovery: %+v", recovered)
	}
}

func TestRuleRejectsNonIncreasingObservationTimes(t *testing.T) {
	rules := NewRules([]config.HostRule{{ID: "cpu", Metric: "host.cpu.usage_percent", Operator: "gt", Value: 90, For: config.Duration(time.Minute), Recovery: config.RecoveryCondition{Operator: "lt", Value: 80, For: config.Duration(time.Minute)}}})
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, seconds := range []int{0, 30, 20, 60} {
		if transitions := rules.Evaluate(Snapshot{ObservedAt: start.Add(time.Duration(seconds) * time.Second), CPUPercent: number(95)}); len(transitions) != 0 {
			t.Fatalf("unexpected transition at %ds: %+v", seconds, transitions)
		}
	}
}

func number(value float64) *float64 { return &value }
