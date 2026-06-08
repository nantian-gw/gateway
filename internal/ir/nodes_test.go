package ir

import (
	"testing"
	"time"
)

func TestNodeStatusStoreTracksLifecycle(t *testing.T) {
	store := NewNodeStatusStore()
	now := time.Unix(1_700_000_000, 0).UTC()

	store.Connect("dp-1", "kind", []string{"*"}, now)
	store.ObservePublished("dp-1", "v1", now.Add(time.Second))
	store.ObserveAck("dp-1", "kind", "v1", "v1", []string{"*"}, now.Add(2*time.Second))
	store.ObserveReport("dp-1", "v1", true, "ready", now.Add(3*time.Second))
	store.Disconnect("dp-1", now.Add(4*time.Second))

	nodes := store.List()
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}

	node := nodes[0]
	if node.NodeID != "dp-1" {
		t.Fatalf("unexpected node id: %s", node.NodeID)
	}
	if node.Connected {
		t.Fatalf("expected disconnected node")
	}
	if node.LastSentVersion != "v1" || node.LastAckVersion != "v1" {
		t.Fatalf("unexpected versions: %+v", node)
	}
	if node.LastConfigStatus != "ACK" {
		t.Fatalf("expected ack status, got %+v", node)
	}
	if node.Ready {
		t.Fatalf("expected disconnect to clear readiness, got %+v", node)
	}
}

func TestNodeStatusStoreDisconnectWithMessageMarksNodeNotReady(t *testing.T) {
	store := NewNodeStatusStore()
	now := time.Unix(1_700_000_050, 0).UTC()

	store.Connect("dp-1", "kind", []string{"*"}, now)
	store.ObserveReport("dp-1", "v1", true, "ready", now.Add(time.Second))
	store.DisconnectWithMessage("dp-1", now.Add(2*time.Second), "xds stream drained for shutdown")

	node, ok := store.Get("dp-1")
	if !ok {
		t.Fatal("expected node to exist")
	}
	if node.Connected {
		t.Fatalf("expected disconnected node, got %+v", node)
	}
	if node.Ready {
		t.Fatalf("expected disconnect reason to clear readiness, got %+v", node)
	}
	if node.Message != "xds stream drained for shutdown" {
		t.Fatalf("expected disconnect message to be retained, got %+v", node)
	}
}

func TestNodeStatusStoreTracksStructuredDisconnectMetadataAndReconnectClearsIt(t *testing.T) {
	store := NewNodeStatusStore()
	now := time.Unix(1_700_000_075, 0).UTC()

	store.Connect("dp-1", "kind", []string{"*"}, now)
	store.ObserveAck("dp-1", "kind", "v1", "v1", []string{"*"}, now.Add(time.Second))
	store.ObserveReport("dp-1", "v1", true, "ready", now.Add(2*time.Second))

	disconnectedAt := now.Add(3 * time.Second)
	store.DisconnectWithReason(
		"dp-1",
		disconnectedAt,
		"ack_timeout",
		"timed out waiting for dataplane snapshot ack",
	)

	node, ok := store.Get("dp-1")
	if !ok {
		t.Fatal("expected disconnected node to exist")
	}
	if node.DisconnectReason != "ack_timeout" {
		t.Fatalf("expected disconnect reason to be retained, got %+v", node)
	}
	if !node.DisconnectedAt.Equal(disconnectedAt) {
		t.Fatalf("expected disconnectedAt to be recorded, got %+v", node)
	}
	if node.Message != "timed out waiting for dataplane snapshot ack" {
		t.Fatalf("expected disconnect message to be retained, got %+v", node)
	}

	store.ObserveReport("dp-1", "v9", true, "ready-again", disconnectedAt.Add(time.Second))

	node, ok = store.Get("dp-1")
	if !ok {
		t.Fatal("expected node to still exist after post-disconnect report")
	}
	if node.Connected {
		t.Fatalf("expected post-disconnect report to keep node disconnected, got %+v", node)
	}
	if node.Ready {
		t.Fatalf("expected post-disconnect report to keep node not ready, got %+v", node)
	}
	if node.LastAckVersion != "v1" {
		t.Fatalf("expected post-disconnect report to preserve ack version, got %+v", node)
	}
	if node.Message != "timed out waiting for dataplane snapshot ack" {
		t.Fatalf("expected post-disconnect report to preserve disconnect message, got %+v", node)
	}
	if node.DisconnectReason != "ack_timeout" {
		t.Fatalf("expected post-disconnect report to preserve disconnect reason, got %+v", node)
	}
	if !node.LastSeenAt.Equal(disconnectedAt.Add(time.Second)) {
		t.Fatalf("expected post-disconnect report to refresh lastSeenAt only, got %+v", node)
	}

	reconnectedAt := disconnectedAt.Add(2 * time.Second)
	store.Connect("dp-1", "kind", []string{"routes"}, reconnectedAt)

	node, ok = store.Get("dp-1")
	if !ok {
		t.Fatal("expected reconnected node to exist")
	}
	if !node.Connected {
		t.Fatalf("expected node to reconnect, got %+v", node)
	}
	if node.DisconnectReason != "" || !node.DisconnectedAt.IsZero() {
		t.Fatalf("expected reconnect to clear disconnect metadata, got %+v", node)
	}
	if node.Message != "" {
		t.Fatalf("expected reconnect to clear disconnect message, got %+v", node)
	}
	if !node.ConnectedAt.Equal(reconnectedAt) {
		t.Fatalf("expected reconnect to refresh connectedAt, got %+v", node)
	}
}

