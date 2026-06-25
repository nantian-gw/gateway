package xds

import (
	"log/slog"
	"sync"

	"github.com/nantian-gw/gateway/internal/ir"
	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"
)

type snapshotProtoBuilder func(*ir.Snapshot, projectionProfile, *slog.Logger) *controlv1.ConfigSnapshot

type snapshotProtoCache struct {
	mu      sync.Mutex
	version string
	// Cached snapshots are treated as immutable after construction. Stream
	// handlers only hand them to gRPC for serialization and never mutate them.
	snapshots map[string]*controlv1.ConfigSnapshot
	build     snapshotProtoBuilder
}

func newSnapshotProtoCache(build snapshotProtoBuilder) *snapshotProtoCache {
	if build == nil {
		build = buildProjectedProtoSnapshot
	}
	return &snapshotProtoCache{build: build}
}

func (c *snapshotProtoCache) get(snapshot *ir.Snapshot, profile projectionProfile, logger *slog.Logger) *controlv1.ConfigSnapshot {
	if snapshot == nil {
		return &controlv1.ConfigSnapshot{}
	}

	c.mu.Lock()
	if c.version != snapshot.ID {
		c.version = snapshot.ID
		c.snapshots = nil
	}
	if c.snapshots == nil {
		c.snapshots = make(map[string]*controlv1.ConfigSnapshot, 32)
	}
	cached := c.snapshots[profile.projectionKey]
	if cached == nil {
		cached = c.build(snapshot, profile, logger)
		c.snapshots[profile.projectionKey] = cached
	}
	c.mu.Unlock()

	return cached
}
