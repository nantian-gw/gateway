package xds

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"

	"github.com/nantian-gw/gateway/internal/config"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/noderegistry"
	"github.com/nantian-gw/gateway/internal/observability"
)

func allDeltaTypeURLs() []string {
	return []string{
		typeURLListener,
		typeURLHTTPRoute,
		typeURLGRPCRoute,
		typeURLStreamRoute,
		typeURLBackend,
		typeURLSecret,
	}
}

type fakeDeltaStream struct {
	ctx         context.Context
	cancel      context.CancelFunc
	recvRelease chan struct{}
	recvQueue   chan *controlv1.DeltaDiscoveryRequest
	initialRecv chan *controlv1.DeltaDiscoveryRequest
	sendMu      sync.Mutex
	sent        []*controlv1.DeltaDiscoveryResponse
	sendNotify  chan struct{}
	recvCount   int
}

func newFakeDeltaStream() *fakeDeltaStream {
	ctx, cancel := context.WithCancel(context.Background())
	return &fakeDeltaStream{
		ctx:         ctx,
		cancel:      cancel,
		recvRelease: make(chan struct{}),
		recvQueue:   make(chan *controlv1.DeltaDiscoveryRequest, 8),
		initialRecv: make(chan *controlv1.DeltaDiscoveryRequest, 1),
		sendNotify:  make(chan struct{}, 32),
	}
}

func (f *fakeDeltaStream) withInitialRequest(req *controlv1.DeltaDiscoveryRequest) *fakeDeltaStream {
	f.initialRecv <- req
	return f
}

func (f *fakeDeltaStream) Send(resp *controlv1.DeltaDiscoveryResponse) error {
	select {
	case <-f.recvRelease:
		return status.Error(codes.Canceled, "stream closed")
	case <-f.ctx.Done():
		return status.Error(codes.Canceled, "stream closed")
	default:
	}

	f.sendMu.Lock()
	f.sent = append(f.sent, resp)
	f.sendMu.Unlock()

	select {
	case f.sendNotify <- struct{}{}:
	default:
	}
	return nil
}

func (f *fakeDeltaStream) Recv() (*controlv1.DeltaDiscoveryRequest, error) {
	if f.recvCount == 0 {
		f.recvCount++
		select {
		case req := <-f.initialRecv:
			return req, nil
		case <-f.recvRelease:
			return nil, io.EOF
		case <-f.ctx.Done():
			return nil, io.EOF
		default:
		}
		return &controlv1.DeltaDiscoveryRequest{
			NodeId:                 "dp-delta-1",
			Cluster:                "default",
			ResourceNamesSubscribe: allDeltaTypeURLs(),
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

func (f *fakeDeltaStream) pushRecv(req *controlv1.DeltaDiscoveryRequest) {
	select {
	case f.recvQueue <- req:
	case <-f.ctx.Done():
	}
}

func (f *fakeDeltaStream) release() {
	close(f.recvRelease)
	// Don't cancel ctx — it races with the main loop's ctx.Done() select.
	// Closing recvRelease is sufficient to unblock Recv() and return io.EOF.
}

func (f *fakeDeltaStream) sentResponses() []*controlv1.DeltaDiscoveryResponse {
	f.sendMu.Lock()
	defer f.sendMu.Unlock()
	out := make([]*controlv1.DeltaDiscoveryResponse, len(f.sent))
	copy(out, f.sent)
	return out
}

func (f *fakeDeltaStream) waitForSendCount(t *testing.T, want int) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		if len(f.sentResponses()) >= want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %d sends (got %d)", want, len(f.sentResponses()))
		case <-f.sendNotify:
		}
	}
}

func findLastResponseByTypeURL(resps []*controlv1.DeltaDiscoveryResponse, typeURL string) *controlv1.DeltaDiscoveryResponse {
	for i := len(resps) - 1; i >= 0; i-- {
		if resps[i].GetTypeUrl() == typeURL {
			return resps[i]
		}
	}
	return nil
}

func responseCountByTypeURL(resps []*controlv1.DeltaDiscoveryResponse, typeURL string) int {
	count := 0
	for _, r := range resps {
		if r.GetTypeUrl() == typeURL {
			count++
		}
	}
	return count
}

