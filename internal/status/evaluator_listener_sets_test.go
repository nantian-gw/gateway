package status

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestEvaluateListenerSets(t *testing.T) {
	ns := func(name gatewayv1.Namespace) *gatewayv1.Namespace { return &name }
	from := func(from gatewayv1.FromNamespaces) *gatewayv1.FromNamespaces { return &from }

	t.Run("managed Gateway", func(t *testing.T) {
		state := &clusterState{controllerName: "example.com/gateway"}
		lses := []gatewayv1.ListenerSet{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "my-ls",
					Namespace:  "default",
					Generation: 3,
				},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Namespace: ns("default"),
						Name:      "gw1",
					},
				},
			},
		}
		managedGateways := map[string]gatewayv1.Gateway{
			"default/gw1": {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gw1",
					Namespace: "default",
				},
				Spec: gatewayv1.GatewaySpec{
					AllowedListeners: &gatewayv1.AllowedListeners{
						Namespaces: &gatewayv1.ListenerNamespaces{
							From: from(gatewayv1.NamespacesFromAll),
						},
					},
				},
			},
		}

		result := evaluateListenerSets(state, lses, managedGateways, nil)

		if len(result) != 1 {
			t.Fatalf("expected 1 result, got %d", len(result))
		}
		eval, ok := result["default/my-ls"]
		if !ok {
			t.Fatalf("expected key 'default/my-ls', got %v", result)
		}
		if eval.accepted.Type != string(gatewayv1.ListenerSetConditionAccepted) {
			t.Fatalf("accepted type = %s, want %s", eval.accepted.Type, string(gatewayv1.ListenerSetConditionAccepted))
		}
		if eval.accepted.Status != metav1.ConditionTrue {
			t.Fatalf("accepted status = %s, want True", eval.accepted.Status)
		}
		if eval.accepted.Reason != string(gatewayv1.ListenerSetReasonAccepted) {
			t.Fatalf("accepted reason = %s, want %s", eval.accepted.Reason, string(gatewayv1.ListenerSetReasonAccepted))
		}
		if eval.accepted.Message != "ListenerSet is accepted" {
			t.Fatalf("accepted message = %s, want 'ListenerSet is accepted'", eval.accepted.Message)
		}
		if eval.accepted.ObservedGeneration != 3 {
			t.Fatalf("accepted observedGeneration = %d, want 3", eval.accepted.ObservedGeneration)
		}

		if eval.programmed.Type != string(gatewayv1.ListenerSetConditionProgrammed) {
			t.Fatalf("programmed type = %s, want %s", eval.programmed.Type, string(gatewayv1.ListenerSetConditionProgrammed))
		}
		if eval.programmed.Status != metav1.ConditionTrue {
			t.Fatalf("programmed status = %s, want True", eval.programmed.Status)
		}
		if eval.programmed.Reason != string(gatewayv1.ListenerSetReasonProgrammed) {
			t.Fatalf("programmed reason = %s, want %s", eval.programmed.Reason, string(gatewayv1.ListenerSetReasonProgrammed))
		}
		if eval.programmed.Message != "ListenerSet listeners are programmed" {
			t.Fatalf("programmed message = %s, want 'ListenerSet listeners are programmed'", eval.programmed.Message)
		}
		if eval.programmed.ObservedGeneration != 3 {
			t.Fatalf("programmed observedGeneration = %d, want 3", eval.programmed.ObservedGeneration)
		}
	})

	t.Run("managed Gateway defaults omitted ParentRef namespace to ListenerSet namespace", func(t *testing.T) {
		state := &clusterState{controllerName: "example.com/gateway"}
		lses := []gatewayv1.ListenerSet{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "my-ls",
					Namespace:  "default",
					Generation: 3,
				},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Name: "gw1",
					},
				},
			},
		}
		managedGateways := map[string]gatewayv1.Gateway{
			"default/gw1": {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gw1",
					Namespace: "default",
				},
				Spec: gatewayv1.GatewaySpec{
					AllowedListeners: &gatewayv1.AllowedListeners{
						Namespaces: &gatewayv1.ListenerNamespaces{
							From: from(gatewayv1.NamespacesFromAll),
						},
					},
				},
			},
		}

		result := evaluateListenerSets(state, lses, managedGateways, nil)

		if len(result) != 1 {
			t.Fatalf("expected 1 result, got %d", len(result))
		}
		eval, ok := result["default/my-ls"]
		if !ok {
			t.Fatalf("expected key 'default/my-ls', got %v", result)
		}
		if eval.accepted.Status != metav1.ConditionTrue {
			t.Fatalf("accepted status = %s, want True; reason=%s message=%s", eval.accepted.Status, eval.accepted.Reason, eval.accepted.Message)
		}
	})

	t.Run("managed Gateway with namespace in ParentRef", func(t *testing.T) {
		state := &clusterState{controllerName: "example.com/gateway"}
		lses := []gatewayv1.ListenerSet{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "cross-ls",
					Namespace:  "app-ns",
					Generation: 1,
				},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Name:      "gw-cross",
						Namespace: ns("other-ns"),
					},
				},
			},
		}
		managedGateways := map[string]gatewayv1.Gateway{
			"other-ns/gw-cross": {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gw-cross",
					Namespace: "other-ns",
				},
				Spec: gatewayv1.GatewaySpec{
					AllowedListeners: &gatewayv1.AllowedListeners{
						Namespaces: &gatewayv1.ListenerNamespaces{
							From: from(gatewayv1.NamespacesFromAll),
						},
					},
				},
			},
		}

		result := evaluateListenerSets(state, lses, managedGateways, nil)

		if len(result) != 1 {
			t.Fatalf("expected 1 result, got %d", len(result))
		}
		eval, ok := result["app-ns/cross-ls"]
		if !ok {
			t.Fatalf("expected key 'app-ns/cross-ls', got %v", result)
		}
		if eval.accepted.Type != string(gatewayv1.ListenerSetConditionAccepted) {
			t.Fatalf("accepted type = %s, want %s", eval.accepted.Type, string(gatewayv1.ListenerSetConditionAccepted))
		}
		if eval.accepted.Status != metav1.ConditionTrue {
			t.Fatalf("accepted status = %s, want True", eval.accepted.Status)
		}
	})

	t.Run("unmanaged Gateway", func(t *testing.T) {
		state := &clusterState{controllerName: "example.com/gateway"}
		lses := []gatewayv1.ListenerSet{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "unmanaged-ls",
					Namespace: "default",
				},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Namespace: ns("default"),
						Name:      "unmanaged-gw",
					},
				},
			},
		}
		managedGateways := map[string]gatewayv1.Gateway{
			"default/gw1": {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gw1",
					Namespace: "default",
				},
			},
		}

		result := evaluateListenerSets(state, lses, managedGateways, nil)

		if len(result) != 0 {
			t.Fatalf("expected 0 results for unmanaged gateway, got %d", len(result))
		}
	})

	t.Run("no managed gateways", func(t *testing.T) {
		state := &clusterState{controllerName: "example.com/gateway"}
		lses := []gatewayv1.ListenerSet{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "ls1", Namespace: "default"},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Name:      "gw1",
						Namespace: ns("default"),
					},
				},
			},
		}

		result := evaluateListenerSets(state, lses, nil, nil)

		if len(result) != 0 {
			t.Fatalf("expected 0 results with nil managed gateways, got %d", len(result))
		}

		result = evaluateListenerSets(state, lses, map[string]gatewayv1.Gateway{}, nil)

		if len(result) != 0 {
			t.Fatalf("expected 0 results with empty managed gateways, got %d", len(result))
		}
	})

	t.Run("empty ListenerSets", func(t *testing.T) {
		state := &clusterState{controllerName: "example.com/gateway"}
		managedGateways := map[string]gatewayv1.Gateway{
			"default/gw1": {},
		}

		result := evaluateListenerSets(state, nil, managedGateways, nil)
		if len(result) != 0 {
			t.Fatalf("expected 0 results with nil ListenerSets, got %d", len(result))
		}

		result = evaluateListenerSets(state, []gatewayv1.ListenerSet{}, managedGateways, nil)
		if len(result) != 0 {
			t.Fatalf("expected 0 results with empty ListenerSets, got %d", len(result))
		}
	})

	t.Run("multiple ListenerSets for same Gateway", func(t *testing.T) {
		state := &clusterState{controllerName: "example.com/gateway"}
		lses := []gatewayv1.ListenerSet{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "ls-http",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Name:      "gw1",
						Namespace: ns("default"),
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "ls-https",
					Namespace:  "default",
					Generation: 5,
				},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Name:      "gw1",
						Namespace: ns("default"),
					},
				},
			},
		}
		managedGateways := map[string]gatewayv1.Gateway{
			"default/gw1": {
				ObjectMeta: metav1.ObjectMeta{Name: "gw1", Namespace: "default"},
				Spec: gatewayv1.GatewaySpec{
					AllowedListeners: &gatewayv1.AllowedListeners{
						Namespaces: &gatewayv1.ListenerNamespaces{
							From: from(gatewayv1.NamespacesFromAll),
						},
					},
				},
			},
		}

		result := evaluateListenerSets(state, lses, managedGateways, nil)

		if len(result) != 2 {
			t.Fatalf("expected 2 results, got %d", len(result))
		}

		eval1, ok := result["default/ls-http"]
		if !ok {
			t.Fatalf("expected key 'default/ls-http', got %v", result)
		}
		if eval1.accepted.ObservedGeneration != 1 {
			t.Fatalf("ls-http observedGeneration = %d, want 1", eval1.accepted.ObservedGeneration)
		}

		eval2, ok := result["default/ls-https"]
		if !ok {
			t.Fatalf("expected key 'default/ls-https', got %v", result)
		}
		if eval2.accepted.ObservedGeneration != 5 {
			t.Fatalf("ls-https observedGeneration = %d, want 5", eval2.accepted.ObservedGeneration)
		}
	})

	t.Run("protocol conflict leaves ListenerSet without valid listeners", func(t *testing.T) {
		state := &clusterState{
			controllerName: "example.com/gateway",
			namespaceByName: map[string]corev1.Namespace{
				"default": {ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			},
		}
		lses := []gatewayv1.ListenerSet{{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "tcp-ls",
				Namespace:  "default",
				Generation: 2,
			},
			Spec: gatewayv1.ListenerSetSpec{
				ParentRef: gatewayv1.ParentGatewayReference{
					Name:      "gw1",
					Namespace: ns("default"),
				},
				Listeners: []gatewayv1.ListenerEntry{{
					Name:     "tcp",
					Port:     80,
					Protocol: gatewayv1.TCPProtocolType,
				}},
			},
		}}
		managedGateways := map[string]gatewayv1.Gateway{
			"default/gw1": {
				ObjectMeta: metav1.ObjectMeta{Name: "gw1", Namespace: "default"},
				Spec: gatewayv1.GatewaySpec{
					AllowedListeners: &gatewayv1.AllowedListeners{
						Namespaces: &gatewayv1.ListenerNamespaces{
							From: from(gatewayv1.NamespacesFromAll),
						},
					},
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Port:     80,
						Protocol: gatewayv1.HTTPProtocolType,
					}},
				},
			},
		}

		result := evaluateListenerSets(state, lses, managedGateways, nil)
		eval := result["default/tcp-ls"]

		assertConditionSpec(t, eval.accepted, string(gatewayv1.ListenerSetConditionAccepted), metav1.ConditionFalse, string(gatewayv1.ListenerSetReasonListenersNotValid), 2)
		assertConditionSpec(t, eval.programmed, string(gatewayv1.ListenerSetConditionProgrammed), metav1.ConditionFalse, string(gatewayv1.ListenerSetReasonListenersNotValid), 2)
		listener := listenerEntryStatusByName(t, eval.listeners, "tcp")
		assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionAccepted), metav1.ConditionFalse, string(gatewayv1.ListenerReasonProtocolConflict), 2)
		assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionProgrammed), metav1.ConditionFalse, string(gatewayv1.ListenerReasonProtocolConflict), 2)
		assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionConflicted), metav1.ConditionTrue, string(gatewayv1.ListenerReasonProtocolConflict), 2)
	})

	t.Run("same timestamp ListenerSets use namespace and name tiebreaker", func(t *testing.T) {
		state := &clusterState{
			controllerName: "example.com/gateway",
			namespaceByName: map[string]corev1.Namespace{
				"default": {ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			},
		}
		sameTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		hostname := gatewayv1.Hostname("shared.example.com")
		lses := []gatewayv1.ListenerSet{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "b-listener-set",
					Namespace:         "default",
					Generation:        1,
					CreationTimestamp: metav1.NewTime(sameTime),
				},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Name:      "gw1",
						Namespace: ns("default"),
					},
					Listeners: []gatewayv1.ListenerEntry{{
						Name:     "http-b",
						Port:     80,
						Protocol: gatewayv1.HTTPProtocolType,
						Hostname: &hostname,
					}},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "a-listener-set",
					Namespace:         "default",
					Generation:        1,
					CreationTimestamp: metav1.NewTime(sameTime),
				},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Name:      "gw1",
						Namespace: ns("default"),
					},
					Listeners: []gatewayv1.ListenerEntry{{
						Name:     "http-a",
						Port:     80,
						Protocol: gatewayv1.HTTPProtocolType,
						Hostname: &hostname,
					}},
				},
			},
		}
		managedGateways := map[string]gatewayv1.Gateway{
			"default/gw1": {
				ObjectMeta: metav1.ObjectMeta{Name: "gw1", Namespace: "default"},
				Spec: gatewayv1.GatewaySpec{
					AllowedListeners: &gatewayv1.AllowedListeners{
						Namespaces: &gatewayv1.ListenerNamespaces{
							From: from(gatewayv1.NamespacesFromAll),
						},
					},
				},
			},
		}

		result := evaluateListenerSets(state, lses, managedGateways, nil)

		first := result["default/a-listener-set"]
		assertConditionSpec(t, first.accepted, string(gatewayv1.ListenerSetConditionAccepted), metav1.ConditionTrue, string(gatewayv1.ListenerSetReasonAccepted), 1)
		assertCondition(t, listenerEntryStatusByName(t, first.listeners, "http-a").Conditions, string(gatewayv1.ListenerConditionAccepted), metav1.ConditionTrue, string(gatewayv1.ListenerReasonAccepted), 1)

		second := result["default/b-listener-set"]
		assertConditionSpec(t, second.accepted, string(gatewayv1.ListenerSetConditionAccepted), metav1.ConditionFalse, string(gatewayv1.ListenerSetReasonListenersNotValid), 1)
		assertCondition(t, listenerEntryStatusByName(t, second.listeners, "http-b").Conditions, string(gatewayv1.ListenerConditionAccepted), metav1.ConditionFalse, string(gatewayv1.ListenerReasonHostnameConflict), 1)
	})

	t.Run("cross namespace certificate ref without ReferenceGrant leaves ListenerSet without valid listeners", func(t *testing.T) {
		state := &clusterState{
			controllerName: "example.com/gateway",
			namespaceByName: map[string]corev1.Namespace{
				"default": {ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			},
		}
		secretNamespace := gatewayv1.Namespace("shared-certs")
		lses := []gatewayv1.ListenerSet{{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "https-ls",
				Namespace:  "default",
				Generation: 3,
			},
			Spec: gatewayv1.ListenerSetSpec{
				ParentRef: gatewayv1.ParentGatewayReference{
					Name:      "gw1",
					Namespace: ns("default"),
				},
				Listeners: []gatewayv1.ListenerEntry{{
					Name:     "https",
					Port:     443,
					Protocol: gatewayv1.HTTPSProtocolType,
					TLS: &gatewayv1.ListenerTLSConfig{
						CertificateRefs: []gatewayv1.SecretObjectReference{{
							Name:      "server-cert",
							Namespace: &secretNamespace,
						}},
					},
				}},
			},
		}}
		managedGateways := map[string]gatewayv1.Gateway{
			"default/gw1": {
				ObjectMeta: metav1.ObjectMeta{Name: "gw1", Namespace: "default"},
				Spec: gatewayv1.GatewaySpec{
					AllowedListeners: &gatewayv1.AllowedListeners{
						Namespaces: &gatewayv1.ListenerNamespaces{
							From: from(gatewayv1.NamespacesFromAll),
						},
					},
				},
			},
		}

		result := evaluateListenerSets(state, lses, managedGateways, nil)
		eval := result["default/https-ls"]

		assertConditionSpec(t, eval.accepted, string(gatewayv1.ListenerSetConditionAccepted), metav1.ConditionFalse, string(gatewayv1.ListenerSetReasonListenersNotValid), 3)
		assertConditionSpec(t, eval.programmed, string(gatewayv1.ListenerSetConditionProgrammed), metav1.ConditionFalse, string(gatewayv1.ListenerSetReasonListenersNotValid), 3)
		listener := listenerEntryStatusByName(t, eval.listeners, "https")
		assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionResolvedRefs), metav1.ConditionFalse, string(gatewayv1.ListenerReasonRefNotPermitted), 3)
		assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionAccepted), metav1.ConditionFalse, string(gatewayv1.ListenerReasonRefNotPermitted), 3)
		assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionProgrammed), metav1.ConditionFalse, string(gatewayv1.ListenerReasonRefNotPermitted), 3)
	})

	t.Run("mixed managed and unmanaged", func(t *testing.T) {
		state := &clusterState{controllerName: "example.com/gateway"}
		lses := []gatewayv1.ListenerSet{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "managed-ls",
					Namespace: "default",
				},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Name:      "gw1",
						Namespace: ns("default"),
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "unmanaged-ls",
					Namespace: "default",
				},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Name:      "gw2",
						Namespace: ns("default"),
					},
				},
			},
		}
		managedGateways := map[string]gatewayv1.Gateway{
			"default/gw1": {
				ObjectMeta: metav1.ObjectMeta{Name: "gw1", Namespace: "default"},
				Spec: gatewayv1.GatewaySpec{
					AllowedListeners: &gatewayv1.AllowedListeners{
						Namespaces: &gatewayv1.ListenerNamespaces{
							From: from(gatewayv1.NamespacesFromAll),
						},
					},
				},
			},
		}

		result := evaluateListenerSets(state, lses, managedGateways, nil)

		if len(result) != 1 {
			t.Fatalf("expected 1 result, got %d", len(result))
		}
		if _, ok := result["default/managed-ls"]; !ok {
			t.Fatalf("expected 'default/managed-ls' in result, got %v", result)
		}
		if _, ok := result["default/unmanaged-ls"]; ok {
			t.Fatal("unmanaged-ls should not be in result")
		}
	})
}

