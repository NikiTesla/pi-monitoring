//go:build !linux

package host

import (
	"slices"
	"time"
)

type Collector struct{ mounts []string }

func New(mounts []string) *Collector {
	return &Collector{mounts: slices.Clone(mounts)}
}

func (c *Collector) Collect() Snapshot {
	snapshot := Snapshot{ObservedAt: time.Now()}
	for _, mount := range c.mounts {
		snapshot.Disks = append(snapshot.Disks, Disk{Mount: mount})
	}
	return snapshot
}
