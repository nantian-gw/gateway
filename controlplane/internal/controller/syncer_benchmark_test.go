package controller

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	backendlbv1alpha2 "github.com/nantian-gw/gateway/controlplane/internal/gatewayapiexperimental/backendlbv1alpha2"
	"github.com/nantian-gw/gateway/controlplane/internal/ir"
	"github.com/nantian-gw/gateway/controlplane/internal/translator"
)

func BenchmarkPublishSnapshotRouteFanout(b *testing.B) {
	for _, routeCount := range []int{50, 200} {
		b.Run(fmt.Sprintf("routes_%d", routeCount), func(b *testing.B) {
			b.ReportAllocs()
			ctx := context.Background()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				syncer := newSnapshotBenchmarkSyncer(b, routeCount, true)
				b.StartTimer()

				published, err := syncer.publishSnapshot(ctx)
				if err != nil {
					b.Fatalf("publishSnapshot returned error: %v", err)
				}
				if !published {
					b.Fatal("expected publishSnapshot to publish a snapshot")
				}
			}
		})
	}
}

func BenchmarkPublishSnapshotAttachDetachStorm(b *testing.B) {
	for _, routeCount := range []int{50, 200} {
		b.Run(fmt.Sprintf("routes_%d", routeCount), func(b *testing.B) {
			b.ReportAllocs()
			ctx := context.Background()
			fixture := newSnapshotStormBenchmarkFixture(b, routeCount)

			published, err := fixture.syncer.publishSnapshot(ctx)
			if err != nil {
				b.Fatalf("initial publishSnapshot returned error: %v", err)
			}
			if !published {
				b.Fatal("expected initial publishSnapshot to publish a snapshot")
			}
			attached := false

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				if err := fixture.setRoutesAttached(ctx, attached); err != nil {
					b.Fatalf("setRoutesAttached returned error: %v", err)
				}
				b.StartTimer()

				published, err := fixture.syncer.publishSnapshot(ctx)
				if err != nil {
					b.Fatalf("storm publishSnapshot returned error: %v", err)
				}
				if !published {
					b.Fatal("expected storm publishSnapshot to publish a snapshot")
				}
				attached = !attached
			}
		})
	}
}

func BenchmarkSnapshotInputStatusStorm(b *testing.B) {
	for _, routeCount := range []int{50, 200} {
		b.Run(fmt.Sprintf("routes_%d", routeCount), func(b *testing.B) {
			b.ReportAllocs()
			predicate := snapshotInputMutationPredicate()
			events := snapshotStatusStormEvents(routeCount)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				filtered := 0
				for _, updateEvent := range events {
					if predicate.Update(updateEvent) {
						filtered++
					}
				}
				if filtered != 0 {
					b.Fatalf("expected status-only storm updates to be filtered, got %d", filtered)
				}
			}
		})
	}
}

func BenchmarkSnapshotInputIrrelevantAnnotationStorm(b *testing.B) {
	for _, routeCount := range []int{50, 200} {
		b.Run(fmt.Sprintf("routes_%d", routeCount), func(b *testing.B) {
			b.ReportAllocs()
			predicate := snapshotInputMutationPredicate()
			events := snapshotIrrelevantAnnotationStormEvents(routeCount)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				filtered := 0
				for _, updateEvent := range events {
					if predicate.Update(updateEvent) {
						filtered++
					}
				}
				if filtered != 0 {
					b.Fatalf("expected irrelevant annotation-only updates to be filtered, got %d", filtered)
				}
			}
		})
	}
}

func BenchmarkReconcileEndpointSliceBackendStorm(b *testing.B) {
	for _, routeCount := range []int{50, 200} {
		b.Run(fmt.Sprintf("routes_%d", routeCount), func(b *testing.B) {
			b.ReportAllocs()
			ctx := context.Background()
			fixture := newSnapshotBackendStormBenchmarkFixture(b, routeCount)

			published, err := fixture.syncer.publishSnapshot(ctx)
			if err != nil {
				b.Fatalf("initial publishSnapshot returned error: %v", err)
			}
			if !published {
				b.Fatal("expected initial publishSnapshot to publish a snapshot")
			}
			suffix := 11

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				address := fmt.Sprintf("10.0.0.%d", suffix)

				b.StopTimer()
				if err := fixture.setEndpointAddress(ctx, address); err != nil {
					b.Fatalf("setEndpointAddress returned error: %v", err)
				}
				b.StartTimer()

				if _, err := fixture.syncer.Reconcile(
					ctx,
					snapshotBackendsReconcileRequestForService(client.ObjectKey{
						Namespace: "default",
						Name:      "echo",
					}),
				); err != nil {
					b.Fatalf("backend storm Reconcile returned error: %v", err)
				}

				current := fixture.syncer.store.Current()
				if current == nil || len(current.Backends) != 1 {
					b.Fatalf("expected 1 backend after endpoint storm reconcile, got %#v", current)
				}
				if got := current.Backends[0].Endpoints[0].Address; got != address {
					b.Fatalf("backend endpoint address = %q, want %q", got, address)
				}
				suffix++
				if suffix > 250 {
					suffix = 11
				}
			}
		})
	}
}