func TestReconcileSetsListenerSetAttachedRoutes(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}, &gatewayv1.Gateway{}, &gatewayv1.HTTPRoute{}, &gatewayv1.ListenerSet{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec:       gatewayv1.GatewayClassSpec{ControllerName: controllerName},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					AllowedListeners: &gatewayv1.AllowedListeners{
						Namespaces: &gatewayv1.ListenerNamespaces{
							From: ptr(gatewayv1.NamespacesFromAll),
						},
					},
					Listeners: []gatewayv1.Listener{{
						Name:     "gateway-listener",
						Port:     80,
						Protocol: gatewayv1.HTTPProtocolType,
					}},
				},
			},
			&gatewayv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "default", Generation: 1},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Name:      "gw",
						Namespace: ptr(gatewayv1.Namespace("default")),
					},
					Listeners: []gatewayv1.ListenerEntry{{
						Name:     "ls-listener",
						Port:     80,
						Protocol: gatewayv1.HTTPProtocolType,
						Hostname: ptr(gatewayv1.Hostname("ls.example.com")),
					}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "ls-route", Namespace: "default", Generation: 1},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Group: ptr(gatewayv1.Group(gatewayv1.GroupName)),
							Kind:  ptr(gatewayv1.Kind("ListenerSet")),
							Name:  "ls",
						}},
					},
					Hostnames: []gatewayv1.Hostname{"ls.example.com"},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "backend",
									Port: ptr(gatewayv1.PortNumber(8080)),
								},
							},
						}},
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "backend", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8080}},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var listenerSet gatewayv1.ListenerSet
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "ls"}, &listenerSet); err != nil {
		t.Fatalf("Get ListenerSet returned error: %v", err)
	}
	listener := listenerEntryStatusByName(t, listenerSet.Status.Listeners, "ls-listener")
	if listener.AttachedRoutes != 1 {
		t.Fatalf("expected ListenerSet listener attachedRoutes=1, got %d", listener.AttachedRoutes)
	}
}

