package admin

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nantian-gw/gateway/internal/ir"
)

func TestListenerEndpointsSupportFilteringAndDetails(t *testing.T) {
	server := newTestServer(t)

	var listeners []ir.Listener
	recorder := performRequest(t, server, http.MethodGet, "/v1/listeners?protocol=listener_protocol_http&hostname=app.example.com", &listeners)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if len(listeners) != 1 || listeners[0].Name != "web" {
		t.Fatalf("unexpected listeners: %+v", listeners)
	}
	if listeners[0].Status == nil || listeners[0].Status.Accepted == nil || listeners[0].Status.Accepted.Status != "True" {
		t.Fatalf("expected listener status summary, got %+v", listeners[0].Status)
	}

	var detail ir.Listener
	recorder = performRequest(t, server, http.MethodGet, "/v1/listeners/web", &detail)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if detail.Protocol != "HTTP" || detail.Name != "web" {
		t.Fatalf("unexpected listener detail: %+v", detail)
	}
	if len(detail.Addresses) != 2 || detail.Addresses[0] != "192.0.2.10" || detail.Addresses[1] != "gw.example.com" {
		t.Fatalf("unexpected listener addresses: %+v", detail.Addresses)
	}
	if detail.Status == nil || detail.Status.Programmed == nil || detail.Status.Programmed.Status != "True" {
		t.Fatalf("unexpected listener status detail: %+v", detail.Status)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/listeners?protocol=invalid", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid protocol filter, got %d", recorder.Code)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/listeners/missing", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", recorder.Code)
	}
}

func TestListenerEndpointsSupportSortingContract(t *testing.T) {
	server := newTestServer(t)

	var listeners []ir.Listener
	recorder := performRequest(t, server, http.MethodGet, "/v1/listeners", &listeners)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if got := strings.Join(listenerNames(listeners), ","); got != "passthrough,web" {
		t.Fatalf("unexpected default listener order: %s", got)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/listeners?sort=protocol&order=asc", &listeners)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if got := strings.Join(listenerNames(listeners), ","); got != "web,passthrough" {
		t.Fatalf("unexpected protocol-sorted listeners: %s", got)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/listeners?sort=invalid", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid listener sort, got %d", recorder.Code)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/listeners?order=sideways", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid listener order, got %d", recorder.Code)
	}
}

func TestListenerEndpointsSupportPaginationContract(t *testing.T) {
	server := newTestServer(t)

	var listeners []ir.Listener
	recorder := performRequest(t, server, http.MethodGet, "/v1/listeners?offset=1&limit=1", &listeners)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if got := strings.Join(listenerNames(listeners), ","); got != "web" {
		t.Fatalf("unexpected paginated listeners: %s", got)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/listeners?sort=protocol&order=asc&limit=1", &listeners)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if got := strings.Join(listenerNames(listeners), ","); got != "web" {
		t.Fatalf("unexpected sorted paginated listeners: %s", got)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/listeners?limit=0", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid listener limit, got %d", recorder.Code)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/listeners?offset=-1", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid listener offset, got %d", recorder.Code)
	}
}

func TestListenersClampLimitAndEmitPaginationHeaders(t *testing.T) {
	t.Parallel()

	server := newTestServerWithOptions(t, Options{MaxListItems: 1})

	var listeners []ir.Listener
	recorder := performRequest(t, server, http.MethodGet, "/v1/listeners?offset=0&limit=5", &listeners)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if len(listeners) != 1 {
		t.Fatalf("expected clamped listener page size of 1, got %d", len(listeners))
	}
	if got := recorder.Header().Get("X-Nantian-Page-Limit"); got != "1" {
		t.Fatalf("unexpected effective page limit header: %q", got)
	}
	if got := recorder.Header().Get("X-Nantian-Page-Offset"); got != "0" {
		t.Fatalf("unexpected page offset header: %q", got)
	}
	if got := recorder.Header().Get("X-Nantian-Total-Count"); got != "2" {
		t.Fatalf("unexpected total count header: %q", got)
	}
	if got := recorder.Header().Get("X-Nantian-Has-Next-Page"); got != "true" {
		t.Fatalf("unexpected has-next-page header: %q", got)
	}
}

