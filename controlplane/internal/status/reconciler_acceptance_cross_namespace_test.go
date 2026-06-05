package status

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func TestReconcileKeepsGatewayAcceptedWhenAtLeastOneListenerIsValid(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}, &gatewayv1.Gateway{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway", Generation: 1},
				Spec:       gatewayv1.GatewayClassSpec{ControllerName: controllerName},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "aether-gateway",
					Listeners: []gatewayv1.Listener{
						{
							Name:     "http",
							Protocol: gatewayv1.HTTPProtocolType,
							Port:     80,
						},
						{
							Name:     "broken-https",
							Protocol: gatewayv1.HTTPSProtocolType,
							Port:     443,
						},
					},
				},
			},
			gatewayInfrastructureService("default", "gw"),
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	if len(gateway.Status.Listeners) != 2 {
		t.Fatalf("expected 2 listener statuses, got %d", len(gateway.Status.Listeners))
	}

	listeners := map[gatewayv1.SectionName]gatewayv1.ListenerStatus{}
	for _, listener := range gateway.Status.Listeners {
		listeners[listener.Name] = listener
	}
	assertCondition(t, listeners["http"].Conditions, string(gatewayv1.ListenerConditionAccepted), metav1.ConditionTrue, string(gatewayv1.ListenerReasonAccepted), 1)
	assertCondition(t, listeners["http"].Conditions, string(gatewayv1.ListenerConditionProgrammed), metav1.ConditionTrue, string(gatewayv1.ListenerReasonProgrammed), 1)
	assertCondition(t, listeners["broken-https"].Conditions, string(gatewayv1.ListenerConditionAccepted), metav1.ConditionFalse, string(gatewayv1.ListenerReasonInvalid), 1)
	assertCondition(t, listeners["broken-https"].Conditions, string(gatewayv1.ListenerConditionProgrammed), metav1.ConditionFalse, string(gatewayv1.ListenerReasonInvalid), 1)

	assertCondition(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionAccepted), metav1.ConditionTrue, string(gatewayv1.GatewayReasonListenersNotValid), 1)
	assertCondition(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionProgrammed), metav1.ConditionFalse, string(gatewayv1.GatewayReasonListenersNotValid), 1)
	if conditionMessage(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionAccepted)) == "" {
		t.Fatalf("expected readable gateway accepted message")
	}
}

func TestReconcileDisallowsCrossNamespaceRouteWhenAllowedRoutesFromSame(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")
	namespaceMode := gatewayv1.NamespacesFromSame

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
			&gatewayv1.HTTPRoute{},
		).
		WithObjects(
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gateway-conformance-infra",
					Labels: map[string]string{
						"kubernetes.io/metadata.name": "gateway-conformance-infra",
					},
				},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gateway-conformance-web-backend",
					Labels: map[string]string{
						"kubernetes.io/metadata.name": "gateway-conformance-web-backend",
					},
				},
			},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "gateway-conformance-infra", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "aether-gateway",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
						AllowedRoutes: &gatewayv1.AllowedRoutes{
							Namespaces: &gatewayv1.RouteNamespaces{
								From: &namespaceMode,
							},
						},
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "gateway-conformance-web-backend"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8080}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "gateway-conformance-web-backend", Generation: 1},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Name:      "gw",
							Namespace: namespacePtr("gateway-conformance-infra"),
						}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "echo",
									Port: portPtr(8080),
								},
							},
						}},
					}},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "gateway-conformance-infra", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	if len(gateway.Status.Listeners) != 1 {
		t.Fatalf("expected 1 listener status, got %d", len(gateway.Status.Listeners))
	}
	if gateway.Status.Listeners[0].AttachedRoutes != 0 {
		t.Fatalf("expected attachedRoutes=0, got %d", gateway.Status.Listeners[0].AttachedRoutes)
	}

	var route gatewayv1.HTTPRoute
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "gateway-conformance-web-backend", Name: "route"}, &route); err != nil {
		t.Fatalf("Get HTTPRoute returned error: %v", err)
	}
	if len(route.Status.Parents) != 1 {
		t.Fatalf("expected 1 parent status, got %d", len(route.Status.Parents))
	}
	assertCondition(t, route.Status.Parents[0].Conditions, string(gatewayv1.RouteConditionAccepted), metav1.ConditionFalse, string(gatewayv1.RouteReasonNotAllowedByListeners), 1)
	assertCondition(t, route.Status.Parents[0].Conditions, string(gatewayv1.RouteConditionResolvedRefs), metav1.ConditionTrue, string(gatewayv1.RouteReasonResolvedRefs), 1)
}

