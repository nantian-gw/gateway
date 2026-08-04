package controller

import (
	"context"
	"io"
	"log/slog"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/mesh"
	"github.com/nantian-gw/gateway/internal/translator"
)

func TestReconcileServiceImportScopedRequestRefreshesBackendRefsAndBackends(t *testing.T) {
	scheme := newPartialRebuildTestScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	servicePort := gatewayv1.PortNumber(8080)

	baseClient := newControllerClientBuilder(scheme).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Name:        "gw",
							SectionName: ptr[gatewayv1.SectionName]("http"),
						}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Group: ptr[gatewayv1.Group](mcsv1alpha1.GroupName),
									Kind:  ptr[gatewayv1.Kind]("ServiceImport"),
									Name:  "echo",
									Port:  &servicePort,
								},
							},
						}},
					}},
				},
			},
			&mcsv1alpha1.ServiceImport{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec: mcsv1alpha1.ServiceImportSpec{
					Type: mcsv1alpha1.ClusterSetIP,
					Ports: []mcsv1alpha1.ServicePort{{
						Port:     8080,
						Protocol: corev1.ProtocolTCP,
					}},
				},
			},
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "echo-import-1",
					Namespace: "default",
					Labels: map[string]string{
						mcsv1alpha1.LabelServiceName: "echo",
					},
				},
				Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](8080)}},
				Endpoints: []discoveryv1.Endpoint{{
					Addresses: []string{"10.0.0.20"},
				}},
			},
		).
		Build()

	validatingClient := &partialRebuildValidatingClient{Client: baseClient}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	syncer := NewSyncer(
		validatingClient,
		translator.New(string(controllerName), logger),
		store,
		testMetrics(),
		0,
		logger,
	)
	syncer.SetSettleDelay(0)

	if _, err := syncer.Reconcile(context.Background(), snapshotReconcileRequest); err != nil {
		t.Fatalf("initial Reconcile returned error: %v", err)
	}

	current := store.Current()
	if current == nil || len(current.Backends) != 1 {
		t.Fatalf("expected initial serviceimport backend, got %#v", current)
	}
	if metadata := current.HTTPRoutes[0].Rules[0].BackendRefs[0].Metadata; metadata["nantian.dev/backend-ref-valid"] != "" {
		t.Fatalf("expected serviceimport backend ref to start valid, got %#v", metadata)
	}

	if err := validatingClient.Delete(
		context.Background(),
		&mcsv1alpha1.ServiceImport{
			ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
		},
	); err != nil {
		t.Fatalf("delete service import: %v", err)
	}
	validatingClient.forbiddenLists = map[reflect.Type]string{
		reflect.TypeOf(&gatewayv1.GatewayClassList{}):    "serviceimport-scoped rebuild should not list GatewayClasses",
		reflect.TypeOf(&gatewayv1.GatewayList{}):         "serviceimport-scoped rebuild should not list Gateways",
		reflect.TypeOf(&gatewayv1.HTTPRouteList{}):       "serviceimport-scoped rebuild should not list HTTPRoutes",
		reflect.TypeOf(&gatewayv1.GRPCRouteList{}):       "serviceimport-scoped rebuild should not list GRPCRoutes",
		reflect.TypeOf(&gatewayv1alpha2.TCPRouteList{}):  "serviceimport-scoped rebuild should not list TCPRoutes",
		reflect.TypeOf(&gatewayv1alpha2.UDPRouteList{}):  "serviceimport-scoped rebuild should not list UDPRoutes",
		reflect.TypeOf(&gatewayv1alpha2.TLSRouteList{}):  "serviceimport-scoped rebuild should not list TLSRoutes",
		reflect.TypeOf(&corev1.SecretList{}):             "serviceimport-scoped rebuild should not list Secrets",
		reflect.TypeOf(&corev1.ConfigMapList{}):          "serviceimport-scoped rebuild should not list ConfigMaps",
	}
	validatingClient.listValidators = map[reflect.Type]func(client.ListOptions) error{
		reflect.TypeOf(&discoveryv1.EndpointSliceList{}): requireEndpointSliceList(
			mcsv1alpha1.LabelServiceName,
		),
	}

	if _, err := syncer.Reconcile(
		context.Background(),
		snapshotBackendDependenciesReconcileRequestForServiceImport(client.ObjectKey{
			Namespace: "default",
			Name:      "echo",
		}),
	); err != nil {
		t.Fatalf("serviceimport-scoped Reconcile returned error: %v", err)
	}

	current = store.Current()
	if current == nil {
		t.Fatalf("expected snapshot after serviceimport-scoped rebuild")
	}
	if len(current.Backends) != 0 {
		t.Fatalf("expected serviceimport-scoped rebuild to drop missing backend, got %#v", current.Backends)
	}
	metadata := current.HTTPRoutes[0].Rules[0].BackendRefs[0].Metadata
	if metadata["nantian.dev/backend-ref-valid"] != "false" {
		t.Fatalf("expected serviceimport backend ref to become invalid, got %#v", metadata)
	}
	if metadata["nantian.dev/backend-ref-reason"] != string(gatewayv1.RouteReasonBackendNotFound) {
		t.Fatalf("expected missing serviceimport to surface backend-not-found, got %#v", metadata)
	}
	if len(current.Listeners) != 1 || len(current.Listeners[0].AttachedRoutes) != 1 {
		t.Fatalf("expected serviceimport-scoped rebuild to preserve listeners and attachments, got %#v", current.Listeners)
	}
}

