package infrastructure

import (
	"context"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/nantian-gw/gateway/controlplane/internal/ir"
	"github.com/nantian-gw/gateway/controlplane/internal/mesh"
	"github.com/nantian-gw/gateway/controlplane/internal/nodestatus"
)

func TestReconcileGatewayInfrastructureServicesAreIdempotent(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := newInfrastructureClientBuilder(scheme).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "public",
					Namespace: "default",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nantian-dataplane-0",
					Namespace: defaultDataplaneNamespace,
					Labels:    map[string]string{"app": "nantian-dataplane"},
				},
				Status: corev1.PodStatus{
					PodIP: "10.0.0.50",
					Conditions: []corev1.PodCondition{{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					}},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("first Reconcile returned error: %v", err)
	}

	firstGatewayService, err := mustGetService(
		context.Background(),
		k8sClient,
		client.ObjectKey{Namespace: "default", Name: gatewayServiceName("public")},
	)
	if err != nil {
		t.Fatalf("Get gateway Service after first reconcile returned error: %v", err)
	}
	firstSharedService, err := mustGetService(
		context.Background(),
		k8sClient,
		client.ObjectKey{Namespace: defaultDataplaneNamespace, Name: defaultSharedServiceName},
	)
	if err != nil {
		t.Fatalf("Get shared Service after first reconcile returned error: %v", err)
	}

	var gatewaySlices discoveryv1.EndpointSliceList
	if err := k8sClient.List(
		context.Background(),
		&gatewaySlices,
		client.InNamespace("default"),
		client.MatchingLabels{
			discoveryv1.LabelManagedBy:   managedByValue,
			discoveryv1.LabelServiceName: gatewayServiceName("public"),
			serviceRoleLabel:             gatewayEndpointSliceRoleValue,
		},
	); err != nil {
		t.Fatalf("List gateway endpoint slices after first reconcile returned error: %v", err)
	}
	if len(gatewaySlices.Items) != 1 {
		t.Fatalf("expected 1 gateway endpoint slice after first reconcile, got %d", len(gatewaySlices.Items))
	}

	var sharedSlices discoveryv1.EndpointSliceList
	if err := k8sClient.List(
		context.Background(),
		&sharedSlices,
		client.InNamespace(defaultDataplaneNamespace),
		client.MatchingLabels{
			discoveryv1.LabelManagedBy:   managedByValue,
			discoveryv1.LabelServiceName: defaultSharedServiceName,
			serviceRoleLabel:             sharedEndpointSliceRoleValue,
		},
	); err != nil {
		t.Fatalf("List shared endpoint slices after first reconcile returned error: %v", err)
	}
	if len(sharedSlices.Items) != 1 {
		t.Fatalf("expected 1 shared endpoint slice after first reconcile, got %d", len(sharedSlices.Items))
	}

	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("second Reconcile returned error: %v", err)
	}

	secondGatewayService, err := mustGetService(
		context.Background(),
		k8sClient,
		client.ObjectKey{Namespace: "default", Name: gatewayServiceName("public")},
	)
	if err != nil {
		t.Fatalf("Get gateway Service after second reconcile returned error: %v", err)
	}
	secondSharedService, err := mustGetService(
		context.Background(),
		k8sClient,
		client.ObjectKey{Namespace: defaultDataplaneNamespace, Name: defaultSharedServiceName},
	)
	if err != nil {
		t.Fatalf("Get shared Service after second reconcile returned error: %v", err)
	}

	if !serviceEqual(firstGatewayService, secondGatewayService) {
		t.Fatalf("gateway service changed after second reconcile:\nfirst=%#v\nsecond=%#v", firstGatewayService.Spec, secondGatewayService.Spec)
	}
	if !serviceEqual(firstSharedService, secondSharedService) {
		t.Fatalf("shared service changed after second reconcile:\nfirst=%#v\nsecond=%#v", firstSharedService.Spec, secondSharedService.Spec)
	}

	gatewaySlices = discoveryv1.EndpointSliceList{}
	if err := k8sClient.List(
		context.Background(),
		&gatewaySlices,
		client.InNamespace("default"),
		client.MatchingLabels{
			discoveryv1.LabelManagedBy:   managedByValue,
			discoveryv1.LabelServiceName: gatewayServiceName("public"),
			serviceRoleLabel:             gatewayEndpointSliceRoleValue,
		},
	); err != nil {
		t.Fatalf("List gateway endpoint slices after second reconcile returned error: %v", err)
	}
	if len(gatewaySlices.Items) != 1 {
		t.Fatalf("expected 1 gateway endpoint slice after second reconcile, got %d", len(gatewaySlices.Items))
	}

	sharedSlices = discoveryv1.EndpointSliceList{}
	if err := k8sClient.List(
		context.Background(),
		&sharedSlices,
		client.InNamespace(defaultDataplaneNamespace),
		client.MatchingLabels{
			discoveryv1.LabelManagedBy:   managedByValue,
			discoveryv1.LabelServiceName: defaultSharedServiceName,
			serviceRoleLabel:             sharedEndpointSliceRoleValue,
		},
	); err != nil {
		t.Fatalf("List shared endpoint slices after second reconcile returned error: %v", err)
	}
	if len(sharedSlices.Items) != 1 {
		t.Fatalf("expected 1 shared endpoint slice after second reconcile, got %d", len(sharedSlices.Items))
	}
}

