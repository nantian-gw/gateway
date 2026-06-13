package status

import (
	"context"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

func TestReconcileHTTPRouteObjectUsesReaderStateForObservedGeneration(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	staleClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.HTTPRoute{}).
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
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			gatewayInfrastructureService("default", "gw"),
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8080}},
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

	freshReader := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gatewayv1.HTTPRoute{}, statusHTTPRouteGatewayParentIndex, statusHTTPRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1.HTTPRoute{}, statusHTTPRouteListenerSetParentIndex, statusHTTPRouteListenerSetParentIndexKeys).
		WithIndex(&gatewayv1.GRPCRoute{}, statusGRPCRouteGatewayParentIndex, statusGRPCRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1alpha2.TCPRoute{}, statusTCPRouteGatewayParentIndex, statusTCPRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1alpha2.UDPRoute{}, statusUDPRouteGatewayParentIndex, statusUDPRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1alpha2.TLSRoute{}, statusTLSRouteGatewayParentIndex, statusTLSRouteGatewayParentIndexKeys).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 2},
				Spec:       gatewayv1.GatewayClassSpec{ControllerName: controllerName},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 2},
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
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8080}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default", Generation: 2},
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

	reconciler := NewWithAddressesAndReader(
		staleClient,
		freshReader,
		string(controllerName),
		[]string{"127.0.0.1"},
		discardLogger(),
	)
	if err := reconciler.ReconcileHTTPRouteObject(
		context.Background(),
		client.ObjectKey{Namespace: "default", Name: "route"},
	); err != nil {
		t.Fatalf("ReconcileHTTPRouteObject returned error: %v", err)
	}

	var route gatewayv1.HTTPRoute
	if err := staleClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "route"}, &route); err != nil {
		t.Fatalf("Get HTTPRoute returned error: %v", err)
	}
	if len(route.Status.Parents) != 1 {
		t.Fatalf("expected 1 parent status, got %d", len(route.Status.Parents))
	}
	assertCondition(t, route.Status.Parents[0].Conditions, string(gatewayv1.RouteConditionAccepted), metav1.ConditionTrue, string(gatewayv1.RouteReasonAccepted), 2)
	assertCondition(t, route.Status.Parents[0].Conditions, string(gatewayv1.RouteConditionResolvedRefs), metav1.ConditionTrue, string(gatewayv1.RouteReasonResolvedRefs), 2)
}

func TestReconcileHTTPRouteObjectBindsDefaultGateway(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	staleClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.HTTPRoute{}).
		WithObjects(&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default", Generation: 1},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					UseDefaultGateways: gatewayv1.GatewayDefaultScopeAll,
				},
			},
		}).
		Build()

	freshReader := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gatewayv1.HTTPRoute{}, statusHTTPRouteGatewayParentIndex, statusHTTPRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1.HTTPRoute{}, statusHTTPRouteListenerSetParentIndex, statusHTTPRouteListenerSetParentIndexKeys).
		WithIndex(&gatewayv1.GRPCRoute{}, statusGRPCRouteGatewayParentIndex, statusGRPCRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1alpha2.TCPRoute{}, statusTCPRouteGatewayParentIndex, statusTCPRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1alpha2.UDPRoute{}, statusUDPRouteGatewayParentIndex, statusUDPRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1alpha2.TLSRoute{}, statusTLSRouteGatewayParentIndex, statusTLSRouteGatewayParentIndexKeys).
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
					DefaultScope:     gatewayv1.GatewayDefaultScopeAll,
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8080}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default", Generation: 2},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						UseDefaultGateways: gatewayv1.GatewayDefaultScopeAll,
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

	reconciler := NewWithAddressesAndReader(
		staleClient,
		freshReader,
		string(controllerName),
		[]string{"127.0.0.1"},
		discardLogger(),
	)
	if err := reconciler.ReconcileHTTPRouteObject(
		context.Background(),
		client.ObjectKey{Namespace: "default", Name: "route"},
	); err != nil {
		t.Fatalf("ReconcileHTTPRouteObject returned error: %v", err)
	}

	var route gatewayv1.HTTPRoute
	if err := staleClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "route"}, &route); err != nil {
		t.Fatalf("Get HTTPRoute returned error: %v", err)
	}
	if len(route.Status.Parents) != 1 {
		t.Fatalf("expected 1 parent status, got %d: %#v", len(route.Status.Parents), route.Status.Parents)
	}
	parent := route.Status.Parents[0]
	if parent.ParentRef.Name != "gw" {
		t.Fatalf("parentRef.name = %q, want gw", parent.ParentRef.Name)
	}
	assertCondition(t, parent.Conditions, string(gatewayv1.RouteConditionAccepted), metav1.ConditionTrue, string(gatewayv1.RouteReasonAccepted), 2)
	assertCondition(t, parent.Conditions, string(gatewayv1.RouteConditionResolvedRefs), metav1.ConditionTrue, string(gatewayv1.RouteReasonResolvedRefs), 2)
}

