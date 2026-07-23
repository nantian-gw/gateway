package translator

import (
	"context"
	"io"
	"log/slog"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/translator/testutil"
)

func TestBuildRoutesForSnapshotAddsNewlyReferencedBackends(t *testing.T) {
	scheme := testutil.BuildSupportScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	servicePort := gatewayv1.PortNumber(80)

	routeV1 := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "route-v1", Namespace: "default"},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{{Name: "gw"}},
			},
			Rules: []gatewayv1.HTTPRouteRule{{
				BackendRefs: []gatewayv1.HTTPBackendRef{{
					BackendRef: gatewayv1.BackendRef{
						BackendObjectReference: gatewayv1.BackendObjectReference{
							Name: "echo-v1",
							Port: &servicePort,
						},
					},
				}},
			}},
		},
	}
	routeV2 := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "route-v2", Namespace: "default"},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{{Name: "gw"}},
			},
			Rules: []gatewayv1.HTTPRouteRule{{
				BackendRefs: []gatewayv1.HTTPBackendRef{{
					BackendRef: gatewayv1.BackendRef{
						BackendObjectReference: gatewayv1.BackendObjectReference{
							Name: "echo-v2",
							Port: &servicePort,
						},
					},
				}},
			}},
		},
	}

	cl := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
					Labels: map[string]string{
						"kubernetes.io/metadata.name": "default",
					},
				},
			},
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
				ObjectMeta: metav1.ObjectMeta{Name: "echo-v1", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "http",
						Port:       80,
						TargetPort: intstr.FromInt(8080),
						Protocol:   corev1.ProtocolTCP,
					}},
				},
			},
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "echo-v1-1",
					Namespace: "default",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "echo-v1",
					},
				},
				Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](8080)}},
				Endpoints: []discoveryv1.Endpoint{{
					Addresses: []string{"10.0.0.11"},
				}},
			},
			routeV1,
		).
		Build()

	translator := New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	current, err := translator.Build(context.Background(), cl)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(current.Backends) != 1 || current.Backends[0].Name != "echo-v1:80" {
		t.Fatalf("expected current snapshot to only contain echo-v1 backend, got %#v", current.Backends)
	}

	for _, object := range []client.Object{
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "echo-v2", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt(8080),
					Protocol:   corev1.ProtocolTCP,
				}},
			},
		},
		&discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "echo-v2-1",
				Namespace: "default",
				Labels: map[string]string{
					discoveryv1.LabelServiceName: "echo-v2",
				},
			},
			Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](8080)}},
			Endpoints: []discoveryv1.Endpoint{{
				Addresses: []string{"10.0.0.12"},
			}},
		},
		routeV2,
	} {
		if err := cl.Create(context.Background(), object); err != nil {
			t.Fatalf("create %T: %v", object, err)
		}
	}

	next, err := translator.BuildRoutesForSnapshot(
		context.Background(),
		cl,
		current,
		[]client.ObjectKey{{Namespace: "default", Name: "route-v2"}},
		nil,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("BuildRoutesForSnapshot returned error: %v", err)
	}

	if len(next.HTTPRoutes) != 2 {
		t.Fatalf("expected rebuilt snapshot to contain both routes, got %#v", next.HTTPRoutes)
	}

	backendByName := make(map[string]string, len(next.Backends))
	for _, backend := range next.Backends {
		backendByName[backend.Name] = backend.Endpoints[0].Address
	}
	if got := backendByName["echo-v1:80"]; got != "10.0.0.11" {
		t.Fatalf("echo-v1 backend address = %q, want %q", got, "10.0.0.11")
	}
	if got := backendByName["echo-v2:80"]; got != "10.0.0.12" {
		t.Fatalf("echo-v2 backend address = %q, want %q (all backends: %#v)", got, "10.0.0.12", next.Backends)
	}
}

