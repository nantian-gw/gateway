package ir

import (
	"io"
	"log/slog"
	"testing"
	"time"
)

type subscribeResult struct {
	ch          <-chan *Snapshot
	unsubscribe func()
}

func TestSnapshotStoreKeepsLatestSnapshotForSlowSubscribers(t *testing.T) {
	store := NewSnapshotStore(testSnapshotStoreLogger())
	sub, unsubscribe := store.Subscribe()
	defer unsubscribe()

	if !store.Publish(snapshotWithListener("listener-one")) {
		t.Fatal("expected first snapshot to publish")
	}
	if !store.Publish(snapshotWithListener("listener-two")) {
		t.Fatal("expected second snapshot to publish")
	}

	got := <-sub
	if len(got.Listeners) != 1 || got.Listeners[0].Name != "listener-two" {
		t.Fatalf("expected latest snapshot, got %+v", got.Listeners)
	}
}

func TestSnapshotStoreSeedsNewSubscribersWithCurrentSnapshot(t *testing.T) {
	store := NewSnapshotStore(testSnapshotStoreLogger())
	if !store.Publish(snapshotWithListener("listener-current")) {
		t.Fatal("expected snapshot to publish")
	}

	sub, unsubscribe := store.Subscribe()
	defer unsubscribe()

	got := <-sub
	if len(got.Listeners) != 1 || got.Listeners[0].Name != "listener-current" {
		t.Fatalf("expected current snapshot, got %+v", got.Listeners)
	}
}

func TestSnapshotStoreReportsSlowSubscriberQueueReplacements(t *testing.T) {
	store := NewSnapshotStore(testSnapshotStoreLogger())
	subOne, unsubscribeOne := store.Subscribe()
	defer unsubscribeOne()
	subTwo, unsubscribeTwo := store.Subscribe()
	defer unsubscribeTwo()

	var gotVersion string
	var gotReplaced int
	store.SetHooks(SnapshotStoreHooks{
		OnSubscriberQueueReplace: func(version string, replaced int) {
			gotVersion = version
			gotReplaced = replaced
		},
	})

	first := snapshotWithListener("listener-one")
	if !store.Publish(first) {
		t.Fatal("expected first snapshot to publish")
	}
	second := snapshotWithListener("listener-two")
	if !store.Publish(second) {
		t.Fatal("expected second snapshot to publish")
	}

	if gotVersion != second.ID {
		t.Fatalf("replacement callback version = %q, want %q", gotVersion, second.ID)
	}
	if gotReplaced != 2 {
		t.Fatalf("replacement callback count = %d, want 2", gotReplaced)
	}

	got := <-subOne
	if len(got.Listeners) != 1 || got.Listeners[0].Name != "listener-two" {
		t.Fatalf("expected first subscriber to receive latest snapshot, got %+v", got.Listeners)
	}
	got = <-subTwo
	if len(got.Listeners) != 1 || got.Listeners[0].Name != "listener-two" {
		t.Fatalf("expected second subscriber to receive latest snapshot, got %+v", got.Listeners)
	}
}

func TestSnapshotStorePublishReleasesLockBeforeFanout(t *testing.T) {
	store := NewSnapshotStore(testSnapshotStoreLogger())
	sub, unsubscribe := store.Subscribe()
	defer unsubscribe()

	if !store.Publish(snapshotWithListener("listener-initial")) {
		t.Fatal("expected initial snapshot to publish")
	}
	<-sub

	beforeFanout := make(chan struct{})
	releaseFanout := make(chan struct{})
	store.SetHooks(SnapshotStoreHooks{
		BeforeFanout: func(version string, subscribers int) {
			if version == "" {
				t.Error("expected normalized snapshot version")
			}
			if subscribers != 1 {
				t.Errorf("subscribers = %d, want 1", subscribers)
			}
			close(beforeFanout)
			<-releaseFanout
		},
	})

	publishDone := make(chan bool, 1)
	go func() {
		publishDone <- store.Publish(snapshotWithListener("listener-next"))
	}()

	select {
	case <-beforeFanout:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("publish did not reach fanout")
	}

	currentDone := make(chan *Snapshot, 1)
	go func() {
		currentDone <- store.Current()
	}()

	var current *Snapshot
	select {
	case current = <-currentDone:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Current blocked behind publish fanout")
	}
	if len(current.Listeners) != 1 || current.Listeners[0].Name != "listener-next" {
		t.Fatalf("expected current snapshot to update before fanout, got %+v", current.Listeners)
	}

	subscribeDone := make(chan subscribeResult, 1)
	go func() {
		ch, unsubscribe := store.Subscribe()
		subscribeDone <- subscribeResult{ch: ch, unsubscribe: unsubscribe}
	}()

	var seeded subscribeResult
	select {
	case seeded = <-subscribeDone:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Subscribe blocked behind publish fanout")
	}
	defer seeded.unsubscribe()

	select {
	case got := <-seeded.ch:
		if len(got.Listeners) != 1 || got.Listeners[0].Name != "listener-next" {
			t.Fatalf("expected seeded subscriber to observe latest snapshot, got %+v", got.Listeners)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("seeded subscriber did not receive current snapshot")
	}

	close(releaseFanout)
	select {
	case ok := <-publishDone:
		if !ok {
			t.Fatal("expected publish to succeed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("publish did not complete after fanout release")
	}
}

func TestSnapshotStorePublishIgnoresSubscriberClosedDuringFanout(t *testing.T) {
	store := NewSnapshotStore(testSnapshotStoreLogger())
	sub, unsubscribe := store.Subscribe()

	if !store.Publish(snapshotWithListener("listener-initial")) {
		t.Fatal("expected initial snapshot to publish")
	}
	<-sub

	beforeFanout := make(chan struct{})
	releaseFanout := make(chan struct{})
	store.SetHooks(SnapshotStoreHooks{
		BeforeFanout: func(version string, subscribers int) {
			if version == "" {
				t.Error("expected normalized snapshot version")
			}
			if subscribers != 1 {
				t.Errorf("subscribers = %d, want 1", subscribers)
			}
			close(beforeFanout)
			<-releaseFanout
		},
	})

	publishDone := make(chan bool, 1)
	go func() {
		publishDone <- store.Publish(snapshotWithListener("listener-next"))
	}()

	select {
	case <-beforeFanout:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("publish did not reach fanout")
	}

	unsubscribe()
	close(releaseFanout)

	select {
	case ok := <-publishDone:
		if !ok {
			t.Fatal("expected publish to succeed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("publish did not complete after unsubscribe")
	}
}

func snapshotWithListener(name string) *Snapshot {
	return &Snapshot{
		Listeners: []Listener{
			{
				Name:     name,
				Address:  "0.0.0.0",
				Port:     80,
				Protocol: "HTTP",
			},
		},
	}
}

func testSnapshotStoreLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