func TestReconcileServiceScopedRequestRefreshesMeshListenersBackendsAndBackendRefs(t *testing.T) {
	scheme := newPartialRebuildTestScheme(t)
	servicePort := gatewayv1.PortNumber(8080)

	baseClient := newControllerClientBuilder(scheme).
		WithObjects(
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Kind: ptr[gatewayv1.Kind]("Service"),
							Name: "echo",
						}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "echo",
									Port: &servicePort,
								},
							},
						}},
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "http",
						Port:       8080,
						TargetPort: intstr.FromInt(8080),
						Protocol:   corev1.ProtocolTCP,
					}},
				},
			},
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "echo-1",
					Namespace: "default",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "echo",
					},
				},
				Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](8080)}},
				Endpoints: []discoveryv1.Endpoint{{
					Addresses: []string{"10.0.0.30"},
				}},
			},
		).
		Build()

	validatingClient := &partialRebuildValidatingClient{Client: baseClient}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	syncer := NewSyncer(
		validatingClient,
		translator.New("gateway.networking.k8s.io/nantian-gw", logger),
		store,
		testMetrics(),
		0,
		logger,
	)
	syncer.SetSettleDelay(0)

	if _, err := syncer.Reconcile(context.Background(), snapshotReconcileRequest); err != nil {
		t.Fatalf("initial Reconcile returned error: %v", err)
	}

	current := store.Current()
	if current == nil || len(current.Listeners) != 1 {
		t.Fatalf("expected initial mesh listener, got %#v", current)
	}
	if current.Listeners[0].Metadata[mesh.FrontendKindMetadataKey] != mesh.FrontendKindService {
		t.Fatalf("expected initial listener to be a mesh service listener, got %#v", current.Listeners[0].Metadata)
	}
	if len(current.Listeners[0].AttachedRoutes) != 1 {
		t.Fatalf("expected initial mesh listener attachment, got %#v", current.Listeners[0].AttachedRoutes)
	}
	if len(current.Backends) != 1 {
		t.Fatalf("expected initial backend, got %#v", current.Backends)
	}
	if metadata := current.HTTPRoutes[0].Rules[0].BackendRefs[0].Metadata; metadata["nantian.dev/backend-ref-valid"] != "" {
		t.Fatalf("expected service backend ref to start valid, got %#v", metadata)
	}

	if err := validatingClient.Delete(
		context.Background(),
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
		},
	); err != nil {
		t.Fatalf("delete service: %v", err)
	}
	validatingClient.forbiddenLists = map[reflect.Type]string{
		reflect.TypeOf(&gatewayv1.GatewayClassList{}):    "service-scoped rebuild should not list GatewayClasses",
		reflect.TypeOf(&gatewayv1.GatewayList{}):         "service-scoped rebuild should not list Gateways",
		reflect.TypeOf(&gatewayv1.HTTPRouteList{}):       "service-scoped rebuild should not list HTTPRoutes",
		reflect.TypeOf(&gatewayv1.GRPCRouteList{}):       "service-scoped rebuild should not list GRPCRoutes",
		reflect.TypeOf(&gatewayv1alpha2.TCPRouteList{}):  "service-scoped rebuild should not list TCPRoutes",
		reflect.TypeOf(&gatewayv1alpha2.UDPRouteList{}):  "service-scoped rebuild should not list UDPRoutes",
		reflect.TypeOf(&gatewayv1alpha2.TLSRouteList{}):  "service-scoped rebuild should not list TLSRoutes",
		reflect.TypeOf(&corev1.NamespaceList{}):          "service-scoped rebuild should not list Namespaces",
		reflect.TypeOf(&corev1.SecretList{}):             "service-scoped rebuild should not list Secrets",
		reflect.TypeOf(&corev1.ConfigMapList{}):          "service-scoped rebuild should not list ConfigMaps",
	}
	validatingClient.listValidators = map[reflect.Type]func(client.ListOptions) error{
		reflect.TypeOf(&discoveryv1.EndpointSliceList{}): requireEndpointSliceList(
			discoveryv1.LabelServiceName,
		),
	}

	if _, err := syncer.Reconcile(
		context.Background(),
		snapshotServiceDependenciesReconcileRequestForService(client.ObjectKey{
			Namespace: "default",
			Name:      "echo",
		}),
	); err != nil {
		t.Fatalf("service-scoped Reconcile returned error: %v", err)
	}

	current = store.Current()
	if current == nil {
		t.Fatalf("expected snapshot after service-scoped rebuild")
	}
	if len(current.Listeners) != 0 {
		t.Fatalf("expected service-scoped rebuild to drop mesh listeners for deleted service, got %#v", current.Listeners)
	}
	if len(current.Backends) != 0 {
		t.Fatalf("expected service-scoped rebuild to drop deleted service backends, got %#v", current.Backends)
	}
	metadata := current.HTTPRoutes[0].Rules[0].BackendRefs[0].Metadata
	if metadata["nantian.dev/backend-ref-valid"] != "false" {
		t.Fatalf("expected deleted service backend ref to become invalid, got %#v", metadata)
	}
	if metadata["nantian.dev/backend-ref-reason"] != string(gatewayv1.RouteReasonBackendNotFound) {
		t.Fatalf("expected deleted service to surface backend-not-found, got %#v", metadata)
	}
}

