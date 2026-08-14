package xds

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"

	"github.com/nantian-gw/gateway/internal/config"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/noderegistry"
	"github.com/nantian-gw/gateway/internal/observability"
)

func TestCanonicalizeSupportedFeaturesTrimsSortsAndDeduplicates(t *testing.T) {
	got := canonicalizeSupportedFeatures([]string{" route.labels.v1 ", "", "core.v1", "core.v1"})
	want := []string{featureCoreV1, featureRouteLabelsV1}
	assert.Equal(t, want, got)
}

func TestEffectiveProjectionProfileUsesLegacyFallbackForEmptyAdvertisement(t *testing.T) {
	got := effectiveProjectionProfile(nil)
	wantEffective := []string{
		featureCoreV1,
		featureBackendAIServiceV1,
		featureBackendTokenPolicyV1,
		featureBackendWasmPluginV1,
	}
	require.Empty(t, got.advertised, "advertised features should be empty")
	assert.Equal(t, wantEffective, got.effective)
	assert.Equal(t, compatibilityProfileLegacyPreNegotiationV1, got.compatibilityProfile)
	assert.Equal(t, compatibilityProfileLegacyPreNegotiationV1, got.projectionKey)
}

func TestStreamConfigurationPreservesAdvertisedFeaturesWhenAckOmitsSupportedFeatures(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	metrics := observability.NewMetrics()
	nodes := noderegistry.NewRegistry(ir.NewNodeStatusStore(), nil, logger, noderegistry.Options{Metrics: metrics})
	defer nodes.Close()

	server, err := New(":18080", config.GRPCTLSConfig{}, config.GRPCRuntimeConfig{}, store, nodes, logger, metrics)
	require.NoError(t, err, "New returned error")

	stream := newFakeConfigStream()
	stream.initialRecv <- &controlv1.DiscoveryRequest{
		NodeId:            "dp-1",
		Cluster:           "default",
		Subscriptions:     []string{"*"},
		SupportedFeatures: []string{" route.labels.v1 ", "core.v1", "core.v1"},
	}

	result := make(chan error, 1)
	go func() {
		result <- server.StreamConfiguration(stream)
	}()

	waitForNodeConnection(t, nodes, "dp-1")
	status := waitForNodeStatus(t, nodes, "dp-1", func(status ir.NodeStatus) bool {
		return assert.ObjectsAreEqual(status.SupportedFeatures, []string{featureCoreV1, featureRouteLabelsV1})
	})
	assert.Equal(t, []string{featureCoreV1, featureRouteLabelsV1}, status.SupportedFeatures)

	snapshot := &ir.Snapshot{GeneratedAt: time.Now().UTC()}
	store.Publish(snapshot)
	stream.waitForSendCount(t, time.Second)

	stream.pushRecv(&controlv1.DiscoveryRequest{
		NodeId:        "dp-1",
		Cluster:       "default",
		Version:       snapshot.ID,
		Nonce:         snapshot.ID,
		Subscriptions: []string{"*"},
		ResultStatus:  controlv1.DiscoveryResultStatus_DISCOVERY_ACK,
	})

	status = waitForNodeStatus(t, nodes, "dp-1", func(status ir.NodeStatus) bool {
		return status.LastAckVersion == snapshot.ID
	})
	assert.Equal(t, []string{featureCoreV1, featureRouteLabelsV1}, status.SupportedFeatures, "supported features after ack without supported_features")

	stream.release()
	select {
	case err := <-result:
		require.NoError(t, err, "expected stream to exit cleanly after release")
	case <-time.After(time.Second):
		t.Fatal("StreamConfiguration did not return after stream release")
	}
}

func waitForNodeStatus(
	t *testing.T,
	nodes *noderegistry.Registry,
	nodeID string,
	match func(ir.NodeStatus) bool,
) ir.NodeStatus {
	t.Helper()

	deadline := time.After(time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		status, ok := nodes.Get(context.Background(), nodeID)
		if ok && (match == nil || match(status)) {
			return status
		}

		select {
		case <-deadline:
			t.Fatalf("timed out waiting for node %q status match", nodeID)
		case <-ticker.C:
		}
	}
}