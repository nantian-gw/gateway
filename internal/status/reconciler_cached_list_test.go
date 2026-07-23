package status

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	backend "github.com/nantian-gw/gateway/internal/gatewayexp/backend"
	"github.com/nantian-gw/gateway/internal/resources"
)

const (
	testStatusGatewayClassControllerNameIndex = "nantian.dev/infrastructure.gatewayclass.controller-name"
	testStatusGatewayGatewayClassNameIndex    = "nantian.dev/infrastructure.gateway.gatewayclass-name"
)

func TestReconcileUsesClientForBulkStateLists(t *testing.T) {
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

	reconciler := NewWithAddressesAndReader(
		k8sClient,
		restrictedReader{
			Reader:           k8sClient,
			blockedListTypes: blockedStatusListTypesForFullReconcile(),
		},
		string(controllerName),
		[]string{"127.0.0.1"},
		discardLogger(),
	)
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}
}

func TestReconcileStandardModeSkipsExperimentalGatewayAPILists(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
			&gatewayv1.HTTPRoute{},
			&gatewayv1.GRPCRoute{},
		).
		WithObjects(
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
		).
		Build()

	reconciler := NewWithAddressesAndReaderOptions(
		k8sClient,
		restrictedReader{
			Reader: k8sClient,
			blockedListTypes: map[reflect.Type]string{
				reflect.TypeOf(&gatewayv1alpha2.TCPRouteList{}): "standard mode should not list TCPRoutes",
				reflect.TypeOf(&gatewayv1alpha2.UDPRouteList{}): "standard mode should not list UDPRoutes",
				reflect.TypeOf(&gatewayv1alpha2.TLSRouteList{}): "standard mode should not list TLSRoutes",
				reflect.TypeOf(&gatewayv1.ListenerSetList{}):    "standard mode should not list ListenerSets",
			},
		},
		string(controllerName),
		[]string{"127.0.0.1"},
		discardLogger(),
		Options{EnableExperimentalGateway: false},
	)
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}
}

func TestReconcileScopesManagedGatewayListsWithIndexes(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gatewayv1.GatewayClass{}, testStatusGatewayClassControllerNameIndex, func(object client.Object) []string {
			gatewayClass, ok := object.(*gatewayv1.GatewayClass)
			if !ok || gatewayClass.Spec.ControllerName == "" {
				return nil
			}
			return []string{string(gatewayClass.Spec.ControllerName)}
		}).
		WithIndex(&gatewayv1.Gateway{}, testStatusGatewayGatewayClassNameIndex, func(object client.Object) []string {
			gateway, ok := object.(*gatewayv1.Gateway)
			if !ok || gateway.Spec.GatewayClassName == "" {
				return nil
			}
			return []string{string(gateway.Spec.GatewayClassName)}
		}).
		WithStatusSubresource(&gatewayv1.GatewayClass{}, &gatewayv1.Gateway{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "foreign", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: gatewayv1.GatewayController("example.com/other"),
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
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "foreign", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "foreign",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     8080,
					}},
				},
			},
		).
		Build()

	reconciler := NewWithAddressesAndReader(
		k8sClient,
		restrictedReader{
			Reader: k8sClient,
			blockedListTypes: map[reflect.Type]string{
				reflect.TypeOf(&gatewayv1.GatewayClassList{}): "full reconcile should not use the object reader for GatewayClass list scans",
				reflect.TypeOf(&gatewayv1.GatewayList{}):      "full reconcile should not use the object reader for Gateway list scans",
			},
		},
		string(controllerName),
		[]string{"127.0.0.1"},
		discardLogger(),
	)
	reconciler.listReader = validatingListReader{
		Reader: k8sClient,
		listValidators: map[reflect.Type]func(client.ListOptions) error{
			reflect.TypeOf(&gatewayv1.GatewayClassList{}): requireGatewayClassControllerList(string(controllerName)),
			reflect.TypeOf(&gatewayv1.GatewayList{}):      requireGatewayClassNameList("nantian-gw"),
		},
	}

	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	assertCondition(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionAccepted), metav1.ConditionTrue, string(gatewayv1.GatewayReasonAccepted), 1)

	var foreign gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "foreign"}, &foreign); err != nil {
		t.Fatalf("Get foreign Gateway returned error: %v", err)
	}
	if len(foreign.Status.Conditions) != 0 {
		t.Fatalf("expected unmanaged Gateway status to stay empty, got %#v", foreign.Status.Conditions)
	}
}

