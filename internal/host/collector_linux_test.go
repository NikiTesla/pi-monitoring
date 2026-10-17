//go:build linux

package host

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func fixtureCollector(t *testing.T) *Collector {
	t.Helper()
	root := t.TempDir()
	collector := New([]string{"/", "/unavailable"})
	collector.procRoot = filepath.Join(root, "proc")
	collector.thermalRoot = filepath.Join(root, "thermal")
	collector.statfs = func(mount string, stats *syscall.Statfs_t) error {
		if mount == "/unavailable" {
			return errors.New("unavailable")
		}
		stats.Blocks, stats.Bavail, stats.Bsize = 1000, 250, 4096
		return nil
	}
	return collector
}

func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCollectMetricsAndUnavailableValues(t *testing.T) {
	c := fixtureCollector(t)
	writeFixture(t, filepath.Join(c.procRoot, "stat"), "cpu 100 0 100 800 0 0 0 0 20 0\n")
	writeFixture(t, filepath.Join(c.procRoot, "meminfo"), "MemTotal: 1000 kB\nMemAvailable: 400 kB\n")
	writeFixture(t, filepath.Join(c.procRoot, "loadavg"), "1.25 1.0 0.5 1/10 2\n")
	writeFixture(t, filepath.Join(c.thermalRoot, "thermal_zone0", "type"), "cpu-thermal\n")
	writeFixture(t, filepath.Join(c.thermalRoot, "thermal_zone0", "temp"), "42500\n")
	writeFixture(t, filepath.Join(c.thermalRoot, "thermal_zone1", "type"), "battery\n")
	writeFixture(t, filepath.Join(c.thermalRoot, "thermal_zone1", "temp"), "90000\n")
	first := c.Collect()
	if first.CPUPercent != nil {
		t.Fatal("CPU needs two samples")
	}
	if first.MemoryTotalBytes != 1000*1024 || first.MemoryAvailableBytes == nil || *first.MemoryAvailableBytes != 400*1024 {
		t.Fatalf("unexpected memory: %+v", first)
	}
	if first.Load1 == nil || *first.Load1 != 1.25 || first.TemperatureCelsius == nil || *first.TemperatureCelsius != 42.5 {
		t.Fatalf("unexpected load or temperature: %+v", first)
	}
	if len(first.Disks) != 2 || first.Disks[0].FreeBytes == nil || *first.Disks[0].FreeBytes != 250*4096 || first.Disks[0].FreePercent == nil || *first.Disks[0].FreePercent != 25 || first.Disks[1].FreeBytes != nil {
		t.Fatalf("unexpected disks: %+v", first.Disks)
	}
	writeFixture(t, filepath.Join(c.procRoot, "stat"), "cpu 150 0 100 850 0 0 0 0 50 0\n")
	second := c.Collect()
	if second.CPUPercent == nil || math.Abs(*second.CPUPercent-50) > 0.001 {
		t.Fatalf("CPU usage: %v", second.CPUPercent)
	}
}

func TestCounterResetMissingAndMalformedMetrics(t *testing.T) {
	c := fixtureCollector(t)
	missing := c.Collect()
	if missing.CPUPercent != nil || missing.MemoryAvailableBytes != nil || missing.Load1 != nil || missing.TemperatureCelsius != nil {
		t.Fatal("missing metrics must be unavailable")
	}
	writeFixture(t, filepath.Join(c.procRoot, "stat"), "cpu 100 0 100 800\n")
	c.Collect()
	writeFixture(t, filepath.Join(c.procRoot, "stat"), "cpu 1 0 1 8\n")
	writeFixture(t, filepath.Join(c.procRoot, "meminfo"), "MemTotal: 100 kB\nMemAvailable: 101 kB\n")
	writeFixture(t, filepath.Join(c.procRoot, "loadavg"), "NaN 0 0\n")
	writeFixture(t, filepath.Join(c.thermalRoot, "thermal_zone0", "type"), "cpu-thermal\n")
	writeFixture(t, filepath.Join(c.thermalRoot, "thermal_zone0", "temp"), "NaN\n")
	reset := c.Collect()
	if reset.CPUPercent != nil || reset.MemoryAvailableBytes != nil || reset.Load1 != nil || reset.TemperatureCelsius != nil {
		t.Fatal("invalid samples must be unavailable")
	}
	writeFixture(t, filepath.Join(c.procRoot, "stat"), "cpu 2 0 1 9\n")
	if after := c.Collect(); after.CPUPercent == nil || *after.CPUPercent != 50 {
		t.Fatal("CPU must recover after counter reset")
	}
	writeFixture(t, filepath.Join(c.procRoot, "stat"), "cpu invalid 0 1 9\n")
	if after := c.Collect(); after.CPUPercent != nil {
		t.Fatal("malformed CPU sample must be unavailable")
	}
	writeFixture(t, filepath.Join(c.procRoot, "stat"), "cpu 3 0 1 10\n")
	if after := c.Collect(); after.CPUPercent != nil {
		t.Fatal("malformed CPU sample must break rate continuity")
	}
}