func TestNodeStatusStoreSupportsUpsertAndGet(t *testing.T) {
	store := NewNodeStatusStore()
	now := time.Unix(1_700_000_100, 0).UTC()

	store.Upsert(NodeStatus{
		NodeID:          "dp-2",
		Cluster:         "kind",
		Connected:       true,
		ConnectedAt:     now,
		LastSeenAt:      now,
		LastSentVersion: "v2",
		LastAckVersion:  "v2",
		Ready:           true,
		Subscriptions:   []string{"routes"},
	})

	node, ok := store.Get("dp-2")
	if !ok {
		t.Fatal("expected node to exist")
	}
	if node.LastAckVersion != "v2" || node.Cluster != "kind" {
		t.Fatalf("unexpected node contents: %+v", node)
	}

	node.Subscriptions[0] = "mutated"
	fresh, ok := store.Get("dp-2")
	if !ok {
		t.Fatal("expected node to still exist")
	}
	if fresh.Subscriptions[0] != "routes" {
		t.Fatalf("expected subscriptions to be cloned, got %+v", fresh.Subscriptions)
	}
}

func TestNodeStatusStoreHeartbeatReportPreservesAckAndMessage(t *testing.T) {
	store := NewNodeStatusStore()
	now := time.Unix(1_700_000_200, 0).UTC()

	store.Upsert(NodeStatus{
		NodeID:         "dp-3",
		Connected:      true,
		LastSeenAt:     now,
		LastAckVersion: "v1",
		Ready:          true,
		Message:        "snapshot applied",
	})

	store.ObserveReport("dp-3", "", true, "", now.Add(5*time.Second))

	node, ok := store.Get("dp-3")
	if !ok {
		t.Fatal("expected heartbeat-updated node to exist")
	}
	if node.LastAckVersion != "v1" {
		t.Fatalf("expected heartbeat to preserve last ack version, got %+v", node)
	}
	if node.Message != "snapshot applied" {
		t.Fatalf("expected heartbeat to preserve message, got %+v", node)
	}
	if !node.LastSeenAt.Equal(now.Add(5 * time.Second)) {
		t.Fatalf("expected heartbeat to refresh last seen timestamp, got %+v", node)
	}
}

func TestNodeStatusStoreIgnoresStaleStatusReport(t *testing.T) {
	store := NewNodeStatusStore()
	now := time.Unix(1_700_000_250, 0).UTC()

	store.Upsert(NodeStatus{
		NodeID:         "dp-3",
		Connected:      true,
		LastSeenAt:     now.Add(5 * time.Second),
		LastAckVersion: "v2",
		Ready:          true,
		Message:        "snapshot applied",
	})

	store.ObserveReport("dp-3", "v1", false, "stale heartbeat", now.Add(4*time.Second))

	node, ok := store.Get("dp-3")
	if !ok {
		t.Fatal("expected node to exist")
	}
	if !node.LastSeenAt.Equal(now.Add(5 * time.Second)) {
		t.Fatalf("expected stale report to preserve last seen timestamp, got %+v", node)
	}
	if node.LastAckVersion != "v2" {
		t.Fatalf("expected stale report to preserve last ack version, got %+v", node)
	}
	if !node.Ready {
		t.Fatalf("expected stale report to preserve readiness, got %+v", node)
	}
	if node.Message != "snapshot applied" {
		t.Fatalf("expected stale report to preserve message, got %+v", node)
	}
}

func TestNodeStatusStoreTracksNackWithoutOverwritingAckVersion(t *testing.T) {
	store := NewNodeStatusStore()
	now := time.Unix(1_700_000_300, 0).UTC()

	store.Upsert(NodeStatus{
		NodeID:           "dp-4",
		Cluster:          "kind",
		Connected:        true,
		LastSeenAt:       now,
		LastAckVersion:   "v1",
		LastNonce:        "nonce-1",
		LastConfigStatus: "ACK",
		Ready:            true,
		Message:          "snapshot applied",
	})

	store.ObserveNack("dp-4", "kind", "v2", "nonce-2", "listener reload failed", []string{"*"}, now.Add(time.Second))

	node, ok := store.Get("dp-4")
	if !ok {
		t.Fatal("expected nacked node to exist")
	}
	if node.LastAckVersion != "v1" {
		t.Fatalf("expected nack to preserve last ack version, got %+v", node)
	}
	if node.LastConfigStatus != "NACK" || node.LastNackVersion != "v2" || node.LastNackNonce != "nonce-2" {
		t.Fatalf("unexpected nack fields: %+v", node)
	}
	if node.LastNackMessage != "listener reload failed" || node.Message != "listener reload failed" {
		t.Fatalf("expected nack message to be retained, got %+v", node)
	}
}
