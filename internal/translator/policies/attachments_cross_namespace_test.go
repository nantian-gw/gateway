package policies_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/nantian-gw/gateway/internal/mesh"
	"github.com/nantian-gw/gateway/internal/translator"
	"github.com/nantian-gw/gateway/internal/translator/testutil"
)

func TestBuildSnapshotDoesNotAttachCrossNamespaceRouteWhenAllowedRoutesFromSame(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	portNumber := gatewayv1.PortNumber(8080)

	client := testutil.NewTranslatorClientBuilder(scheme).
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
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "same-only", Namespace: "gateway-conformance-infra"},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
						AllowedRoutes: &gatewayv1.AllowedRoutes{
							Namespaces: &gatewayv1.RouteNamespaces{
								From: ptr(gatewayv1.NamespacesFromSame),
							},
						},
					}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cross-namespace-route",
					Namespace: "gateway-conformance-web-backend",
				},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Name:      "same-only",
							Namespace: ptr[gatewayv1.Namespace]("gateway-conformance-infra"),
						}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "web-backend",
									Port: &portNumber,
								},
							},
						}},
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "web-backend", Namespace: "gateway-conformance-web-backend"},
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
					Name:      "web-backend-1",
					Namespace: "gateway-conformance-web-backend",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "web-backend",
					},
				},
				Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](8080)}},
				Endpoints: []discoveryv1.Endpoint{{
					Addresses: []string{"10.0.0.20"},
				}},
			},
		).
		Build()

	xlator := translator.New(string(controllerName), slog.New(slog.NewTextHandler(io.Discard, nil)))
	snapshot, err := xlator.Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.Listeners) != 1 {
		t.Fatalf("expected 1 listener, got %d", len(snapshot.Listeners))
	}
	if got := snapshot.Listeners[0].AttachedRoutes; len(got) != 0 {
		t.Fatalf("expected no attached routes for cross-namespace route, got %#v", got)
	}
}

func TestBuildSnapshotAttachesCrossNamespaceRouteWhenAllowedRoutesFromAll(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	namespaceMode := gatewayv1.NamespacesFromAll
	portNumber := gatewayv1.PortNumber(8080)

	client := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "infra"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "apps"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "infra"},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
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
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "apps"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Name:      "gw",
							Namespace: ptr[gatewayv1.Namespace]("infra"),
						}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "echo",
									Port: &portNumber,
								},
							},
						}},
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "apps"},
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
					Namespace: "apps",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "echo",
					},
				},
				Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](8080)}},
				Endpoints: []discoveryv1.Endpoint{{
					Addresses: []string{"10.0.0.20"},
				}},
			},
		).
		Build()

	snapshot, err := translator.New(string(controllerName), slog.New(slog.NewTextHandler(io.Discard, nil))).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.Listeners) != 1 {
		t.Fatalf("expected 1 listener, got %d", len(snapshot.Listeners))
	}
	if got := snapshot.Listeners[0].AttachedRoutes; len(got) != 1 || got[0] != "apps/route" {
		t.Fatalf("unexpected attached routes: %#v", got)
	}
}

