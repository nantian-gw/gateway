package xds

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/nantian-gw/gateway/internal/config"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/nodeinfo"
	"github.com/nantian-gw/gateway/internal/observability"
	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"
)

func TestToListenerProtocolMapsTLS(t *testing.T) {
	if got := toListenerProtocol("TLS"); got != controlv1.ListenerProtocol_LISTENER_PROTOCOL_TLS {
		t.Fatalf("expected TLS to map to LISTENER_PROTOCOL_TLS, got %v", got)
	}
	if got := toListenerProtocol("TLS_PASSTHROUGH"); got != controlv1.ListenerProtocol_LISTENER_PROTOCOL_TLS_PASSTHROUGH {
		t.Fatalf("expected TLS_PASSTHROUGH to map to passthrough, got %v", got)
	}
}

func TestToProtoRouteTimeouts(t *testing.T) {
	request := 12 * time.Second
	backendRequest := 3 * time.Second

	timeouts := toProtoRouteTimeouts(&ir.RouteTimeouts{
		Request:        &request,
		BackendRequest: &backendRequest,
	})

	if timeouts == nil {
		t.Fatal("expected route timeouts")
	}
	if timeouts.Request.AsDuration() != 12*time.Second {
		t.Fatalf("unexpected request timeout: %v", timeouts.Request)
	}
	if timeouts.BackendRequest.AsDuration() != 3*time.Second {
		t.Fatalf("unexpected backend request timeout: %v", timeouts.BackendRequest)
	}
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
	if len(out.Backends) != 1 {
		t.Fatalf("expected 1 backend, got %d", len(out.Backends))
	}
	if out.Backends[0].ConnectTimeout == nil || out.Backends[0].ConnectTimeout.AsDuration() != 5*time.Second {
		t.Fatalf("unexpected connect timeout: %v", out.Backends[0].ConnectTimeout)
	}
	if out.Backends[0].RequestTimeout != nil {
		t.Fatalf("expected zero request timeout to be omitted, got %v", out.Backends[0].RequestTimeout)
	}
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

	if len(backends) != 1 {
		t.Fatalf("expected 1 backend ref, got %d", len(backends))
	}
	if len(backends[0].Filters) != 1 {
		t.Fatalf("expected 1 backend filter, got %d", len(backends[0].Filters))
	}
	if got := backends[0].Filters[0].GetType(); got != "RequestHeaderModifier" {
		t.Fatalf("unexpected backend filter type: %q", got)
	}
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

	if len(filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(filters))
	}
	if filters[0].GetType() != "ExtensionRef" {
		t.Fatalf("unexpected filter type: %q", filters[0].GetType())
	}
	directResponse := filters[0].GetConfig().GetFields()["directResponse"].GetStructValue()
	if directResponse == nil {
		t.Fatalf("expected nested directResponse config, got %#v", filters[0].GetConfig())
	}
	if got := directResponse.Fields["statusCode"].GetNumberValue(); got != 503 {
		t.Fatalf("unexpected status code: %v", got)
	}
	if got := directResponse.Fields["body"].GetStringValue(); got != "maintenance" {
		t.Fatalf("unexpected body: %q", got)
	}
}