func (f *fakeDeltaStream) SetHeader(metadata.MD) error  { return nil }
func (f *fakeDeltaStream) SendHeader(metadata.MD) error { return nil }
func (f *fakeDeltaStream) SetTrailer(metadata.MD)       {}
func (f *fakeDeltaStream) Context() context.Context     { return f.ctx }
func (f *fakeDeltaStream) SendMsg(any) error            { return nil }
func (f *fakeDeltaStream) RecvMsg(any) error            { return nil }

// helpers

func makeTestSnapshot(extra ...func(*ir.Snapshot)) *ir.Snapshot {
	snap := &ir.Snapshot{
		ID:          "snap-test",
		GeneratedAt: time.Now().UTC(),
		Listeners: []ir.Listener{
			{Name: "http-listener", Port: 80, Protocol: "HTTP"},
			{Name: "https-listener", Port: 443, Protocol: "HTTPS"},
		},
		HTTPRoutes: []ir.HTTPRoute{
			{Namespace: "default", Name: "api-route"},
			{Namespace: "default", Name: "web-route"},
		},
		GRPCRoutes: []ir.GRPCRoute{
			{Namespace: "default", Name: "grpc-users"},
		},
		StreamRoutes: []ir.StreamRoute{
			{Namespace: "default", Name: "tcp-proxy", Kind: "TCP"},
		},
		Backends: []ir.BackendCluster{
			{Name: "users:80"},
			{Name: "orders:8080"},
			{Name: "cache:6379"},
		},
		Secrets: []ir.SecretMaterial{
			{Name: "tls-cert"},
		},
	}
	for _, fn := range extra {
		fn(snap)
	}
	return snap
}

func setupDeltaTestServer(t *testing.T) (*Server, *ir.SnapshotStore) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	metrics := observability.NewMetrics()
	nodes := noderegistry.NewRegistry(ir.NewNodeStatusStore(), nil, logger, noderegistry.Options{})
	t.Cleanup(func() { nodes.Close() })
	server, err := New(
		":18080",
		config.GRPCTLSConfig{},
		config.GRPCRuntimeConfig{XDSProtocol: "delta"},
		store, nodes, logger, metrics,
	)
	require.NoError(t, err)
	return server, store
}

// unblockRecv sends a no-op delta request so the main loop's Recv()
// returns and can process snapshot channel events.
func unblockRecv(stream *fakeDeltaStream, nodeID string) {
	stream.pushRecv(&controlv1.DeltaDiscoveryRequest{
		NodeId:  nodeID,
		Cluster: "default",
	})
}

// Tests

func TestDeltaServer_InitialFullSnapshot(t *testing.T) {
	server, store := setupDeltaTestServer(t)
	stream := newFakeDeltaStream()
	snap := makeTestSnapshot()
	store.Publish(snap)

	result := make(chan error, 1)
	go func() { result <- server.DeltaStreamConfiguration(stream) }()

	stream.waitForSendCount(t, len(allDeltaTypeURLs()))
	responses := stream.sentResponses()

	for _, typeURL := range allDeltaTypeURLs() {
		resp := findLastResponseByTypeURL(responses, typeURL)
		require.NotNil(t, resp, "expected response for type_url=%s", typeURL)
		require.NotEmpty(t, resp.GetSystemVersionInfo())
		require.NotEmpty(t, resp.GetNonce())
	}
	stream.release()
	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("stream did not return")
	}
}

func TestDeltaServer_SubscribedTypesOnly(t *testing.T) {
	server, store := setupDeltaTestServer(t)
	stream := newFakeDeltaStream().withInitialRequest(&controlv1.DeltaDiscoveryRequest{
		NodeId:                 "dp-delta-2",
		Cluster:                "default",
		ResourceNamesSubscribe: []string{typeURLListener, typeURLBackend},
	})
	snap := makeTestSnapshot()
	store.Publish(snap)

	result := make(chan error, 1)
	go func() { result <- server.DeltaStreamConfiguration(stream) }()

	stream.waitForSendCount(t, 2)
	responses := stream.sentResponses()

	require.NotNil(t, findLastResponseByTypeURL(responses, typeURLListener))
	require.NotNil(t, findLastResponseByTypeURL(responses, typeURLBackend))
	require.Equal(t, 0, responseCountByTypeURL(responses, typeURLHTTPRoute))
	require.Equal(t, 0, responseCountByTypeURL(responses, typeURLSecret))

	stream.release()
	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("stream did not return")
	}
}