func BenchmarkSnapshotReconcileRequestsNamespaceSelectorStorm(b *testing.B) {
	for _, gatewayCount := range []int{25, 100} {
		b.Run(fmt.Sprintf("gateways_%d", gatewayCount), func(b *testing.B) {
			b.ReportAllocs()
			ctx := context.Background()
			syncer := newNamespaceSelectorStormBenchmarkSyncer(b, gatewayCount)
			namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "apps-0"}}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				requests := syncer.snapshotReconcileRequests(ctx, namespace)
				if len(requests) != 1 || requests[0] != snapshotAttachmentsReconcileRequest("apps-0") {
					b.Fatalf("snapshotReconcileRequests() = %#v, want attachment rebuild for apps-0", requests)
				}
			}
		})
	}
}

type snapshotStormBenchmarkFixture struct {
	client    client.Client
	syncer    *Syncer
	routeKeys []client.ObjectKey
}

type snapshotBackendStormBenchmarkFixture struct {
	client   client.Client
	syncer   *Syncer
	sliceKey client.ObjectKey
}

func newNamespaceSelectorStormBenchmarkSyncer(b *testing.B, gatewayCount int) *Syncer {
	b.Helper()

	scheme := newSnapshotBenchmarkScheme(b)
	cl := newControllerClientBuilder(scheme).
		WithObjects(namespaceSelectorStormBenchmarkObjects(gatewayCount)...).
		WithIndex(&gatewayv1.Gateway{}, gatewayNamespaceSelectorIndex, gatewayNamespaceSelectorIndexKeys).
		Build()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewSyncer(
		cl,
		translator.New("gateway.networking.k8s.io/nantian-gw", logger),
		ir.NewSnapshotStore(logger),
		testMetrics(),
		0,
		logger,
	)
}

func newSnapshotStormBenchmarkFixture(b *testing.B, routeCount int) *snapshotStormBenchmarkFixture {
	b.Helper()

	syncer, cl := newSnapshotBenchmarkSyncerWithClient(b, routeCount, true)
	return &snapshotStormBenchmarkFixture{
		client:    cl,
		syncer:    syncer,
		routeKeys: snapshotBenchmarkRouteKeys(routeCount),
	}
}

func newSnapshotBackendStormBenchmarkFixture(b *testing.B, routeCount int) *snapshotBackendStormBenchmarkFixture {
	b.Helper()

	syncer, cl := newSnapshotBenchmarkSyncerWithClient(b, routeCount, true)
	return &snapshotBackendStormBenchmarkFixture{
		client: cl,
		syncer: syncer,
		sliceKey: client.ObjectKey{
			Namespace: "default",
			Name:      "echo-1",
		},
	}
}

func (f *snapshotStormBenchmarkFixture) setRoutesAttached(ctx context.Context, attached bool) error {
	parentName := gatewayv1.ObjectName("gw-detached")
	if attached {
		parentName = "gw"
	}

	for _, key := range f.routeKeys {
		var route gatewayv1.HTTPRoute
		if err := f.client.Get(ctx, key, &route); err != nil {
			return err
		}

		route.Generation++
		route.Spec.CommonRouteSpec.ParentRefs = []gatewayv1.ParentReference{{
			Name: parentName,
		}}
		if err := f.client.Update(ctx, &route); err != nil {
			return err
		}
	}

	return nil
}

func (f *snapshotBackendStormBenchmarkFixture) setEndpointAddress(ctx context.Context, address string) error {
	var slice discoveryv1.EndpointSlice
	if err := f.client.Get(ctx, f.sliceKey, &slice); err != nil {
		return err
	}

	slice.Endpoints = []discoveryv1.Endpoint{{
		Addresses: []string{address},
	}}
	return f.client.Update(ctx, &slice)
}

func newSnapshotBenchmarkSyncer(b *testing.B, routeCount int, attached bool) *Syncer {
	b.Helper()

	syncer, _ := newSnapshotBenchmarkSyncerWithClient(b, routeCount, attached)
	return syncer
}

func newSnapshotBenchmarkSyncerWithClient(b *testing.B, routeCount int, attached bool) (*Syncer, client.Client) {
	b.Helper()

	scheme := newSnapshotBenchmarkScheme(b)
	cl := newControllerClientBuilder(scheme).
		WithObjects(snapshotBenchmarkObjects(routeCount, attached)...).
		Build()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewSyncer(
		cl,
		translator.New("gateway.networking.k8s.io/nantian-gw", logger),
		ir.NewSnapshotStore(logger),
		testMetrics(),
		0,
		logger,
	), cl
}