func TestReconcileFrontsMeshServiceAndCreatesShadow(t *testing.T) {
	scheme := newScheme(t)
	serviceKind := gatewayv1.Kind("Service")
	servicePort := gatewayv1.PortNumber(80)
	httpProtocol := "http"

	k8sClient := newInfrastructureClientBuilder(scheme).
		WithObjects(
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "echo"},
					Ports: []corev1.ServicePort{
						{
							Name:        "http",
							Port:        80,
							TargetPort:  intstr.FromInt(8080),
							Protocol:    corev1.ProtocolTCP,
							AppProtocol: &httpProtocol,
						},
						{
							Name:        "http-alt",
							Port:        8080,
							TargetPort:  intstr.FromInt(8080),
							Protocol:    corev1.ProtocolTCP,
							AppProtocol: &httpProtocol,
						},
					},
				},
			},
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "echo-stale",
					Namespace: "default",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "echo",
					},
				},
				AddressType: discoveryv1.AddressTypeIPv4,
				Endpoints: []discoveryv1.Endpoint{{
					Addresses: []string{"10.0.0.10"},
				}},
				Ports: []discoveryv1.EndpointPort{{
					Name: func() *string {
						value := "http"
						return &value
					}(),
					Port: func() *int32 {
						value := int32(8080)
						return &value
					}(),
				}},
			},
			&corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "echo",
					Namespace: "default",
				},
				Subsets: []corev1.EndpointSubset{{
					Addresses: []corev1.EndpointAddress{{
						IP: "10.0.0.10",
					}},
					Ports: []corev1.EndpointPort{{
						Name: "http",
						Port: 8080,
					}},
				}},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "mesh", Namespace: "default"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Kind: &serviceKind,
							Name: "echo",
							Port: &servicePort,
						}},
					},
				},
			},
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nantian-dataplane-0",
					Namespace: defaultDataplaneNamespace,
					Labels:    map[string]string{"app": "nantian-dataplane"},
				},
				Status: corev1.PodStatus{
					PodIP: "10.0.0.50",
					Conditions: []corev1.PodCondition{{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					}},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, "gateway.networking.k8s.io/nantian-gw", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	service, err := mustGetService(context.Background(), k8sClient, client.ObjectKey{Name: "echo", Namespace: "default"})
	if err != nil {
		t.Fatalf("Get mesh frontend service returned error: %v", err)
	}
	if service.Annotations[mesh.ManagedServiceAnnotation] != "true" {
		t.Fatalf("expected managed annotation, got %#v", service.Annotations)
	}
	if service.Labels[managedByLabel] != managedByValue {
		t.Fatalf("expected managed-by label, got %#v", service.Labels)
	}
	if service.Labels[serviceRoleLabel] != "mesh-frontend" {
		t.Fatalf("expected mesh frontend role label, got %#v", service.Labels)
	}
	if service.Spec.Selector != nil {
		t.Fatalf("mesh service selector = %#v, want nil selector with managed EndpointSlices", service.Spec.Selector)
	}
	if service.Spec.Ports[0].TargetPort == intstr.FromInt(8080) || service.Spec.Ports[1].TargetPort == intstr.FromInt(8080) {
		t.Fatalf("expected mesh service target ports to be remapped, got %#v", service.Spec.Ports)
	}
	policy := &networkingv1.NetworkPolicy{}
	if err := k8sClient.Get(
		context.Background(),
		client.ObjectKey{
			Namespace: defaultDataplaneNamespace,
			Name:      defaultDataplaneNetworkPolicyName,
		},
		policy,
	); err != nil {
		t.Fatalf("Get dataplane NetworkPolicy returned error: %v", err)
	}
	for _, port := range service.Spec.Ports {
		assertNetworkPolicyPort(
			t,
			policy.Spec.Ingress,
			int32(port.TargetPort.IntValue()),
			port.Protocol,
		)
	}
	endpointSlice := &discoveryv1.EndpointSlice{}
	if err := k8sClient.Get(
		context.Background(),
		client.ObjectKey{Name: "echo-stale", Namespace: "default"},
		endpointSlice,
	); client.IgnoreNotFound(err) != nil {
		t.Fatalf("Get mesh frontend EndpointSlice returned error: %v", err)
	}
	if endpointSlice.Name != "" {
		t.Fatalf("expected stale service EndpointSlice to be removed, got %#v", endpointSlice.Endpoints)
	}
	endpoints := &corev1.Endpoints{}
	if err := k8sClient.Get(
		context.Background(),
		client.ObjectKey{Name: "echo", Namespace: "default"},
		endpoints,
	); client.IgnoreNotFound(err) != nil {
		t.Fatalf("Get mesh frontend Endpoints returned error: %v", err)
	}
	if endpoints.Name != "" {
		t.Fatalf("expected stale service Endpoints to be removed, got %#v", endpoints.Subsets)
	}

	shadowName := mesh.ShadowServiceName("default", "echo")
	shadow, err := mustGetService(context.Background(), k8sClient, client.ObjectKey{Name: shadowName, Namespace: "default"})
	if err != nil {
		t.Fatalf("Get mesh shadow service returned error: %v", err)
	}
	if shadow.Labels[mesh.ShadowServiceRoleLabel] != mesh.ShadowServiceRoleValue {
		t.Fatalf("expected shadow service role label, got %#v", shadow.Labels)
	}
	if shadow.Spec.Selector["app"] != "echo" {
		t.Fatalf("shadow selector = %#v", shadow.Spec.Selector)
	}
	if shadow.Spec.Ports[0].TargetPort != intstr.FromInt(8080) {
		t.Fatalf("shadow ports should preserve original target ports, got %#v", shadow.Spec.Ports)
	}

	var endpointSlices discoveryv1.EndpointSliceList
	if err := k8sClient.List(
		context.Background(),
		&endpointSlices,
		client.InNamespace("default"),
		client.MatchingLabels{
			discoveryv1.LabelManagedBy:   managedByValue,
			discoveryv1.LabelServiceName: "echo",
			serviceRoleLabel:             meshEndpointSliceRoleValue,
		},
	); err != nil {
		t.Fatalf("List mesh endpoint slices returned error: %v", err)
	}
	if len(endpointSlices.Items) != 1 {
		t.Fatalf("expected 1 mesh endpoint slice, got %d", len(endpointSlices.Items))
	}
	if got := endpointSlices.Items[0].Endpoints; len(got) != 1 || len(got[0].Addresses) != 1 || got[0].Addresses[0] != "10.0.0.50" {
		t.Fatalf("unexpected mesh endpoint slice endpoints: %#v", got)
	}
	if got := endpointSlices.Items[0].Ports; len(got) != 2 || got[0].Port == nil || got[1].Port == nil {
		t.Fatalf("unexpected mesh endpoint slice ports: %#v", got)
	}
	serviceTargetPorts := map[string]int32{}
	for _, port := range service.Spec.Ports {
		serviceTargetPorts[port.Name] = int32(port.TargetPort.IntValue())
	}
	for _, port := range endpointSlices.Items[0].Ports {
		if port.Name == nil || port.Port == nil {
			t.Fatalf("expected named mesh endpoint slice ports, got %#v", endpointSlices.Items[0].Ports)
		}
		if serviceTargetPorts[*port.Name] != *port.Port {
			t.Fatalf("mesh endpoint slice port %s = %d, want %d", *port.Name, *port.Port, serviceTargetPorts[*port.Name])
		}
	}
}

