package status

import (
	"context"
	"errors"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/nantian-gw/gateway/internal/managedresources"
)

func TestReconcileGatewayObjectUsesReaderStateForObservedGeneration(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	freshGateway := gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 2},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "nantian-gw",
			Listeners: []gatewayv1.Listener{{
				Name:     "http",
				Protocol: gatewayv1.HTTPProtocolType,
				Port:     80,
			}},
		},
	}
	freshService := gatewayInfrastructureServiceForGateway(freshGateway)
	freshEndpointSlice := gatewayInfrastructureEndpointSliceForService(
		freshService,
		managedresources.EndpointSliceRoleGatewayFrontend,
	)

	staleClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.Gateway{}).
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
			freshService,
			freshEndpointSlice,
		).
		Build()

	freshReader := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gatewayv1.HTTPRoute{}, statusHTTPRouteGatewayParentIndex, statusHTTPRouteGatewayParentIndexKeys).
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
			&freshGateway,
			freshService,
			freshEndpointSlice,
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
	if err := reconciler.ReconcileGatewayObject(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}); err != nil {
		t.Fatalf("ReconcileGatewayObject returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := staleClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	assertCondition(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionAccepted), metav1.ConditionTrue, string(gatewayv1.GatewayReasonAccepted), 2)
	assertCondition(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionProgrammed), metav1.ConditionTrue, string(gatewayv1.GatewayReasonProgrammed), 2)
	if len(gateway.Status.Listeners) != 1 {
		t.Fatalf("expected 1 listener status, got %d", len(gateway.Status.Listeners))
	}
	if gateway.Status.Listeners[0].AttachedRoutes != 1 {
		t.Fatalf("expected attachedRoutes=1, got %d", gateway.Status.Listeners[0].AttachedRoutes)
	}
}

func TestReconcileGatewayObjectCountsDefaultedRoute(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	defaultGateway := gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 2},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "nantian-gw",
			DefaultScope:     gatewayv1.GatewayDefaultScopeAll,
			Listeners: []gatewayv1.Listener{{
				Name:     "http",
				Protocol: gatewayv1.HTTPProtocolType,
				Port:     80,
			}},
		},
	}
	service := gatewayInfrastructureServiceForGateway(defaultGateway)
	endpointSlice := gatewayInfrastructureEndpointSliceForService(
		service,
		managedresources.EndpointSliceRoleGatewayFrontend,
	)

	staleClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.Gateway{}).
		WithObjects(
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
			service,
			endpointSlice,
		).
		Build()

	freshReader := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gatewayv1.HTTPRoute{}, statusHTTPRouteGatewayParentIndex, statusHTTPRouteGatewayParentIndexKeys).
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
			&defaultGateway,
			service,
			endpointSlice,
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
	if err := reconciler.ReconcileGatewayObject(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}); err != nil {
		t.Fatalf("ReconcileGatewayObject returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := staleClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	assertCondition(t, gateway.Status.Conditions, "DefaultGateway", metav1.ConditionTrue, "DefaultGateway", 2)
	assertConditionMessage(t, gateway.Status.Conditions, "DefaultGateway", "Gateway has default scope All")
	if len(gateway.Status.Listeners) != 1 {
		t.Fatalf("expected 1 listener status, got %d", len(gateway.Status.Listeners))
	}
	if gateway.Status.Listeners[0].AttachedRoutes != 1 {
		t.Fatalf("expected attachedRoutes=1, got %d", gateway.Status.Listeners[0].AttachedRoutes)
	}
}

func TestReconcileGatewayObjectCountsListenerSetOnlyRoutes(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	parentGroup := gatewayv1.Group(gatewayv1.GroupName)
	listenerSetKind := gatewayv1.Kind("ListenerSet")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.Gateway{}, &gatewayv1.ListenerSet{}).
		WithIndex(&gatewayv1.HTTPRoute{}, statusHTTPRouteGatewayParentIndex, statusHTTPRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1.HTTPRoute{}, statusHTTPRouteListenerSetParentIndex, statusHTTPRouteListenerSetParentIndexKeys).
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

	reconciler := NewWithAddresses(k8sClient, string(controllerName), []string{"127.0.0.1"}, discardLogger())
	if err := reconciler.ReconcileGatewayObject(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}); err != nil {
		t.Fatalf("ReconcileGatewayObject returned error: %v", err)
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

func TestReconcileGatewayObjectRefreshesListenerSetStatusBeforeGatewayStatus(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.Gateway{}, &gatewayv1.ListenerSet{}).
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
				},
			},
		).
		Build()

	orderedClient := listenerSetBeforeGatewayStatusClient{Client: k8sClient}
	reconciler := NewWithAddressesAndReader(
		orderedClient,
		k8sClient,
		string(controllerName),
		[]string{"127.0.0.1"},
		discardLogger(),
	)
	if err := reconciler.ReconcileGatewayObject(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}); err != nil {
		t.Fatalf("ReconcileGatewayObject returned error: %v", err)
	}
}

