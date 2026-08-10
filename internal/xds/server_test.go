package xds

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"

	"github.com/nantian-gw/gateway/internal/config"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/noderegistry"
	"github.com/nantian-gw/gateway/internal/observability"
)

func TestToListenerProtocolMapsTLS(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  controlv1.ListenerProtocol
	}{
		{"TLS maps to LISTENER_PROTOCOL_TLS", "TLS", controlv1.ListenerProtocol_LISTENER_PROTOCOL_TLS},
		{"TLS_PASSTHROUGH maps to passthrough", "TLS_PASSTHROUGH", controlv1.ListenerProtocol_LISTENER_PROTOCOL_TLS_PASSTHROUGH},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toListenerProtocol(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestToProtoRouteTimeouts(t *testing.T) {
	request := 12 * time.Second
	backendRequest := 3 * time.Second

	timeouts := toProtoRouteTimeouts(&ir.RouteTimeouts{
		Request:        &request,
		BackendRequest: &backendRequest,
	})

	require.NotNil(t, timeouts, "expected route timeouts")
	assert.Equal(t, 12*time.Second, timeouts.Request.AsDuration())
	assert.Equal(t, 3*time.Second, timeouts.BackendRequest.AsDuration())
}

func TestToProtoSnapshotOmitsZeroBackendRequestTimeout(t *testing.T) {
	snapshot := &ir.Snapshot{
		Backends: []ir.BackendCluster{{
			Name:           "orders:80",
			Namespace:      "default",
			Protocol:       "HTTP",
			ConnectTimeout: 5 * time.Second,
		}},
	}

	out := toProtoSnapshot(snapshot)
	require.Len(t, out.Backends, 1)
	assert.Equal(t, 5*time.Second, out.Backends[0].ConnectTimeout.AsDuration())
	assert.Nil(t, out.Backends[0].RequestTimeout, "expected zero request timeout to be omitted")
}

func TestToProtoBackendsPreservesFilters(t *testing.T) {
	backends := toProtoBackends([]ir.BackendRef{{
		Namespace: "default",
		Name:      "echo",
		Port:      8080,
		Weight:    1,
		Filters: []ir.Filter{{
			Type: "RequestHeaderModifier",
			Config: map[string]any{
				"set": []any{
					map[string]any{
						"name":  "X-Test",
						"value": "value",
					},
				},
			},
		}},
	}})

	require.Len(t, backends, 1)
	require.Len(t, backends[0].Filters, 1)
	assert.Equal(t, "RequestHeaderModifier", backends[0].Filters[0].GetType())
}

func TestToProtoFiltersPreservesNestedDirectResponseExtension(t *testing.T) {
	filters := toProtoFilters([]ir.Filter{{
		Type: "ExtensionRef",
		Config: map[string]any{
			"resolved":      true,
			"extensionType": "DirectResponse",
			"directResponse": map[string]any{
				"statusCode":  503,
				"body":        "maintenance",
				"contentType": "text/plain",
			},
		},
	}})

	require.Len(t, filters, 1)
	assert.Equal(t, "ExtensionRef", filters[0].GetType())
	directResponse := filters[0].GetConfig().GetFields()["directResponse"].GetStructValue()
	require.NotNil(t, directResponse, "expected nested directResponse config")
	assert.Equal(t, float64(503), directResponse.Fields["statusCode"].GetNumberValue())
	assert.Equal(t, "maintenance", directResponse.Fields["body"].GetStringValue())
}

func TestToProtoSnapshotPreservesWorkloadExtensions(t *testing.T) {
	snapshot := toProtoSnapshot(&ir.Snapshot{
		Workloads: []ir.Workload{{
			Namespace: "gateway-conformance-mesh-consumer",
			Name:      "echo-v1",
			IP:        "10.0.0.10",
		}},
	})

	require.NotNil(t, snapshot.Extensions, "expected extensions to be populated")
	workloads, ok := snapshot.Extensions.Fields["workloads"]
	require.True(t, ok, "expected workloads extension")
	require.Len(t, workloads.GetListValue().GetValues(), 1)
}

func TestToProtoFiltersLogsAndFallsBackOnStructError(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	original := newStructPB
	newStructPB = func(map[string]any) (*structpb.Struct, error) {
		return nil, errors.New("boom")
	}
	defer func() {
		newStructPB = original
	}()

	filters := toProtoFiltersWithLogger([]ir.Filter{{
		Type: "ExtensionRef",
		Config: map[string]any{
			"invalid": "value",
		},
	}}, logger)

	require.Len(t, filters, 1)
	assert.Equal(t, "ExtensionRef", filters[0].GetType())
	require.NotNil(t, filters[0].GetConfig(), "expected fallback empty struct, got nil")
	require.Empty(t, filters[0].GetConfig().GetFields(), "expected empty fallback struct")
	assert.True(t, strings.Contains(logs.String(), "failed to build filter config struct"), "expected warning log")
}

func TestToProtoSnapshotLogsAndDropsExtensionsOnStructError(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	original := newStructPB
	newStructPB = func(map[string]any) (*structpb.Struct, error) {
		return nil, errors.New("boom")
	}
	defer func() {
		newStructPB = original
	}()

	snapshot := toProtoSnapshotWithLogger(&ir.Snapshot{
		Workloads: []ir.Workload{{
			Namespace: "default",
			Name:      "wl",
			IP:        "10.0.0.1",
		}},
	}, logger)

	assert.Nil(t, snapshot.Extensions, "expected nil extensions fallback")
	assert.True(t, strings.Contains(logs.String(), "failed to build snapshot extensions struct"), "expected warning log")
}

func TestToEmptyStructLogsAndFallsBackOnStructError(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	original := newStructPB
	newStructPB = func(map[string]any) (*structpb.Struct, error) {
		return nil, errors.New("boom")
	}
	defer func() {
		newStructPB = original
	}()

	payload := toEmptyStructWithLogger(logger)
	require.NotNil(t, payload, "expected fallback empty struct")
	require.Empty(t, payload.GetFields(), "expected empty fields")
	assert.True(t, strings.Contains(logs.String(), "failed to build empty struct"), "expected warning log")
}

func TestToProtoSnapshotPreservesRouteAnnotations(t *testing.T) {
	snapshot := toProtoSnapshot(&ir.Snapshot{
		Listeners: []ir.Listener{{
			Name:      "web",
			Address:   "192.0.2.10",
			Addresses: []string{"192.0.2.10", "gw.example.com"},
		}},
		HTTPRoutes: []ir.HTTPRoute{{
			Name:      "route",
			Namespace: "default",
			Annotations: map[string]string{
				"gateway.nantian.dev/access-log-mode": "json",
			},
		}},
		GRPCRoutes: []ir.GRPCRoute{{
			Name:      "grpc-route",
			Namespace: "default",
			Annotations: map[string]string{
				"gateway.nantian.dev/access-log-enabled": "false",
			},
		}},
		StreamRoutes: []ir.StreamRoute{{
			Name:      "tcp-route",
			Namespace: "default",
			Kind:      "TCP",
			Annotations: map[string]string{
				"gateway.nantian.dev/access-log-path": "/var/log/nantian-gw/tcp.log",
			},
		}},
	})

	assert.Equal(t, "json", snapshot.HttpRoutes[0].Annotations["gateway.nantian.dev/access-log-mode"])
	require.Len(t, snapshot.Listeners[0].Addresses, 2)
	assert.Equal(t, "192.0.2.10", snapshot.Listeners[0].Addresses[0])
	assert.Equal(t, "gw.example.com", snapshot.Listeners[0].Addresses[1])
	assert.Equal(t, "false", snapshot.GrpcRoutes[0].Annotations["gateway.nantian.dev/access-log-enabled"])
	assert.Equal(t, "/var/log/nantian-gw/tcp.log", snapshot.StreamRoutes[0].Annotations["gateway.nantian.dev/access-log-path"])
}

func TestToProtoSessionPersistence(t *testing.T) {
	absolute := 5 * time.Minute
	idle := 30 * time.Second

	item := toProtoSessionPersistence(&ir.SessionPersistencePolicy{
		SessionName:     "nantian-gw-http-session",
		Type:            "Cookie",
		AbsoluteTimeout: &absolute,
		IdleTimeout:     &idle,
		Cookie: &ir.CookieConfig{
			LifetimeType: "Permanent",
		},
	})

	require.NotNil(t, item, "expected session persistence proto")
	assert.Equal(t, "nantian-gw-http-session", item.SessionName)
	assert.Equal(t, controlv1.SessionPersistenceType_SESSION_PERSISTENCE_TYPE_COOKIE, item.Type)
	require.NotNil(t, item.Cookie)
	assert.Equal(t, controlv1.CookieLifetimeType_COOKIE_LIFETIME_TYPE_PERMANENT, item.Cookie.LifetimeType)
	assert.Equal(t, 5*time.Minute, item.AbsoluteTimeout.AsDuration())
	assert.Equal(t, 30*time.Second, item.IdleTimeout.AsDuration())
}

func TestToProtoLoadBalancing(t *testing.T) {
	item := toProtoLoadBalancing(&ir.LoadBalancingPolicy{
		Type: "ConsistentHash",
		ConsistentHash: &ir.ConsistentHashPolicy{
			KeyType:    "Header",
			HeaderName: "x-user-id",
		},
	})

	require.NotNil(t, item, "expected load balancing proto")
	assert.Equal(t, controlv1.LoadBalancingPolicyType_LOAD_BALANCING_POLICY_TYPE_CONSISTENT_HASH, item.Type)
	require.NotNil(t, item.ConsistentHash, "expected consistent hash proto")
	assert.Equal(t, controlv1.ConsistentHashKeyType_CONSISTENT_HASH_KEY_TYPE_HEADER, item.ConsistentHash.KeyType)
	assert.Equal(t, "x-user-id", item.ConsistentHash.HeaderName)
}

func TestToProtoSnapshotPreservesGrpcMatchType(t *testing.T) {
	snapshot := toProtoSnapshot(&ir.Snapshot{
		GRPCRoutes: []ir.GRPCRoute{{
			Name:      "grpc-route",
			Namespace: "default",
			Rules: []ir.GRPCRule{{
				Matches: []ir.GRPCMatch{{
					Service:   "helloworld\\..+",
					Method:    "Say(H|G).*",
					MatchType: "RegularExpression",
				}},
			}},
		}},
	})

	require.Len(t, snapshot.GrpcRoutes, 1)
	require.Len(t, snapshot.GrpcRoutes[0].Rules, 1)
	require.Len(t, snapshot.GrpcRoutes[0].Rules[0].Matches, 1)
	assert.Equal(t, "RegularExpression", snapshot.GrpcRoutes[0].Rules[0].Matches[0].MatchType)
}

func TestStreamConfigurationDrainsOnServerShutdown(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	metrics := observability.NewMetrics()
	nodes := noderegistry.NewRegistry(ir.NewNodeStatusStore(), nil, logger, noderegistry.Options{})
	defer nodes.Close()

	server, err := New(":18080", config.GRPCTLSConfig{}, config.GRPCRuntimeConfig{}, store, nodes, logger, metrics)
	require.NoError(t, err, "New returned error")
	server.signalShutdown()

	stream := newFakeConfigStream()
	result := make(chan error, 1)
	go func() {
		result <- server.StreamConfiguration(stream)
	}()

	select {
	case err := <-result:
		assert.Equal(t, codes.Unavailable, status.Code(err), "expected unavailable status on shutdown")
	case <-time.After(time.Second):
		t.Fatal("StreamConfiguration did not return after shutdown")
	}

	stream.release()
	_, ok := nodes.Get(context.Background(), "dp-1")
	assert.False(t, ok, "expected blocked handshake stream to avoid recording node status after shutdown")
	assert.Equal(t, float64(1), testutil.ToFloat64(metrics.XDSStreamTerminationsTotal.WithLabelValues("shutdown")))
}

func TestStreamConfigurationInterruptsBlockedInitialRecvOnShutdown(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	metrics := observability.NewMetrics()
	nodes := noderegistry.NewRegistry(ir.NewNodeStatusStore(), nil, logger, noderegistry.Options{})
	defer nodes.Close()

	server, err := New(":18080", config.GRPCTLSConfig{}, config.GRPCRuntimeConfig{}, store, nodes, logger, metrics)
	require.NoError(t, err, "New returned error")

	stream := newFakeConfigStream()
	stream.blockInitialRecv()
	result := make(chan error, 1)
	go func() {
		result <- server.StreamConfiguration(stream)
	}()

	server.signalShutdown()

	select {
	case err := <-result:
		assert.Equal(t, codes.Unavailable, status.Code(err), "expected unavailable status on shutdown during initial recv")
	case <-time.After(200 * time.Millisecond):
		t.Fatal("StreamConfiguration did not return after shutdown while initial recv was blocked")
	}

	stream.release()
	assert.Equal(t, float64(1), testutil.ToFloat64(metrics.XDSStreamTerminationsTotal.WithLabelValues("shutdown")))
}

func TestStreamConfigurationInterruptsBlockedSendOnShutdown(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	nodes := noderegistry.NewRegistry(ir.NewNodeStatusStore(), nil, logger, noderegistry.Options{})
	defer nodes.Close()

	server, err := New(":18080", config.GRPCTLSConfig{}, config.GRPCRuntimeConfig{}, store, nodes, logger, nil)
	require.NoError(t, err, "New returned error")

	stream := newFakeConfigStream()
	stream.blockSend()
	result := make(chan error, 1)
	go func() {
		result <- server.StreamConfiguration(stream)
	}()

	waitForNodeConnection(t, nodes, "dp-1")
	store.Publish(&ir.Snapshot{ID: "v-test", GeneratedAt: time.Now().UTC()})
	stream.waitForSendStart(t)

	server.signalShutdown()

	select {
	case err := <-result:
		assert.Equal(t, codes.Unavailable, status.Code(err), "expected unavailable status on shutdown")
	case <-time.After(200 * time.Millisecond):
		t.Fatal("StreamConfiguration did not return after shutdown while send was blocked")
	}

	stream.releaseSend()
	stream.release()

	nodeStatus, ok := nodes.Get(context.Background(), "dp-1")
	require.True(t, ok, "expected node status to be recorded")
	assert.False(t, nodeStatus.Connected, "expected node to be disconnected after shutdown")
	assert.False(t, nodeStatus.Ready, "expected shutdown disconnect to clear readiness")
	assert.Equal(t, "shutdown", nodeStatus.DisconnectReason)
	assert.Equal(t, "xds stream drained for controlplane shutdown", nodeStatus.Message)
}

func TestStreamConfigurationSupersedesExistingNodeStream(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	metrics := observability.NewMetrics()
	nodes := noderegistry.NewRegistry(ir.NewNodeStatusStore(), nil, logger, noderegistry.Options{})
	defer nodes.Close()

	server, err := New(":18080", config.GRPCTLSConfig{}, config.GRPCRuntimeConfig{}, store, nodes, logger, metrics)
	require.NoError(t, err, "New returned error")

	oldStream := newFakeConfigStream()
	oldResult := make(chan error, 1)
	go func() {
		oldResult <- server.StreamConfiguration(oldStream)
	}()
	waitForNodeConnection(t, nodes, "dp-1")

	newStream := newFakeConfigStream()
	newResult := make(chan error, 1)
	go func() {
		newResult <- server.StreamConfiguration(newStream)
	}()

	select {
	case err := <-oldResult:
		assert.Equal(t, codes.Unavailable, status.Code(err), "expected unavailable status on superseded stream")
	case <-time.After(200 * time.Millisecond):
		t.Fatal("old stream did not return after replacement stream connected")
	}

	nodeStatus, ok := nodes.Get(context.Background(), "dp-1")
	require.True(t, ok, "expected node status to be recorded")
	assert.True(t, nodeStatus.Connected, "expected node to remain connected after stream replacement")
	assert.Equal(t, float64(1), testutil.ToFloat64(metrics.XDSStreamTerminationsTotal.WithLabelValues("superseded")))

	oldStream.release()
	newStream.release()

	select {
	case err := <-newResult:
		require.NoError(t, err, "expected replacement stream to exit cleanly after release")
	case <-time.After(time.Second):
		t.Fatal("replacement stream did not return after release")
	}
}

func TestStreamConfigurationInterruptsBlockedSendOnSupersededStream(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	metrics := observability.NewMetrics()
	nodes := noderegistry.NewRegistry(ir.NewNodeStatusStore(), nil, logger, noderegistry.Options{})
	defer nodes.Close()

	server, err := New(
		":18080",
		config.GRPCTLSConfig{},
		config.GRPCRuntimeConfig{SnapshotSendTimeout: "5s"},
		store,
		nodes,
		logger,
		metrics,
	)
	require.NoError(t, err, "New returned error")

	oldStream := newFakeConfigStream()
	oldStream.blockSend()
	oldResult := make(chan error, 1)
	go func() {
		oldResult <- server.StreamConfiguration(oldStream)
	}()

	waitForNodeConnection(t, nodes, "dp-1")
	store.Publish(&ir.Snapshot{ID: "v-replace", GeneratedAt: time.Now().UTC()})
	oldStream.waitForSendStart(t)

	newStream := newFakeConfigStream()
	newResult := make(chan error, 1)
	go func() {
		newResult <- server.StreamConfiguration(newStream)
	}()

	select {
	case err := <-oldResult:
		assert.Equal(t, codes.Unavailable, status.Code(err), "expected unavailable status on superseded blocked stream")
	case <-time.After(200 * time.Millisecond):
		t.Fatal("blocked send stream did not return after replacement stream connected")
	}

	assert.Equal(t, float64(0), testutil.ToFloat64(metrics.XDSSnapshotSendTimeoutsTotal), "unexpected send timeout count for superseded stream")
	assert.Equal(t, float64(1), testutil.ToFloat64(metrics.XDSStreamTerminationsTotal.WithLabelValues("superseded")), "superseded stream termination count")

	nodeStatus, ok := nodes.Get(context.Background(), "dp-1")
	require.True(t, ok, "expected node status to be recorded")
	assert.True(t, nodeStatus.Connected, "expected node to remain connected after superseding blocked stream")

	oldStream.releaseSend()
	oldStream.release()
	newStream.release()

	select {
	case err := <-newResult:
		require.NoError(t, err, "expected replacement stream to exit cleanly after release")
	case <-time.After(time.Second):
		t.Fatal("replacement stream did not return after release")
	}
}

func TestStreamConfigurationTimesOutBlockedSend(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	nodes := noderegistry.NewRegistry(ir.NewNodeStatusStore(), nil, logger, noderegistry.Options{})
	defer nodes.Close()

	server, err := New(
		":18080",
		config.GRPCTLSConfig{},
		config.GRPCRuntimeConfig{SnapshotSendTimeout: "25ms"},
		store,
		nodes,
		logger,
		nil,
	)
	require.NoError(t, err, "New returned error")

	stream := newFakeConfigStream()
	stream.blockSend()
	result := make(chan error, 1)
	go func() {
		result <- server.StreamConfiguration(stream)
	}()

	waitForNodeConnection(t, nodes, "dp-1")
	store.Publish(&ir.Snapshot{ID: "v-timeout", GeneratedAt: time.Now().UTC()})
	stream.waitForSendStart(t)

	select {
	case err := <-result:
		assert.Equal(t, codes.DeadlineExceeded, status.Code(err), "expected deadline exceeded status on blocked send timeout")
	case <-time.After(200 * time.Millisecond):
		t.Fatal("StreamConfiguration did not return after blocked send timed out")
	}

	stream.releaseSend()
	stream.release()

	nodeStatus, ok := nodes.Get(context.Background(), "dp-1")
	require.True(t, ok, "expected node status to be recorded")
	assert.False(t, nodeStatus.Connected, "expected node to be disconnected after blocked send timeout")
	assert.False(t, nodeStatus.Ready, "expected send timeout disconnect to clear readiness")
	assert.Equal(t, "send_timeout", nodeStatus.DisconnectReason)
	assert.Equal(t, "timed out sending snapshot to dataplane", nodeStatus.Message)
}

func TestStreamConfigurationRecordsSnapshotSendDurationMetric(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	metrics := observability.NewMetrics()
	nodes := noderegistry.NewRegistry(ir.NewNodeStatusStore(), nil, logger, noderegistry.Options{})
	defer nodes.Close()

	server, err := New(
		":18080",
		config.GRPCTLSConfig{},
		config.GRPCRuntimeConfig{},
		store,
		nodes,
		logger,
		metrics,
	)
	require.NoError(t, err, "New returned error")

	stream := newFakeConfigStream()
	stream.setSendDelay(15 * time.Millisecond)
	result := make(chan error, 1)
	go func() {
		result <- server.StreamConfiguration(stream)
	}()

	waitForNodeConnection(t, nodes, "dp-1")
	store.Publish(&ir.Snapshot{ID: "v-metric", GeneratedAt: time.Now().UTC()})
	waitForHistogramSampleCount(t, metrics.XDSSnapshotSendDurationSeconds, 1, time.Second)

	assert.Equal(t, float64(0), testutil.ToFloat64(metrics.XDSSnapshotSendTimeoutsTotal), "unexpected send timeout count")
	assert.True(t, histogramSampleSum(t, metrics.XDSSnapshotSendDurationSeconds) > 0, "expected positive send duration sum")

	stream.release()
	select {
	case err := <-result:
		require.NoError(t, err, "expected stream to exit cleanly")
	case <-time.After(time.Second):
		t.Fatal("StreamConfiguration did not return after stream release")
	}
}

func TestStreamConfigurationRecordsSnapshotSendTimeoutMetric(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	metrics := observability.NewMetrics()
	nodes := noderegistry.NewRegistry(ir.NewNodeStatusStore(), nil, logger, noderegistry.Options{})
	defer nodes.Close()

	server, err := New(
		":18080",
		config.GRPCTLSConfig{},
		config.GRPCRuntimeConfig{SnapshotSendTimeout: "25ms"},
		store,
		nodes,
		logger,
		metrics,
	)
	require.NoError(t, err, "New returned error")

	stream := newFakeConfigStream()
	stream.blockSend()
	result := make(chan error, 1)
	go func() {
		result <- server.StreamConfiguration(stream)
	}()

	waitForNodeConnection(t, nodes, "dp-1")
	store.Publish(&ir.Snapshot{ID: "v-timeout-metric", GeneratedAt: time.Now().UTC()})
	stream.waitForSendStart(t)

	select {
	case err := <-result:
		assert.Equal(t, codes.DeadlineExceeded, status.Code(err), "expected deadline exceeded status on blocked send timeout")
	case <-time.After(200 * time.Millisecond):
		t.Fatal("StreamConfiguration did not return after blocked send timed out")
	}

	assert.Equal(t, float64(1), testutil.ToFloat64(metrics.XDSSnapshotSendTimeoutsTotal), "snapshot send timeout count")
	assert.Equal(t, uint64(1), histogramSampleCount(t, metrics.XDSSnapshotSendDurationSeconds), "snapshot send duration sample count")
	assert.Equal(t, float64(1), testutil.ToFloat64(metrics.XDSStreamTerminationsTotal.WithLabelValues("send_timeout")), "send timeout stream termination count")

	stream.releaseSend()
	stream.release()
}

func TestStreamConfigurationTimesOutStaleSnapshotAck(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	metrics := observability.NewMetrics()
	nodes := noderegistry.NewRegistry(ir.NewNodeStatusStore(), nil, logger, noderegistry.Options{Metrics: metrics})
	defer nodes.Close()

	server, err := New(
		":18080",
		config.GRPCTLSConfig{},
		config.GRPCRuntimeConfig{SnapshotAckTimeout: "25ms"},
		store,
		nodes,
		logger,
		metrics,
	)
	require.NoError(t, err, "New returned error")

	stream := newFakeConfigStream()
	result := make(chan error, 1)
	go func() {
		result <- server.StreamConfiguration(stream)
	}()

	waitForNodeConnection(t, nodes, "dp-1")
	store.Publish(&ir.Snapshot{ID: "v-stale", GeneratedAt: time.Now().UTC()})

	select {
	case err := <-result:
		assert.Equal(t, codes.DeadlineExceeded, status.Code(err), "expected deadline exceeded status on stale snapshot ack timeout")
	case <-time.After(200 * time.Millisecond):
		t.Fatal("StreamConfiguration did not return after stale snapshot ack timeout")
	}

	assert.Equal(t, float64(1), testutil.ToFloat64(metrics.XDSSnapshotAckTimeoutsTotal), "snapshot ack timeout count")
	assert.Equal(t, float64(1), testutil.ToFloat64(metrics.XDSStreamTerminationsTotal.WithLabelValues("ack_timeout")), "ack timeout stream termination count")

	nodeStatus, ok := nodes.Get(context.Background(), "dp-1")
	require.True(t, ok, "expected node status to be recorded")
	assert.False(t, nodeStatus.Connected, "expected node to be disconnected after stale snapshot ack timeout")
	assert.False(t, nodeStatus.Ready, "expected ack timeout disconnect to clear readiness")
	assert.Equal(t, "ack_timeout", nodeStatus.DisconnectReason)
	assert.Equal(t, "timed out waiting for dataplane snapshot ack", nodeStatus.Message)

	stream.release()
}

func TestStreamConfigurationMatchingAckPreventsStaleSnapshotTimeout(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	metrics := observability.NewMetrics()
	nodes := noderegistry.NewRegistry(ir.NewNodeStatusStore(), nil, logger, noderegistry.Options{Metrics: metrics})
	defer nodes.Close()

	server, err := New(
		":18080",
		config.GRPCTLSConfig{},
		config.GRPCRuntimeConfig{SnapshotAckTimeout: "80ms"},
		store,
		nodes,
		logger,
		metrics,
	)
	require.NoError(t, err, "New returned error")

	stream := newFakeConfigStream()
	result := make(chan error, 1)
	go func() {
		result <- server.StreamConfiguration(stream)
	}()

	waitForNodeConnection(t, nodes, "dp-1")
	snapshot := &ir.Snapshot{GeneratedAt: time.Now().UTC()}
	store.Publish(snapshot)
	stream.pushRecv(&controlv1.DiscoveryRequest{
		NodeId:        "dp-1",
		Cluster:       "default",
		Version:       snapshot.ID,
		Nonce:         snapshot.ID,
		Subscriptions: []string{"*"},
		ResultStatus:  controlv1.DiscoveryResultStatus_DISCOVERY_RESULT_STATUS_ACK,
	})

	select {
	case err := <-result:
		t.Fatalf("expected stream to remain open after matching ack, got %v", err)
	case <-time.After(140 * time.Millisecond):
	}

	assert.Equal(t, float64(0), testutil.ToFloat64(metrics.XDSSnapshotAckTimeoutsTotal), "unexpected snapshot ack timeout count")

	stream.release()
	select {
	case err := <-result:
		require.NoError(t, err, "expected stream to exit cleanly after release")
	case <-time.After(time.Second):
		t.Fatal("StreamConfiguration did not return after stream release")
	}
	assert.Equal(t, float64(1), testutil.ToFloat64(metrics.XDSStreamTerminationsTotal.WithLabelValues("client_disconnect")), "client disconnect stream termination count")

	nodeStatus, ok := nodes.Get(context.Background(), "dp-1")
	require.True(t, ok, "expected node status after client disconnect")
	assert.False(t, nodeStatus.Connected, "expected node to be disconnected after client disconnect")
	assert.False(t, nodeStatus.Ready, "expected client disconnect to clear readiness")
	assert.Equal(t, "client_disconnect", nodeStatus.DisconnectReason)
	assert.Equal(t, "xds stream closed by dataplane", nodeStatus.Message)
}

func TestStreamConfigurationSendsIdleHeartbeatWithoutSnapshotAckTimeout(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	metrics := observability.NewMetrics()
	nodes := noderegistry.NewRegistry(ir.NewNodeStatusStore(), nil, logger, noderegistry.Options{Metrics: metrics})
	defer nodes.Close()

	server, err := New(
		":18080",
		config.GRPCTLSConfig{},
		config.GRPCRuntimeConfig{
			KeepaliveTime:      "30ms",
			SnapshotAckTimeout: "50ms",
		},
		store,
		nodes,
		logger,
		metrics,
	)
	require.NoError(t, err, "New returned error")

	stream := newFakeConfigStream()
	result := make(chan error, 1)
	go func() {
		result <- server.StreamConfiguration(stream)
	}()

	waitForNodeConnection(t, nodes, "dp-1")
	stream.waitForSendCount(t, 200*time.Millisecond)

	responses := stream.snapshotSentResponses()
	require.Len(t, responses, 1, "expected 1 heartbeat response")
	assert.Empty(t, responses[0].GetVersion(), "expected heartbeat version to be empty")
	assert.Empty(t, responses[0].GetNonce(), "expected heartbeat nonce to be empty")
	assert.Nil(t, responses[0].GetSnapshot(), "expected heartbeat snapshot to be nil")

	select {
	case err := <-result:
		t.Fatalf("expected stream to remain open across idle heartbeat interval, got %v", err)
	case <-time.After(90 * time.Millisecond):
	}

	assert.Equal(t, float64(0), testutil.ToFloat64(metrics.XDSSnapshotAckTimeoutsTotal), "unexpected snapshot ack timeout count")
	assert.Equal(t, uint64(0), histogramSampleCount(t, metrics.XDSSnapshotSendDurationSeconds), "idle heartbeat should not record snapshot send duration samples")
	assert.Equal(t, float64(0), testutil.ToFloat64(metrics.XDSSnapshotSendTimeoutsTotal), "idle heartbeat should not record snapshot send timeouts")

	stream.release()
	select {
	case err := <-result:
		require.NoError(t, err, "expected stream to exit cleanly after release")
	case <-time.After(time.Second):
		t.Fatal("StreamConfiguration did not return after stream release")
	}
}

func TestStreamConfigurationPublishesDifferentSnapshotVariantsPerCapabilityProfile(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	metrics := observability.NewMetrics()
	nodes := noderegistry.NewRegistry(ir.NewNodeStatusStore(), nil, logger, noderegistry.Options{Metrics: metrics})
	defer nodes.Close()

	server, err := New(":18080", config.GRPCTLSConfig{}, config.GRPCRuntimeConfig{}, store, nodes, logger, metrics)
	require.NoError(t, err, "New returned error")

	fullStream := newFakeConfigStream()
	fullStream.initialRecv <- &controlv1.DiscoveryRequest{
		NodeId:        "dp-full",
		Cluster:       "default",
		Subscriptions: []string{"*"},
		SupportedFeatures: []string{
			featureCoreV1,
			featureRouteLabelsV1,
			featureBackendAIServiceV1,
			featureBackendTokenPolicyV1,
			featureBackendWasmPluginV1,
		},
	}
	coreOnlyStream := newFakeConfigStream()
	coreOnlyStream.initialRecv <- &controlv1.DiscoveryRequest{
		NodeId:            "dp-core",
		Cluster:           "default",
		Subscriptions:     []string{"*"},
		SupportedFeatures: []string{featureCoreV1},
	}

	fullResult := make(chan error, 1)
	go func() {
		fullResult <- server.StreamConfiguration(fullStream)
	}()
	coreOnlyResult := make(chan error, 1)
	go func() {
		coreOnlyResult <- server.StreamConfiguration(coreOnlyStream)
	}()

	waitForNodeConnection(t, nodes, "dp-full")
	waitForNodeConnection(t, nodes, "dp-core")

	store.Publish(projectionTestSnapshot())

	fullStream.waitForSendCount(t, time.Second)
	coreOnlyStream.waitForSendCount(t, time.Second)

	fullResponses := fullStream.snapshotSentResponses()
	coreOnlyResponses := coreOnlyStream.snapshotSentResponses()
	require.Len(t, fullResponses, 1)
	require.Len(t, coreOnlyResponses, 1)

	fullSnapshot := fullResponses[0].GetSnapshot()
	coreOnlySnapshot := coreOnlyResponses[0].GetSnapshot()

	assert.Equal(t, compatibilityProfileFullV1, fullSnapshot.GetCompatibilityProfile())
	wantCoreOnlyProfile := buildCompatibilityProfile([]string{featureCoreV1})
	assert.Equal(t, wantCoreOnlyProfile, coreOnlySnapshot.GetCompatibilityProfile())

	assert.Equal(t, "prod", findProjectedHTTPRoute(t, fullSnapshot, "http-labeled").GetLabels()["env"])
	assert.Nil(t, findProjectedHTTPRoute(t, coreOnlySnapshot, "http-labeled").GetLabels(), "core-only stream http labels should be nil")

	require.Len(t, fullSnapshot.GetBackends(), 4)
	require.Len(t, coreOnlySnapshot.GetBackends(), 1)
	require.NotNil(t, findProjectedBackend(t, fullSnapshot, "ai-backend").GetAiService(), "expected full stream to preserve ai service backend")
	require.Len(t, coreOnlySnapshot.GetListeners(), 1)
	assert.Equal(t, []string{"default/http-direct-response", "default/http-labeled"}, coreOnlySnapshot.GetListeners()[0].GetAttachedRoutes())

	require.Len(t, coreOnlySnapshot.GetGrpcRoutes(), 0, "core-only grpc routes should be empty")
	require.Len(t, coreOnlySnapshot.GetStreamRoutes(), 0, "core-only stream routes should be empty")

	httpDirectResponse := findProjectedHTTPRoute(t, coreOnlySnapshot, "http-direct-response")
	require.Len(t, httpDirectResponse.GetRules(), 1)
	require.Len(t, httpDirectResponse.GetRules()[0].GetBackendRefs(), 0, "core-only direct-response backends should be empty")
	assert.False(t, hasProjectedHTTPRoute(coreOnlySnapshot, "http-ai-only"), "expected core-only stream to prune http-ai-only route")

	fullStream.release()
	coreOnlyStream.release()

	select {
	case err := <-fullResult:
		require.NoError(t, err, "expected full stream to exit cleanly after release")
	case <-time.After(time.Second):
		t.Fatal("full stream did not return after release")
	}
	select {
	case err := <-coreOnlyResult:
		require.NoError(t, err, "expected core-only stream to exit cleanly after release")
	case <-time.After(time.Second):
		t.Fatal("core-only stream did not return after release")
	}
}

func waitForNodeConnection(t *testing.T, nodes *noderegistry.Registry, nodeID string) {
	t.Helper()

	deadline := time.After(time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		status, ok := nodes.Get(context.Background(), nodeID)
		if ok && status.Connected {
			return
		}

		select {
		case <-deadline:
			t.Fatalf("timed out waiting for node %q to connect", nodeID)
		case <-ticker.C:
		}
	}
}

type fakeConfigStream struct {
	ctx         context.Context
	cancel      context.CancelFunc
	recvRelease chan struct{}
	recvQueue   chan *controlv1.DiscoveryRequest
	initialRecv chan *controlv1.DiscoveryRequest
	blockFirst  bool
	sendRelease chan struct{}
	sendStarted chan struct{}
	sendOnce    sync.Once
	sendMu      sync.Mutex
	sent        []*controlv1.DiscoveryResponse
	sendNotify  chan struct{}
	sendDelay   time.Duration
	recvCount   int
}

func newFakeConfigStream() *fakeConfigStream {
	ctx, cancel := context.WithCancel(context.Background())
	return &fakeConfigStream{
		ctx:         ctx,
		cancel:      cancel,
		recvRelease: make(chan struct{}),
		recvQueue:   make(chan *controlv1.DiscoveryRequest, 8),
		initialRecv: make(chan *controlv1.DiscoveryRequest, 1),
		sendStarted: make(chan struct{}),
		sendNotify:  make(chan struct{}, 32),
	}
}

func (f *fakeConfigStream) Send(response *controlv1.DiscoveryResponse) error {
	f.sendOnce.Do(func() {
		close(f.sendStarted)
	})
	if f.sendRelease == nil {
		f.recordSend(response)
		return nil
	}

	select {
	case <-f.sendRelease:
		if f.sendDelay > 0 {
			time.Sleep(f.sendDelay)
		}
		f.recordSend(response)
		return nil
	case <-f.ctx.Done():
		return status.Error(codes.Canceled, "stream closed")
	}
}

func (f *fakeConfigStream) blockSend() {
	if f.sendRelease == nil {
		f.sendRelease = make(chan struct{})
	}
}

func (f *fakeConfigStream) setSendDelay(delay time.Duration) {
	f.sendDelay = delay
	f.sendRelease = make(chan struct{})
	close(f.sendRelease)
}

func (f *fakeConfigStream) waitForSendStart(t *testing.T) {
	t.Helper()

	select {
	case <-f.sendStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for stream send to start")
	}
}

func (f *fakeConfigStream) releaseSend() {
	if f.sendRelease == nil {
		return
	}

	select {
	case <-f.sendRelease:
	default:
		close(f.sendRelease)
	}
}

func (f *fakeConfigStream) recordSend(response *controlv1.DiscoveryResponse) {
	f.sendMu.Lock()
	f.sent = append(f.sent, response)
	f.sendMu.Unlock()

	select {
	case f.sendNotify <- struct{}{}:
	default:
	}
}

func (f *fakeConfigStream) waitForSendCount(t *testing.T, timeout time.Duration) {
	t.Helper()

	deadline := time.After(timeout)
	for {
		if len(f.snapshotSentResponses()) >= 1 {
			return
		}

		select {
		case <-deadline:
			t.Fatalf("timed out waiting for sends")
		case <-f.sendNotify:
		}
	}
}

func (f *fakeConfigStream) snapshotSentResponses() []*controlv1.DiscoveryResponse {
	f.sendMu.Lock()
	defer f.sendMu.Unlock()

	out := make([]*controlv1.DiscoveryResponse, len(f.sent))
	copy(out, f.sent)
	return out
}

func (f *fakeConfigStream) Recv() (*controlv1.DiscoveryRequest, error) {
	if f.recvCount == 0 {
		f.recvCount++
		if f.blockFirst {
			select {
			case req := <-f.initialRecv:
				return req, nil
			case <-f.recvRelease:
				return nil, io.EOF
			case <-f.ctx.Done():
				return nil, io.EOF
			}
		}
		select {
		case req := <-f.initialRecv:
			return req, nil
		default:
		}
		return &controlv1.DiscoveryRequest{
			NodeId:        "dp-1",
			Cluster:       "default",
			Subscriptions: []string{"*"},
		}, nil
	}

	select {
	case req := <-f.recvQueue:
		return req, nil
	case <-f.recvRelease:
		return nil, io.EOF
	case <-f.ctx.Done():
		return nil, io.EOF
	}
}

func (f *fakeConfigStream) blockInitialRecv() {
	f.blockFirst = true
	f.initialRecv = make(chan *controlv1.DiscoveryRequest)
}

func (f *fakeConfigStream) pushRecv(req *controlv1.DiscoveryRequest) {
	select {
	case f.recvQueue <- req:
	case <-f.ctx.Done():
	}
}

func (f *fakeConfigStream) release() {
	f.cancel()
	close(f.recvRelease)
}

func (f *fakeConfigStream) SetHeader(metadata.MD) error {
	return nil
}

func (f *fakeConfigStream) SendHeader(metadata.MD) error {
	return nil
}

func (f *fakeConfigStream) SetTrailer(metadata.MD) {}

func (f *fakeConfigStream) Context() context.Context {
	return f.ctx
}

func (f *fakeConfigStream) SendMsg(any) error {
	return nil
}

func (f *fakeConfigStream) RecvMsg(any) error {
	return nil
}

func waitForHistogramSampleCount(t *testing.T, histogram prometheus.Histogram, want uint64, timeout time.Duration) {
	t.Helper()

	deadline := time.After(timeout)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		if got := histogramSampleCount(t, histogram); got == want {
			return
		}

		select {
		case <-deadline:
			t.Fatalf("timed out waiting for histogram sample count %d", want)
		case <-ticker.C:
		}
	}
}

func histogramSampleCount(t *testing.T, histogram prometheus.Histogram) uint64 {
	t.Helper()

	metric, ok := histogram.(prometheus.Metric)
	if !ok {
		t.Fatal("histogram does not implement prometheus.Metric")
	}

	dtoMetric := &dto.Metric{}
	if err := metric.Write(dtoMetric); err != nil {
		t.Fatalf("write histogram metric: %v", err)
	}
	return dtoMetric.GetHistogram().GetSampleCount()
}

func histogramSampleSum(t *testing.T, histogram prometheus.Histogram) float64 {
	t.Helper()

	metric, ok := histogram.(prometheus.Metric)
	if !ok {
		t.Fatal("histogram does not implement prometheus.Metric")
	}

	dtoMetric := &dto.Metric{}
	if err := metric.Write(dtoMetric); err != nil {
		t.Fatalf("write histogram metric: %v", err)
	}
	return dtoMetric.GetHistogram().GetSampleSum()
}