package admin

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/noderegistry"
)

func TestBackendAndNodeEndpointsSupportFilteringAndDetails(t *testing.T) {
	server := newTestServer(t)

	var backends []ir.BackendCluster
	recorder := performRequest(t, server, http.MethodGet, "/v1/backends?namespace=default&protocol=http&service=api", &backends)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if len(backends) != 1 || backends[0].Name != "api:80" {
		t.Fatalf("unexpected backends: %+v", backends)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/backends?all=true", &backends)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if len(backends) != 2 {
		t.Fatalf("expected all backends to include unreferenced entries, got %+v", backends)
	}

	var backend ir.BackendCluster
	recorder = performRequest(t, server, http.MethodGet, "/v1/backends/default/api:80", &backend)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if backend.Protocol != "HTTP" {
		t.Fatalf("unexpected backend detail: %+v", backend)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/backends/ops/metrics:9090", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected unreferenced backend detail to return 404, got %d", recorder.Code)
	}

	var nodes []ir.NodeStatus
	recorder = performRequest(t, server, http.MethodGet, "/v1/nodes?connected=true&ready=true", &nodes)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if len(nodes) != 1 || nodes[0].NodeID != "dp-1" {
		t.Fatalf("unexpected nodes: %+v", nodes)
	}

	var node ir.NodeStatus
	recorder = performRequest(t, server, http.MethodGet, "/v1/nodes/dp-1", &node)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if node.LastAckVersion != server.store.Current().ID || !node.Ready {
		t.Fatalf("unexpected node detail: %+v", node)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/nodes?ready=maybe", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid readiness filter, got %d", recorder.Code)
	}
}

func TestBackendEndpointsSupportSortingContract(t *testing.T) {
	server := newTestServer(t)
	server.store.Publish(&ir.Snapshot{
		GeneratedAt: time.Now().UTC(),
		Backends: []ir.BackendCluster{
			{
				Name:      "metrics:9090",
				Namespace: "ops",
				Protocol:  "TCP",
				Metadata:  map[string]string{"service": "metrics"},
			},
			{
				Name:      "api:80",
				Namespace: "default",
				Protocol:  "HTTP",
				Metadata:  map[string]string{"service": "api"},
			},
		},
	})

	var backends []ir.BackendCluster
	recorder := performRequest(t, server, http.MethodGet, "/v1/backends?all=true", &backends)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if got := strings.Join(backendKeys(backends), ","); got != "default/api:80,ops/metrics:9090" {
		t.Fatalf("unexpected default backend order: %s", got)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/backends?all=true&sort=protocol&order=asc", &backends)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if got := strings.Join(backendKeys(backends), ","); got != "default/api:80,ops/metrics:9090" {
		t.Fatalf("unexpected protocol-sorted backends: %s", got)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/backends?sort=invalid", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid backend sort, got %d", recorder.Code)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/backends?order=sideways", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid backend order, got %d", recorder.Code)
	}
}

func TestBackendEndpointsSupportPaginationContract(t *testing.T) {
	server := newTestServer(t)
	server.store.Publish(&ir.Snapshot{
		GeneratedAt: time.Now().UTC(),
		Backends: []ir.BackendCluster{
			{
				Name:      "metrics:9090",
				Namespace: "ops",
				Protocol:  "TCP",
				Metadata:  map[string]string{"service": "metrics"},
			},
			{
				Name:      "api:80",
				Namespace: "default",
				Protocol:  "HTTP",
				Metadata:  map[string]string{"service": "api"},
			},
		},
	})

	var backends []ir.BackendCluster
	recorder := performRequest(t, server, http.MethodGet, "/v1/backends?all=true&offset=1&limit=1", &backends)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if got := strings.Join(backendKeys(backends), ","); got != "ops/metrics:9090" {
		t.Fatalf("unexpected paginated backends: %s", got)
	}
	if got := recorder.Header().Get("X-Nantian-Page-Limit"); got != "1" {
		t.Fatalf("unexpected backend page limit header: %q", got)
	}
	if got := recorder.Header().Get("X-Nantian-Page-Offset"); got != "1" {
		t.Fatalf("unexpected backend page offset header: %q", got)
	}
	if got := recorder.Header().Get("X-Nantian-Total-Count"); got != "2" {
		t.Fatalf("unexpected backend total count header: %q", got)
	}
	if got := recorder.Header().Get("X-Nantian-Has-Next-Page"); got != "false" {
		t.Fatalf("unexpected backend has-next-page header: %q", got)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/backends?all=true&sort=name&order=desc&limit=1", &backends)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if got := strings.Join(backendKeys(backends), ","); got != "ops/metrics:9090" {
		t.Fatalf("unexpected sorted paginated backends: %s", got)
	}
	if got := recorder.Header().Get("X-Nantian-Page-Limit"); got != "1" {
		t.Fatalf("unexpected backend page limit header: %q", got)
	}
	if got := recorder.Header().Get("X-Nantian-Page-Offset"); got != "0" {
		t.Fatalf("unexpected backend page offset header: %q", got)
	}
	if got := recorder.Header().Get("X-Nantian-Total-Count"); got != "2" {
		t.Fatalf("unexpected backend total count header: %q", got)
	}
	if got := recorder.Header().Get("X-Nantian-Has-Next-Page"); got != "true" {
		t.Fatalf("unexpected backend has-next-page header: %q", got)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/backends?limit=0", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid backend limit, got %d", recorder.Code)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/backends?offset=-1", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid backend offset, got %d", recorder.Code)
	}
}

