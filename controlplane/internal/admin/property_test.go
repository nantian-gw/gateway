package admin

import (
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"
	"time"

	"github.com/aether-gateway/aether-gateway/controlplane/internal/ir"
)

func TestBuildSummaryStableAcrossPermutationsProperty(t *testing.T) {
	now := time.Unix(1_700_000_900, 0).UTC()
	expected := buildSummary(summaryPropertySnapshotFixture(), summaryPropertyNodesFixture(), 15*time.Second, now)

	cfg := &quick.Config{MaxCount: 64}
	if err := quick.Check(func(seed uint64) bool {
		snapshot, nodes := shuffledSummaryPropertyInputs(seed)
		got := buildSummary(snapshot, nodes, 15*time.Second, now)
		return reflect.DeepEqual(expected, got)
	}, cfg); err != nil {
		t.Fatalf("buildSummary permutation property failed: %v", err)
	}
}

func TestBuildSnapshotSyncStableAcrossNodePermutationsProperty(t *testing.T) {
	now := time.Unix(1_700_000_900, 0).UTC()
	snapshot := summaryPropertySnapshotFixture()
	expected := buildSnapshotSync(snapshot, summaryPropertyNodesFixture(), readinessModeCurrentSnapshotAll, 15*time.Second, now)

	cfg := &quick.Config{MaxCount: 64}
	if err := quick.Check(func(seed uint64) bool {
		nodes := append([]ir.NodeStatus(nil), summaryPropertyNodesFixture()...)
		rng := rand.New(rand.NewSource(int64(seed)))
		rng.Shuffle(len(nodes), func(i, j int) { nodes[i], nodes[j] = nodes[j], nodes[i] })
		got := buildSnapshotSync(summaryPropertySnapshotFixture(), nodes, readinessModeCurrentSnapshotAll, 15*time.Second, now)
		return reflect.DeepEqual(expected, got)
	}, cfg); err != nil {
		t.Fatalf("buildSnapshotSync permutation property failed: %v", err)
	}
}

func summaryPropertySnapshotFixture() *ir.Snapshot {
	generatedAt := time.Unix(1_700_000_000, 0).UTC()
	return &ir.Snapshot{
		ID:          "v-current",
		GeneratedAt: generatedAt,
		Listeners: []ir.Listener{
			{Name: "web"},
			{Name: "grpc"},
		},
		HTTPRoutes: []ir.HTTPRoute{
			{
				Name:      "http-a",
				Namespace: "default",
				Rules: []ir.HTTPRule{{
					BackendRefs: []ir.BackendRef{
						{Name: "echo", Namespace: "default", Port: 8080},
					},
				}},
			},
			{
				Name:      "http-b",
				Namespace: "default",
				Rules: []ir.HTTPRule{{
					BackendRefs: []ir.BackendRef{
						{Name: "payments", Namespace: "default", Port: 9090},
					},
				}},
			},
		},
		GRPCRoutes: []ir.GRPCRoute{{
			Name:      "grpc-a",
			Namespace: "default",
			Rules: []ir.GRPCRule{{
				BackendRefs: []ir.BackendRef{
					{Name: "echo", Namespace: "default", Port: 8080},
				},
			}},
		}},
		StreamRoutes: []ir.StreamRoute{{
			Name:      "tcp-a",
			Namespace: "default",
			Kind:      "TCP",
			Rules: []ir.StreamRule{{
				BackendRefs: []ir.BackendRef{
					{Name: "payments", Namespace: "default", Port: 9090},
				},
			}},
		}},
		Backends: []ir.BackendCluster{
			{Name: "payments:9090", Namespace: "default", Metadata: map[string]string{"service": "payments"}},
			{Name: "unused:7070", Namespace: "default", Metadata: map[string]string{"service": "unused"}},
			{Name: "echo:8080", Namespace: "default", Metadata: map[string]string{"service": "echo"}},
		},
		Secrets: []ir.SecretMaterial{
			{Name: "cert-b", Namespace: "default"},
			{Name: "cert-a", Namespace: "default"},
		},
	}
}

func summaryPropertyNodesFixture() []ir.NodeStatus {
	return []ir.NodeStatus{
		{NodeID: "dp-3", Cluster: "kind", Connected: true, Ready: false, LastAckVersion: "v-current", LastSentVersion: "v-current", Message: "warming"},
		{NodeID: "dp-1", Cluster: "kind", Connected: true, Ready: true, LastAckVersion: "v-current", LastSentVersion: "v-current", Message: "ready"},
		{NodeID: "dp-4", Cluster: "kind", Connected: false, Ready: false, LastAckVersion: "v-old", Message: "offline"},
		{NodeID: "dp-2", Cluster: "kind", Connected: true, Ready: true, LastAckVersion: "v-old", LastSentVersion: "v-current", Message: "stale"},
		{
			NodeID:           "dp-5",
			Cluster:          "kind",
			Connected:        true,
			Ready:            true,
			LastAckVersion:   "v-old",
			LastSentVersion:  "v-current",
			LastConfigStatus: "NACK",
			LastNackVersion:  "v-current",
			LastNackNonce:    "v-current",
			LastNackMessage:  "listener reload failed",
			Message:          "listener reload failed",
		},
	}
}

func shuffledSummaryPropertyInputs(seed uint64) (*ir.Snapshot, []ir.NodeStatus) {
	snapshot := summaryPropertySnapshotFixture().Clone()
	nodes := append([]ir.NodeStatus(nil), summaryPropertyNodesFixture()...)
	rng := rand.New(rand.NewSource(int64(seed)))

	rng.Shuffle(len(snapshot.Listeners), func(i, j int) {
		snapshot.Listeners[i], snapshot.Listeners[j] = snapshot.Listeners[j], snapshot.Listeners[i]
	})
	rng.Shuffle(len(snapshot.HTTPRoutes), func(i, j int) {
		snapshot.HTTPRoutes[i], snapshot.HTTPRoutes[j] = snapshot.HTTPRoutes[j], snapshot.HTTPRoutes[i]
	})
	rng.Shuffle(len(snapshot.GRPCRoutes), func(i, j int) {
		snapshot.GRPCRoutes[i], snapshot.GRPCRoutes[j] = snapshot.GRPCRoutes[j], snapshot.GRPCRoutes[i]
	})
	rng.Shuffle(len(snapshot.StreamRoutes), func(i, j int) {
		snapshot.StreamRoutes[i], snapshot.StreamRoutes[j] = snapshot.StreamRoutes[j], snapshot.StreamRoutes[i]
	})
	rng.Shuffle(len(snapshot.Backends), func(i, j int) {
		snapshot.Backends[i], snapshot.Backends[j] = snapshot.Backends[j], snapshot.Backends[i]
	})
	rng.Shuffle(len(snapshot.Secrets), func(i, j int) {
		snapshot.Secrets[i], snapshot.Secrets[j] = snapshot.Secrets[j], snapshot.Secrets[i]
	})
	rng.Shuffle(len(nodes), func(i, j int) {
		nodes[i], nodes[j] = nodes[j], nodes[i]
	})

	return snapshot, nodes
}