func TestSnapshotSyncEndpointReturnsAckAndReadinessBreakdown(t *testing.T) {
	server := newTestServerWithOptions(t, Options{ReadinessMode: readinessModeCurrentSnapshotAll, NodeDriftWarningThreshold: 15 * time.Second})
	server.now = func() time.Time {
		return server.store.Current().GeneratedAt.Add(20 * time.Second)
	}
	currentVersion := server.store.Current().ID
	server.nodes.Connect(context.Background(), "dp-3", "kind", []string{"routes"}, server.now())
	server.nodes.ObservePublished(context.Background(), "dp-3", currentVersion, server.now())
	server.nodes.ObserveAck(context.Background(), "dp-3", "kind", "older-version", currentVersion, nil, server.now())
	server.nodes.ObserveReport(context.Background(), "dp-3", "older-version", false, "stale ack", server.now())

	var response SnapshotSyncResponse
	recorder := performRequest(t, server, http.MethodGet, "/v1/snapshot-sync", &response)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if response.ReadinessMode != readinessModeCurrentSnapshotAll {
		t.Fatalf("unexpected readiness mode: %+v", response)
	}
	if response.SnapshotVersion != currentVersion {
		t.Fatalf("unexpected snapshot sync response: %+v", response)
	}
	if response.Summary.DriftedNodeCount != 1 || response.Summary.AwaitingReadyNodeCount != 0 {
		t.Fatalf("unexpected sync breakdown: %+v", response.Summary)
	}
	if len(response.Nodes) != 3 {
		t.Fatalf("expected visible workload nodes, got %+v", response.Nodes)
	}

	var driftedNode SnapshotSyncNode
	for _, node := range response.Nodes {
		if node.NodeID == "dp-3" {
			driftedNode = node
			break
		}
	}
	if driftedNode.NodeID == "" || driftedNode.State != "drifted" {
		t.Fatalf("expected dp-3 to be drifted, got %+v", response.Nodes)
	}
	if !strings.Contains(driftedNode.Reason, "older snapshot") {
		t.Fatalf("expected drift reason, got %+v", driftedNode)
	}
}

func TestSnapshotEndpointIncludesConditionStatusDetails(t *testing.T) {
	server := newTestServer(t)

	var snapshot ir.Snapshot
	recorder := performRequest(t, server, http.MethodGet, "/v1/snapshot", &snapshot)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if len(snapshot.Listeners) != 2 {
		t.Fatalf("unexpected listeners in snapshot: %+v", snapshot.Listeners)
	}
	var webListener *ir.Listener
	for i := range snapshot.Listeners {
		if snapshot.Listeners[i].Name == "web" {
			webListener = &snapshot.Listeners[i]
			break
		}
	}
	if webListener == nil {
		t.Fatalf("expected web listener in snapshot, got %+v", snapshot.Listeners)
	}
	if webListener.Status == nil || webListener.Status.ResolvedRefs == nil {
		t.Fatalf("expected listener status summary in snapshot, got %+v", webListener.Status)
	}
	if webListener.Status.ResolvedRefs.Status != "True" {
		t.Fatalf("unexpected listener resolved refs summary: %+v", webListener.Status.ResolvedRefs)
	}
	if len(snapshot.HTTPRoutes) != 1 || snapshot.HTTPRoutes[0].Status == nil || len(snapshot.HTTPRoutes[0].Status.Parents) != 1 {
		t.Fatalf("unexpected http route snapshot status: %+v", snapshot.HTTPRoutes)
	}
	parent := snapshot.HTTPRoutes[0].Status.Parents[0]
	if parent.Accepted == nil || parent.ResolvedRefs == nil {
		t.Fatalf("expected route parent condition summaries, got %+v", parent)
	}
	if parent.Accepted.Status != "True" || parent.ResolvedRefs.Status != "True" {
		t.Fatalf("unexpected route parent status summaries: %+v", parent)
	}
}

func TestListenerEndpointsPreferDisplayAddresses(t *testing.T) {
	server := newTestServer(t)

	server.store.Publish(&ir.Snapshot{
		GeneratedAt: time.Now().UTC(),
		Listeners: []ir.Listener{{
			Name:      "web",
			Address:   "0.0.0.0",
			Addresses: []string{"0.0.0.0"},
			Port:      80,
			Protocol:  "HTTP",
			Metadata: map[string]string{
				listenerDisplayAddressesMetadataKey: "127.0.0.1,gw.example.com",
			},
		}},
	})

	var listeners []ir.Listener
	recorder := performRequest(t, server, http.MethodGet, "/v1/listeners", &listeners)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if len(listeners) != 1 {
		t.Fatalf("unexpected listeners: %+v", listeners)
	}
	if listeners[0].Address != "127.0.0.1" {
		t.Fatalf("unexpected display address: %q", listeners[0].Address)
	}
	if len(listeners[0].Addresses) != 2 || listeners[0].Addresses[0] != "127.0.0.1" || listeners[0].Addresses[1] != "gw.example.com" {
		t.Fatalf("unexpected display addresses: %+v", listeners[0].Addresses)
	}
}

