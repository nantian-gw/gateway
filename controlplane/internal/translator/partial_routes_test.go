package translator

import (
	"context"
	"io"
	"log/slog"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/nantian-gw/gateway/controlplane/internal/ir"
)

func TestBuildRoutesForSnapshotAddsNewlyReferencedBackends(t *testing.T) {
	scheme := buildSupportScheme(t)
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

	cl := newTranslatorClientBuilder(scheme).
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

func TestBuildRoutesForSnapshotAddsTLSRouteTerminateStreamRoute(t *testing.T) {
	scheme := buildSupportScheme(t)
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

	cl := newTranslatorClientBuilder(scheme).
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
					"tls.crt": readTestTLSAsset(t, "client.crt"),
					"tls.key": readTestTLSAsset(t, "client.key"),
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
