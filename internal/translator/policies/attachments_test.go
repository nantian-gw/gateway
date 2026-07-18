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

	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/mesh"
	"github.com/nantian-gw/gateway/internal/translator"
	"github.com/nantian-gw/gateway/internal/translator/testutil"
)

func TestBuildSnapshotAttachesRoutesOnlyToIntersectingListeners(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	wildcardHostname := gatewayv1.Hostname("*.wildcard.io")
	specificHostname := gatewayv1.Hostname("very.specific.com")
	pathPrefix := gatewayv1.PathMatchPathPrefix
	portNumber := gatewayv1.PortNumber(8080)

	client := testutil.NewTranslatorClientBuilder(scheme).
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
					Listeners: []gatewayv1.Listener{
						{
							Name:     "specific",
							Protocol: gatewayv1.HTTPProtocolType,
							Port:     80,
							Hostname: &specificHostname,
						},
						{
							Name:     "wildcard",
							Protocol: gatewayv1.HTTPProtocolType,
							Port:     80,
							Hostname: &wildcardHostname,
						},
					},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "specific-route", Namespace: "default"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{Name: "gw"}},
					},
					Hostnames: []gatewayv1.Hostname{"very.specific.com"},
					Rules: []gatewayv1.HTTPRouteRule{{
						Matches: []gatewayv1.HTTPRouteMatch{{
							Path: &gatewayv1.HTTPPathMatch{Type: &pathPrefix, Value: ptr("/specific")},
						}},
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
				ObjectMeta: metav1.ObjectMeta{Name: "wildcard-route", Namespace: "default"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{Name: "gw"}},
					},
					Hostnames: []gatewayv1.Hostname{"foo.wildcard.io"},
					Rules: []gatewayv1.HTTPRouteRule{{
						Matches: []gatewayv1.HTTPRouteMatch{{
							Path: &gatewayv1.HTTPPathMatch{Type: &pathPrefix, Value: ptr("/wildcard")},
						}},
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
				ObjectMeta: metav1.ObjectMeta{Name: "non-intersecting-route", Namespace: "default"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{Name: "gw"}},
					},
					Hostnames: []gatewayv1.Hostname{"wildcard.io"},
					Rules: []gatewayv1.HTTPRouteRule{{
						Matches: []gatewayv1.HTTPRouteMatch{{
							Path: &gatewayv1.HTTPPathMatch{Type: &pathPrefix, Value: ptr("/blocked")},
						}},
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

	xlator := translator.New(string(controllerName), slog.New(slog.NewTextHandler(io.Discard, nil)))
	snapshot, err := xlator.Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	listeners := make(map[string][]string, len(snapshot.Listeners))
	for _, listener := range snapshot.Listeners {
		listeners[listener.Name] = listener.AttachedRoutes
	}

	if got := listeners["default/gw/specific"]; len(got) != 1 || got[0] != "default/specific-route" {
		t.Fatalf("unexpected specific listener attachments: %#v", got)
	}
	if got := listeners["default/gw/wildcard"]; len(got) != 1 || got[0] != "default/wildcard-route" {
		t.Fatalf("unexpected wildcard listener attachments: %#v", got)
	}
}

func TestBuildSnapshotScopesListenerSetParentRoutesToListenerSetListeners(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	gatewayHostname := gatewayv1.Hostname("gateway.example.com")
	listenerSetHostname := gatewayv1.Hostname("listenerset.example.com")
	portNumber := gatewayv1.PortNumber(8080)

	client := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec:       gatewayv1.GatewayClassSpec{ControllerName: controllerName},
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
					Listeners: []gatewayv1.Listener{{
						Name:     "gateway-listener",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
						Hostname: &gatewayHostname,
					}},
				},
			},
			&gatewayv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "default"},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Name:      "gw",
						Namespace: ptr(gatewayv1.Namespace("default")),
					},
					Listeners: []gatewayv1.ListenerEntry{{
						Name:     "ls-listener",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
						Hostname: &listenerSetHostname,
						AllowedRoutes: &gatewayv1.AllowedRoutes{
							Namespaces: &gatewayv1.RouteNamespaces{
								From: ptr(gatewayv1.NamespacesFromAll),
							},
						},
					}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "ls-route", Namespace: "default"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Group: ptr(gatewayv1.Group(gatewayv1.GroupName)),
							Kind:  ptr(gatewayv1.Kind("ListenerSet")),
							Name:  "ls",
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

	xlator := translator.New(string(controllerName), slog.New(slog.NewTextHandler(io.Discard, nil)))
	snapshot, err := xlator.Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	listeners := make(map[string][]string, len(snapshot.Listeners))
	for _, listener := range snapshot.Listeners {
		listeners[listener.Name] = listener.AttachedRoutes
	}

	if got := listeners["default/gw/gateway-listener"]; len(got) != 0 {
		t.Fatalf("Gateway listener unexpectedly attached ListenerSet route: %#v", got)
	}
	if got := listeners["default/gw/default/ls/ls-listener"]; len(got) != 1 || got[0] != "default/ls-route" {
		t.Fatalf("ListenerSet listener attachments = %#v, want only default/ls-route", got)
	}
}

