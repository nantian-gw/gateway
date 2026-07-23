package noderegistry

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/observability"
)

func TestRegistrySeedsSharedStateBeforeApplyingUpdates(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_500, 0).UTC()
	repository := &memoryRepository{
		items: map[string]ir.NodeStatus{
			"dp-1": {
				NodeID:          "dp-1",
				Cluster:         "kind",
				Connected:       true,
				ConnectedAt:     now.Add(-10 * time.Second),
				LastSeenAt:      now.Add(-5 * time.Second),
				LastSentVersion: "v1",
				LastAckVersion:  "v1",
				LastNonce:       "nonce-1",
				Ready:           true,
				Message:         "ready",
				Subscriptions:   []string{"routes"},
			},
		},
	}
	registry := NewRegistry(
		ir.NewNodeStatusStore(),
		repository,
		testLogger(),
		Options{PersistTimeout: time.Second},
	)
	t.Cleanup(registry.Close)

	registry.Connect(context.Background(), "dp-1", "kind", []string{"routes", "listeners"}, now)

	status, ok := waitForNodeStatusMatching(t, repository, "dp-1", time.Second, func(status ir.NodeStatus) bool {
		return status.LastSeenAt.Equal(now)
	})
	if !ok {
		t.Fatal("expected node status to be persisted")
	}
	if status.LastAckVersion != "v1" || status.LastNonce != "nonce-1" {
		t.Fatalf("expected shared fields to be preserved, got %+v", status)
	}
	if !status.Ready {
		t.Fatalf("expected ready state to be preserved, got %+v", status)
	}
	if !status.LastSeenAt.Equal(now) {
		t.Fatalf("unexpected last seen: %v", status.LastSeenAt)
	}
	if len(status.Subscriptions) != 2 {
		t.Fatalf("unexpected subscriptions: %+v", status.Subscriptions)
	}
}

func TestLeaseRepositoryRoundTripsNodeStatus(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add lease scheme: %v", err)
	}

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	repository := NewLeaseRepository(
		client,
		client,
		"gateway-system",
		"Node Status",
		testLogger(),
	)

	now := time.Unix(1_700_000_800, 0).UTC()
	status := ir.NodeStatus{
		NodeID:          "dp-2",
		Cluster:         "kind",
		Connected:       true,
		ConnectedAt:     now.Add(-10 * time.Second),
		LastSeenAt:      now,
		LastSentVersion: "v2",
		LastAckVersion:  "v2",
		LastNonce:       "nonce-2",
		Ready:           true,
		Message:         "snapshot applied",
		Subscriptions:   []string{"*"},
		SupportedFeatures: []string{
			"core.v1",
			"route.labels.v1",
		},
	}

	if err := repository.Upsert(context.Background(), status); err != nil {
		t.Fatalf("upsert status: %v", err)
	}

	got, ok, err := repository.Get(context.Background(), "dp-2")
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	if !ok {
		t.Fatal("expected stored status to exist")
	}
	if got.NodeID != "dp-2" || got.LastAckVersion != "v2" || !got.Ready {
		t.Fatalf("unexpected status: %+v", got)
	}
	if !reflect.DeepEqual(got.SupportedFeatures, status.SupportedFeatures) {
		t.Fatalf("supported features = %#v, want %#v", got.SupportedFeatures, status.SupportedFeatures)
	}

	items, err := repository.List(context.Background())
	if err != nil {
		t.Fatalf("list statuses: %v", err)
	}
	if len(items) != 1 || items[0].NodeID != "dp-2" {
		t.Fatalf("unexpected list output: %+v", items)
	}
	if repository.Namespace() != "gateway-system" {
		t.Fatalf("unexpected namespace: %s", repository.Namespace())
	}
	if repository.Prefix() != "node-status" {
		t.Fatalf("unexpected prefix: %s", repository.Prefix())
	}
}

