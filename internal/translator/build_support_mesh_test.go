package translator

import (
	"context"
	"io"
	"log/slog"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/mesh"
	"github.com/nantian-gw/gateway/internal/translator/testutil"
)

func TestBuildLoadsMeshShadowBackendsOnDemand(t *testing.T) {
	scheme := testutil.BuildSupportScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	portNumber := gatewayv1.PortNumber(8080)

	baseClient := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "apps"},
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
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "apps"},
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
									Name: "api",
									Port: &portNumber,
								},
							},
						}},
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api",
					Namespace: "apps",
					Annotations: map[string]string{
						mesh.ManagedServiceAnnotation: "true",
						mesh.ShadowServiceAnnotation:  "nantian-gw-shadow-api",
					},
				},
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
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nantian-gw-shadow-api",
					Namespace: "apps",
					Labels: map[string]string{
						mesh.ShadowServiceRoleLabel:        mesh.ShadowServiceRoleValue,
						mesh.OriginalServiceNamespaceLabel: "apps",
						mesh.OriginalServiceNameLabel:      "api",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "http",
						Port:       8080,
						TargetPort: intstr.FromInt(18080),
						Protocol:   corev1.ProtocolTCP,
					}},
				},
			},
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nantian-gw-shadow-api-1",
					Namespace: "apps",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "nantian-gw-shadow-api",
					},
				},
				Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](18080)}},
				Endpoints: []discoveryv1.Endpoint{{
					Addresses: []string{"10.0.0.20"},
				}},
			},
		).
		Build()

	snapshot, err := New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), testutil.NewFakeScopedBuildDependencyValidatingClient(baseClient, nil))
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.Backends) != 1 {
		t.Fatalf("expected 1 backend, got %#v", snapshot.Backends)
	}
	if got := snapshot.Backends[0].Name; got != "api:8080" {
		t.Fatalf("backend name = %q, want %q", got, "api:8080")
	}
	if len(snapshot.Backends[0].Endpoints) != 1 {
		t.Fatalf("expected 1 shadow endpoint, got %#v", snapshot.Backends[0].Endpoints)
	}
	if got := snapshot.Backends[0].Endpoints[0].Address; got != "10.0.0.20" {
		t.Fatalf("backend endpoint address = %q, want %q", got, "10.0.0.20")
	}
	if got := snapshot.Backends[0].Endpoints[0].Port; got != 18080 {
		t.Fatalf("backend endpoint port = %d, want %d", got, 18080)
	}
}

func TestBuildLoadsMeshWorkloadsPerRouteNamespace(t *testing.T) {
	scheme := testutil.BuildSupportScheme(t)
	portNumber := gatewayv1.PortNumber(8080)

	baseClient := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "apps"},
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
					Addresses: []string{"10.0.0.10"},
				}},
			},
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "client-a", Namespace: "apps"},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					PodIP: "10.1.0.10",
				},
			},
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "client-b", Namespace: "other"},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					PodIP: "10.2.0.20",
				},
			},
		).
		Build()

	snapshot, err := New(
		"gateway.networking.k8s.io/nantian-gw",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), testutil.NewFakeScopedBuildDependencyValidatingClient(baseClient,
		map[string]struct{}{"apps": {}},
	))
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.Workloads) != 1 {
		t.Fatalf("expected 1 workload from mesh route namespaces, got %#v", snapshot.Workloads)
	}
	if got := snapshot.Workloads[0].Namespace + "/" + snapshot.Workloads[0].Name; got != "apps/client-a" {
		t.Fatalf("unexpected workload set: %#v", snapshot.Workloads)
	}
}

func TestBuildBackendsForSnapshotUsesMeshShadowServiceEndpoints(t *testing.T) {
	scheme := testutil.BuildSupportScheme(t)

	baseClient := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api",
					Namespace: "apps",
					Annotations: map[string]string{
						mesh.ManagedServiceAnnotation: "true",
						mesh.ShadowServiceAnnotation:  "nantian-gw-shadow-api",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "http",
						Port:       8080,
						TargetPort: intstr.FromInt(26060),
						Protocol:   corev1.ProtocolTCP,
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nantian-gw-shadow-api",
					Namespace: "apps",
					Labels: map[string]string{
						mesh.ShadowServiceRoleLabel:        mesh.ShadowServiceRoleValue,
						mesh.OriginalServiceNamespaceLabel: "apps",
						mesh.OriginalServiceNameLabel:      "api",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "http",
						Port:       8080,
						TargetPort: intstr.FromInt(18080),
						Protocol:   corev1.ProtocolTCP,
					}},
				},
			},
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nantian-gw-shadow-api-1",
					Namespace: "apps",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "nantian-gw-shadow-api",
					},
				},
				Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](18080)}},
				Endpoints: []discoveryv1.Endpoint{{
					Addresses: []string{"10.0.0.20"},
				}},
			},
		).
		Build()

	current := &ir.Snapshot{
		Backends: []ir.BackendCluster{{
			Name:      "api:8080",
			Namespace: "apps",
			Endpoints: []ir.BackendEndpoint{{
				Address: "10.0.0.10",
				Port:    8080,
				Healthy: true,
			}},
			Metadata: map[string]string{
				"service": "api",
			},
		}},
	}

	backends, err := New(
		"gateway.networking.k8s.io/nantian-gw",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).BuildBackendsForSnapshot(
		context.Background(),
		baseClient,
		current,
		[]client.ObjectKey{{Namespace: "apps", Name: "api"}},
		nil,
	)
	if err != nil {
		t.Fatalf("BuildBackendsForSnapshot returned error: %v", err)
	}

	if len(backends) != 1 {
		t.Fatalf("expected one backend, got %#v", backends)
	}
	if len(backends[0].Endpoints) != 1 {
		t.Fatalf("expected backend to use shadow endpoint, got %#v", backends[0])
	}
	if got := backends[0].Endpoints[0].Address; got != "10.0.0.20" {
		t.Fatalf("backend endpoint address = %q, want %q", got, "10.0.0.20")
	}
	if got := backends[0].Endpoints[0].Port; got != 18080 {
		t.Fatalf("backend endpoint port = %d, want %d", got, 18080)
	}
}

