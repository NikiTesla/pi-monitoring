//go:build linux

package host

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Collector is owned by one collection goroutine; CPU usage uses successive samples.
type Collector struct {
	mounts      []string
	procRoot    string
	thermalRoot string
	previousCPU []uint64
	statfs      func(string, *syscall.Statfs_t) error
}

func New(mounts []string) *Collector {
	return &Collector{mounts: append([]string(nil), mounts...), procRoot: "/proc", thermalRoot: "/sys/class/thermal", statfs: syscall.Statfs}
}

func (c *Collector) Collect() Snapshot {
	snapshot := Snapshot{ObservedAt: time.Now()}
	snapshot.CPUPercent = c.cpuUsage()
	snapshot.MemoryTotalBytes, snapshot.MemoryAvailableBytes = c.memory()
	if content, err := os.ReadFile(filepath.Join(c.procRoot, "loadavg")); err == nil {
		fields := strings.Fields(string(content))
		if len(fields) > 0 {
			if value, err := strconv.ParseFloat(fields[0], 64); err == nil && !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 {
				snapshot.Load1 = &value
			}
		}
	}
	for _, mount := range c.mounts {
		disk := Disk{Mount: mount}
		var stats syscall.Statfs_t
		if err := c.statfs(mount, &stats); err == nil && stats.Bsize > 0 && stats.Blocks > 0 && stats.Bavail <= stats.Blocks && stats.Blocks <= math.MaxUint64/uint64(stats.Bsize) {
			disk.TotalBytes = stats.Blocks * uint64(stats.Bsize)
			free := stats.Bavail * uint64(stats.Bsize)
			percent := 100 * float64(stats.Bavail) / float64(stats.Blocks)
			disk.FreeBytes, disk.FreePercent = &free, &percent
		}
		snapshot.Disks = append(snapshot.Disks, disk)
	}
	snapshot.TemperatureCelsius = c.temperature()
	return snapshot
}

func (c *Collector) cpuUsage() *float64 {
	content, err := os.ReadFile(filepath.Join(c.procRoot, "stat"))
	if err != nil {
		c.previousCPU = nil
		return nil
	}
	fields := strings.Fields(strings.SplitN(string(content), "\n", 2)[0])
	if len(fields) < 5 || fields[0] != "cpu" {
		c.previousCPU = nil
		return nil
	}
	// Guest counters are already included in user/nice, so only sum the first eight.
	count := min(len(fields)-1, 8)
	current := make([]uint64, count)
	for index := range current {
		value, err := strconv.ParseUint(fields[index+1], 10, 64)
		if err != nil {
			c.previousCPU = nil
			return nil
		}
		current[index] = value
	}
	previous := c.previousCPU
	c.previousCPU = current
	if len(previous) != len(current) {
		return nil
	}
	var total, idle float64
	for index, value := range current {
		if value < previous[index] {
			return nil
		}
		delta := float64(value - previous[index])
		total += delta
		if index == 3 || index == 4 {
			idle += delta
		}
	}
	if total == 0 {
		return nil
	}
	usage := 100 * (total - idle) / total
	return &usage
}

func (c *Collector) memory() (uint64, *uint64) {
	content, err := os.ReadFile(filepath.Join(c.procRoot, "meminfo"))
	if err != nil {
		return 0, nil
	}
	var total uint64
	var available *uint64
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 || fields[2] != "kB" || (fields[0] != "MemTotal:" && fields[0] != "MemAvailable:") {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil || value > math.MaxUint64/1024 {
			continue
		}
		value *= 1024
		if fields[0] == "MemTotal:" {
			total = value
		} else {
			available = &value
		}
	}
	if total == 0 || (available != nil && *available > total) {
		available = nil
	}
	return total, available
}

func (c *Collector) temperature() *float64 {
	zones, err := filepath.Glob(filepath.Join(c.thermalRoot, "thermal_zone*"))
	if err != nil {
		return nil
	}
	var hottest *float64
	for _, zone := range zones {
		kind, err := os.ReadFile(filepath.Join(zone, "type"))
		if err != nil {
			continue
		}
		// Restrict readings to CPU/SoC sensors; unrelated zones can measure batteries.
		sensor := strings.ToLower(strings.TrimSpace(string(kind)))
		if !strings.Contains(sensor, "cpu") && !strings.Contains(sensor, "soc") && sensor != "x86_pkg_temp" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(zone, "temp"))
		if err != nil {
			continue
		}
		millidegrees, err := strconv.ParseFloat(strings.TrimSpace(string(content)), 64)
		if err != nil || math.IsNaN(millidegrees) || math.IsInf(millidegrees, 0) || millidegrees < -40000 || millidegrees > 150000 {
			continue
		}
		value := millidegrees / 1000
		if hottest == nil || value > *hottest {
			hottest = &value
		}
	}
	return hottest
}