func TestBuildSnapshotAttachesRoutesOnlyForSelectorMatchedNamespacesAndParentRefPort(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	namespaceMode := gatewayv1.NamespacesFromSelector
	portNumber := gatewayv1.PortNumber(8080)

	client := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "infra"}},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "matched",
					Labels: map[string]string{"tenant": "edge"},
				},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "other",
					Labels: map[string]string{"tenant": "other"},
				},
			},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "infra"},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{
						{
							Name:     "http-80",
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
						},
						{
							Name:     "http-8080",
							Protocol: gatewayv1.HTTPProtocolType,
							Port:     8080,
							AllowedRoutes: &gatewayv1.AllowedRoutes{
								Namespaces: &gatewayv1.RouteNamespaces{
									From: &namespaceMode,
									Selector: &metav1.LabelSelector{
										MatchLabels: map[string]string{"tenant": "edge"},
									},
								},
							},
						},
					},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "matched-route", Namespace: "matched"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Name:      "gw",
							Namespace: ptr[gatewayv1.Namespace]("infra"),
							Port:      ptr[gatewayv1.PortNumber](8080),
						}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "echo",
									Port: &portNumber,
								},
							},
						}},
					}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "other-route", Namespace: "other"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Name:      "gw",
							Namespace: ptr[gatewayv1.Namespace]("infra"),
							Port:      ptr[gatewayv1.PortNumber](8080),
						}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "echo",
									Port: &portNumber,
								},
							},
						}},
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "matched"},
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
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "other"},
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
					Name:      "echo-matched-1",
					Namespace: "matched",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "echo",
					},
				},
				Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](8080)}},
				Endpoints: []discoveryv1.Endpoint{{
					Addresses: []string{"10.0.0.21"},
				}},
			},
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "echo-other-1",
					Namespace: "other",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "echo",
					},
				},
				Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](8080)}},
				Endpoints: []discoveryv1.Endpoint{{
					Addresses: []string{"10.0.0.22"},
				}},
			},
		).
		Build()

	snapshot, err := translator.New(string(controllerName), slog.New(slog.NewTextHandler(io.Discard, nil))).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	listeners := make(map[string][]string, len(snapshot.Listeners))
	for _, listener := range snapshot.Listeners {
		listeners[listener.Name] = listener.AttachedRoutes
	}

	if got := listeners["infra/gw/http-80"]; len(got) != 0 {
		t.Fatalf("unexpected attached routes on port 80 listener: %#v", got)
	}
	if got := listeners["infra/gw/http-8080"]; len(got) != 1 || got[0] != "matched/matched-route" {
		t.Fatalf("unexpected attached routes on port 8080 listener: %#v", got)
	}
}

func TestBuildSnapshotAllowsListenerSetFromSelectorMatchedNamespace(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	client := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "infra"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Name:   "allowed-listenersets",
				Labels: map[string]string{"allowed": "ns"},
			}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec:       gatewayv1.GatewayClassSpec{ControllerName: controllerName},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "infra"},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "gateway-listener",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
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
				ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "allowed-listenersets"},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Name:      "gw",
						Namespace: ptr(gatewayv1.Namespace("infra")),
					},
					Listeners: []gatewayv1.ListenerEntry{{
						Name:     "ls-listener",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
						Hostname: ptr(gatewayv1.Hostname("ls.example.com")),
					}},
				},
			},
		).
		Build()

	snapshot, err := translator.New(string(controllerName), slog.New(slog.NewTextHandler(io.Discard, nil))).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	listeners := make(map[string]bool, len(snapshot.Listeners))
	for _, listener := range snapshot.Listeners {
		listeners[listener.Name] = true
	}
	if !listeners["infra/gw/allowed-listenersets/ls/ls-listener"] {
		t.Fatalf("expected selector-matched ListenerSet listener in snapshot, got %#v", listeners)
	}
}

func TestBuildSnapshotListenerSetAllowedRoutesSameUsesListenerSetNamespace(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	portNumber := gatewayv1.PortNumber(8080)

	client := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "infra"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "apps"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec:       gatewayv1.GatewayClassSpec{ControllerName: controllerName},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "infra"},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					AllowedListeners: &gatewayv1.AllowedListeners{
						Namespaces: &gatewayv1.ListenerNamespaces{From: ptr(gatewayv1.NamespacesFromAll)},
					},
				},
			},
			&gatewayv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "apps"},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Name:      "gw",
						Namespace: ptr(gatewayv1.Namespace("infra")),
					},
					Listeners: []gatewayv1.ListenerEntry{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
						AllowedRoutes: &gatewayv1.AllowedRoutes{
							Namespaces: &gatewayv1.RouteNamespaces{From: ptr(gatewayv1.NamespacesFromSame)},
						},
					}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "apps"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Group:     ptr(gatewayv1.Group(gatewayv1.GroupName)),
							Kind:      ptr(gatewayv1.Kind("ListenerSet")),
							Name:      "ls",
							Namespace: ptr(gatewayv1.Namespace("apps")),
						}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "echo",
									Port: &portNumber,
								},
							},
						}},
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "apps"},
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
					Namespace: "apps",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "echo",
					},
				},
				Ports:     []discoveryv1.EndpointPort{{Port: ptr[int32](8080)}},
				Endpoints: []discoveryv1.Endpoint{{Addresses: []string{"10.0.0.21"}}},
			},
		).
		Build()

	snapshot, err := translator.New(string(controllerName), slog.New(slog.NewTextHandler(io.Discard, nil))).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	listeners := make(map[string][]string, len(snapshot.Listeners))
	for _, listener := range snapshot.Listeners {
		listeners[listener.Name] = listener.AttachedRoutes
	}
	if got := listeners["infra/gw/apps/ls/http"]; len(got) != 1 || got[0] != "apps/route" {
		t.Fatalf("ListenerSet listener attachments = %#v, want only apps/route", got)
	}
}

