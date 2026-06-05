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
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/controlplane/internal/managedresources"
)

func TestReconcileAcceptsServiceImportBackendRef(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

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
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			&mcsv1alpha1.ServiceImport{
				ObjectMeta: metav1.ObjectMeta{Name: "payments", Namespace: "default"},
				Spec: mcsv1alpha1.ServiceImportSpec{
					Type: mcsv1alpha1.ClusterSetIP,
					Ports: []mcsv1alpha1.ServicePort{{
						Name:     "grpc",
						Port:     9443,
						Protocol: corev1.ProtocolTCP,
					}},
				},
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
									Group: ptr(gatewayv1.Group(mcsv1alpha1.GroupName)),
									Kind:  ptr(gatewayv1.Kind("ServiceImport")),
									Name:  "payments",
									Port:  portPtr(9443),
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
	if len(route.Status.Parents) != 1 {
		t.Fatalf("expected 1 parent status, got %d", len(route.Status.Parents))
	}
	assertCondition(t, route.Status.Parents[0].Conditions, string(gatewayv1.RouteConditionAccepted), metav1.ConditionTrue, string(gatewayv1.RouteReasonAccepted), 1)
	assertCondition(t, route.Status.Parents[0].Conditions, string(gatewayv1.RouteConditionResolvedRefs), metav1.ConditionTrue, string(gatewayv1.RouteReasonResolvedRefs), 1)
}

func TestReconcileRejectsHTTPRequestRedirectAndURLRewriteInSameRule(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	redirectScheme := "https"
	rewriteHostname := gatewayv1.PreciseHostname("backend.internal")

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
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "backend", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8080}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default", Generation: 2},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Name: "gw",
						}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						Filters: []gatewayv1.HTTPRouteFilter{
							{
								Type: gatewayv1.HTTPRouteFilterRequestRedirect,
								RequestRedirect: &gatewayv1.HTTPRequestRedirectFilter{
									Scheme: &redirectScheme,
								},
							},
							{
								Type: gatewayv1.HTTPRouteFilterURLRewrite,
								URLRewrite: &gatewayv1.HTTPURLRewriteFilter{
									Hostname: &rewriteHostname,
								},
							},
						},
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "backend",
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
	if len(route.Status.Parents) != 1 {
		t.Fatalf("expected 1 parent status, got %d", len(route.Status.Parents))
	}
	parent := route.Status.Parents[0]
	assertCondition(t, parent.Conditions, string(gatewayv1.RouteConditionAccepted), metav1.ConditionTrue, string(gatewayv1.RouteReasonAccepted), 2)
	assertCondition(t, parent.Conditions, string(gatewayv1.RouteConditionResolvedRefs), metav1.ConditionFalse, string(gatewayv1.RouteReasonUnsupportedValue), 2)
	assertConditionAbsent(t, parent.Conditions, string(gatewayv1.RouteConditionPartiallyInvalid))
	if message := conditionMessage(t, parent.Conditions, string(gatewayv1.RouteConditionResolvedRefs)); message != "HTTPRoute rule 1 must not combine RequestRedirect and URLRewrite filters" {
		t.Fatalf("unexpected resolved refs message: %q", message)
	}
}

func TestReconcileRejectsUnsupportedHTTPExternalAuthFilter(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

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
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "backend", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8080}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default", Generation: 5},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Name: "gw",
						}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						Filters: []gatewayv1.HTTPRouteFilter{{
							Type: gatewayv1.HTTPRouteFilterExternalAuth,
							ExternalAuth: &gatewayv1.HTTPExternalAuthFilter{
								ExternalAuthProtocol: gatewayv1.HTTPRouteExternalAuthProtocol("CUSTOM"),
								BackendRef: gatewayv1.BackendObjectReference{
									Name: "auth",
									Port: portPtr(9000),
								},
							},
						}},
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "backend",
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
	if len(route.Status.Parents) != 1 {
		t.Fatalf("expected 1 parent status, got %d", len(route.Status.Parents))
	}

	parent := route.Status.Parents[0]
	assertCondition(t, parent.Conditions, string(gatewayv1.RouteConditionAccepted), metav1.ConditionFalse, string(gatewayv1.RouteReasonUnsupportedValue), 5)
	assertCondition(t, parent.Conditions, string(gatewayv1.RouteConditionResolvedRefs), metav1.ConditionTrue, string(gatewayv1.RouteReasonResolvedRefs), 5)
	assertConditionMessage(t, parent.Conditions, string(gatewayv1.RouteConditionAccepted), "HTTPRoute rule 1 uses unsupported ExternalAuth filter")
}