func TestReconcileAllowsListenerSetFromSelectedNamespace(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}, &gatewayv1.Gateway{}, &gatewayv1.ListenerSet{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Name:   "gateway-api-listenerset-selector-allowed-ns",
				Labels: map[string]string{"allowed": "ns"},
			}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec:       gatewayv1.GatewayClassSpec{ControllerName: controllerName},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gateway-allows-listenerset-in-selected-namespace", Namespace: "gateway-conformance-infra", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "gateway-listener",
						Port:     80,
						Protocol: gatewayv1.HTTPProtocolType,
						Hostname: ptr(gatewayv1.Hostname("gateway-listener.com")),
						AllowedRoutes: &gatewayv1.AllowedRoutes{
							Namespaces: &gatewayv1.RouteNamespaces{From: ptr(gatewayv1.NamespacesFromAll)},
						},
					}},
					AllowedListeners: &gatewayv1.AllowedListeners{
						Namespaces: &gatewayv1.ListenerNamespaces{
							From: ptr(gatewayv1.NamespacesFromSelector),
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"allowed": "ns"},
							},
						},
					},
				},
			},
			&gatewayv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{Name: "listenerset-in-selected-namespace", Namespace: "gateway-api-listenerset-selector-allowed-ns", Generation: 1},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Name:      "gateway-allows-listenerset-in-selected-namespace",
						Namespace: ptr(gatewayv1.Namespace("gateway-conformance-infra")),
					},
					Listeners: []gatewayv1.ListenerEntry{{
						Name:     "listenerset-in-selected-namespace-listener",
						Port:     80,
						Protocol: gatewayv1.HTTPProtocolType,
						Hostname: ptr(gatewayv1.Hostname("listenerset-in-selected-namespace-listener.com")),
						AllowedRoutes: &gatewayv1.AllowedRoutes{
							Namespaces: &gatewayv1.RouteNamespaces{From: ptr(gatewayv1.NamespacesFromAll)},
						},
					}},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	for i := 0; i < 2; i++ {
		if err := reconciler.Reconcile(context.Background()); err != nil {
			t.Fatalf("Reconcile #%d returned error: %v", i+1, err)
		}
	}

	var listenerSet gatewayv1.ListenerSet
	if err := k8sClient.Get(context.Background(), client.ObjectKey{
		Namespace: "gateway-api-listenerset-selector-allowed-ns",
		Name:      "listenerset-in-selected-namespace",
	}, &listenerSet); err != nil {
		t.Fatalf("Get ListenerSet returned error: %v", err)
	}
	assertCondition(t, listenerSet.Status.Conditions, string(gatewayv1.ListenerSetConditionAccepted), metav1.ConditionTrue, string(gatewayv1.ListenerSetReasonAccepted), 1)
	listener := listenerEntryStatusByName(t, listenerSet.Status.Listeners, "listenerset-in-selected-namespace-listener")
	assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionAccepted), metav1.ConditionTrue, string(gatewayv1.ListenerReasonAccepted), 1)
	assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionProgrammed), metav1.ConditionTrue, string(gatewayv1.ListenerReasonProgrammed), 1)

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{
		Namespace: "gateway-conformance-infra",
		Name:      "gateway-allows-listenerset-in-selected-namespace",
	}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	if gateway.Status.AttachedListenerSets == nil || *gateway.Status.AttachedListenerSets != 1 {
		t.Fatalf("AttachedListenerSets = %#v, want 1", gateway.Status.AttachedListenerSets)
	}
	if len(gateway.Status.Listeners) != 2 {
		t.Fatalf("Gateway listeners = %d, want 2: %#v", len(gateway.Status.Listeners), gateway.Status.Listeners)
	}
}