func TestBuildSnapshotListenerSetAllowedRoutesSelectorLoadsRouteNamespace(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	portNumber := gatewayv1.PortNumber(8080)

	client := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "infra"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Name:   "selected-routes",
				Labels: map[string]string{"allowed": "ns"},
			}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec:       gatewayv1.GatewayClassSpec{ControllerName: controllerName},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "infra"},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					AllowedListeners: &gatewayv1.AllowedListeners{
						Namespaces: &gatewayv1.ListenerNamespaces{From: ptr(gatewayv1.NamespacesFromAll)},
					},
				},
			},
			&gatewayv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "infra"},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{Name: "gw"},
					Listeners: []gatewayv1.ListenerEntry{{
						Name:     "selector",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
						AllowedRoutes: &gatewayv1.AllowedRoutes{
							Namespaces: &gatewayv1.RouteNamespaces{
								From: ptr(gatewayv1.NamespacesFromSelector),
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"allowed": "ns"},
								},
							},
						},
					}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "selected-routes"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Group:     ptr(gatewayv1.Group(gatewayv1.GroupName)),
							Kind:      ptr(gatewayv1.Kind("ListenerSet")),
							Name:      "ls",
							Namespace: ptr(gatewayv1.Namespace("infra")),
						}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "echo",
									Port: &portNumber,
								},
							},
						}},
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "selected-routes"},
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
					Namespace: "selected-routes",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "echo",
					},
				},
				Ports:     []discoveryv1.EndpointPort{{Port: ptr[int32](8080)}},
				Endpoints: []discoveryv1.Endpoint{{Addresses: []string{"10.0.0.22"}}},
			},
		).
		Build()

	snapshot, err := translator.New(string(controllerName), slog.New(slog.NewTextHandler(io.Discard, nil))).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	listeners := make(map[string][]string, len(snapshot.Listeners))
	for _, listener := range snapshot.Listeners {
		listeners[listener.Name] = listener.AttachedRoutes
	}
	if got := listeners["infra/gw/infra/ls/selector"]; len(got) != 1 || got[0] != "selected-routes/route" {
		t.Fatalf("ListenerSet listener attachments = %#v, want only selected-routes/route", got)
	}
}

