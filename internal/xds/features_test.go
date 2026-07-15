package xds

import (
	"context"
	"io"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/nantian-gw/gateway/internal/config"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/noderegistry"
	"github.com/nantian-gw/gateway/internal/observability"
	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"
)

func TestCanonicalizeSupportedFeaturesTrimsSortsAndDeduplicates(t *testing.T) {
	got := canonicalizeSupportedFeatures([]string{" route.labels.v1 ", "", "core.v1", "core.v1"})
	want := []string{featureCoreV1, featureRouteLabelsV1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("canonicalizeSupportedFeatures() = %#v, want %#v", got, want)
	}
}

func TestEffectiveProjectionProfileUsesLegacyFallbackForEmptyAdvertisement(t *testing.T) {
	got := effectiveProjectionProfile(nil)
	wantEffective := []string{
		featureCoreV1,
		featureBackendAIServiceV1,
		featureBackendTokenPolicyV1,
		featureBackendWasmPluginV1,
	}
	if len(got.advertised) != 0 {
		t.Fatalf("advertised features = %#v, want empty", got.advertised)
	}
	if !reflect.DeepEqual(got.effective, wantEffective) {
		t.Fatalf("effective features = %#v, want %#v", got.effective, wantEffective)
	}
	if got.compatibilityProfile != compatibilityProfileLegacyPreNegotiationV1 {
		t.Fatalf("compatibility profile = %q, want %q", got.compatibilityProfile, compatibilityProfileLegacyPreNegotiationV1)
	}
	if got.projectionKey != compatibilityProfileLegacyPreNegotiationV1 {
		t.Fatalf("projection key = %q, want %q", got.projectionKey, compatibilityProfileLegacyPreNegotiationV1)
	}
}

func TestStreamConfigurationPreservesAdvertisedFeaturesWhenAckOmitsSupportedFeatures(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	metrics := observability.NewMetrics()
	nodes := noderegistry.NewRegistry(ir.NewNodeStatusStore(), nil, logger, noderegistry.Options{Metrics: metrics})
	defer nodes.Close()

	server, err := New(":18080", config.GRPCTLSConfig{}, config.GRPCRuntimeConfig{}, store, nodes, logger, metrics)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

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
		return reflect.DeepEqual(status.SupportedFeatures, []string{featureCoreV1, featureRouteLabelsV1})
	})
	if !reflect.DeepEqual(status.SupportedFeatures, []string{featureCoreV1, featureRouteLabelsV1}) {
		t.Fatalf("supported features after connect = %#v, want %#v", status.SupportedFeatures, []string{featureCoreV1, featureRouteLabelsV1})
	}

	snapshot := &ir.Snapshot{GeneratedAt: time.Now().UTC()}
	store.Publish(snapshot)
	stream.waitForSendCount(t, 1, time.Second)

	stream.pushRecv(&controlv1.DiscoveryRequest{
		NodeId:        "dp-1",
		Cluster:       "default",
		Version:       snapshot.ID,
		Nonce:         snapshot.ID,
		Subscriptions: []string{"*"},
		ResultStatus:  controlv1.DiscoveryResultStatus_DISCOVERY_RESULT_STATUS_ACK,
	})

	status = waitForNodeStatus(t, nodes, "dp-1", func(status ir.NodeStatus) bool {
		return status.LastAckVersion == snapshot.ID
	})
	if !reflect.DeepEqual(status.SupportedFeatures, []string{featureCoreV1, featureRouteLabelsV1}) {
		t.Fatalf("supported features after ack without supported_features = %#v, want %#v", status.SupportedFeatures, []string{featureCoreV1, featureRouteLabelsV1})
	}

	stream.release()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("expected stream to exit cleanly after release, got %v", err)
		}
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