func TestBuildRoutesForSnapshotAttachesListenerSetParentRoute(t *testing.T) {
	scheme := testutil.BuildSupportScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	servicePort := gatewayv1.PortNumber(80)
	listenerHostname := gatewayv1.Hostname("listener-set.example.com")
	parentGroup := gatewayv1.Group(gatewayv1.GroupName)
	listenerSetKind := gatewayv1.Kind("ListenerSet")

	cl := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
					Labels: map[string]string{
						"kubernetes.io/metadata.name": "default",
					},
				},
			},
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
					AllowedListeners: &gatewayv1.AllowedListeners{
						Namespaces: &gatewayv1.ListenerNamespaces{
							From: ptr(gatewayv1.NamespacesFromAll),
						},
					},
				},
			},
			&gatewayv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "default"},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{Name: "gw"},
					Listeners: []gatewayv1.ListenerEntry{{
						Name:     "ls-listener",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
						Hostname: &listenerHostname,
						AllowedRoutes: &gatewayv1.AllowedRoutes{
							Namespaces: &gatewayv1.RouteNamespaces{
								From: ptr(gatewayv1.NamespacesFromAll),
							},
						},
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "http",
						Port:       80,
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

	translator := New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	current, err := translator.Build(context.Background(), cl)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(current.HTTPRoutes) != 0 {
		t.Fatalf("expected initial snapshot without HTTPRoutes, got %#v", current.HTTPRoutes)
	}
	if got := listenerAttachedRoutes(current.Listeners, "default/gw/default/ls/ls-listener"); len(got) != 0 {
		t.Fatalf("expected initial ListenerSet listener without attachments, got %#v", got)
	}

	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "ls-route", Namespace: "default"},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{{
					Group: &parentGroup,
					Kind:  &listenerSetKind,
					Name:  "ls",
				}},
			},
			Hostnames: []gatewayv1.Hostname{listenerHostname},
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
	if err := cl.Create(context.Background(), route); err != nil {
		t.Fatalf("create route: %v", err)
	}

	next, err := translator.BuildRoutesForSnapshot(
		context.Background(),
		cl,
		current,
		[]client.ObjectKey{{Namespace: "default", Name: "ls-route"}},
		nil,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("BuildRoutesForSnapshot returned error: %v", err)
	}

	if got := listenerAttachedRoutes(next.Listeners, "default/gw/default/ls/ls-listener"); len(got) != 1 || got[0] != "default/ls-route" {
		t.Fatalf("expected route-scoped rebuild to attach ListenerSet route, got %#v", got)
	}
}

func TestBuildRoutesForSnapshotRebuildsMissingListenerSetListenersBeforeAttachingRoutes(t *testing.T) {
	scheme := testutil.BuildSupportScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	servicePort := gatewayv1.PortNumber(80)
	parentGroup := gatewayv1.Group(gatewayv1.GroupName)
	listenerSetKind := gatewayv1.Kind("ListenerSet")
	listenerHostname := gatewayv1.Hostname("listener-set.example.com")

	cl := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
					Labels: map[string]string{
						"kubernetes.io/metadata.name": "default",
					},
				},
			},
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
						Name:     "gateway-listener",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
					AllowedListeners: &gatewayv1.AllowedListeners{
						Namespaces: &gatewayv1.ListenerNamespaces{
							From: ptr(gatewayv1.NamespacesFromAll),
						},
					},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "http",
						Port:       80,
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

	translator := New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	current, err := translator.Build(context.Background(), cl)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if got := listenerAttachedRoutes(current.Listeners, "default/gw/default/ls/ls-listener"); len(got) != 0 {
		t.Fatalf("expected snapshot without ListenerSet listener attachments, got %#v", got)
	}

	listenerSet := &gatewayv1.ListenerSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "default"},
		Spec: gatewayv1.ListenerSetSpec{
			ParentRef: gatewayv1.ParentGatewayReference{Name: "gw"},
			Listeners: []gatewayv1.ListenerEntry{{
				Name:     "ls-listener",
				Protocol: gatewayv1.HTTPProtocolType,
				Port:     80,
				Hostname: &listenerHostname,
				AllowedRoutes: &gatewayv1.AllowedRoutes{
					Namespaces: &gatewayv1.RouteNamespaces{
						From: ptr(gatewayv1.NamespacesFromAll),
					},
				},
			}},
		},
	}
	if err := cl.Create(context.Background(), listenerSet); err != nil {
		t.Fatalf("create listenerset: %v", err)
	}

	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "ls-route", Namespace: "default"},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{{
					Group: &parentGroup,
					Kind:  &listenerSetKind,
					Name:  "ls",
				}},
			},
			Hostnames: []gatewayv1.Hostname{listenerHostname},
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
	if err := cl.Create(context.Background(), route); err != nil {
		t.Fatalf("create route: %v", err)
	}

	next, err := translator.BuildRoutesForSnapshot(
		context.Background(),
		cl,
		current,
		[]client.ObjectKey{{Namespace: "default", Name: "ls-route"}},
		nil,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("BuildRoutesForSnapshot returned error: %v", err)
	}

	if got := listenerAttachedRoutes(next.Listeners, "default/gw/default/ls/ls-listener"); len(got) != 1 || got[0] != "default/ls-route" {
		t.Fatalf("expected route-scoped rebuild to load missing ListenerSet listener and attach route, got %#v", got)
	}
}