func TestReconcileAcceptsCrossNamespaceRouteWhenAllowedRoutesFromAll(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")
	namespaceMode := gatewayv1.NamespacesFromAll

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
			&gatewayv1.HTTPRoute{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-web-backend"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway", Generation: 1},
				Spec:       gatewayv1.GatewayClassSpec{ControllerName: controllerName},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "gateway-conformance-infra", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "aether-gateway",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
						AllowedRoutes: &gatewayv1.AllowedRoutes{
							Namespaces: &gatewayv1.RouteNamespaces{From: &namespaceMode},
						},
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "gateway-conformance-web-backend"},
				Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 8080}}},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "gateway-conformance-web-backend", Generation: 1},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Name:      "gw",
							Namespace: namespacePtr("gateway-conformance-infra"),
						}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "echo",
									Port: portPtr(8080),
								},
							},
						}},
					}},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "gateway-conformance-infra", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	if len(gateway.Status.Listeners) != 1 {
		t.Fatalf("expected 1 listener status, got %d", len(gateway.Status.Listeners))
	}
	if gateway.Status.Listeners[0].AttachedRoutes != 1 {
		t.Fatalf("expected attachedRoutes=1, got %d", gateway.Status.Listeners[0].AttachedRoutes)
	}

	var route gatewayv1.HTTPRoute
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "gateway-conformance-web-backend", Name: "route"}, &route); err != nil {
		t.Fatalf("Get HTTPRoute returned error: %v", err)
	}
	if len(route.Status.Parents) != 1 {
		t.Fatalf("expected 1 parent status, got %d", len(route.Status.Parents))
	}
	assertCondition(t, route.Status.Parents[0].Conditions, string(gatewayv1.RouteConditionAccepted), metav1.ConditionTrue, string(gatewayv1.RouteReasonAccepted), 1)
	assertCondition(t, route.Status.Parents[0].Conditions, string(gatewayv1.RouteConditionResolvedRefs), metav1.ConditionTrue, string(gatewayv1.RouteReasonResolvedRefs), 1)
}

func TestReconcileRecomputesCrossNamespaceRouteWhenNamespaceSelectorChanges(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")
	namespaceMode := gatewayv1.NamespacesFromSelector

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
			&gatewayv1.HTTPRoute{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "infra"}},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "apps",
					Labels: map[string]string{"tenant": "edge"},
				},
			},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway", Generation: 1},
				Spec:       gatewayv1.GatewayClassSpec{ControllerName: controllerName},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "infra", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "aether-gateway",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
						AllowedRoutes: &gatewayv1.AllowedRoutes{
							Namespaces: &gatewayv1.RouteNamespaces{
								From: &namespaceMode,
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"tenant": "edge"},
								},
							},
						},
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "apps"},
				Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 8080}}},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "apps", Generation: 1},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Name:      "gw",
							Namespace: namespacePtr("infra"),
						}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "echo",
									Port: portPtr(8080),
								},
							},
						}},
					}},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("initial Reconcile returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "infra", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	if got := gateway.Status.Listeners[0].AttachedRoutes; got != 1 {
		t.Fatalf("initial attachedRoutes = %d, want 1", got)
	}

	var route gatewayv1.HTTPRoute
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "apps", Name: "route"}, &route); err != nil {
		t.Fatalf("Get HTTPRoute returned error: %v", err)
	}
	assertCondition(t, route.Status.Parents[0].Conditions, string(gatewayv1.RouteConditionAccepted), metav1.ConditionTrue, string(gatewayv1.RouteReasonAccepted), 1)

	var routeNamespace corev1.Namespace
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: "apps"}, &routeNamespace); err != nil {
		t.Fatalf("Get Namespace returned error: %v", err)
	}
	routeNamespace.Labels = map[string]string{"tenant": "other"}
	if err := k8sClient.Update(context.Background(), &routeNamespace); err != nil {
		t.Fatalf("Update Namespace returned error: %v", err)
	}

	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("second Reconcile returned error: %v", err)
	}

	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "infra", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	if got := gateway.Status.Listeners[0].AttachedRoutes; got != 0 {
		t.Fatalf("updated attachedRoutes = %d, want 0", got)
	}

	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "apps", Name: "route"}, &route); err != nil {
		t.Fatalf("Get HTTPRoute returned error: %v", err)
	}
	assertCondition(t, route.Status.Parents[0].Conditions, string(gatewayv1.RouteConditionAccepted), metav1.ConditionFalse, string(gatewayv1.RouteReasonNotAllowedByListeners), 1)
	assertCondition(t, route.Status.Parents[0].Conditions, string(gatewayv1.RouteConditionResolvedRefs), metav1.ConditionTrue, string(gatewayv1.RouteReasonResolvedRefs), 1)
}

