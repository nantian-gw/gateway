package admin

import (
	"strings"
	"testing"
	"time"

	"github.com/nantian-gw/gateway/internal/ir"
)

func TestBuildSummaryAggregatesSnapshotAndNodes(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	snapshot := &ir.Snapshot{
		ID:           "v1",
		GeneratedAt:  now,
		Listeners:    []ir.Listener{{Name: "l1"}},
		HTTPRoutes:   []ir.HTTPRoute{{Name: "http"}},
		GRPCRoutes:   []ir.GRPCRoute{{Name: "grpc"}},
		StreamRoutes: []ir.StreamRoute{{Name: "tcp"}},
		Backends:     []ir.BackendCluster{{Name: "b1"}},
		Secrets:      []ir.SecretMaterial{{Name: "s1"}},
	}
	nodes := []ir.NodeStatus{
		{NodeID: "dp-1", Connected: true, Ready: true},
		{NodeID: "dp-2", Connected: false, Ready: false},
	}

	summary := buildSummary(snapshot, nodes, 15*time.Second, now)
	if summary.SnapshotVersion != "v1" {
		t.Fatalf("unexpected snapshot version: %s", summary.SnapshotVersion)
	}
	if summary.RouteCount != 3 {
		t.Fatalf("expected 3 routes, got %d", summary.RouteCount)
	}
	if summary.BackendCount != 0 {
		t.Fatalf("expected referenced backend count to default to 0, got %d", summary.BackendCount)
	}
	if summary.NodeCount != 2 || summary.ConnectedNodeCount != 1 || summary.ReadyNodeCount != 1 {
		t.Fatalf("unexpected node summary: %+v", summary)
	}
	if summary.ListenerCount != 1 || summary.ReadyListenerCount != 0 || summary.WarningListenerCount != 1 || summary.FailedListenerCount != 0 {
		t.Fatalf("unexpected listener health: %+v", summary)
	}
	if !summary.GeneratedAt.Equal(now) {
		t.Fatalf("unexpected timestamp: %v", summary.GeneratedAt)
	}
}

func TestBuildSummaryWarnsOnPersistentVersionDrift(t *testing.T) {
	now := time.Unix(1_700_000_500, 0).UTC()
	snapshot := &ir.Snapshot{
		ID:          "v2",
		GeneratedAt: now.Add(-20 * time.Second),
	}
	nodes := []ir.NodeStatus{
		{NodeID: "dp-1", Connected: true, Ready: true, LastAckVersion: "v1"},
		{NodeID: "dp-2", Connected: true, Ready: false, LastAckVersion: "v2"},
	}

	summary := buildSummary(snapshot, nodes, 15*time.Second, now)
	if summary.DriftedNodeCount != 1 {
		t.Fatalf("expected 1 drifted node, got %+v", summary)
	}
	if len(summary.Warnings) == 0 {
		t.Fatalf("expected drift warning, got %+v", summary)
	}
	if !strings.Contains(summary.Warnings[0], "snapshot v2") || !strings.Contains(summary.Warnings[0], "dp-1") {
		t.Fatalf("expected warning to mention snapshot version and drifted node, got %+v", summary.Warnings)
	}
}

func TestBuildSnapshotSyncClassifiesNodeStates(t *testing.T) {
	now := time.Unix(1_700_000_500, 0).UTC()
	snapshot := &ir.Snapshot{
		ID:          "v2",
		GeneratedAt: now.Add(-20 * time.Second),
	}
	nodes := []ir.NodeStatus{
		{NodeID: "dp-1", Cluster: "kind", Connected: true, Ready: true, LastAckVersion: "v2", LastSentVersion: "v2", Message: "ready"},
		{NodeID: "dp-2", Cluster: "kind", Connected: true, Ready: false, LastAckVersion: "v2", LastSentVersion: "v2", Message: "warming"},
		{NodeID: "dp-3", Cluster: "kind", Connected: true, Ready: true, LastAckVersion: "v1", LastSentVersion: "v2", Message: "stale"},
		{
			NodeID:           "dp-4",
			Cluster:          "kind",
			Connected:        false,
			Ready:            false,
			LastAckVersion:   "v1",
			DisconnectedAt:   now.Add(-5 * time.Second),
			DisconnectReason: "ack_timeout",
			Message:          "offline",
		},
	}

	response := buildSnapshotSync(snapshot, nodes, readinessModeCurrentSnapshotAll, 15*time.Second, now)
	if response.ReadinessMode != readinessModeCurrentSnapshotAll {
		t.Fatalf("unexpected readiness mode: %+v", response)
	}
	if response.PublishedFor != "20s" {
		t.Fatalf("unexpected published duration: %+v", response)
	}
	if response.Summary.NodeCount != 4 || response.Summary.ConnectedNodeCount != 3 {
		t.Fatalf("unexpected node summary: %+v", response.Summary)
	}
	if response.Summary.CurrentVersionNodeCount != 2 || response.Summary.CurrentVersionReadyCount != 1 {
		t.Fatalf("unexpected snapshot sync counts: %+v", response.Summary)
	}
	if response.Summary.DriftedNodeCount != 1 || response.Summary.DisconnectedNodeCount != 1 || response.Summary.AwaitingReadyNodeCount != 1 {
		t.Fatalf("unexpected node state distribution: %+v", response.Summary)
	}
	if len(response.Warnings) == 0 {
		t.Fatalf("expected warnings, got %+v", response)
	}

	states := map[string]string{}
	for _, node := range response.Nodes {
		states[node.NodeID] = node.State
		if node.NodeID == "dp-4" {
			if node.DisconnectReason != "ack_timeout" {
				t.Fatalf("expected disconnect reason in snapshot sync view, got %+v", node)
			}
			if node.DisconnectedAt.IsZero() {
				t.Fatalf("expected disconnectedAt in snapshot sync view, got %+v", node)
			}
		}
	}
	if states["dp-1"] != "current-ready" || states["dp-2"] != "awaiting-ready" || states["dp-3"] != "drifted" || states["dp-4"] != "disconnected" {
		t.Fatalf("unexpected node states: %+v", response.Nodes)
	}
}

func TestBuildSnapshotSyncClassifiesRejectedCurrentSnapshot(t *testing.T) {
	now := time.Unix(1_700_000_700, 0).UTC()
	snapshot := &ir.Snapshot{
		ID:          "v3",
		GeneratedAt: now.Add(-20 * time.Second),
	}
	nodes := []ir.NodeStatus{
		{
			NodeID:           "dp-1",
			Cluster:          "kind",
			Connected:        true,
			Ready:            true,
			LastAckVersion:   "v2",
			LastSentVersion:  "v3",
			LastConfigStatus: "NACK",
			LastNackVersion:  "v3",
			LastNackNonce:    "v3",
			LastNackMessage:  "listener reload failed",
			Message:          "listener reload failed",
		},
	}

	response := buildSnapshotSync(snapshot, nodes, readinessModeCurrentSnapshotAny, 15*time.Second, now)
	if response.Summary.DriftedNodeCount != 1 || response.Summary.CurrentVersionNodeCount != 0 {
		t.Fatalf("unexpected rejected sync summary: %+v", response.Summary)
	}
	if len(response.Nodes) != 1 || response.Nodes[0].State != "rejected" {
		t.Fatalf("expected rejected node state, got %+v", response.Nodes)
	}
	if response.Nodes[0].Message != "listener reload failed" {
		t.Fatalf("expected rejected node message, got %+v", response.Nodes[0])
	}
}