func TestBuildSnapshotAttachesListenerSetParentToAllDerivedListeners(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	gatewayHostname := gatewayv1.Hostname("gateway.example.com")
	listenerOneHostname := gatewayv1.Hostname("one.example.com")
	listenerTwoHostname := gatewayv1.Hostname("two.example.com")
	portNumber := gatewayv1.PortNumber(8080)

	client := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec:       gatewayv1.GatewayClassSpec{ControllerName: controllerName},
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
					Listeners: []gatewayv1.Listener{{
						Name:     "gateway-listener",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
						Hostname: &gatewayHostname,
					}},
				},
			},
			&gatewayv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "default"},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Name:      "gw",
						Namespace: ptr(gatewayv1.Namespace("default")),
					},
					Listeners: []gatewayv1.ListenerEntry{
						{
							Name:     "one",
							Protocol: gatewayv1.HTTPProtocolType,
							Port:     80,
							Hostname: &listenerOneHostname,
							AllowedRoutes: &gatewayv1.AllowedRoutes{
								Namespaces: &gatewayv1.RouteNamespaces{
									From: ptr(gatewayv1.NamespacesFromAll),
								},
							},
						},
						{
							Name:     "two",
							Protocol: gatewayv1.HTTPProtocolType,
							Port:     80,
							Hostname: &listenerTwoHostname,
							AllowedRoutes: &gatewayv1.AllowedRoutes{
								Namespaces: &gatewayv1.RouteNamespaces{
									From: ptr(gatewayv1.NamespacesFromAll),
								},
							},
						},
					},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "all-listeners-route", Namespace: "default"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Group: ptr(gatewayv1.Group(gatewayv1.GroupName)),
							Kind:  ptr(gatewayv1.Kind("ListenerSet")),
							Name:  "ls",
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

	xlator := translator.New(string(controllerName), slog.New(slog.NewTextHandler(io.Discard, nil)))
	snapshot, err := xlator.Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	listeners := make(map[string][]string, len(snapshot.Listeners))
	for _, listener := range snapshot.Listeners {
		listeners[listener.Name] = listener.AttachedRoutes
	}

	if got := listeners["default/gw/default/ls/one"]; len(got) != 1 || got[0] != "default/all-listeners-route" {
		t.Fatalf("ListenerSet listener one attachments = %#v, want only default/all-listeners-route", got)
	}
	if got := listeners["default/gw/default/ls/two"]; len(got) != 1 || got[0] != "default/all-listeners-route" {
		t.Fatalf("ListenerSet listener two attachments = %#v, want only default/all-listeners-route", got)
	}
}