func TestDeltaServer_NonIncremental(t *testing.T) {
	server, store := setupDeltaTestServer(t)
	stream := newFakeDeltaStream().withInitialRequest(&controlv1.DeltaDiscoveryRequest{
		NodeId:                 "dp-delta-3",
		Cluster:                "default",
		ResourceNamesSubscribe: []string{typeURLListener},
	})

	snap1 := makeTestSnapshot()
	store.Publish(snap1)

	result := make(chan error, 1)
	go func() { result <- server.DeltaStreamConfiguration(stream) }()

	stream.waitForSendCount(t, 1)

	before := len(stream.sentResponses())
	snap2 := makeTestSnapshot(func(s *ir.Snapshot) {
		s.ID = "snap-2"
		s.Listeners = []ir.Listener{
			{Name: "http-listener", Port: 80, Protocol: "HTTP", Hostnames: []string{"v2.example.com"}},
			{Name: "https-listener", Port: 443, Protocol: "HTTPS", Hostnames: []string{"v2.example.com"}},
			{Name: "admin-listener", Port: 9090, Protocol: "HTTP"},
		}
	})
	store.Publish(snap2)
	unblockRecv(stream, "dp-delta-3")

	stream.waitForSendCount(t, before+1)
	secondResp := findLastResponseByTypeURL(stream.sentResponses(), typeURLListener)
	require.NotNil(t, secondResp)
	require.True(t, secondResp.GetNonIncremental(),
		">50%% changes should set non_incremental=true")

	before = len(stream.sentResponses())
	snap3 := makeTestSnapshot(func(s *ir.Snapshot) {
		s.ID = "snap-3"
		s.Listeners = []ir.Listener{
			{Name: "http-listener", Port: 80, Protocol: "HTTP", Hostnames: []string{"v2.example.com"}},
			{Name: "https-listener", Port: 443, Protocol: "HTTPS", Hostnames: []string{"v2.example.com"}},
			{Name: "admin-listener", Port: 9090, Protocol: "HTTP", Hostnames: []string{"admin.example.com"}},
		}
	})
	store.Publish(snap3)
	unblockRecv(stream, "dp-delta-3")

	stream.waitForSendCount(t, before+1)
	thirdResp := findLastResponseByTypeURL(stream.sentResponses(), typeURLListener)
	require.NotNil(t, thirdResp)
	require.False(t, thirdResp.GetNonIncremental(),
		"<=50%% changes should have non_incremental=false")

	stream.release()
	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("stream did not return")
	}
}

func TestDeltaServer_AckNonce(t *testing.T) {
	server, store := setupDeltaTestServer(t)
	stream := newFakeDeltaStream().withInitialRequest(&controlv1.DeltaDiscoveryRequest{
		NodeId:                 "dp-delta-4",
		Cluster:                "default",
		ResourceNamesSubscribe: []string{typeURLListener},
	})

	snap := makeTestSnapshot()
	store.Publish(snap)

	result := make(chan error, 1)
	go func() { result <- server.DeltaStreamConfiguration(stream) }()

	stream.waitForSendCount(t, 1)
	initialResp := findLastResponseByTypeURL(stream.sentResponses(), typeURLListener)
	require.NotNil(t, initialResp)

	stream.pushRecv(&controlv1.DeltaDiscoveryRequest{
		NodeId:        "dp-delta-4",
		Cluster:       "default",
		ResponseNonce: initialResp.GetNonce(),
		TypeUrl:       typeURLListener,
		ResultStatus:  controlv1.DiscoveryResultStatus_DISCOVERY_RESULT_STATUS_ACK,
	})

	before := len(stream.sentResponses())
	snap2 := makeTestSnapshot(func(s *ir.Snapshot) {
		s.ID = "snap-ack-test"
		s.Listeners = []ir.Listener{
			{Name: "http-listener", Port: 80, Protocol: "HTTP", Hostnames: []string{"updated.example.com"}},
		}
	})
	store.Publish(snap2)
	unblockRecv(stream, "dp-delta-4")

	stream.waitForSendCount(t, before+1)
	secondResp := findLastResponseByTypeURL(stream.sentResponses(), typeURLListener)
	require.NotNil(t, secondResp, "expected new response after ACK")

	stream.release()
	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("stream did not return")
	}
}

