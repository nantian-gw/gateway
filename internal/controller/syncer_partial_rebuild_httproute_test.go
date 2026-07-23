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

func TestReconcileHTTPRouteScopedRequestRebuildsOnlyChangedRoute(t *testing.T) {
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
				ObjectMeta: metav1.ObjectMeta{Name: "route-a", Namespace: "default"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Name:        "gw",
							SectionName: ptr[gatewayv1.SectionName]("http"),
						}},
					},
					Hostnames: []gatewayv1.Hostname{"a.example.com"},
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
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route-b", Namespace: "default"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Name:        "gw",
							SectionName: ptr[gatewayv1.SectionName]("http"),
						}},
					},
					Hostnames: []gatewayv1.Hostname{"b.example.com"},
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
	if current == nil || len(current.HTTPRoutes) != 2 {
		t.Fatalf("expected initial routes, got %#v", current)
	}

	var route gatewayv1.HTTPRoute
	if err := validatingClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "route-a"}, &route); err != nil {
		t.Fatalf("get route-a: %v", err)
	}
	route.Spec.Hostnames = []gatewayv1.Hostname{"updated.example.com"}
	route.Spec.Rules[0].BackendRefs = []gatewayv1.HTTPBackendRef{{
		BackendRef: gatewayv1.BackendRef{
			BackendObjectReference: gatewayv1.BackendObjectReference{
				Name: "missing",
				Port: &servicePort,
			},
		},
	}}
	if err := validatingClient.Update(context.Background(), &route); err != nil {
		t.Fatalf("update route-a: %v", err)
	}

	validatingClient.forbiddenLists = map[reflect.Type]string{
		reflect.TypeOf(&gatewayv1.HTTPRouteList{}):           "route-scoped rebuild should not list HTTPRoutes",
		reflect.TypeOf(&gatewayv1.GRPCRouteList{}):           "route-scoped rebuild should not list GRPCRoutes",
		reflect.TypeOf(&gatewayv1alpha2.TCPRouteList{}):      "route-scoped rebuild should not list TCPRoutes",
		reflect.TypeOf(&gatewayv1alpha2.UDPRouteList{}):      "route-scoped rebuild should not list UDPRoutes",
		reflect.TypeOf(&gatewayv1alpha2.TLSRouteList{}):      "route-scoped rebuild should not list TLSRoutes",
		reflect.TypeOf(&corev1.ServiceList{}):                "route-scoped rebuild should not list Services",
		reflect.TypeOf(&mcsv1alpha1.ServiceImportList{}):     "route-scoped rebuild should not list ServiceImports",
		reflect.TypeOf(&discoveryv1.EndpointSliceList{}):     "route-scoped rebuild should not list EndpointSlices",
		reflect.TypeOf(&gatewayv1beta1.ReferenceGrantList{}): "route-scoped rebuild should not list ReferenceGrants",
		reflect.TypeOf(&corev1.SecretList{}):                 "route-scoped rebuild should not list Secrets",
		reflect.TypeOf(&corev1.ConfigMapList{}):              "route-scoped rebuild should not list ConfigMaps",
		reflect.TypeOf(&corev1.PodList{}):                    "route-scoped rebuild should not list Pods",
	}

	if _, err := syncer.Reconcile(
		context.Background(),
		snapshotScopedObjectReconcileRequest("snapshot-routes-http", types.NamespacedName{
			Namespace: "default",
			Name:      "route-a",
		}),
	); err != nil {
		t.Fatalf("route-scoped Reconcile returned error: %v", err)
	}

	current = store.Current()
	if current == nil || len(current.HTTPRoutes) != 2 {
		t.Fatalf("expected route-scoped rebuild to preserve route catalog, got %#v", current)
	}
	routeByKey := make(map[string]ir.HTTPRoute, len(current.HTTPRoutes))
	for _, item := range current.HTTPRoutes {
		routeByKey[item.Namespace+"/"+item.Name] = item
	}
	if got := routeByKey["default/route-a"].Hostnames; len(got) != 1 || got[0] != "updated.example.com" {
		t.Fatalf("route-a hostnames = %#v, want updated hostname", got)
	}
	if metadata := routeByKey["default/route-a"].Rules[0].BackendRefs[0].Metadata; metadata["nantian.dev/backend-ref-valid"] != "false" {
		t.Fatalf("expected route-a backend ref to become invalid, got %#v", metadata)
	}
	if got := routeByKey["default/route-b"].Hostnames; len(got) != 1 || got[0] != "b.example.com" {
		t.Fatalf("route-b hostnames = %#v, want unchanged hostnames", got)
	}
	if len(current.Listeners) != 1 || len(current.Listeners[0].AttachedRoutes) != 2 {
		t.Fatalf("expected route-scoped rebuild to preserve listener attachments, got %#v", current.Listeners)
	}
}

