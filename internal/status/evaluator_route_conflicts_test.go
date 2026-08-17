package status

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestObserveRouteHostnameOverlapsKeepsBothRoutesAccepted(t *testing.T) {
	gateway := gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "nantian-gw",
			Listeners: []gatewayv1.Listener{{
				Name:     "http",
				Protocol: gatewayv1.HTTPProtocolType,
				Port:     80,
				AllowedRoutes: &gatewayv1.AllowedRoutes{
					Namespaces: &gatewayv1.RouteNamespaces{
						From: ptr(gatewayv1.NamespacesFromAll),
					},
				},
			}},
		},
	}

	older := gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "older",
			Namespace:         "default",
			Generation:        1,
			CreationTimestamp: metav1.NewTime(t0()),
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{{
					Name:      "gw",
					Namespace: ptr(gatewayv1.Namespace("default")),
				}},
			},
			Hostnames: []gatewayv1.Hostname{"api.example.com"},
		},
	}
	newer := older.DeepCopy()
	newer.Name = "newer"
	newer.CreationTimestamp = metav1.NewTime(t0().Add(1))

	state := &clusterState{
		controllerName:  "gateway.networking.k8s.io/nantian-gw",
		gateways:        []gatewayv1.Gateway{gateway},
		managedGateways: []gatewayv1.Gateway{gateway},
		managedGatewayByKey: map[string]gatewayv1.Gateway{
			"default/gw": gateway,
		},
		httpRoutes: []gatewayv1.HTTPRoute{older, *newer},
		namespaces: []corev1.Namespace{
			{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		},
	}
	state.index()

	routeHostnameOverlapTotal.Reset()
	defer routeHostnameOverlapTotal.Reset()

	out := evaluateRoutes(state)

	for _, route := range []gatewayv1.HTTPRoute{older, *newer} {
		evals := out.http[client.ObjectKeyFromObject(&route)]
		if len(evals) != 1 {
			t.Fatalf("expected 1 evaluation for route %s, got %d", route.Name, len(evals))
		}
		if evals[0].acceptedCondition.Status != metav1.ConditionTrue {
			t.Fatalf("route %s accepted status = %s, want True", route.Name, evals[0].acceptedCondition.Status)
		}
		if len(evals[0].extraConditions) != 0 {
			t.Fatalf("route %s should not be rejected; overlap is observability-only, got extraConditions %#v", route.Name, evals[0].extraConditions)
		}
	}

	// Both routes must remain attached to the listener: the Gateway API spec
	// requires intersecting hostnames on the same listener to all be Accepted and
	// counted in AttachedRoutes (conformance: HTTPRouteHostnameIntersection).
	listener := listenerKey{gatewayNamespace: "default", gatewayName: "gw", listenerName: "http"}
	for _, route := range []gatewayv1.HTTPRoute{older, *newer} {
		if _, stillAttached := out.attachments[listener][client.ObjectKeyFromObject(&route)]; !stillAttached {
			t.Fatalf("route %s should remain attached to the listener", route.Name)
		}
	}

	// The overlap is reported observably: the counter for the listener is bumped once
	// for the single overlapping pair.
	if got := testutil.ToFloat64(routeHostnameOverlapTotal.WithLabelValues("default/gw/http")); got != 1 {
		t.Fatalf("route hostname overlap counter = %v, want 1", got)
	}
}

func TestObserveRouteHostnameOverlapsKeepsDistinctHostnames(t *testing.T) {
	gateway := gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "nantian-gw",
			Listeners: []gatewayv1.Listener{{
				Name:     "http",
				Protocol: gatewayv1.HTTPProtocolType,
				Port:     80,
				AllowedRoutes: &gatewayv1.AllowedRoutes{
					Namespaces: &gatewayv1.RouteNamespaces{
						From: ptr(gatewayv1.NamespacesFromAll),
					},
				},
			}},
		},
	}

	routeA := gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default", Generation: 1},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{{
					Name:      "gw",
					Namespace: ptr(gatewayv1.Namespace("default")),
				}},
			},
			Hostnames: []gatewayv1.Hostname{"api.example.com"},
		},
	}
	routeB := routeA.DeepCopy()
	routeB.Name = "b"
	routeB.Spec.Hostnames = []gatewayv1.Hostname{"other.example.com"}

	state := &clusterState{
		controllerName:  "gateway.networking.k8s.io/nantian-gw",
		gateways:        []gatewayv1.Gateway{gateway},
		managedGateways: []gatewayv1.Gateway{gateway},
		managedGatewayByKey: map[string]gatewayv1.Gateway{
			"default/gw": gateway,
		},
		httpRoutes: []gatewayv1.HTTPRoute{routeA, *routeB},
		namespaces: []corev1.Namespace{
			{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		},
	}
	state.index()

	out := evaluateRoutes(state)

	for _, route := range []gatewayv1.HTTPRoute{routeA, *routeB} {
		evals := out.http[client.ObjectKeyFromObject(&route)]
		if len(evals) != 1 {
			t.Fatalf("expected 1 evaluation for route %s, got %d", route.Name, len(evals))
		}
		if evals[0].acceptedCondition.Status != metav1.ConditionTrue {
			t.Fatalf("route %s accepted status = %s, want True", route.Name, evals[0].acceptedCondition.Status)
		}
		if len(evals[0].extraConditions) != 0 {
			t.Fatalf("route %s should have no extra conditions, got %#v", route.Name, evals[0].extraConditions)
		}
	}
}