func TestReconcileMeshServicesUsesServiceParentRouteIndexes(t *testing.T) {
	scheme := newScheme(t)
	serviceKind := gatewayv1.Kind("Service")
	servicePort := gatewayv1.PortNumber(80)

	baseClient := withInfrastructureRouteParentIndexes(
		fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{"app": "echo"},
						Ports: []corev1.ServicePort{{
							Name:       "http",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
							Protocol:   corev1.ProtocolTCP,
						}},
					},
				},
				&gatewayv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{Name: "mesh", Namespace: "default"},
					Spec: gatewayv1.HTTPRouteSpec{
						CommonRouteSpec: gatewayv1.CommonRouteSpec{
							ParentRefs: []gatewayv1.ParentReference{{
								Kind: &serviceKind,
								Name: "echo",
								Port: &servicePort,
							}},
						},
					},
				},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nantian-dataplane-0",
						Namespace: defaultDataplaneNamespace,
						Labels:    map[string]string{"app": "nantian-dataplane"},
					},
					Status: corev1.PodStatus{
						PodIP: "10.0.0.50",
						Conditions: []corev1.PodCondition{{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						}},
					},
				},
			),
	).Build()

	reconciler := New(
		rawValidatingClient{
			Client: baseClient,
			listValidators: map[reflect.Type]func([]client.ListOption) error{
				reflect.TypeOf(&gatewayv1.HTTPRouteList{}):      requireMatchingField(httpRouteServiceParentIndex, serviceParentIndexMarker),
				reflect.TypeOf(&gatewayv1.GRPCRouteList{}):      requireMatchingField(grpcRouteServiceParentIndex, serviceParentIndexMarker),
				reflect.TypeOf(&gatewayv1alpha2.TCPRouteList{}): requireMatchingField(tcpRouteServiceParentIndex, serviceParentIndexMarker),
				reflect.TypeOf(&gatewayv1alpha2.UDPRouteList{}): requireMatchingField(udpRouteServiceParentIndex, serviceParentIndexMarker),
				reflect.TypeOf(&gatewayv1alpha2.TLSRouteList{}): requireMatchingField(tlsRouteServiceParentIndex, serviceParentIndexMarker),
			},
		},
		"gateway.networking.k8s.io/nantian-gw",
		discardLogger(),
	)

	if err := reconciler.reconcileMeshServices(context.Background()); err != nil {
		t.Fatalf("reconcileMeshServices returned error: %v", err)
	}
}

