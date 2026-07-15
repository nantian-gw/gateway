package infrastructure

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	backendlb "github.com/nantian-gw/gateway/internal/gatewayexp/backendlb"
	"github.com/nantian-gw/gateway/internal/mesh"
)

func BenchmarkInspectInfrastructureRouteFanout(b *testing.B) {
	for _, routeCount := range []int{50, 200} {
		b.Run(fmt.Sprintf("routes_%d", routeCount), func(b *testing.B) {
			b.ReportAllocs()
			ctx := context.Background()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				reconciler := newInfrastructureBenchmarkReconciler(b, routeCount)
				b.StartTimer()

				report, err := reconciler.Inspect(ctx)
				if err != nil {
					b.Fatalf("Inspect returned error: %v", err)
				}
				if report.Summary.ResourceCount == 0 {
					b.Fatal("expected derived infrastructure records")
				}
			}
		})
	}
}

func BenchmarkInspectInfrastructureAttachDetachStorm(b *testing.B) {
	for _, routeCount := range []int{50, 200} {
		b.Run(fmt.Sprintf("routes_%d", routeCount), func(b *testing.B) {
			b.ReportAllocs()
			ctx := context.Background()
			fixture := newInfrastructureAttachDetachStormFixture(b, routeCount)
			attached := false

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				if err := fixture.setRoutesAttached(ctx, attached); err != nil {
					b.Fatalf("setRoutesAttached returned error: %v", err)
				}
				b.StartTimer()

				report, err := fixture.reconciler.Inspect(ctx)
				if err != nil {
					b.Fatalf("Inspect returned error: %v", err)
				}
				wantResources := infrastructureExpectedResourceCount(routeCount, attached)
				if report.Summary.ResourceCount != wantResources {
					b.Fatalf("resource count = %d, want %d", report.Summary.ResourceCount, wantResources)
				}
				attached = !attached
			}
		})
	}
}

func TestInfrastructureBenchmarkReconcilerRegistersManagedGatewayIndexes(t *testing.T) {
	t.Parallel()

	reconciler := newInfrastructureBenchmarkReconciler(t, 1)
	gateways, err := reconciler.loadManagedGateways(context.Background())
	if err != nil {
		t.Fatalf("loadManagedGateways returned error: %v", err)
	}
	if len(gateways) != 1 {
		t.Fatalf("expected 1 managed gateway, got %d", len(gateways))
	}
	if gateways[0].Namespace != "default" || gateways[0].Name != "public" {
		t.Fatalf("unexpected managed gateway %s/%s", gateways[0].Namespace, gateways[0].Name)
	}
}

type infrastructureAttachDetachStormFixture struct {
	client     client.Client
	reconciler *Reconciler
	routeKeys  []client.ObjectKey
}

func newInfrastructureBenchmarkReconciler(tb testing.TB, routeCount int) *Reconciler {
	tb.Helper()

	reconciler, _ := newInfrastructureBenchmarkReconcilerWithClient(tb, routeCount)
	return reconciler
}

func newInfrastructureBenchmarkReconcilerWithClient(tb testing.TB, routeCount int) (*Reconciler, client.Client) {
	tb.Helper()

	k8sClient := withInfrastructureGatewayIndexes(
		withInfrastructureRouteParentIndexes(
			fake.NewClientBuilder().
				WithScheme(newInfrastructureBenchmarkScheme(tb)).
				WithObjects(infrastructureBenchmarkObjects(routeCount)...),
		),
	).Build()

	return New(k8sClient, "gateway.networking.k8s.io/nantian-gw", discardLogger()), k8sClient
}

func newInfrastructureAttachDetachStormFixture(tb testing.TB, routeCount int) *infrastructureAttachDetachStormFixture {
	tb.Helper()

	reconciler, k8sClient := newInfrastructureBenchmarkReconcilerWithClient(tb, routeCount)
	return &infrastructureAttachDetachStormFixture{
		client:     k8sClient,
		reconciler: reconciler,
		routeKeys:  infrastructureBenchmarkRouteKeys(routeCount),
	}
}