func TestBuildRoutesForSnapshotAttachesListenerSetRoutesForMixedAllowedRoutesNamespaces(t *testing.T) {
	scheme := testutil.BuildSupportScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	servicePort := gatewayv1.PortNumber(8080)
	allNamespaces := gatewayv1.NamespacesFromAll
	sameNamespace := gatewayv1.NamespacesFromSame
	selectorNamespaces := gatewayv1.NamespacesFromSelector
	parentGroup := gatewayv1.Group(gatewayv1.GroupName)
	listenerSetKind := gatewayv1.Kind("ListenerSet")

	cl := testutil.NewTranslatorClientBuilder(scheme).
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
					Name: "gateway-api-routes-allowed-ns",
					Labels: map[string]string{
						"allowed":                     "ns",
						"kubernetes.io/metadata.name": "gateway-api-routes-allowed-ns",
					},
				},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gateway-api-routes-not-allowed-ns",
					Labels: map[string]string{
						"kubernetes.io/metadata.name": "gateway-api-routes-not-allowed-ns",
					},
				},
			},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway-with-listener-sets-test-allowed-routes",
					Namespace: "gateway-conformance-infra",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "gateway-listener",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
						Hostname: ptr(gatewayv1.Hostname("gateway-listener.com")),
						AllowedRoutes: &gatewayv1.AllowedRoutes{
							Namespaces: &gatewayv1.RouteNamespaces{From: &allNamespaces},
						},
					}},
					AllowedListeners: &gatewayv1.AllowedListeners{
						Namespaces: &gatewayv1.ListenerNamespaces{
							From: ptr(gatewayv1.NamespacesFromAll),
						},
					},
				},
			},
			&gatewayv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "listenerset-test-allowed-routes-namespaces",
					Namespace: "gateway-conformance-infra",
				},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Name:      "gateway-with-listener-sets-test-allowed-routes",
						Namespace: ptr(gatewayv1.Namespace("gateway-conformance-infra")),
					},
					Listeners: []gatewayv1.ListenerEntry{
						{
							Name:     "listener-set-listener-allowed-routes-all",
							Protocol: gatewayv1.HTTPProtocolType,
							Port:     80,
							Hostname: ptr(gatewayv1.Hostname("listener-set-listener-allowed-routes-all.com")),
							AllowedRoutes: &gatewayv1.AllowedRoutes{
								Namespaces: &gatewayv1.RouteNamespaces{From: &allNamespaces},
							},
						},
						{
							Name:     "listener-set-listener-allowed-routes-same",
							Protocol: gatewayv1.HTTPProtocolType,
							Port:     80,
							Hostname: ptr(gatewayv1.Hostname("listener-set-listener-allowed-routes-same.com")),
							AllowedRoutes: &gatewayv1.AllowedRoutes{
								Namespaces: &gatewayv1.RouteNamespaces{From: &sameNamespace},
							},
						},
						{
							Name:     "listener-set-listener-allowed-routes-selector",
							Protocol: gatewayv1.HTTPProtocolType,
							Port:     80,
							Hostname: ptr(gatewayv1.Hostname("listener-set-listener-allowed-routes-selector.com")),
							AllowedRoutes: &gatewayv1.AllowedRoutes{
								Namespaces: &gatewayv1.RouteNamespaces{
									From: &selectorNamespaces,
									Selector: &metav1.LabelSelector{
										MatchLabels: map[string]string{"allowed": "ns"},
									},
								},
							},
						},
					},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "infra-backend-v1", Namespace: "gateway-conformance-infra"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Name: "http", Port: 8080, TargetPort: intstr.FromInt(8080), Protocol: corev1.ProtocolTCP}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "infra-backend-v2", Namespace: "gateway-conformance-infra"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Name: "http", Port: 8080, TargetPort: intstr.FromInt(8080), Protocol: corev1.ProtocolTCP}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "infra-backend-v3", Namespace: "gateway-conformance-infra"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Name: "http", Port: 8080, TargetPort: intstr.FromInt(8080), Protocol: corev1.ProtocolTCP}},
				},
			},
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "infra-backend-v1-1",
					Namespace: "gateway-conformance-infra",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "infra-backend-v1",
					},
				},
				Ports:     []discoveryv1.EndpointPort{{Port: ptr[int32](8080)}},
				Endpoints: []discoveryv1.Endpoint{{Addresses: []string{"10.0.0.10"}}},
			},
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "infra-backend-v2-1",
					Namespace: "gateway-conformance-infra",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "infra-backend-v2",
					},
				},
				Ports:     []discoveryv1.EndpointPort{{Port: ptr[int32](8080)}},
				Endpoints: []discoveryv1.Endpoint{{Addresses: []string{"10.0.0.11"}}},
			},
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "infra-backend-v3-1",
					Namespace: "gateway-conformance-infra",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "infra-backend-v3",
					},
				},
				Ports:     []discoveryv1.EndpointPort{{Port: ptr[int32](8080)}},
				Endpoints: []discoveryv1.Endpoint{{Addresses: []string{"10.0.0.12"}}},
			},
		).
		Build()

	translator := New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	current, err := translator.Build(context.Background(), cl)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(current.HTTPRoutes) != 0 {
		t.Fatalf("expected initial snapshot without HTTPRoutes, got %#v", current.HTTPRoutes)
	}

	routes := []*gatewayv1.HTTPRoute{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "route-in-same-namespace",
				Namespace: "gateway-conformance-infra",
			},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{
						Group: &parentGroup,
						Kind:  &listenerSetKind,
						Name:  "listenerset-test-allowed-routes-namespaces",
					}},
				},
				Rules: []gatewayv1.HTTPRouteRule{{
					BackendRefs: []gatewayv1.HTTPBackendRef{{
						BackendRef: gatewayv1.BackendRef{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Name: "infra-backend-v1",
								Port: &servicePort,
							},
						},
					}},
				}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "route-in-selected-namespace",
				Namespace: "gateway-api-routes-allowed-ns",
			},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{
						Group:     &parentGroup,
						Kind:      &listenerSetKind,
						Name:      "listenerset-test-allowed-routes-namespaces",
						Namespace: ptr(gatewayv1.Namespace("gateway-conformance-infra")),
					}},
				},
				Rules: []gatewayv1.HTTPRouteRule{{
					BackendRefs: []gatewayv1.HTTPBackendRef{{
						BackendRef: gatewayv1.BackendRef{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Name:      "infra-backend-v2",
								Namespace: ptr(gatewayv1.Namespace("gateway-conformance-infra")),
								Port:      &servicePort,
							},
						},
					}},
				}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "route-not-in-selected-namespace",
				Namespace: "gateway-api-routes-not-allowed-ns",
			},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{
						Group:     &parentGroup,
						Kind:      &listenerSetKind,
						Name:      "listenerset-test-allowed-routes-namespaces",
						Namespace: ptr(gatewayv1.Namespace("gateway-conformance-infra")),
					}},
				},
				Rules: []gatewayv1.HTTPRouteRule{{
					BackendRefs: []gatewayv1.HTTPBackendRef{{
						BackendRef: gatewayv1.BackendRef{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Name:      "infra-backend-v3",
								Namespace: ptr(gatewayv1.Namespace("gateway-conformance-infra")),
								Port:      &servicePort,
							},
						},
					}},
				}},
			},
		},
	}
	for _, route := range routes {
		if err := cl.Create(context.Background(), route); err != nil {
			t.Fatalf("create route %s/%s: %v", route.Namespace, route.Name, err)
		}
	}

	next, err := translator.BuildRoutesForSnapshot(
		context.Background(),
		cl,
		current,
		[]client.ObjectKey{
			{Namespace: "gateway-conformance-infra", Name: "route-in-same-namespace"},
			{Namespace: "gateway-api-routes-allowed-ns", Name: "route-in-selected-namespace"},
			{Namespace: "gateway-api-routes-not-allowed-ns", Name: "route-not-in-selected-namespace"},
		},
		nil,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("BuildRoutesForSnapshot returned error: %v", err)
	}

	if got := listenerAttachedRoutes(next.Listeners, "gateway-conformance-infra/gateway-with-listener-sets-test-allowed-routes/gateway-conformance-infra/listenerset-test-allowed-routes-namespaces/listener-set-listener-allowed-routes-all"); len(got) != 3 {
		t.Fatalf("all listener attached routes = %#v, want 3 routes", got)
	}
	if got := listenerAttachedRoutes(next.Listeners, "gateway-conformance-infra/gateway-with-listener-sets-test-allowed-routes/gateway-conformance-infra/listenerset-test-allowed-routes-namespaces/listener-set-listener-allowed-routes-same"); len(got) != 1 || got[0] != "gateway-conformance-infra/route-in-same-namespace" {
		t.Fatalf("same listener attached routes = %#v, want only same-namespace route", got)
	}
	if got := listenerAttachedRoutes(next.Listeners, "gateway-conformance-infra/gateway-with-listener-sets-test-allowed-routes/gateway-conformance-infra/listenerset-test-allowed-routes-namespaces/listener-set-listener-allowed-routes-selector"); len(got) != 1 || got[0] != "gateway-api-routes-allowed-ns/route-in-selected-namespace" {
		t.Fatalf("selector listener attached routes = %#v, want only selected-namespace route", got)
	}
}