func TestReconcileFrontsMeshServiceFromSnapshotStoreWhenRouteCacheLags(t *testing.T) {
	scheme := newScheme(t)
	httpProtocol := "http"

	k8sClient := newInfrastructureClientBuilder(scheme).
		WithObjects(
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "echo"},
					Ports: []corev1.ServicePort{{
						Name:        "http",
						Port:        80,
						TargetPort:  intstr.FromInt(8080),
						Protocol:    corev1.ProtocolTCP,
						AppProtocol: &httpProtocol,
					}},
				},
			},
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nantian-dataplane-0",
					Namespace: defaultDataplaneNamespace,
					Labels:    map[string]string{"app": "nantian-dataplane"},
				},
				Status: corev1.PodStatus{
					PodIP: "10.0.0.50",
					Conditions: []corev1.PodCondition{{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					}},
				},
			},
		).
		Build()

	store := ir.NewSnapshotStore(discardLogger())
	if !store.Publish(&ir.Snapshot{
		HTTPRoutes: []ir.HTTPRoute{{
			Name:      "mesh-redirect-port",
			Namespace: "default",
			ParentRefs: []ir.ParentRef{{
				Kind: "Service",
				Name: "echo",
				Port: 80,
			}},
		}},
	}) {
		t.Fatal("expected snapshot publish to succeed")
	}

	options := DefaultOptions()
	options.SnapshotStore = store
	reconciler := NewWithOptions(
		k8sClient,
		"gateway.networking.k8s.io/nantian-gw",
		options,
		discardLogger(),
	)

	if err := reconciler.reconcileMeshServices(context.Background()); err != nil {
		t.Fatalf("reconcileMeshServices returned error: %v", err)
	}

	service, err := mustGetService(
		context.Background(),
		k8sClient,
		client.ObjectKey{Name: "echo", Namespace: "default"},
	)
	if err != nil {
		t.Fatalf("Get mesh frontend service returned error: %v", err)
	}
	if service.Annotations[mesh.ManagedServiceAnnotation] != "true" {
		t.Fatalf("expected managed annotation, got %#v", service.Annotations)
	}
	if service.Spec.Selector != nil {
		t.Fatalf("mesh service selector = %#v, want nil selector with managed EndpointSlices", service.Spec.Selector)
	}

	shadowName := mesh.ShadowServiceName("default", "echo")
	if _, err := mustGetService(
		context.Background(),
		k8sClient,
		client.ObjectKey{Name: shadowName, Namespace: "default"},
	); err != nil {
		t.Fatalf("Get mesh shadow service returned error: %v", err)
	}
}