func TestReconcileAcceptsGRPCRouteOnHTTPListener(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
			&gatewayv1.GRPCRoute{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "same-namespace", Namespace: "gateway-conformance-infra", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
						AllowedRoutes: &gatewayv1.AllowedRoutes{
							Namespaces: &gatewayv1.RouteNamespaces{
								From: namespaceFromPtr(gatewayv1.NamespacesFromSame),
							},
						},
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "grpc-infra-backend-v1", Namespace: "gateway-conformance-infra"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8080}},
				},
			},
			&gatewayv1.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "exact-matching", Namespace: "gateway-conformance-infra", Generation: 1},
				Spec: gatewayv1.GRPCRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Name: "same-namespace",
						}},
					},
					Rules: []gatewayv1.GRPCRouteRule{{
						Matches: []gatewayv1.GRPCRouteMatch{{
							Method: &gatewayv1.GRPCMethodMatch{
								Service: ptr("gateway_api_conformance.echo_basic.grpcecho.GrpcEcho"),
								Method:  ptr("Echo"),
							},
						}},
						BackendRefs: []gatewayv1.GRPCBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "grpc-infra-backend-v1",
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
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "gateway-conformance-infra", Name: "same-namespace"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	if len(gateway.Status.Listeners) != 1 {
		t.Fatalf("expected 1 listener status, got %d", len(gateway.Status.Listeners))
	}
	assertSupportedKinds(t, gateway.Status.Listeners[0].SupportedKinds, "GRPCRoute", "HTTPRoute")
	if gateway.Status.Listeners[0].AttachedRoutes != 1 {
		t.Fatalf("expected attachedRoutes=1, got %d", gateway.Status.Listeners[0].AttachedRoutes)
	}

	var route gatewayv1.GRPCRoute
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "gateway-conformance-infra", Name: "exact-matching"}, &route); err != nil {
		t.Fatalf("Get GRPCRoute returned error: %v", err)
	}
	if len(route.Status.Parents) != 1 {
		t.Fatalf("expected 1 parent status, got %d", len(route.Status.Parents))
	}
	assertCondition(t, route.Status.Parents[0].Conditions, string(gatewayv1.RouteConditionAccepted), metav1.ConditionTrue, string(gatewayv1.RouteReasonAccepted), 1)
	assertCondition(t, route.Status.Parents[0].Conditions, string(gatewayv1.RouteConditionResolvedRefs), metav1.ConditionTrue, string(gatewayv1.RouteReasonResolvedRefs), 1)
}

func TestReconcileMarksHTTPRoutePartiallyInvalidWhenDroppingBadRule(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	redirectScheme := "https"
	rewriteHostname := gatewayv1.PreciseHostname("example.com")

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
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			gatewayInfrastructureService("default", "gw"),
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "backend", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8080}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default", Generation: 4},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Name: "gw",
						}},
					},
					Rules: []gatewayv1.HTTPRouteRule{
						{
							Filters: []gatewayv1.HTTPRouteFilter{
								{
									Type: gatewayv1.HTTPRouteFilterRequestRedirect,
									RequestRedirect: &gatewayv1.HTTPRequestRedirectFilter{
										Scheme: &redirectScheme,
									},
								},
								{
									Type: gatewayv1.HTTPRouteFilterURLRewrite,
									URLRewrite: &gatewayv1.HTTPURLRewriteFilter{
										Hostname: &rewriteHostname,
									},
								},
							},
							BackendRefs: []gatewayv1.HTTPBackendRef{{
								BackendRef: gatewayv1.BackendRef{
									BackendObjectReference: gatewayv1.BackendObjectReference{
										Name: "backend",
										Port: portPtr(8080),
									},
								},
							}},
						},
						{
							BackendRefs: []gatewayv1.HTTPBackendRef{{
								BackendRef: gatewayv1.BackendRef{
									BackendObjectReference: gatewayv1.BackendObjectReference{
										Name: "backend",
										Port: portPtr(8080),
									},
								},
							}},
						},
					},
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
	if len(route.Status.Parents) != 1 {
		t.Fatalf("expected 1 parent status, got %d", len(route.Status.Parents))
	}

	parent := route.Status.Parents[0]
	assertCondition(t, parent.Conditions, string(gatewayv1.RouteConditionAccepted), metav1.ConditionTrue, string(gatewayv1.RouteReasonAccepted), 4)
	assertCondition(t, parent.Conditions, string(gatewayv1.RouteConditionResolvedRefs), metav1.ConditionTrue, string(gatewayv1.RouteReasonResolvedRefs), 4)
	assertCondition(t, parent.Conditions, string(gatewayv1.RouteConditionPartiallyInvalid), metav1.ConditionTrue, string(gatewayv1.RouteReasonUnsupportedValue), 4)
	if message := conditionMessage(t, parent.Conditions, string(gatewayv1.RouteConditionPartiallyInvalid)); message != "Dropped Rule 1 because HTTPRoute rule 1 must not combine RequestRedirect and URLRewrite filters" {
		t.Fatalf("unexpected partially invalid message: %q", message)
	}
}