func TestEvaluateListenerSetAttachedRoutesForAllDerivedListeners(t *testing.T) {
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	parentGroup := gatewayv1.Group(gatewayv1.GroupName)
	parentKind := gatewayv1.Kind("ListenerSet")
	listenerOne := gatewayv1.SectionName("listener-one")
	hostOne := gatewayv1.Hostname("one.example.com")
	hostTwo := gatewayv1.Hostname("two.example.com")

	state := &clusterState{
		controllerName: string(controllerName),
		gatewayClasses: []gatewayv1.GatewayClass{{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
			Spec:       gatewayv1.GatewayClassSpec{ControllerName: controllerName},
		}},
		gateways: []gatewayv1.Gateway{{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
			Spec: gatewayv1.GatewaySpec{
				GatewayClassName: "nantian-gw",
				AllowedListeners: &gatewayv1.AllowedListeners{
					Namespaces: &gatewayv1.ListenerNamespaces{
						From: ptr(gatewayv1.NamespacesFromAll),
					},
				},
			},
		}},
		listenerSets: []gatewayv1.ListenerSet{{
			ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "default", Generation: 1},
			Spec: gatewayv1.ListenerSetSpec{
				ParentRef: gatewayv1.ParentGatewayReference{
					Name:      "gw",
					Namespace: ptr(gatewayv1.Namespace("default")),
				},
				Listeners: []gatewayv1.ListenerEntry{
					{
						Name:     listenerOne,
						Port:     80,
						Protocol: gatewayv1.HTTPProtocolType,
						Hostname: &hostOne,
						AllowedRoutes: &gatewayv1.AllowedRoutes{
							Namespaces: &gatewayv1.RouteNamespaces{
								From: ptr(gatewayv1.NamespacesFromAll),
							},
						},
					},
					{
						Name:     "listener-two",
						Port:     80,
						Protocol: gatewayv1.HTTPProtocolType,
						Hostname: &hostTwo,
						AllowedRoutes: &gatewayv1.AllowedRoutes{
							Namespaces: &gatewayv1.RouteNamespaces{
								From: ptr(gatewayv1.NamespacesFromAll),
							},
						},
					},
				},
			},
		}},
		httpRoutes: []gatewayv1.HTTPRoute{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "all-listeners-route", Namespace: "default", Generation: 1},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Group: &parentGroup,
							Kind:  &parentKind,
							Name:  "ls",
						}},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "listener-set-route", Namespace: "default", Generation: 1},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Group: &parentGroup,
							Kind:  &parentKind,
							Name:  "ls",
						}},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "section-route", Namespace: "default", Generation: 1},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Group:       &parentGroup,
							Kind:        &parentKind,
							Name:        "ls",
							SectionName: &listenerOne,
						}},
					},
				},
			},
		},
		namespaces: []corev1.Namespace{
			{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		},
	}
	state.index()

	routeState := evaluateRoutes(state)
	evals := evaluateListenerSets(state, state.listenerSets, state.managedGatewayByKey, routeState.attachments)
	eval := evals["default/ls"]

	listenerOneStatus := listenerEntryStatusByName(t, eval.listeners, "listener-one")
	if listenerOneStatus.AttachedRoutes != 3 {
		t.Fatalf("expected listener-one attachedRoutes=3, got %d", listenerOneStatus.AttachedRoutes)
	}
	listenerTwoStatus := listenerEntryStatusByName(t, eval.listeners, "listener-two")
	if listenerTwoStatus.AttachedRoutes != 2 {
		t.Fatalf("expected listener-two attachedRoutes=2, got %d", listenerTwoStatus.AttachedRoutes)
	}
}