func TestReconcileBindsRouteToGatewayListenerByParentRefPort(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
			&gatewayv1.HTTPRoute{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway", Generation: 1},
				Spec:       gatewayv1.GatewayClassSpec{ControllerName: controllerName},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "aether-gateway",
					Listeners: []gatewayv1.Listener{
						{Name: "http-80", Protocol: gatewayv1.HTTPProtocolType, Port: 80},
						{Name: "http-8080", Protocol: gatewayv1.HTTPProtocolType, Port: 8080},
					},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 8080}}},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default", Generation: 1},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Name: "gw",
							Port: portPtr(8080),
						}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "echo",
									Port: portPtr(8080),
								},
							},
						}},
					}},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	if len(gateway.Status.Listeners) != 2 {
		t.Fatalf("expected 2 listener statuses, got %d", len(gateway.Status.Listeners))
	}

	listeners := make(map[gatewayv1.SectionName]gatewayv1.ListenerStatus, len(gateway.Status.Listeners))
	for _, listener := range gateway.Status.Listeners {
		listeners[listener.Name] = listener
	}
	if got := listeners["http-80"].AttachedRoutes; got != 0 {
		t.Fatalf("http-80 attachedRoutes = %d, want 0", got)
	}
	if got := listeners["http-8080"].AttachedRoutes; got != 1 {
		t.Fatalf("http-8080 attachedRoutes = %d, want 1", got)
	}

	var route gatewayv1.HTTPRoute
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "route"}, &route); err != nil {
		t.Fatalf("Get HTTPRoute returned error: %v", err)
	}
	if len(route.Status.Parents) != 1 {
		t.Fatalf("expected 1 parent status, got %d", len(route.Status.Parents))
	}
	if route.Status.Parents[0].ParentRef.Port == nil || *route.Status.Parents[0].ParentRef.Port != 8080 {
		t.Fatalf("parentRef.port = %#v, want 8080", route.Status.Parents[0].ParentRef.Port)
	}
	assertCondition(t, route.Status.Parents[0].Conditions, string(gatewayv1.RouteConditionAccepted), metav1.ConditionTrue, string(gatewayv1.RouteReasonAccepted), 1)
}

