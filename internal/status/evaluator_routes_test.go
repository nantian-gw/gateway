package status

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestEvaluateRoutePrefersNotAllowedReasonBeforeHostnameMismatch(t *testing.T) {
	gateway := gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "infra"},
		Spec: gatewayv1.GatewaySpec{
			Listeners: []gatewayv1.Listener{{
				Name:     "http",
				Protocol: gatewayv1.HTTPProtocolType,
				Port:     80,
				Hostname: ptr(gatewayv1.Hostname("api.example.com")),
				AllowedRoutes: &gatewayv1.AllowedRoutes{
					Namespaces: &gatewayv1.RouteNamespaces{
						From: ptr(gatewayv1.NamespacesFromSame),
					},
				},
			}},
		},
	}

	state := &clusterState{
		controllerName:  "gateway.networking.k8s.io/nantian-gw",
		managedGateways: []gatewayv1.Gateway{gateway},
		managedGatewayByKey: map[string]gatewayv1.Gateway{
			"infra/gw": gateway,
		},
		namespaceByName: map[string]corev1.Namespace{
			"apps": {ObjectMeta: metav1.ObjectMeta{Name: "apps"}},
		},
	}

	evals := evaluateRoute(state, routeInput{
		kind:       routeKindHTTP,
		namespace:  "apps",
		name:       "route",
		generation: 1,
		hostnames:  []gatewayv1.Hostname{"other.example.com"},
		parentRefs: []gatewayv1.ParentReference{{
			Name:      "gw",
			Namespace: ptr(gatewayv1.Namespace("infra")),
		}},
	})

	if len(evals) != 1 {
		t.Fatalf("expected 1 evaluation, got %d", len(evals))
	}
	if got := evals[0].acceptedCondition.Reason; got != string(gatewayv1.RouteReasonNotAllowedByListeners) {
		t.Fatalf("accepted reason = %s, want %s", got, gatewayv1.RouteReasonNotAllowedByListeners)
	}
}

func TestEvaluateRouteReportsHostnameMismatchAfterAllowedListenerCheck(t *testing.T) {
	gateway := gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "apps"},
		Spec: gatewayv1.GatewaySpec{
			Listeners: []gatewayv1.Listener{{
				Name:     "http",
				Protocol: gatewayv1.HTTPProtocolType,
				Port:     80,
				Hostname: ptr(gatewayv1.Hostname("api.example.com")),
				AllowedRoutes: &gatewayv1.AllowedRoutes{
					Namespaces: &gatewayv1.RouteNamespaces{
						From: ptr(gatewayv1.NamespacesFromSame),
					},
				},
			}},
		},
	}

	state := &clusterState{
		controllerName:  "gateway.networking.k8s.io/nantian-gw",
		managedGateways: []gatewayv1.Gateway{gateway},
		managedGatewayByKey: map[string]gatewayv1.Gateway{
			"apps/gw": gateway,
		},
		namespaceByName: map[string]corev1.Namespace{
			"apps": {ObjectMeta: metav1.ObjectMeta{Name: "apps"}},
		},
	}

	evals := evaluateRoute(state, routeInput{
		kind:       routeKindHTTP,
		namespace:  "apps",
		name:       "route",
		generation: 1,
		hostnames:  []gatewayv1.Hostname{"other.example.com"},
		parentRefs: []gatewayv1.ParentReference{{
			Name: "gw",
		}},
	})

	if len(evals) != 1 {
		t.Fatalf("expected 1 evaluation, got %d", len(evals))
	}
	if got := evals[0].acceptedCondition.Reason; got != string(gatewayv1.RouteReasonNoMatchingListenerHostname) {
		t.Fatalf("accepted reason = %s, want %s", got, gatewayv1.RouteReasonNoMatchingListenerHostname)
	}
}