type listenerSetBeforeGatewayStatusClient struct {
	client.Client
}

func (c listenerSetBeforeGatewayStatusClient) Status() client.SubResourceWriter {
	return listenerSetBeforeGatewayStatusWriter{
		SubResourceWriter: c.Client.Status(),
		reader:            c.Client,
	}
}

type listenerSetBeforeGatewayStatusWriter struct {
	client.SubResourceWriter
	reader client.Reader
}

func (w listenerSetBeforeGatewayStatusWriter) Update(
	ctx context.Context,
	obj client.Object,
	opts ...client.SubResourceUpdateOption,
) error {
	if _, ok := obj.(*gatewayv1.Gateway); ok {
		var listenerSet gatewayv1.ListenerSet
		if err := w.reader.Get(ctx, client.ObjectKey{Namespace: "default", Name: "ls"}, &listenerSet); err != nil {
			return err
		}
		if !conditionObservedGenerationCurrent(listenerSet.Status.Conditions, string(gatewayv1.ListenerSetConditionAccepted), listenerSet.Generation) ||
			!conditionObservedGenerationCurrent(listenerSet.Status.Conditions, string(gatewayv1.ListenerSetConditionProgrammed), listenerSet.Generation) {
			return errors.New("gateway status updated before ListenerSet status reached current observedGeneration")
		}
	}
	return w.SubResourceWriter.Update(ctx, obj, opts...)
}

func TestReconcileListenerSetObjectUsesReaderGenerationForStatus(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	staleClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.Gateway{}, &gatewayv1.ListenerSet{}).
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
			&gatewayv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "default"},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{Name: "gw"},
					Listeners: []gatewayv1.ListenerEntry{{
						Name:     "ls-listener",
						Port:     80,
						Protocol: gatewayv1.HTTPProtocolType,
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
			&gatewayv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "default", Generation: 1},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{Name: "gw"},
					Listeners: []gatewayv1.ListenerEntry{{
						Name:     "ls-listener",
						Port:     80,
						Protocol: gatewayv1.HTTPProtocolType,
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
	if err := reconciler.ReconcileListenerSetObject(context.Background(), client.ObjectKey{Namespace: "default", Name: "ls"}); err != nil {
		t.Fatalf("ReconcileListenerSetObject returned error: %v", err)
	}

	var listenerSet gatewayv1.ListenerSet
	if err := staleClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "ls"}, &listenerSet); err != nil {
		t.Fatalf("Get ListenerSet returned error: %v", err)
	}
	assertCondition(t, listenerSet.Status.Conditions, string(gatewayv1.ListenerSetConditionAccepted), metav1.ConditionTrue, string(gatewayv1.ListenerSetReasonAccepted), 1)
	assertCondition(t, listenerSet.Status.Conditions, string(gatewayv1.ListenerSetConditionProgrammed), metav1.ConditionTrue, string(gatewayv1.ListenerSetReasonProgrammed), 1)
	listener := listenerEntryStatusByName(t, listenerSet.Status.Listeners, "ls-listener")
	assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionAccepted), metav1.ConditionTrue, string(gatewayv1.ListenerReasonAccepted), 1)
	assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionProgrammed), metav1.ConditionTrue, string(gatewayv1.ListenerReasonProgrammed), 1)
}

