package controller

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/nantian-gw/gateway/controlplane/internal/ir"
	"github.com/nantian-gw/gateway/controlplane/internal/managedresources"
	"github.com/nantian-gw/gateway/controlplane/internal/observability"
	"github.com/nantian-gw/gateway/controlplane/internal/translator"
)

func TestReconcilePublishesSnapshotFromGatewayInputs(t *testing.T) {
	scheme := runtime.NewScheme()
	mustAddToScheme(t, scheme, gatewayv1.Install)
	mustAddToScheme(t, scheme, gatewayv1alpha2.Install)
	mustAddToScheme(t, scheme, gatewayv1beta1.Install)
	mustAddToScheme(t, scheme, corev1.AddToScheme)
	mustAddToScheme(t, scheme, discoveryv1.AddToScheme)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	hostname := gatewayv1.Hostname("example.com")
	pathType := gatewayv1.PathMatchPathPrefix
	portNumber := gatewayv1.PortNumber(8080)

	client := newControllerClientBuilder(scheme).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "default"},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{
						{
							Name:     "http",
							Protocol: gatewayv1.HTTPProtocolType,
							Port:     80,
							Hostname: &hostname,
						},
					},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{
							{
								Name:        "edge",
								SectionName: ptr[gatewayv1.SectionName]("http"),
							},
						},
					},
					Hostnames: []gatewayv1.Hostname{hostname},
					Rules: []gatewayv1.HTTPRouteRule{
						{
							Matches: []gatewayv1.HTTPRouteMatch{
								{
									Path: &gatewayv1.HTTPPathMatch{
										Type:  &pathType,
										Value: ptr("/"),
									},
								},
							},
							BackendRefs: []gatewayv1.HTTPBackendRef{
								{
									BackendRef: gatewayv1.BackendRef{
										BackendObjectReference: gatewayv1.BackendObjectReference{
											Name: "echo",
											Port: &portNumber,
										},
									},
								},
							},
						},
					},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Port:       8080,
							TargetPort: intstr.FromInt(8080),
							Protocol:   corev1.ProtocolTCP,
						},
					},
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
				Ports: []discoveryv1.EndpointPort{
					{Port: ptr[int32](8080)},
				},
				Endpoints: []discoveryv1.Endpoint{
					{Addresses: []string{"10.0.0.10"}},
				},
			},
		).
		Build()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	metrics := testMetrics()
	syncer := NewSyncer(
		client,
		translator.New(string(controllerName), logger),
		store,
		metrics,
		0,
		logger,
	)
	syncer.SetSettleDelay(0)

	if _, err := syncer.Reconcile(context.Background(), ctrl.Request{}); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	snapshot := store.Current()
	if snapshot == nil {
		t.Fatal("expected published snapshot")
	}
	if len(snapshot.Listeners) != 1 {
		t.Fatalf("expected 1 listener, got %d", len(snapshot.Listeners))
	}
	if len(snapshot.HTTPRoutes) != 1 {
		t.Fatalf("expected 1 http route, got %d", len(snapshot.HTTPRoutes))
	}
	if len(snapshot.Backends) != 1 {
		t.Fatalf("expected 1 backend, got %d", len(snapshot.Backends))
	}
	if got := snapshotHistogramSampleCount(t, metrics.SnapshotBuildDurationSeconds); got != 1 {
		t.Fatalf("snapshot build duration sample count = %d, want 1", got)
	}
	if got := snapshotHistogramVecSampleCount(t, metrics.SnapshotResourceCount, "listeners"); got != 1 {
		t.Fatalf("snapshot resource count sample count for listeners = %d, want 1", got)
	}
	if got := snapshotHistogramVecSampleSum(t, metrics.SnapshotResourceCount, "listeners"); got != 1 {
		t.Fatalf("snapshot resource count sum for listeners = %v, want 1", got)
	}
	if got := snapshotHistogramVecSampleSum(t, metrics.SnapshotResourceCount, "http_routes"); got != 1 {
		t.Fatalf("snapshot resource count sum for http_routes = %v, want 1", got)
	}
	if got := snapshotHistogramVecSampleSum(t, metrics.SnapshotResourceCount, "backends"); got != 1 {
		t.Fatalf("snapshot resource count sum for backends = %v, want 1", got)
	}
	if got := snapshotHistogramSampleCount(t, metrics.SnapshotListenerAttachedRoutes); got != 1 {
		t.Fatalf("snapshot listener attached routes sample count = %d, want 1", got)
	}
	if got := snapshotHistogramSampleSum(t, metrics.SnapshotListenerAttachedRoutes); got != 1 {
		t.Fatalf("snapshot listener attached routes sample sum = %v, want 1", got)
	}
}

