// Package host collects current host metrics. A nil measurement means unavailable.
package host

import "time"

type Snapshot struct {
	ObservedAt           time.Time
	CPUPercent           *float64
	MemoryTotalBytes     uint64
	MemoryAvailableBytes *uint64
	Load1                *float64
	Disks                []Disk
	TemperatureCelsius   *float64
}

type Disk struct {
	Mount       string
	TotalBytes  uint64
	FreeBytes   *uint64
	FreePercent *float64
}
