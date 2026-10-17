//go:build !linux

package host

import "time"

type Collector struct{ mounts []string }

func New(mounts []string) *Collector {
	return &Collector{mounts: append([]string(nil), mounts...)}
}

func (c *Collector) Collect() Snapshot {
	snapshot := Snapshot{ObservedAt: time.Now()}
	for _, mount := range c.mounts {
		snapshot.Disks = append(snapshot.Disks, Disk{Mount: mount})
	}
	return snapshot
}