func TestReconcileAllowsNilMetrics(t *testing.T) {
	scheme := runtime.NewScheme()
	mustAddToScheme(t, scheme, gatewayv1.Install)
	mustAddToScheme(t, scheme, gatewayv1alpha2.Install)
	mustAddToScheme(t, scheme, gatewayv1beta1.Install)
	mustAddToScheme(t, scheme, corev1.AddToScheme)
	mustAddToScheme(t, scheme, discoveryv1.AddToScheme)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	syncer := NewSyncer(
		newControllerClientBuilder(scheme).Build(),
		translator.New("gateway.networking.k8s.io/nantian-gw", logger),
		store,
		nil,
		0,
		logger,
	)

	if _, err := syncer.Reconcile(context.Background(), ctrl.Request{}); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}
	if snapshot := store.Current(); snapshot == nil {
		t.Fatal("expected snapshot to be published with nil metrics")
	}
}

func TestReconcileIgnoresManagedFrontendResourceChanges(t *testing.T) {
	scheme := runtime.NewScheme()
	mustAddToScheme(t, scheme, gatewayv1.Install)
	mustAddToScheme(t, scheme, gatewayv1alpha2.Install)
	mustAddToScheme(t, scheme, gatewayv1beta1.Install)
	mustAddToScheme(t, scheme, corev1.AddToScheme)
	mustAddToScheme(t, scheme, discoveryv1.AddToScheme)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	hostname := gatewayv1.Hostname("example.com")
	pathType := gatewayv1.PathMatchPathPrefix
	portNumber := gatewayv1.PortNumber(8080)

	cl := newControllerClientBuilder(scheme).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "default"},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
						Hostname: &hostname,
					}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Name:        "edge",
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
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "echo-1",
					Namespace: "default",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "echo",
					},
				},
				Ports:     []discoveryv1.EndpointPort{{Port: ptr[int32](8080)}},
				Endpoints: []discoveryv1.Endpoint{{Addresses: []string{"10.0.0.10"}}},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nantian-dataplane",
					Namespace: "nantian-gw",
					Labels: map[string]string{
						managedresources.ManagedByLabel: managedresources.ManagedByValue,
						managedresources.ServiceRoleKey: managedresources.ServiceRoleShared,
					},
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeNodePort,
					Ports: []corev1.ServicePort{{
						Name:       "http-80",
						Port:       80,
						TargetPort: intstr.FromInt(80),
						NodePort:   30080,
						Protocol:   corev1.ProtocolTCP,
					}},
				},
			},
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aeg-shared-ep-nantian-dataplane-ipv4",
					Namespace: "nantian-gw",
					Labels: map[string]string{
						managedresources.ManagedByLabel: managedresources.ManagedByValue,
						managedresources.ServiceRoleKey: managedresources.EndpointSliceRoleSharedFrontend,
						discoveryv1.LabelManagedBy:      managedresources.ManagedByValue,
						discoveryv1.LabelServiceName:    "nantian-dataplane",
					},
				},
				AddressType: discoveryv1.AddressTypeIPv4,
				Ports:       []discoveryv1.EndpointPort{{Port: ptr[int32](80)}},
				Endpoints:   []discoveryv1.Endpoint{{Addresses: []string{"10.0.0.100"}}},
			},
		).
		Build()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	syncer := NewSyncer(
		cl,
		translator.New(string(controllerName), logger),
		store,
		testMetrics(),
		0,
		logger,
	)
	syncer.SetSettleDelay(0)

	if _, err := syncer.Reconcile(context.Background(), ctrl.Request{}); err != nil {
		t.Fatalf("initial Reconcile returned error: %v", err)
	}
	initialSnapshot := store.Current()
	if initialSnapshot == nil {
		t.Fatal("expected initial snapshot")
	}
	if len(initialSnapshot.Backends) != 1 {
		t.Fatalf("expected managed frontend resources to be excluded from backends, got %d", len(initialSnapshot.Backends))
	}
	initialVersion := initialSnapshot.ID

	var frontendSlice discoveryv1.EndpointSlice
	if err := cl.Get(
		context.Background(),
		client.ObjectKey{
			Namespace: "nantian-gw",
			Name:      "aeg-shared-ep-nantian-dataplane-ipv4",
		},
		&frontendSlice,
	); err != nil {
		t.Fatalf("Get managed frontend endpoint slice returned error: %v", err)
	}
	frontendSlice.Endpoints = []discoveryv1.Endpoint{{Addresses: []string{"10.0.0.101"}}}
	if err := cl.Update(context.Background(), &frontendSlice); err != nil {
		t.Fatalf("Update managed frontend endpoint slice returned error: %v", err)
	}

	if _, err := syncer.Reconcile(context.Background(), ctrl.Request{}); err != nil {
		t.Fatalf("second Reconcile returned error: %v", err)
	}
	if current := store.Current(); current == nil || current.ID != initialVersion {
		t.Fatalf("expected managed frontend resource changes to keep snapshot version %q, got %#v", initialVersion, current)
	}
}