func TestReconcileMarksCrossNamespaceBackendRefAsNotPermitted(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

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
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "infra", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "backend"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8080}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "infra", Generation: 1},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Name: "gw",
						}},
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
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var route gatewayv1.HTTPRoute
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "infra", Name: "route"}, &route); err != nil {
		t.Fatalf("Get HTTPRoute returned error: %v", err)
	}
	if len(route.Status.Parents) != 1 {
		t.Fatalf("expected 1 parent status, got %d", len(route.Status.Parents))
	}
	assertCondition(t, route.Status.Parents[0].Conditions, string(gatewayv1.RouteConditionAccepted), metav1.ConditionTrue, string(gatewayv1.RouteReasonAccepted), 1)
	assertCondition(t, route.Status.Parents[0].Conditions, string(gatewayv1.RouteConditionResolvedRefs), metav1.ConditionFalse, string(gatewayv1.RouteReasonRefNotPermitted), 1)
}

func TestReconcileGatewayInvalidRouteKindsStatus(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	httpKind := gatewayv1.Kind("HTTPRoute")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gateway-only-invalid-route-kind", Namespace: "gateway-conformance-infra", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
						AllowedRoutes: &gatewayv1.AllowedRoutes{
							Kinds: []gatewayv1.RouteGroupKind{{
								Group: groupPtr(gatewayGroup),
								Kind:  "InvalidRoute",
							}},
						},
					}},
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gateway-supported-and-invalid-route-kind", Namespace: "gateway-conformance-infra", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
						AllowedRoutes: &gatewayv1.AllowedRoutes{
							Kinds: []gatewayv1.RouteGroupKind{
								{
									Group: groupPtr(gatewayGroup),
									Kind:  "HTTPRoute",
								},
								{
									Group: groupPtr(gatewayGroup),
									Kind:  "InvalidRoute",
								},
							},
						},
					}},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var invalidOnly gatewayv1.Gateway
	if err := k8sClient.Get(
		context.Background(),
		client.ObjectKey{Namespace: "gateway-conformance-infra", Name: "gateway-only-invalid-route-kind"},
		&invalidOnly,
	); err != nil {
		t.Fatalf("Get invalid-only Gateway returned error: %v", err)
	}
	if len(invalidOnly.Status.Listeners) != 1 {
		t.Fatalf("expected 1 listener status for invalid-only gateway, got %d", len(invalidOnly.Status.Listeners))
	}
	if invalidOnly.Status.Listeners[0].SupportedKinds == nil {
		t.Fatalf("expected supportedKinds to be an empty slice, got nil")
	}
	if len(invalidOnly.Status.Listeners[0].SupportedKinds) != 0 {
		t.Fatalf("expected no supported kinds, got %#v", invalidOnly.Status.Listeners[0].SupportedKinds)
	}
	assertCondition(t, invalidOnly.Status.Conditions, string(gatewayv1.GatewayConditionAccepted), metav1.ConditionTrue, string(gatewayv1.GatewayReasonAccepted), 1)
	assertCondition(t, invalidOnly.Status.Conditions, string(gatewayv1.GatewayConditionProgrammed), metav1.ConditionFalse, string(gatewayv1.GatewayReasonListenersNotValid), 1)
	assertCondition(t, invalidOnly.Status.Listeners[0].Conditions, string(gatewayv1.ListenerConditionResolvedRefs), metav1.ConditionFalse, string(gatewayv1.ListenerReasonInvalidRouteKinds), 1)

	var mixed gatewayv1.Gateway
	if err := k8sClient.Get(
		context.Background(),
		client.ObjectKey{Namespace: "gateway-conformance-infra", Name: "gateway-supported-and-invalid-route-kind"},
		&mixed,
	); err != nil {
		t.Fatalf("Get mixed Gateway returned error: %v", err)
	}
	if len(mixed.Status.Listeners) != 1 {
		t.Fatalf("expected 1 listener status for mixed gateway, got %d", len(mixed.Status.Listeners))
	}
	if len(mixed.Status.Listeners[0].SupportedKinds) != 1 || mixed.Status.Listeners[0].SupportedKinds[0].Kind != httpKind {
		t.Fatalf("unexpected mixed supported kinds: %#v", mixed.Status.Listeners[0].SupportedKinds)
	}
	assertCondition(t, mixed.Status.Listeners[0].Conditions, string(gatewayv1.ListenerConditionResolvedRefs), metav1.ConditionFalse, string(gatewayv1.ListenerReasonInvalidRouteKinds), 1)
}