func TestEvaluateListenerSetAttachedRoutesDefaultsParentGatewayNamespace(t *testing.T) {
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	parentGroup := gatewayv1.Group(gatewayv1.GroupName)
	parentKind := gatewayv1.Kind("ListenerSet")

	state := &clusterState{
		controllerName: string(controllerName),
		gatewayClasses: []gatewayv1.GatewayClass{{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
			Spec:       gatewayv1.GatewayClassSpec{ControllerName: controllerName},
		}},
		gateways: []gatewayv1.Gateway{{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
			Spec: gatewayv1.GatewaySpec{
				GatewayClassName: "nantian-gw",
				AllowedListeners: &gatewayv1.AllowedListeners{
					Namespaces: &gatewayv1.ListenerNamespaces{
						From: ptr(gatewayv1.NamespacesFromAll),
					},
				},
			},
		}},
		listenerSets: []gatewayv1.ListenerSet{{
			ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "default", Generation: 1},
			Spec: gatewayv1.ListenerSetSpec{
				ParentRef: gatewayv1.ParentGatewayReference{Name: "gw"},
				Listeners: []gatewayv1.ListenerEntry{{
					Name:     "ls-listener",
					Port:     80,
					Protocol: gatewayv1.HTTPProtocolType,
					AllowedRoutes: &gatewayv1.AllowedRoutes{
						Namespaces: &gatewayv1.RouteNamespaces{
							From: ptr(gatewayv1.NamespacesFromAll),
						},
					},
				}},
			},
		}},
		httpRoutes: []gatewayv1.HTTPRoute{{
			ObjectMeta: metav1.ObjectMeta{Name: "listener-set-route", Namespace: "default", Generation: 1},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{
						Group: &parentGroup,
						Kind:  &parentKind,
						Name:  "ls",
					}},
				},
			},
		}},
		namespaces: []corev1.Namespace{
			{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		},
	}
	state.index()

	routeState := evaluateRoutes(state)
	evals := evaluateListenerSets(state, state.listenerSets, state.managedGatewayByKey, routeState.attachments)
	eval, ok := evals["default/ls"]
	if !ok {
		t.Fatalf("expected ListenerSet evaluation for default/ls, got %#v", evals)
	}

	listenerStatus := listenerEntryStatusByName(t, eval.listeners, "ls-listener")
	if listenerStatus.AttachedRoutes != 1 {
		t.Fatalf("expected ls-listener attachedRoutes=1, got %d", listenerStatus.AttachedRoutes)
	}
}