func TestNodeEndpointsSupportSortingContract(t *testing.T) {
	server := newTestServer(t)

	now := time.Now().UTC()
	server.nodes.ObserveAck(context.Background(), "dp-1", "kind", "v1", "v1", nil, now)
	server.nodes.ObserveReport(context.Background(), "dp-1", "v1", true, "ready", now)
	server.nodes.ObserveAck(context.Background(), "dp-2", "kind", "v9", "v9", nil, now)
	server.nodes.ObserveReport(context.Background(), "dp-2", "v9", false, "warming", now)

	var nodes []ir.NodeStatus
	recorder := performRequest(t, server, http.MethodGet, "/v1/nodes", &nodes)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if got := strings.Join(nodeIDs(nodes), ","); got != "dp-1,dp-2" {
		t.Fatalf("unexpected default node order: %s", got)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/nodes?sort=version&order=desc", &nodes)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if got := strings.Join(nodeIDs(nodes), ","); got != "dp-2,dp-1" {
		t.Fatalf("unexpected version-sorted nodes: %s", got)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/nodes?sort=invalid", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid node sort, got %d", recorder.Code)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/nodes?order=sideways", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid node order, got %d", recorder.Code)
	}
}

func TestNodeEndpointsSupportPaginationContract(t *testing.T) {
	server := newTestServer(t)

	now := time.Now().UTC()
	server.nodes.ObserveAck(context.Background(), "dp-1", "kind", "v1", "v1", nil, now)
	server.nodes.ObserveReport(context.Background(), "dp-1", "v1", true, "ready", now)
	server.nodes.ObserveAck(context.Background(), "dp-2", "kind", "v9", "v9", nil, now)
	server.nodes.ObserveReport(context.Background(), "dp-2", "v9", false, "warming", now)

	var nodes []ir.NodeStatus
	recorder := performRequest(t, server, http.MethodGet, "/v1/nodes?offset=1&limit=1", &nodes)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if got := strings.Join(nodeIDs(nodes), ","); got != "dp-2" {
		t.Fatalf("unexpected paginated nodes: %s", got)
	}
	if got := recorder.Header().Get("X-Nantian-Page-Limit"); got != "1" {
		t.Fatalf("unexpected node page limit header: %q", got)
	}
	if got := recorder.Header().Get("X-Nantian-Page-Offset"); got != "1" {
		t.Fatalf("unexpected node page offset header: %q", got)
	}
	if got := recorder.Header().Get("X-Nantian-Total-Count"); got != "2" {
		t.Fatalf("unexpected node total count header: %q", got)
	}
	if got := recorder.Header().Get("X-Nantian-Has-Next-Page"); got != "false" {
		t.Fatalf("unexpected node has-next-page header: %q", got)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/nodes?sort=version&order=desc&limit=1", &nodes)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if got := strings.Join(nodeIDs(nodes), ","); got != "dp-2" {
		t.Fatalf("unexpected sorted paginated nodes: %s", got)
	}
	if got := recorder.Header().Get("X-Nantian-Page-Limit"); got != "1" {
		t.Fatalf("unexpected node page limit header: %q", got)
	}
	if got := recorder.Header().Get("X-Nantian-Page-Offset"); got != "0" {
		t.Fatalf("unexpected node page offset header: %q", got)
	}
	if got := recorder.Header().Get("X-Nantian-Total-Count"); got != "2" {
		t.Fatalf("unexpected node total count header: %q", got)
	}
	if got := recorder.Header().Get("X-Nantian-Has-Next-Page"); got != "true" {
		t.Fatalf("unexpected node has-next-page header: %q", got)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/nodes?limit=0", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid node limit, got %d", recorder.Code)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/nodes?offset=-1", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid node offset, got %d", recorder.Code)
	}
}