func TestRegistryRoutingStateChangedIncludesSupportedFeatures(t *testing.T) {
	t.Parallel()

	repository := newTrackingRepository()
	registry := NewRegistry(
		ir.NewNodeStatusStore(),
		repository,
		testLogger(),
		Options{
			PersistTimeout:  time.Second,
			PersistDebounce: time.Minute,
		},
	)
	t.Cleanup(registry.Close)

	triggered := 0
	registry.SetOnChange(func() {
		triggered++
	})

	now := time.Now().UTC()
	registry.ConnectWithFeatures(context.Background(), "dp-features", "kind", []string{"*"}, []string{"core.v1"}, now)
	if _, ok := repository.waitForUpserts(1); !ok {
		t.Fatal("expected connect to persist immediately")
	}
	if triggered != 1 {
		t.Fatalf("expected connect to trigger once, got %d", triggered)
	}
	repository.reset()

	registry.ObserveAckWithFeatures(
		context.Background(),
		"dp-features",
		"kind",
		"v1",
		"nonce-1",
		[]string{"*"},
		[]string{"core.v1"},
		now.Add(time.Second),
	)
	if _, ok := repository.waitForUpserts(1); !ok {
		t.Fatal("expected initial ack to persist immediately")
	}
	if triggered != 2 {
		t.Fatalf("expected ack version change to trigger once, got %d", triggered)
	}
	repository.reset()

	registry.ObserveAckWithFeatures(
		context.Background(),
		"dp-features",
		"kind",
		"v1",
		"nonce-2",
		[]string{"*"},
		[]string{"core.v1", "route.labels.v1"},
		now.Add(2*time.Second),
	)

	items, ok := repository.waitForUpserts(1)
	if !ok {
		t.Fatal("expected supported feature change to persist immediately")
	}
	if triggered != 3 {
		t.Fatalf("expected supported feature change to trigger routing state change, got %d", triggered)
	}
	if !reflect.DeepEqual(items[0].SupportedFeatures, []string{"core.v1", "route.labels.v1"}) {
		t.Fatalf("persisted supported features = %#v, want %#v", items[0].SupportedFeatures, []string{"core.v1", "route.labels.v1"})
	}

	status, found := registry.Get(context.Background(), "dp-features")
	if !found {
		t.Fatal("expected node to remain readable from registry")
	}
	if !reflect.DeepEqual(status.SupportedFeatures, []string{"core.v1", "route.labels.v1"}) {
		t.Fatalf("registry supported features = %#v, want %#v", status.SupportedFeatures, []string{"core.v1", "route.labels.v1"})
	}
}

func TestRegistryListPrefersFreshestStateForDuplicateNodes(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	repository := &memoryRepository{
		items: map[string]ir.NodeStatus{
			"dp-3": {
				NodeID:         "dp-3",
				Cluster:        "kind",
				Connected:      true,
				LastSeenAt:     now,
				LastAckVersion: "v1",
				LastNonce:      "shared",
				Ready:          true,
			},
		},
	}
	local := ir.NewNodeStatusStore()
	local.Upsert(ir.NodeStatus{
		NodeID:         "dp-3",
		Cluster:        "kind",
		Connected:      true,
		LastSeenAt:     now.Add(5 * time.Second),
		LastAckVersion: "v1",
		LastNonce:      "local",
		Ready:          true,
	})

	registry := NewRegistry(local, repository, testLogger(), Options{PersistTimeout: time.Second})
	t.Cleanup(registry.Close)
	items := registry.List(context.Background())
	if len(items) != 1 {
		t.Fatalf("expected 1 node, got %+v", items)
	}
	if items[0].LastNonce != "local" {
		t.Fatalf("expected freshest state to win, got %+v", items[0])
	}
}

