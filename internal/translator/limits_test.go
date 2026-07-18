package translator

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nantian-gw/gateway/internal/translator/shared"
)

func TestBuildReturnsErrorWhenInputObjectLimitExceeded(t *testing.T) {
	t.Parallel()

	client := newTranslatorLimitsFixture(t)
	xlator := NewWithOptions(
		"gateway.networking.k8s.io/nantian-gw",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		shared.Options{
			Limits: shared.Limits{
				MaxInputObjects: 3,
			},
		},
	)

	_, err := xlator.Build(context.Background(), client)
	if err == nil {
		t.Fatal("expected Build to fail when maxInputObjects is exceeded")
	}
	if !strings.Contains(err.Error(), "maxInputObjects") {
		t.Fatalf("expected maxInputObjects error, got %v", err)
	}
}

func TestBuildReturnsErrorWhenSnapshotObjectLimitExceeded(t *testing.T) {
	t.Parallel()

	client := newTranslatorLimitsFixture(t)
	xlator := NewWithOptions(
		"gateway.networking.k8s.io/nantian-gw",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		shared.Options{
			Limits: shared.Limits{
				MaxSnapshotObjects: 2,
			},
		},
	)

	_, err := xlator.Build(context.Background(), client)
	if err == nil {
		t.Fatal("expected Build to fail when maxSnapshotObjects is exceeded")
	}
	if !strings.Contains(err.Error(), "maxSnapshotObjects") {
		t.Fatalf("expected maxSnapshotObjects error, got %v", err)
	}
}

func TestBuildReturnsErrorWhenSnapshotEndpointLimitExceeded(t *testing.T) {
	t.Parallel()

	client := newTranslatorLimitsFixture(t)
	xlator := NewWithOptions(
		"gateway.networking.k8s.io/nantian-gw",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		shared.Options{
			Limits: shared.Limits{
				MaxSnapshotEndpoints: 1,
			},
		},
	)

	_, err := xlator.Build(context.Background(), client)
	if err == nil {
		t.Fatal("expected Build to fail when maxSnapshotEndpoints is exceeded")
	}
	if !strings.Contains(err.Error(), "maxSnapshotEndpoints") {
		t.Fatalf("expected maxSnapshotEndpoints error, got %v", err)
	}
}

func TestBuildIgnoresDisabledSnapshotLimits(t *testing.T) {
	t.Parallel()

	client := newTranslatorLimitsFixture(t)
	xlator := NewWithOptions(
		"gateway.networking.k8s.io/nantian-gw",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		shared.Options{
			Limits: shared.Limits{
				MaxInputObjects:      0,
				MaxSnapshotObjects:   0,
				MaxSnapshotEndpoints: 0,
			},
		},
	)

	snapshot, err := xlator.Build(context.Background(), client)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(snapshot.Listeners) != 1 {
		t.Fatalf("expected 1 listener, got %d", len(snapshot.Listeners))
	}
	if len(snapshot.Backends) != 1 {
		t.Fatalf("expected 1 backend, got %d", len(snapshot.Backends))
	}
	if got := len(snapshot.Backends[0].Endpoints); got != 2 {
		t.Fatalf("expected 2 backend endpoints, got %d", got)
	}
}

func newTranslatorLimitsFixture(t *testing.T) client.Client {
	t.Helper()

	scheme := buildSupportScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	pathType := gatewayv1.PathMatchPathPrefix
	hostname := gatewayv1.Hostname("example.com")
	portNumber := gatewayv1.PortNumber(8080)

	return newTranslatorClientBuilder(scheme).
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
						Hostname: &hostname,
					}},
				},
				Status: gatewayv1.GatewayStatus{
					Listeners: []gatewayv1.ListenerStatus{{
						Name:           "http",
						AttachedRoutes: 1,
						Conditions: []metav1.Condition{
							{
								Type:               string(gatewayv1.ListenerConditionAccepted),
								Status:             metav1.ConditionTrue,
								ObservedGeneration: 1,
							},
							{
								Type:               string(gatewayv1.ListenerConditionProgrammed),
								Status:             metav1.ConditionTrue,
								ObservedGeneration: 1,
							},
							{
								Type:               string(gatewayv1.ListenerConditionResolvedRefs),
								Status:             metav1.ConditionTrue,
								ObservedGeneration: 1,
							},
						},
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
					Hostnames: []gatewayv1.Hostname{hostname},
					Rules: []gatewayv1.HTTPRouteRule{{
						Matches: []gatewayv1.HTTPRouteMatch{{
							Path: &gatewayv1.HTTPPathMatch{
								Type:  &pathType,
								Value: ptr("/"),
							},
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
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "echo-pod", Namespace: "default"},
				Status: corev1.PodStatus{
					PodIP: "10.0.0.20",
					Phase: corev1.PodRunning,
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
				Endpoints: []discoveryv1.Endpoint{
					{Addresses: []string{"10.0.0.10"}},
					{Addresses: []string{"10.0.0.11"}},
				},
			},
		).
		Build()
}