func TestNodeEndpointsExposeStructuredDisconnectMetadata(t *testing.T) {
	server := newTestServer(t)
	now := time.Now().UTC()

	server.nodes.DisconnectWithReason(
		context.Background(),
		"dp-2",
		"ack_timeout",
		"timed out waiting for dataplane snapshot ack",
		now.Add(time.Second),
	)

	var node ir.NodeStatus
	recorder := performRequest(t, server, http.MethodGet, "/v1/nodes/dp-2", &node)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if node.DisconnectReason != "ack_timeout" {
		t.Fatalf("expected structured disconnect reason, got %+v", node)
	}
	if node.DisconnectedAt.IsZero() {
		t.Fatalf("expected disconnectedAt to be populated, got %+v", node)
	}
	if node.Message != "timed out waiting for dataplane snapshot ack" {
		t.Fatalf("expected disconnect message to be exposed, got %+v", node)
	}
}

func TestBackendEndpointsCanonicalizeH2CProtocolFilter(t *testing.T) {
	server := newTestServer(t)
	server.store.Publish(&ir.Snapshot{
		GeneratedAt: time.Now().UTC(),
		Backends: []ir.BackendCluster{
			{
				Name:      "http2-clear:8080",
				Namespace: "default",
				Protocol:  "H2C",
				Metadata:  map[string]string{"service": "http2-clear"},
			},
			{
				Name:      "api:80",
				Namespace: "default",
				Protocol:  "HTTP",
				Metadata:  map[string]string{"service": "api"},
			},
		},
	})

	var backends []ir.BackendCluster
	recorder := performRequest(t, server, http.MethodGet, "/v1/backends?protocol=h2c&all=true", &backends)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if len(backends) != 1 || backends[0].Name != "http2-clear:8080" || backends[0].Protocol != "H2C" {
		t.Fatalf("unexpected h2c backends: %+v", backends)
	}
}

func TestNodeEndpointsIncludeSharedRepositoryState(t *testing.T) {
	server := newTestServerWithRepository(t, &testNodeRepository{
		items: map[string]ir.NodeStatus{
			"dp-3": {
				NodeID:         "dp-3",
				Cluster:        "kind",
				Connected:      false,
				LastSeenAt:     time.Now().UTC(),
				LastAckVersion: "v1",
				Message:        "terminated",
			},
		},
	}, Options{})

	var node ir.NodeStatus
	recorder := performRequest(t, server, http.MethodGet, "/v1/nodes/dp-3", &node)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if node.NodeID != "dp-3" || node.Message != "terminated" {
		t.Fatalf("unexpected remote node detail: %+v", node)
	}
}