func TestReconcileHTTPRouteObjectKeepsListenerSetParentStatuses(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	parentGroup := gatewayv1.Group(gatewayv1.GroupName)
	listenerSetKind := gatewayv1.Kind("ListenerSet")

	staleClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.HTTPRoute{}).
		WithObjects(&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default", Generation: 1},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{
						{Name: "gw"},
						{Group: &parentGroup, Kind: &listenerSetKind, Name: "ls-1"},
						{Group: &parentGroup, Kind: &listenerSetKind, Name: "ls-2"},
					},
				},
			},
		}).
		Build()

	freshReader := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gatewayv1.HTTPRoute{}, statusHTTPRouteGatewayParentIndex, statusHTTPRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1.HTTPRoute{}, statusHTTPRouteListenerSetParentIndex, statusHTTPRouteListenerSetParentIndexKeys).
		WithIndex(&gatewayv1.GRPCRoute{}, statusGRPCRouteGatewayParentIndex, statusGRPCRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1alpha2.TCPRoute{}, statusTCPRouteGatewayParentIndex, statusTCPRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1alpha2.UDPRoute{}, statusUDPRouteGatewayParentIndex, statusUDPRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1alpha2.TLSRoute{}, statusTLSRouteGatewayParentIndex, statusTLSRouteGatewayParentIndexKeys).
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
					Listeners: []gatewayv1.Listener{{
						Name:     "gateway-listener",
						Port:     80,
						Protocol: gatewayv1.HTTPProtocolType,
						AllowedRoutes: &gatewayv1.AllowedRoutes{
							Namespaces: &gatewayv1.RouteNamespaces{From: namespaceFromPtr(gatewayv1.NamespacesFromAll)},
						},
					}},
					AllowedListeners: &gatewayv1.AllowedListeners{
						Namespaces: &gatewayv1.ListenerNamespaces{From: namespaceFromPtr(gatewayv1.NamespacesFromAll)},
					},
				},
			},
			&gatewayv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{Name: "ls-1", Namespace: "default", Generation: 1},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{Name: "gw"},
					Listeners: []gatewayv1.ListenerEntry{{
						Name:     "ls-1-listener",
						Port:     80,
						Protocol: gatewayv1.HTTPProtocolType,
						AllowedRoutes: &gatewayv1.AllowedRoutes{
							Namespaces: &gatewayv1.RouteNamespaces{From: namespaceFromPtr(gatewayv1.NamespacesFromAll)},
						},
					}},
				},
			},
			&gatewayv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{Name: "ls-2", Namespace: "default", Generation: 1},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{Name: "gw"},
					Listeners: []gatewayv1.ListenerEntry{{
						Name:     "ls-2-listener",
						Port:     81,
						Protocol: gatewayv1.HTTPProtocolType,
						AllowedRoutes: &gatewayv1.AllowedRoutes{
							Namespaces: &gatewayv1.RouteNamespaces{From: namespaceFromPtr(gatewayv1.NamespacesFromAll)},
						},
					}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default", Generation: 2},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{
							{Name: "gw"},
							{Group: &parentGroup, Kind: &listenerSetKind, Name: "ls-1"},
							{Group: &parentGroup, Kind: &listenerSetKind, Name: "ls-2"},
						},
					},
				},
			},
		).
		Build()

	reconciler := NewWithAddressesAndReader(
		staleClient,
		freshReader,
		string(controllerName),
		[]string{"127.0.0.1"},
		discardLogger(),
	)
	if err := reconciler.ReconcileHTTPRouteObject(
		context.Background(),
		client.ObjectKey{Namespace: "default", Name: "route"},
	); err != nil {
		t.Fatalf("ReconcileHTTPRouteObject returned error: %v", err)
	}

	var route gatewayv1.HTTPRoute
	if err := staleClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "route"}, &route); err != nil {
		t.Fatalf("Get HTTPRoute returned error: %v", err)
	}
	if len(route.Status.Parents) != 3 {
		t.Fatalf("expected 3 parent statuses, got %d: %#v", len(route.Status.Parents), route.Status.Parents)
	}
	for _, want := range []gatewayv1.ObjectName{"gw", "ls-1", "ls-2"} {
		parent := routeParentStatusByName(t, route.Status.Parents, want)
		assertCondition(t, parent.Conditions, string(gatewayv1.RouteConditionAccepted), metav1.ConditionTrue, string(gatewayv1.RouteReasonAccepted), 2)
		assertCondition(t, parent.Conditions, string(gatewayv1.RouteConditionResolvedRefs), metav1.ConditionTrue, string(gatewayv1.RouteReasonResolvedRefs), 2)
	}
}

