package admin

import (
	"testing"
	"time"

	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/observability"
)

func TestSnapshotDetailIndexCacheReusesSnapshotAndRefreshesOnPublish(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	snapshot := server.store.Current()

	first := server.snapshotDetailIndex(snapshot)
	if first == nil {
		t.Fatal("expected snapshot detail index")
	}
	second := server.snapshotDetailIndex(snapshot)
	if first != second {
		t.Fatal("expected identical snapshot to reuse cached detail index")
	}

	listener, ok := first.listener("web")
	if !ok || listener.Name != "web" || listener.Address != "192.0.2.10" {
		t.Fatalf("unexpected indexed listener: %+v ok=%v", listener, ok)
	}

	backend, ok := first.backend("default", "api:80")
	if !ok || backend.Name != "api:80" || backend.Namespace != "default" {
		t.Fatalf("unexpected indexed backend: %+v ok=%v", backend, ok)
	}

	route, ok, err := first.route("http", "default", "web")
	if err != nil {
		t.Fatalf("expected indexed route lookup to succeed, got %v", err)
	}
	httpRoute, typed := route.(ir.HTTPRoute)
	if !ok || !typed || httpRoute.Name != "web" || httpRoute.Namespace != "default" {
		t.Fatalf("unexpected indexed route: %#v ok=%v typed=%v", route, ok, typed)
	}

	server.store.Publish(&ir.Snapshot{
		ID:          "v-next",
		GeneratedAt: time.Now().UTC(),
		Listeners: []ir.Listener{{
			Name:     "edge",
			Protocol: "HTTP",
		}},
		HTTPRoutes: []ir.HTTPRoute{{
			Name:      "edge",
			Namespace: "default",
		}},
		Backends: []ir.BackendCluster{{
			Name:      "edge:8080",
			Namespace: "default",
			Protocol:  "HTTP",
			Metadata: map[string]string{
				"service": "edge:8080",
			},
		}},
	})

	refreshedSnapshot := server.store.Current()
	refreshed := server.snapshotDetailIndex(refreshedSnapshot)
	if refreshed == nil {
		t.Fatal("expected refreshed snapshot detail index")
	}
	if refreshed == first {
		t.Fatal("expected published snapshot to refresh detail index cache")
	}
	if _, ok := refreshed.listener("web"); ok {
		t.Fatal("expected refreshed index to drop previous snapshot listener")
	}
	if listener, ok := refreshed.listener("edge"); !ok || listener.Name != "edge" {
		t.Fatalf("unexpected refreshed listener lookup: %+v ok=%v", listener, ok)
	}
}

func TestSnapshotDetailIndexVisibleBackendsRefreshWithSnapshot(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	current := server.store.Current()

	first := server.snapshotDetailIndex(current)
	if first == nil {
		t.Fatal("expected snapshot detail index")
	}
	if got := len(first.visibleBackendList()); got != 1 {
		t.Fatalf("visible backend count = %d, want 1", got)
	}
	if got := first.visibleBackendList()[0].Name; got != "api:80" {
		t.Fatalf("unexpected initial visible backend: %q", got)
	}

	server.store.Publish(&ir.Snapshot{
		ID:          "v-visible-next",
		GeneratedAt: time.Now().UTC(),
		HTTPRoutes: []ir.HTTPRoute{
			{
				Name:      "web",
				Namespace: "default",
				Rules: []ir.HTTPRule{{
					BackendRefs: []ir.BackendRef{{
						Name: "api",
						Port: 80,
					}},
				}},
			},
			{
				Name:      "metrics",
				Namespace: "ops",
				Rules: []ir.HTTPRule{{
					BackendRefs: []ir.BackendRef{{
						Name: "metrics",
						Port: 9090,
					}},
				}},
			},
		},
		Backends: []ir.BackendCluster{
			{
				Name:      "api:80",
				Namespace: "default",
				Protocol:  "HTTP",
				Metadata:  map[string]string{"service": "api"},
			},
			{
				Name:      "metrics:9090",
				Namespace: "ops",
				Protocol:  "TCP",
				Metadata:  map[string]string{"service": "metrics"},
			},
		},
	})

	refreshed := server.snapshotDetailIndex(server.store.Current())
	if refreshed == nil {
		t.Fatal("expected refreshed snapshot detail index")
	}
	if refreshed == first {
		t.Fatal("expected new snapshot pointer to rebuild detail index")
	}
	if got := len(refreshed.visibleBackendList()); got != 2 {
		t.Fatalf("visible backend count after publish = %d, want 2", got)
	}
	if _, ok := refreshed.backend("ops", "metrics:9090"); !ok {
		t.Fatal("expected refreshed visible backend lookup to include ops/metrics:9090")
	}
}

func TestSnapshotDetailIndexRecordsBuildDurationOnlyOnCacheMiss(t *testing.T) {
	t.Parallel()

	metrics := observability.NewMetrics()
	server := newTestServerWithOptions(t, Options{Metrics: metrics})
	snapshot := server.store.Current()

	if first := server.snapshotDetailIndex(snapshot); first == nil {
		t.Fatal("expected initial snapshot detail index")
	}
	if second := server.snapshotDetailIndex(snapshot); second == nil {
		t.Fatal("expected cached snapshot detail index")
	}
	if got := histogramSampleCount(t, metrics.AdminDetailIndexBuildDurationSeconds); got != 1 {
		t.Fatalf("detail index build duration sample count = %d, want 1 after cached reuse", got)
	}

	server.store.Publish(&ir.Snapshot{
		ID:          "v-detail-index-metrics",
		GeneratedAt: time.Now().UTC(),
		Listeners: []ir.Listener{{
			Name:     "metrics",
			Protocol: "HTTP",
		}},
	})
	if refreshed := server.snapshotDetailIndex(server.store.Current()); refreshed == nil {
		t.Fatal("expected refreshed snapshot detail index")
	}
	if got := histogramSampleCount(t, metrics.AdminDetailIndexBuildDurationSeconds); got != 2 {
		t.Fatalf("detail index build duration sample count = %d, want 2 after snapshot refresh", got)
	}
}