func TestReconcileFrontsMeshServiceOnlyWithAckedCurrentSnapshotNodes(t *testing.T) {
	scheme := newScheme(t)
	serviceKind := gatewayv1.Kind("Service")
	servicePort := gatewayv1.PortNumber(80)

	k8sClient := newInfrastructureClientBuilder(scheme).
		WithObjects(
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "echo"},
					Ports: []corev1.ServicePort{{
						Name:       "http",
						Port:       80,
						TargetPort: intstr.FromInt(8080),
						Protocol:   corev1.ProtocolTCP,
					}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "mesh", Namespace: "default"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Kind: &serviceKind,
							Name: "echo",
							Port: &servicePort,
						}},
					},
				},
			},
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nantian-dataplane-current",
					Namespace: defaultDataplaneNamespace,
					Labels:    map[string]string{"app": "nantian-dataplane"},
				},
				Status: corev1.PodStatus{
					PodIP: "10.0.0.50",
					Conditions: []corev1.PodCondition{{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					}},
				},
			},
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nantian-dataplane-stale",
					Namespace: defaultDataplaneNamespace,
					Labels:    map[string]string{"app": "nantian-dataplane"},
				},
				Status: corev1.PodStatus{
					PodIP: "10.0.0.51",
					Conditions: []corev1.PodCondition{{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					}},
				},
			},
		).
		Build()

	store := ir.NewSnapshotStore(discardLogger())
	if !store.Publish(&ir.Snapshot{}) {
		t.Fatal("expected snapshot publish to succeed")
	}
	currentVersion := store.Current().ID

	nodes := nodestatus.NewRegistry(ir.NewNodeStatusStore(), nil, discardLogger(), nodestatus.Options{})
	now := time.Now().UTC()
	nodes.Connect(context.Background(), "nantian-dataplane-current", "kind", nil, now)
	nodes.ObservePublished(context.Background(), "nantian-dataplane-current", currentVersion, now)
	nodes.ObserveAck(context.Background(), "nantian-dataplane-current", "kind", currentVersion, currentVersion, nil, now)
	nodes.ObserveReport(context.Background(), "nantian-dataplane-current", currentVersion, true, "ready", now)
	nodes.Connect(context.Background(), "nantian-dataplane-stale", "kind", nil, now)
	nodes.ObservePublished(context.Background(), "nantian-dataplane-stale", currentVersion, now)
	nodes.ObserveAck(context.Background(), "nantian-dataplane-stale", "kind", "stale-version", "stale-version", nil, now)
	nodes.ObserveReport(context.Background(), "nantian-dataplane-stale", "stale-version", true, "stale", now)

	options := DefaultOptions()
	options.SnapshotStore = store
	options.NodeStatus = nodes
	reconciler := NewWithOptions(k8sClient, "gateway.networking.k8s.io/nantian-gw", options, discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var endpointSlices discoveryv1.EndpointSliceList
	if err := k8sClient.List(
		context.Background(),
		&endpointSlices,
		client.InNamespace("default"),
		client.MatchingLabels{
			discoveryv1.LabelManagedBy:   managedByValue,
			discoveryv1.LabelServiceName: "echo",
			serviceRoleLabel:             meshEndpointSliceRoleValue,
		},
	); err != nil {
		t.Fatalf("List mesh endpoint slices returned error: %v", err)
	}
	if len(endpointSlices.Items) != 1 {
		t.Fatalf("expected 1 mesh endpoint slice, got %d", len(endpointSlices.Items))
	}
	if got := endpointSlices.Items[0].Endpoints; len(got) != 1 || len(got[0].Addresses) != 1 || got[0].Addresses[0] != "10.0.0.50" {
		t.Fatalf("unexpected mesh endpoint slice endpoints: %#v", got)
	}
}