func TestReconcileRejectsRouteWhenListenerKindsExcludeRouteKind(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
			&gatewayv1.HTTPRoute{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway", Generation: 1},
				Spec:       gatewayv1.GatewayClassSpec{ControllerName: controllerName},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "aether-gateway",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
						AllowedRoutes: &gatewayv1.AllowedRoutes{
							Kinds: []gatewayv1.RouteGroupKind{{
								Group: ptr(gatewayv1.Group(gatewayv1.GroupName)),
								Kind:  "GRPCRoute",
							}},
						},
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 8080}}},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default", Generation: 1},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{Name: "gw"}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "echo",
									Port: portPtr(8080),
								},
							},
						}},
					}},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	if got := gateway.Status.Listeners[0].AttachedRoutes; got != 0 {
		t.Fatalf("attachedRoutes = %d, want 0", got)
	}

	var route gatewayv1.HTTPRoute
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "route"}, &route); err != nil {
		t.Fatalf("Get HTTPRoute returned error: %v", err)
	}
	if len(route.Status.Parents) != 1 {
		t.Fatalf("expected 1 parent status, got %d", len(route.Status.Parents))
	}
	assertCondition(t, route.Status.Parents[0].Conditions, string(gatewayv1.RouteConditionAccepted), metav1.ConditionFalse, string(gatewayv1.RouteReasonNotAllowedByListeners), 1)
	assertCondition(t, route.Status.Parents[0].Conditions, string(gatewayv1.RouteConditionResolvedRefs), metav1.ConditionTrue, string(gatewayv1.RouteReasonResolvedRefs), 1)
}

func TestReconcileDoesNotWriteStatusForMissingGatewayParent(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}, &gatewayv1.HTTPRoute{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway", Generation: 1},
				Spec:       gatewayv1.GatewayClassSpec{ControllerName: controllerName},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 8080}}},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default", Generation: 1},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{Name: "missing"}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "echo",
									Port: portPtr(8080),
								},
							},
						}},
					}},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var route gatewayv1.HTTPRoute
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "route"}, &route); err != nil {
		t.Fatalf("Get HTTPRoute returned error: %v", err)
	}
	if len(route.Status.Parents) != 0 {
		t.Fatalf("expected no parent statuses for missing Gateway, got %#v", route.Status.Parents)
	}
}

func TestReconcileRefreshesResolvedRefsWhenReferenceGrantChanges(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
			&gatewayv1.HTTPRoute{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "infra"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "backend"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway", Generation: 1},
				Spec:       gatewayv1.GatewayClassSpec{ControllerName: controllerName},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "infra", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "aether-gateway",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "backend"},
				Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 8080}}},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "infra", Generation: 1},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{Name: "gw"}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name:      "echo",
									Namespace: namespacePtr("backend"),
									Port:      portPtr(8080),
								},
							},
						}},
					}},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("initial Reconcile returned error: %v", err)
	}

	var route gatewayv1.HTTPRoute
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "infra", Name: "route"}, &route); err != nil {
		t.Fatalf("Get HTTPRoute returned error: %v", err)
	}
	assertCondition(t, route.Status.Parents[0].Conditions, string(gatewayv1.RouteConditionAccepted), metav1.ConditionTrue, string(gatewayv1.RouteReasonAccepted), 1)
	assertCondition(t, route.Status.Parents[0].Conditions, string(gatewayv1.RouteConditionResolvedRefs), metav1.ConditionFalse, string(gatewayv1.RouteReasonRefNotPermitted), 1)

	grant := &gatewayv1beta1.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{Name: "allow-infra-route", Namespace: "backend"},
		Spec: gatewayv1beta1.ReferenceGrantSpec{
			From: []gatewayv1beta1.ReferenceGrantFrom{{
				Group:     gatewayv1beta1.Group(gatewayv1.GroupName),
				Kind:      gatewayv1beta1.Kind("HTTPRoute"),
				Namespace: gatewayv1beta1.Namespace("infra"),
			}},
			To: []gatewayv1beta1.ReferenceGrantTo{{
				Group: gatewayv1beta1.Group(""),
				Kind:  gatewayv1beta1.Kind("Service"),
				Name:  ptr[gatewayv1beta1.ObjectName]("echo"),
			}},
		},
	}
	if err := k8sClient.Create(context.Background(), grant); err != nil {
		t.Fatalf("Create ReferenceGrant returned error: %v", err)
	}

	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("second Reconcile returned error: %v", err)
	}

	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "infra", Name: "route"}, &route); err != nil {
		t.Fatalf("Get HTTPRoute returned error: %v", err)
	}
	assertCondition(t, route.Status.Parents[0].Conditions, string(gatewayv1.RouteConditionResolvedRefs), metav1.ConditionTrue, string(gatewayv1.RouteReasonResolvedRefs), 1)

	if err := k8sClient.Delete(context.Background(), grant); err != nil {
		t.Fatalf("Delete ReferenceGrant returned error: %v", err)
	}

	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("third Reconcile returned error: %v", err)
	}

	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "infra", Name: "route"}, &route); err != nil {
		t.Fatalf("Get HTTPRoute returned error: %v", err)
	}
	assertCondition(t, route.Status.Parents[0].Conditions, string(gatewayv1.RouteConditionResolvedRefs), metav1.ConditionFalse, string(gatewayv1.RouteReasonRefNotPermitted), 1)
}