func TestBuildSnapshotAttachesSecondListenerSetHTTPRoutingConformanceRoutes(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	allNamespaces := gatewayv1.NamespacesFromAll
	servicePort := gatewayv1.PortNumber(8080)
	parentGroup := gatewayv1.Group(gatewayv1.GroupName)
	listenerSetKind := gatewayv1.Kind("ListenerSet")
	gatewayNamespace := gatewayv1.Namespace("gateway-conformance-infra")
	gatewayHostnameOne := gatewayv1.Hostname("gateway-listener-1.com")
	gatewayHostnameTwo := gatewayv1.Hostname("gateway-listener-2.com")
	ls1HostnameOne := gatewayv1.Hostname("listener-set-http-routing-1-listener-1.com")
	ls1HostnameTwo := gatewayv1.Hostname("listener-set-http-routing-1-listener-2.com")
	ls2HostnameOne := gatewayv1.Hostname("listener-set-http-routing-2-listener-1.com")
	ls2HostnameTwo := gatewayv1.Hostname("listener-set-http-routing-2-listener-2.com")

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
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway-with-listener-sets-http-routing",
					Namespace: "gateway-conformance-infra",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					AllowedListeners: &gatewayv1.AllowedListeners{
						Namespaces: &gatewayv1.ListenerNamespaces{
							From: &allNamespaces,
						},
					},
					Listeners: []gatewayv1.Listener{
						{
							Name:     "gateway-listener-1",
							Port:     80,
							Protocol: gatewayv1.HTTPProtocolType,
							Hostname: &gatewayHostnameOne,
							AllowedRoutes: &gatewayv1.AllowedRoutes{
								Namespaces: &gatewayv1.RouteNamespaces{From: &allNamespaces},
							},
						},
						{
							Name:     "gateway-listener-2",
							Port:     80,
							Protocol: gatewayv1.HTTPProtocolType,
							Hostname: &gatewayHostnameTwo,
							AllowedRoutes: &gatewayv1.AllowedRoutes{
								Namespaces: &gatewayv1.RouteNamespaces{From: &allNamespaces},
							},
						},
					},
				},
			},
			&gatewayv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{Name: "listener-set-http-routing-1", Namespace: "gateway-conformance-infra"},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Group:     ptr(parentGroup),
						Kind:      ptr(gatewayv1.Kind("Gateway")),
						Name:      "gateway-with-listener-sets-http-routing",
						Namespace: &gatewayNamespace,
					},
					Listeners: []gatewayv1.ListenerEntry{
						{
							Name:     "listener-set-http-routing-1-listener-1",
							Port:     80,
							Protocol: gatewayv1.HTTPProtocolType,
							Hostname: &ls1HostnameOne,
							AllowedRoutes: &gatewayv1.AllowedRoutes{
								Namespaces: &gatewayv1.RouteNamespaces{From: &allNamespaces},
							},
						},
						{
							Name:     "listener-set-http-routing-1-listener-2",
							Port:     80,
							Protocol: gatewayv1.HTTPProtocolType,
							Hostname: &ls1HostnameTwo,
							AllowedRoutes: &gatewayv1.AllowedRoutes{
								Namespaces: &gatewayv1.RouteNamespaces{From: &allNamespaces},
							},
						},
					},
				},
			},
			&gatewayv1.ListenerSet{
				ObjectMeta: metav1.ObjectMeta{Name: "listener-set-http-routing-2", Namespace: "gateway-conformance-infra"},
				Spec: gatewayv1.ListenerSetSpec{
					ParentRef: gatewayv1.ParentGatewayReference{
						Group:     ptr(parentGroup),
						Kind:      ptr(gatewayv1.Kind("Gateway")),
						Name:      "gateway-with-listener-sets-http-routing",
						Namespace: &gatewayNamespace,
					},
					Listeners: []gatewayv1.ListenerEntry{
						{
							Name:     "listener-set-http-routing-2-listener-1",
							Port:     80,
							Protocol: gatewayv1.HTTPProtocolType,
							Hostname: &ls2HostnameOne,
							AllowedRoutes: &gatewayv1.AllowedRoutes{
								Namespaces: &gatewayv1.RouteNamespaces{From: &allNamespaces},
							},
						},
						{
							Name:     "listener-set-http-routing-2-listener-2",
							Port:     80,
							Protocol: gatewayv1.HTTPProtocolType,
							Hostname: &ls2HostnameTwo,
							AllowedRoutes: &gatewayv1.AllowedRoutes{
								Namespaces: &gatewayv1.RouteNamespaces{From: &allNamespaces},
							},
						},
					},
				},
			},
			httpRouteForListenerSetHTTPRouting(
				"attaches-to-all-listeners",
				[]gatewayv1.ParentReference{
					{Name: "gateway-with-listener-sets-http-routing", Namespace: &gatewayNamespace},
					{Group: &parentGroup, Kind: &listenerSetKind, Name: "listener-set-http-routing-1", Namespace: &gatewayNamespace},
					{Group: &parentGroup, Kind: &listenerSetKind, Name: "listener-set-http-routing-2", Namespace: &gatewayNamespace},
				},
				"/route",
				"infra-backend-v1",
				servicePort,
			),
			httpRouteForListenerSetHTTPRouting(
				"gateway-route",
				[]gatewayv1.ParentReference{{Name: "gateway-with-listener-sets-http-routing", Namespace: &gatewayNamespace}},
				"/gateway-route",
				"infra-backend-v2",
				servicePort,
			),
			httpRouteForListenerSetHTTPRouting(
				"gateway-section-route",
				[]gatewayv1.ParentReference{{
					Name:        "gateway-with-listener-sets-http-routing",
					Namespace:   &gatewayNamespace,
					SectionName: ptr(gatewayv1.SectionName("gateway-listener-1")),
				}},
				"/gateway-section-route",
				"infra-backend-v3",
				servicePort,
			),
			httpRouteForListenerSetHTTPRouting(
				"listener-set-http-routing-1-route",
				[]gatewayv1.ParentReference{{Group: &parentGroup, Kind: &listenerSetKind, Name: "listener-set-http-routing-1", Namespace: &gatewayNamespace}},
				"/listener-set-http-routing-1-route",
				"infra-backend-v2",
				servicePort,
			),
			httpRouteForListenerSetHTTPRouting(
				"listener-set-http-routing-1-section-route",
				[]gatewayv1.ParentReference{{
					Group:       &parentGroup,
					Kind:        &listenerSetKind,
					Name:        "listener-set-http-routing-1",
					Namespace:   &gatewayNamespace,
					SectionName: ptr(gatewayv1.SectionName("listener-set-http-routing-1-listener-1")),
				}},
				"/listener-set-http-routing-1-section-route",
				"infra-backend-v3",
				servicePort,
			),
			httpRouteForListenerSetHTTPRouting(
				"listener-set-http-routing-2-route",
				[]gatewayv1.ParentReference{{Group: &parentGroup, Kind: &listenerSetKind, Name: "listener-set-http-routing-2", Namespace: &gatewayNamespace}},
				"/listener-set-http-routing-2-route",
				"infra-backend-v2",
				servicePort,
			),
			serviceForListenerSetHTTPRouting("infra-backend-v1"),
			serviceForListenerSetHTTPRouting("infra-backend-v2"),
			serviceForListenerSetHTTPRouting("infra-backend-v3"),
			endpointSliceForListenerSetHTTPRouting("infra-backend-v1", "10.0.0.1"),
			endpointSliceForListenerSetHTTPRouting("infra-backend-v2", "10.0.0.2"),
			endpointSliceForListenerSetHTTPRouting("infra-backend-v3", "10.0.0.3"),
		).
		Build()

	xlator := translator.New(string(controllerName), slog.New(slog.NewTextHandler(io.Discard, nil)))
	snapshot, err := xlator.Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	listeners := make(map[string][]string, len(snapshot.Listeners))
	for _, listener := range snapshot.Listeners {
		listeners[listener.Name] = listener.AttachedRoutes
	}

	for _, listenerName := range []string{
		"gateway-conformance-infra/gateway-with-listener-sets-http-routing/gateway-conformance-infra/listener-set-http-routing-2/listener-set-http-routing-2-listener-1",
		"gateway-conformance-infra/gateway-with-listener-sets-http-routing/gateway-conformance-infra/listener-set-http-routing-2/listener-set-http-routing-2-listener-2",
	} {
		got := listeners[listenerName]
		want := []string{
			"gateway-conformance-infra/attaches-to-all-listeners",
			"gateway-conformance-infra/listener-set-http-routing-2-route",
		}
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Fatalf("second ListenerSet listener %s attached routes = %#v, want %#v", listenerName, got, want)
		}
	}
}