func TestReconcileMeshServiceRequiresCurrentSnapshotAckNodes(t *testing.T) {
	scheme := newScheme(t)
	serviceKind := gatewayv1.Kind("Service")
	servicePort := gatewayv1.PortNumber(80)

	k8sClient := newInfrastructureClientBuilder(scheme).
		WithObjects(
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "echo"},
					Ports: []corev1.ServicePort{{
						Name:       "http",
						Port:       80,
						TargetPort: intstr.FromInt(8080),
						Protocol:   corev1.ProtocolTCP,
					}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "mesh", Namespace: "default"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Kind: &serviceKind,
							Name: "echo",
							Port: &servicePort,
						}},
					},
				},
			},
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nantian-dataplane-stable",
					Namespace: defaultDataplaneNamespace,
					Labels:    map[string]string{"app": "nantian-dataplane"},
				},
				Status: corev1.PodStatus{
					PodIP: "10.0.0.50",
					Conditions: []corev1.PodCondition{{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					}},
				},
			},
		).
		Build()

	store := ir.NewSnapshotStore(discardLogger())
	if !store.Publish(&ir.Snapshot{}) {
		t.Fatal("expected snapshot publish to succeed")
	}

	nodes := nodestatus.NewRegistry(ir.NewNodeStatusStore(), nil, discardLogger(), nodestatus.Options{})
	now := time.Now().UTC()
	nodes.Connect(context.Background(), "nantian-dataplane-stable", "kind", nil, now)
	nodes.ObservePublished(context.Background(), "nantian-dataplane-stable", store.Current().ID, now)
	nodes.ObserveAck(context.Background(), "nantian-dataplane-stable", "kind", "stable-version", "stable-version", nil, now)
	nodes.ObserveReport(context.Background(), "nantian-dataplane-stable", "stable-version", true, "stable", now)

	options := DefaultOptions()
	options.SnapshotStore = store
	options.NodeStatus = nodes
	reconciler := NewWithOptions(k8sClient, "gateway.networking.k8s.io/nantian-gw", options, discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var endpointSlices discoveryv1.EndpointSliceList
	if err := k8sClient.List(
		context.Background(),
		&endpointSlices,
		client.InNamespace("default"),
		client.MatchingLabels{
			discoveryv1.LabelManagedBy:   managedByValue,
			discoveryv1.LabelServiceName: "echo",
			serviceRoleLabel:             meshEndpointSliceRoleValue,
		},
	); err != nil {
		t.Fatalf("List mesh endpoint slices returned error: %v", err)
	}
	if len(endpointSlices.Items) != 0 {
		t.Fatalf("expected no mesh endpoint slice without a current snapshot ack, got %#v", endpointSlices.Items)
	}
}

func TestReconcileMeshServiceDeletesForeignEndpointSlicesBeforeManagedReplacement(t *testing.T) {
	scheme := newScheme(t)
	serviceKind := gatewayv1.Kind("Service")
	servicePort := gatewayv1.PortNumber(80)
	staleReady := true

	baseClient := newInfrastructureClientBuilder(scheme).
		WithObjects(
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "echo"},
					Ports: []corev1.ServicePort{{
						Name:       "http",
						Port:       80,
						TargetPort: intstr.FromInt(8080),
						Protocol:   corev1.ProtocolTCP,
					}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "mesh", Namespace: "default"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Kind: &serviceKind,
							Name: "echo",
							Port: &servicePort,
						}},
					},
				},
			},
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "echo-stale",
					Namespace: "default",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "echo",
					},
				},
				AddressType: discoveryv1.AddressTypeIPv4,
				Endpoints: []discoveryv1.Endpoint{{
					Addresses: []string{"10.0.0.10"},
					Conditions: discoveryv1.EndpointConditions{
						Ready: &staleReady,
					},
				}},
			},
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nantian-dataplane-0",
					Namespace: defaultDataplaneNamespace,
					Labels:    map[string]string{"app": "nantian-dataplane"},
				},
				Status: corev1.PodStatus{
					PodIP: "10.0.0.50",
					Conditions: []corev1.PodCondition{{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					}},
				},
			},
		).
		Build()

	operations := make([]string, 0)
	reconciler := New(
		meshEndpointOperationRecorder{
			Client:     baseClient,
			operations: &operations,
		},
		"gateway.networking.k8s.io/nantian-gw",
		discardLogger(),
	)
	if err := reconciler.reconcileMeshServices(context.Background()); err != nil {
		t.Fatalf("reconcileMeshServices returned error: %v", err)
	}

	staleDelete := operationIndex(operations, "delete endpointslice/default/echo-stale")
	managedCreate := operationIndex(operations, "create endpointslice/default/aeg-mesh-ep-echo-ipv4")
	if staleDelete < 0 || managedCreate < 0 {
		t.Fatalf("expected stale delete and managed create operations, got %#v", operations)
	}
	if staleDelete > managedCreate {
		t.Fatalf("stale EndpointSlice deleted after managed replacement creation; operations: %#v", operations)
	}
}

type meshEndpointOperationRecorder struct {
	client.Client
	operations *[]string
}

func (c meshEndpointOperationRecorder) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	c.record("create", obj)
	return c.Client.Create(ctx, obj, opts...)
}

func (c meshEndpointOperationRecorder) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	c.record("update", obj)
	return c.Client.Update(ctx, obj, opts...)
}

func (c meshEndpointOperationRecorder) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	c.record("delete", obj)
	return c.Client.Delete(ctx, obj, opts...)
}

func (c meshEndpointOperationRecorder) record(action string, obj client.Object) {
	if c.operations == nil {
		return
	}
	switch obj.(type) {
	case *discoveryv1.EndpointSlice:
		*c.operations = append(*c.operations, action+" endpointslice/"+obj.GetNamespace()+"/"+obj.GetName())
	case *corev1.Service:
		*c.operations = append(*c.operations, action+" service/"+obj.GetNamespace()+"/"+obj.GetName())
	}
}

