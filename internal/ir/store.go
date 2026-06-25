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
}

type SnapshotStoreHooks struct {
	OnSubscriberQueueReplace func(version string, replaced int)
	BeforeFanout             func(version string, subscribers int)
	AfterFanout              func(version string, subscribers int)
}

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

	if err := snapshot.Normalize(); err != nil {
		s.logger.Error("failed to normalize snapshot", "error", err)
		return false
	}

	s.mu.Lock()
	if s.current != nil && s.current.ID == snapshot.ID {
		s.mu.Unlock()
		return false
	}

	s.current = snapshot
	subscribers := s.snapshotSubscribersLocked()
	hooks := s.hooks
	s.mu.Unlock()

	if hooks.BeforeFanout != nil {
		hooks.BeforeFanout(snapshot.ID, len(subscribers))
	}

	replaced := 0
	for _, subscriber := range subscribers {
		if wasReplaced, delivered := pushLatestSnapshotSafe(subscriber, snapshot); delivered && wasReplaced {
			replaced++
		}
	}

	if hooks.AfterFanout != nil {
		hooks.AfterFanout(snapshot.ID, len(subscribers))
	}

	if replaced > 0 {
		s.logger.Warn(
			"coalesced snapshot fanout for slow subscribers",
			"version",
			snapshot.ID,
			"slow_subscribers",
			replaced,
		)
		if hooks.OnSubscriberQueueReplace != nil {
			hooks.OnSubscriberQueueReplace(snapshot.ID, replaced)
		}
	}

	s.logger.Info("published snapshot", "version", snapshot.ID)
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
			close(current.ch)
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

func pushLatestSnapshotSafe(sub *snapshotSubscriber, snapshot *Snapshot) (replaced bool, delivered bool) {
	if sub == nil || sub.closed.Load() {
		return false, false
	}

	delivered = true
	defer func() {
		if recover() != nil {
			replaced = false
			delivered = false
		}
	}()

	replaced = pushLatestSnapshot(sub.ch, snapshot)
	return replaced, delivered
}

func pushLatestSnapshot(ch chan *Snapshot, snapshot *Snapshot) bool {
	select {
	case ch <- snapshot:
		return false
	default:
	}

	for {
		select {
		case <-ch:
		default:
			select {
			case ch <- snapshot:
			default:
			}
			return true
		}
	}
}
