package infrastructure

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func BenchmarkReconcileMeshServicesRouteFanout(b *testing.B) {
	for _, routeCount := range []int{50, 200} {
		b.Run(fmt.Sprintf("routes_%d", routeCount), func(b *testing.B) {
			b.ReportAllocs()
			ctx := context.Background()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				reconciler := newMeshServiceBenchmarkReconciler(b, routeCount, true)
				b.StartTimer()

				if err := reconciler.reconcileMeshServices(ctx); err != nil {
					b.Fatalf("reconcileMeshServices returned error: %v", err)
				}
			}
		})
	}
}

func BenchmarkReconcileMeshServicesAttachDetachStorm(b *testing.B) {
	for _, routeCount := range []int{50, 200} {
		b.Run(fmt.Sprintf("routes_%d", routeCount), func(b *testing.B) {
			b.ReportAllocs()
			ctx := context.Background()
			fixture := newMeshServiceStormBenchmarkFixture(b, routeCount)

			if err := fixture.reconciler.reconcileMeshServices(ctx); err != nil {
				b.Fatalf("initial reconcileMeshServices returned error: %v", err)
			}

			attached := false
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				if err := fixture.setRoutesAttached(ctx, attached); err != nil {
					b.Fatalf("setRoutesAttached returned error: %v", err)
				}
				b.StartTimer()

				if err := fixture.reconciler.reconcileMeshServices(ctx); err != nil {
					b.Fatalf("storm reconcileMeshServices returned error: %v", err)
				}
				attached = !attached
			}
		})
	}
}

type meshServiceStormBenchmarkFixture struct {
	client     client.Client
	reconciler *Reconciler
	routeKeys  []client.ObjectKey
}

func newMeshServiceStormBenchmarkFixture(b *testing.B, routeCount int) *meshServiceStormBenchmarkFixture {
	b.Helper()

	reconciler, cl := newMeshServiceBenchmarkReconcilerWithClient(b, routeCount, true)
	return &meshServiceStormBenchmarkFixture{
		client:     cl,
		reconciler: reconciler,
		routeKeys:  meshServiceBenchmarkRouteKeys(routeCount),
	}
}

func (f *meshServiceStormBenchmarkFixture) setRoutesAttached(ctx context.Context, attached bool) error {
	serviceKind := gatewayv1.Kind("Service")
	servicePort := gatewayv1.PortNumber(80)

	for idx, key := range f.routeKeys {
		var route gatewayv1.HTTPRoute
		if err := f.client.Get(ctx, key, &route); err != nil {
			return err
		}

		route.Generation++
		if attached {
			route.Spec.ParentRefs = []gatewayv1.ParentReference{{
				Kind: &serviceKind,
				Name: gatewayv1.ObjectName(meshServiceBenchmarkServiceName(idx)),
				Port: &servicePort,
			}}
		} else {
			route.Spec.ParentRefs = nil
		}
		if err := f.client.Update(ctx, &route); err != nil {
			return err
		}
	}

	if !attached {
		return nil
	}

	for i := range f.routeKeys {
		if err := ensureMeshServiceBenchmarkSourceState(
			ctx,
			f.client,
			"default",
			meshServiceBenchmarkServiceName(i),
		); err != nil {
			return err
		}
	}
	return nil
}

func newMeshServiceBenchmarkReconciler(b *testing.B, routeCount int, attached bool) *Reconciler {
	b.Helper()

	reconciler, _ := newMeshServiceBenchmarkReconcilerWithClient(b, routeCount, attached)
	return reconciler
}

func newMeshServiceBenchmarkReconcilerWithClient(
	b *testing.B,
	routeCount int,
	attached bool,
) (*Reconciler, client.Client) {
	b.Helper()

	cl := withInfrastructureRouteParentIndexes(
		fake.NewClientBuilder().
			WithScheme(newInfrastructureBenchmarkScheme(b)).
			WithObjects(meshServiceBenchmarkObjects(routeCount, attached)...),
	).Build()

	return New(cl, "gateway.networking.k8s.io/nantian-gw", discardLogger()), cl
}