func TestEvaluateListenerSetAllowedRoutesSameUsesListenerSetNamespace(t *testing.T) {
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	parentGroup := gatewayv1.Group(gatewayv1.GroupName)
	parentKind := gatewayv1.Kind("ListenerSet")

	state := &clusterState{
		controllerName: string(controllerName),
		gatewayClasses: []gatewayv1.GatewayClass{{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
			Spec:       gatewayv1.GatewayClassSpec{ControllerName: controllerName},
		}},
		gateways: []gatewayv1.Gateway{{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "infra", Generation: 1},
			Spec: gatewayv1.GatewaySpec{
				GatewayClassName: "nantian-gw",
				AllowedListeners: &gatewayv1.AllowedListeners{
					Namespaces: &gatewayv1.ListenerNamespaces{
						From: ptr(gatewayv1.NamespacesFromAll),
					},
				},
			},
		}},
		listenerSets: []gatewayv1.ListenerSet{{
			ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "apps", Generation: 1},
			Spec: gatewayv1.ListenerSetSpec{
				ParentRef: gatewayv1.ParentGatewayReference{
					Name:      "gw",
					Namespace: ptr(gatewayv1.Namespace("infra")),
				},
				Listeners: []gatewayv1.ListenerEntry{{
					Name:     "http",
					Port:     80,
					Protocol: gatewayv1.HTTPProtocolType,
					AllowedRoutes: &gatewayv1.AllowedRoutes{
						Namespaces: &gatewayv1.RouteNamespaces{
							From: ptr(gatewayv1.NamespacesFromSame),
						},
					},
				}},
			},
		}},
		httpRoutes: []gatewayv1.HTTPRoute{{
			ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "apps", Generation: 1},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{
						Group:     &parentGroup,
						Kind:      &parentKind,
						Name:      "ls",
						Namespace: ptr(gatewayv1.Namespace("apps")),
					}},
				},
			},
		}},
		namespaces: []corev1.Namespace{
			{ObjectMeta: metav1.ObjectMeta{Name: "apps"}},
		},
	}
	state.index()

	routeState := evaluateRoutes(state)
	evals := routeState.http[client.ObjectKey{Namespace: "apps", Name: "route"}]
	if len(evals) != 1 {
		t.Fatalf("expected 1 route parent evaluation, got %d", len(evals))
	}
	if evals[0].acceptedCondition.Status != metav1.ConditionTrue {
		t.Fatalf("Accepted = %s reason=%s message=%s, want True", evals[0].acceptedCondition.Status, evals[0].acceptedCondition.Reason, evals[0].acceptedCondition.Message)
	}

	listenerKey := listenerKey{gatewayNamespace: "infra", gatewayName: "gw", listenerName: "apps/ls/http"}
	if got := len(routeState.attachments[listenerKey]); got != 1 {
		t.Fatalf("attached routes for ListenerSet listener = %d, want 1: %#v", got, routeState.attachments)
	}
}

