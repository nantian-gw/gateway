package ir

import (
	"log/slog"
	"sync"
	"sync/atomic"
)

type SnapshotStore struct {
	logger      *slog.Logger
	mu          sync.RWMutex
	current     *Snapshot
	subscribers map[int]*snapshotSubscriber
	nextID      int
	hooks       SnapshotStoreHooks
}

const snapshotSubscriberBufferSize = 1

type snapshotSubscriber struct {
	ch     chan *Snapshot
	closed atomic.Bool
	mu     sync.Mutex
}

type SnapshotStoreHooks struct {
	OnSubscriberQueueReplace func(version string, replaced int)
	BeforeFanout             func(version string, subscribers int)
	AfterFanout              func(version string, subscribers int)
	OnPublish                func(version string, result string)
}

// PublishResult values reported through SnapshotStoreHooks.OnPublish.
const (
	PublishResultPublished = "published"
	PublishResultDedup     = "dedup"
)

func NewSnapshotStore(logger *slog.Logger) *SnapshotStore {
	return &SnapshotStore{
		logger:      logger,
		subscribers: make(map[int]*snapshotSubscriber, 8),
	}
}

func (s *SnapshotStore) SetHooks(hooks SnapshotStoreHooks) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hooks = hooks
}

func (s *SnapshotStore) Current() *Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.current
}

func (s *SnapshotStore) Publish(snapshot *Snapshot) bool {
	if snapshot == nil {
		return false
	}

	owned := snapshot.Clone()
	if err := owned.Normalize(); err != nil {
		s.logger.Error("failed to normalize snapshot", "error", err)
		return false
	}
	snapshot.ID = owned.ID
	snapshot.GeneratedAt = owned.GeneratedAt

	s.mu.Lock()
	hooks := s.hooks
	if s.current != nil && s.current.ID == owned.ID {
		s.mu.Unlock()
		if hooks.OnPublish != nil {
			hooks.OnPublish(owned.ID, PublishResultDedup)
		}
		return false
	}

	s.current = owned
	subscribers := s.snapshotSubscribersLocked()
	s.mu.Unlock()

	if hooks.BeforeFanout != nil {
		hooks.BeforeFanout(owned.ID, len(subscribers))
	}

	replaced := 0
	droppedIDs := make([]string, 0, len(subscribers))
	for _, subscriber := range subscribers {
		if dropped, wasReplaced, delivered := pushLatestSnapshotSafe(subscriber, owned); delivered && wasReplaced {
			replaced++
			if dropped != nil && dropped.ID != "" {
				droppedIDs = append(droppedIDs, dropped.ID)
			}
		}
	}

	if hooks.AfterFanout != nil {
		hooks.AfterFanout(owned.ID, len(subscribers))
	}

	if replaced > 0 {
		s.logger.Warn(
			"coalesced snapshot fanout for slow subscribers",
			"version",
			owned.ID,
			"slow_subscribers",
			replaced,
			"dropped_snapshots",
			droppedIDs,
		)
		if hooks.OnSubscriberQueueReplace != nil {
			hooks.OnSubscriberQueueReplace(owned.ID, replaced)
		}
	}

	if hooks.OnPublish != nil {
		hooks.OnPublish(owned.ID, PublishResultPublished)
	}

	s.logger.Info("published snapshot", "version", owned.ID)
	return true
}

func (s *SnapshotStore) Subscribe() (<-chan *Snapshot, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.nextID
	s.nextID++

	subscriber := &snapshotSubscriber{
		ch: make(chan *Snapshot, snapshotSubscriberBufferSize),
	}
	if s.current != nil {
		subscriber.ch <- s.current
	}
	s.subscribers[id] = subscriber

	return subscriber.ch, func() {
		s.mu.Lock()
		current, ok := s.subscribers[id]
		if ok {
			delete(s.subscribers, id)
		}
		s.mu.Unlock()

		if ok && current.closed.CompareAndSwap(false, true) {
			current.mu.Lock()
			close(current.ch)
			current.mu.Unlock()
		}
	}
}

func (s *SnapshotStore) snapshotSubscribersLocked() []*snapshotSubscriber {
	subscribers := make([]*snapshotSubscriber, 0, len(s.subscribers))
	for _, subscriber := range s.subscribers {
		subscribers = append(subscribers, subscriber)
	}
	return subscribers
}

func pushLatestSnapshotSafe(sub *snapshotSubscriber, snapshot *Snapshot) (dropped *Snapshot, replaced bool, delivered bool) {
	if sub == nil {
		return nil, false, false
	}

	sub.mu.Lock()
	defer sub.mu.Unlock()

	if sub.closed.Load() {
		return nil, false, false
	}

	dropped, replaced = pushLatestSnapshot(sub.ch, snapshot)
	return dropped, replaced, true
}

func pushLatestSnapshot(ch chan *Snapshot, snapshot *Snapshot) (dropped *Snapshot, replaced bool) {
	select {
	case ch <- snapshot:
		return nil, false
	default:
	}

	var firstDropped *Snapshot
	for {
		select {
		case dropped := <-ch:
			if firstDropped == nil {
				firstDropped = dropped
			}
		default:
			select {
			case ch <- snapshot:
			default:
			}
			return firstDropped, true
		}
	}
}