func TestReconcileHTTPRouteObjectRefreshesListenerSetParentStatus(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	parentGroup := gatewayv1.Group(gatewayv1.GroupName)
	listenerSetKind := gatewayv1.Kind("ListenerSet")

	staleClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.Gateway{}, &gatewayv1.HTTPRoute{}, &gatewayv1.ListenerSet{}).
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
						Namespaces: &gatewayv1.ListenerNamespaces{From: namespaceFromPtr(gatewayv1.NamespacesFromAll)},
					},
				},
			},
			gatewayInfrastructureService("default", "gw"),
			&gatewayv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "default", Generation: 1},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{Name: "gw"},
					Listeners: []gatewayv1.ListenerEntry{{
						Name:     "ls-listener",
						Port:     80,
						Protocol: gatewayv1.HTTPProtocolType,
						AllowedRoutes: &gatewayv1.AllowedRoutes{
							Namespaces: &gatewayv1.RouteNamespaces{From: namespaceFromPtr(gatewayv1.NamespacesFromAll)},
						},
					}},
				},
				Status: gatewayv1.ListenerSetStatus{
					Conditions: []metav1.Condition{
						{
							Type:               string(gatewayv1.ListenerSetConditionAccepted),
							Status:             metav1.ConditionTrue,
							Reason:             string(gatewayv1.ListenerSetReasonAccepted),
							ObservedGeneration: 0,
						},
						{
							Type:               string(gatewayv1.ListenerSetConditionProgrammed),
							Status:             metav1.ConditionTrue,
							Reason:             string(gatewayv1.ListenerSetReasonProgrammed),
							ObservedGeneration: 0,
						},
					},
					Listeners: []gatewayv1.ListenerEntryStatus{{
						Name:           "ls-listener",
						AttachedRoutes: 0,
						Conditions: []metav1.Condition{
							{
								Type:               string(gatewayv1.ListenerConditionAccepted),
								Status:             metav1.ConditionTrue,
								Reason:             string(gatewayv1.ListenerReasonAccepted),
								ObservedGeneration: 0,
							},
							{
								Type:               string(gatewayv1.ListenerConditionProgrammed),
								Status:             metav1.ConditionTrue,
								Reason:             string(gatewayv1.ListenerReasonProgrammed),
								ObservedGeneration: 0,
							},
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
						ParentRefs: []gatewayv1.ParentReference{{
							Group: &parentGroup,
							Kind:  &listenerSetKind,
							Name:  "ls",
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

	freshReader := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gatewayv1.HTTPRoute{}, statusHTTPRouteGatewayParentIndex, statusHTTPRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1.HTTPRoute{}, statusHTTPRouteListenerSetParentIndex, statusHTTPRouteListenerSetParentIndexKeys).
		WithIndex(&gatewayv1.GRPCRoute{}, statusGRPCRouteGatewayParentIndex, statusGRPCRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1alpha2.TCPRoute{}, statusTCPRouteGatewayParentIndex, statusTCPRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1alpha2.UDPRoute{}, statusUDPRouteGatewayParentIndex, statusUDPRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1alpha2.TLSRoute{}, statusTLSRouteGatewayParentIndex, statusTLSRouteGatewayParentIndexKeys).
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
						Namespaces: &gatewayv1.ListenerNamespaces{From: namespaceFromPtr(gatewayv1.NamespacesFromAll)},
					},
				},
			},
			gatewayInfrastructureService("default", "gw"),
			&gatewayv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "default", Generation: 1},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{Name: "gw"},
					Listeners: []gatewayv1.ListenerEntry{{
						Name:     "ls-listener",
						Port:     80,
						Protocol: gatewayv1.HTTPProtocolType,
						AllowedRoutes: &gatewayv1.AllowedRoutes{
							Namespaces: &gatewayv1.RouteNamespaces{From: namespaceFromPtr(gatewayv1.NamespacesFromAll)},
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
						ParentRefs: []gatewayv1.ParentReference{{
							Group: &parentGroup,
							Kind:  &listenerSetKind,
							Name:  "ls",
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

	reconciler := NewWithAddressesAndReader(
		staleClient,
		freshReader,
		string(controllerName),
		[]string{"127.0.0.1"},
		discardLogger(),
	)
	if err := reconciler.ReconcileHTTPRouteObject(
		context.Background(),
		client.ObjectKey{Namespace: "default", Name: "route"},
	); err != nil {
		t.Fatalf("ReconcileHTTPRouteObject returned error: %v", err)
	}

	var route gatewayv1.HTTPRoute
	if err := staleClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "route"}, &route); err != nil {
		t.Fatalf("Get HTTPRoute returned error: %v", err)
	}
	parent := routeParentStatusByName(t, route.Status.Parents, "ls")
	assertCondition(t, parent.Conditions, string(gatewayv1.RouteConditionAccepted), metav1.ConditionTrue, string(gatewayv1.RouteReasonAccepted), 1)
	assertCondition(t, parent.Conditions, string(gatewayv1.RouteConditionResolvedRefs), metav1.ConditionTrue, string(gatewayv1.RouteReasonResolvedRefs), 1)

	var listenerSet gatewayv1.ListenerSet
	if err := staleClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "ls"}, &listenerSet); err != nil {
		t.Fatalf("Get ListenerSet returned error: %v", err)
	}
	assertCondition(t, listenerSet.Status.Conditions, string(gatewayv1.ListenerSetConditionAccepted), metav1.ConditionTrue, string(gatewayv1.ListenerSetReasonAccepted), 1)
	assertCondition(t, listenerSet.Status.Conditions, string(gatewayv1.ListenerSetConditionProgrammed), metav1.ConditionTrue, string(gatewayv1.ListenerSetReasonProgrammed), 1)
	listener := listenerEntryStatusByName(t, listenerSet.Status.Listeners, "ls-listener")
	if listener.AttachedRoutes != 1 {
		t.Fatalf("expected ListenerSet listener attachedRoutes=1, got %d", listener.AttachedRoutes)
	}
	assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionAccepted), metav1.ConditionTrue, string(gatewayv1.ListenerReasonAccepted), 1)
	assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionProgrammed), metav1.ConditionTrue, string(gatewayv1.ListenerReasonProgrammed), 1)
}