func TestReconcileInvalidatesCrossNamespaceBackendWhenReferenceGrantDeleted(t *testing.T) {
	scheme := runtime.NewScheme()
	mustAddToScheme(t, scheme, gatewayv1.Install)
	mustAddToScheme(t, scheme, gatewayv1alpha2.Install)
	mustAddToScheme(t, scheme, gatewayv1beta1.Install)
	mustAddToScheme(t, scheme, corev1.AddToScheme)
	mustAddToScheme(t, scheme, discoveryv1.AddToScheme)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	pathType := gatewayv1.PathMatchPathPrefix
	servicePort := gatewayv1.PortNumber(8080)

	cl := newControllerClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "backend"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "default"},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{
						{
							Name:     "http",
							Protocol: gatewayv1.HTTPProtocolType,
							Port:     80,
						},
					},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "cross-ns", Namespace: "default"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{
							{
								Name:        "edge",
								SectionName: ptr[gatewayv1.SectionName]("http"),
							},
						},
					},
					Rules: []gatewayv1.HTTPRouteRule{
						{
							Matches: []gatewayv1.HTTPRouteMatch{
								{
									Path: &gatewayv1.HTTPPathMatch{
										Type:  &pathType,
										Value: ptr("/"),
									},
								},
							},
							BackendRefs: []gatewayv1.HTTPBackendRef{
								{
									BackendRef: gatewayv1.BackendRef{
										BackendObjectReference: gatewayv1.BackendObjectReference{
											Name:      "web",
											Namespace: ptr[gatewayv1.Namespace]("backend"),
											Port:      &servicePort,
										},
									},
								},
							},
						},
					},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "backend"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Port:       8080,
							TargetPort: intstr.FromInt(8080),
							Protocol:   corev1.ProtocolTCP,
						},
					},
				},
			},
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-1",
					Namespace: "backend",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "web",
					},
				},
				Ports: []discoveryv1.EndpointPort{
					{Port: ptr[int32](8080)},
				},
				Endpoints: []discoveryv1.Endpoint{
					{Addresses: []string{"10.0.0.20"}},
				},
			},
			&gatewayv1beta1.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{Name: "allow-web", Namespace: "backend"},
				Spec: gatewayv1beta1.ReferenceGrantSpec{
					From: []gatewayv1beta1.ReferenceGrantFrom{
						{
							Group:     gatewayv1beta1.Group(gatewayv1.GroupName),
							Kind:      gatewayv1beta1.Kind("HTTPRoute"),
							Namespace: gatewayv1beta1.Namespace("default"),
						},
					},
					To: []gatewayv1beta1.ReferenceGrantTo{
						{
							Group: gatewayv1beta1.Group(""),
							Kind:  gatewayv1beta1.Kind("Service"),
							Name:  ptr[gatewayv1beta1.ObjectName]("web"),
						},
					},
				},
			},
		).
		Build()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	syncer := NewSyncer(
		cl,
		translator.New(string(controllerName), logger),
		store,
		testMetrics(),
		0,
		logger,
	)
	syncer.SetSettleDelay(0)

	if _, err := syncer.Reconcile(context.Background(), ctrl.Request{}); err != nil {
		t.Fatalf("initial Reconcile returned error: %v", err)
	}

	snapshot := store.Current()
	if snapshot == nil || len(snapshot.HTTPRoutes) != 1 {
		t.Fatalf("expected snapshot with 1 http route, got %#v", snapshot)
	}
	metadata := snapshot.HTTPRoutes[0].Rules[0].BackendRefs[0].Metadata
	if metadata["nantian.dev/backend-ref-valid"] != "" {
		t.Fatalf("expected backend ref to start valid, got metadata %#v", metadata)
	}

	if err := cl.Delete(
		context.Background(),
		&gatewayv1beta1.ReferenceGrant{
			ObjectMeta: metav1.ObjectMeta{Name: "allow-web", Namespace: "backend"},
		},
	); err != nil {
		t.Fatalf("delete reference grant: %v", err)
	}

	if _, err := syncer.Reconcile(context.Background(), ctrl.Request{}); err != nil {
		t.Fatalf("reconcile after deleting reference grant returned error: %v", err)
	}

	snapshot = store.Current()
	if snapshot == nil || len(snapshot.HTTPRoutes) != 1 {
		t.Fatalf("expected snapshot with 1 http route after deleting grant, got %#v", snapshot)
	}
	metadata = snapshot.HTTPRoutes[0].Rules[0].BackendRefs[0].Metadata
	if metadata["nantian.dev/backend-ref-valid"] != "false" {
		t.Fatalf("expected backend ref to become invalid, got metadata %#v", metadata)
	}
	if metadata["nantian.dev/backend-ref-reason"] != string(gatewayv1.RouteReasonRefNotPermitted) {
		t.Fatalf("expected ref-not-permitted reason, got metadata %#v", metadata)
	}
}