func httpRouteForListenerSetHTTPRouting(
	name string,
	parentRefs []gatewayv1.ParentReference,
	path string,
	backendName gatewayv1.ObjectName,
	servicePort gatewayv1.PortNumber,
) *gatewayv1.HTTPRoute {
	pathType := gatewayv1.PathMatchPathPrefix
	return &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "gateway-conformance-infra"},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: parentRefs,
			},
			Rules: []gatewayv1.HTTPRouteRule{{
				Matches: []gatewayv1.HTTPRouteMatch{{
					Path: &gatewayv1.HTTPPathMatch{
						Type:  &pathType,
						Value: ptr(path),
					},
				}},
				BackendRefs: []gatewayv1.HTTPBackendRef{{
					BackendRef: gatewayv1.BackendRef{
						BackendObjectReference: gatewayv1.BackendObjectReference{
							Name: backendName,
							Port: &servicePort,
						},
					},
				}},
			}},
		},
	}
}

func serviceForListenerSetHTTPRouting(name string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "gateway-conformance-infra"},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}
}

func endpointSliceForListenerSetHTTPRouting(serviceName, address string) *discoveryv1.EndpointSlice {
	return &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName + "-1",
			Namespace: "gateway-conformance-infra",
			Labels: map[string]string{
				discoveryv1.LabelServiceName: serviceName,
			},
		},
		Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](8080)}},
		Endpoints: []discoveryv1.Endpoint{{
			Addresses: []string{address},
		}},
	}
}

func TestBuildSnapshotAttachesDefaultGatewayRoute(t *testing.T) {
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
					DefaultScope:     gatewayv1.GatewayDefaultScopeAll,
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
						UseDefaultGateways: gatewayv1.GatewayDefaultScopeAll,
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

	xlator := translator.New(string(controllerName), slog.New(slog.NewTextHandler(io.Discard, nil)))
	snapshot, err := xlator.Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.HTTPRoutes) != 1 {
		t.Fatalf("expected 1 HTTPRoute, got %d", len(snapshot.HTTPRoutes))
	}
	parents := snapshot.HTTPRoutes[0].ParentRefs
	if len(parents) != 1 {
		t.Fatalf("expected 1 synthetic parentRef, got %#v", parents)
	}
	if parents[0].Namespace != "default" || parents[0].Name != "gw" || parents[0].Kind != "Gateway" {
		t.Fatalf("unexpected synthetic parentRef: %#v", parents[0])
	}

	listeners := make(map[string][]string, len(snapshot.Listeners))
	for _, listener := range snapshot.Listeners {
		listeners[listener.Name] = listener.AttachedRoutes
	}
	if got := listeners["default/gw/http"]; len(got) != 1 || got[0] != "default/route" {
		t.Fatalf("unexpected default gateway listener attachments: %#v", got)
	}
}