func TestReconcileRefreshesManagedGatewayGenerationsWithNamespaceScopedMetadataLists(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	staleClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}, &gatewayv1.Gateway{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "team-b"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "foreign", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: gatewayv1.GatewayController("example.com/other"),
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
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw-b", Namespace: "team-b", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     8081,
					}},
				},
			},
			gatewayInfrastructureService("team-b", "gw-b"),
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "foreign", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "foreign",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     8080,
					}},
				},
			},
		).
		Build()

	freshReaderClient := fake.NewClientBuilder().
		WithScheme(scheme).
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
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw-b", Namespace: "team-b", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     8081,
					}},
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "foreign", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "foreign",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     8080,
					}},
				},
			},
		).
		Build()

	seenNamespaces := make(map[string]int)
	reader := &countingGetReader{
		Reader: rawValidatingReader{
			Reader: freshReaderClient,
			listValidators: map[reflect.Type]func([]client.ListOption) error{
				reflect.TypeOf(&metav1.PartialObjectMetadataList{}): func(opts []client.ListOption) error {
					var listOpts client.ListOptions
					for _, opt := range opts {
						opt.ApplyToList(&listOpts)
					}
					if listOpts.Namespace == "" {
						return fmt.Errorf("Gateway metadata refresh must be namespace-scoped")
					}
					switch listOpts.Namespace {
					case "default", "team-b":
					default:
						return fmt.Errorf(
							"Gateway metadata refresh namespace = %q, want one of default/team-b",
							listOpts.Namespace,
						)
					}
					seenNamespaces[listOpts.Namespace]++
					return nil
				},
				reflect.TypeOf(&gatewayv1.GatewayList{}): func([]client.ListOption) error {
					return fmt.Errorf("full reconcile should not use the object reader for Gateway generation list scans")
				},
			},
		},
	}

	reconciler := NewWithAddressesAndReader(
		staleClient,
		reader,
		string(controllerName),
		[]string{"127.0.0.1"},
		discardLogger(),
	)
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	if reader.gatewayGets != 0 {
		t.Fatalf("gateway Get count = %d, want 0 when managed Gateway generation is current", reader.gatewayGets)
	}
	if seenNamespaces["default"] != 1 {
		t.Fatalf("default namespace metadata list count = %d, want 1", seenNamespaces["default"])
	}
	if seenNamespaces["team-b"] != 1 {
		t.Fatalf("team-b namespace metadata list count = %d, want 1", seenNamespaces["team-b"])
	}
}