func TestReconcileQueuesSettleRun(t *testing.T) {
	scheme := runtime.NewScheme()
	mustAddToScheme(t, scheme, gatewayv1.Install)
	mustAddToScheme(t, scheme, gatewayv1alpha2.Install)
	mustAddToScheme(t, scheme, gatewayv1beta1.Install)
	mustAddToScheme(t, scheme, corev1.AddToScheme)
	mustAddToScheme(t, scheme, discoveryv1.AddToScheme)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	cl := newControllerClientBuilder(scheme).Build()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	syncer := NewSyncer(
		cl,
		translator.New(string(controllerName), logger),
		store,
		testMetrics(),
		time.Minute,
		logger,
	)
	syncer.SetSettleDelay(20 * time.Millisecond)
	defer syncer.stopSettleRun()
	settled := make(chan struct{}, 1)
	syncer.settleRun = func(context.Context) {
		select {
		case settled <- struct{}{}:
		default:
		}
	}

	if _, err := syncer.Reconcile(context.Background(), ctrl.Request{}); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	select {
	case <-settled:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected settle run to fire after reconcile")
	}
}

func TestReconcileServiceParentGRPCRouteScopedRequestPublishesImmediatelyWithSettleDelay(t *testing.T) {
	scheme := runtime.NewScheme()
	mustAddToScheme(t, scheme, gatewayv1.Install)
	mustAddToScheme(t, scheme, gatewayv1alpha2.Install)
	mustAddToScheme(t, scheme, gatewayv1beta1.Install)
	mustAddToScheme(t, scheme, corev1.AddToScheme)
	mustAddToScheme(t, scheme, discoveryv1.AddToScheme)

	servicePort := gatewayv1.PortNumber(8080)
	cl := newControllerClientBuilder(scheme).
		WithObjects(
			&gatewayv1.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default"},
				Spec: gatewayv1.GRPCRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Kind: ptr[gatewayv1.Kind]("Service"),
							Name: "echo",
						}},
					},
					Rules: []gatewayv1.GRPCRouteRule{{
						BackendRefs: []gatewayv1.GRPCBackendRef{{
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
						Name:       "grpc",
						Port:       8080,
						TargetPort: intstr.FromInt(8080),
						Protocol:   corev1.ProtocolTCP,
					}},
				},
			},
		).
		Build()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	triggered := make(chan struct{}, 1)
	var observedScopes []ReconcilerRunnerScope
	syncer := NewSyncer(
		cl,
		translator.New("gateway.networking.k8s.io/nantian-gw", logger),
		store,
		testMetrics(),
		time.Minute,
		logger,
		func(scopes ...ReconcilerRunnerScope) {
			observedScopes = append([]ReconcilerRunnerScope(nil), scopes...)
			select {
			case triggered <- struct{}{}:
			default:
			}
		},
	)
	syncer.SetSettleDelay(50 * time.Millisecond)
	defer syncer.stopSettleRun()

	if _, err := syncer.Reconcile(
		context.Background(),
		snapshotGRPCRoutesReconcileRequestForKey(client.ObjectKey{
			Namespace: "default",
			Name:      "route",
		}),
	); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	current := store.Current()
	if current == nil {
		t.Fatal("expected service-parent GRPCRoute request to publish immediately")
	}
	if len(current.GRPCRoutes) != 1 {
		t.Fatalf("expected immediate publish to include grpc route, got %#v", current.GRPCRoutes)
	}
	if len(current.Listeners) != 1 {
		t.Fatalf("expected immediate publish to include mesh listener, got %#v", current.Listeners)
	}

	select {
	case <-triggered:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected immediate publish to queue leader run")
	}
	wantScopes := []ReconcilerRunnerScope{
		ReconcilerRunnerScopeInfra,
		ReconcilerRunnerScopeGatewayStatus,
		ReconcilerRunnerScopeRouteStatus,
		ReconcilerRunnerScopePolicyStatus,
	}
	if !sameRunnerScopes(observedScopes, wantScopes) {
		t.Fatalf("leader run scopes = %v, want %v", observedScopes, wantScopes)
	}
}

