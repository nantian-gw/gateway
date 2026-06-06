package admin

import (
	"testing"
	"time"

	"github.com/nantian-gw/gateway/internal/ir"
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