func TestToProtoSnapshotPreservesWorkloadExtensions(t *testing.T) {
	snapshot := toProtoSnapshot(&ir.Snapshot{
		Workloads: []ir.Workload{{
			Namespace: "gateway-conformance-mesh-consumer",
			Name:      "echo-v1",
			IP:        "10.0.0.10",
		}},
	})

	if snapshot.Extensions == nil {
		t.Fatal("expected extensions to be populated")
	}

	workloads, ok := snapshot.Extensions.Fields["workloads"]
	if !ok {
		t.Fatalf("expected workloads extension, got %#v", snapshot.Extensions.Fields)
	}
	if len(workloads.GetListValue().GetValues()) != 1 {
		t.Fatalf("expected 1 workload, got %#v", workloads)
	}
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

	if len(filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(filters))
	}
	if filters[0].GetType() != "ExtensionRef" {
		t.Fatalf("unexpected filter type: %q", filters[0].GetType())
	}
	if filters[0].GetConfig() == nil {
		t.Fatal("expected fallback empty struct, got nil")
	}
	if len(filters[0].GetConfig().GetFields()) != 0 {
		t.Fatalf("expected empty fallback struct, got %#v", filters[0].GetConfig().GetFields())
	}
	if !strings.Contains(logs.String(), "failed to build filter config struct") {
		t.Fatalf("expected warning log, got %q", logs.String())
	}
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

	if snapshot.Extensions != nil {
		t.Fatalf("expected nil extensions fallback, got %#v", snapshot.Extensions)
	}
	if !strings.Contains(logs.String(), "failed to build snapshot extensions struct") {
		t.Fatalf("expected warning log, got %q", logs.String())
	}
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
	if payload == nil {
		t.Fatal("expected fallback empty struct")
	}
	if len(payload.GetFields()) != 0 {
		t.Fatalf("expected empty fields, got %#v", payload.GetFields())
	}
	if !strings.Contains(logs.String(), "failed to build empty struct") {
		t.Fatalf("expected warning log, got %q", logs.String())
	}
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

	if got := snapshot.HttpRoutes[0].Annotations["gateway.nantian.dev/access-log-mode"]; got != "json" {
		t.Fatalf("expected http route annotation, got %q", got)
	}
	if got := snapshot.Listeners[0].Addresses; len(got) != 2 || got[0] != "192.0.2.10" || got[1] != "gw.example.com" {
		t.Fatalf("expected listener addresses, got %#v", got)
	}
	if got := snapshot.GrpcRoutes[0].Annotations["gateway.nantian.dev/access-log-enabled"]; got != "false" {
		t.Fatalf("expected grpc route annotation, got %q", got)
	}
	if got := snapshot.StreamRoutes[0].Annotations["gateway.nantian.dev/access-log-path"]; got != "/var/log/nantian-gw/tcp.log" {
		t.Fatalf("expected stream route annotation, got %q", got)
	}
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

	if item == nil {
		t.Fatal("expected session persistence proto")
	}
	if item.SessionName != "nantian-gw-http-session" {
		t.Fatalf("unexpected session name: %s", item.SessionName)
	}
	if item.Type != controlv1.SessionPersistenceType_SESSION_PERSISTENCE_TYPE_COOKIE {
		t.Fatalf("unexpected session type: %v", item.Type)
	}
	if item.Cookie == nil || item.Cookie.LifetimeType != controlv1.CookieLifetimeType_COOKIE_LIFETIME_TYPE_PERMANENT {
		t.Fatalf("unexpected cookie config: %#v", item.Cookie)
	}
	if item.AbsoluteTimeout.AsDuration() != 5*time.Minute {
		t.Fatalf("unexpected absolute timeout: %v", item.AbsoluteTimeout)
	}
	if item.IdleTimeout.AsDuration() != 30*time.Second {
		t.Fatalf("unexpected idle timeout: %v", item.IdleTimeout)
	}
}

func TestToProtoLoadBalancing(t *testing.T) {
	item := toProtoLoadBalancing(&ir.LoadBalancingPolicy{
		Type: "ConsistentHash",
		ConsistentHash: &ir.ConsistentHashPolicy{
			KeyType:    "Header",
			HeaderName: "x-user-id",
		},
	})

	if item == nil {
		t.Fatal("expected load balancing proto")
	}
	if item.Type != controlv1.LoadBalancingPolicyType_LOAD_BALANCING_POLICY_TYPE_CONSISTENT_HASH {
		t.Fatalf("unexpected load balancing type: %v", item.Type)
	}
	if item.ConsistentHash == nil {
		t.Fatal("expected consistent hash proto")
	}
	if item.ConsistentHash.KeyType != controlv1.ConsistentHashKeyType_CONSISTENT_HASH_KEY_TYPE_HEADER {
		t.Fatalf("unexpected consistent hash key type: %v", item.ConsistentHash.KeyType)
	}
	if item.ConsistentHash.HeaderName != "x-user-id" {
		t.Fatalf("unexpected header name: %q", item.ConsistentHash.HeaderName)
	}
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

	if len(snapshot.GrpcRoutes) != 1 || len(snapshot.GrpcRoutes[0].Rules) != 1 {
		t.Fatalf("unexpected grpc routes: %#v", snapshot.GrpcRoutes)
	}
	if len(snapshot.GrpcRoutes[0].Rules[0].Matches) != 1 {
		t.Fatalf("unexpected grpc matches: %#v", snapshot.GrpcRoutes[0].Rules[0].Matches)
	}
	if got := snapshot.GrpcRoutes[0].Rules[0].Matches[0].MatchType; got != "RegularExpression" {
		t.Fatalf("expected regex match type, got %q", got)
	}
}