func TestReconcileDeletedServiceParentGRPCRouteScopedRequestPublishesImmediatelyWithSettleDelay(t *testing.T) {
	scheme := runtime.NewScheme()
	mustAddToScheme(t, scheme, gatewayv1.Install)
	mustAddToScheme(t, scheme, gatewayv1alpha2.Install)
	mustAddToScheme(t, scheme, gatewayv1beta1.Install)
	mustAddToScheme(t, scheme, corev1.AddToScheme)
	mustAddToScheme(t, scheme, discoveryv1.AddToScheme)

	servicePort := gatewayv1.PortNumber(8080)
	route := &gatewayv1.GRPCRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default"},
		Spec: gatewayv1.GRPCRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{{
					Kind: ptr[gatewayv1.Kind]("Service"),
					Name: "echo",
				}},
			},
			Rules: []gatewayv1.GRPCRouteRule{{
				BackendRefs: []gatewayv1.GRPCBackendRef{{
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
	cl := newControllerClientBuilder(scheme).
		WithObjects(
			route,
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "grpc",
						Port:       8080,
						TargetPort: intstr.FromInt(8080),
						Protocol:   corev1.ProtocolTCP,
					}},
				},
			},
		).
		Build()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	syncer := NewSyncer(
		cl,
		translator.New("gateway.networking.k8s.io/nantian-gw", logger),
		store,
		testMetrics(),
		time.Minute,
		logger,
	)
	syncer.SetSettleDelay(0)
	if _, err := syncer.Reconcile(context.Background(), ctrl.Request{}); err != nil {
		t.Fatalf("initial Reconcile returned error: %v", err)
	}

	current := store.Current()
	if current == nil || len(current.GRPCRoutes) != 1 || len(current.Listeners) != 1 {
		t.Fatalf("expected initial mesh snapshot, got %#v", current)
	}

	if err := cl.Delete(context.Background(), route); err != nil {
		t.Fatalf("delete route: %v", err)
	}

	syncer.SetSettleDelay(50 * time.Millisecond)
	defer syncer.stopSettleRun()

	if _, err := syncer.Reconcile(
		context.Background(),
		snapshotGRPCRoutesReconcileRequestForKey(client.ObjectKey{
			Namespace: "default",
			Name:      "route",
		}),
	); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	current = store.Current()
	if current == nil {
		t.Fatal("expected deleted service-parent GRPCRoute request to publish immediately")
	}
	if len(current.GRPCRoutes) != 0 {
		t.Fatalf("expected immediate publish to remove deleted grpc route, got %#v", current.GRPCRoutes)
	}
	if len(current.Listeners) != 0 {
		t.Fatalf("expected immediate publish to remove deleted mesh listener, got %#v", current.Listeners)
	}
}

func TestReconcileSettleRunUsesLifecycleContext(t *testing.T) {
	scheme := runtime.NewScheme()
	mustAddToScheme(t, scheme, gatewayv1.Install)
	mustAddToScheme(t, scheme, gatewayv1alpha2.Install)
	mustAddToScheme(t, scheme, gatewayv1beta1.Install)
	mustAddToScheme(t, scheme, corev1.AddToScheme)
	mustAddToScheme(t, scheme, discoveryv1.AddToScheme)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	cl := newControllerClientBuilder(scheme).Build()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	syncer := NewSyncer(
		cl,
		translator.New(string(controllerName), logger),
		store,
		testMetrics(),
		time.Minute,
		logger,
	)
	syncer.SetSettleDelay(20 * time.Millisecond)
	defer syncer.stopSettleRun()

	type settleContextKey struct{}
	lifecycleCtx := context.WithValue(context.Background(), settleContextKey{}, "lifecycle")
	reconcileCtx := context.WithValue(context.Background(), settleContextKey{}, "reconcile")
	syncer.lifecycleCtx = lifecycleCtx

	values := make(chan any, 1)
	syncer.settleRun = func(ctx context.Context) {
		select {
		case values <- ctx.Value(settleContextKey{}):
		default:
		}
	}

	if _, err := syncer.Reconcile(reconcileCtx, ctrl.Request{}); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	select {
	case got := <-values:
		if got != "lifecycle" {
			t.Fatalf("expected lifecycle context, got %#v", got)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected settle run to fire after reconcile")
	}
}

func TestNewSyncerPublishesImmediatelyByDefault(t *testing.T) {
	scheme := runtime.NewScheme()
	mustAddToScheme(t, scheme, gatewayv1.Install)
	mustAddToScheme(t, scheme, gatewayv1alpha2.Install)
	mustAddToScheme(t, scheme, gatewayv1beta1.Install)
	mustAddToScheme(t, scheme, corev1.AddToScheme)
	mustAddToScheme(t, scheme, discoveryv1.AddToScheme)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	cl := newControllerClientBuilder(scheme).Build()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	syncer := NewSyncer(
		cl,
		translator.New(string(controllerName), logger),
		store,
		testMetrics(),
		time.Minute,
		logger,
	)

	if syncer.settleDelay != 0 {
		t.Fatalf("expected default settle delay to be disabled, got %v", syncer.settleDelay)
	}
}

func TestReconcileImmediatePublishQueuesLeaderRun(t *testing.T) {
	scheme := runtime.NewScheme()
	mustAddToScheme(t, scheme, gatewayv1.Install)
	mustAddToScheme(t, scheme, gatewayv1alpha2.Install)
	mustAddToScheme(t, scheme, gatewayv1beta1.Install)
	mustAddToScheme(t, scheme, corev1.AddToScheme)
	mustAddToScheme(t, scheme, discoveryv1.AddToScheme)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	cl := newControllerClientBuilder(scheme).Build()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	triggered := make(chan struct{}, 1)
	syncer := NewSyncer(
		cl,
		translator.New(string(controllerName), logger),
		store,
		testMetrics(),
		time.Minute,
		logger,
		func(...ReconcilerRunnerScope) {
			select {
			case triggered <- struct{}{}:
			default:
			}
		},
	)

	if _, err := syncer.Reconcile(context.Background(), ctrl.Request{}); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	select {
	case <-triggered:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected immediate reconcile to queue leader run")
	}
}

func TestReconcileQueuesLeaderRunAfterPublishingSnapshot(t *testing.T) {
	scheme := runtime.NewScheme()
	mustAddToScheme(t, scheme, gatewayv1.Install)
	mustAddToScheme(t, scheme, gatewayv1alpha2.Install)
	mustAddToScheme(t, scheme, gatewayv1beta1.Install)
	mustAddToScheme(t, scheme, corev1.AddToScheme)
	mustAddToScheme(t, scheme, discoveryv1.AddToScheme)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	cl := newControllerClientBuilder(scheme).Build()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)

	var observedVersion string
	syncer := NewSyncer(
		cl,
		translator.New(string(controllerName), logger),
		store,
		testMetrics(),
		time.Minute,
		logger,
		func(...ReconcilerRunnerScope) {
			snapshot := store.Current()
			if snapshot != nil {
				observedVersion = snapshot.ID
			}
		},
	)

	if _, err := syncer.Reconcile(context.Background(), ctrl.Request{}); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	current := store.Current()
	if current == nil || current.ID == "" {
		t.Fatalf("expected current snapshot to be published, got %#v", current)
	}
	if observedVersion == "" {
		t.Fatal("expected leader run to observe the published snapshot version")
	}
	if observedVersion != current.ID {
		t.Fatalf("leader run observed snapshot %q, want %q", observedVersion, current.ID)
	}
}