func TestBuildSnapshotAttachesListenerSetRoutesForMixedAllowedRoutesNamespaces(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	portNumber := gatewayv1.PortNumber(8080)
	allNamespaces := gatewayv1.NamespacesFromAll
	sameNamespace := gatewayv1.NamespacesFromSame
	selectorNamespaces := gatewayv1.NamespacesFromSelector

	client := testutil.NewTranslatorClientBuilder(scheme).
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
				Spec:       gatewayv1.GatewayClassSpec{ControllerName: controllerName},
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
						Namespaces: &gatewayv1.ListenerNamespaces{From: ptr(gatewayv1.NamespacesFromAll)},
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
						Group:     ptr(gatewayv1.Group(gatewayv1.GroupName)),
						Kind:      ptr(gatewayv1.Kind("Gateway")),
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
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "route-in-same-namespace",
					Namespace: "gateway-conformance-infra",
				},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Group:     ptr(gatewayv1.Group(gatewayv1.GroupName)),
							Kind:      ptr(gatewayv1.Kind("ListenerSet")),
							Name:      "listenerset-test-allowed-routes-namespaces",
							Namespace: ptr(gatewayv1.Namespace("gateway-conformance-infra")),
						}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "infra-backend-v1",
									Port: &portNumber,
								},
							},
						}},
					}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "route-in-selected-namespace",
					Namespace: "gateway-api-routes-allowed-ns",
				},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Group:     ptr(gatewayv1.Group(gatewayv1.GroupName)),
							Kind:      ptr(gatewayv1.Kind("ListenerSet")),
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
									Port:      &portNumber,
								},
							},
						}},
					}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "route-not-in-selected-namespace",
					Namespace: "gateway-api-routes-not-allowed-ns",
				},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Group:     ptr(gatewayv1.Group(gatewayv1.GroupName)),
							Kind:      ptr(gatewayv1.Kind("ListenerSet")),
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
									Port:      &portNumber,
								},
							},
						}},
					}},
				},
			},
			&gatewayv1beta1.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "listenerset-test-allowed-routes-namespaces-reference-grant",
					Namespace: "gateway-conformance-infra",
				},
				Spec: gatewayv1beta1.ReferenceGrantSpec{
					From: []gatewayv1beta1.ReferenceGrantFrom{
						{
							Group:     gatewayv1beta1.Group(gatewayv1.GroupVersion.Group),
							Kind:      gatewayv1beta1.Kind("HTTPRoute"),
							Namespace: gatewayv1beta1.Namespace("gateway-api-routes-allowed-ns"),
						},
						{
							Group:     gatewayv1beta1.Group(gatewayv1.GroupVersion.Group),
							Kind:      gatewayv1beta1.Kind("HTTPRoute"),
							Namespace: gatewayv1beta1.Namespace("gateway-api-routes-not-allowed-ns"),
						},
					},
					To: []gatewayv1beta1.ReferenceGrantTo{{
						Group: gatewayv1beta1.Group(""),
						Kind:  gatewayv1beta1.Kind("Service"),
					}},
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

	snapshot, err := translator.New(string(controllerName), slog.New(slog.NewTextHandler(io.Discard, nil))).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	listeners := make(map[string][]string, len(snapshot.Listeners))
	for _, listener := range snapshot.Listeners {
		listeners[listener.Name] = listener.AttachedRoutes
	}

	if got := listeners["gateway-conformance-infra/gateway-with-listener-sets-test-allowed-routes/gateway-conformance-infra/listenerset-test-allowed-routes-namespaces/listener-set-listener-allowed-routes-all"]; len(got) != 3 {
		t.Fatalf("all listener attached routes = %#v, want 3 routes", got)
	}
	if got := listeners["gateway-conformance-infra/gateway-with-listener-sets-test-allowed-routes/gateway-conformance-infra/listenerset-test-allowed-routes-namespaces/listener-set-listener-allowed-routes-same"]; len(got) != 1 || got[0] != "gateway-conformance-infra/route-in-same-namespace" {
		t.Fatalf("same listener attached routes = %#v, want only same-namespace route", got)
	}
	if got := listeners["gateway-conformance-infra/gateway-with-listener-sets-test-allowed-routes/gateway-conformance-infra/listenerset-test-allowed-routes-namespaces/listener-set-listener-allowed-routes-selector"]; len(got) != 1 || got[0] != "gateway-api-routes-allowed-ns/route-in-selected-namespace" {
		t.Fatalf("selector listener attached routes = %#v, want only selected-namespace route", got)
	}
}

func TestBuildSnapshotSynthesizesMeshServiceListeners(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	serviceKind := gatewayv1.Kind("Service")
	servicePort := gatewayv1.PortNumber(80)
	httpProtocol := "http"

	client := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
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
							AppProtocol: ptr(httpProtocol),
						},
						{
							Name:        "http-alt",
							Port:        8080,
							TargetPort:  intstr.FromInt(8080),
							Protocol:    corev1.ProtocolTCP,
							AppProtocol: ptr(httpProtocol),
						},
					},
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
		).
		Build()

	snapshot, err := translator.New("", slog.New(slog.NewTextHandler(io.Discard, nil))).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	var port80Attached bool
	var port8080Attached bool
	var meshListeners int
	for _, listener := range snapshot.Listeners {
		if listener.Metadata[mesh.FrontendKindMetadataKey] != mesh.FrontendKindService {
			continue
		}
		meshListeners++
		switch listener.Metadata[mesh.FrontendPortMetadataKey] {
		case "80":
			port80Attached = len(listener.AttachedRoutes) == 1 && listener.AttachedRoutes[0] == "default/mesh"
		case "8080":
			port8080Attached = len(listener.AttachedRoutes) == 1
		}
	}

	if meshListeners != 2 {
		t.Fatalf("expected 2 mesh listeners, got %d", meshListeners)
	}
	if !port80Attached {
		t.Fatalf("expected route to attach to port 80 mesh listener")
	}
	if port8080Attached {
		t.Fatalf("did not expect route to attach to port 8080 mesh listener")
	}
}