func TestReconcileSkipsGatewayMetadataRefreshForFullyConvergedGateways(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	currentGateway := gatewayWithNameAndGenerationForConvergenceTest("gw", 1)
	service, slice := benchmarkGatewayInfrastructureObjects(*currentGateway)
	currentGateway.Status.Addresses = []gatewayv1.GatewayStatusAddress{{
		Type:  ptr(gatewayv1.IPAddressType),
		Value: "127.0.0.1",
	}}
	setCondition(&currentGateway.Status.Conditions, conditionSpec{
		Type:               string(gatewayv1.GatewayConditionAccepted),
		Status:             metav1.ConditionTrue,
		Reason:             string(gatewayv1.GatewayReasonAccepted),
		Message:            "Gateway is accepted by nantian-gw",
		ObservedGeneration: currentGateway.Generation,
	})
	setCondition(&currentGateway.Status.Conditions, conditionSpec{
		Type:               string(gatewayv1.GatewayConditionProgrammed),
		Status:             metav1.ConditionTrue,
		Reason:             string(gatewayv1.GatewayReasonProgrammed),
		Message:            "Gateway is programmed",
		ObservedGeneration: currentGateway.Generation,
	})
	currentGateway.Status.Listeners = []gatewayv1.ListenerStatus{{
		Name:           "http",
		SupportedKinds: []gatewayv1.RouteGroupKind{{Group: ptr(gatewayv1.Group(gatewayv1.GroupName)), Kind: gatewayv1.Kind("HTTPRoute")}},
		Conditions: []metav1.Condition{
			{
				Type:               string(gatewayv1.ListenerConditionAccepted),
				Status:             metav1.ConditionTrue,
				Reason:             string(gatewayv1.ListenerReasonAccepted),
				Message:            "Listener is accepted by nantian-gw",
				ObservedGeneration: currentGateway.Generation,
			},
			{
				Type:               string(gatewayv1.ListenerConditionResolvedRefs),
				Status:             metav1.ConditionTrue,
				Reason:             string(gatewayv1.ListenerReasonResolvedRefs),
				Message:            "Listener references are resolved",
				ObservedGeneration: currentGateway.Generation,
			},
			{
				Type:               string(gatewayv1.ListenerConditionProgrammed),
				Status:             metav1.ConditionTrue,
				Reason:             string(gatewayv1.ListenerReasonProgrammed),
				Message:            "Listener is programmed",
				ObservedGeneration: currentGateway.Generation,
			},
		},
	}}

	staleClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}, &gatewayv1.Gateway{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			currentGateway.DeepCopy(),
			service.DeepCopy(),
			slice.DeepCopy(),
		).
		Build()

	reader := &countingGetReader{Reader: fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			currentGateway.DeepCopy(),
			service.DeepCopy(),
			slice.DeepCopy(),
		).
		Build()}

	reconciler := NewWithAddressesAndReader(
		staleClient,
		reader,
		string(controllerName),
		[]string{"127.0.0.1"},
		discardLogger(),
	)
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}
	if reader.partialMetadataLists != 0 {
		t.Fatalf("partial metadata list count = %d, want 0 for fully converged gateways", reader.partialMetadataLists)
	}
	if reader.gatewayGets != 0 {
		t.Fatalf("gateway reader Get count = %d, want 0 for fully converged gateways", reader.gatewayGets)
	}
}

func TestReconcileStillRefreshesGatewayMetadataWhenGatewayProgrammedIsNotCurrent(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	staleGateway := gatewayWithNameAndGenerationForConvergenceTest("gw", 1)
	setCondition(&staleGateway.Status.Conditions, conditionSpec{
		Type:               string(gatewayv1.GatewayConditionProgrammed),
		Status:             metav1.ConditionFalse,
		Reason:             string(gatewayv1.GatewayReasonPending),
		Message:            "Waiting for derived Gateway Service to be created",
		ObservedGeneration: 0,
	})

	freshGateway := gatewayWithNameAndGenerationForConvergenceTest("gw", 2)
	service, slice := benchmarkGatewayInfrastructureObjects(*freshGateway)

	staleClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}, &gatewayv1.Gateway{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			staleGateway.DeepCopy(),
		).
		Build()

	reader := &countingGetReader{Reader: fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 2},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			freshGateway.DeepCopy(),
			service.DeepCopy(),
			slice.DeepCopy(),
		).
		Build()}

	reconciler := NewWithAddressesAndReader(
		staleClient,
		reader,
		string(controllerName),
		[]string{"127.0.0.1"},
		discardLogger(),
	)
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}
	if reader.partialMetadataLists != 1 {
		t.Fatalf("partial metadata list count = %d, want 1 when Gateway status is stale", reader.partialMetadataLists)
	}
	if reader.gatewayGets != 1 {
		t.Fatalf("gateway reader Get count = %d, want 1 when API reader refresh is required", reader.gatewayGets)
	}
}