func namespaceSelectorStormBenchmarkObjects(gatewayCount int) []client.Object {
	fromSelector := gatewayv1.NamespacesFromSelector
	objects := []client.Object{
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw"),
			},
		},
	}

	for i := 0; i < gatewayCount; i++ {
		gatewayName := fmt.Sprintf("edge-%d", i)
		routeNamespace := fmt.Sprintf("apps-%d", i)

		objects = append(
			objects,
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: gatewayName, Namespace: "infra", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
						AllowedRoutes: &gatewayv1.AllowedRoutes{
							Namespaces: &gatewayv1.RouteNamespaces{From: &fromSelector},
						},
					}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:       snapshotBenchmarkRouteName(i),
					Namespace:  routeNamespace,
					Generation: 1,
				},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Name:      gatewayv1.ObjectName(gatewayName),
							Namespace: ptr[gatewayv1.Namespace]("infra"),
						}},
					},
				},
			},
		)
	}

	return objects
}

func snapshotBenchmarkObjects(routeCount int, attached bool) []client.Object {
	objects := []client.Object{
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw"),
			},
		},
		&gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
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

	parentName := gatewayv1.ObjectName("gw-detached")
	if attached {
		parentName = "gw"
	}

	for i := 0; i < routeCount; i++ {
		objects = append(objects, &gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:       snapshotBenchmarkRouteName(i),
				Namespace:  "default",
				Generation: 1,
			},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{
						Name: parentName,
					}},
				},
				Rules: []gatewayv1.HTTPRouteRule{{
					BackendRefs: []gatewayv1.HTTPBackendRef{{
						BackendRef: gatewayv1.BackendRef{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Name: "echo",
								Port: ptr[gatewayv1.PortNumber](8080),
							},
						},
					}},
				}},
			},
		})
	}

	return objects
}

func snapshotStatusStormEvents(routeCount int) []event.UpdateEvent {
	events := make([]event.UpdateEvent, 0, routeCount)
	for i := 0; i < routeCount; i++ {
		oldRoute := &gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:       snapshotBenchmarkRouteName(i),
				Namespace:  "default",
				Generation: 1,
			},
		}
		newRoute := oldRoute.DeepCopy()
		newRoute.Status.RouteStatus.Parents = []gatewayv1.RouteParentStatus{{
			ControllerName: "gateway.networking.k8s.io/nantian-gw",
		}}
		events = append(events, event.UpdateEvent{
			ObjectOld: oldRoute,
			ObjectNew: newRoute,
		})
	}
	return events
}

func snapshotIrrelevantAnnotationStormEvents(routeCount int) []event.UpdateEvent {
	events := make([]event.UpdateEvent, 0, routeCount)
	for i := 0; i < routeCount; i++ {
		oldRoute := &gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:       snapshotBenchmarkRouteName(i),
				Namespace:  "default",
				Generation: 1,
			},
		}
		newRoute := oldRoute.DeepCopy()
		newRoute.Annotations = map[string]string{
			"example.com/trace": strconv.Itoa(i),
		}
		events = append(events, event.UpdateEvent{
			ObjectOld: oldRoute,
			ObjectNew: newRoute,
		})
	}
	return events
}

func snapshotBenchmarkRouteKeys(routeCount int) []client.ObjectKey {
	keys := make([]client.ObjectKey, 0, routeCount)
	for i := 0; i < routeCount; i++ {
		keys = append(keys, client.ObjectKey{
			Namespace: "default",
			Name:      snapshotBenchmarkRouteName(i),
		})
	}
	return keys
}

func snapshotBenchmarkRouteName(index int) string {
	return "snapshot-bench-route-" + strconv.Itoa(index)
}

func newSnapshotBenchmarkScheme(b *testing.B) *runtime.Scheme {
	b.Helper()

	scheme := runtime.NewScheme()
	snapshotBenchmarkMustAddToScheme(b, scheme, corev1.AddToScheme)
	snapshotBenchmarkMustAddToScheme(b, scheme, discoveryv1.AddToScheme)
	snapshotBenchmarkMustAddToScheme(b, scheme, gatewayv1.Install)
	snapshotBenchmarkMustAddToScheme(b, scheme, gatewayv1alpha2.Install)
	snapshotBenchmarkMustAddToScheme(b, scheme, gatewayv1alpha3.Install)
	snapshotBenchmarkMustAddToScheme(b, scheme, gatewayv1beta1.Install)
	snapshotBenchmarkMustAddToScheme(b, scheme, backendlbv1alpha2.Install)
	snapshotBenchmarkMustAddToScheme(b, scheme, mcsv1alpha1.AddToScheme)
	return scheme
}

func snapshotBenchmarkMustAddToScheme(b *testing.B, scheme *runtime.Scheme, add func(*runtime.Scheme) error) {
	b.Helper()
	if err := add(scheme); err != nil {
		b.Fatalf("add to scheme: %v", err)
	}
}