func TestReconcileHTTPRouteScopedRequestRefreshesMeshListenersForServiceParents(t *testing.T) {
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
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "http",
						Port:       8080,
						TargetPort: intstr.FromInt(8080),
						Protocol:   corev1.ProtocolTCP,
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
	if current == nil || len(current.Listeners) != 1 {
		t.Fatalf("expected initial mesh listener, got %#v", current)
	}
	if got := current.Listeners[0].Metadata[mesh.FrontendNameMetadataKey]; got != "echo" {
		t.Fatalf("expected initial mesh listener for echo, got %#v", current.Listeners[0].Metadata)
	}

	var route gatewayv1.HTTPRoute
	if err := validatingClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "route"}, &route); err != nil {
		t.Fatalf("get route: %v", err)
	}
	route.Spec.ParentRefs = []gatewayv1.ParentReference{{
		Kind: ptr[gatewayv1.Kind]("Service"),
		Name: "api",
	}}
	route.Spec.Rules[0].BackendRefs = []gatewayv1.HTTPBackendRef{{
		BackendRef: gatewayv1.BackendRef{
			BackendObjectReference: gatewayv1.BackendObjectReference{
				Name: "api",
				Port: &servicePort,
			},
		},
	}}
	if err := validatingClient.Update(context.Background(), &route); err != nil {
		t.Fatalf("update route: %v", err)
	}

	validatingClient.forbiddenLists = map[reflect.Type]string{
		reflect.TypeOf(&gatewayv1.HTTPRouteList{}):           "route-scoped mesh rebuild should not list HTTPRoutes",
		reflect.TypeOf(&gatewayv1.GRPCRouteList{}):           "route-scoped mesh rebuild should not list GRPCRoutes",
		reflect.TypeOf(&gatewayv1alpha2.TCPRouteList{}):      "route-scoped mesh rebuild should not list TCPRoutes",
		reflect.TypeOf(&gatewayv1alpha2.UDPRouteList{}):      "route-scoped mesh rebuild should not list UDPRoutes",
		reflect.TypeOf(&gatewayv1alpha2.TLSRouteList{}):      "route-scoped mesh rebuild should not list TLSRoutes",
		reflect.TypeOf(&corev1.ServiceList{}):                "route-scoped mesh rebuild should not list Services",
		reflect.TypeOf(&mcsv1alpha1.ServiceImportList{}):     "route-scoped mesh rebuild should not list ServiceImports",
		reflect.TypeOf(&discoveryv1.EndpointSliceList{}):     "route-scoped mesh rebuild should not list EndpointSlices",
		reflect.TypeOf(&gatewayv1beta1.ReferenceGrantList{}): "route-scoped mesh rebuild should not list ReferenceGrants",
		reflect.TypeOf(&corev1.SecretList{}):                 "route-scoped mesh rebuild should not list Secrets",
		reflect.TypeOf(&corev1.ConfigMapList{}):              "route-scoped mesh rebuild should not list ConfigMaps",
		reflect.TypeOf(&corev1.PodList{}):                    "route-scoped mesh rebuild should not list Pods",
	}

	if _, err := syncer.Reconcile(
		context.Background(),
		snapshotScopedObjectReconcileRequest("snapshot-routes-http", types.NamespacedName{
			Namespace: "default",
			Name:      "route",
		}),
	); err != nil {
		t.Fatalf("route-scoped mesh Reconcile returned error: %v", err)
	}

	current = store.Current()
	if current == nil || len(current.Listeners) != 1 {
		t.Fatalf("expected mesh listener after route-scoped rebuild, got %#v", current)
	}
	if got := current.Listeners[0].Metadata[mesh.FrontendNameMetadataKey]; got != "api" {
		t.Fatalf("expected mesh listener to move to api, got %#v", current.Listeners[0].Metadata)
	}
	if len(current.Listeners[0].AttachedRoutes) != 1 || current.Listeners[0].AttachedRoutes[0] != "default/route" {
		t.Fatalf("expected updated mesh listener attachment, got %#v", current.Listeners[0].AttachedRoutes)
	}
}