func TestRouteEndpointsSupportFilteringAndDetails(t *testing.T) {
	server := newTestServer(t)

	var routes routeListResponse
	recorder := performRequest(t, server, http.MethodGet, "/v1/routes?kind=tlsroute&hostname=secure.example.com", &routes)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if len(routes.Stream) != 1 || routes.Stream[0].Name != "passthrough" {
		t.Fatalf("unexpected stream routes: %+v", routes.Stream)
	}
	if len(routes.HTTP) != 0 || len(routes.GRPC) != 0 {
		t.Fatalf("expected only stream routes, got %+v", routes)
	}

	var detail ir.StreamRoute
	recorder = performRequest(t, server, http.MethodGet, "/v1/routes/tls/default/passthrough", &detail)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if detail.Kind != "TLS" || detail.Namespace != "default" {
		t.Fatalf("unexpected route detail: %+v", detail)
	}
	if detail.Status == nil || len(detail.Status.Parents) != 1 || detail.Status.Parents[0].Accepted == nil || detail.Status.Parents[0].Accepted.Status != "True" {
		t.Fatalf("unexpected route status detail: %+v", detail.Status)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/routes/not-a-kind/default/passthrough", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid route kind, got %d", recorder.Code)
	}
}

func TestRouteEndpointsSupportSortingAndKindScopedPagination(t *testing.T) {
	server := newTestServer(t)
	server.store.Publish(&ir.Snapshot{
		GeneratedAt: time.Now().UTC(),
		HTTPRoutes: []ir.HTTPRoute{
			{Name: "web", Namespace: "default", Hostnames: []string{"web.example.com"}},
			{Name: "api", Namespace: "team-b", Hostnames: []string{"api.example.com"}},
		},
		GRPCRoutes: []ir.GRPCRoute{
			{Name: "payments", Namespace: "ops", Hostnames: []string{"payments.example.com"}},
			{Name: "accounts", Namespace: "default", Hostnames: []string{"accounts.example.com"}},
		},
		StreamRoutes: []ir.StreamRoute{
			{Name: "z-tcp", Namespace: "ops", Kind: "TCP"},
			{Name: "a-tls", Namespace: "default", Kind: "TLS"},
		},
	})

	var routes routeListResponse
	recorder := performRequest(t, server, http.MethodGet, "/v1/routes", &routes)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if got := strings.Join(httpRouteKeys(routes.HTTP), ","); got != "default/web,team-b/api" {
		t.Fatalf("unexpected default HTTP route order: %s", got)
	}
	if got := strings.Join(grpcRouteKeys(routes.GRPC), ","); got != "default/accounts,ops/payments" {
		t.Fatalf("unexpected default GRPC route order: %s", got)
	}
	if got := strings.Join(streamRouteKeys(routes.Stream), ","); got != "default/a-tls,ops/z-tcp" {
		t.Fatalf("unexpected default stream route order: %s", got)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/routes?sort=name&order=desc", &routes)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if got := strings.Join(httpRouteKeys(routes.HTTP), ","); got != "default/web,team-b/api" {
		t.Fatalf("unexpected name-desc HTTP route order: %s", got)
	}
	if got := strings.Join(grpcRouteKeys(routes.GRPC), ","); got != "ops/payments,default/accounts" {
		t.Fatalf("unexpected name-desc GRPC route order: %s", got)
	}
	if got := strings.Join(streamRouteKeys(routes.Stream), ","); got != "ops/z-tcp,default/a-tls" {
		t.Fatalf("unexpected name-desc stream route order: %s", got)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/routes?kind=grpc&sort=name&order=desc&limit=1&offset=1", &routes)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if len(routes.HTTP) != 0 || len(routes.Stream) != 0 {
		t.Fatalf("expected only paginated grpc routes, got %+v", routes)
	}
	if got := strings.Join(grpcRouteKeys(routes.GRPC), ","); got != "default/accounts" {
		t.Fatalf("unexpected paginated GRPC route order: %s", got)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/routes?limit=1", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when route pagination omits kind, got %d", recorder.Code)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/routes?sort=broken", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid route sort, got %d", recorder.Code)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/routes?order=sideways", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid route order, got %d", recorder.Code)
	}
}

func TestRoutesEmitPaginationHeadersForKindScopedPagination(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	server.store.Publish(&ir.Snapshot{
		GeneratedAt: time.Now().UTC(),
		GRPCRoutes: []ir.GRPCRoute{
			{Name: "payments", Namespace: "ops", Hostnames: []string{"payments.example.com"}},
			{Name: "accounts", Namespace: "default", Hostnames: []string{"accounts.example.com"}},
		},
	})

	var routes routeListResponse
	recorder := performRequest(t, server, http.MethodGet, "/v1/routes?kind=grpc&limit=1&offset=1", &routes)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("X-Nantian-Page-Limit"); got != "1" {
		t.Fatalf("unexpected effective page limit header: %q", got)
	}
	if got := recorder.Header().Get("X-Nantian-Page-Offset"); got != "1" {
		t.Fatalf("unexpected page offset header: %q", got)
	}
	if got := recorder.Header().Get("X-Nantian-Total-Count"); got != "2" {
		t.Fatalf("unexpected total count header: %q", got)
	}
	if got := recorder.Header().Get("X-Nantian-Has-Next-Page"); got != "false" {
		t.Fatalf("unexpected has-next-page header: %q", got)
	}
}