func TestSyncerRunSkipsTickerBuildWhenNoRetryPending(t *testing.T) {
	syncer, retryClient, store := newPeriodicRetryTestSyncer(t, 20*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		syncer.Run(ctx)
		close(done)
	}()

	waitForPublishedSnapshot(t, store)
	initialGatewayClassLists := retryClient.listCount(typeName(&gatewayv1.GatewayClassList{}))

	time.Sleep(80 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for syncer run loop to stop")
	}

	if got := retryClient.listCount(typeName(&gatewayv1.GatewayClassList{})); got != initialGatewayClassLists {
		t.Fatalf("expected no extra full rebuilds while retry queue is empty, got gateway class lists %d -> %d", initialGatewayClassLists, got)
	}
}

func TestSyncerRunRetriesStartupFailureFromPendingScope(t *testing.T) {
	syncer, retryClient, store := newPeriodicRetryTestSyncer(t, 20*time.Millisecond)
	retryClient.failNextList(typeName(&gatewayv1.GatewayClassList{}))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		syncer.Run(ctx)
		close(done)
	}()

	waitForPublishedSnapshot(t, store)
	recoveredGatewayClassLists := retryClient.listCount(typeName(&gatewayv1.GatewayClassList{}))
	time.Sleep(80 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for syncer run loop to stop")
	}

	if recoveredGatewayClassLists < 2 {
		t.Fatalf("expected startup failure plus retry to rebuild gateway classes at least twice, got %d", recoveredGatewayClassLists)
	}
	if got := retryClient.listCount(typeName(&gatewayv1.GatewayClassList{})); got != recoveredGatewayClassLists {
		t.Fatalf("expected ticker to stop rebuilding after retry recovery, got gateway class lists %d -> %d", recoveredGatewayClassLists, got)
	}
}