func TestReconcileListenerSetObjectUpdatesListenerSetBeforeGatewayStatus(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	staleClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.Gateway{}, &gatewayv1.ListenerSet{}).
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
			&gatewayv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "default"},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{Name: "gw"},
					Listeners: []gatewayv1.ListenerEntry{{
						Name:     "ls-listener",
						Port:     80,
						Protocol: gatewayv1.HTTPProtocolType,
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
			&gatewayv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "default", Generation: 1},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{Name: "gw"},
					Listeners: []gatewayv1.ListenerEntry{{
						Name:     "ls-listener",
						Port:     80,
						Protocol: gatewayv1.HTTPProtocolType,
					}},
				},
			},
		).
		Build()

	reconciler := NewWithAddressesAndReader(
		listenerSetStatusOnlyGatewayClient{
			Client: staleClient,
		},
		freshReader,
		string(controllerName),
		[]string{"127.0.0.1"},
		discardLogger(),
	)
	if err := reconciler.ReconcileListenerSetObject(context.Background(), client.ObjectKey{Namespace: "default", Name: "ls"}); err != nil {
		t.Fatalf("ReconcileListenerSetObject returned error: %v", err)
	}

	var listenerSet gatewayv1.ListenerSet
	if err := staleClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "ls"}, &listenerSet); err != nil {
		t.Fatalf("Get ListenerSet returned error: %v", err)
	}
	assertCondition(t, listenerSet.Status.Conditions, string(gatewayv1.ListenerSetConditionAccepted), metav1.ConditionTrue, string(gatewayv1.ListenerSetReasonAccepted), 1)
	assertCondition(t, listenerSet.Status.Conditions, string(gatewayv1.ListenerSetConditionProgrammed), metav1.ConditionTrue, string(gatewayv1.ListenerSetReasonProgrammed), 1)
}

type listenerSetStatusOnlyGatewayClient struct {
	client.Client
}

func (c listenerSetStatusOnlyGatewayClient) Status() client.SubResourceWriter {
	return listenerSetStatusOnlyGatewayWriter{SubResourceWriter: c.Client.Status()}
}

type listenerSetStatusOnlyGatewayWriter struct {
	client.SubResourceWriter
}

func (w listenerSetStatusOnlyGatewayWriter) Update(
	ctx context.Context,
	obj client.Object,
	opts ...client.SubResourceUpdateOption,
) error {
	if _, ok := obj.(*gatewayv1.Gateway); ok {
		return errors.New("listener set reconcile should not need to update Gateway status")
	}
	return w.SubResourceWriter.Update(ctx, obj, opts...)
}