func operationIndex(operations []string, operation string) int {
	for idx, item := range operations {
		if item == operation {
			return idx
		}
	}
	return -1
}

func TestReconcileFrontsSharedAndGatewayServicesOnlyWithAckedCurrentSnapshotNodes(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := newInfrastructureClientBuilder(scheme).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "public",
					Namespace: "default",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nantian-dataplane-current",
					Namespace: defaultDataplaneNamespace,
					Labels:    map[string]string{"app": "nantian-dataplane"},
				},
				Status: corev1.PodStatus{
					PodIP: "10.0.0.50",
					Conditions: []corev1.PodCondition{{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					}},
				},
			},
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nantian-dataplane-stale",
					Namespace: defaultDataplaneNamespace,
					Labels:    map[string]string{"app": "nantian-dataplane"},
				},
				Status: corev1.PodStatus{
					PodIP: "10.0.0.51",
					Conditions: []corev1.PodCondition{{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					}},
				},
			},
		).
		Build()

	store := ir.NewSnapshotStore(discardLogger())
	if !store.Publish(&ir.Snapshot{}) {
		t.Fatal("expected snapshot publish to succeed")
	}
	currentVersion := store.Current().ID

	nodes := nodestatus.NewRegistry(ir.NewNodeStatusStore(), nil, discardLogger(), nodestatus.Options{})
	now := time.Now().UTC()
	nodes.Connect(context.Background(), "nantian-dataplane-current", "kind", nil, now)
	nodes.ObservePublished(context.Background(), "nantian-dataplane-current", currentVersion, now)
	nodes.ObserveAck(context.Background(), "nantian-dataplane-current", "kind", currentVersion, currentVersion, nil, now)
	nodes.ObserveReport(context.Background(), "nantian-dataplane-current", currentVersion, true, "ready", now)
	nodes.Connect(context.Background(), "nantian-dataplane-stale", "kind", nil, now)
	nodes.ObservePublished(context.Background(), "nantian-dataplane-stale", currentVersion, now)
	nodes.ObserveAck(context.Background(), "nantian-dataplane-stale", "kind", "stale-version", "stale-version", nil, now)
	nodes.ObserveReport(context.Background(), "nantian-dataplane-stale", "stale-version", true, "stale", now)

	options := DefaultOptions()
	options.SnapshotStore = store
	options.NodeStatus = nodes
	reconciler := NewWithOptions(k8sClient, string(controllerName), options, discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	sharedService, err := mustGetService(
		context.Background(),
		k8sClient,
		client.ObjectKey{Namespace: defaultDataplaneNamespace, Name: defaultSharedServiceName},
	)
	if err != nil {
		t.Fatalf("Get shared Service returned error: %v", err)
	}
	if sharedService.Spec.Selector != nil {
		t.Fatalf("shared service selector = %#v, want nil with managed EndpointSlices", sharedService.Spec.Selector)
	}

	gatewayService, err := mustGetService(
		context.Background(),
		k8sClient,
		client.ObjectKey{Namespace: "default", Name: gatewayServiceName("public")},
	)
	if err != nil {
		t.Fatalf("Get gateway Service returned error: %v", err)
	}
	if gatewayService.Spec.Selector != nil {
		t.Fatalf("gateway service selector = %#v, want nil with managed EndpointSlices", gatewayService.Spec.Selector)
	}

	var sharedSlices discoveryv1.EndpointSliceList
	if err := k8sClient.List(
		context.Background(),
		&sharedSlices,
		client.InNamespace(defaultDataplaneNamespace),
		client.MatchingLabels{
			discoveryv1.LabelManagedBy:   managedByValue,
			discoveryv1.LabelServiceName: defaultSharedServiceName,
			serviceRoleLabel:             sharedEndpointSliceRoleValue,
		},
	); err != nil {
		t.Fatalf("List shared endpoint slices returned error: %v", err)
	}
	if len(sharedSlices.Items) != 1 {
		t.Fatalf("expected 1 shared endpoint slice, got %d", len(sharedSlices.Items))
	}
	if got := sharedSlices.Items[0].Endpoints; len(got) != 1 || len(got[0].Addresses) != 1 || got[0].Addresses[0] != "10.0.0.50" {
		t.Fatalf("unexpected shared endpoint slice endpoints: %#v", got)
	}

	var gatewaySlices discoveryv1.EndpointSliceList
	if err := k8sClient.List(
		context.Background(),
		&gatewaySlices,
		client.InNamespace("default"),
		client.MatchingLabels{
			discoveryv1.LabelManagedBy:   managedByValue,
			discoveryv1.LabelServiceName: gatewayServiceName("public"),
			serviceRoleLabel:             gatewayEndpointSliceRoleValue,
		},
	); err != nil {
		t.Fatalf("List gateway endpoint slices returned error: %v", err)
	}
	if len(gatewaySlices.Items) != 1 {
		t.Fatalf("expected 1 gateway endpoint slice, got %d", len(gatewaySlices.Items))
	}
	if got := gatewaySlices.Items[0].Endpoints; len(got) != 1 || len(got[0].Addresses) != 1 || got[0].Addresses[0] != "10.0.0.50" {
		t.Fatalf("unexpected gateway endpoint slice endpoints: %#v", got)
	}
}