func TestBuildBackendsForSnapshotRefreshesLogicalBackendFromShadowServiceChange(t *testing.T) {
	scheme := testutil.BuildSupportScheme(t)

	baseClient := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api",
					Namespace: "apps",
					Annotations: map[string]string{
						mesh.ManagedServiceAnnotation: "true",
						mesh.ShadowServiceAnnotation:  "nantian-gw-shadow-api",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "http",
						Port:       8080,
						TargetPort: intstr.FromInt(26060),
						Protocol:   corev1.ProtocolTCP,
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nantian-gw-shadow-api",
					Namespace: "apps",
					Labels: map[string]string{
						mesh.ShadowServiceRoleLabel:        mesh.ShadowServiceRoleValue,
						mesh.OriginalServiceNamespaceLabel: "apps",
						mesh.OriginalServiceNameLabel:      "api",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "http",
						Port:       8080,
						TargetPort: intstr.FromInt(18080),
						Protocol:   corev1.ProtocolTCP,
					}},
				},
			},
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nantian-gw-shadow-api-1",
					Namespace: "apps",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "nantian-gw-shadow-api",
					},
				},
				Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](18080)}},
				Endpoints: []discoveryv1.Endpoint{{
					Addresses: []string{"10.0.0.20"},
				}},
			},
		).
		Build()

	current := &ir.Snapshot{
		Backends: []ir.BackendCluster{{
			Name:      "api:8080",
			Namespace: "apps",
			Endpoints: []ir.BackendEndpoint{{
				Address: "10.0.0.10",
				Port:    8080,
				Healthy: true,
			}},
			Metadata: map[string]string{
				"service": "api",
			},
		}},
	}

	backends, err := New(
		"gateway.networking.k8s.io/nantian-gw",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).BuildBackendsForSnapshot(
		context.Background(),
		baseClient,
		current,
		[]client.ObjectKey{{Namespace: "apps", Name: "nantian-gw-shadow-api"}},
		nil,
	)
	if err != nil {
		t.Fatalf("BuildBackendsForSnapshot returned error: %v", err)
	}

	if len(backends) != 1 {
		t.Fatalf("expected one backend, got %#v", backends)
	}
	if len(backends[0].Endpoints) != 1 {
		t.Fatalf("expected backend to use shadow endpoint, got %#v", backends[0])
	}
	if got := backends[0].Endpoints[0].Address; got != "10.0.0.20" {
		t.Fatalf("backend endpoint address = %q, want %q", got, "10.0.0.20")
	}
	if got := backends[0].Endpoints[0].Port; got != 18080 {
		t.Fatalf("backend endpoint port = %d, want %d", got, 18080)
	}
}

func TestRebuildMeshServiceListenersLoadsParentServicesOnDemand(t *testing.T) {
	scheme := testutil.BuildSupportScheme(t)
	baseClient := testutil.NewTranslatorClientBuilder(scheme).
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
		).
		Build()

	current := &ir.Snapshot{
		Listeners: []ir.Listener{{
			Name:     "public-http",
			Protocol: "HTTP",
		}},
		HTTPRoutes: []ir.HTTPRoute{{
			Name:      "route",
			Namespace: "default",
			ParentRefs: []ir.ParentRef{{
				Kind:      "Service",
				Namespace: "default",
				Name:      "echo",
			}},
		}},
	}

	listeners, err := New(
		"gateway.networking.k8s.io/nantian-gw",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).RebuildMeshServiceListeners(context.Background(), testutil.NewFakeValidatingTranslatorClient(baseClient,
		map[reflect.Type]string{
			reflect.TypeOf(&corev1.ServiceList{}): "RebuildMeshServiceListeners should load parent Services on demand",
		},
	), current)
	if err != nil {
		t.Fatalf("RebuildMeshServiceListeners returned error: %v", err)
	}

	if len(listeners) != 2 {
		t.Fatalf("expected public + mesh listener set, got %#v", listeners)
	}

	meshListenerCount := 0
	for _, listener := range listeners {
		if listener.Metadata[mesh.FrontendKindMetadataKey] != mesh.FrontendKindService {
			continue
		}
		meshListenerCount++
		if got := listener.AttachedRoutes; len(got) != 1 || got[0] != "default/route" {
			t.Fatalf("unexpected mesh listener attachments: %#v", got)
		}
	}
	if meshListenerCount != 1 {
		t.Fatalf("expected 1 rebuilt mesh listener, got %#v", listeners)
	}
}