func TestObserveRouteHostnameOverlapsCatchAllWithSpecificHostname(t *testing.T) {
	gateway := gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "nantian-gw",
			Listeners: []gatewayv1.Listener{{
				Name:     "http",
				Protocol: gatewayv1.HTTPProtocolType,
				Port:     80,
				AllowedRoutes: &gatewayv1.AllowedRoutes{
					Namespaces: &gatewayv1.RouteNamespaces{
						From: ptr(gatewayv1.NamespacesFromAll),
					},
				},
			}},
		},
	}

	specific := gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "specific",
			Namespace:         "default",
			Generation:        1,
			CreationTimestamp: metav1.NewTime(t0()),
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{{
					Name:      "gw",
					Namespace: ptr(gatewayv1.Namespace("default")),
				}},
			},
			Hostnames: []gatewayv1.Hostname{"api.example.com"},
		},
	}
	catchAll := specific.DeepCopy()
	catchAll.Name = "catch-all"
	catchAll.CreationTimestamp = metav1.NewTime(t0().Add(1))
	catchAll.Spec.Hostnames = nil

	state := &clusterState{
		controllerName:  "gateway.networking.k8s.io/nantian-gw",
		gateways:        []gatewayv1.Gateway{gateway},
		managedGateways: []gatewayv1.Gateway{gateway},
		managedGatewayByKey: map[string]gatewayv1.Gateway{
			"default/gw": gateway,
		},
		httpRoutes: []gatewayv1.HTTPRoute{specific, *catchAll},
		namespaces: []corev1.Namespace{
			{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		},
	}
	state.index()

	out := evaluateRoutes(state)

	catchAllEvals := out.http[client.ObjectKeyFromObject(catchAll)]
	if len(catchAllEvals) != 1 {
		t.Fatalf("expected 1 evaluation for catch-all route, got %d", len(catchAllEvals))
	}
	if catchAllEvals[0].acceptedCondition.Status != metav1.ConditionTrue {
		t.Fatalf("catch-all route accepted status = %s, want True", catchAllEvals[0].acceptedCondition.Status)
	}
	if len(catchAllEvals[0].extraConditions) != 0 {
		t.Fatalf("catch-all route should not be marked conflicted (specific hostname wins), got %#v", catchAllEvals[0].extraConditions)
	}

	specificEvals := out.http[client.ObjectKeyFromObject(&specific)]
	if len(specificEvals) != 1 {
		t.Fatalf("expected 1 evaluation for specific route, got %d", len(specificEvals))
	}
	if specificEvals[0].acceptedCondition.Status != metav1.ConditionTrue {
		t.Fatalf("specific route accepted status = %s, want True", specificEvals[0].acceptedCondition.Status)
	}
}

func TestObserveRouteHostnameOverlapsIgnoresDistinctListeners(t *testing.T) {
	gateway := gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "nantian-gw",
			Listeners: []gatewayv1.Listener{
				{
					Name:     "http-a",
					Protocol: gatewayv1.HTTPProtocolType,
					Port:     80,
					AllowedRoutes: &gatewayv1.AllowedRoutes{
						Namespaces: &gatewayv1.RouteNamespaces{
							From: ptr(gatewayv1.NamespacesFromAll),
						},
					},
				},
				{
					Name:     "http-b",
					Protocol: gatewayv1.HTTPProtocolType,
					Port:     81,
					AllowedRoutes: &gatewayv1.AllowedRoutes{
						Namespaces: &gatewayv1.RouteNamespaces{
							From: ptr(gatewayv1.NamespacesFromAll),
						},
					},
				},
			},
		},
	}

	routeA := gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "a",
			Namespace:         "default",
			Generation:        1,
			CreationTimestamp: metav1.NewTime(t0()),
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{{
					Name:      "gw",
					Namespace: ptr(gatewayv1.Namespace("default")),
					SectionName: func() *gatewayv1.SectionName {
						s := gatewayv1.SectionName("http-a")
						return &s
					}(),
				}},
			},
			Hostnames: []gatewayv1.Hostname{"api.example.com"},
		},
	}
	routeB := routeA.DeepCopy()
	routeB.Name = "b"
	routeB.CreationTimestamp = metav1.NewTime(t0().Add(1))
	routeB.Spec.ParentRefs[0].SectionName = func() *gatewayv1.SectionName {
		s := gatewayv1.SectionName("http-b")
		return &s
	}()

	state := &clusterState{
		controllerName:  "gateway.networking.k8s.io/nantian-gw",
		gateways:        []gatewayv1.Gateway{gateway},
		managedGateways: []gatewayv1.Gateway{gateway},
		managedGatewayByKey: map[string]gatewayv1.Gateway{
			"default/gw": gateway,
		},
		httpRoutes: []gatewayv1.HTTPRoute{routeA, *routeB},
		namespaces: []corev1.Namespace{
			{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		},
	}
	state.index()

	out := evaluateRoutes(state)

	for _, route := range []gatewayv1.HTTPRoute{routeA, *routeB} {
		evals := out.http[client.ObjectKeyFromObject(&route)]
		if len(evals) != 1 {
			t.Fatalf("expected 1 evaluation for route %s, got %d", route.Name, len(evals))
		}
		if evals[0].acceptedCondition.Status != metav1.ConditionTrue {
			t.Fatalf("route %s accepted status = %s, want True", route.Name, evals[0].acceptedCondition.Status)
		}
		if len(evals[0].extraConditions) != 0 {
			t.Fatalf("route %s should have no extra conditions, got %#v", route.Name, evals[0].extraConditions)
		}
	}
}

func t0() time.Time {
	t, _ := time.Parse(time.RFC3339, "2024-01-01T00:00:00Z")
	return t
}
