package status

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestEvaluateRouteConflictsMarksLaterRouteConflicted(t *testing.T) {
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

	out := evaluateRoutes(state)

	olderEvals := out.http[client.ObjectKeyFromObject(&older)]
	if len(olderEvals) != 1 {
		t.Fatalf("expected 1 evaluation for older route, got %d", len(olderEvals))
	}
	if olderEvals[0].acceptedCondition.Status != metav1.ConditionTrue {
		t.Fatalf("older route accepted status = %s, want True", olderEvals[0].acceptedCondition.Status)
	}
	if len(olderEvals[0].extraConditions) != 0 {
		t.Fatalf("older route should have no extra conditions, got %#v", olderEvals[0].extraConditions)
	}

	newerEvals := out.http[client.ObjectKeyFromObject(newer)]
	if len(newerEvals) != 1 {
		t.Fatalf("expected 1 evaluation for newer route, got %d", len(newerEvals))
	}
	conflict := findConditionByType(newerEvals[0].extraConditions, string(gatewayv1.RouteConditionAccepted))
	if conflict == nil {
		t.Fatalf("newer route should carry a conflicted Accepted condition, got extraConditions %#v", newerEvals[0].extraConditions)
	}
	if conflict.Status != metav1.ConditionFalse {
		t.Fatalf("newer route conflict status = %s, want False", conflict.Status)
	}
	if conflict.Reason != routeReasonHostnameConflict {
		t.Fatalf("newer route conflict reason = %s, want %s", conflict.Reason, routeReasonHostnameConflict)
	}

	// The conflicted route must not count as attached to the listener.
	listener := listenerKey{gatewayNamespace: "default", gatewayName: "gw", listenerName: "http"}
	if _, stillAttached := out.attachments[listener][client.ObjectKeyFromObject(newer)]; stillAttached {
		t.Fatalf("conflicted route should be removed from listener attachments")
	}
}

func TestEvaluateRouteConflictsKeepsDistinctHostnames(t *testing.T) {
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

func TestEvaluateRouteConflictsCatchAllConflictsWithSpecificHostname(t *testing.T) {
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

func TestEvaluateRouteConflictsIgnoresDistinctListeners(t *testing.T) {
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

func findConditionByType(conditions []conditionSpec, conditionType string) *conditionSpec {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

func t0() time.Time {
	t, _ := time.Parse(time.RFC3339, "2024-01-01T00:00:00Z")
	return t
}