func TestSyncerReconcileFailureQueuesScopedRetryWithoutFullRebuild(t *testing.T) {
	syncer, retryClient, store := newPeriodicRetryTestSyncer(t, time.Minute)

	if _, err := syncer.Reconcile(context.Background(), ctrl.Request{}); err != nil {
		t.Fatalf("initial Reconcile returned error: %v", err)
	}
	initialSnapshot := store.Current()
	if initialSnapshot == nil || initialSnapshot.ID == "" {
		t.Fatalf("expected initial snapshot, got %#v", initialSnapshot)
	}

	retryClient.failNextGet(typeName(&gatewayv1.HTTPRoute{}))
	if _, err := syncer.Reconcile(
		context.Background(),
		snapshotHTTPRoutesReconcileRequestForKey(client.ObjectKey{Namespace: "default", Name: "echo"}),
	); err != nil {
		t.Fatalf("route-scoped Reconcile returned error: %v", err)
	}

	scope, attachmentNamespaces, backendNamespaces, gatewayKeys, serviceKeys, serviceImportKeys, routeKeys := syncer.consumeRetryPendingBuild()
	if scope != snapshotBuildScopeRoutes {
		t.Fatalf("expected route-scoped retry pending build, got scope %v", scope)
	}
	if len(routeKeys.http) != 1 || routeKeys.http[0] != (client.ObjectKey{Namespace: "default", Name: "echo"}) {
		t.Fatalf("expected retry pending build to keep the scoped http route key, got %#v", routeKeys.http)
	}
	syncer.mergeRetryPendingBuild(scope, attachmentNamespaces, backendNamespaces, gatewayKeys, serviceKeys, serviceImportKeys, routeKeys)

	initialHTTPRouteLists := retryClient.listCount(typeName(&gatewayv1.HTTPRouteList{}))
	initialRouteGets := retryClient.getCount(typeName(&gatewayv1.HTTPRoute{}))
	if ran := syncer.runRetryBuild(context.Background()); !ran {
		t.Fatal("expected retry build to run")
	}

	if got := retryClient.listCount(typeName(&gatewayv1.HTTPRouteList{})); got != initialHTTPRouteLists {
		t.Fatalf("expected scoped retry to avoid full route relist, got http route lists %d -> %d", initialHTTPRouteLists, got)
	}
	if got := retryClient.getCount(typeName(&gatewayv1.HTTPRoute{})); got <= initialRouteGets {
		t.Fatalf("expected scoped retry to reload the http route, got http route gets %d -> %d", initialRouteGets, got)
	}
	if ran := syncer.runRetryBuild(context.Background()); ran {
		t.Fatal("expected retry queue to be empty after successful scoped retry")
	}
	currentSnapshot := store.Current()
	if currentSnapshot == nil || currentSnapshot.ID != initialSnapshot.ID {
		t.Fatalf("expected scoped retry with unchanged output to keep snapshot version %q, got %#v", initialSnapshot.ID, currentSnapshot)
	}
}

func mustAddToScheme(t *testing.T, scheme *runtime.Scheme, add func(*runtime.Scheme) error) {
	t.Helper()
	if err := add(scheme); err != nil {
		t.Fatalf("add to scheme: %v", err)
	}
}

func testMetrics() *observability.Metrics {
	return &observability.Metrics{
		BuildsTotal: prometheus.NewCounter(prometheus.CounterOpts{Name: "test_builds_total", Help: "test"}),
		BuildFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "test_build_failures_total", Help: "test",
		}),
		PublishedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "test_published_total", Help: "test",
		}),
		LastBuildSuccess: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "test_last_build_success", Help: "test",
		}),
		SnapshotBuildDurationSeconds: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "test_snapshot_build_duration_seconds", Help: "test",
		}),
		SnapshotResourceCount: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "test_snapshot_resource_count", Help: "test",
			},
			[]string{"resource"},
		),
		SnapshotListenerAttachedRoutes: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "test_snapshot_listener_attached_routes", Help: "test",
		}),
	}
}

func ptr[T any](value T) *T {
	return &value
}

func snapshotHistogramSampleCount(t *testing.T, histogram prometheus.Histogram) uint64 {
	t.Helper()

	metric, ok := histogram.(prometheus.Metric)
	if !ok {
		t.Fatal("histogram does not implement prometheus.Metric")
	}

	dtoMetric := &dto.Metric{}
	if err := metric.Write(dtoMetric); err != nil {
		t.Fatalf("write histogram metric: %v", err)
	}
	return dtoMetric.GetHistogram().GetSampleCount()
}

func snapshotHistogramSampleSum(t *testing.T, histogram prometheus.Histogram) float64 {
	t.Helper()

	metric, ok := histogram.(prometheus.Metric)
	if !ok {
		t.Fatal("histogram does not implement prometheus.Metric")
	}

	dtoMetric := &dto.Metric{}
	if err := metric.Write(dtoMetric); err != nil {
		t.Fatalf("write histogram metric: %v", err)
	}
	return dtoMetric.GetHistogram().GetSampleSum()
}

func snapshotHistogramVecSampleCount(
	t *testing.T,
	histogram *prometheus.HistogramVec,
	labelValues ...string,
) uint64 {
	t.Helper()

	observer, err := histogram.GetMetricWithLabelValues(labelValues...)
	if err != nil {
		t.Fatalf("get histogram metric: %v", err)
	}

	metric, ok := observer.(prometheus.Metric)
	if !ok {
		t.Fatal("histogram observer does not implement prometheus.Metric")
	}

	dtoMetric := &dto.Metric{}
	if err := metric.Write(dtoMetric); err != nil {
		t.Fatalf("write histogram metric: %v", err)
	}
	return dtoMetric.GetHistogram().GetSampleCount()
}

func snapshotHistogramVecSampleSum(
	t *testing.T,
	histogram *prometheus.HistogramVec,
	labelValues ...string,
) float64 {
	t.Helper()

	observer, err := histogram.GetMetricWithLabelValues(labelValues...)
	if err != nil {
		t.Fatalf("get histogram metric: %v", err)
	}

	metric, ok := observer.(prometheus.Metric)
	if !ok {
		t.Fatal("histogram observer does not implement prometheus.Metric")
	}

	dtoMetric := &dto.Metric{}
	if err := metric.Write(dtoMetric); err != nil {
		t.Fatalf("write histogram metric: %v", err)
	}
	return dtoMetric.GetHistogram().GetSampleSum()
}

