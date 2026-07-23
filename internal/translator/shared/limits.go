package shared

import (
	"fmt"
	"time"

	"github.com/nantian-gw/gateway/internal/ir"
)

type Options struct {
	Limits Limits
}

type Limits struct {
	MaxInputObjects       int
	MaxSnapshotObjects    int
	MaxSnapshotEndpoints  int
	DefaultConnectTimeout time.Duration
}

const DefaultConnectTimeout = 5 * time.Second

func NormalizeLimits(limits Limits) Limits {
	return Limits{
		MaxInputObjects:       PositiveIntOrZero(limits.MaxInputObjects),
		MaxSnapshotObjects:    PositiveIntOrZero(limits.MaxSnapshotObjects),
		MaxSnapshotEndpoints:  PositiveIntOrZero(limits.MaxSnapshotEndpoints),
		DefaultConnectTimeout: DefaultDurationIfZero(limits.DefaultConnectTimeout, DefaultConnectTimeout),
	}
}

func (l Limits) ValidateInputObjects(total int) error {
	return ValidateLimit("maxInputObjects", total, l.MaxInputObjects)
}

func (l Limits) ValidateSnapshot(snapshot *ir.Snapshot) error {
	if err := ValidateLimit("maxSnapshotObjects", SnapshotObjectCount(snapshot), l.MaxSnapshotObjects); err != nil {
		return err
	}
	return ValidateLimit("maxSnapshotEndpoints", SnapshotEndpointCount(snapshot), l.MaxSnapshotEndpoints)
}

func ValidateLimit(name string, actual, limit int) error {
	if limit > 0 && actual > limit {
		return fmt.Errorf("translator resource limit %s exceeded: got %d, limit %d", name, actual, limit)
	}
	return nil
}

func SnapshotObjectCount(snapshot *ir.Snapshot) int {
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

func SnapshotEndpointCount(snapshot *ir.Snapshot) int {
	if snapshot == nil {
		return 0
	}

	total := 0
	for _, backend := range snapshot.Backends {
		total += len(backend.Endpoints)
	}
	return total
}

func PositiveIntOrZero(value int) int {
	if value > 0 {
		return value
	}
	return 0
}

func DefaultDurationIfZero(value, def time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return def
}
