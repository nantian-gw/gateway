package grpcserver

import (
	"log/slog"
	"sync"

	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"
	"github.com/nantian-gw/gateway/internal/ir"
)

type snapshotProtoBuilder func(*ir.Snapshot, *slog.Logger) *controlv1.ConfigSnapshot

type snapshotProtoCache struct {
	mu      sync.Mutex
	version string
	// Cached snapshots are treated as immutable after construction. Stream
	// handlers only hand them to gRPC for serialization and never mutate them.
	snapshot *controlv1.ConfigSnapshot
	build    snapshotProtoBuilder
}

func newSnapshotProtoCache(build snapshotProtoBuilder) *snapshotProtoCache {
	if build == nil {
		build = toProtoSnapshotWithLogger
	}
	return &snapshotProtoCache{build: build}
}

func (c *snapshotProtoCache) get(snapshot *ir.Snapshot, logger *slog.Logger) *controlv1.ConfigSnapshot {
	if snapshot == nil {
		return &controlv1.ConfigSnapshot{}
	}

	c.mu.Lock()
	cached := c.snapshot
	if cached == nil || c.version != snapshot.ID {
		cached = c.build(snapshot, logger)
		c.version = snapshot.ID
		c.snapshot = cached
	}
	c.mu.Unlock()

	return cached
}