func TestReconcileHTTPRouteScopedRequestAttachesNewRouteToExistingGatewayListener(t *testing.T) {
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
					Addresses: []string{"10.0.0.10"},
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
	if current == nil || len(current.Listeners) != 1 || len(current.HTTPRoutes) != 0 {
		t.Fatalf("expected initial listener-only snapshot, got %#v", current)
	}
	if len(current.Listeners[0].AttachedRoutes) != 0 {
		t.Fatalf("expected no initial route attachments, got %#v", current.Listeners[0].AttachedRoutes)
	}

	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default"},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{{
					Name:        "gw",
					SectionName: ptr[gatewayv1.SectionName]("http"),
				}},
			},
			Hostnames: []gatewayv1.Hostname{"example.com"},
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
	}
	if err := validatingClient.Create(context.Background(), route); err != nil {
		t.Fatalf("create route: %v", err)
	}

	validatingClient.forbiddenLists = map[reflect.Type]string{
		reflect.TypeOf(&gatewayv1.HTTPRouteList{}):           "route-scoped rebuild should not list HTTPRoutes",
		reflect.TypeOf(&gatewayv1.GRPCRouteList{}):           "route-scoped rebuild should not list GRPCRoutes",
		reflect.TypeOf(&gatewayv1alpha2.TCPRouteList{}):      "route-scoped rebuild should not list TCPRoutes",
		reflect.TypeOf(&gatewayv1alpha2.UDPRouteList{}):      "route-scoped rebuild should not list UDPRoutes",
		reflect.TypeOf(&gatewayv1alpha2.TLSRouteList{}):      "route-scoped rebuild should not list TLSRoutes",
		reflect.TypeOf(&corev1.ServiceList{}):                "route-scoped rebuild should not list Services",
		reflect.TypeOf(&mcsv1alpha1.ServiceImportList{}):     "route-scoped rebuild should not list ServiceImports",
		reflect.TypeOf(&discoveryv1.EndpointSliceList{}):     "route-scoped rebuild should not list EndpointSlices",
		reflect.TypeOf(&gatewayv1beta1.ReferenceGrantList{}): "route-scoped rebuild should not list ReferenceGrants",
		reflect.TypeOf(&corev1.SecretList{}):                 "route-scoped rebuild should not list Secrets",
		reflect.TypeOf(&corev1.ConfigMapList{}):              "route-scoped rebuild should not list ConfigMaps",
		reflect.TypeOf(&corev1.PodList{}):                    "route-scoped rebuild should not list Pods",
	}

	if _, err := syncer.Reconcile(
		context.Background(),
		snapshotScopedObjectReconcileRequest("snapshot-routes-http", types.NamespacedName{
			Namespace: "default",
			Name:      "route",
		}),
	); err != nil {
		t.Fatalf("route-scoped Reconcile returned error: %v", err)
	}

	current = store.Current()
	if current == nil || len(current.HTTPRoutes) != 1 {
		t.Fatalf("expected route-scoped rebuild to add route, got %#v", current)
	}
	if len(current.Listeners) != 1 || len(current.Listeners[0].AttachedRoutes) != 1 {
		t.Fatalf("expected route-scoped rebuild to attach new route, got %#v", current.Listeners)
	}
	if got := current.Listeners[0].AttachedRoutes[0]; got != "default/route" {
		t.Fatalf("attached route = %q, want default/route", got)
	}
}

func TestReconcileHTTPRouteScopedRequestRebuildsMissingParentGatewayListener(t *testing.T) {
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
					Addresses: []string{"10.0.0.10"},
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
	if current == nil || len(current.Listeners) != 0 || len(current.HTTPRoutes) != 0 {
		t.Fatalf("expected initial empty snapshot, got %#v", current)
	}

	gateway := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "nantian-gw",
			Listeners: []gatewayv1.Listener{{
				Name:     "http",
				Protocol: gatewayv1.HTTPProtocolType,
				Port:     80,
			}},
		},
	}
	if err := validatingClient.Create(context.Background(), gateway); err != nil {
		t.Fatalf("create gateway: %v", err)
	}

	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default"},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{{
					Name:        "gw",
					SectionName: ptr[gatewayv1.SectionName]("http"),
				}},
			},
			Hostnames: []gatewayv1.Hostname{"example.com"},
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
	}
	if err := validatingClient.Create(context.Background(), route); err != nil {
		t.Fatalf("create route: %v", err)
	}

	validatingClient.forbiddenLists = map[reflect.Type]string{
		reflect.TypeOf(&gatewayv1.GatewayList{}):             "route-scoped rebuild should not list Gateways",
		reflect.TypeOf(&gatewayv1.HTTPRouteList{}):           "route-scoped rebuild should not list HTTPRoutes",
		reflect.TypeOf(&gatewayv1.GRPCRouteList{}):           "route-scoped rebuild should not list GRPCRoutes",
		reflect.TypeOf(&gatewayv1alpha2.TCPRouteList{}):      "route-scoped rebuild should not list TCPRoutes",
		reflect.TypeOf(&gatewayv1alpha2.UDPRouteList{}):      "route-scoped rebuild should not list UDPRoutes",
		reflect.TypeOf(&gatewayv1alpha2.TLSRouteList{}):      "route-scoped rebuild should not list TLSRoutes",
		reflect.TypeOf(&corev1.ServiceList{}):                "route-scoped rebuild should not list Services",
		reflect.TypeOf(&mcsv1alpha1.ServiceImportList{}):     "route-scoped rebuild should not list ServiceImports",
		reflect.TypeOf(&discoveryv1.EndpointSliceList{}):     "route-scoped rebuild should not list EndpointSlices",
		reflect.TypeOf(&gatewayv1beta1.ReferenceGrantList{}): "route-scoped rebuild should not list ReferenceGrants",
		reflect.TypeOf(&corev1.SecretList{}):                 "route-scoped rebuild should not list Secrets",
		reflect.TypeOf(&corev1.ConfigMapList{}):              "route-scoped rebuild should not list ConfigMaps",
		reflect.TypeOf(&corev1.PodList{}):                    "route-scoped rebuild should not list Pods",
	}

	if _, err := syncer.Reconcile(
		context.Background(),
		snapshotScopedObjectReconcileRequest("snapshot-routes-http", types.NamespacedName{
			Namespace: "default",
			Name:      "route",
		}),
	); err != nil {
		t.Fatalf("route-scoped Reconcile returned error: %v", err)
	}

	current = store.Current()
	if current == nil || len(current.HTTPRoutes) != 1 {
		t.Fatalf("expected route-scoped rebuild to add route, got %#v", current)
	}
	if len(current.Listeners) != 1 {
		t.Fatalf("expected route-scoped rebuild to recreate missing gateway listener, got %#v", current.Listeners)
	}
	if len(current.Listeners[0].AttachedRoutes) != 1 || current.Listeners[0].AttachedRoutes[0] != "default/route" {
		t.Fatalf("expected route-scoped rebuild to attach route to recreated listener, got %#v", current.Listeners[0].AttachedRoutes)
	}
}