func meshServiceBenchmarkObjects(routeCount int, attached bool) []client.Object {
	objects := []client.Object{
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nantian-dataplane-0",
				Namespace: defaultDataplaneNamespace,
				Labels:    map[string]string{"app": "nantian-gw-dataplane"},
			},
			Status: corev1.PodStatus{
				PodIP: "10.0.0.50",
				Conditions: []corev1.PodCondition{{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				}},
			},
		},
	}

	serviceKind := gatewayv1.Kind("Service")
	servicePort := gatewayv1.PortNumber(80)
	httpProtocol := "http"

	for i := 0; i < routeCount; i++ {
		serviceName := meshServiceBenchmarkServiceName(i)
		route := &gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      meshServiceBenchmarkRouteName(i),
				Namespace: "default",
			},
			Spec: gatewayv1.HTTPRouteSpec{},
		}
		if attached {
			route.Spec.ParentRefs = []gatewayv1.ParentReference{{
				Kind: &serviceKind,
				Name: gatewayv1.ObjectName(serviceName),
				Port: &servicePort,
			}}
		}

		objects = append(objects,
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": serviceName},
					Ports: []corev1.ServicePort{{
						Name:        "http",
						Port:        80,
						TargetPort:  intstr.FromInt(8080),
						Protocol:    corev1.ProtocolTCP,
						AppProtocol: &httpProtocol,
					}},
				},
			},
			benchmarkSourceEndpoints("default", serviceName),
			benchmarkSourceEndpointSlice("default", serviceName),
			route,
		)
	}

	return objects
}

func meshServiceBenchmarkRouteKeys(routeCount int) []client.ObjectKey {
	keys := make([]client.ObjectKey, 0, routeCount)
	for i := 0; i < routeCount; i++ {
		keys = append(keys, client.ObjectKey{
			Namespace: "default",
			Name:      meshServiceBenchmarkRouteName(i),
		})
	}
	return keys
}

func meshServiceBenchmarkServiceName(index int) string {
	return "mesh-bench-svc-" + strconv.Itoa(index)
}

func meshServiceBenchmarkRouteName(index int) string {
	return "mesh-bench-route-" + strconv.Itoa(index)
}

func ensureMeshServiceBenchmarkSourceState(
	ctx context.Context,
	cl client.Client,
	namespace string,
	serviceName string,
) error {
	if err := recreateMeshServiceBenchmarkObject(ctx, cl, benchmarkSourceEndpoints(namespace, serviceName)); err != nil {
		return err
	}
	return recreateMeshServiceBenchmarkObject(ctx, cl, benchmarkSourceEndpointSlice(namespace, serviceName))
}

func recreateMeshServiceBenchmarkObject[T client.Object](ctx context.Context, cl client.Client, desired T) error {
	current := desired.DeepCopyObject().(T)
	if err := cl.Get(ctx, client.ObjectKeyFromObject(desired), current); client.IgnoreNotFound(err) != nil {
		return err
	}
	if current.GetName() != "" {
		if err := cl.Delete(ctx, current); client.IgnoreNotFound(err) != nil {
			return err
		}
	}
	return cl.Create(ctx, desired)
}

//nolint:staticcheck // SA1019: deprecated API used correctly for backward compatibility
func benchmarkSourceEndpoints(namespace string, serviceName string) *corev1.Endpoints {
	//nolint:staticcheck // SA1019: deprecated API used correctly for backward compatibility
	return &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
		},
		//nolint:staticcheck // SA1019: deprecated API used correctly for backward compatibility
		Subsets: []corev1.EndpointSubset{{
			Addresses: []corev1.EndpointAddress{{
				IP: "10.0.0.10",
			}},
			Ports: []corev1.EndpointPort{{
				Name: "http",
				Port: 8080,
			}},
		}},
	}
}

func benchmarkSourceEndpointSlice(namespace string, serviceName string) *discoveryv1.EndpointSlice {
	portName := "http"
	portNumber := int32(8080)
	protocol := corev1.ProtocolTCP
	return &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName + "-source",
			Namespace: namespace,
			Labels: map[string]string{
				discoveryv1.LabelServiceName: serviceName,
			},
		},
		AddressType: discoveryv1.AddressTypeIPv4,
		Endpoints: []discoveryv1.Endpoint{{
			Addresses: []string{"10.0.0.10"},
		}},
		Ports: []discoveryv1.EndpointPort{{
			Name:     &portName,
			Port:     &portNumber,
			Protocol: &protocol,
		}},
	}
}