func TestEvaluateGatewayListenerSetListeners(t *testing.T) {
	ns := func(name gatewayv1.Namespace) *gatewayv1.Namespace { return &name }
	from := func(from gatewayv1.FromNamespaces) *gatewayv1.FromNamespaces { return &from }

	defaultGateway := gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gw1", Namespace: "default", Generation: 1,
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "test-class",
			AllowedListeners: &gatewayv1.AllowedListeners{
				Namespaces: &gatewayv1.ListenerNamespaces{
					From: from(gatewayv1.NamespacesFromAll),
				},
			},
		},
	}
	defaultState := &clusterState{
		controllerName: "example.com/gateway",
		namespaceByName: map[string]corev1.Namespace{
			"default": {ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			"other":   {ObjectMeta: metav1.ObjectMeta{Name: "other"}},
		},
	}

	httpProtocol := gatewayv1.HTTPProtocolType
	hostname := func(h gatewayv1.Hostname) *gatewayv1.Hostname { return &h }

	t.Run("no listenerSets returns empty", func(t *testing.T) {
		result := evaluateGatewayListenerSetListeners(defaultState, defaultGateway, nil, nil)
		if len(result) != 0 {
			t.Fatalf("expected 0 evaluations, got %d", len(result))
		}
	})

	t.Run("listenerSet with one entry produces one evaluation", func(t *testing.T) {
		state := &clusterState{
			controllerName:  "example.com/gateway",
			namespaceByName: defaultState.namespaceByName,
		}
		lses := []gatewayv1.ListenerSet{{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "my-ls",
				Namespace:         "default",
				Generation:        1,
				CreationTimestamp: metav1.Now(),
			},
			Spec: gatewayv1.ListenerSetSpec{
				ParentRef: gatewayv1.ParentGatewayReference{
					Namespace: ns("default"), Name: "gw1",
				},
				Listeners: []gatewayv1.ListenerEntry{{
					Name:     "http",
					Port:     80,
					Protocol: httpProtocol,
				}},
			},
		}}
		lses[0].Status.Conditions = []metav1.Condition{{
			Type:   string(gatewayv1.ListenerSetConditionAccepted),
			Status: metav1.ConditionTrue,
			Reason: string(gatewayv1.ListenerSetReasonAccepted),
		}}

		result := evaluateGatewayListenerSetListeners(state, defaultGateway, lses, nil)
		if len(result) != 1 {
			t.Fatalf("expected 1 evaluation, got %d", len(result))
		}
		eval := result[0]
		expectedName := gatewayv1.SectionName("default/my-ls/http")
		if eval.name != expectedName {
			t.Fatalf("eval name = %s, want %s", eval.name, expectedName)
		}
		if eval.acceptedCondition.Status != metav1.ConditionTrue {
			t.Fatalf("accepted = %s, want True. reason=%s msg=%s", eval.acceptedCondition.Status, eval.acceptedCondition.Reason, eval.acceptedCondition.Message)
		}
	})

	t.Run("listenerSet not accepted does not contribute listeners", func(t *testing.T) {
		state := &clusterState{
			controllerName:  "example.com/gateway",
			namespaceByName: defaultState.namespaceByName,
		}
		lses := []gatewayv1.ListenerSet{{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "my-ls",
				Namespace:         "default",
				Generation:        1,
				CreationTimestamp: metav1.Now(),
			},
			Spec: gatewayv1.ListenerSetSpec{
				ParentRef: gatewayv1.ParentGatewayReference{
					Namespace: ns("default"), Name: "gw1",
				},
				Listeners: []gatewayv1.ListenerEntry{{
					Name: "http", Port: 80, Protocol: httpProtocol,
				}},
			},
		}}

		result := evaluateGatewayListenerSetListeners(state, defaultGateway, lses, nil)
		if len(result) != 0 {
			t.Fatalf("expected 0 evaluations (listenerset not accepted), got %d", len(result))
		}
	})

	t.Run("listenerSet denied by namespace policy excluded", func(t *testing.T) {
		state := &clusterState{
			controllerName:  "example.com/gateway",
			namespaceByName: defaultState.namespaceByName,
		}
		gw := defaultGateway.DeepCopy()
		gw.Spec.AllowedListeners.Namespaces.From = from(gatewayv1.NamespacesFromSame)
		lses := []gatewayv1.ListenerSet{{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "my-ls",
				Namespace:         "other",
				Generation:        1,
				CreationTimestamp: metav1.Now(),
			},
			Spec: gatewayv1.ListenerSetSpec{
				ParentRef: gatewayv1.ParentGatewayReference{
					Namespace: ns("other"), Name: "gw1",
				},
				Listeners: []gatewayv1.ListenerEntry{{
					Name: "http", Port: 80, Protocol: httpProtocol,
				}},
			},
		}}
		lses[0].Status.Conditions = []metav1.Condition{{
			Type: string(gatewayv1.ListenerSetConditionAccepted), Status: metav1.ConditionTrue,
			Reason: string(gatewayv1.ListenerSetReasonAccepted),
		}}

		result := evaluateGatewayListenerSetListeners(state, *gw, lses, nil)
		if len(result) != 0 {
			t.Fatalf("expected 0 evaluations (namespace policy denied), got %d", len(result))
		}
	})

	t.Run("gateway spec listener wins over ListenerSet entry with same port/protocol/hostname", func(t *testing.T) {
		state := &clusterState{
			controllerName:  "example.com/gateway",
			namespaceByName: defaultState.namespaceByName,
		}
		gw := defaultGateway.DeepCopy()
		gw.Spec.Listeners = []gatewayv1.Listener{{
			Name:     "gw-http",
			Port:     80,
			Protocol: httpProtocol,
		}}
		lses := []gatewayv1.ListenerSet{{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "my-ls",
				Namespace:         "default",
				Generation:        1,
				CreationTimestamp: metav1.Now(),
			},
			Spec: gatewayv1.ListenerSetSpec{
				ParentRef: gatewayv1.ParentGatewayReference{
					Namespace: ns("default"), Name: "gw1",
				},
				Listeners: []gatewayv1.ListenerEntry{{
					Name: "http", Port: 80, Protocol: httpProtocol,
				}},
			},
		}}
		lses[0].Status.Conditions = []metav1.Condition{{
			Type: string(gatewayv1.ListenerSetConditionAccepted), Status: metav1.ConditionTrue,
			Reason: string(gatewayv1.ListenerSetReasonAccepted),
		}}

		result := evaluateGatewayListenerSetListeners(state, *gw, lses, nil)
		if len(result) != 0 {
			t.Fatalf("expected 0 evaluations (gateway spec listener conflicts), got %d", len(result))
		}
	})

	t.Run("two ListenerSets with conflicting entries: earlier wins", func(t *testing.T) {
		state := &clusterState{
			controllerName:  "example.com/gateway",
			namespaceByName: defaultState.namespaceByName,
		}
		earlier := metav1.Now()
		later := metav1.NewTime(earlier.Add(1 * time.Second))
		lses := []gatewayv1.ListenerSet{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "first-ls",
					Namespace:         "default",
					Generation:        1,
					CreationTimestamp: earlier,
				},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Namespace: ns("default"), Name: "gw1",
					},
					Listeners: []gatewayv1.ListenerEntry{{
						Name: "http-80", Port: 80, Protocol: httpProtocol,
					}},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "second-ls",
					Namespace:         "default",
					Generation:        1,
					CreationTimestamp: later,
				},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Namespace: ns("default"), Name: "gw1",
					},
					Listeners: []gatewayv1.ListenerEntry{{
						Name: "http-80-dup", Port: 80, Protocol: httpProtocol,
					}},
				},
			},
		}
		for i := range lses {
			lses[i].Status.Conditions = []metav1.Condition{{
				Type: string(gatewayv1.ListenerSetConditionAccepted), Status: metav1.ConditionTrue,
				Reason: string(gatewayv1.ListenerSetReasonAccepted),
			}}
		}

		result := evaluateGatewayListenerSetListeners(state, defaultGateway, lses, nil)
		if len(result) != 1 {
			t.Fatalf("expected 1 evaluation (earlier wins), got %d", len(result))
		}
		if result[0].name != "default/first-ls/http-80" {
			t.Fatalf("expected first-ls entry, got %s", result[0].name)
		}
	})

	t.Run("two ListenerSets with non-conflicting entries: both included", func(t *testing.T) {
		state := &clusterState{
			controllerName:  "example.com/gateway",
			namespaceByName: defaultState.namespaceByName,
		}
		lses := []gatewayv1.ListenerSet{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "ls-http",
					Namespace:         "default",
					Generation:        1,
					CreationTimestamp: metav1.Now(),
				},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Namespace: ns("default"), Name: "gw1",
					},
					Listeners: []gatewayv1.ListenerEntry{{
						Name: "http", Port: 80, Protocol: httpProtocol,
					}},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "ls-grpc",
					Namespace:         "default",
					Generation:        1,
					CreationTimestamp: metav1.Now(),
				},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Namespace: ns("default"), Name: "gw1",
					},
					Listeners: []gatewayv1.ListenerEntry{{
						Name: "grpc", Port: 9090, Protocol: gatewayv1.HTTPProtocolType,
					}},
				},
			},
		}
		for i := range lses {
			lses[i].Status.Conditions = []metav1.Condition{{
				Type: string(gatewayv1.ListenerSetConditionAccepted), Status: metav1.ConditionTrue,
				Reason: string(gatewayv1.ListenerSetReasonAccepted),
			}}
		}

		result := evaluateGatewayListenerSetListeners(state, defaultGateway, lses, nil)
		if len(result) != 2 {
			t.Fatalf("expected 2 evaluations, got %d", len(result))
		}
	})

	t.Run("different ListenerSets same port different hostname: both included", func(t *testing.T) {
		state := &clusterState{
			controllerName:  "example.com/gateway",
			namespaceByName: defaultState.namespaceByName,
		}
		lses := []gatewayv1.ListenerSet{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "ls-foo",
					Namespace:         "default",
					Generation:        1,
					CreationTimestamp: metav1.Now(),
				},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{Namespace: ns("default"), Name: "gw1"},
					Listeners: []gatewayv1.ListenerEntry{{
						Name: "foo", Port: 80, Protocol: httpProtocol, Hostname: hostname("foo.example.com"),
					}},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "ls-bar",
					Namespace:         "default",
					Generation:        1,
					CreationTimestamp: metav1.Now(),
				},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{Namespace: ns("default"), Name: "gw1"},
					Listeners: []gatewayv1.ListenerEntry{{
						Name: "bar", Port: 80, Protocol: httpProtocol, Hostname: hostname("bar.example.com"),
					}},
				},
			},
		}
		for i := range lses {
			lses[i].Status.Conditions = []metav1.Condition{{
				Type: string(gatewayv1.ListenerSetConditionAccepted), Status: metav1.ConditionTrue,
				Reason: string(gatewayv1.ListenerSetReasonAccepted),
			}}
		}

		result := evaluateGatewayListenerSetListeners(state, defaultGateway, lses, nil)
		if len(result) != 2 {
			t.Fatalf("expected 2 evaluations (different hostnames), got %d", len(result))
		}
	})
}