func TestReconcileLoadsReferencedNamespacesSecretsAndConfigMapsOnDemand(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	mode := gatewayv1.TLSModeTerminate
	namespaceMode := gatewayv1.NamespacesFromSelector
	gatewayObj := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "infra", Generation: 1},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "nantian-gw",
			Infrastructure: &gatewayv1.GatewayInfrastructure{
				ParametersRef: &gatewayv1.LocalParametersReference{
					Group: "",
					Kind:  gatewayv1.Kind("ConfigMap"),
					Name:  "gateway-infra",
				},
			},
			Listeners: []gatewayv1.Listener{{
				Name:     "https",
				Protocol: gatewayv1.HTTPSProtocolType,
				Port:     443,
				AllowedRoutes: &gatewayv1.AllowedRoutes{
					Namespaces: &gatewayv1.RouteNamespaces{
						From: &namespaceMode,
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"team": "app"},
						},
					},
				},
				TLS: &gatewayv1.ListenerTLSConfig{
					Mode: &mode,
					CertificateRefs: []gatewayv1.SecretObjectReference{{
						Name: "gateway-cert",
					}},
				},
			}},
		},
	}
	gatewayService := gatewayInfrastructureServiceForGateway(*gatewayObj)
	gatewayEndpointSlice := gatewayInfrastructureEndpointSliceForService(
		gatewayService,
		resources.EndpointSliceRoleGatewayFrontend,
	)

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
					Name:   "app",
					Labels: map[string]string{"team": "app"},
				},
			},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "infra"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway-infra",
					Namespace: "infra",
				},
				Data: map[string]string{
					"service.yaml": "",
				},
			},
			gatewayObj,
			gatewayService,
			gatewayEndpointSlice,
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway-cert",
					Namespace: "infra",
				},
				Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": []byte(readStatusTLSAsset(t, "client.crt")),
					"tls.key": []byte(readStatusTLSAsset(t, "client.key")),
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "app"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8080}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "app", Generation: 1},
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

	reconciler := NewWithAddressesAndReader(
		k8sClient,
		restrictedReader{
			Reader:           k8sClient,
			blockedListTypes: blockedStatusListTypesForFullReconcile(),
		},
		string(controllerName),
		[]string{"127.0.0.1"},
		discardLogger(),
	)
	reconciler.listReader = restrictedReader{
		Reader:           k8sClient,
		blockedListTypes: blockedStatusOnDemandListTypesForFullReconcile(),
	}

	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "infra", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	assertCondition(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionAccepted), metav1.ConditionTrue, string(gatewayv1.GatewayReasonAccepted), 1)
	assertCondition(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionProgrammed), metav1.ConditionTrue, string(gatewayv1.GatewayReasonProgrammed), 1)
	if len(gateway.Status.Listeners) != 1 {
		t.Fatalf("expected 1 listener status, got %d", len(gateway.Status.Listeners))
	}
	assertCondition(t, gateway.Status.Listeners[0].Conditions, string(gatewayv1.ListenerConditionResolvedRefs), metav1.ConditionTrue, string(gatewayv1.ListenerReasonResolvedRefs), 1)

	var route gatewayv1.HTTPRoute
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "app", Name: "route"}, &route); err != nil {
		t.Fatalf("Get HTTPRoute returned error: %v", err)
	}
	if len(route.Status.Parents) != 1 {
		t.Fatalf("expected 1 parent status, got %d", len(route.Status.Parents))
	}
	assertCondition(t, route.Status.Parents[0].Conditions, string(gatewayv1.RouteConditionAccepted), metav1.ConditionTrue, string(gatewayv1.RouteReasonAccepted), 1)
	assertCondition(t, route.Status.Parents[0].Conditions, string(gatewayv1.RouteConditionResolvedRefs), metav1.ConditionTrue, string(gatewayv1.RouteReasonResolvedRefs), 1)
}