func TestReconcileListenerSetObjectRefreshesStatusWhenListCacheMissesListenerSet(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	parentGroup := gatewayv1.Group(gatewayv1.GroupName)
	listenerSetKind := gatewayv1.Kind("ListenerSet")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.Gateway{}, &gatewayv1.ListenerSet{}).
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

	reconciler := NewWithAddressesAndReader(
		k8sClient,
		listenerSetListHidingReader{Reader: k8sClient},
		string(controllerName),
		[]string{"127.0.0.1"},
		discardLogger(),
	)
	if err := reconciler.ReconcileListenerSetObject(context.Background(), client.ObjectKey{Namespace: "default", Name: "ls"}); err != nil {
		t.Fatalf("ReconcileListenerSetObject returned error: %v", err)
	}

	var listenerSet gatewayv1.ListenerSet
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "ls"}, &listenerSet); err != nil {
		t.Fatalf("Get ListenerSet returned error: %v", err)
	}
	assertCondition(t, listenerSet.Status.Conditions, string(gatewayv1.ListenerSetConditionAccepted), metav1.ConditionTrue, string(gatewayv1.ListenerSetReasonAccepted), 1)
	assertCondition(t, listenerSet.Status.Conditions, string(gatewayv1.ListenerSetConditionProgrammed), metav1.ConditionTrue, string(gatewayv1.ListenerSetReasonProgrammed), 1)
	listener := listenerEntryStatusByName(t, listenerSet.Status.Listeners, "ls-listener")
	if listener.AttachedRoutes != 1 {
		t.Fatalf("expected ListenerSet listener attachedRoutes=1, got %d", listener.AttachedRoutes)
	}
	assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionResolvedRefs), metav1.ConditionTrue, string(gatewayv1.ListenerReasonResolvedRefs), 1)
	assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionAccepted), metav1.ConditionTrue, string(gatewayv1.ListenerReasonAccepted), 1)
	assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionProgrammed), metav1.ConditionTrue, string(gatewayv1.ListenerReasonProgrammed), 1)
}

type listenerSetListHidingReader struct {
	client.Reader
}

func (r listenerSetListHidingReader) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	if listenerSets, ok := list.(*gatewayv1.ListenerSetList); ok {
		listenerSets.Items = nil
		return nil
	}
	return r.Reader.List(ctx, list, opts...)
}

func TestReconcileGatewayObjectAvoidsDuplicateGatewayReaderGets(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	freshGateway := gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 2},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "nantian-gw",
			Listeners: []gatewayv1.Listener{{
				Name:     "http",
				Protocol: gatewayv1.HTTPProtocolType,
				Port:     80,
			}},
		},
	}

	staleClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.Gateway{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec:       gatewayv1.GatewayClassSpec{ControllerName: controllerName},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
				Spec:       freshGateway.Spec,
			},
			gatewayInfrastructureServiceForGateway(freshGateway),
		).
		Build()

	baseReader := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gatewayv1.HTTPRoute{}, statusHTTPRouteGatewayParentIndex, statusHTTPRouteGatewayParentIndexKeys).
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
			&freshGateway,
			gatewayInfrastructureServiceForGateway(freshGateway),
		).
		Build()
	reader := &countingGetReader{Reader: baseReader}

	reconciler := NewWithAddressesAndReader(
		staleClient,
		reader,
		string(controllerName),
		[]string{"127.0.0.1"},
		discardLogger(),
	)
	if err := reconciler.ReconcileGatewayObject(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}); err != nil {
		t.Fatalf("ReconcileGatewayObject returned error: %v", err)
	}
	if reader.gatewayGets != 1 {
		t.Fatalf("gateway reader Get count = %d, want 1", reader.gatewayGets)
	}
}

func TestReconcileGatewayObjectWaitsForFrontendEndpointSliceConvergence(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	freshGateway := gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "gw",
			Namespace:  "default",
			Generation: 2,
			UID:        types.UID("gateway-object-waits-for-slices"),
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "nantian-gw",
			Listeners: []gatewayv1.Listener{{
				Name:     "http",
				Protocol: gatewayv1.HTTPProtocolType,
				Port:     80,
			}},
		},
	}

	staleClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.Gateway{}).
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
		).
		Build()

	freshReader := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gatewayv1.HTTPRoute{}, statusHTTPRouteGatewayParentIndex, statusHTTPRouteGatewayParentIndexKeys).
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
			&freshGateway,
			gatewayInfrastructureServiceForGateway(freshGateway),
			gatewayInfrastructureEndpointSlice("default", "gw", "shared-frontend-endpoints"),
		).
		Build()

	reconciler := NewWithAddressesAndReader(
		staleClient,
		freshReader,
		string(controllerName),
		[]string{"127.0.0.1"},
		discardLogger(),
	)
	if err := reconciler.ReconcileGatewayObject(
		context.Background(),
		client.ObjectKey{Namespace: "default", Name: "gw"},
	); err != nil {
		t.Fatalf("ReconcileGatewayObject returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := staleClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionProgrammed),
		metav1.ConditionFalse,
		string(gatewayv1.GatewayReasonPending),
		2,
	)
	assertConditionMessage(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionProgrammed),
		"Waiting for derived Gateway frontend EndpointSlices to converge",
	)
}

