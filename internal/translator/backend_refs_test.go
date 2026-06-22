package translator

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
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/extfilter"
	"github.com/nantian-gw/gateway/internal/ir"
)

func TestBuildMarksInvalidBackendKind(t *testing.T) {
	snapshot := buildTranslatorSnapshot(t,
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "invalid-kind", Namespace: "default"},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{Name: "gw"}},
				},
				Rules: []gatewayv1.HTTPRouteRule{{
					BackendRefs: []gatewayv1.HTTPBackendRef{{
						BackendRef: gatewayv1.BackendRef{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Group: ptr[gatewayv1.Group]("unknownkind.example.com"),
								Kind:  ptr[gatewayv1.Kind]("NonExistent"),
								Name:  "echo",
								Port:  ptr[gatewayv1.PortNumber](8080),
							},
						},
					}},
				}},
			},
		},
	)

	backend := snapshot.HTTPRoutes[0].Rules[0].BackendRefs[0]
	if backend.Metadata[backendRefMetaValid] != "false" {
		t.Fatalf("expected invalid backend metadata, got %#v", backend.Metadata)
	}
	if backend.Metadata[backendRefMetaReason] != string(gatewayv1.RouteReasonInvalidKind) {
		t.Fatalf("unexpected invalid backend reason: %#v", backend.Metadata)
	}
}

func TestBuildMarksCrossNamespaceBackendWithoutGrant(t *testing.T) {
	snapshot := buildTranslatorSnapshot(t,
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "other"},
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
				Name:      "echo-other-1",
				Namespace: "other",
				Labels: map[string]string{
					discoveryv1.LabelServiceName: "echo",
				},
			},
			Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](8080)}},
			Endpoints: []discoveryv1.Endpoint{{
				Addresses: []string{"10.0.0.20"},
			}},
		},
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "cross-ns", Namespace: "default"},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{Name: "gw"}},
				},
				Rules: []gatewayv1.HTTPRouteRule{{
					BackendRefs: []gatewayv1.HTTPBackendRef{{
						BackendRef: gatewayv1.BackendRef{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Name:      "echo",
								Namespace: ptr[gatewayv1.Namespace]("other"),
								Port:      ptr[gatewayv1.PortNumber](8080),
							},
						},
					}},
				}},
			},
		},
	)

	backend := snapshot.HTTPRoutes[0].Rules[0].BackendRefs[0]
	if backend.Metadata[backendRefMetaValid] != "false" {
		t.Fatalf("expected cross-namespace backend metadata, got %#v", backend.Metadata)
	}
	if backend.Metadata[backendRefMetaReason] != string(gatewayv1.RouteReasonRefNotPermitted) {
		t.Fatalf("unexpected cross-namespace backend reason: %#v", backend.Metadata)
	}
}

func TestBuildAllowsCrossNamespaceBackendForMeshServiceParent(t *testing.T) {
	snapshot := buildTranslatorSnapshot(t,
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-mesh"},
		},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-mesh-consumer"},
		},
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
		&discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "echo-v1-1",
				Namespace: "gateway-conformance-mesh",
				Labels: map[string]string{
					discoveryv1.LabelServiceName: "echo-v1",
				},
			},
			Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](8080)}},
			Endpoints: []discoveryv1.Endpoint{{
				Addresses: []string{"10.0.0.20"},
			}},
		},
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mesh-echo-add-header",
				Namespace: "gateway-conformance-mesh-consumer",
			},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{
						Group:     ptr[gatewayv1.Group](""),
						Kind:      ptr[gatewayv1.Kind]("Service"),
						Name:      "echo-v1",
						Namespace: ptr[gatewayv1.Namespace]("gateway-conformance-mesh"),
					}},
				},
				Rules: []gatewayv1.HTTPRouteRule{{
					BackendRefs: []gatewayv1.HTTPBackendRef{{
						BackendRef: gatewayv1.BackendRef{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Name:      "echo-v1",
								Namespace: ptr[gatewayv1.Namespace]("gateway-conformance-mesh"),
								Port:      ptr[gatewayv1.PortNumber](80),
							},
						},
					}},
				}},
			},
		},
	)

	backend := snapshot.HTTPRoutes[0].Rules[0].BackendRefs[0]
	if len(backend.Metadata) != 0 {
		t.Fatalf("expected mesh backend ref to remain valid, got %#v", backend.Metadata)
	}
}

func TestBuildPreservesHTTPBackendRefFilters(t *testing.T) {
	snapshot := buildTranslatorSnapshot(t,
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "backend-filters", Namespace: "default"},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{Name: "gw"}},
				},
				Rules: []gatewayv1.HTTPRouteRule{{
					BackendRefs: []gatewayv1.HTTPBackendRef{{
						BackendRef: gatewayv1.BackendRef{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Name: "echo",
								Port: ptr[gatewayv1.PortNumber](8080),
							},
						},
						Filters: []gatewayv1.HTTPRouteFilter{{
							Type: gatewayv1.HTTPRouteFilterRequestHeaderModifier,
							RequestHeaderModifier: &gatewayv1.HTTPHeaderFilter{
								Set: []gatewayv1.HTTPHeader{{
									Name:  "X-Backend-Only",
									Value: "true",
								}},
							},
						}},
					}},
				}},
			},
		},
	)

	backend := snapshot.HTTPRoutes[0].Rules[0].BackendRefs[0]
	if len(backend.Filters) != 1 {
		t.Fatalf("expected 1 backend filter, got %#v", backend.Filters)
	}
	if backend.Filters[0].Type != string(gatewayv1.HTTPRouteFilterRequestHeaderModifier) {
		t.Fatalf("unexpected backend filter type: %#v", backend.Filters[0])
	}
}