func TestBuildRoutesForSnapshotUsesDirectParentListenerSetWhenGatewayListIsStale(t *testing.T) {
	scheme := testutil.BuildSupportScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	servicePort := gatewayv1.PortNumber(80)
	parentGroup := gatewayv1.Group(gatewayv1.GroupName)
	listenerSetKind := gatewayv1.Kind("ListenerSet")
	ls1Hostname := gatewayv1.Hostname("listener-set-1.example.com")
	ls2Hostname := gatewayv1.Hostname("listener-set-2.example.com")

	baseClient := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
					Labels: map[string]string{
						"kubernetes.io/metadata.name": "default",
					},
				},
			},
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
						Name:     "gateway-listener",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
					AllowedListeners: &gatewayv1.AllowedListeners{
						Namespaces: &gatewayv1.ListenerNamespaces{
							From: ptr(gatewayv1.NamespacesFromAll),
						},
					},
				},
			},
			&gatewayv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{Name: "ls-1", Namespace: "default"},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{Name: "gw"},
					Listeners: []gatewayv1.ListenerEntry{{
						Name:     "listener-1",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
						Hostname: &ls1Hostname,
						AllowedRoutes: &gatewayv1.AllowedRoutes{
							Namespaces: &gatewayv1.RouteNamespaces{
								From: ptr(gatewayv1.NamespacesFromAll),
							},
						},
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "http",
						Port:       80,
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

	translator := New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	current, err := translator.Build(context.Background(), baseClient)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	listenerSet := &gatewayv1.ListenerSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ls-2", Namespace: "default"},
		Spec: gatewayv1.ListenerSetSpec{
			ParentRef: gatewayv1.ParentGatewayReference{Name: "gw"},
			Listeners: []gatewayv1.ListenerEntry{{
				Name:     "listener-2",
				Protocol: gatewayv1.HTTPProtocolType,
				Port:     80,
				Hostname: &ls2Hostname,
				AllowedRoutes: &gatewayv1.AllowedRoutes{
					Namespaces: &gatewayv1.RouteNamespaces{
						From: ptr(gatewayv1.NamespacesFromAll),
					},
				},
			}},
		},
	}
	if err := baseClient.Create(context.Background(), listenerSet); err != nil {
		t.Fatalf("create listenerset: %v", err)
	}

	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "ls-2-route", Namespace: "default"},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{{
					Group: &parentGroup,
					Kind:  &listenerSetKind,
					Name:  "ls-2",
				}},
			},
			Hostnames: []gatewayv1.Hostname{ls2Hostname},
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
	if err := baseClient.Create(context.Background(), route); err != nil {
		t.Fatalf("create route: %v", err)
	}

	staleClient := &staleListenerSetListClient{
		Client: baseClient,
		staleByGateway: map[string][]gatewayv1.ListenerSet{
			"default/gw": {{
				ObjectMeta: metav1.ObjectMeta{Name: "ls-1", Namespace: "default"},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{Name: "gw"},
					Listeners: []gatewayv1.ListenerEntry{{
						Name:     "listener-1",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
						Hostname: &ls1Hostname,
					}},
				},
			}},
		},
	}

	next, err := translator.BuildRoutesForSnapshot(
		context.Background(),
		staleClient,
		current,
		[]client.ObjectKey{{Namespace: "default", Name: "ls-2-route"}},
		nil,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("BuildRoutesForSnapshot returned error: %v", err)
	}

	if got := listenerAttachedRoutes(next.Listeners, "default/gw/default/ls-2/listener-2"); len(got) != 1 || got[0] != "default/ls-2-route" {
		t.Fatalf("expected route-scoped rebuild to use directly referenced ListenerSet when gateway listener list is stale, got %#v", got)
	}
}