func TestRegistryListFiltersStaleSharedNodes(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	repository := &memoryRepository{
		items: map[string]ir.NodeStatus{
			"dp-stale": {
				NodeID:     "dp-stale",
				Cluster:    "kind",
				Connected:  true,
				Ready:      true,
				LastSeenAt: now.Add(-defaultLeaseDuration - time.Second),
			},
			"dp-fresh": {
				NodeID:     "dp-fresh",
				Cluster:    "kind",
				Connected:  true,
				Ready:      true,
				LastSeenAt: now.Add(-time.Second),
			},
		},
	}

	registry := NewRegistry(ir.NewNodeStatusStore(), repository, testLogger(), Options{PersistTimeout: time.Second})
	t.Cleanup(registry.Close)
	items := registry.List(context.Background())
	if len(items) != 1 {
		t.Fatalf("expected only fresh nodes to remain, got %+v", items)
	}
	if items[0].NodeID != "dp-fresh" {
		t.Fatalf("unexpected node list after stale filtering: %+v", items)
	}
}

func TestRegistryListUsesLifecycleContextWhenCallerContextIsNil(t *testing.T) {
	t.Parallel()

	lifecycleCtx, cancel := context.WithCancel(context.Background())
	cancel()

	repository := newContextCapturingRepository()
	registry := NewRegistry(
		ir.NewNodeStatusStore(),
		repository,
		testLogger(),
		Options{
			BaseContext:    lifecycleCtx,
			PersistTimeout: time.Second,
		},
	)
	t.Cleanup(registry.Close)

	_ = registry.List(context.TODO())

	ctx, ok := repository.waitForListContext(time.Second)
	if !ok {
		t.Fatal("expected list to call shared repository")
	}
	if ctx.Err() != context.Canceled {
		t.Fatalf("expected list context to inherit lifecycle cancellation, got %v", ctx.Err())
	}
}

func TestRegistryTriggersOnChangeForRoutingRelevantState(t *testing.T) {
	t.Parallel()

	triggered := 0
	registry := NewRegistry(
		ir.NewNodeStatusStore(),
		nil,
		testLogger(),
		Options{PersistTimeout: time.Second},
	)
	t.Cleanup(registry.Close)
	registry.SetOnChange(func() {
		triggered++
	})

	now := time.Now().UTC()
	registry.Connect(context.Background(), "dp-1", "kind", []string{"*"}, now)
	if triggered != 1 {
		t.Fatalf("expected connect to trigger once, got %d", triggered)
	}

	registry.ObserveAck(context.Background(), "dp-1", "kind", "v1", "nonce-1", []string{"*"}, now)
	if triggered != 2 {
		t.Fatalf("expected ack version change to trigger once, got %d", triggered)
	}

	registry.ObserveAck(context.Background(), "dp-1", "kind", "v1", "nonce-2", []string{"*"}, now)
	if triggered != 2 {
		t.Fatalf("expected nonce-only update to avoid triggering, got %d", triggered)
	}

	registry.ObserveReport(context.Background(), "dp-1", "v1", true, "ready", now)
	if triggered != 3 {
		t.Fatalf("expected ready transition to trigger once, got %d", triggered)
	}

	registry.ObserveReport(context.Background(), "dp-1", "v1", true, "still ready", now)
	if triggered != 3 {
		t.Fatalf("expected message-only update to avoid triggering, got %d", triggered)
	}

	registry.Disconnect(context.Background(), "dp-1", now)
	if triggered != 4 {
		t.Fatalf("expected disconnect to trigger once, got %d", triggered)
	}
}

func TestRegistryDebouncesNonRoutingStatusPersistence(t *testing.T) {
	t.Parallel()

	repository := newTrackingRepository()
	registry := NewRegistry(
		ir.NewNodeStatusStore(),
		repository,
		testLogger(),
		Options{
			PersistTimeout:  time.Second,
			PersistDebounce: 20 * time.Millisecond,
		},
	)
	t.Cleanup(registry.Close)

	now := time.Unix(1_700_000_900, 0).UTC()
	registry.Connect(context.Background(), "dp-9", "kind", []string{"*"}, now)
	if _, ok := repository.waitForUpserts(1); !ok {
		t.Fatal("expected connect to persist immediately")
	}
	repository.reset()

	registry.ObserveReport(context.Background(), "dp-9", "", false, "warming-1", now.Add(time.Second))
	registry.ObserveReport(context.Background(), "dp-9", "", false, "warming-2", now.Add(2*time.Second))

	items, ok := repository.waitForUpserts(1)
	if !ok {
		t.Fatal("expected debounced persist to flush")
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 persisted item, got %+v", items)
	}
	if items[0].Message != "warming-2" {
		t.Fatalf("expected latest debounced status to persist, got %+v", items[0])
	}
	if !items[0].LastSeenAt.Equal(now.Add(2 * time.Second)) {
		t.Fatalf("expected latest last seen to persist, got %+v", items[0])
	}
}