func TestReconcileLoadsReferencedServicesAndServiceImportsOnDemand(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	service := gatewayInfrastructureService("default", "gw")
	endpointSlice := gatewayInfrastructureEndpointSliceForService(
		service,
		resources.EndpointSliceRoleGatewayFrontend,
	)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
			&gatewayv1.HTTPRoute{},
			&backend.BackendLBPolicy{},
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
			service,
			endpointSlice,
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8080}},
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
									Name: "orders",
									Port: portPtr(8080),
								},
							},
						}},
					}},
				},
			},
			&backend.BackendLBPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "payments-sticky", Namespace: "default", Generation: 1},
				Spec: backend.BackendLBPolicySpec{
					TargetRefs: []backend.LocalPolicyTargetReference{{
						Group: mcsv1alpha1.GroupName,
						Kind:  "ServiceImport",
						Name:  "payments",
					}},
					SessionPersistence: &gatewayv1.SessionPersistence{},
				},
			},
		).
		Build()

	reconciler := NewWithAddressesAndReader(
		k8sClient,
		restrictedReader{
			Reader:           k8sClient,
			blockedListTypes: blockedStatusListTypesForFullReconcile(),
		},
		string(controllerName),
		[]string{"127.0.0.1"},
		discardLogger(),
	)
	reconciler.listReader = restrictedReader{
		Reader:           k8sClient,
		blockedListTypes: blockedStatusTrafficSupportBulkListTypesForFullReconcile(),
	}

	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	assertCondition(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionProgrammed), metav1.ConditionTrue, string(gatewayv1.GatewayReasonProgrammed), 1)

	var route gatewayv1.HTTPRoute
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "route"}, &route); err != nil {
		t.Fatalf("Get HTTPRoute returned error: %v", err)
	}
	if len(route.Status.Parents) != 1 {
		t.Fatalf("expected 1 parent status, got %d", len(route.Status.Parents))
	}
	assertCondition(t, route.Status.Parents[0].Conditions, string(gatewayv1.RouteConditionResolvedRefs), metav1.ConditionTrue, string(gatewayv1.RouteReasonResolvedRefs), 1)

	var policy backend.BackendLBPolicy
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "payments-sticky"}, &policy); err != nil {
		t.Fatalf("Get BackendLBPolicy returned error: %v", err)
	}
	if len(policy.Status.Ancestors) != 1 {
		t.Fatalf("expected 1 ancestor, got %d", len(policy.Status.Ancestors))
	}
	assertCondition(t, policy.Status.Ancestors[0].Conditions, string(backend.PolicyConditionAccepted), metav1.ConditionTrue, string(backend.PolicyReasonAccepted), 1)
	assertCondition(t, policy.Status.Ancestors[0].Conditions, backendLBPolicyConditionResolvedRefs, metav1.ConditionTrue, backendLBPolicyReasonResolvedRefs, 1)
}

func TestReconcileLoadsReferenceGrantsPerReferencedNamespace(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	mode := gatewayv1.TLSModeTerminate
	seenGrantNamespaces := make(map[string]int)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
			&gatewayv1.HTTPRoute{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "shared"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "certs"}},
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
								Namespace: namespacePtr("certs"),
							}},
						},
					}},
				},
			},
			gatewayInfrastructureService("default", "gw"),
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "shared-cert", Namespace: "certs"},
				Type:       corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": []byte(readStatusTLSAsset(t, "client.crt")),
					"tls.key": []byte(readStatusTLSAsset(t, "client.key")),
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "shared"},
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
				ObjectMeta: metav1.ObjectMeta{Name: "allow-route-backend", Namespace: "shared"},
				Spec: gatewayv1beta1.ReferenceGrantSpec{
					From: []gatewayv1beta1.ReferenceGrantFrom{{
						Group:     gatewayv1beta1.Group(gatewayv1.GroupName),
						Kind:      gatewayv1beta1.Kind("HTTPRoute"),
						Namespace: gatewayv1beta1.Namespace("default"),
					}},
					To: []gatewayv1beta1.ReferenceGrantTo{{
						Group: gatewayv1beta1.Group(""),
						Kind:  gatewayv1beta1.Kind("Service"),
						Name:  ptr(gatewayv1beta1.ObjectName("echo")),
					}},
				},
			},
			&gatewayv1beta1.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{Name: "allow-gateway-cert", Namespace: "certs"},
				Spec: gatewayv1beta1.ReferenceGrantSpec{
					From: []gatewayv1beta1.ReferenceGrantFrom{{
						Group:     gatewayv1beta1.Group(gatewayv1.GroupName),
						Kind:      gatewayv1beta1.Kind("Gateway"),
						Namespace: gatewayv1beta1.Namespace("default"),
					}},
					To: []gatewayv1beta1.ReferenceGrantTo{{
						Group: gatewayv1beta1.Group(""),
						Kind:  gatewayv1beta1.Kind("Secret"),
						Name:  ptr(gatewayv1beta1.ObjectName("shared-cert")),
					}},
				},
			},
		).
		Build()

	reconciler := NewWithAddressesAndReader(
		k8sClient,
		restrictedReader{
			Reader:           k8sClient,
			blockedListTypes: blockedStatusListTypesForFullReconcile(),
		},
		string(controllerName),
		[]string{"127.0.0.1"},
		discardLogger(),
	)
	reconciler.listReader = validatingListReader{
		Reader: k8sClient,
		listValidators: map[reflect.Type]func(client.ListOptions) error{
			reflect.TypeOf(&gatewayv1beta1.ReferenceGrantList{}): func(opts client.ListOptions) error {
				if opts.Namespace == "" {
					return fmt.Errorf("ReferenceGrant list must be namespaced")
				}
				switch opts.Namespace {
				case "shared", "certs":
					seenGrantNamespaces[opts.Namespace]++
					return nil
				default:
					return fmt.Errorf("unexpected ReferenceGrant namespace %q", opts.Namespace)
				}
			},
		},
	}

	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	if seenGrantNamespaces["shared"] == 0 {
		t.Fatalf("expected ReferenceGrant list for shared namespace")
	}
	if seenGrantNamespaces["certs"] == 0 {
		t.Fatalf("expected ReferenceGrant list for certs namespace")
	}
}