func TestBuildSnapshotAttachesConformanceSameNamespaceRoute(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	portNumber := gatewayv1.PortNumber(8080)
	parentGroup := gatewayv1.Group(gatewayv1.GroupName)
	parentKind := gatewayv1.Kind("Gateway")
	backendGroup := gatewayv1.Group("unknownkind.example.com")
	backendKind := gatewayv1.Kind("NonExistent")

	client := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gateway-conformance-infra",
					Labels: map[string]string{
						"gateway-conformance":         "infra",
						"kubernetes.io/metadata.name": "gateway-conformance-infra",
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
				ObjectMeta: metav1.ObjectMeta{Name: "same-namespace", Namespace: "gateway-conformance-infra"},
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
					Name:      "invalid-backend-ref-unknown-kind",
					Namespace: "gateway-conformance-infra",
				},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Group: &parentGroup,
							Kind:  &parentKind,
							Name:  "same-namespace",
						}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Group: &backendGroup,
									Kind:  &backendKind,
									Name:  "infra-backend-v1",
									Port:  &portNumber,
								},
							},
						}},
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "infra-backend-v1", Namespace: "gateway-conformance-infra"},
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
					Name:      "infra-backend-v1-1",
					Namespace: "gateway-conformance-infra",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "infra-backend-v1",
					},
				},
				Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](8080)}},
				Endpoints: []discoveryv1.Endpoint{{
					Addresses: []string{"10.0.0.10"},
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
	if got := snapshot.Listeners[0].AttachedRoutes; len(got) != 1 || got[0] != "gateway-conformance-infra/invalid-backend-ref-unknown-kind" {
		t.Fatalf("unexpected conformance listener attachments: %#v", got)
	}
}

func TestBuildSnapshotAttachesGRPCRouteToHTTPListener(t *testing.T) {
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
						"gateway-conformance":         "infra",
						"kubernetes.io/metadata.name": "gateway-conformance-infra",
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
				ObjectMeta: metav1.ObjectMeta{Name: "same-namespace", Namespace: "gateway-conformance-infra"},
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
			&gatewayv1.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "exact-matching",
					Namespace: "gateway-conformance-infra",
				},
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
									Port: &portNumber,
								},
							},
						}},
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "grpc-infra-backend-v1", Namespace: "gateway-conformance-infra"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "grpc",
						Port:       8080,
						TargetPort: intstr.FromInt(8080),
						Protocol:   corev1.ProtocolTCP,
					}},
				},
			},
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "grpc-infra-backend-v1-1",
					Namespace: "gateway-conformance-infra",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "grpc-infra-backend-v1",
					},
				},
				Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](8080)}},
				Endpoints: []discoveryv1.Endpoint{{
					Addresses: []string{"10.0.0.10"},
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
	if got := snapshot.Listeners[0].AttachedRoutes; len(got) != 1 || got[0] != "gateway-conformance-infra/exact-matching" {
		t.Fatalf("unexpected gRPC listener attachments: %#v", got)
	}
}

func TestBuildSnapshotAttachesMeshGRPCRouteToServiceFrontendListener(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	serviceKind := gatewayv1.Kind("Service")
	servicePort := gatewayv1.PortNumber(7070)
	appGRPC := "grpc"

	client := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-mesh"}},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "gateway-conformance-mesh"},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "echo"},
					Ports: []corev1.ServicePort{{
						Name:        "grpc",
						Port:        7070,
						Protocol:    corev1.ProtocolTCP,
						AppProtocol: ptr(appGRPC),
						TargetPort:  intstr.FromInt(7070),
					}},
				},
			},
			&gatewayv1.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mesh-grpc-weighted-backends",
					Namespace: "gateway-conformance-mesh",
				},
				Spec: gatewayv1.GRPCRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Group: ptr(gatewayv1.Group("")),
							Kind:  &serviceKind,
							Name:  "echo",
							Port:  &servicePort,
						}},
					},
					Rules: []gatewayv1.GRPCRouteRule{{
						BackendRefs: []gatewayv1.GRPCBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "echo-v1",
									Port: &servicePort,
								},
								Weight: ptr(int32(70)),
							},
						}},
					}},
				},
			},
		).
		Build()

	snapshot, err := translator.New("", slog.New(slog.NewTextHandler(io.Discard, nil))).Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	var attached bool
	for _, listener := range snapshot.Listeners {
		if listener.Metadata[mesh.FrontendKindMetadataKey] != mesh.FrontendKindService {
			continue
		}
		if listener.Metadata[mesh.FrontendNamespaceMetadataKey] != "gateway-conformance-mesh" {
			continue
		}
		if listener.Metadata[mesh.FrontendNameMetadataKey] != "echo" {
			continue
		}
		if listener.Metadata[mesh.FrontendPortMetadataKey] != "7070" {
			continue
		}

		if listener.Protocol != "GRPC" {
			t.Fatalf("expected mesh GRPC listener protocol, got %q", listener.Protocol)
		}
		attached = len(listener.AttachedRoutes) == 1 &&
			listener.AttachedRoutes[0] == "gateway-conformance-mesh/mesh-grpc-weighted-backends"
	}

	if !attached {
		t.Fatalf("expected mesh GRPCRoute to attach to echo:7070 service frontend listener")
	}
}