func TestReconcileFrontsSharedAndGatewayServicesRequireCurrentSnapshotAck(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := newInfrastructureClientBuilder(scheme).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "public",
					Namespace: "default",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nantian-dataplane-stable-a",
					Namespace: defaultDataplaneNamespace,
					Labels:    map[string]string{"app": "nantian-dataplane"},
				},
				Status: corev1.PodStatus{
					PodIP: "10.0.0.50",
					Conditions: []corev1.PodCondition{{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					}},
				},
			},
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nantian-dataplane-stable-b",
					Namespace: defaultDataplaneNamespace,
					Labels:    map[string]string{"app": "nantian-dataplane"},
				},
				Status: corev1.PodStatus{
					PodIP: "10.0.0.51",
					Conditions: []corev1.PodCondition{{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					}},
				},
			},
		).
		Build()

	store := ir.NewSnapshotStore(discardLogger())
	if !store.Publish(&ir.Snapshot{}) {
		t.Fatal("expected snapshot publish to succeed")
	}

	nodes := nodestatus.NewRegistry(ir.NewNodeStatusStore(), nil, discardLogger(), nodestatus.Options{})
	now := time.Now().UTC()
	nodes.Connect(context.Background(), "nantian-dataplane-stable-a", "kind", nil, now)
	nodes.ObservePublished(context.Background(), "nantian-dataplane-stable-a", store.Current().ID, now)
	nodes.ObserveAck(context.Background(), "nantian-dataplane-stable-a", "kind", "stable-version", "stable-version", nil, now)
	nodes.ObserveReport(context.Background(), "nantian-dataplane-stable-a", "stable-version", true, "stable-a", now)
	nodes.Connect(context.Background(), "nantian-dataplane-stable-b", "kind", nil, now)
	nodes.ObservePublished(context.Background(), "nantian-dataplane-stable-b", store.Current().ID, now)
	nodes.ObserveAck(context.Background(), "nantian-dataplane-stable-b", "kind", "stable-version", "stable-version", nil, now)
	nodes.ObserveReport(context.Background(), "nantian-dataplane-stable-b", "stable-version", true, "stable-b", now)

	options := DefaultOptions()
	options.SnapshotStore = store
	options.NodeStatus = nodes
	reconciler := NewWithOptions(k8sClient, string(controllerName), options, discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var sharedSlices discoveryv1.EndpointSliceList
	if err := k8sClient.List(
		context.Background(),
		&sharedSlices,
		client.InNamespace(defaultDataplaneNamespace),
		client.MatchingLabels{
			discoveryv1.LabelManagedBy:   managedByValue,
			discoveryv1.LabelServiceName: defaultSharedServiceName,
			serviceRoleLabel:             sharedEndpointSliceRoleValue,
		},
	); err != nil {
		t.Fatalf("List shared endpoint slices returned error: %v", err)
	}
	if len(sharedSlices.Items) != 0 {
		t.Fatalf("expected no shared endpoint slice without a current snapshot ack, got %#v", sharedSlices.Items)
	}

	var gatewaySlices discoveryv1.EndpointSliceList
	if err := k8sClient.List(
		context.Background(),
		&gatewaySlices,
		client.InNamespace("default"),
		client.MatchingLabels{
			discoveryv1.LabelManagedBy:   managedByValue,
			discoveryv1.LabelServiceName: gatewayServiceName("public"),
			serviceRoleLabel:             gatewayEndpointSliceRoleValue,
		},
	); err != nil {
		t.Fatalf("List gateway endpoint slices returned error: %v", err)
	}
	if len(gatewaySlices.Items) != 0 {
		t.Fatalf("expected no gateway endpoint slice without a current snapshot ack, got %#v", gatewaySlices.Items)
	}
}