func infrastructureBenchmarkObjects(routeCount int) []client.Object {
	objects := []client.Object{
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw"),
			},
		},
		&gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "public",
				Namespace: "default",
			},
			Spec: gatewayv1.GatewaySpec{
				GatewayClassName: "nantian-gw",
				Listeners: []gatewayv1.Listener{{
					Name:     "http",
					Protocol: gatewayv1.HTTPProtocolType,
					Port:     80,
				}},
			},
		},
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
		serviceName := infrastructureBenchmarkServiceName(i)
		routeName := infrastructureBenchmarkRouteName(i)

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
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      routeName,
					Namespace: "default",
				},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Kind: &serviceKind,
							Name: gatewayv1.ObjectName(serviceName),
							Port: &servicePort,
						}},
					},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mesh.ShadowServiceName("default", serviceName),
					Namespace: "default",
					Labels: map[string]string{
						managedByLabel:                     managedByValue,
						mesh.ShadowServiceRoleLabel:        mesh.ShadowServiceRoleValue,
						mesh.OriginalServiceNamespaceLabel: "default",
						mesh.OriginalServiceNameLabel:      serviceName,
					},
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeClusterIP,
				},
			},
		)
	}

	return objects
}

func infrastructureBenchmarkServiceName(index int) string {
	return "inspect-bench-svc-" + strconv.Itoa(index)
}

func infrastructureBenchmarkRouteName(index int) string {
	return "inspect-bench-route-" + strconv.Itoa(index)
}

func infrastructureBenchmarkRouteKeys(routeCount int) []client.ObjectKey {
	keys := make([]client.ObjectKey, 0, routeCount)
	for i := 0; i < routeCount; i++ {
		keys = append(keys, client.ObjectKey{
			Namespace: "default",
			Name:      infrastructureBenchmarkRouteName(i),
		})
	}
	return keys
}

func (f *infrastructureAttachDetachStormFixture) setRoutesAttached(ctx context.Context, attached bool) error {
	serviceKind := gatewayv1.Kind("Service")
	servicePort := gatewayv1.PortNumber(80)

	for idx, key := range f.routeKeys {
		var route gatewayv1.HTTPRoute
		if err := f.client.Get(ctx, key, &route); err != nil {
			return err
		}

		route.Generation++
		if attached {
			route.Spec.CommonRouteSpec.ParentRefs = []gatewayv1.ParentReference{{
				Kind: &serviceKind,
				Name: gatewayv1.ObjectName(infrastructureBenchmarkServiceName(idx)),
				Port: &servicePort,
			}}
		} else {
			route.Spec.CommonRouteSpec.ParentRefs = nil
		}
		if err := f.client.Update(ctx, &route); err != nil {
			return err
		}
	}

	return nil
}

func infrastructureExpectedResourceCount(routeCount int, attached bool) int {
	const baseResources = 4
	if attached {
		return baseResources + (routeCount * 3)
	}
	return baseResources + routeCount
}

func newInfrastructureBenchmarkScheme(tb testing.TB) *runtime.Scheme {
	tb.Helper()

	scheme := runtime.NewScheme()
	infrastructureBenchmarkMustAddToScheme(tb, scheme, corev1.AddToScheme)
	infrastructureBenchmarkMustAddToScheme(tb, scheme, discoveryv1.AddToScheme)
	infrastructureBenchmarkMustAddToScheme(tb, scheme, gatewayv1.Install)
	infrastructureBenchmarkMustAddToScheme(tb, scheme, gatewayv1alpha2.Install)
	infrastructureBenchmarkMustAddToScheme(tb, scheme, gatewayv1alpha3.Install)
	infrastructureBenchmarkMustAddToScheme(tb, scheme, gatewayv1beta1.Install)
	infrastructureBenchmarkMustAddToScheme(tb, scheme, backendlb.Install)
	infrastructureBenchmarkMustAddToScheme(tb, scheme, mcsv1alpha1.AddToScheme)
	return scheme
}

func infrastructureBenchmarkMustAddToScheme(tb testing.TB, scheme *runtime.Scheme, add func(*runtime.Scheme) error) {
	tb.Helper()
	if err := add(scheme); err != nil {
		tb.Fatalf("add to scheme: %v", err)
	}
}