func TestRegistryImmediatelyPersistsRoutingStateChanges(t *testing.T) {
	t.Parallel()

	repository := newTrackingRepository()
	registry := NewRegistry(
		ir.NewNodeStatusStore(),
		repository,
		testLogger(),
		Options{
			PersistTimeout:  time.Second,
			PersistDebounce: time.Minute,
		},
	)
	t.Cleanup(registry.Close)

	now := time.Unix(1_700_001_000, 0).UTC()
	registry.Connect(context.Background(), "dp-10", "kind", []string{"*"}, now)
	if _, ok := repository.waitForUpserts(1); !ok {
		t.Fatal("expected connect to persist immediately")
	}
	repository.reset()

	registry.ObserveAck(context.Background(), "dp-10", "kind", "v1", "nonce-1", []string{"*"}, now.Add(time.Second))

	items, ok := repository.waitForUpserts(1)
	if !ok {
		t.Fatal("expected ack version change to persist immediately")
	}
	if len(items) != 1 || items[0].LastAckVersion != "v1" {
		t.Fatalf("unexpected persisted routing state: %+v", items)
	}
}

func TestRegistryImmediatelyPersistsNackStateChanges(t *testing.T) {
	t.Parallel()

	repository := newTrackingRepository()
	registry := NewRegistry(
		ir.NewNodeStatusStore(),
		repository,
		testLogger(),
		Options{
			PersistTimeout:  time.Second,
			PersistDebounce: time.Minute,
		},
	)
	t.Cleanup(registry.Close)

	now := time.Unix(1_700_001_100, 0).UTC()
	registry.Connect(context.Background(), "dp-11", "kind", []string{"*"}, now)
	registry.ObserveAck(context.Background(), "dp-11", "kind", "v1", "nonce-1", []string{"*"}, now.Add(time.Second))
	if _, ok := repository.waitForUpserts(2); !ok {
		t.Fatal("expected connect and ack to persist")
	}
	repository.reset()

	registry.ObserveNack(
		context.Background(),
		"dp-11",
		"kind",
		"v2",
		"nonce-2",
		"listener reload failed",
		[]string{"*"},
		now.Add(2*time.Second),
	)

	items, ok := repository.waitForUpserts(1)
	if !ok {
		t.Fatal("expected nack to persist immediately")
	}
	if len(items) != 1 || items[0].LastConfigStatus != "NACK" || items[0].LastNackVersion != "v2" {
		t.Fatalf("unexpected persisted nack state: %+v", items)
	}
	if items[0].LastAckVersion != "v1" {
		t.Fatalf("expected nack to preserve last ack version, got %+v", items[0])
	}
}

