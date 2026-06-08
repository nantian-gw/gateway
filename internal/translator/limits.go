package translator

import (
	"fmt"
	"time"

	"github.com/nantian-gw/gateway/internal/ir"
)

type Options struct {
	Limits Limits
}

type Limits struct {
	MaxInputObjects            int
	MaxSnapshotObjects         int
	MaxSnapshotEndpoints       int
	DefaultConnectTimeout time.Duration
}

func normalizeLimits(limits Limits) Limits {
	return Limits{
		MaxInputObjects:            positiveIntOrZero(limits.MaxInputObjects),
		MaxSnapshotObjects:         positiveIntOrZero(limits.MaxSnapshotObjects),
		MaxSnapshotEndpoints:       positiveIntOrZero(limits.MaxSnapshotEndpoints),
		DefaultConnectTimeout:      defaultDurationIfZero(limits.DefaultConnectTimeout, 5*time.Second),
	}
}

func (l Limits) validateInputObjects(total int) error {
	return validateLimit("maxInputObjects", total, l.MaxInputObjects)
}

func (l Limits) validateSnapshot(snapshot *ir.Snapshot) error {
	if err := validateLimit("maxSnapshotObjects", snapshotObjectCount(snapshot), l.MaxSnapshotObjects); err != nil {
		return err
	}
	return validateLimit("maxSnapshotEndpoints", snapshotEndpointCount(snapshot), l.MaxSnapshotEndpoints)
}

func validateLimit(name string, actual, limit int) error {
	if limit > 0 && actual > limit {
		return fmt.Errorf("translator resource limit %s exceeded: got %d, limit %d", name, actual, limit)
	}
	return nil
}

func snapshotObjectCount(snapshot *ir.Snapshot) int {
	if snapshot == nil {
		return 0
	}
	return len(snapshot.Listeners) +
		len(snapshot.HTTPRoutes) +
		len(snapshot.GRPCRoutes) +
		len(snapshot.StreamRoutes) +
		len(snapshot.Backends) +
		len(snapshot.Secrets) +
		len(snapshot.Workloads)
}

func snapshotEndpointCount(snapshot *ir.Snapshot) int {
	if snapshot == nil {
		return 0
	}

	total := 0
	for _, backend := range snapshot.Backends {
		total += len(backend.Endpoints)
	}
	return total
}

func positiveIntOrZero(value int) int {
	if value > 0 {
		return value
	}
	return 0
}

func defaultDurationIfZero(value, def time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return def
}