func TestReconcileRefreshesCertificateRefsWhenReferenceGrantChanges(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")
	mode := gatewayv1.TLSModeTerminate

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "shared"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway", Generation: 1},
				Spec:       gatewayv1.GatewayClassSpec{ControllerName: controllerName},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "aether-gateway",
					Listeners: []gatewayv1.Listener{{
						Name:     "https",
						Protocol: gatewayv1.HTTPSProtocolType,
						Port:     443,
						TLS: &gatewayv1.ListenerTLSConfig{
							Mode: &mode,
							CertificateRefs: []gatewayv1.SecretObjectReference{{
								Name:      "shared-cert",
								Namespace: namespacePtr("shared"),
							}},
						},
					}},
				},
			},
			gatewayInfrastructureService("default", "gw"),
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "shared-cert", Namespace: "shared"},
				Type:       corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": []byte(readStatusTLSAsset(t, "client.crt")),
					"tls.key": []byte(readStatusTLSAsset(t, "client.key")),
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("initial Reconcile returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	assertCondition(t, gateway.Status.Listeners[0].Conditions, string(gatewayv1.ListenerConditionResolvedRefs), metav1.ConditionFalse, string(gatewayv1.ListenerReasonRefNotPermitted), 1)

	grant := &gatewayv1beta1.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{Name: "allow-default-gateway", Namespace: "shared"},
		Spec: gatewayv1beta1.ReferenceGrantSpec{
			From: []gatewayv1beta1.ReferenceGrantFrom{{
				Group:     gatewayv1beta1.Group(gatewayv1.GroupName),
				Kind:      gatewayv1beta1.Kind("Gateway"),
				Namespace: gatewayv1beta1.Namespace("default"),
			}},
			To: []gatewayv1beta1.ReferenceGrantTo{{
				Group: gatewayv1beta1.Group(""),
				Kind:  gatewayv1beta1.Kind("Secret"),
				Name:  ptr[gatewayv1beta1.ObjectName]("shared-cert"),
			}},
		},
	}
	if err := k8sClient.Create(context.Background(), grant); err != nil {
		t.Fatalf("Create ReferenceGrant returned error: %v", err)
	}

	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("second Reconcile returned error: %v", err)
	}

	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	assertCondition(t, gateway.Status.Listeners[0].Conditions, string(gatewayv1.ListenerConditionResolvedRefs), metav1.ConditionTrue, string(gatewayv1.ListenerReasonResolvedRefs), 1)
	assertCondition(t, gateway.Status.Listeners[0].Conditions, string(gatewayv1.ListenerConditionProgrammed), metav1.ConditionTrue, string(gatewayv1.ListenerReasonProgrammed), 1)

	if err := k8sClient.Delete(context.Background(), grant); err != nil {
		t.Fatalf("Delete ReferenceGrant returned error: %v", err)
	}

	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("third Reconcile returned error: %v", err)
	}

	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	assertCondition(t, gateway.Status.Listeners[0].Conditions, string(gatewayv1.ListenerConditionResolvedRefs), metav1.ConditionFalse, string(gatewayv1.ListenerReasonRefNotPermitted), 1)
	assertCondition(t, gateway.Status.Listeners[0].Conditions, string(gatewayv1.ListenerConditionProgrammed), metav1.ConditionFalse, string(gatewayv1.ListenerReasonInvalid), 1)
}