func TestReconcileHTTPRouteObjectAvoidsFullStateLists(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	staleClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gatewayv1.GatewayClass{}, statusGatewayClassControllerNameIndex, func(object client.Object) []string {
			gatewayClass, ok := object.(*gatewayv1.GatewayClass)
			if !ok || gatewayClass.Spec.ControllerName == "" {
				return nil
			}
			return []string{string(gatewayClass.Spec.ControllerName)}
		}).
		WithStatusSubresource(&gatewayv1.HTTPRoute{}).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec:       gatewayv1.GatewayClassSpec{ControllerName: controllerName},
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

	freshReader := fake.NewClientBuilder().
		WithScheme(scheme).
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
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			gatewayInfrastructureService("default", "gw"),
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8080}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default", Generation: 2},
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

	reader := &countingGetReader{Reader: restrictedReader{
		Reader: freshReader,
		blockedListTypes: map[reflect.Type]string{
			reflect.TypeOf(&gatewayv1.GatewayClassList{}):        "route object reconcile should direct-Get referenced GatewayClasses",
			reflect.TypeOf(&gatewayv1.GatewayList{}):             "route object reconcile should fetch referenced parents directly",
			reflect.TypeOf(&gatewayv1.HTTPRouteList{}):           "route object reconcile should not reload all HTTPRoutes",
			reflect.TypeOf(&gatewayv1.GRPCRouteList{}):           "route object reconcile should not reload all GRPCRoutes",
			reflect.TypeOf(&gatewayv1alpha2.TCPRouteList{}):      "route object reconcile should not reload all TCPRoutes",
			reflect.TypeOf(&gatewayv1alpha2.UDPRouteList{}):      "route object reconcile should not reload all UDPRoutes",
			reflect.TypeOf(&gatewayv1alpha2.TLSRouteList{}):      "route object reconcile should not reload all TLSRoutes",
			reflect.TypeOf(&gatewayv1beta1.ReferenceGrantList{}): "route object reconcile should not load ReferenceGrants when all backend refs stay in-namespace",
			reflect.TypeOf(&corev1.ServiceList{}):                "route object reconcile should fetch referenced Services directly",
			reflect.TypeOf(&mcsv1alpha1.ServiceImportList{}):     "route object reconcile should fetch referenced ServiceImports directly",
			reflect.TypeOf(&corev1.NamespaceList{}):              "route object reconcile should fetch the route Namespace directly",
			reflect.TypeOf(&corev1.ConfigMapList{}):              "route object reconcile should fetch ExtensionRef ConfigMaps directly",
			reflect.TypeOf(&corev1.SecretList{}):                 "route object reconcile should not need Secrets",
		},
	}}

	reconciler := NewWithAddressesAndReader(
		staleClient,
		reader,
		string(controllerName),
		[]string{"127.0.0.1"},
		discardLogger(),
	)
	reconciler.listReader = restrictedReader{
		Reader: staleClient,
		blockedListTypes: map[reflect.Type]string{
			reflect.TypeOf(&gatewayv1.GatewayClassList{}): "route object reconcile should not list GatewayClasses from listReader",
		},
	}
	if err := reconciler.ReconcileHTTPRouteObject(
		context.Background(),
		client.ObjectKey{Namespace: "default", Name: "route"},
	); err != nil {
		t.Fatalf("ReconcileHTTPRouteObject returned error: %v", err)
	}
	if reader.gatewayClassGets != 1 {
		t.Fatalf("GatewayClass reader Get count = %d, want 1", reader.gatewayClassGets)
	}
}