func TestRegistryDoesNotBlockCallersWhenPersistenceBacklogIsFull(t *testing.T) {
	t.Parallel()

	metrics := observability.NewMetrics()
	repository := newBlockingTrackingRepository()
	registry := NewRegistry(
		ir.NewNodeStatusStore(),
		repository,
		testLogger(),
		Options{
			PersistTimeout:  time.Second,
			PersistDebounce: 20 * time.Millisecond,
			Metrics:         metrics,
		},
	)
	t.Cleanup(func() {
		repository.release()
		registry.Close()
	})

	now := time.Unix(1_700_001_200, 0).UTC()
	registry.Connect(context.Background(), "dp-000", "kind", []string{"*"}, now)
	if !repository.waitForStarted(time.Second) {
		t.Fatal("expected first persist to start")
	}

	done := make(chan struct{})
	go func() {
		for i := 1; i <= persistQueueSize+32; i++ {
			registry.Connect(
				context.Background(),
				fmt.Sprintf("dp-%03d", i),
				"kind",
				[]string{"*"},
				now.Add(time.Duration(i)*time.Millisecond),
			)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected node status updates to avoid blocking under persistence backlog pressure")
	}

	if got := testutil.ToFloat64(metrics.NodeStatusPersistDroppedTotal); got == 0 {
		t.Fatal("expected dropped persistence updates once the bounded backlog is exhausted")
	}
	if got := testutil.ToFloat64(metrics.NodeStatusPersistQueueDepth); got == 0 {
		t.Fatal("expected persistence backlog depth to be reported")
	}
}

func TestRegistryRecordsPersistenceMetricsForImmediateAndDebouncedFlushes(t *testing.T) {
	t.Parallel()

	metrics := observability.NewMetrics()
	repository := newTrackingRepository()
	registry := NewRegistry(
		ir.NewNodeStatusStore(),
		repository,
		testLogger(),
		Options{
			PersistTimeout:  time.Second,
			PersistDebounce: 20 * time.Millisecond,
			Metrics:         metrics,
		},
	)
	t.Cleanup(registry.Close)

	now := time.Unix(1_700_001_300, 0).UTC()
	registry.Connect(context.Background(), "dp-12", "kind", []string{"*"}, now)
	if _, ok := repository.waitForUpserts(1); !ok {
		t.Fatal("expected connect to persist immediately")
	}
	repository.reset()

	registry.ObserveReport(context.Background(), "dp-12", "", false, "warming-1", now.Add(time.Second))
	registry.ObserveReport(context.Background(), "dp-12", "", false, "warming-2", now.Add(2*time.Second))

	items, ok := repository.waitForUpserts(1)
	if !ok {
		t.Fatal("expected debounced persist to flush")
	}
	if len(items) != 1 || items[0].Message != "warming-2" {
		t.Fatalf("expected latest debounced status to persist, got %+v", items)
	}

	if got := testutil.ToFloat64(metrics.NodeStatusPersistImmediateTotal); got != 1 {
		t.Fatalf("immediate persistence count = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.NodeStatusPersistDebouncedTotal); got != 2 {
		t.Fatalf("debounced persistence count = %v, want 2", got)
	}
	if got := histogramSampleCount(t, metrics.NodeStatusPersistFlushDurationSeconds); got != 1 {
		t.Fatalf("flush duration sample count = %d, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.NodeStatusPersistQueueDepth); got != 0 {
		t.Fatalf("queue depth after flush = %v, want 0", got)
	}
	if got := testutil.ToFloat64(metrics.NodeStatusPersistPendingNodes); got != 0 {
		t.Fatalf("pending nodes after flush = %v, want 0", got)
	}
}

func TestRegistryAsyncPersistenceUsesLifecycleContext(t *testing.T) {
	t.Parallel()

	lifecycleCtx, cancel := context.WithCancel(context.Background())
	cancel()

	repository := newContextCapturingRepository()
	registry := NewRegistry(
		ir.NewNodeStatusStore(),
		repository,
		testLogger(),
		Options{
			BaseContext:    lifecycleCtx,
			PersistTimeout: time.Second,
		},
	)
	t.Cleanup(registry.Close)

	registry.Connect(context.Background(), "dp-ctx", "kind", []string{"*"}, time.Now().UTC())

	ctx, ok := repository.waitForUpsertContext(time.Second)
	if !ok {
		t.Fatal("expected async persistence to call shared repository")
	}
	if ctx.Err() != context.Canceled {
		t.Fatalf("expected async persistence context to inherit lifecycle cancellation, got %v", ctx.Err())
	}
}

func TestRegistryRecordsPublishToAckLagForMatchingVersion(t *testing.T) {
	t.Parallel()

	metrics := observability.NewMetrics()
	registry := NewRegistry(
		ir.NewNodeStatusStore(),
		nil,
		testLogger(),
		Options{
			PersistTimeout: time.Second,
			Metrics:        metrics,
		},
	)
	t.Cleanup(registry.Close)

	now := time.Unix(1_700_001_400, 0).UTC()
	registry.Connect(context.Background(), "dp-20", "kind", []string{"*"}, now)
	registry.ObservePublished(context.Background(), "dp-20", "v1", now.Add(time.Second))
	registry.ObserveAck(context.Background(), "dp-20", "kind", "v1", "nonce-1", []string{"*"}, now.Add(4*time.Second))
	registry.ObserveAck(context.Background(), "dp-20", "kind", "v1", "nonce-2", []string{"*"}, now.Add(5*time.Second))

	if got := histogramSampleCount(t, metrics.XDSPublishAckLagSeconds); got != 1 {
		t.Fatalf("ack lag sample count = %d, want 1", got)
	}
	if got := histogramSampleSum(t, metrics.XDSPublishAckLagSeconds); got != 3 {
		t.Fatalf("ack lag sample sum = %v, want 3", got)
	}
}

func TestRegistryRecordsPublishToNackLagForMatchingVersionOnly(t *testing.T) {
	t.Parallel()

	metrics := observability.NewMetrics()
	registry := NewRegistry(
		ir.NewNodeStatusStore(),
		nil,
		testLogger(),
		Options{
			PersistTimeout: time.Second,
			Metrics:        metrics,
		},
	)
	t.Cleanup(registry.Close)

	now := time.Unix(1_700_001_500, 0).UTC()
	registry.Connect(context.Background(), "dp-21", "kind", []string{"*"}, now)
	registry.ObservePublished(context.Background(), "dp-21", "v2", now.Add(time.Second))
	registry.ObserveNack(context.Background(), "dp-21", "kind", "v1", "nonce-old", "stale", []string{"*"}, now.Add(2*time.Second))
	registry.ObserveNack(context.Background(), "dp-21", "kind", "v2", "nonce-new", "rejected", []string{"*"}, now.Add(6*time.Second))

	if got := histogramSampleCount(t, metrics.XDSPublishNackLagSeconds); got != 1 {
		t.Fatalf("nack lag sample count = %d, want 1", got)
	}
	if got := histogramSampleSum(t, metrics.XDSPublishNackLagSeconds); got != 5 {
		t.Fatalf("nack lag sample sum = %v, want 5", got)
	}
}

type memoryRepository struct {
	mu    sync.Mutex
	items map[string]ir.NodeStatus
}

func (m *memoryRepository) Get(_ context.Context, nodeID string) (ir.NodeStatus, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.items[nodeID]
	return clone(item), ok, nil
}

func (m *memoryRepository) List(_ context.Context) ([]ir.NodeStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ir.NodeStatus, 0, len(m.items))
	for _, item := range m.items {
		out = append(out, clone(item))
	}
	return out, nil
}

func (m *memoryRepository) Upsert(_ context.Context, status ir.NodeStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.items == nil {
		m.items = make(map[string]ir.NodeStatus)
	}
	m.items[status.NodeID] = clone(status)
	return nil
}

type contextCapturingRepository struct {
	listCtxCh   chan context.Context
	upsertCtxCh chan context.Context
}

func newContextCapturingRepository() *contextCapturingRepository {
	return &contextCapturingRepository{
		listCtxCh:   make(chan context.Context, 1),
		upsertCtxCh: make(chan context.Context, 1),
	}
}

func (r *contextCapturingRepository) Get(context.Context, string) (ir.NodeStatus, bool, error) {
	return ir.NodeStatus{}, false, nil
}

func (r *contextCapturingRepository) List(ctx context.Context) ([]ir.NodeStatus, error) {
	select {
	case r.listCtxCh <- ctx:
	default:
	}
	return nil, nil
}

func (r *contextCapturingRepository) Upsert(ctx context.Context, status ir.NodeStatus) error {
	select {
	case r.upsertCtxCh <- ctx:
	default:
	}
	return nil
}

func (r *contextCapturingRepository) waitForListContext(timeout time.Duration) (context.Context, bool) {
	select {
	case ctx := <-r.listCtxCh:
		return ctx, true
	case <-time.After(timeout):
		return nil, false
	}
}

func (r *contextCapturingRepository) waitForUpsertContext(timeout time.Duration) (context.Context, bool) {
	select {
	case ctx := <-r.upsertCtxCh:
		return ctx, true
	case <-time.After(timeout):
		return nil, false
	}
}

type trackingRepository struct {
	mu      sync.Mutex
	items   map[string]ir.NodeStatus
	upserts []ir.NodeStatus
	ch      chan struct{}
}

func newTrackingRepository() *trackingRepository {
	return &trackingRepository{
		items: make(map[string]ir.NodeStatus),
		ch:    make(chan struct{}, 32),
	}
}

type blockingTrackingRepository struct {
	trackingRepository
	started     chan struct{}
	releaseCh   chan struct{}
	startOnce   sync.Once
	releaseOnce sync.Once
}

func newBlockingTrackingRepository() *blockingTrackingRepository {
	return &blockingTrackingRepository{
		trackingRepository: trackingRepository{
			items: make(map[string]ir.NodeStatus),
			ch:    make(chan struct{}, 32),
		},
		started:   make(chan struct{}),
		releaseCh: make(chan struct{}),
	}
}

func (r *blockingTrackingRepository) Upsert(ctx context.Context, status ir.NodeStatus) error {
	r.mu.Lock()
	r.items[status.NodeID] = clone(status)
	r.upserts = append(r.upserts, clone(status))
	r.mu.Unlock()

	r.startOnce.Do(func() {
		close(r.started)
	})

	select {
	case r.ch <- struct{}{}:
	default:
	}

	select {
	case <-r.releaseCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *blockingTrackingRepository) waitForStarted(timeout time.Duration) bool {
	select {
	case <-r.started:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (r *blockingTrackingRepository) release() {
	r.releaseOnce.Do(func() {
		close(r.releaseCh)
	})
}

func (r *trackingRepository) Get(_ context.Context, nodeID string) (ir.NodeStatus, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.items[nodeID]
	return clone(item), ok, nil
}

func (r *trackingRepository) List(_ context.Context) ([]ir.NodeStatus, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ir.NodeStatus, 0, len(r.items))
	for _, item := range r.items {
		out = append(out, clone(item))
	}
	return out, nil
}

func (r *trackingRepository) Upsert(_ context.Context, status ir.NodeStatus) error {
	r.mu.Lock()
	r.items[status.NodeID] = clone(status)
	r.upserts = append(r.upserts, clone(status))
	r.mu.Unlock()

	select {
	case r.ch <- struct{}{}:
	default:
	}
	return nil
}

func (r *trackingRepository) reset() {
	r.mu.Lock()
	r.upserts = nil
	r.mu.Unlock()

	for {
		select {
		case <-r.ch:
		default:
			return
		}
	}
}

func (r *trackingRepository) waitForUpserts(expected int) ([]ir.NodeStatus, bool) {
	deadline := time.After(time.Second)
	for {
		r.mu.Lock()
		if len(r.upserts) >= expected {
			out := append([]ir.NodeStatus(nil), r.upserts...)
			r.mu.Unlock()
			return out, true
		}
		r.mu.Unlock()

		select {
		case <-r.ch:
		case <-deadline:
			r.mu.Lock()
			out := append([]ir.NodeStatus(nil), r.upserts...)
			r.mu.Unlock()
			return out, false
		}
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func waitForNodeStatusMatching(
	t *testing.T,
	repository Repository,
	nodeID string,
	timeout time.Duration,
	match func(ir.NodeStatus) bool,
) (ir.NodeStatus, bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, ok, err := repository.Get(context.Background(), nodeID)
		if err != nil {
			t.Fatalf("get status: %v", err)
		}
		if ok && (match == nil || match(status)) {
			return status, true
		}
		time.Sleep(10 * time.Millisecond)
	}

	status, ok, err := repository.Get(context.Background(), nodeID)
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	return status, ok && (match == nil || match(status))
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