func TestReconcileListsGatewayEndpointSlicesByService(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
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
			gatewayInfrastructureEndpointSlice("default", "gw", resources.EndpointSliceRoleSharedFrontend),
		).
		Build()

	blockedReaderListTypes := blockedStatusListTypesForFullReconcile()
	delete(blockedReaderListTypes, reflect.TypeOf(&corev1.ServiceList{}))
	delete(blockedReaderListTypes, reflect.TypeOf(&discoveryv1.EndpointSliceList{}))

	reconciler := NewWithAddressesAndReader(
		k8sClient,
		rawValidatingReader{
			Reader: restrictedReader{
				Reader:           k8sClient,
				blockedListTypes: blockedReaderListTypes,
			},
			listValidators: map[reflect.Type]func([]client.ListOption) error{
				reflect.TypeOf(&corev1.ServiceList{}): func(opts []client.ListOption) error {
					var listOptions client.ListOptions
					for _, opt := range opts {
						opt.ApplyToList(&listOptions)
					}
					return requireNamespaceScopedList("default")(listOptions)
				},
				reflect.TypeOf(&discoveryv1.EndpointSliceList{}): func(opts []client.ListOption) error {
					var listOptions client.ListOptions
					for _, opt := range opts {
						opt.ApplyToList(&listOptions)
					}
					return requireNamespaceScopedList("default")(listOptions)
				},
			},
		},
		string(controllerName),
		[]string{"127.0.0.1"},
		discardLogger(),
	)

	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	assertCondition(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionProgrammed), metav1.ConditionFalse, string(gatewayv1.GatewayReasonPending), 1)
	assertConditionMessage(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionProgrammed), "Waiting for derived Gateway frontend EndpointSlices to converge")
}

func blockedStatusListTypesForFullReconcile() map[reflect.Type]string {
	return map[reflect.Type]string{
		reflect.TypeOf(&gatewayv1.GatewayClassList{}):        "full reconcile should not use the object reader for GatewayClass list scans",
		reflect.TypeOf(&gatewayv1.GatewayList{}):             "full reconcile should not use the object reader for Gateway list scans",
		reflect.TypeOf(&gatewayv1.HTTPRouteList{}):           "full reconcile should not use the object reader for HTTPRoute list scans",
		reflect.TypeOf(&gatewayv1.GRPCRouteList{}):           "full reconcile should not use the object reader for GRPCRoute list scans",
		reflect.TypeOf(&gatewayv1alpha2.TCPRouteList{}):      "full reconcile should not use the object reader for TCPRoute list scans",
		reflect.TypeOf(&gatewayv1alpha2.UDPRouteList{}):      "full reconcile should not use the object reader for UDPRoute list scans",
		reflect.TypeOf(&gatewayv1alpha2.TLSRouteList{}):      "full reconcile should not use the object reader for TLSRoute list scans",
		reflect.TypeOf(&gatewayv1beta1.ReferenceGrantList{}): "full reconcile should not use the object reader for ReferenceGrant list scans",
		reflect.TypeOf(&backend.BackendLBPolicyList{}):       "full reconcile should not use the object reader for BackendLBPolicy list scans",
		reflect.TypeOf(&unstructured.UnstructuredList{}):     "full reconcile should not use the object reader for BackendTLSPolicy list scans",
		reflect.TypeOf(&corev1.ServiceList{}):                "full reconcile should not use the object reader for Service list scans",
		reflect.TypeOf(&discoveryv1.EndpointSliceList{}):     "full reconcile should not use the object reader for EndpointSlice list scans",
		reflect.TypeOf(&mcsv1alpha1.ServiceImportList{}):     "full reconcile should not use the object reader for ServiceImport list scans",
		reflect.TypeOf(&corev1.NamespaceList{}):              "full reconcile should not use the object reader for Namespace list scans",
		reflect.TypeOf(&corev1.SecretList{}):                 "full reconcile should not use the object reader for Secret list scans",
		reflect.TypeOf(&corev1.ConfigMapList{}):              "full reconcile should not use the object reader for ConfigMap list scans",
	}
}