func routeParentStatusByName(
	t *testing.T,
	parents []gatewayv1.RouteParentStatus,
	name gatewayv1.ObjectName,
) gatewayv1.RouteParentStatus {
	t.Helper()
	for _, parent := range parents {
		if parent.ParentRef.Name == name {
			return parent
		}
	}
	t.Fatalf("parent status for %q not found in %#v", name, parents)
	return gatewayv1.RouteParentStatus{}
}

func TestReconcileHTTPRouteObjectScopesReferenceGrantListsToTargetNamespace(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	staleClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gatewayv1.GatewayClass{}, statusGatewayClassControllerNameIndex, func(object client.Object) []string {
			gatewayClass, ok := object.(*gatewayv1.GatewayClass)
			if !ok || gatewayClass.Spec.ControllerName == "" {
				return nil
			}
			return []string{string(gatewayClass.Spec.ControllerName)}
		}).
		WithStatusSubresource(&gatewayv1.HTTPRoute{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "shared"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec:       gatewayv1.GatewayClassSpec{ControllerName: controllerName},
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
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "shared"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8080}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default", Generation: 2},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{Name: "gw"}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name:      "echo",
									Namespace: namespacePtr("shared"),
									Port:      portPtr(8080),
								},
							},
						}},
					}},
				},
			},
			&gatewayv1beta1.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{Name: "allow-default-route", Namespace: "shared"},
				Spec: gatewayv1beta1.ReferenceGrantSpec{
					From: []gatewayv1beta1.ReferenceGrantFrom{{
						Group:     gatewayv1beta1.Group(gatewayGroup),
						Kind:      gatewayv1beta1.Kind("HTTPRoute"),
						Namespace: gatewayv1beta1.Namespace("default"),
					}},
					To: []gatewayv1beta1.ReferenceGrantTo{{
						Group: "",
						Kind:  "Service",
						Name:  ptr(gatewayv1beta1.ObjectName("echo")),
					}},
				},
			},
		).
		Build()

	freshReader := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "shared"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec:       gatewayv1.GatewayClassSpec{ControllerName: controllerName},
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
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "shared"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8080}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default", Generation: 2},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{Name: "gw"}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name:      "echo",
									Namespace: namespacePtr("shared"),
									Port:      portPtr(8080),
								},
							},
						}},
					}},
				},
			},
			&gatewayv1beta1.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{Name: "allow-default-route", Namespace: "shared"},
				Spec: gatewayv1beta1.ReferenceGrantSpec{
					From: []gatewayv1beta1.ReferenceGrantFrom{{
						Group:     gatewayv1beta1.Group(gatewayGroup),
						Kind:      gatewayv1beta1.Kind("HTTPRoute"),
						Namespace: gatewayv1beta1.Namespace("default"),
					}},
					To: []gatewayv1beta1.ReferenceGrantTo{{
						Group: "",
						Kind:  "Service",
						Name:  ptr(gatewayv1beta1.ObjectName("echo")),
					}},
				},
			},
		).
		Build()

	reader := &countingGetReader{Reader: restrictedReader{
		Reader: freshReader,
		blockedListTypes: map[reflect.Type]string{
			reflect.TypeOf(&gatewayv1.GatewayClassList{}):    "route object reconcile should direct-Get referenced GatewayClasses",
			reflect.TypeOf(&gatewayv1.GatewayList{}):         "route object reconcile should fetch referenced parents directly",
			reflect.TypeOf(&gatewayv1.HTTPRouteList{}):       "route object reconcile should not reload all HTTPRoutes",
			reflect.TypeOf(&gatewayv1.GRPCRouteList{}):       "route object reconcile should not reload all GRPCRoutes",
			reflect.TypeOf(&gatewayv1alpha2.TCPRouteList{}):  "route object reconcile should not reload all TCPRoutes",
			reflect.TypeOf(&gatewayv1alpha2.UDPRouteList{}):  "route object reconcile should not reload all UDPRoutes",
			reflect.TypeOf(&gatewayv1alpha2.TLSRouteList{}):  "route object reconcile should not reload all TLSRoutes",
			reflect.TypeOf(&corev1.ServiceList{}):            "route object reconcile should fetch referenced Services directly",
			reflect.TypeOf(&mcsv1alpha1.ServiceImportList{}): "route object reconcile should fetch referenced ServiceImports directly",
			reflect.TypeOf(&corev1.NamespaceList{}):          "route object reconcile should fetch the route Namespace directly",
			reflect.TypeOf(&corev1.ConfigMapList{}):          "route object reconcile should fetch ExtensionRef ConfigMaps directly",
			reflect.TypeOf(&corev1.SecretList{}):             "route object reconcile should not need Secrets",
		},
	}}

	reconciler := NewWithAddressesAndReader(
		staleClient,
		reader,
		string(controllerName),
		[]string{"127.0.0.1"},
		discardLogger(),
	)
	reconciler.listReader = rawValidatingReader{
		Reader: restrictedReader{
			Reader: staleClient,
			blockedListTypes: map[reflect.Type]string{
				reflect.TypeOf(&gatewayv1.GatewayClassList{}): "route object reconcile should not list GatewayClasses from listReader",
			},
		},
		listValidators: map[reflect.Type]func([]client.ListOption) error{
			reflect.TypeOf(&gatewayv1beta1.ReferenceGrantList{}): func(opts []client.ListOption) error {
				return requireNamespaceOption(opts, "shared")
			},
		},
	}
	if err := reconciler.ReconcileHTTPRouteObject(
		context.Background(),
		client.ObjectKey{Namespace: "default", Name: "route"},
	); err != nil {
		t.Fatalf("ReconcileHTTPRouteObject returned error: %v", err)
	}
	if reader.gatewayClassGets != 1 {
		t.Fatalf("GatewayClass reader Get count = %d, want 1", reader.gatewayClassGets)
	}
}