func TestReconcileMarksMalformedTLSSecretAsInvalidCertificateRef(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gateway-certificate-malformed-secret", Namespace: "gateway-conformance-infra", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "https",
						Protocol: gatewayv1.HTTPSProtocolType,
						Port:     443,
						TLS: &gatewayv1.ListenerTLSConfig{
							Mode: func() *gatewayv1.TLSModeType {
								mode := gatewayv1.TLSModeTerminate
								return &mode
							}(),
							CertificateRefs: []gatewayv1.SecretObjectReference{{
								Name: "malformed-certificate",
							}},
						},
					}},
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "malformed-certificate",
					Namespace: "gateway-conformance-infra",
				},
				Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": []byte("Hello world\n"),
					"tls.key": []byte("Hello world\n"),
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(
		context.Background(),
		client.ObjectKey{Namespace: "gateway-conformance-infra", Name: "gateway-certificate-malformed-secret"},
		&gateway,
	); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	if len(gateway.Status.Listeners) != 1 {
		t.Fatalf("expected 1 listener status, got %d", len(gateway.Status.Listeners))
	}

	listener := gateway.Status.Listeners[0]
	assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionResolvedRefs), metav1.ConditionFalse, string(gatewayv1.ListenerReasonInvalidCertificateRef), 1)
	assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionProgrammed), metav1.ConditionFalse, string(gatewayv1.ListenerReasonInvalid), 1)
	assertCondition(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionProgrammed), metav1.ConditionFalse, string(gatewayv1.GatewayReasonListenersNotValid), 1)
}

func TestReconcileMarksMissingTLSSecretAsInvalidCertificateRef(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	mode := gatewayv1.TLSModeTerminate

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gateway-certificate-missing-secret", Namespace: "gateway-conformance-infra", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "https",
						Protocol: gatewayv1.HTTPSProtocolType,
						Port:     443,
						TLS: &gatewayv1.ListenerTLSConfig{
							Mode: &mode,
							CertificateRefs: []gatewayv1.SecretObjectReference{{
								Name: "missing-certificate",
							}},
						},
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
	if err := k8sClient.Get(
		context.Background(),
		client.ObjectKey{Namespace: "gateway-conformance-infra", Name: "gateway-certificate-missing-secret"},
		&gateway,
	); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	if len(gateway.Status.Listeners) != 1 {
		t.Fatalf("expected 1 listener status, got %d", len(gateway.Status.Listeners))
	}

	listener := gateway.Status.Listeners[0]
	assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionResolvedRefs), metav1.ConditionFalse, string(gatewayv1.ListenerReasonInvalidCertificateRef), 1)
	assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionProgrammed), metav1.ConditionFalse, string(gatewayv1.ListenerReasonInvalid), 1)
	assertCondition(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionProgrammed), metav1.ConditionFalse, string(gatewayv1.GatewayReasonListenersNotValid), 1)
}

