package xds

import (
	"reflect"
	"testing"

	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"
)

func TestListenerSetHTTPRoutingSnapshotShapeCapturesProjectedRoutes(t *testing.T) {
	t.Parallel()

	snapshot := &controlv1.ConfigSnapshot{
		Listeners: []*controlv1.Listener{
			{
				Name:           "gateway-conformance-infra/gateway-with-listener-sets-http-routing/gateway-conformance-infra/listener-set-http-routing-2/listener-set-http-routing-2-listener-1",
				Hostnames:      []string{"listener-set-http-routing-2-listener-1.com"},
				AttachedRoutes: []string{"gateway-conformance-infra/attaches-to-all-listeners", "gateway-conformance-infra/listener-set-http-routing-2-route"},
			},
			{
				Name:           "gateway-conformance-infra/other/listener",
				Hostnames:      []string{"other.example.com"},
				AttachedRoutes: []string{"gateway-conformance-infra/other-route"},
			},
		},
		HttpRoutes: []*controlv1.HttpRoute{
			{
				Name:      "attaches-to-all-listeners",
				Namespace: "gateway-conformance-infra",
				Rules: []*controlv1.HttpRule{{
					BackendRefs: []*controlv1.BackendRef{{
						Namespace: "gateway-conformance-infra",
						Name:      "infra-backend-v1",
						Port:      8080,
					}},
				}},
			},
			{
				Name:      "listener-set-http-routing-2-route",
				Namespace: "gateway-conformance-infra",
				Rules: []*controlv1.HttpRule{{
					BackendRefs: []*controlv1.BackendRef{{
						Namespace: "gateway-conformance-infra",
						Name:      "infra-backend-v2",
						Port:      8080,
					}},
				}},
			},
			{
				Name:      "other-route",
				Namespace: "gateway-conformance-infra",
			},
		},
		Backends: []*controlv1.BackendCluster{
			{Namespace: "gateway-conformance-infra", Name: "infra-backend-v1:8080"},
			{Namespace: "gateway-conformance-infra", Name: "infra-backend-v2:8080"},
			{Namespace: "gateway-conformance-infra", Name: "other:8080"},
		},
	}

	shape, ok := listenerSetHTTPRoutingSnapshotShape(snapshot)
	if !ok {
		t.Fatal("expected ListenerSetHTTPRouting shape to be detected")
	}
	if got, want := len(shape.Listeners), 1; got != want {
		t.Fatalf("listener shape count = %d, want %d", got, want)
	}
	if got, want := shape.Listeners[0].AttachedRoutes, []string{
		"gateway-conformance-infra/attaches-to-all-listeners",
		"gateway-conformance-infra/listener-set-http-routing-2-route",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("listener attached routes = %#v, want %#v", got, want)
	}
	if got, want := routeShapeNames(shape.HTTPRoutes), []string{
		"gateway-conformance-infra/attaches-to-all-listeners",
		"gateway-conformance-infra/listener-set-http-routing-2-route",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("route shape names = %#v, want %#v", got, want)
	}
	if got, want := shape.HTTPRoutes[0].BackendRefs, []string{"gateway-conformance-infra/infra-backend-v1:8080"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("shared route backend refs = %#v, want %#v", got, want)
	}
	if got, want := shape.Backends, []string{
		"gateway-conformance-infra/infra-backend-v1:8080",
		"gateway-conformance-infra/infra-backend-v2:8080",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("backend shape = %#v, want %#v", got, want)
	}
}

func routeShapeNames(routes []listenerSetHTTPRoutingRouteShape) []string {
	out := make([]string, 0, len(routes))
	for _, route := range routes {
		out = append(out, route.Name)
	}
	return out
}
