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

	"github.com/nantian-gw/gateway/internal/ir"
)

func TestBuildMarksTCPRouteCrossNamespaceBackendWithoutGrant(t *testing.T) {
	otherNamespace := gatewayv1.Namespace("other")
	servicePort := gatewayv1.PortNumber(7000)

	snapshot := buildTCPRouteSupplementalSnapshot(t,
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "other"},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "other"},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{
					Name:       "tcp",
					Port:       7000,
					TargetPort: intstr.FromInt(7000),
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
			Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](7000)}},
			Endpoints: []discoveryv1.Endpoint{{
				Addresses: []string{"10.0.0.20"},
			}},
		},
		&gatewayv1alpha2.TCPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "cross-ns", Namespace: "default"},
			Spec: gatewayv1alpha2.TCPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{Name: "gw"}},
				},
				Rules: []gatewayv1alpha2.TCPRouteRule{{
					BackendRefs: []gatewayv1alpha2.BackendRef{{
						BackendObjectReference: gatewayv1.BackendObjectReference{
							Name:      "echo",
							Namespace: &otherNamespace,
							Port:      &servicePort,
						},
					}},
				}},
			},
		},
	)

	backend := snapshot.StreamRoutes[0].Rules[0].BackendRefs[0]
	if backend.Metadata[backendRefMetaValid] != "false" {
		t.Fatalf("expected invalid TCPRoute backend metadata, got %#v", backend.Metadata)
	}
	if backend.Metadata[backendRefMetaReason] != string(gatewayv1.RouteReasonRefNotPermitted) {
		t.Fatalf("unexpected TCPRoute backend reason: %#v", backend.Metadata)
	}
}

func TestBuildAllowsTCPRouteCrossNamespaceBackendWithReferenceGrant(t *testing.T) {
	otherNamespace := gatewayv1.Namespace("other")
	servicePort := gatewayv1.PortNumber(7000)

	snapshot := buildTCPRouteSupplementalSnapshot(t,
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "other"},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "other"},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{
					Name:       "tcp",
					Port:       7000,
					TargetPort: intstr.FromInt(7000),
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
			Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](7000)}},
			Endpoints: []discoveryv1.Endpoint{{
				Addresses: []string{"10.0.0.20"},
			}},
		},
		&gatewayv1beta1.ReferenceGrant{
			ObjectMeta: metav1.ObjectMeta{Name: "allow-tcp", Namespace: "other"},
			Spec: gatewayv1beta1.ReferenceGrantSpec{
				From: []gatewayv1beta1.ReferenceGrantFrom{{
					Group:     gatewayv1beta1.Group(gatewayv1.GroupName),
					Kind:      gatewayv1beta1.Kind("TCPRoute"),
					Namespace: gatewayv1beta1.Namespace("default"),
				}},
				To: []gatewayv1beta1.ReferenceGrantTo{{
					Group: "",
					Kind:  "Service",
					Name:  objectNamePtr("echo"),
				}},
			},
		},
		&gatewayv1alpha2.TCPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "cross-ns", Namespace: "default"},
			Spec: gatewayv1alpha2.TCPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{Name: "gw"}},
				},
				Rules: []gatewayv1alpha2.TCPRouteRule{{
					BackendRefs: []gatewayv1alpha2.BackendRef{{
						BackendObjectReference: gatewayv1.BackendObjectReference{
							Name:      "echo",
							Namespace: &otherNamespace,
							Port:      &servicePort,
						},
					}},
				}},
			},
		},
	)

	backend := snapshot.StreamRoutes[0].Rules[0].BackendRefs[0]
	if len(backend.Metadata) != 0 {
		t.Fatalf("expected ReferenceGrant to keep TCPRoute backend valid, got %#v", backend.Metadata)
	}
}

func buildTCPRouteSupplementalSnapshot(t *testing.T, objects ...runtime.Object) *ir.Snapshot {
	t.Helper()

	scheme := buildSupportScheme(t)
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
					Name:     "tcp",
					Protocol: gatewayv1.TCPProtocolType,
					Port:     9000,
				}},
			},
		},
	}
	baseObjects = append(baseObjects, objects...)

	cl := newTranslatorClientBuilder(scheme).
		WithRuntimeObjects(baseObjects...).
		Build()

	snapshot, err := New(
		"gateway.networking.k8s.io/nantian-gw",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), cl)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(snapshot.StreamRoutes) != 1 {
		t.Fatalf("expected 1 TCPRoute stream route, got %d", len(snapshot.StreamRoutes))
	}
	return snapshot
}