func TestReconcileKeepsProgrammedWhenAtLeastOneCertificateRefIsValid(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	mode := gatewayv1.TLSModeTerminate

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gateway-certificate-partially-valid", Namespace: "gateway-conformance-infra", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "https",
						Protocol: gatewayv1.HTTPSProtocolType,
						Port:     443,
						TLS: &gatewayv1.ListenerTLSConfig{
							Mode: &mode,
							CertificateRefs: []gatewayv1.SecretObjectReference{
								{Name: "valid-certificate"},
								{Name: "missing-certificate"},
							},
						},
					}},
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "valid-certificate",
					Namespace: "gateway-conformance-infra",
				},
				Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": []byte(readStatusTLSAsset(t, "client.crt")),
					"tls.key": []byte(readStatusTLSAsset(t, "client.key")),
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(
		context.Background(),
		client.ObjectKey{Namespace: "gateway-conformance-infra", Name: "gateway-certificate-partially-valid"},
		&gateway,
	); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	if len(gateway.Status.Listeners) != 1 {
		t.Fatalf("expected 1 listener status, got %d", len(gateway.Status.Listeners))
	}

	listener := gateway.Status.Listeners[0]
	assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionResolvedRefs), metav1.ConditionFalse, string(gatewayv1.ListenerReasonInvalidCertificateRef), 1)
	assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionAccepted), metav1.ConditionTrue, string(gatewayv1.ListenerReasonAccepted), 1)
	assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionProgrammed), metav1.ConditionTrue, string(gatewayv1.ListenerReasonProgrammed), 1)
}

func TestReconcileRejectsCrossNamespaceCertificateRefWithoutReferenceGrant(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
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
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
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
				ObjectMeta: metav1.ObjectMeta{
					Name:      "shared-cert",
					Namespace: "shared",
				},
				Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": []byte(readStatusTLSAsset(t, "client.crt")),
					"tls.key": []byte(readStatusTLSAsset(t, "client.key")),
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
	if len(gateway.Status.Listeners) != 1 {
		t.Fatalf("expected 1 listener status, got %d", len(gateway.Status.Listeners))
	}

	listener := gateway.Status.Listeners[0]
	assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionResolvedRefs), metav1.ConditionFalse, string(gatewayv1.ListenerReasonRefNotPermitted), 1)
	assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionProgrammed), metav1.ConditionFalse, string(gatewayv1.ListenerReasonInvalid), 1)
	assertCondition(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionProgrammed), metav1.ConditionFalse, string(gatewayv1.GatewayReasonListenersNotValid), 1)
}

func TestReconcileAcceptsCrossNamespaceCertificateRefWithReferenceGrant(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	mode := gatewayv1.TLSModeTerminate
	service := gatewayInfrastructureService("default", "gw")
	endpointSlice := gatewayInfrastructureEndpointSliceForService(service, managedresources.EndpointSliceRoleGatewayFrontend)

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
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
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
			service,
			endpointSlice,
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "shared-cert",
					Namespace: "shared",
				},
				Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": []byte(readStatusTLSAsset(t, "client.crt")),
					"tls.key": []byte(readStatusTLSAsset(t, "client.key")),
				},
			},
			&gatewayv1beta1.ReferenceGrant{
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
	if len(gateway.Status.Listeners) != 1 {
		t.Fatalf("expected 1 listener status, got %d", len(gateway.Status.Listeners))
	}

	listener := gateway.Status.Listeners[0]
	assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionAccepted), metav1.ConditionTrue, string(gatewayv1.ListenerReasonAccepted), 1)
	assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionResolvedRefs), metav1.ConditionTrue, string(gatewayv1.ListenerReasonResolvedRefs), 1)
	assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionProgrammed), metav1.ConditionTrue, string(gatewayv1.ListenerReasonProgrammed), 1)
	assertCondition(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionProgrammed), metav1.ConditionTrue, string(gatewayv1.GatewayReasonProgrammed), 1)
}

func TestReconcileMarksHostnameConflictingListeners(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	hostname := gatewayv1.Hostname("foo.example.com")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}, &gatewayv1.Gateway{}).
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
					Listeners: []gatewayv1.Listener{
						{
							Name:     "first",
							Protocol: gatewayv1.HTTPProtocolType,
							Port:     80,
							Hostname: &hostname,
						},
						{
							Name:     "second",
							Protocol: gatewayv1.HTTPProtocolType,
							Port:     80,
							Hostname: &hostname,
						},
					},
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

	for _, listener := range gateway.Status.Listeners {
		assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionAccepted), metav1.ConditionFalse, string(gatewayv1.ListenerReasonHostnameConflict), 1)
		assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionProgrammed), metav1.ConditionFalse, string(gatewayv1.ListenerReasonHostnameConflict), 1)
		assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionConflicted), metav1.ConditionTrue, string(gatewayv1.ListenerReasonHostnameConflict), 1)
		message := conditionMessage(t, listener.Conditions, string(gatewayv1.ListenerConditionAccepted))
		if message == "" {
			t.Fatalf("expected readable accepted message for listener %s", listener.Name)
		}
	}

	assertCondition(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionAccepted), metav1.ConditionFalse, string(gatewayv1.GatewayReasonListenersNotValid), 1)
	assertCondition(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionProgrammed), metav1.ConditionFalse, string(gatewayv1.GatewayReasonListenersNotValid), 1)
}