func TestReconcileServiceScopedRequestRefreshesOnlyAffectedBackendRefs(t *testing.T) {
	scheme := newPartialRebuildTestScheme(t)
	echoPort := gatewayv1.PortNumber(8080)
	sparePort := gatewayv1.PortNumber(9090)

	baseClient := newControllerClientBuilder(scheme).
		WithObjects(
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "http",
						Port:       8080,
						TargetPort: intstr.FromInt(8080),
						Protocol:   corev1.ProtocolTCP,
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "spare", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "http",
						Port:       9090,
						TargetPort: intstr.FromInt(9090),
						Protocol:   corev1.ProtocolTCP,
					}},
				},
			},
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "echo-1",
					Namespace: "default",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "echo",
					},
				},
				Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](8080)}},
				Endpoints: []discoveryv1.Endpoint{{
					Addresses: []string{"10.0.0.10"},
				}},
			},
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "spare-1",
					Namespace: "default",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "spare",
					},
				},
				Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](9090)}},
				Endpoints: []discoveryv1.Endpoint{{
					Addresses: []string{"10.0.0.90"},
				}},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route-echo", Namespace: "default"},
				Spec: gatewayv1.HTTPRouteSpec{
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "echo",
									Port: &echoPort,
								},
							},
						}},
					}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route-spare", Namespace: "default"},
				Spec: gatewayv1.HTTPRouteSpec{
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "spare",
									Port: &sparePort,
								},
							},
						}},
					}},
				},
			},
		).
		Build()

	validatingClient := &partialRebuildValidatingClient{Client: baseClient}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	syncer := NewSyncer(
		validatingClient,
		translator.New("gateway.networking.k8s.io/nantian-gw", logger),
		store,
		testMetrics(),
		0,
		logger,
	)
	syncer.SetSettleDelay(0)

	if _, err := syncer.Reconcile(context.Background(), snapshotReconcileRequest); err != nil {
		t.Fatalf("initial Reconcile returned error: %v", err)
	}

	current := store.Current()
	if current == nil || len(current.HTTPRoutes) != 2 || len(current.Backends) != 2 {
		t.Fatalf("expected initial snapshot with 2 routes and 2 backends, got %#v", current)
	}
	for idx := range current.HTTPRoutes {
		if current.HTTPRoutes[idx].Name != "route-spare" {
			continue
		}
		current.HTTPRoutes[idx].Rules[0].BackendRefs[0].Metadata = map[string]string{
			"nantian.dev/backend-ref-valid":  "false",
			"nantian.dev/backend-ref-reason": string(gatewayv1.RouteReasonBackendNotFound),
		}
	}

	if err := validatingClient.Delete(
		context.Background(),
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
		},
	); err != nil {
		t.Fatalf("delete service: %v", err)
	}
	validatingClient.forbiddenLists = map[reflect.Type]string{
		reflect.TypeOf(&gatewayv1.GatewayClassList{}):    "service-scoped rebuild should not list GatewayClasses",
		reflect.TypeOf(&gatewayv1.GatewayList{}):         "service-scoped rebuild should not list Gateways",
		reflect.TypeOf(&gatewayv1.HTTPRouteList{}):       "service-scoped rebuild should not list HTTPRoutes",
		reflect.TypeOf(&gatewayv1.GRPCRouteList{}):       "service-scoped rebuild should not list GRPCRoutes",
		reflect.TypeOf(&gatewayv1alpha2.TCPRouteList{}):  "service-scoped rebuild should not list TCPRoutes",
		reflect.TypeOf(&gatewayv1alpha2.UDPRouteList{}):  "service-scoped rebuild should not list UDPRoutes",
		reflect.TypeOf(&gatewayv1alpha2.TLSRouteList{}):  "service-scoped rebuild should not list TLSRoutes",
		reflect.TypeOf(&corev1.NamespaceList{}):          "service-scoped rebuild should not list Namespaces",
		reflect.TypeOf(&corev1.SecretList{}):             "service-scoped rebuild should not list Secrets",
		reflect.TypeOf(&corev1.ConfigMapList{}):          "service-scoped rebuild should not list ConfigMaps",
	}
	validatingClient.listValidators = map[reflect.Type]func(client.ListOptions) error{
		reflect.TypeOf(&discoveryv1.EndpointSliceList{}): requireEndpointSliceList(
			discoveryv1.LabelServiceName,
		),
	}

	if _, err := syncer.Reconcile(
		context.Background(),
		snapshotServiceDependenciesReconcileRequestForService(client.ObjectKey{
			Namespace: "default",
			Name:      "echo",
		}),
	); err != nil {
		t.Fatalf("service-scoped Reconcile returned error: %v", err)
	}

	current = store.Current()
	if current == nil {
		t.Fatalf("expected snapshot after service-scoped rebuild")
	}
	if len(current.Backends) != 1 || current.Backends[0].Name != "spare:9090" {
		t.Fatalf("expected service-scoped rebuild to preserve only untouched spare backend, got %#v", current.Backends)
	}

	routeMetadata := make(map[string]map[string]string, len(current.HTTPRoutes))
	for _, route := range current.HTTPRoutes {
		routeMetadata[route.Name] = route.Rules[0].BackendRefs[0].Metadata
	}
	if routeMetadata["route-echo"]["nantian.dev/backend-ref-valid"] != "false" {
		t.Fatalf("expected changed service route to become invalid, got %#v", routeMetadata["route-echo"])
	}
	if routeMetadata["route-echo"]["nantian.dev/backend-ref-reason"] != string(gatewayv1.RouteReasonBackendNotFound) {
		t.Fatalf("expected changed service route to surface backend-not-found, got %#v", routeMetadata["route-echo"])
	}
	if routeMetadata["route-spare"]["nantian.dev/backend-ref-valid"] != "false" {
		t.Fatalf("expected unrelated route metadata to stay untouched, got %#v", routeMetadata["route-spare"])
	}
	if routeMetadata["route-spare"]["nantian.dev/backend-ref-reason"] != string(gatewayv1.RouteReasonBackendNotFound) {
		t.Fatalf("expected unrelated route reason to stay untouched, got %#v", routeMetadata["route-spare"])
	}
}

