package admin

import (
	"net/http"
	"reflect"
	"testing"
)

func TestTopologyEndpointReturnsFlowGraph(t *testing.T) {
	t.Parallel()

	server := newTestServerWithResourceManager(t, resourceManagerForTest(t))

	var topology TopologyResponse
	recorder := performRequest(t, server, http.MethodGet, "/v1/topology", &topology)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if topology.Summary.ListenerCount != 2 {
		t.Fatalf("unexpected topology summary: %+v", topology.Summary)
	}
	if len(topology.Nodes) == 0 || len(topology.Edges) == 0 {
		t.Fatalf("expected topology graph, got %+v", topology)
	}
}

func TestTopologyEndpointSupportsDrilldownFiltering(t *testing.T) {
	t.Parallel()

	server := newTestServerWithResourceManager(t, resourceManagerForTest(t))

	var topology TopologyResponse
	recorder := performRequest(
		t,
		server,
		http.MethodGet,
		"/v1/topology?type=route&kind=http&namespace=default&name=web&includeRelated=true",
		&topology,
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if topology.Summary.ListenerCount != 2 || topology.Summary.RouteCount != 1 || topology.Summary.BackendCount != 1 {
		t.Fatalf("expected full topology summary to remain intact, got %+v", topology.Summary)
	}
	expectedNodeIDs := []string{
		"backend:default/api:80",
		"endpoint-set:default/api:80",
		"listener:web",
		"plane:controlplane",
		"plane:dataplane",
		"route:HTTPRoute:default/web",
	}
	if got := topologyNodeIDs(topology.Nodes); !reflect.DeepEqual(got, expectedNodeIDs) {
		t.Fatalf("unexpected topology node drilldown: got %v want %v", got, expectedNodeIDs)
	}
	expectedEdgeIDs := []string{
		"edge:backend:default/api:80:endpoint-set:default/api:80",
		"edge:controlplane:dataplane",
		"edge:listener:web:route:HTTPRoute:default/web",
		"edge:plane:dataplane:listener:web",
		"edge:route:HTTPRoute:default/web:backend:default/api:80",
	}
	if got := topologyEdgeIDs(topology.Edges); !reflect.DeepEqual(got, expectedEdgeIDs) {
		t.Fatalf("unexpected topology edge drilldown: got %v want %v", got, expectedEdgeIDs)
	}

	recorder = performRequest(
		t,
		server,
		http.MethodGet,
		"/v1/topology?type=listener&status=tls",
		&topology,
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 for listener filter, got %d: %s", recorder.Code, recorder.Body.String())
	}
	expectedNodeIDs = []string{"listener:passthrough"}
	if got := topologyNodeIDs(topology.Nodes); !reflect.DeepEqual(got, expectedNodeIDs) {
		t.Fatalf("unexpected filtered topology nodes: got %v want %v", got, expectedNodeIDs)
	}
	if len(topology.Edges) != 0 {
		t.Fatalf("expected no edges without related expansion, got %+v", topology.Edges)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/topology?type=broken", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid type 400, got %d: %s", recorder.Code, recorder.Body.String())
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/topology?kind=broken", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid kind 400, got %d: %s", recorder.Code, recorder.Body.String())
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/topology?includeRelated=maybe", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid includeRelated 400, got %d: %s", recorder.Code, recorder.Body.String())
	}
}