func TestBuildPreservesHTTPBackendRefCORSExtensionFilter(t *testing.T) {
	snapshot := buildTranslatorSnapshot(t,
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "backend-cors", Namespace: "default"},
			Data: map[string]string{
				extfilter.ConfigMapDataKey: `
type: CORS
cors:
  allowOrigins:
    - https://app.example
  allowMethods:
    - GET
    - POST
  maxAge: 600
`,
			},
		},
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "backend-cors-filters", Namespace: "default"},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{Name: "gw"}},
				},
				Rules: []gatewayv1.HTTPRouteRule{{
					BackendRefs: []gatewayv1.HTTPBackendRef{{
						BackendRef: gatewayv1.BackendRef{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Name: "echo",
								Port: ptr[gatewayv1.PortNumber](8080),
							},
						},
						Filters: []gatewayv1.HTTPRouteFilter{{
							Type: gatewayv1.HTTPRouteFilterExtensionRef,
							ExtensionRef: &gatewayv1.LocalObjectReference{
								Group: "",
								Kind:  "ConfigMap",
								Name:  "backend-cors",
							},
						}},
					}},
				}},
			},
		},
	)

	backend := snapshot.HTTPRoutes[0].Rules[0].BackendRefs[0]
	if len(backend.Filters) != 1 {
		t.Fatalf("expected 1 backend filter, got %#v", backend.Filters)
	}
	if backend.Filters[0].Type != "CORS" {
		t.Fatalf("unexpected backend filter type: %#v", backend.Filters[0])
	}
	if got := backend.Filters[0].Config["maxAge"]; got != 600 {
		t.Fatalf("unexpected backend filter maxAge: %#v", got)
	}
}

func TestBuildAllowsServiceImportBackendRef(t *testing.T) {
	snapshot := buildTranslatorSnapshot(t,
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
		&discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "payments-import-1",
				Namespace: "default",
				Labels: map[string]string{
					mcsv1alpha1.LabelServiceName: "payments",
				},
			},
			Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](19443)}},
			Endpoints: []discoveryv1.Endpoint{{
				Addresses: []string{"10.0.0.30"},
			}},
		},
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "serviceimport", Namespace: "default"},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{Name: "gw"}},
				},
				Rules: []gatewayv1.HTTPRouteRule{{
					BackendRefs: []gatewayv1.HTTPBackendRef{{
						BackendRef: gatewayv1.BackendRef{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Group: ptr[gatewayv1.Group](mcsv1alpha1.GroupName),
								Kind:  ptr[gatewayv1.Kind]("ServiceImport"),
								Name:  "payments",
								Port:  ptr[gatewayv1.PortNumber](9443),
							},
						},
					}},
				}},
			},
		},
	)

	backend := snapshot.HTTPRoutes[0].Rules[0].BackendRefs[0]
	if len(backend.Metadata) != 0 {
		t.Fatalf("expected serviceimport backend ref to remain valid, got %#v", backend.Metadata)
	}
}

func TestBuildPreservesHTTPBackendRefWeights(t *testing.T) {
	snapshot := buildTranslatorSnapshot(t,
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "weighted", Namespace: "default"},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{Name: "gw"}},
				},
				Rules: []gatewayv1.HTTPRouteRule{{
					BackendRefs: []gatewayv1.HTTPBackendRef{
						{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "echo",
									Port: ptr[gatewayv1.PortNumber](8080),
								},
								Weight: ptr(int32(90)),
							},
						},
						{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "echo",
									Port: ptr[gatewayv1.PortNumber](8080),
								},
								Weight: ptr(int32(0)),
							},
						},
					},
				}},
			},
		},
	)

	refs := snapshot.HTTPRoutes[0].Rules[0].BackendRefs
	if len(refs) != 2 {
		t.Fatalf("expected 2 backend refs, got %#v", refs)
	}
	if refs[0].Weight != 90 {
		t.Fatalf("expected explicit weight 90, got %#v", refs[0])
	}
	if refs[1].Weight != 0 {
		t.Fatalf("expected explicit weight 0, got %#v", refs[1])
	}
}

func buildTranslatorSnapshot(t *testing.T, objects ...runtime.Object) *ir.Snapshot {
	t.Helper()

	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(mcsv1alpha1.AddToScheme(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	baseObjects := []runtime.Object{
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
				ControllerName: gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw"),
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
	}
	baseObjects = append(baseObjects, objects...)

	cl := newTranslatorClientBuilder(scheme).
		WithRuntimeObjects(baseObjects...).
		Build()

	xlator := New(
		"gateway.networking.k8s.io/nantian-gw",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	snapshot, err := xlator.Build(context.Background(), cl)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(snapshot.HTTPRoutes) != 1 {
		t.Fatalf("expected 1 http route, got %d", len(snapshot.HTTPRoutes))
	}
	return snapshot
}