func TestNodeEndpointsFallbackToSharedLeaseState(t *testing.T) {
	now := time.Now().UTC()
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(coordinationv1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(&coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
					Name:      "nantian-gw-node-test",
				Namespace: "nantian-gw",
				Labels: map[string]string{
					nodeStatusManagedByLabelKey: nodeStatusManagedByLabelValue,
					nodeStatusComponentLabelKey: nodeStatusComponentLabelValue,
				},
				Annotations: map[string]string{
					nodeStatusNodeIDAnnotationKey: "dp-lease",
					nodeStatusAnnotationKey:       `{"nodeId":"dp-lease","cluster":"kind","connected":true,"ready":true,"lastAckVersion":"v2","message":"snapshot applied"}`,
				},
			},
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity:       ptr("dp-lease"),
				LeaseDurationSeconds: ptr(int32(300)),
				AcquireTime:          &metav1.MicroTime{Time: now.Add(-time.Minute)},
				RenewTime:            &metav1.MicroTime{Time: now},
			},
		}).
		Build()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	store.Publish(&ir.Snapshot{
		ID:          "lease-view",
		GeneratedAt: now,
		Workloads: []ir.Workload{
			{Namespace: "nantian-gw", Name: "dp-lease", IP: "10.0.0.10"},
			{Namespace: "nantian-gw", Name: "ops-console", IP: "10.0.0.20"},
		},
	})

	server := NewServer(
		":0",
		store,
		noderegistry.NewRegistry(ir.NewNodeStatusStore(), nil, logger, noderegistry.Options{PersistTimeout: time.Second}),
		NewResourceManager(client, logger),
		logger,
		Options{BearerToken: testAuthToken},
	)

	var nodes []ir.NodeStatus
	recorder := performRequest(t, server, http.MethodGet, "/v1/nodes", &nodes)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if len(nodes) != 1 || nodes[0].NodeID != "dp-lease" || !nodes[0].Connected || !nodes[0].Ready {
		t.Fatalf("unexpected nodes from lease fallback: %+v", nodes)
	}

	var summary Summary
	recorder = performRequest(t, server, http.MethodGet, "/v1/summary", &summary)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if summary.NodeCount != 1 || summary.ConnectedNodeCount != 1 || summary.ReadyNodeCount != 1 {
		t.Fatalf("unexpected summary from lease fallback: %+v", summary)
	}
}

func TestNodeEndpointsIgnoreStaleSharedLeaseState(t *testing.T) {
	now := time.Now().UTC()
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(coordinationv1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			&coordinationv1.Lease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nantian-gw-node-stale",
					Namespace: "nantian-gw",
					Labels: map[string]string{
						nodeStatusManagedByLabelKey: nodeStatusManagedByLabelValue,
						nodeStatusComponentLabelKey: nodeStatusComponentLabelValue,
					},
					Annotations: map[string]string{
						nodeStatusNodeIDAnnotationKey: "dp-stale",
						nodeStatusAnnotationKey:       `{"nodeId":"dp-stale","cluster":"kind","connected":true,"ready":true,"lastAckVersion":"v1","message":"old snapshot applied"}`,
					},
				},
				Spec: coordinationv1.LeaseSpec{
					HolderIdentity:       ptr("dp-stale"),
					LeaseDurationSeconds: ptr(int32(300)),
					AcquireTime:          &metav1.MicroTime{Time: now.Add(-20 * time.Minute)},
					RenewTime:            &metav1.MicroTime{Time: now.Add(-20 * time.Minute)},
				},
			},
			&coordinationv1.Lease{
				ObjectMeta: metav1.ObjectMeta{
						Name:      "nantian-gw-node-fresh",
					Namespace: "nantian-gw",
					Labels: map[string]string{
						nodeStatusManagedByLabelKey: nodeStatusManagedByLabelValue,
						nodeStatusComponentLabelKey: nodeStatusComponentLabelValue,
					},
					Annotations: map[string]string{
						nodeStatusNodeIDAnnotationKey: "dp-fresh",
						nodeStatusAnnotationKey:       `{"nodeId":"dp-fresh","cluster":"kind","connected":true,"ready":true,"lastAckVersion":"v2","message":"snapshot applied"}`,
					},
				},
				Spec: coordinationv1.LeaseSpec{
					HolderIdentity:       ptr("dp-fresh"),
					LeaseDurationSeconds: ptr(int32(300)),
					AcquireTime:          &metav1.MicroTime{Time: now.Add(-time.Minute)},
					RenewTime:            &metav1.MicroTime{Time: now},
				},
			},
		).
		Build()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	store.Publish(&ir.Snapshot{
		ID:          "lease-view",
		GeneratedAt: now,
		Workloads: []ir.Workload{
			{Namespace: "nantian-gw", Name: "dp-stale", IP: "10.0.0.10"},
			{Namespace: "nantian-gw", Name: "dp-fresh", IP: "10.0.0.11"},
		},
	})

	server := NewServer(
		":0",
		store,
		noderegistry.NewRegistry(ir.NewNodeStatusStore(), nil, logger, noderegistry.Options{PersistTimeout: time.Second}),
		NewResourceManager(client, logger),
		logger,
		Options{BearerToken: testAuthToken},
	)

	var nodes []ir.NodeStatus
	recorder := performRequest(t, server, http.MethodGet, "/v1/nodes", &nodes)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if len(nodes) != 1 || nodes[0].NodeID != "dp-fresh" {
		t.Fatalf("expected only fresh lease-backed node, got %+v", nodes)
	}
}
