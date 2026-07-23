package xds

import (
	"context"
	"log/slog"
	"sync"

	"golang.org/x/sync/singleflight"

	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"

	"github.com/nantian-gw/gateway/internal/ir"
)

type snapshotProtoBuilder func(context.Context, *ir.Snapshot, projectionProfile, *slog.Logger) *controlv1.ConfigSnapshot

type snapshotProtoCache struct {
	mu      sync.RWMutex
	version string
	// Cached snapshots are treated as immutable after construction. Stream
	// handlers only hand them to gRPC for serialization and never mutate them.
	snapshots map[string]*controlv1.ConfigSnapshot
	// group deduplicates concurrent builds of the same (version, profile) so a
	// slow projection cannot serialize unrelated profiles or block cache hits.
	group singleflight.Group
	build snapshotProtoBuilder
}

func newSnapshotProtoCache(build snapshotProtoBuilder) *snapshotProtoCache {
	if build == nil {
		build = buildProjectedProtoSnapshot
	}
	return &snapshotProtoCache{build: build}
}

func (c *snapshotProtoCache) get(ctx context.Context, snapshot *ir.Snapshot, profile projectionProfile, logger *slog.Logger) *controlv1.ConfigSnapshot {
	if snapshot == nil {
		return &controlv1.ConfigSnapshot{}
	}

	// Fast path: a read-locked cache hit for the current version lets many
	// streams read concurrently without serializing on a single mutex.
	if cached := c.lookup(snapshot.ID, profile.projectionKey); cached != nil {
		return cached
	}

	// Miss (or version change): build outside the cache lock. singleflight keys
	// on (version, profile) so identical requests collapse into one build while
	// distinct profiles build in parallel.
	key := snapshot.ID + "\x00" + profile.projectionKey
	result, _, _ := c.group.Do(key, func() (interface{}, error) {
		// Re-check under the lock in case another goroutine populated the cache
		// while this call was waiting to enter the singleflight.
		if cached := c.lookup(snapshot.ID, profile.projectionKey); cached != nil {
			return cached, nil
		}

		built := c.build(ctx, snapshot, profile, logger)

		c.mu.Lock()
		if c.version != snapshot.ID || c.snapshots == nil {
			c.version = snapshot.ID
			c.snapshots = make(map[string]*controlv1.ConfigSnapshot, 32)
		}
		c.snapshots[profile.projectionKey] = built
		c.mu.Unlock()

		return built, nil
	})

	return result.(*controlv1.ConfigSnapshot)
}

func (c *snapshotProtoCache) lookup(version, projectionKey string) *controlv1.ConfigSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.version != version || c.snapshots == nil {
		return nil
	}
	return c.snapshots[projectionKey]
}