func TestBuildSnapshotAttachesTLSRouteToTLSListener(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	listenerHostname := gatewayv1.Hostname("*.example.com")
	parentNamespace := gatewayv1.Namespace("gateway-conformance-infra")
	tlsMode := gatewayv1.TLSModePassthrough
	portNumber := gatewayv1.PortNumber(443)

	client := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gateway-conformance-infra",
					Labels: map[string]string{
						"gateway-conformance":         "infra",
						"kubernetes.io/metadata.name": "gateway-conformance-infra",
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
				ObjectMeta: metav1.ObjectMeta{Name: "gateway-tlsroute", Namespace: "gateway-conformance-infra"},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "https",
						Protocol: gatewayv1.TLSProtocolType,
						Port:     443,
						Hostname: &listenerHostname,
						AllowedRoutes: &gatewayv1.AllowedRoutes{
							Namespaces: &gatewayv1.RouteNamespaces{
								From: ptr(gatewayv1.NamespacesFromSame),
							},
							Kinds: []gatewayv1.RouteGroupKind{{
								Kind: "TLSRoute",
							}},
						},
						TLS: &gatewayv1.ListenerTLSConfig{
							Mode: &tlsMode,
						},
					}},
				},
			},
			&gatewayv1alpha2.TLSRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway-conformance-infra-test",
					Namespace: "gateway-conformance-infra",
				},
				Spec: gatewayv1alpha2.TLSRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Name:      "gateway-tlsroute",
							Namespace: &parentNamespace,
						}},
					},
					Hostnames: []gatewayv1.Hostname{"abc.example.com"},
					Rules: []gatewayv1alpha2.TLSRouteRule{{
						BackendRefs: []gatewayv1alpha2.BackendRef{{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Name: "tls-backend",
								Port: &portNumber,
							},
						}},
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "tls-backend", Namespace: "gateway-conformance-infra"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "tls",
						Port:       443,
						TargetPort: intstr.FromInt(8443),
						Protocol:   corev1.ProtocolTCP,
					}},
				},
			},
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tls-backend-1",
					Namespace: "gateway-conformance-infra",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "tls-backend",
					},
				},
				Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](8443)}},
				Endpoints: []discoveryv1.Endpoint{{
					Addresses: []string{"10.0.0.40"},
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
	if got := snapshot.Listeners[0].AttachedRoutes; len(got) != 1 || got[0] != "gateway-conformance-infra/gateway-conformance-infra-test" {
		t.Fatalf("unexpected TLS listener attachments: %#v", got)
	}
}

func TestBuildSnapshotDoesNotAttachTLSRouteToHTTPSListener(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	listenerHostname := gatewayv1.Hostname("*.example.com")
	parentNamespace := gatewayv1.Namespace("gateway-conformance-infra")
	tlsMode := gatewayv1.TLSModeTerminate
	portNumber := gatewayv1.PortNumber(443)

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
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gateway-https", Namespace: "gateway-conformance-infra"},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "https",
						Protocol: gatewayv1.HTTPSProtocolType,
						Port:     443,
						Hostname: &listenerHostname,
						TLS: &gatewayv1.ListenerTLSConfig{
							Mode: &tlsMode,
							CertificateRefs: []gatewayv1.SecretObjectReference{{
								Name: "gateway-cert",
							}},
						},
					}},
				},
			},
			&gatewayv1alpha2.TLSRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway-conformance-infra-test",
					Namespace: "gateway-conformance-infra",
				},
				Spec: gatewayv1alpha2.TLSRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Name:      "gateway-https",
							Namespace: &parentNamespace,
						}},
					},
					Hostnames: []gatewayv1.Hostname{"abc.example.com"},
					Rules: []gatewayv1alpha2.TLSRouteRule{{
						BackendRefs: []gatewayv1alpha2.BackendRef{{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Name: "tls-backend",
								Port: &portNumber,
							},
						}},
					}},
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "gateway-cert", Namespace: "gateway-conformance-infra"},
				Type:       corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": testutil.ReadTestTLSAsset(t, "client.crt"),
					"tls.key": testutil.ReadTestTLSAsset(t, "client.key"),
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "tls-backend", Namespace: "gateway-conformance-infra"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "tls",
						Port:       443,
						TargetPort: intstr.FromInt(8443),
						Protocol:   corev1.ProtocolTCP,
					}},
				},
			},
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tls-backend-1",
					Namespace: "gateway-conformance-infra",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "tls-backend",
					},
				},
				Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](8443)}},
				Endpoints: []discoveryv1.Endpoint{{
					Addresses: []string{"10.0.0.40"},
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
		t.Fatalf("expected HTTPS listener to reject TLSRoute attachments, got %#v", got)
	}
}