func TestReconcileGatewayObjectSkipsForeignGatewayClassAfterDirectGet(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	staleClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.Gateway{}).
		WithObjects(
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "foreign",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
		).
		Build()

	reader := &countingGetReader{Reader: restrictedReader{
		Reader: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(
				&gatewayv1.GatewayClass{
					ObjectMeta: metav1.ObjectMeta{Name: "foreign", Generation: 2},
					Spec: gatewayv1.GatewayClassSpec{
						ControllerName: gatewayv1.GatewayController("example.com/foreign"),
					},
				},
				&gatewayv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 2},
					Spec: gatewayv1.GatewaySpec{
						GatewayClassName: "foreign",
						Listeners: []gatewayv1.Listener{{
							Name:     "http",
							Protocol: gatewayv1.HTTPProtocolType,
							Port:     80,
						}},
					},
				},
			).
			Build(),
		blockedListTypes: map[reflect.Type]string{
			reflect.TypeOf(&gatewayv1.GatewayClassList{}):        "gateway object reconcile should direct-Get the referenced GatewayClass",
			reflect.TypeOf(&gatewayv1.GatewayList{}):             "gateway object reconcile should read the target Gateway directly",
			reflect.TypeOf(&gatewayv1.HTTPRouteList{}):           "unmanaged Gateway should return before loading routes",
			reflect.TypeOf(&gatewayv1.GRPCRouteList{}):           "unmanaged Gateway should return before loading routes",
			reflect.TypeOf(&gatewayv1alpha2.TCPRouteList{}):      "unmanaged Gateway should return before loading routes",
			reflect.TypeOf(&gatewayv1alpha2.UDPRouteList{}):      "unmanaged Gateway should return before loading routes",
			reflect.TypeOf(&gatewayv1alpha2.TLSRouteList{}):      "unmanaged Gateway should return before loading routes",
			reflect.TypeOf(&gatewayv1beta1.ReferenceGrantList{}): "unmanaged Gateway should return before loading reference grants",
			reflect.TypeOf(&corev1.ServiceList{}):                "unmanaged Gateway should return before loading Services",
			reflect.TypeOf(&discoveryv1.EndpointSliceList{}):     "unmanaged Gateway should return before loading EndpointSlices",
			reflect.TypeOf(&corev1.NamespaceList{}):              "unmanaged Gateway should return before loading Namespaces",
			reflect.TypeOf(&corev1.ConfigMapList{}):              "unmanaged Gateway should return before loading ConfigMaps",
			reflect.TypeOf(&corev1.SecretList{}):                 "unmanaged Gateway should return before loading Secrets",
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
			reflect.TypeOf(&gatewayv1.GatewayClassList{}): "gateway object reconcile should not list GatewayClasses from listReader",
		},
	}

	if err := reconciler.ReconcileGatewayObject(
		context.Background(),
		client.ObjectKey{Namespace: "default", Name: "gw"},
	); err != nil {
		t.Fatalf("ReconcileGatewayObject returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := staleClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	if len(gateway.Status.Conditions) != 0 {
		t.Fatalf("expected unmanaged Gateway status conditions to stay empty, got %#v", gateway.Status.Conditions)
	}
	if len(gateway.Status.Listeners) != 0 {
		t.Fatalf("expected unmanaged Gateway listener status to stay empty, got %#v", gateway.Status.Listeners)
	}
	if reader.gatewayClassGets != 1 {
		t.Fatalf("GatewayClass reader Get count = %d, want 1", reader.gatewayClassGets)
	}
}