type snapshotRetryTestClient struct {
	client.Client
	mu         sync.Mutex
	listCounts map[string]int
	getCounts  map[string]int
	failLists  map[string]int
	failGets   map[string]int
}

func (c *snapshotRetryTestClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	key := typeName(list)

	c.mu.Lock()
	c.listCounts[key]++
	if c.failLists[key] > 0 {
		c.failLists[key]--
		c.mu.Unlock()
		return fmt.Errorf("forced list failure for %s", key)
	}
	c.mu.Unlock()

	return c.Client.List(ctx, list, opts...)
}

func (c *snapshotRetryTestClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	typeKey := typeName(obj)

	c.mu.Lock()
	c.getCounts[typeKey]++
	if c.failGets[typeKey] > 0 {
		c.failGets[typeKey]--
		c.mu.Unlock()
		return fmt.Errorf("forced get failure for %s", typeKey)
	}
	c.mu.Unlock()

	return c.Client.Get(ctx, key, obj, opts...)
}

func (c *snapshotRetryTestClient) failNextList(typeKey string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failLists[typeKey]++
}

func (c *snapshotRetryTestClient) failNextGet(typeKey string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failGets[typeKey]++
}

func (c *snapshotRetryTestClient) listCount(typeKey string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.listCounts[typeKey]
}

func (c *snapshotRetryTestClient) getCount(typeKey string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.getCounts[typeKey]
}

func newPeriodicRetryTestSyncer(t *testing.T, interval time.Duration) (*Syncer, *snapshotRetryTestClient, *ir.SnapshotStore) {
	t.Helper()

	scheme := runtime.NewScheme()
	mustAddToScheme(t, scheme, gatewayv1.Install)
	mustAddToScheme(t, scheme, gatewayv1alpha2.Install)
	mustAddToScheme(t, scheme, gatewayv1beta1.Install)
	mustAddToScheme(t, scheme, corev1.AddToScheme)
	mustAddToScheme(t, scheme, discoveryv1.AddToScheme)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	hostname := gatewayv1.Hostname("example.com")
	pathType := gatewayv1.PathMatchPathPrefix
	portNumber := gatewayv1.PortNumber(8080)

	baseClient := newControllerClientBuilder(scheme).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "default"},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{
						{
							Name:     "http",
							Protocol: gatewayv1.HTTPProtocolType,
							Port:     80,
							Hostname: &hostname,
						},
					},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{
							{
								Name:        "edge",
								SectionName: ptr[gatewayv1.SectionName]("http"),
							},
						},
					},
					Hostnames: []gatewayv1.Hostname{hostname},
					Rules: []gatewayv1.HTTPRouteRule{
						{
							Matches: []gatewayv1.HTTPRouteMatch{
								{
									Path: &gatewayv1.HTTPPathMatch{
										Type:  &pathType,
										Value: ptr("/"),
									},
								},
							},
							BackendRefs: []gatewayv1.HTTPBackendRef{
								{
									BackendRef: gatewayv1.BackendRef{
										BackendObjectReference: gatewayv1.BackendObjectReference{
											Name: "echo",
											Port: &portNumber,
										},
									},
								},
							},
						},
					},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Port:       8080,
							TargetPort: intstr.FromInt(8080),
							Protocol:   corev1.ProtocolTCP,
						},
					},
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
				Ports: []discoveryv1.EndpointPort{
					{Port: ptr[int32](8080)},
				},
				Endpoints: []discoveryv1.Endpoint{
					{Addresses: []string{"10.0.0.10"}},
				},
			},
		).
		Build()

	retryClient := &snapshotRetryTestClient{
		Client:     baseClient,
		listCounts: make(map[string]int),
		getCounts:  make(map[string]int),
		failLists:  make(map[string]int),
		failGets:   make(map[string]int),
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	syncer := NewSyncer(
		retryClient,
		translator.New(string(controllerName), logger),
		store,
		testMetrics(),
		interval,
		logger,
	)
	syncer.SetSettleDelay(0)

	return syncer, retryClient, store
}

func waitForPublishedSnapshot(t *testing.T, store *ir.SnapshotStore) *ir.Snapshot {
	t.Helper()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if snapshot := store.Current(); snapshot != nil && snapshot.ID != "" {
			return snapshot
		}
		time.Sleep(10 * time.Millisecond)
	}

	snapshot := store.Current()
	t.Fatalf("timed out waiting for published snapshot, last snapshot %#v", snapshot)
	return nil
}

func typeName(value any) string {
	return fmt.Sprintf("%T", value)
}