func assertConditionSpec(
	t *testing.T,
	condition conditionSpec,
	condType string,
	status metav1.ConditionStatus,
	reason string,
	observedGeneration int64,
) {
	t.Helper()
	if condition.Type != condType {
		t.Fatalf("condition type = %s, want %s", condition.Type, condType)
	}
	if condition.Status != status {
		t.Fatalf("condition %s status = %s, want %s", condType, condition.Status, status)
	}
	if condition.Reason != reason {
		t.Fatalf("condition %s reason = %s, want %s", condType, condition.Reason, reason)
	}
	if condition.ObservedGeneration != observedGeneration {
		t.Fatalf("condition %s observedGeneration = %d, want %d", condType, condition.ObservedGeneration, observedGeneration)
	}
}

func listenerEntryStatusByName(
	t *testing.T,
	listeners []gatewayv1.ListenerEntryStatus,
	name gatewayv1.SectionName,
) gatewayv1.ListenerEntryStatus {
	t.Helper()
	for _, listener := range listeners {
		if listener.Name == name {
			return listener
		}
	}
	t.Fatalf("listener %s not found in %#v", name, listeners)
	return gatewayv1.ListenerEntryStatus{}
}

func TestEvaluateGatewaysWithListenerSets(t *testing.T) {
	ns := func(name gatewayv1.Namespace) *gatewayv1.Namespace { return &name }
	from := func(from gatewayv1.FromNamespaces) *gatewayv1.FromNamespaces { return &from }

	t.Run("gateway with 0 spec listeners + 1 ListenerSet reports 1 listener", func(t *testing.T) {
		state := &clusterState{
			controllerName: "example.com/gateway",
			gatewayClasses: []gatewayv1.GatewayClass{{
				ObjectMeta: metav1.ObjectMeta{Name: "test-class"},
				Spec:       gatewayv1.GatewayClassSpec{ControllerName: "example.com/gateway"},
			}},
			gateways: []gatewayv1.Gateway{{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gw1", Namespace: "default", Generation: 1,
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "test-class",
					AllowedListeners: &gatewayv1.AllowedListeners{
						Namespaces: &gatewayv1.ListenerNamespaces{
							From: from(gatewayv1.NamespacesFromAll),
						},
					},
				},
			}},
			listenerSets: []gatewayv1.ListenerSet{{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-ls", Namespace: "default", Generation: 1,
				},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Namespace: ns("default"), Name: "gw1",
					},
					Listeners: []gatewayv1.ListenerEntry{{
						Name: "http", Port: 80, Protocol: gatewayv1.HTTPProtocolType,
					}},
				},
				Status: gatewayv1.ListenerSetStatus{
					Conditions: []metav1.Condition{{
						Type:   string(gatewayv1.ListenerSetConditionAccepted),
						Status: metav1.ConditionTrue,
						Reason: string(gatewayv1.ListenerSetReasonAccepted),
					}},
				},
			}},
			namespaceByName: map[string]corev1.Namespace{
				"default": {ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			},
		}
		state.index()

		gwEvals := evaluateGateways(state, nil)
		if len(gwEvals) != 1 {
			t.Fatalf("expected 1 gateway evaluation, got %d", len(gwEvals))
		}
		eval := gwEvals[client.ObjectKey{Namespace: "default", Name: "gw1"}]
		if len(eval.listeners) != 1 {
			t.Fatalf("expected 1 listener (from ListenerSet), got %d", len(eval.listeners))
		}
		if eval.listeners[0].name != "default/my-ls/http" {
			t.Fatalf("unexpected listener name: %s", eval.listeners[0].name)
		}
		if eval.attachedListenerSets != 1 {
			t.Fatalf("expected 1 attached listener set, got %d", eval.attachedListenerSets)
		}
	})

	t.Run("gateway spec listener wins over conflicting ListenerSet entry", func(t *testing.T) {
		state := &clusterState{
			controllerName: "example.com/gateway",
			gatewayClasses: []gatewayv1.GatewayClass{{
				ObjectMeta: metav1.ObjectMeta{Name: "test-class"},
				Spec:       gatewayv1.GatewayClassSpec{ControllerName: "example.com/gateway"},
			}},
			gateways: []gatewayv1.Gateway{{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gw1", Namespace: "default", Generation: 1,
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "test-class",
					Listeners: []gatewayv1.Listener{{
						Name: "gw-http", Port: 80, Protocol: gatewayv1.HTTPProtocolType,
					}},
					AllowedListeners: &gatewayv1.AllowedListeners{
						Namespaces: &gatewayv1.ListenerNamespaces{
							From: from(gatewayv1.NamespacesFromAll),
						},
					},
				},
			}},
			listenerSets: []gatewayv1.ListenerSet{{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-ls", Namespace: "default", Generation: 1,
				},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Namespace: ns("default"), Name: "gw1",
					},
					Listeners: []gatewayv1.ListenerEntry{{
						Name: "http", Port: 80, Protocol: gatewayv1.HTTPProtocolType,
					}},
				},
				Status: gatewayv1.ListenerSetStatus{
					Conditions: []metav1.Condition{{
						Type:   string(gatewayv1.ListenerSetConditionAccepted),
						Status: metav1.ConditionTrue,
						Reason: string(gatewayv1.ListenerSetReasonAccepted),
					}},
				},
			}},
			namespaceByName: map[string]corev1.Namespace{
				"default": {ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			},
		}
		state.index()

		gwEvals := evaluateGateways(state, nil)
		eval := gwEvals[client.ObjectKey{Namespace: "default", Name: "gw1"}]
		if len(eval.listeners) != 1 {
			t.Fatalf("expected 1 listener (gateway spec wins), got %d", len(eval.listeners))
		}
		if eval.listeners[0].name != "gw-http" {
			t.Fatalf("expected gateway spec listener 'gw-http', got %s", eval.listeners[0].name)
		}
	})
}