func TestStreamConfigurationDrainsOnServerShutdown(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	metrics := observability.NewMetrics()
	nodes := nodeinfo.NewRegistry(ir.NewNodeStatusStore(), nil, logger, nodeinfo.Options{})
	defer nodes.Close()

	server, err := New(":18080", config.GRPCTLSConfig{}, config.GRPCRuntimeConfig{}, store, nodes, logger, metrics)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	server.signalShutdown()

	stream := newFakeConfigStream()
	result := make(chan error, 1)
	go func() {
		result <- server.StreamConfiguration(stream)
	}()

	select {
	case err := <-result:
		if status.Code(err) != codes.Unavailable {
			t.Fatalf("expected unavailable status on shutdown, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("StreamConfiguration did not return after shutdown")
	}

	stream.release()
	if _, ok := nodes.Get(context.Background(), "dp-1"); ok {
		t.Fatal("expected blocked handshake stream to avoid recording node status after shutdown")
	}
	if got := testutil.ToFloat64(metrics.XDSStreamTerminationsTotal.WithLabelValues("shutdown")); got != 1 {
		t.Fatalf("shutdown stream termination count = %v, want 1", got)
	}
}

func TestStreamConfigurationInterruptsBlockedInitialRecvOnShutdown(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	metrics := observability.NewMetrics()
	nodes := nodeinfo.NewRegistry(ir.NewNodeStatusStore(), nil, logger, nodeinfo.Options{})
	defer nodes.Close()

	server, err := New(":18080", config.GRPCTLSConfig{}, config.GRPCRuntimeConfig{}, store, nodes, logger, metrics)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	stream := newFakeConfigStream()
	stream.blockInitialRecv()
	result := make(chan error, 1)
	go func() {
		result <- server.StreamConfiguration(stream)
	}()

	server.signalShutdown()

	select {
	case err := <-result:
		if status.Code(err) != codes.Unavailable {
			t.Fatalf("expected unavailable status on shutdown during initial recv, got %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("StreamConfiguration did not return after shutdown while initial recv was blocked")
	}

	stream.release()
	if got := testutil.ToFloat64(metrics.XDSStreamTerminationsTotal.WithLabelValues("shutdown")); got != 1 {
		t.Fatalf("shutdown stream termination count = %v, want 1", got)
	}
}

func TestStreamConfigurationInterruptsBlockedSendOnShutdown(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	nodes := nodeinfo.NewRegistry(ir.NewNodeStatusStore(), nil, logger, nodeinfo.Options{})
	defer nodes.Close()

	server, err := New(":18080", config.GRPCTLSConfig{}, config.GRPCRuntimeConfig{}, store, nodes, logger, nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

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
		if status.Code(err) != codes.Unavailable {
			t.Fatalf("expected unavailable status on shutdown, got %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("StreamConfiguration did not return after shutdown while send was blocked")
	}

	stream.releaseSend()
	stream.release()

	nodeStatus, ok := nodes.Get(context.Background(), "dp-1")
	if !ok {
		t.Fatal("expected node status to be recorded")
	}
	if nodeStatus.Connected {
		t.Fatalf("expected node to be disconnected after shutdown, got %#v", nodeStatus)
	}
	if nodeStatus.Ready {
		t.Fatalf("expected shutdown disconnect to clear readiness, got %#v", nodeStatus)
	}
	if nodeStatus.DisconnectReason != "shutdown" {
		t.Fatalf("expected shutdown disconnect reason, got %#v", nodeStatus)
	}
	if nodeStatus.Message != "xds stream drained for controlplane shutdown" {
		t.Fatalf("expected shutdown disconnect message, got %#v", nodeStatus)
	}
}

func TestStreamConfigurationSupersedesExistingNodeStream(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	metrics := observability.NewMetrics()
	nodes := nodeinfo.NewRegistry(ir.NewNodeStatusStore(), nil, logger, nodeinfo.Options{})
	defer nodes.Close()

	server, err := New(":18080", config.GRPCTLSConfig{}, config.GRPCRuntimeConfig{}, store, nodes, logger, metrics)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

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
		if status.Code(err) != codes.Unavailable {
			t.Fatalf("expected unavailable status on superseded stream, got %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("old stream did not return after replacement stream connected")
	}

	nodeStatus, ok := nodes.Get(context.Background(), "dp-1")
	if !ok {
		t.Fatal("expected node status to be recorded")
	}
	if !nodeStatus.Connected {
		t.Fatalf("expected node to remain connected after stream replacement, got %#v", nodeStatus)
	}
	if got := testutil.ToFloat64(metrics.XDSStreamTerminationsTotal.WithLabelValues("superseded")); got != 1 {
		t.Fatalf("superseded stream termination count = %v, want 1", got)
	}

	oldStream.release()
	newStream.release()

	select {
	case err := <-newResult:
		if err != nil {
			t.Fatalf("expected replacement stream to exit cleanly after release, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("replacement stream did not return after release")
	}
}

func TestStreamConfigurationInterruptsBlockedSendOnSupersededStream(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	metrics := observability.NewMetrics()
	nodes := nodeinfo.NewRegistry(ir.NewNodeStatusStore(), nil, logger, nodeinfo.Options{})
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
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

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
		if status.Code(err) != codes.Unavailable {
			t.Fatalf("expected unavailable status on superseded blocked stream, got %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("blocked send stream did not return after replacement stream connected")
	}

	if got := testutil.ToFloat64(metrics.XDSSnapshotSendTimeoutsTotal); got != 0 {
		t.Fatalf("unexpected send timeout count for superseded stream: %v", got)
	}
	if got := testutil.ToFloat64(metrics.XDSStreamTerminationsTotal.WithLabelValues("superseded")); got != 1 {
		t.Fatalf("superseded stream termination count = %v, want 1", got)
	}

	nodeStatus, ok := nodes.Get(context.Background(), "dp-1")
	if !ok {
		t.Fatal("expected node status to be recorded")
	}
	if !nodeStatus.Connected {
		t.Fatalf("expected node to remain connected after superseding blocked stream, got %#v", nodeStatus)
	}

	oldStream.releaseSend()
	oldStream.release()
	newStream.release()

	select {
	case err := <-newResult:
		if err != nil {
			t.Fatalf("expected replacement stream to exit cleanly after release, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("replacement stream did not return after release")
	}
}

func TestStreamConfigurationTimesOutBlockedSend(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	nodes := nodeinfo.NewRegistry(ir.NewNodeStatusStore(), nil, logger, nodeinfo.Options{})
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
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

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
		if status.Code(err) != codes.DeadlineExceeded {
			t.Fatalf("expected deadline exceeded status on blocked send timeout, got %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("StreamConfiguration did not return after blocked send timed out")
	}

	stream.releaseSend()
	stream.release()

	nodeStatus, ok := nodes.Get(context.Background(), "dp-1")
	if !ok {
		t.Fatal("expected node status to be recorded")
	}
	if nodeStatus.Connected {
		t.Fatalf("expected node to be disconnected after blocked send timeout, got %#v", nodeStatus)
	}
	if nodeStatus.Ready {
		t.Fatalf("expected send timeout disconnect to clear readiness, got %#v", nodeStatus)
	}
	if nodeStatus.DisconnectReason != "send_timeout" {
		t.Fatalf("expected send timeout disconnect reason, got %#v", nodeStatus)
	}
	if nodeStatus.Message != "timed out sending snapshot to dataplane" {
		t.Fatalf("expected send timeout disconnect message, got %#v", nodeStatus)
	}
}

func TestStreamConfigurationRecordsSnapshotSendDurationMetric(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	metrics := observability.NewMetrics()
	nodes := nodeinfo.NewRegistry(ir.NewNodeStatusStore(), nil, logger, nodeinfo.Options{})
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
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	stream := newFakeConfigStream()
	stream.setSendDelay(15 * time.Millisecond)
	result := make(chan error, 1)
	go func() {
		result <- server.StreamConfiguration(stream)
	}()

	waitForNodeConnection(t, nodes, "dp-1")
	store.Publish(&ir.Snapshot{ID: "v-metric", GeneratedAt: time.Now().UTC()})
	waitForHistogramSampleCount(t, metrics.XDSSnapshotSendDurationSeconds, 1, time.Second)

	if got := testutil.ToFloat64(metrics.XDSSnapshotSendTimeoutsTotal); got != 0 {
		t.Fatalf("unexpected send timeout count: %v", got)
	}
	if got := histogramSampleSum(t, metrics.XDSSnapshotSendDurationSeconds); got <= 0 {
		t.Fatalf("expected positive send duration sum, got %v", got)
	}

	stream.release()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("expected stream to exit cleanly, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("StreamConfiguration did not return after stream release")
	}
}

func TestStreamConfigurationRecordsSnapshotSendTimeoutMetric(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	metrics := observability.NewMetrics()
	nodes := nodeinfo.NewRegistry(ir.NewNodeStatusStore(), nil, logger, nodeinfo.Options{})
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
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

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
		if status.Code(err) != codes.DeadlineExceeded {
			t.Fatalf("expected deadline exceeded status on blocked send timeout, got %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("StreamConfiguration did not return after blocked send timed out")
	}

	if got := testutil.ToFloat64(metrics.XDSSnapshotSendTimeoutsTotal); got != 1 {
		t.Fatalf("snapshot send timeout count = %v, want 1", got)
	}
	if got := histogramSampleCount(t, metrics.XDSSnapshotSendDurationSeconds); got != 1 {
		t.Fatalf("snapshot send duration sample count = %d, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.XDSStreamTerminationsTotal.WithLabelValues("send_timeout")); got != 1 {
		t.Fatalf("send timeout stream termination count = %v, want 1", got)
	}

	stream.releaseSend()
	stream.release()
}

func TestStreamConfigurationTimesOutStaleSnapshotAck(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	metrics := observability.NewMetrics()
	nodes := nodeinfo.NewRegistry(ir.NewNodeStatusStore(), nil, logger, nodeinfo.Options{Metrics: metrics})
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
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	stream := newFakeConfigStream()
	result := make(chan error, 1)
	go func() {
		result <- server.StreamConfiguration(stream)
	}()

	waitForNodeConnection(t, nodes, "dp-1")
	store.Publish(&ir.Snapshot{ID: "v-stale", GeneratedAt: time.Now().UTC()})

	select {
	case err := <-result:
		if status.Code(err) != codes.DeadlineExceeded {
			t.Fatalf("expected deadline exceeded status on stale snapshot ack timeout, got %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("StreamConfiguration did not return after stale snapshot ack timeout")
	}

	if got := testutil.ToFloat64(metrics.XDSSnapshotAckTimeoutsTotal); got != 1 {
		t.Fatalf("snapshot ack timeout count = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.XDSStreamTerminationsTotal.WithLabelValues("ack_timeout")); got != 1 {
		t.Fatalf("ack timeout stream termination count = %v, want 1", got)
	}

	nodeStatus, ok := nodes.Get(context.Background(), "dp-1")
	if !ok {
		t.Fatal("expected node status to be recorded")
	}
	if nodeStatus.Connected {
		t.Fatalf("expected node to be disconnected after stale snapshot ack timeout, got %#v", nodeStatus)
	}
	if nodeStatus.Ready {
		t.Fatalf("expected ack timeout disconnect to clear readiness, got %#v", nodeStatus)
	}
	if nodeStatus.DisconnectReason != "ack_timeout" {
		t.Fatalf("expected ack timeout disconnect reason, got %#v", nodeStatus)
	}
	if nodeStatus.Message != "timed out waiting for dataplane snapshot ack" {
		t.Fatalf("expected ack timeout disconnect message, got %#v", nodeStatus)
	}

	stream.release()
}

func TestStreamConfigurationMatchingAckPreventsStaleSnapshotTimeout(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	metrics := observability.NewMetrics()
	nodes := nodeinfo.NewRegistry(ir.NewNodeStatusStore(), nil, logger, nodeinfo.Options{Metrics: metrics})
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
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

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

	if got := testutil.ToFloat64(metrics.XDSSnapshotAckTimeoutsTotal); got != 0 {
		t.Fatalf("unexpected snapshot ack timeout count: %v", got)
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
	if got := testutil.ToFloat64(metrics.XDSStreamTerminationsTotal.WithLabelValues("client_disconnect")); got != 1 {
		t.Fatalf("client disconnect stream termination count = %v, want 1", got)
	}
	nodeStatus, ok := nodes.Get(context.Background(), "dp-1")
	if !ok {
		t.Fatal("expected node status after client disconnect")
	}
	if nodeStatus.Connected {
		t.Fatalf("expected node to be disconnected after client disconnect, got %#v", nodeStatus)
	}
	if nodeStatus.Ready {
		t.Fatalf("expected client disconnect to clear readiness, got %#v", nodeStatus)
	}
	if nodeStatus.DisconnectReason != "client_disconnect" {
		t.Fatalf("expected client disconnect reason, got %#v", nodeStatus)
	}
	if nodeStatus.Message != "xds stream closed by dataplane" {
		t.Fatalf("expected client disconnect message, got %#v", nodeStatus)
	}
}

func TestStreamConfigurationSendsIdleHeartbeatWithoutSnapshotAckTimeout(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	metrics := observability.NewMetrics()
	nodes := nodeinfo.NewRegistry(ir.NewNodeStatusStore(), nil, logger, nodeinfo.Options{Metrics: metrics})
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
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	stream := newFakeConfigStream()
	result := make(chan error, 1)
	go func() {
		result <- server.StreamConfiguration(stream)
	}()

	waitForNodeConnection(t, nodes, "dp-1")
	stream.waitForSendCount(t, 1, 200*time.Millisecond)

	responses := stream.snapshotSentResponses()
	if len(responses) != 1 {
		t.Fatalf("expected 1 heartbeat response, got %d", len(responses))
	}
	if responses[0].GetVersion() != "" {
		t.Fatalf("expected heartbeat version to be empty, got %q", responses[0].GetVersion())
	}
	if responses[0].GetNonce() != "" {
		t.Fatalf("expected heartbeat nonce to be empty, got %q", responses[0].GetNonce())
	}
	if responses[0].GetSnapshot() != nil {
		t.Fatalf("expected heartbeat snapshot to be nil, got %#v", responses[0].GetSnapshot())
	}

	select {
	case err := <-result:
		t.Fatalf("expected stream to remain open across idle heartbeat interval, got %v", err)
	case <-time.After(90 * time.Millisecond):
	}

	if got := testutil.ToFloat64(metrics.XDSSnapshotAckTimeoutsTotal); got != 0 {
		t.Fatalf("unexpected snapshot ack timeout count: %v", got)
	}
	if got := histogramSampleCount(t, metrics.XDSSnapshotSendDurationSeconds); got != 0 {
		t.Fatalf("idle heartbeat should not record snapshot send duration samples, got %d", got)
	}
	if got := testutil.ToFloat64(metrics.XDSSnapshotSendTimeoutsTotal); got != 0 {
		t.Fatalf("idle heartbeat should not record snapshot send timeouts, got %v", got)
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

func TestStreamConfigurationPublishesDifferentSnapshotVariantsPerCapabilityProfile(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	metrics := observability.NewMetrics()
	nodes := nodeinfo.NewRegistry(ir.NewNodeStatusStore(), nil, logger, nodeinfo.Options{Metrics: metrics})
	defer nodes.Close()

	server, err := New(":18080", config.GRPCTLSConfig{}, config.GRPCRuntimeConfig{}, store, nodes, logger, metrics)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

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

	fullStream.waitForSendCount(t, 1, time.Second)
	coreOnlyStream.waitForSendCount(t, 1, time.Second)

	fullResponses := fullStream.snapshotSentResponses()
	coreOnlyResponses := coreOnlyStream.snapshotSentResponses()
	if len(fullResponses) != 1 || len(coreOnlyResponses) != 1 {
		t.Fatalf("unexpected send counts: full=%d core-only=%d", len(fullResponses), len(coreOnlyResponses))
	}

	fullSnapshot := fullResponses[0].GetSnapshot()
	coreOnlySnapshot := coreOnlyResponses[0].GetSnapshot()

	if got, want := fullSnapshot.GetCompatibilityProfile(), compatibilityProfileFullV1; got != want {
		t.Fatalf("full stream compatibility profile = %q, want %q", got, want)
	}
	wantCoreOnlyProfile := buildCompatibilityProfile([]string{featureCoreV1})
	if got := coreOnlySnapshot.GetCompatibilityProfile(); got != wantCoreOnlyProfile {
		t.Fatalf("core-only stream compatibility profile = %q, want %q", got, wantCoreOnlyProfile)
	}

	if got := findProjectedHTTPRoute(t, fullSnapshot, "http-labeled").GetLabels()["env"]; got != "prod" {
		t.Fatalf("full stream http labels = %#v, want env=prod", findProjectedHTTPRoute(t, fullSnapshot, "http-labeled").GetLabels())
	}
	if got := findProjectedHTTPRoute(t, coreOnlySnapshot, "http-labeled").GetLabels(); got != nil {
		t.Fatalf("core-only stream http labels = %#v, want nil", got)
	}

	if len(fullSnapshot.GetBackends()) != 4 {
		t.Fatalf("full stream backend count = %d, want 4", len(fullSnapshot.GetBackends()))
	}
	if len(coreOnlySnapshot.GetBackends()) != 1 {
		t.Fatalf("core-only stream backend count = %d, want 1", len(coreOnlySnapshot.GetBackends()))
	}
	if findProjectedBackend(t, fullSnapshot, "ai-backend").GetAiService() == nil {
		t.Fatal("expected full stream to preserve ai service backend")
	}
	if len(coreOnlySnapshot.GetListeners()) != 1 {
		t.Fatalf("core-only stream listener count = %d, want 1", len(coreOnlySnapshot.GetListeners()))
	}
	if got := coreOnlySnapshot.GetListeners()[0].GetAttachedRoutes(); !reflect.DeepEqual(got, []string{"default/http-direct-response", "default/http-labeled"}) {
		t.Fatalf("core-only attached routes = %#v, want %#v", got, []string{"default/http-direct-response", "default/http-labeled"})
	}
	if len(coreOnlySnapshot.GetGrpcRoutes()) != 0 {
		t.Fatalf("core-only grpc routes = %#v, want none", coreOnlySnapshot.GetGrpcRoutes())
	}
	if len(coreOnlySnapshot.GetStreamRoutes()) != 0 {
		t.Fatalf("core-only stream routes = %#v, want none", coreOnlySnapshot.GetStreamRoutes())
	}
	httpDirectResponse := findProjectedHTTPRoute(t, coreOnlySnapshot, "http-direct-response")
	if len(httpDirectResponse.GetRules()) != 1 {
		t.Fatalf("core-only direct-response route rules = %#v, want one rule", httpDirectResponse.GetRules())
	}
	if got := httpDirectResponse.GetRules()[0].GetBackendRefs(); len(got) != 0 {
		t.Fatalf("core-only direct-response backends = %#v, want none", got)
	}
	if hasProjectedHTTPRoute(coreOnlySnapshot, "http-ai-only") {
		t.Fatal("expected core-only stream to prune http-ai-only route")
	}

	fullStream.release()
	coreOnlyStream.release()

	select {
	case err := <-fullResult:
		if err != nil {
			t.Fatalf("expected full stream to exit cleanly after release, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("full stream did not return after release")
	}
	select {
	case err := <-coreOnlyResult:
		if err != nil {
			t.Fatalf("expected core-only stream to exit cleanly after release, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("core-only stream did not return after release")
	}
}

func waitForNodeConnection(t *testing.T, nodes *nodeinfo.Registry, nodeID string) {
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

func (f *fakeConfigStream) waitForSendCount(t *testing.T, want int, timeout time.Duration) {
	t.Helper()

	deadline := time.After(timeout)
	for {
		if len(f.snapshotSentResponses()) >= want {
			return
		}

		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %d sends", want)
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