func TestReconcileHTTPRouteScopedRequestRefreshesMeshWorkloadsForCrossNamespaceServiceParents(t *testing.T) {
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
					Phase: corev1.PodRunning,
					PodIP: "10.1.0.10",
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
	if current == nil {
		t.Fatalf("expected initial snapshot")
	}
	if len(current.Listeners) != 0 {
		t.Fatalf("expected initial snapshot without mesh listeners, got %#v", current.Listeners)
	}
	if len(current.Workloads) != 0 {
		t.Fatalf("expected initial snapshot without mesh workloads, got %#v", current.Workloads)
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
		reflect.TypeOf(&corev1.ServiceList{}):                "route-scoped mesh rebuild should not list Services",
		reflect.TypeOf(&mcsv1alpha1.ServiceImportList{}):     "route-scoped mesh rebuild should not list ServiceImports",
		reflect.TypeOf(&discoveryv1.EndpointSliceList{}):     "route-scoped mesh rebuild should not list EndpointSlices",
		reflect.TypeOf(&gatewayv1beta1.ReferenceGrantList{}): "route-scoped mesh rebuild should not list ReferenceGrants",
		reflect.TypeOf(&corev1.SecretList{}):                 "route-scoped mesh rebuild should not list Secrets",
		reflect.TypeOf(&corev1.ConfigMapList{}):              "route-scoped mesh rebuild should not list ConfigMaps",
	}
	validatingClient.listValidators = map[reflect.Type]func(client.ListOptions) error{
		reflect.TypeOf(&corev1.PodList{}): requireNamespaceScopedList(
			"gateway-conformance-mesh-consumer",
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

	current = store.Current()
	if current == nil || len(current.Listeners) != 1 {
		t.Fatalf("expected mesh listener after route-scoped rebuild, got %#v", current)
	}
	if got := current.Listeners[0].Metadata[mesh.FrontendNamespaceMetadataKey]; got != "gateway-conformance-mesh" {
		t.Fatalf("expected mesh listener namespace to stay on service parent, got %#v", current.Listeners[0].Metadata)
	}
	if got := current.Listeners[0].Metadata[mesh.FrontendNameMetadataKey]; got != "echo-v1" {
		t.Fatalf("expected mesh listener to target echo-v1, got %#v", current.Listeners[0].Metadata)
	}
	if len(current.Listeners[0].AttachedRoutes) != 1 || current.Listeners[0].AttachedRoutes[0] != "gateway-conformance-mesh-consumer/mesh-echo-add-header" {
		t.Fatalf("expected updated mesh listener attachment, got %#v", current.Listeners[0].AttachedRoutes)
	}
	if len(current.Workloads) != 1 {
		t.Fatalf("expected route-scoped mesh rebuild to refresh workloads, got %#v", current.Workloads)
	}
	if got := current.Workloads[0].Namespace + "/" + current.Workloads[0].Name; got != "gateway-conformance-mesh-consumer/consumer-a" {
		t.Fatalf("unexpected workload set after route-scoped rebuild: %#v", current.Workloads)
	}
}