func blockedStatusOnDemandListTypesForFullReconcile() map[reflect.Type]string {
	return map[reflect.Type]string{
		reflect.TypeOf(&gatewayv1beta1.ReferenceGrantList{}): "full reconcile should not list ReferenceGrants when all refs stay in-namespace",
		reflect.TypeOf(&corev1.NamespaceList{}):              "full reconcile should load route Namespaces on demand",
		reflect.TypeOf(&corev1.SecretList{}):                 "full reconcile should load referenced Secrets on demand",
		reflect.TypeOf(&corev1.ConfigMapList{}):              "full reconcile should load referenced ConfigMaps on demand",
	}
}

func blockedStatusTrafficSupportBulkListTypesForFullReconcile() map[reflect.Type]string {
	return map[reflect.Type]string{
		reflect.TypeOf(&corev1.ServiceList{}):            "full reconcile should load referenced Services on demand",
		reflect.TypeOf(&mcsv1alpha1.ServiceImportList{}): "full reconcile should load referenced ServiceImports on demand",
	}
}

func requireNamespaceScopedList(namespace string) func(client.ListOptions) error {
	return func(opts client.ListOptions) error {
		if opts.Namespace != namespace {
			return fmt.Errorf("list namespace = %q, want %q", opts.Namespace, namespace)
		}
		if opts.LabelSelector != nil && !opts.LabelSelector.Empty() {
			return fmt.Errorf("list selector = %q, want namespace-scoped scan without label selector", opts.LabelSelector.String())
		}
		if opts.FieldSelector != nil && !opts.FieldSelector.Empty() {
			return fmt.Errorf("list field selector = %q, want empty", opts.FieldSelector.String())
		}
		return nil
	}
}

func requireEndpointSliceServiceList(namespace, serviceName string) func(client.ListOptions) error {
	return func(opts client.ListOptions) error {
		if opts.Namespace != namespace {
			return fmt.Errorf("endpoint slice list namespace = %q, want %q", opts.Namespace, namespace)
		}
		if opts.LabelSelector == nil || opts.LabelSelector.Empty() {
			return fmt.Errorf("endpoint slice list must include a service label selector")
		}
		if !opts.LabelSelector.Matches(labels.Set{discoveryv1.LabelServiceName: serviceName}) {
			return fmt.Errorf("endpoint slice list selector = %q does not match service %q", opts.LabelSelector.String(), serviceName)
		}
		if opts.LabelSelector.Matches(labels.Set{discoveryv1.LabelServiceName: serviceName + "-other"}) {
			return fmt.Errorf("endpoint slice list selector = %q is broader than service %q", opts.LabelSelector.String(), serviceName)
		}
		return nil
	}
}

func requireGatewayClassControllerList(controllerName string) func(client.ListOptions) error {
	return func(opts client.ListOptions) error {
		if opts.FieldSelector == nil || opts.FieldSelector.Empty() {
			return fmt.Errorf("GatewayClass list must include a controllerName field selector")
		}
		if !opts.FieldSelector.Matches(fields.Set{testStatusGatewayClassControllerNameIndex: controllerName}) {
			return fmt.Errorf("GatewayClass field selector = %q does not match controllerName %q", opts.FieldSelector.String(), controllerName)
		}
		return nil
	}
}

func requireGatewayClassNameList(gatewayClassName string) func(client.ListOptions) error {
	return func(opts client.ListOptions) error {
		if opts.FieldSelector == nil || opts.FieldSelector.Empty() {
			return fmt.Errorf("Gateway list must include a gatewayClassName field selector")
		}
		if !opts.FieldSelector.Matches(fields.Set{testStatusGatewayGatewayClassNameIndex: gatewayClassName}) {
			return fmt.Errorf("Gateway field selector = %q does not match gatewayClassName %q", opts.FieldSelector.String(), gatewayClassName)
		}
		return nil
	}
}