func TestBuildRoutesForSnapshotRebuildsSecondListenerSetForSharedAndSpecificRoutes(t *testing.T) {
	scheme := testutil.BuildSupportScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	servicePort := gatewayv1.PortNumber(80)
	parentGroup := gatewayv1.Group(gatewayv1.GroupName)
	parentKind := gatewayv1.Kind("ListenerSet")
	gatewayHostnameOne := gatewayv1.Hostname("gateway-listener-1.example.com")
	gatewayHostnameTwo := gatewayv1.Hostname("gateway-listener-2.example.com")
	ls1Hostname := gatewayv1.Hostname("listener-set-1.example.com")
	ls2Hostname := gatewayv1.Hostname("listener-set-2.example.com")

	cl := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
					Labels: map[string]string{
						"kubernetes.io/metadata.name": "default",
					},
				},
			},
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
					AllowedListeners: &gatewayv1.AllowedListeners{
						Namespaces: &gatewayv1.ListenerNamespaces{
							From: ptr(gatewayv1.NamespacesFromAll),
						},
					},
					Listeners: []gatewayv1.Listener{
						{
							Name:     "gateway-listener-1",
							Protocol: gatewayv1.HTTPProtocolType,
							Port:     80,
							Hostname: &gatewayHostnameOne,
						},
						{
							Name:     "gateway-listener-2",
							Protocol: gatewayv1.HTTPProtocolType,
							Port:     80,
							Hostname: &gatewayHostnameTwo,
						},
					},
				},
			},
			&gatewayv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{Name: "ls-1", Namespace: "default"},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{Name: "gw"},
					Listeners: []gatewayv1.ListenerEntry{{
						Name:     "listener-1",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
						Hostname: &ls1Hostname,
						AllowedRoutes: &gatewayv1.AllowedRoutes{
							Namespaces: &gatewayv1.RouteNamespaces{
								From: ptr(gatewayv1.NamespacesFromAll),
							},
						},
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "http",
						Port:       80,
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

	translator := New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	current, err := translator.Build(context.Background(), cl)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if got := listenerAttachedRoutes(current.Listeners, "default/gw/default/ls-2/listener-2"); len(got) != 0 {
		t.Fatalf("expected baseline snapshot to omit second ListenerSet listener attachments, got %#v", got)
	}

	for _, object := range []client.Object{
		&gatewayv1.ListenerSet{
			ObjectMeta: metav1.ObjectMeta{Name: "ls-2", Namespace: "default"},
			Spec: gatewayv1.ListenerSetSpec{
				ParentRef: gatewayv1.ParentGatewayReference{Name: "gw"},
				Listeners: []gatewayv1.ListenerEntry{{
					Name:     "listener-2",
					Protocol: gatewayv1.HTTPProtocolType,
					Port:     80,
					Hostname: &ls2Hostname,
					AllowedRoutes: &gatewayv1.AllowedRoutes{
						Namespaces: &gatewayv1.RouteNamespaces{
							From: ptr(gatewayv1.NamespacesFromAll),
						},
					},
				}},
			},
		},
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "all-listeners-route", Namespace: "default"},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{
						{Name: "gw"},
						{
							Group: &parentGroup,
							Kind:  &parentKind,
							Name:  "ls-1",
						},
						{
							Group: &parentGroup,
							Kind:  &parentKind,
							Name:  "ls-2",
						},
					},
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
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "ls-2-route", Namespace: "default"},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{
						Group: &parentGroup,
						Kind:  &parentKind,
						Name:  "ls-2",
					}},
				},
				Hostnames: []gatewayv1.Hostname{ls2Hostname},
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
	} {
		if err := cl.Create(context.Background(), object); err != nil {
			t.Fatalf("create %T: %v", object, err)
		}
	}

	next, err := translator.BuildRoutesForSnapshot(
		context.Background(),
		cl,
		current,
		[]client.ObjectKey{
			{Namespace: "default", Name: "all-listeners-route"},
			{Namespace: "default", Name: "ls-2-route"},
		},
		nil,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("BuildRoutesForSnapshot returned error: %v", err)
	}

	if got := listenerAttachedRoutes(next.Listeners, "default/gw/gateway-listener-1"); len(got) != 1 || got[0] != "default/all-listeners-route" {
		t.Fatalf("gateway listener attached routes = %#v, want only default/all-listeners-route", got)
	}
	if got := listenerAttachedRoutes(next.Listeners, "default/gw/default/ls-1/listener-1"); len(got) != 1 || got[0] != "default/all-listeners-route" {
		t.Fatalf("first ListenerSet attached routes = %#v, want only default/all-listeners-route", got)
	}
	if got := listenerAttachedRoutes(next.Listeners, "default/gw/default/ls-2/listener-2"); len(got) != 2 || got[0] != "default/all-listeners-route" || got[1] != "default/ls-2-route" {
		t.Fatalf("second ListenerSet attached routes = %#v, want shared and ListenerSet-specific routes", got)
	}
}

