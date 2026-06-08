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
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/gatewayapi"
	backendlbv1alpha2 "github.com/nantian-gw/gateway/internal/gatewayapiexperimental/backendlbv1alpha2"
	"github.com/nantian-gw/gateway/internal/ir"
)

func TestBuildScopesReferenceGrantAndPolicyListsByBackendNamespace(t *testing.T) {
	scheme := buildSupportScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	portNumber := gatewayv1.PortNumber(8080)
	caBundle := gatewayv1.WellKnownCACertificatesSystem
	sessionType := gatewayv1.CookieBasedSessionPersistence

	echoTLSPolicy := &gatewayv1alpha3.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "echo-tls", Namespace: "backends"},
		Spec: gatewayv1.BackendTLSPolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
				LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
					Group: "",
					Kind:  "Service",
					Name:  "echo",
				},
			}},
			Validation: gatewayv1.BackendTLSPolicyValidation{
				Hostname:                "echo.backends.svc.cluster.local",
				WellKnownCACertificates: &caBundle,
			},
		},
	}
	echoTLSPolicyRaw, err := gatewayapi.EncodeBackendTLSPolicyV1(echoTLSPolicy)
	if err != nil {
		t.Fatalf("encode BackendTLSPolicy: %v", err)
	}

	baseClient := newTranslatorClientBuilder(scheme).
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
									Name:      "echo",
									Namespace: ptr[gatewayv1.Namespace]("backends"),
									Port:      &portNumber,
								},
							},
						}},
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "backends"},
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
					Namespace: "backends",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "echo",
					},
				},
				Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](8080)}},
				Endpoints: []discoveryv1.Endpoint{{
					Addresses: []string{"10.0.0.10"},
				}},
			},
			&gatewayv1beta1.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{Name: "allow-apps", Namespace: "backends"},
				Spec: gatewayv1beta1.ReferenceGrantSpec{
					From: []gatewayv1beta1.ReferenceGrantFrom{{
						Group:     gatewayv1beta1.Group(gatewayv1.GroupName),
						Kind:      gatewayv1beta1.Kind("HTTPRoute"),
						Namespace: gatewayv1beta1.Namespace("apps"),
					}},
					To: []gatewayv1beta1.ReferenceGrantTo{{
						Group: gatewayv1beta1.Group(""),
						Kind:  gatewayv1beta1.Kind("Service"),
						Name:  objectNamePtr("echo"),
					}},
				},
			},
			&backendlbv1alpha2.BackendLBPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "echo-lb", Namespace: "backends"},
				Spec: backendlbv1alpha2.BackendLBPolicySpec{
					TargetRefs: []backendlbv1alpha2.LocalPolicyTargetReference{{
						Group: "",
						Kind:  "Service",
						Name:  "echo",
					}},
					SessionPersistence: &gatewayv1.SessionPersistence{
						Type: &sessionType,
					},
				},
			},
			echoTLSPolicyRaw,
		).
		Build()

	snapshot, err := New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), scopedBuildDependencyValidatingTranslatorClient{
		Client: baseClient,
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.Backends) != 1 {
		t.Fatalf("expected 1 backend, got %#v", snapshot.Backends)
	}
	if snapshot.Backends[0].SessionPersistence == nil {
		t.Fatalf("expected BackendLBPolicy to be applied, got %#v", snapshot.Backends[0])
	}
	if snapshot.Backends[0].BackendTLSValidation == nil {
		t.Fatalf("expected BackendTLSPolicy to be applied, got %#v", snapshot.Backends[0])
	}
	if len(snapshot.HTTPRoutes) != 1 || len(snapshot.HTTPRoutes[0].Rules) != 1 || len(snapshot.HTTPRoutes[0].Rules[0].BackendRefs) != 1 {
		t.Fatalf("unexpected translated routes: %#v", snapshot.HTTPRoutes)
	}
	if got := snapshot.HTTPRoutes[0].Rules[0].BackendRefs[0].Metadata; len(got) != 0 {
		t.Fatalf("expected cross-namespace backend ref to remain valid, got %#v", got)
	}
}
func TestRefreshBackendRefMetadataLoadsReferencedBackendsOnDemand(t *testing.T) {
	scheme := buildSupportScheme(t)
	baseClient := newTranslatorClientBuilder(scheme).
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
			&mcsv1alpha1.ServiceImport{
				ObjectMeta: metav1.ObjectMeta{Name: "remote", Namespace: "default"},
				Spec: mcsv1alpha1.ServiceImportSpec{
					Ports: []mcsv1alpha1.ServicePort{{
						Name:     "grpc",
						Port:     9090,
						Protocol: corev1.ProtocolTCP,
					}},
				},
			},
		).
		Build()

	current := &ir.Snapshot{
		HTTPRoutes: []ir.HTTPRoute{{
			Name:      "route",
			Namespace: "default",
			Rules: []ir.HTTPRule{{
				BackendRefs: []ir.BackendRef{{
					Namespace: "default",
					Name:      "echo",
					Port:      8080,
				}},
			}},
		}},
		GRPCRoutes: []ir.GRPCRoute{{
			Name:      "grpc",
			Namespace: "default",
			Rules: []ir.GRPCRule{{
				BackendRefs: []ir.BackendRef{{
					Group:     mcsv1alpha1.GroupName,
					Kind:      "ServiceImport",
					Namespace: "default",
					Name:      "remote",
					Port:      9090,
				}},
			}},
		}},
	}

	httpRoutes, grpcRoutes, _, err := New(
		"gateway.networking.k8s.io/nantian-gw",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).RefreshBackendRefMetadata(context.Background(), validatingTranslatorClient{
		Client: baseClient,
		forbiddenLists: map[reflect.Type]string{
			reflect.TypeOf(&corev1.ServiceList{}):            "RefreshBackendRefMetadata should load Services on demand",
			reflect.TypeOf(&mcsv1alpha1.ServiceImportList{}): "RefreshBackendRefMetadata should load ServiceImports on demand",
		},
	}, current)
	if err != nil {
		t.Fatalf("RefreshBackendRefMetadata returned error: %v", err)
	}

	if got := httpRoutes[0].Rules[0].BackendRefs[0].Metadata; len(got) != 0 {
		t.Fatalf("expected referenced Service backend ref to remain valid, got %#v", got)
	}
	if got := grpcRoutes[0].Rules[0].BackendRefs[0].Metadata; len(got) != 0 {
		t.Fatalf("expected referenced ServiceImport backend ref to remain valid, got %#v", got)
	}
}
func TestRefreshBackendRefMetadataListsReferenceGrantsPerBackendNamespace(t *testing.T) {
	scheme := buildSupportScheme(t)
	baseClient := newTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "backends"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "http",
						Port:       8080,
						TargetPort: intstr.FromInt(8080),
						Protocol:   corev1.ProtocolTCP,
					}},
				},
			},
			&gatewayv1beta1.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{Name: "allow-apps", Namespace: "backends"},
				Spec: gatewayv1beta1.ReferenceGrantSpec{
					From: []gatewayv1beta1.ReferenceGrantFrom{{
						Group:     gatewayv1beta1.Group(gatewayv1.GroupName),
						Kind:      gatewayv1beta1.Kind("HTTPRoute"),
						Namespace: gatewayv1beta1.Namespace("apps"),
					}},
					To: []gatewayv1beta1.ReferenceGrantTo{{
						Group: gatewayv1beta1.Group(""),
						Kind:  gatewayv1beta1.Kind("Service"),
						Name:  objectNamePtr("echo"),
					}},
				},
			},
			&gatewayv1beta1.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{Name: "allow-other", Namespace: "other"},
				Spec: gatewayv1beta1.ReferenceGrantSpec{
					From: []gatewayv1beta1.ReferenceGrantFrom{{
						Group:     gatewayv1beta1.Group(gatewayv1.GroupName),
						Kind:      gatewayv1beta1.Kind("HTTPRoute"),
						Namespace: gatewayv1beta1.Namespace("apps"),
					}},
					To: []gatewayv1beta1.ReferenceGrantTo{{
						Group: gatewayv1beta1.Group(""),
						Kind:  gatewayv1beta1.Kind("Service"),
						Name:  objectNamePtr("other"),
					}},
				},
			},
		).
		Build()

	current := &ir.Snapshot{
		HTTPRoutes: []ir.HTTPRoute{{
			Name:      "route",
			Namespace: "apps",
			Rules: []ir.HTTPRule{{
				BackendRefs: []ir.BackendRef{{
					Namespace: "backends",
					Name:      "echo",
					Port:      8080,
				}},
			}},
		}},
	}

	httpRoutes, _, _, err := New(
		"gateway.networking.k8s.io/nantian-gw",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).RefreshBackendRefMetadata(context.Background(), fakeScopedReferenceGrantValidatingTranslatorClient{
		Client: baseClient,
	}, current)
	if err != nil {
		t.Fatalf("RefreshBackendRefMetadata returned error: %v", err)
	}

	if got := httpRoutes[0].Rules[0].BackendRefs[0].Metadata; len(got) != 0 {
		t.Fatalf("expected cross-namespace backend ref to remain valid, got %#v", got)
	}
}
func TestRefreshBackendRefMetadataSkipsReferenceGrantLookupForSameNamespaceBackends(t *testing.T) {
	scheme := buildSupportScheme(t)
	baseClient := newTranslatorClientBuilder(scheme).
		WithObjects(
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
		).
		Build()

	current := &ir.Snapshot{
		HTTPRoutes: []ir.HTTPRoute{{
			Name:      "route",
			Namespace: "apps",
			Rules: []ir.HTTPRule{{
				BackendRefs: []ir.BackendRef{{
					Namespace: "apps",
					Name:      "echo",
					Port:      8080,
				}},
			}},
		}},
	}

	httpRoutes, _, _, err := New(
		"gateway.networking.k8s.io/nantian-gw",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).RefreshBackendRefMetadata(context.Background(), validatingTranslatorClient{
		Client: baseClient,
		forbiddenLists: map[reflect.Type]string{
			reflect.TypeOf(&gatewayv1beta1.ReferenceGrantList{}): "same-namespace backend refs should not list ReferenceGrants",
		},
	}, current)
	if err != nil {
		t.Fatalf("RefreshBackendRefMetadata returned error: %v", err)
	}

	if got := httpRoutes[0].Rules[0].BackendRefs[0].Metadata; len(got) != 0 {
		t.Fatalf("expected same-namespace backend ref to remain valid, got %#v", got)
	}
}