func TestBuildSnapshotAttachesTCPRouteToMatchingParentRefPort(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	parentNamespace := gatewayv1.Namespace("default")
	targetPort := gatewayv1.PortNumber(9001)
	servicePort := gatewayv1.PortNumber(7001)

	client := testutil.NewTranslatorClientBuilder(scheme).
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
				ObjectMeta: metav1.ObjectMeta{Name: "gateway-tcp", Namespace: "default"},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{
						{
							Name:     "tcp-9000",
							Protocol: gatewayv1.TCPProtocolType,
							Port:     9000,
						},
						{
							Name:     "tcp-9001",
							Protocol: gatewayv1.TCPProtocolType,
							Port:     9001,
						},
					},
				},
			},
			&gatewayv1alpha2.TCPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "route-9001",
					Namespace: "default",
				},
				Spec: gatewayv1alpha2.TCPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Name:      "gateway-tcp",
							Namespace: &parentNamespace,
							Port:      &targetPort,
						}},
					},
					Rules: []gatewayv1alpha2.TCPRouteRule{{
						BackendRefs: []gatewayv1alpha2.BackendRef{{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Name: "tcp-backend",
								Port: &servicePort,
							},
						}},
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "tcp-backend", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "tcp",
						Port:       7001,
						TargetPort: intstr.FromInt(7001),
						Protocol:   corev1.ProtocolTCP,
					}},
				},
			},
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tcp-backend-1",
					Namespace: "default",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "tcp-backend",
					},
				},
				Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](7001)}},
				Endpoints: []discoveryv1.Endpoint{{
					Addresses: []string{"10.0.0.51"},
				}},
			},
		).
		Build()

	xlator := translator.New(string(controllerName), slog.New(slog.NewTextHandler(io.Discard, nil)))
	snapshot, err := xlator.Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.Listeners) != 2 {
		t.Fatalf("expected 2 listeners, got %d", len(snapshot.Listeners))
	}

	listeners := make(map[string][]string, len(snapshot.Listeners))
	for _, listener := range snapshot.Listeners {
		listeners[listener.Name] = listener.AttachedRoutes
	}

	if got := listeners["default/gateway-tcp/tcp-9000"]; len(got) != 0 {
		t.Fatalf("expected no attachments on tcp-9000, got %#v", got)
	}
	if got := listeners["default/gateway-tcp/tcp-9001"]; len(got) != 1 || got[0] != "default/route-9001" {
		t.Fatalf("unexpected attachments on tcp-9001: %#v", got)
	}
}

func TestBuildSnapshotDifferentiatesTLSProtocolModes(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	passthroughMode := gatewayv1.TLSModePassthrough
	terminateMode := gatewayv1.TLSModeTerminate

	client := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
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
					Listeners: []gatewayv1.Listener{
						{
							Name:     "tls-passthrough",
							Protocol: gatewayv1.TLSProtocolType,
							Port:     443,
							TLS: &gatewayv1.ListenerTLSConfig{
								Mode: &passthroughMode,
							},
						},
						{
							Name:     "tls-terminate",
							Protocol: gatewayv1.TLSProtocolType,
							Port:     8443,
							TLS: &gatewayv1.ListenerTLSConfig{
								Mode: &terminateMode,
								CertificateRefs: []gatewayv1.SecretObjectReference{{
									Name: "gateway-cert",
								}},
							},
						},
					},
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
		).
		Build()

	xlator := translator.New(string(controllerName), slog.New(slog.NewTextHandler(io.Discard, nil)))
	snapshot, err := xlator.Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	var passthroughProto, terminateProto string
	for _, l := range snapshot.Listeners {
		switch l.Name {
		case "default/gw-tls/tls-passthrough":
			passthroughProto = l.Protocol
		case "default/gw-tls/tls-terminate":
			terminateProto = l.Protocol
		}
	}

	if passthroughProto != "TLS_PASSTHROUGH" {
		t.Errorf("expected TLS listener with mode Passthrough to have protocol TLS_PASSTHROUGH, got %q", passthroughProto)
	}
	if terminateProto != "TLS" {
		t.Errorf("expected TLS listener with mode Terminate to have protocol TLS, got %q", terminateProto)
	}
}