type staleListenerSetListClient struct {
	client.Client
	staleByGateway map[string][]gatewayv1.ListenerSet
	served         map[string]bool
}

func (c *staleListenerSetListClient) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	listenerSets, ok := list.(*gatewayv1.ListenerSetList)
	if !ok {
		return c.Client.List(ctx, list, opts...)
	}

	var listOptions client.ListOptions
	for _, opt := range opts {
		opt.ApplyToList(&listOptions)
	}

	if c.served == nil {
		c.served = make(map[string]bool, len(c.staleByGateway))
	}
	for gatewayKey, items := range c.staleByGateway {
		if c.served[gatewayKey] {
			continue
		}
		if listOptions.FieldSelector == nil || !listOptions.FieldSelector.Matches(fields.Set{
			listenerSetParentGatewayFieldIndex: gatewayKey,
		}) {
			continue
		}

		listenerSets.Items = append([]gatewayv1.ListenerSet(nil), items...)
		c.served[gatewayKey] = true
		return nil
	}

	return c.Client.List(ctx, list, opts...)
}

func TestBuildRoutesForSnapshotAddsTLSRouteTerminateStreamRoute(t *testing.T) {
	scheme := testutil.BuildSupportScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	terminateMode := gatewayv1.TLSModeTerminate
	servicePort := gatewayv1.PortNumber(80)

	tlsRoute := &gatewayv1alpha2.TLSRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "tls-route", Namespace: "default"},
		Spec: gatewayv1alpha2.TLSRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{{Name: "gw-tls"}},
			},
			Hostnames: []gatewayv1.Hostname{"example.com"},
			Rules: []gatewayv1alpha2.TLSRouteRule{{
				BackendRefs: []gatewayv1alpha2.BackendRef{{
					BackendObjectReference: gatewayv1.BackendObjectReference{
						Name: "echo",
						Port: &servicePort,
					},
				}},
			}},
		},
	}

	cl := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
					Labels: map[string]string{
						"kubernetes.io/metadata.name": "default",
					},
				},
			},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw-tls", Namespace: "default"},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "tls",
						Protocol: gatewayv1.TLSProtocolType,
						Port:     443,
						TLS: &gatewayv1.ListenerTLSConfig{
							Mode: &terminateMode,
							CertificateRefs: []gatewayv1.SecretObjectReference{{
								Name: "gateway-cert",
							}},
						},
					}},
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "gateway-cert", Namespace: "default"},
				Type:       corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": testutil.ReadTestTLSAsset(t, "client.crt"),
					"tls.key": testutil.ReadTestTLSAsset(t, "client.key"),
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "http",
						Port:       80,
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
					Addresses: []string{"10.0.0.80"},
				}},
			},
		).
		Build()

	translator := New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	current, err := translator.Build(context.Background(), cl)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(current.HTTPRoutes) != 0 || len(current.StreamRoutes) != 0 {
		t.Fatalf("expected route-free baseline snapshot, got HTTP=%#v Stream=%#v", current.HTTPRoutes, current.StreamRoutes)
	}

	if err := cl.Create(context.Background(), tlsRoute); err != nil {
		t.Fatalf("create TLSRoute: %v", err)
	}

	next, err := translator.BuildRoutesForSnapshot(
		context.Background(),
		cl,
		current,
		nil,
		nil,
		nil,
		nil,
		[]client.ObjectKey{{Namespace: "default", Name: "tls-route"}},
	)
	if err != nil {
		t.Fatalf("BuildRoutesForSnapshot returned error: %v", err)
	}

	if len(next.HTTPRoutes) != 0 {
		t.Fatalf("expected no TLS-derived HTTPRoutes, got %#v", next.HTTPRoutes)
	}
	if len(next.StreamRoutes) != 1 {
		t.Fatalf("expected one TLS StreamRoute, got %#v", next.StreamRoutes)
	}
	if got := next.StreamRoutes[0].Rules[0].Matches[0].Mode; got != ir.TlsRouteModeTerminate {
		t.Fatalf("TLS StreamRoute match mode = %q, want %q", got, ir.TlsRouteModeTerminate)
	}
	if got := next.StreamRoutes[0].Rules[0].BackendRefs[0].Name; got != "echo" {
		t.Fatalf("TLS StreamRoute backend = %q, want echo", got)
	}
}

func listenerAttachedRoutes(listeners []ir.Listener, name string) []string {
	for _, listener := range listeners {
		if listener.Name == name {
			return listener.AttachedRoutes
		}
	}
	return nil
}