func TestDeltaServer_DynamicSubscription(t *testing.T) {
	server, store := setupDeltaTestServer(t)

	// Start with BOTH listener and backend subscriptions.
	stream := newFakeDeltaStream().withInitialRequest(&controlv1.DeltaDiscoveryRequest{
		NodeId:                 "dp-delta-6",
		Cluster:                "default",
		ResourceNamesSubscribe: []string{typeURLListener, typeURLBackend},
	})

	snap1 := makeTestSnapshot(func(s *ir.Snapshot) {
		s.Backends = []ir.BackendCluster{
			{Name: "svc-a:80"}, {Name: "svc-b:80"}, {Name: "svc-c:80"},
		}
	})
	store.Publish(snap1)

	result := make(chan error, 1)
	go func() { result <- server.DeltaStreamConfiguration(stream) }()

	// 1. Initial push sends both listener and backend.
	stream.waitForSendCount(t, 2)
	all := stream.sentResponses()
	require.NotNil(t, findLastResponseByTypeURL(all, typeURLListener))
	require.NotNil(t, findLastResponseByTypeURL(all, typeURLBackend))

	// 2. Unsubscribe from listeners.
	unblockRecv(stream, "dp-delta-6")
	stream.pushRecv(&controlv1.DeltaDiscoveryRequest{
		NodeId:                   "dp-delta-6",
		ResourceNamesUnsubscribe: []string{typeURLListener},
	})
	// Give the goroutine time to process the unsubscribe before publishing
	// the next snapshot. Without this, the snapshot channel may fire before
	// the Recv loop handles the unsubscribe, causing the listener to remain
	// subscribed (and thus included in the delta response).
	time.Sleep(50 * time.Millisecond)
	// 3. Publish a snapshot that changes BOTH listeners and backends.
	snap3 := makeTestSnapshot(func(s *ir.Snapshot) {
		s.ID = "snap-dynamic-mixed"
		s.Listeners = []ir.Listener{
			{Name: "new-listener", Port: 8080, Protocol: "HTTP"},
		}
		s.Backends = []ir.BackendCluster{
			{Name: "svc-a:80"},
			{Name: "new-svc:9090"},
		}
	})
	store.Publish(snap3)
	unblockRecv(stream, "dp-delta-6")

	// 4. Expect new responses — should be backend only, not listener.
	stream.waitForSendCount(t, 3)
	responses := stream.sentResponses()
	resp3 := responses[len(responses)-1]
	require.NotEqual(t, typeURLListener, resp3.GetTypeUrl(),
		"latest response should not be a listener after unsubscription")
	require.Equal(t, typeURLBackend, resp3.GetTypeUrl(),
		"latest response should be backend after backend change")

	stream.release()
	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("stream did not return")
	}
}

func TestDeltaServer_RemovedResources(t *testing.T) {
	server, store := setupDeltaTestServer(t)
	stream := newFakeDeltaStream().withInitialRequest(&controlv1.DeltaDiscoveryRequest{
		NodeId:                 "dp-delta-7",
		Cluster:                "default",
		ResourceNamesSubscribe: []string{typeURLBackend},
	})

	snap1 := makeTestSnapshot(func(s *ir.Snapshot) {
		s.Backends = []ir.BackendCluster{
			{Name: "svc-a:80"}, {Name: "svc-b:80"}, {Name: "svc-c:80"},
		}
	})
	store.Publish(snap1)

	result := make(chan error, 1)
	go func() { result <- server.DeltaStreamConfiguration(stream) }()

	stream.waitForSendCount(t, 1)
	unblockRecv(stream, "dp-delta-7")

	before := len(stream.sentResponses())
	snap2 := makeTestSnapshot(func(s *ir.Snapshot) {
		s.ID = "snap-removed"
		s.Backends = []ir.BackendCluster{
			{Name: "svc-a:80"}, {Name: "svc-c:80"},
		}
	})
	store.Publish(snap2)
	unblockRecv(stream, "dp-delta-7")

	stream.waitForSendCount(t, before+1)
	secondResp := findLastResponseByTypeURL(stream.sentResponses(), typeURLBackend)
	require.NotNil(t, secondResp)
	require.Contains(t, secondResp.GetRemovedResources(), "svc-b:80")

	stream.release()
	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("stream did not return")
	}
}