func TestReconcileAllowsHostnamePrecedenceListeners(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	mode := gatewayv1.TLSModeTerminate
	specificHostname := gatewayv1.Hostname("second-example.org")
	service := gatewayInfrastructureService("default", "gw")
	endpointSlice := gatewayInfrastructureEndpointSliceForService(service, managedresources.EndpointSliceRoleGatewayFrontend)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}, &gatewayv1.Gateway{}).
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
					Listeners: []gatewayv1.Listener{
						{
							Name:     "https",
							Protocol: gatewayv1.HTTPSProtocolType,
							Port:     443,
							TLS: &gatewayv1.ListenerTLSConfig{
								Mode: &mode,
								CertificateRefs: []gatewayv1.SecretObjectReference{{
									Name: "server-cert",
								}},
							},
						},
						{
							Name:     "https-with-hostname",
							Protocol: gatewayv1.HTTPSProtocolType,
							Port:     443,
							Hostname: &specificHostname,
							TLS: &gatewayv1.ListenerTLSConfig{
								Mode: &mode,
								CertificateRefs: []gatewayv1.SecretObjectReference{{
									Name: "server-cert",
								}},
							},
						},
					},
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "server-cert", Namespace: "default"},
				Type:       corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": []byte(readStatusTLSAsset(t, "client.crt")),
					"tls.key": []byte(readStatusTLSAsset(t, "client.key")),
				},
			},
			service,
			endpointSlice,
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

	for _, listener := range gateway.Status.Listeners {
		assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionAccepted), metav1.ConditionTrue, string(gatewayv1.ListenerReasonAccepted), 1)
		assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionResolvedRefs), metav1.ConditionTrue, string(gatewayv1.ListenerReasonResolvedRefs), 1)
		assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionProgrammed), metav1.ConditionTrue, string(gatewayv1.ListenerReasonProgrammed), 1)
		assertConditionAbsent(t, listener.Conditions, string(gatewayv1.ListenerConditionConflicted))
	}

	assertCondition(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionAccepted), metav1.ConditionTrue, string(gatewayv1.GatewayReasonAccepted), 1)
	assertCondition(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionProgrammed), metav1.ConditionTrue, string(gatewayv1.GatewayReasonProgrammed), 1)
}

func TestReconcileMarksProtocolConflictingListeners(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	mode := gatewayv1.TLSModeTerminate

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}, &gatewayv1.Gateway{}).
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
					Listeners: []gatewayv1.Listener{
						{
							Name:     "http",
							Protocol: gatewayv1.HTTPProtocolType,
							Port:     443,
						},
						{
							Name:     "https",
							Protocol: gatewayv1.HTTPSProtocolType,
							Port:     443,
							TLS: &gatewayv1.ListenerTLSConfig{
								Mode: &mode,
								CertificateRefs: []gatewayv1.SecretObjectReference{{
									Name: "server-cert",
								}},
							},
						},
					},
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "server-cert", Namespace: "default"},
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
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	if len(gateway.Status.Listeners) != 2 {
		t.Fatalf("expected 2 listener statuses, got %d", len(gateway.Status.Listeners))
	}

	for _, listener := range gateway.Status.Listeners {
		assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionAccepted), metav1.ConditionFalse, string(gatewayv1.ListenerReasonProtocolConflict), 1)
		assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionProgrammed), metav1.ConditionFalse, string(gatewayv1.ListenerReasonProtocolConflict), 1)
		assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionConflicted), metav1.ConditionTrue, string(gatewayv1.ListenerReasonProtocolConflict), 1)
	}

	assertCondition(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionAccepted), metav1.ConditionFalse, string(gatewayv1.GatewayReasonListenersNotValid), 1)
	assertCondition(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionProgrammed), metav1.ConditionFalse, string(gatewayv1.GatewayReasonListenersNotValid), 1)
}