func TestBuildSnapshotAnnotatesTLSRouteTerminateStreamRouteWithoutNativeHTTPRoutes(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	terminateMode := gatewayv1.TLSModeTerminate
	servicePort := gatewayv1.PortNumber(80)

	client := testutil.NewTranslatorClientBuilder(scheme).
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
			&gatewayv1alpha2.TLSRoute{
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

	xlator := translator.New(string(controllerName), slog.New(slog.NewTextHandler(io.Discard, nil)))
	snapshot, err := xlator.Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.HTTPRoutes) != 0 {
		t.Fatalf("expected no TLS-derived HTTPRoutes, got %#v", snapshot.HTTPRoutes)
	}
	if len(snapshot.StreamRoutes) != 1 {
		t.Fatalf("expected one TLS StreamRoute, got %#v", snapshot.StreamRoutes)
	}

	streamMatch := snapshot.StreamRoutes[0].Rules[0].Matches[0]
	if streamMatch.Mode != ir.TlsRouteModeTerminate {
		t.Fatalf("TLS StreamRoute match mode = %q, want %q", streamMatch.Mode, ir.TlsRouteModeTerminate)
	}

	streamBackend := snapshot.StreamRoutes[0].Rules[0].BackendRefs[0]
	if streamBackend.Name != "echo" || len(streamBackend.Metadata) != 0 {
		t.Fatalf("unexpected TLS StreamRoute backend ref: %#v", streamBackend)
	}
}

func TestBuildSnapshotSetsTLSRouteModesFromIntersectingListenersOnSharedPort(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	terminateMode := gatewayv1.TLSModeTerminate
	passthroughMode := gatewayv1.TLSModePassthrough
	terminateHostname := gatewayv1.Hostname("tls.example.com")
	passthroughHostname := gatewayv1.Hostname("abc.example.com")
	terminateBackendPort := gatewayv1.PortNumber(3000)
	passthroughBackendPort := gatewayv1.PortNumber(8443)

	client := testutil.NewTranslatorClientBuilder(scheme).
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
					Listeners: []gatewayv1.Listener{
						{
							Name:     "tls-terminate",
							Protocol: gatewayv1.TLSProtocolType,
							Port:     8883,
							Hostname: &terminateHostname,
							TLS: &gatewayv1.ListenerTLSConfig{
								Mode: &terminateMode,
								CertificateRefs: []gatewayv1.SecretObjectReference{{
									Name: "gateway-cert",
								}},
							},
						},
						{
							Name:     "tls-passthrough",
							Protocol: gatewayv1.TLSProtocolType,
							Port:     8883,
							Hostname: &passthroughHostname,
							TLS: &gatewayv1.ListenerTLSConfig{
								Mode: &passthroughMode,
							},
						},
					},
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
			&gatewayv1alpha2.TLSRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "terminate-route", Namespace: "default"},
				Spec: gatewayv1alpha2.TLSRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{Name: "gw-tls"}},
					},
					Hostnames: []gatewayv1.Hostname{terminateHostname},
					Rules: []gatewayv1alpha2.TLSRouteRule{{
						BackendRefs: []gatewayv1alpha2.BackendRef{{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Name: "tcp-backend",
								Port: &terminateBackendPort,
							},
						}},
					}},
				},
			},
			&gatewayv1alpha2.TLSRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "passthrough-route", Namespace: "default"},
				Spec: gatewayv1alpha2.TLSRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{Name: "gw-tls"}},
					},
					Hostnames: []gatewayv1.Hostname{passthroughHostname},
					Rules: []gatewayv1alpha2.TLSRouteRule{{
						BackendRefs: []gatewayv1alpha2.BackendRef{{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Name: "tcp-backend",
								Port: &passthroughBackendPort,
							},
						}},
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "tcp-backend", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Name: "raw", Port: 3000, TargetPort: intstr.FromInt(3000), Protocol: corev1.ProtocolTCP},
						{Name: "tls", Port: 8443, TargetPort: intstr.FromInt(8443), Protocol: corev1.ProtocolTCP},
					},
				},
			},
		).
		Build()

	xlator := translator.New(string(controllerName), slog.New(slog.NewTextHandler(io.Discard, nil)))
	snapshot, err := xlator.Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.HTTPRoutes) != 0 {
		t.Fatalf("expected no TLS-derived HTTPRoutes, got %#v", snapshot.HTTPRoutes)
	}
	if len(snapshot.StreamRoutes) != 2 {
		t.Fatalf("expected two TLS StreamRoutes, got %#v", snapshot.StreamRoutes)
	}

	modesByRoute := map[string]ir.TlsRouteMode{}
	for _, route := range snapshot.StreamRoutes {
		if len(route.Rules) == 0 || len(route.Rules[0].Matches) == 0 {
			t.Fatalf("route %s has no stream matches", route.Name)
		}
		modesByRoute[route.Name] = route.Rules[0].Matches[0].Mode
	}
	if got := modesByRoute["terminate-route"]; got != ir.TlsRouteModeTerminate {
		t.Fatalf("terminate-route mode = %q, want %q", got, ir.TlsRouteModeTerminate)
	}
	if got := modesByRoute["passthrough-route"]; got != ir.TlsRouteModePassthrough {
		t.Fatalf("passthrough-route mode = %q, want %q", got, ir.TlsRouteModePassthrough)
	}
}