func TestReconcilePodEventRefreshesMeshWorkloadsAfterPodIPAssignment(t *testing.T) {
	scheme := newPartialRebuildTestScheme(t)

	baseClient := newControllerClientBuilder(scheme).
		WithObjects(
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo-v1", Namespace: "gateway-conformance-mesh"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "http",
						Port:       80,
						TargetPort: intstr.FromInt(8080),
						Protocol:   corev1.ProtocolTCP,
					}},
				},
			},
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "consumer-a", Namespace: "gateway-conformance-mesh-consumer"},
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
				},
			},
		).
		Build()

	validatingClient := &partialRebuildValidatingClient{Client: baseClient}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	syncer := NewSyncer(
		validatingClient,
		translator.New("gateway.networking.k8s.io/nantian-gw", logger),
		store,
		testMetrics(),
		0,
		logger,
	)
	syncer.SetSettleDelay(0)

	if _, err := syncer.Reconcile(context.Background(), snapshotReconcileRequest); err != nil {
		t.Fatalf("initial Reconcile returned error: %v", err)
	}

	if err := validatingClient.Create(
		context.Background(),
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mesh-echo-add-header",
				Namespace: "gateway-conformance-mesh-consumer",
			},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{
						Kind:      ptr[gatewayv1.Kind]("Service"),
						Name:      "echo-v1",
						Namespace: ptr[gatewayv1.Namespace]("gateway-conformance-mesh"),
					}},
				},
				Rules: []gatewayv1.HTTPRouteRule{{}},
			},
		},
	); err != nil {
		t.Fatalf("create route: %v", err)
	}

	validatingClient.forbiddenLists = map[reflect.Type]string{
		reflect.TypeOf(&gatewayv1.HTTPRouteList{}):           "route-scoped mesh rebuild should not list HTTPRoutes",
		reflect.TypeOf(&gatewayv1.GRPCRouteList{}):           "route-scoped mesh rebuild should not list GRPCRoutes",
		reflect.TypeOf(&gatewayv1alpha2.TCPRouteList{}):      "route-scoped mesh rebuild should not list TCPRoutes",
		reflect.TypeOf(&gatewayv1alpha2.UDPRouteList{}):      "route-scoped mesh rebuild should not list UDPRoutes",
		reflect.TypeOf(&gatewayv1alpha2.TLSRouteList{}):      "route-scoped mesh rebuild should not list TLSRoutes",
		reflect.TypeOf(&gatewayv1beta1.ReferenceGrantList{}): "route-scoped mesh rebuild should not list ReferenceGrants",
		reflect.TypeOf(&corev1.SecretList{}):                 "route-scoped mesh rebuild should not list Secrets",
		reflect.TypeOf(&corev1.ConfigMapList{}):              "route-scoped mesh rebuild should not list ConfigMaps",
	}
	validatingClient.listValidators = map[reflect.Type]func(client.ListOptions) error{
		reflect.TypeOf(&corev1.PodList{}): requireNamespaceScopedListAny(
			[]string{
				"gateway-conformance-mesh-consumer",
				"gateway-conformance-mesh",
			},
			"pod",
		),
	}

	if _, err := syncer.Reconcile(
		context.Background(),
		snapshotScopedObjectReconcileRequest("snapshot-routes-http", types.NamespacedName{
			Namespace: "gateway-conformance-mesh-consumer",
			Name:      "mesh-echo-add-header",
		}),
	); err != nil {
		t.Fatalf("route-scoped mesh Reconcile returned error: %v", err)
	}

	current := store.Current()
	if current == nil || len(current.Workloads) != 0 {
		t.Fatalf("expected pending pod to keep workload view empty, got %#v", current)
	}

	var pod corev1.Pod
	if err := validatingClient.Get(
		context.Background(),
		client.ObjectKey{Namespace: "gateway-conformance-mesh-consumer", Name: "consumer-a"},
		&pod,
	); err != nil {
		t.Fatalf("get pod: %v", err)
	}
	pod.Status.Phase = corev1.PodRunning
	pod.Status.PodIP = "10.1.0.10"
	if err := validatingClient.Status().Update(context.Background(), &pod); err != nil {
		t.Fatalf("update pod status: %v", err)
	}

	workloadRequests := syncer.snapshotReconcileRequests(context.Background(), &pod)
	expectedWorkloadRequest := snapshotScopedObjectReconcileRequest(
		"snapshot-workloads",
		client.ObjectKey{},
	)
	if len(workloadRequests) != 1 || workloadRequests[0] != expectedWorkloadRequest {
		t.Fatalf("expected pod update to queue workload refresh %v, got %v", expectedWorkloadRequest, workloadRequests)
	}

	validatingClient.forbiddenLists = map[reflect.Type]string{
		reflect.TypeOf(&gatewayv1.GatewayClassList{}):        "workload-only rebuild should not list GatewayClasses",
		reflect.TypeOf(&gatewayv1.GatewayList{}):             "workload-only rebuild should not list Gateways",
		reflect.TypeOf(&gatewayv1.HTTPRouteList{}):           "workload-only rebuild should not list HTTPRoutes",
		reflect.TypeOf(&gatewayv1.GRPCRouteList{}):           "workload-only rebuild should not list GRPCRoutes",
		reflect.TypeOf(&gatewayv1alpha2.TCPRouteList{}):      "workload-only rebuild should not list TCPRoutes",
		reflect.TypeOf(&gatewayv1alpha2.UDPRouteList{}):      "workload-only rebuild should not list UDPRoutes",
		reflect.TypeOf(&gatewayv1alpha2.TLSRouteList{}):      "workload-only rebuild should not list TLSRoutes",
		reflect.TypeOf(&gatewayv1beta1.ReferenceGrantList{}): "workload-only rebuild should not list ReferenceGrants",
		reflect.TypeOf(&corev1.NamespaceList{}):              "workload-only rebuild should not list Namespaces",
		reflect.TypeOf(&corev1.SecretList{}):                 "workload-only rebuild should not list Secrets",
		reflect.TypeOf(&corev1.ConfigMapList{}):              "workload-only rebuild should not list ConfigMaps",
	}
	validatingClient.listValidators = map[reflect.Type]func(client.ListOptions) error{
		reflect.TypeOf(&corev1.PodList{}): requireNamespaceScopedListAny(
			[]string{
				"gateway-conformance-mesh-consumer",
				"gateway-conformance-mesh",
			},
			"pod",
		),
	}

	if _, err := syncer.Reconcile(context.Background(), workloadRequests[0]); err != nil {
		t.Fatalf("workload refresh Reconcile returned error: %v", err)
	}

	current = store.Current()
	if current == nil || len(current.Workloads) != 1 {
		t.Fatalf("expected workload refresh to pick up pod IP assignment, got %#v", current)
	}
	if got := current.Workloads[0].Namespace + "/" + current.Workloads[0].Name; got != "gateway-conformance-mesh-consumer/consumer-a" {
		t.Fatalf("unexpected workload set after pod-triggered refresh: %#v", current.Workloads)
	}
	if len(current.Listeners) != 1 || len(current.Listeners[0].AttachedRoutes) != 1 {
		t.Fatalf("expected workload-only refresh to preserve mesh listeners, got %#v", current.Listeners)
	}
}