func TestDeltaServer_NackHandling(t *testing.T) {
	server, store := setupDeltaTestServer(t)
	stream := newFakeDeltaStream().withInitialRequest(&controlv1.DeltaDiscoveryRequest{
		NodeId:                 "dp-delta-8",
		Cluster:                "default",
		ResourceNamesSubscribe: []string{typeURLListener},
	})

	snap := makeTestSnapshot()
	store.Publish(snap)

	result := make(chan error, 1)
	go func() { result <- server.DeltaStreamConfiguration(stream) }()

	stream.waitForSendCount(t, 1)
	initialResp := findLastResponseByTypeURL(stream.sentResponses(), typeURLListener)
	require.NotNil(t, initialResp)

	stream.pushRecv(&controlv1.DeltaDiscoveryRequest{
		NodeId:        "dp-delta-8",
		Cluster:       "default",
		ResponseNonce: initialResp.GetNonce(),
		TypeUrl:       typeURLListener,
		ResultStatus:  controlv1.DiscoveryResultStatus_DISCOVERY_RESULT_STATUS_NACK,
		ErrorDetail:   "failed to apply listener config",
	})

	before := len(stream.sentResponses())
	snap2 := makeTestSnapshot(func(s *ir.Snapshot) {
		s.ID = "snap-nack-test"
		s.Listeners = []ir.Listener{
			{Name: "recovery-listener", Port: 80, Protocol: "HTTP"},
		}
	})
	store.Publish(snap2)
	unblockRecv(stream, "dp-delta-8")

	stream.waitForSendCount(t, before+1)
	secondResp := findLastResponseByTypeURL(stream.sentResponses(), typeURLListener)
	require.NotNil(t, secondResp, "stream should continue sending after NACK")

	stream.release()
	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("stream did not return")
	}
}

func TestDeltaServer_RemovedResourceNonIncremental(t *testing.T) {
	server, store := setupDeltaTestServer(t)
	stream := newFakeDeltaStream().withInitialRequest(&controlv1.DeltaDiscoveryRequest{
		NodeId:                 "dp-delta-9",
		Cluster:                "default",
		ResourceNamesSubscribe: []string{typeURLBackend},
	})

	snap1 := makeTestSnapshot(func(s *ir.Snapshot) {
		s.Backends = []ir.BackendCluster{
			{Name: "a:80"}, {Name: "b:80"}, {Name: "c:80"}, {Name: "d:80"},
		}
	})
	store.Publish(snap1)

	result := make(chan error, 1)
	go func() { result <- server.DeltaStreamConfiguration(stream) }()

	stream.waitForSendCount(t, 1)
	unblockRecv(stream, "dp-delta-9")

	before := len(stream.sentResponses())
	snap2 := makeTestSnapshot(func(s *ir.Snapshot) {
		s.ID = "snap-removed-nonincremental"
		s.Backends = []ir.BackendCluster{{Name: "a:80"}}
	})
	store.Publish(snap2)
	unblockRecv(stream, "dp-delta-9")

	stream.waitForSendCount(t, before+1)
	secondResp := findLastResponseByTypeURL(stream.sentResponses(), typeURLBackend)
	require.NotNil(t, secondResp)
	require.True(t, secondResp.GetNonIncremental(),
		"removing 75%% should set non_incremental=true")
	require.Len(t, secondResp.GetRemovedResources(), 3)

	stream.release()
	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("stream did not return")
	}
}
