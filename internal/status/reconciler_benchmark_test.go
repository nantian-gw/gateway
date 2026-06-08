package status

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/gatewayapi"
	backendlbv1alpha2 "github.com/nantian-gw/gateway/internal/gatewayapiexperimental/backendlbv1alpha2"
)

func BenchmarkReconcileFullStatusRouteFanout(b *testing.B) {
	for _, routeCount := range []int{50, 200} {
		b.Run(fmt.Sprintf("routes_%d", routeCount), func(b *testing.B) {
			b.ReportAllocs()
			ctx := context.Background()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				reconciler := newStatusBenchmarkReconciler(b, routeCount, true)
				b.StartTimer()

				if err := reconciler.Reconcile(ctx); err != nil {
					b.Fatalf("Reconcile returned error: %v", err)
				}
			}
		})
	}
}

func BenchmarkReconcileFullStatusAttachDetachStorm(b *testing.B) {
	for _, routeCount := range []int{50, 200} {
		b.Run(fmt.Sprintf("routes_%d", routeCount), func(b *testing.B) {
			b.ReportAllocs()
			ctx := context.Background()
			fixture := newStatusStormBenchmarkFixture(b, routeCount)

			if err := fixture.reconciler.Reconcile(ctx); err != nil {
				b.Fatalf("initial Reconcile returned error: %v", err)
			}

			attached := false
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				if err := fixture.setRoutesAttached(ctx, attached); err != nil {
					b.Fatalf("setRoutesAttached returned error: %v", err)
				}
				b.StartTimer()

				if err := fixture.reconciler.Reconcile(ctx); err != nil {
					b.Fatalf("storm Reconcile returned error: %v", err)
				}
				attached = !attached
			}
		})
	}
}

func BenchmarkReconcileFullStatusGatewayConvergenceFleet(b *testing.B) {
	for _, gatewayCount := range []int{50, 200} {
		b.Run(fmt.Sprintf("gateways_%d", gatewayCount), func(b *testing.B) {
			b.ReportAllocs()
			ctx := context.Background()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				reconciler := newGatewayConvergenceBenchmarkReconciler(b, gatewayCount)
				b.StartTimer()

				if err := reconciler.Reconcile(ctx); err != nil {
					b.Fatalf("Reconcile returned error: %v", err)
				}
			}
		})
	}
}

func BenchmarkReconcileFullStatusGatewayConvergenceFleetReaderRefresh(b *testing.B) {
	for _, gatewayCount := range []int{50, 200} {
		b.Run(fmt.Sprintf("gateways_%d", gatewayCount), func(b *testing.B) {
			b.ReportAllocs()
			ctx := context.Background()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				staleClient, freshReader := newGatewayConvergenceReaderBenchmarkClients(b, gatewayCount)
				reconciler := NewWithAddressesAndReader(
					staleClient,
					freshReader,
					"gateway.networking.k8s.io/nantian-gw",
					[]string{"127.0.0.1"},
					discardLogger(),
				)
				b.StartTimer()

				if err := reconciler.Reconcile(ctx); err != nil {
					b.Fatalf("Reconcile returned error: %v", err)
				}
			}
		})
	}
}

func BenchmarkLoadStateBackendPolicyFanout(b *testing.B) {
	for _, policyCount := range []int{50, 200} {
		b.Run(fmt.Sprintf("policies_%d", policyCount), func(b *testing.B) {
			b.ReportAllocs()
			ctx := context.Background()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				reconciler := newBackendPolicyStatusBenchmarkReconciler(b, policyCount)
				b.StartTimer()

				state, err := reconciler.loadState(ctx)
				if err != nil {
					b.Fatalf("loadState returned error: %v", err)
				}
				if len(state.backendLBPolicies) != policyCount {
					b.Fatalf("expected %d BackendLBPolicies, got %d", policyCount, len(state.backendLBPolicies))
				}
				if len(state.backendTLSPolicies) != policyCount {
					b.Fatalf("expected %d BackendTLSPolicies, got %d", policyCount, len(state.backendTLSPolicies))
				}
			}
		})
	}
}

type statusStormBenchmarkFixture struct {
	client      client.Client
	reconciler  *Reconciler
	routeKeys   []client.ObjectKey
	gatewayName string
}

func newStatusStormBenchmarkFixture(b *testing.B, routeCount int) *statusStormBenchmarkFixture {
	b.Helper()

	const gatewayName = "gw"
	cl := newStatusBenchmarkClient(b, routeCount, true)
	return &statusStormBenchmarkFixture{
		client:      cl,
		reconciler:  New(cl, "gateway.networking.k8s.io/nantian-gw", "127.0.0.1", discardLogger()),
		routeKeys:   benchmarkRouteKeys(routeCount),
		gatewayName: gatewayName,
	}
}

func (f *statusStormBenchmarkFixture) setRoutesAttached(ctx context.Context, attached bool) error {
	parentName := "gw-detached"
	if attached {
		parentName = f.gatewayName
	}

	for _, key := range f.routeKeys {
		var route gatewayv1.HTTPRoute
		if err := f.client.Get(ctx, key, &route); err != nil {
			return err
		}

		route.Generation++
		route.Spec.CommonRouteSpec.ParentRefs = []gatewayv1.ParentReference{{
			Name: gatewayv1.ObjectName(parentName),
		}}
		if err := f.client.Update(ctx, &route); err != nil {
			return err
		}
	}

	return nil
}

func newStatusBenchmarkReconciler(b *testing.B, routeCount int, attached bool) *Reconciler {
	b.Helper()

	cl := newStatusBenchmarkClient(b, routeCount, attached)
	return New(cl, "gateway.networking.k8s.io/nantian-gw", "127.0.0.1", discardLogger())
}

func newGatewayConvergenceBenchmarkReconciler(b *testing.B, gatewayCount int) *Reconciler {
	b.Helper()

	cl := newGatewayConvergenceBenchmarkClient(b, gatewayCount)
	return New(cl, "gateway.networking.k8s.io/nantian-gw", "127.0.0.1", discardLogger())
}

func newBackendPolicyStatusBenchmarkReconciler(b *testing.B, policyCount int) *Reconciler {
	b.Helper()

	cl := newBackendPolicyStatusBenchmarkClient(b, policyCount)
	return New(cl, "gateway.networking.k8s.io/nantian-gw", "127.0.0.1", discardLogger())
}

func newStatusBenchmarkClient(b *testing.B, routeCount int, attached bool) client.Client {
	b.Helper()

	scheme := newStatusBenchmarkScheme(b)
	objects := statusBenchmarkObjects(routeCount, attached)

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
			&gatewayv1.HTTPRoute{},
		).
		WithObjects(objects...).
		Build()
}

func newGatewayConvergenceBenchmarkClient(b *testing.B, gatewayCount int) client.Client {
	b.Helper()

	scheme := newStatusBenchmarkScheme(b)
	objects := gatewayConvergenceBenchmarkObjects(gatewayCount)

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(objects...).
		Build()
}

func newGatewayConvergenceReaderBenchmarkClients(b *testing.B, gatewayCount int) (client.Client, client.Reader) {
	b.Helper()

	scheme := newStatusBenchmarkScheme(b)
	staleObjects, freshObjects := gatewayConvergenceReaderBenchmarkObjects(gatewayCount)

	staleClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(staleObjects...).
		Build()

	freshReader := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(freshObjects...).
		Build()

	return staleClient, freshReader
}

func newBackendPolicyStatusBenchmarkClient(b *testing.B, policyCount int) client.Client {
	b.Helper()

	scheme := newStatusBenchmarkScheme(b)
	objects := backendPolicyBenchmarkObjects(b, policyCount)

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gatewayv1.GatewayClass{}, statusGatewayClassControllerNameIndex, func(object client.Object) []string {
			gatewayClass, ok := object.(*gatewayv1.GatewayClass)
			if !ok || gatewayClass.Spec.ControllerName == "" {
				return nil
			}
			return []string{string(gatewayClass.Spec.ControllerName)}
		}).
		WithIndex(&gatewayv1.Gateway{}, statusGatewayGatewayClassNameIndex, func(object client.Object) []string {
			gateway, ok := object.(*gatewayv1.Gateway)
			if !ok || gateway.Spec.GatewayClassName == "" {
				return nil
			}
			return []string{string(gateway.Spec.GatewayClassName)}
		}).
		WithIndex(&gatewayv1.HTTPRoute{}, statusHTTPRouteGatewayParentIndex, statusHTTPRouteGatewayParentIndexKeys).
		WithIndex(&gatewayv1.HTTPRoute{}, statusHTTPRouteServiceParentIndex, statusHTTPRouteServiceParentIndexKeys).
		WithIndex(&gatewayv1.HTTPRoute{}, statusHTTPRouteListenerSetParentIndex, statusHTTPRouteListenerSetParentIndexKeys).
		WithIndex(gatewayapi.NewBackendTLSPolicyV1Object(), statusBackendTLSPolicyTargetRefIndex, statusBackendTLSPolicyTargetRefIndexKeys).
		WithIndex(&backendlbv1alpha2.BackendLBPolicy{}, statusBackendLBPolicyTargetRefIndex, statusBackendLBPolicyTargetRefIndexKeys).
		WithObjects(objects...).
		Build()
}

func statusBenchmarkObjects(routeCount int, attached bool) []client.Object {
	objects := []client.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
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
		gatewayInfrastructureService("default", "gw"),
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{Port: 8080}},
			},
		},
	}

	parentName := "gw-detached"
	if attached {
		parentName = "gw"
	}

	for i := 0; i < routeCount; i++ {
		objects = append(objects, &gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:       benchmarkRouteName(i),
				Namespace:  "default",
				Generation: 1,
			},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{
						Name: gatewayv1.ObjectName(parentName),
					}},
				},
				Rules: []gatewayv1.HTTPRouteRule{{
					BackendRefs: []gatewayv1.HTTPBackendRef{{
						BackendRef: gatewayv1.BackendRef{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Name: "echo",
								Port: portPtr(8080),
							},
						},
					}},
				}},
			},
		})
	}

	return objects
}

func backendPolicyBenchmarkObjects(b *testing.B, policyCount int) []client.Object {
	b.Helper()

	caBundle := gatewayv1.WellKnownCACertificatesSystem
	objects := []client.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
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
	}

	for i := 0; i < policyCount; i++ {
		serviceName := "echo-" + strconv.Itoa(i)
		lbType := backendlbv1alpha2.LoadBalancingStrategyTypeRoundRobin
		objects = append(
			objects,
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8080}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:       benchmarkRouteName(i),
					Namespace:  "default",
					Generation: 1,
				},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{Name: "gw"}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: gatewayv1.ObjectName(serviceName),
									Port: portPtr(8080),
								},
							},
						}},
					}},
				},
			},
			backendPolicyBenchmarkTLSPolicy(b, serviceName+"-tls", serviceName, caBundle),
			&backendlbv1alpha2.BackendLBPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName + "-lb", Namespace: "default"},
				Spec: backendlbv1alpha2.BackendLBPolicySpec{
					TargetRefs: []backendlbv1alpha2.LocalPolicyTargetReference{{
						Kind: "Service",
						Name: gatewayv1.ObjectName(serviceName),
					}},
					LoadBalancing: &backendlbv1alpha2.LoadBalancingPolicy{
						Type: &lbType,
					},
				},
			},
		)
	}

	return objects
}

func backendPolicyBenchmarkTLSPolicy(
	b *testing.B,
	policyName string,
	serviceName string,
	caBundle gatewayv1.WellKnownCACertificatesType,
) client.Object {
	b.Helper()

	raw, err := gatewayapi.EncodeBackendTLSPolicyV1(&gatewayv1alpha3.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: policyName, Namespace: "default"},
		Spec: gatewayv1.BackendTLSPolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
				LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
					Kind: "Service",
					Name: gatewayv1.ObjectName(serviceName),
				},
			}},
			Validation: gatewayv1.BackendTLSPolicyValidation{
				Hostname:                gatewayv1.PreciseHostname(serviceName + ".default.svc.cluster.local"),
				WellKnownCACertificates: &caBundle,
			},
		},
	})
	if err != nil {
		b.Fatalf("EncodeBackendTLSPolicyV1 returned error: %v", err)
	}
	return raw
}

func gatewayConvergenceBenchmarkObjects(gatewayCount int) []client.Object {
	objects := []client.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw"),
			},
		},
	}

	for i := 0; i < gatewayCount; i++ {
		gatewayName := benchmarkGatewayName(i)
		gateway := gatewayWithNameAndGenerationForConvergenceTest(gatewayName, 1)

		switch i % 4 {
		case 0:
			gateway.Spec.Listeners[0].Protocol = gatewayv1.ProtocolType("SMTP")
			objects = append(objects, gateway)
		case 1:
			objects = append(objects, gateway)
		case 2:
			service, slice := benchmarkGatewayInfrastructureObjects(*gateway)
			objects = append(objects, gateway, service, slice)
		default:
			setCondition(&gateway.Status.Conditions, conditionSpec{
				Type:               string(gatewayv1.GatewayConditionProgrammed),
				Status:             metav1.ConditionTrue,
				Reason:             string(gatewayv1.GatewayReasonProgrammed),
				Message:            "Gateway is programmed",
				ObservedGeneration: gateway.Generation,
			})
			service, slice := benchmarkGatewayInfrastructureObjects(*gateway)
			objects = append(objects, gateway, service, slice)
		}
	}

	return objects
}

func gatewayConvergenceReaderBenchmarkObjects(gatewayCount int) ([]client.Object, []client.Object) {
	staleObjects := []client.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 2},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw"),
			},
		},
	}
	freshObjects := []client.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 2},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw"),
			},
		},
	}

	for i := 0; i < gatewayCount; i++ {
		gatewayName := benchmarkGatewayName(i)
		gateway := gatewayWithNameAndGenerationForConvergenceTest(gatewayName, 2)

		switch i % 4 {
		case 0:
			gateway.Spec.Listeners[0].Protocol = gatewayv1.ProtocolType("SMTP")
			staleObjects = append(staleObjects, gateway.DeepCopy())
			freshObjects = append(freshObjects, gateway.DeepCopy())
		case 1:
			staleObjects = append(staleObjects, gateway.DeepCopy())
			freshObjects = append(freshObjects, gateway.DeepCopy())
		case 2:
			staleObjects = append(
				staleObjects,
				gateway.DeepCopy(),
				gatewayInfrastructureService(gateway.Namespace, gateway.Name),
			)
			freshService, freshSlice := benchmarkGatewayInfrastructureObjects(*gateway)
			freshObjects = append(
				freshObjects,
				gateway.DeepCopy(),
				freshService,
				freshSlice,
			)
		default:
			benchmarkMarkGatewayFullyConverged(gateway)
			service, slice := benchmarkGatewayInfrastructureObjects(*gateway)
			staleObjects = append(staleObjects, gateway.DeepCopy(), service.DeepCopy(), slice.DeepCopy())
			freshObjects = append(freshObjects, gateway.DeepCopy(), service.DeepCopy(), slice.DeepCopy())
		}
	}

	return staleObjects, freshObjects
}

func benchmarkMarkGatewayFullyConverged(gateway *gatewayv1.Gateway) {
	setCondition(&gateway.Status.Conditions, conditionSpec{
		Type:               string(gatewayv1.GatewayConditionAccepted),
		Status:             metav1.ConditionTrue,
		Reason:             string(gatewayv1.GatewayReasonAccepted),
		Message:            "Gateway is accepted by nantian-gw",
		ObservedGeneration: gateway.Generation,
	})
	setCondition(&gateway.Status.Conditions, conditionSpec{
		Type:               string(gatewayv1.GatewayConditionProgrammed),
		Status:             metav1.ConditionTrue,
		Reason:             string(gatewayv1.GatewayReasonProgrammed),
		Message:            "Gateway is programmed",
		ObservedGeneration: gateway.Generation,
	})
	gateway.Status.Listeners = []gatewayv1.ListenerStatus{{
		Name:           gateway.Spec.Listeners[0].Name,
		SupportedKinds: []gatewayv1.RouteGroupKind{{Group: ptr(gatewayv1.Group(gatewayv1.GroupName)), Kind: gatewayv1.Kind("HTTPRoute")}},
		Conditions: []metav1.Condition{
			{
				Type:               string(gatewayv1.ListenerConditionAccepted),
				Status:             metav1.ConditionTrue,
				Reason:             string(gatewayv1.ListenerReasonAccepted),
				Message:            "Listener is accepted by nantian-gw",
				ObservedGeneration: gateway.Generation,
			},
			{
				Type:               string(gatewayv1.ListenerConditionResolvedRefs),
				Status:             metav1.ConditionTrue,
				Reason:             string(gatewayv1.ListenerReasonResolvedRefs),
				Message:            "Listener references are resolved",
				ObservedGeneration: gateway.Generation,
			},
			{
				Type:               string(gatewayv1.ListenerConditionProgrammed),
				Status:             metav1.ConditionTrue,
				Reason:             string(gatewayv1.ListenerReasonProgrammed),
				Message:            "Listener is programmed",
				ObservedGeneration: gateway.Generation,
			},
		},
	}}
}

func benchmarkRouteKeys(routeCount int) []client.ObjectKey {
	keys := make([]client.ObjectKey, 0, routeCount)
	for i := 0; i < routeCount; i++ {
		keys = append(keys, client.ObjectKey{
			Namespace: "default",
			Name:      benchmarkRouteName(i),
		})
	}
	return keys
}

func benchmarkRouteName(index int) string {
	return "bench-route-" + strconv.Itoa(index)
}

func benchmarkGatewayName(index int) string {
	return "bench-gateway-" + strconv.Itoa(index)
}

func benchmarkGatewayInfrastructureObjects(
	gateway gatewayv1.Gateway,
) (*corev1.Service, *discoveryv1.EndpointSlice) {
	service := gatewayInfrastructureServiceForGateway(gateway)
	slice := gatewayInfrastructureEndpointSlice(
		gateway.Namespace,
		gateway.Name,
		"gateway-frontend-endpoints",
	)
	slice.Annotations = benchmarkCloneStringMap(service.Annotations)
	return service, slice
}

func benchmarkCloneStringMap(items map[string]string) map[string]string {
	if len(items) == 0 {
		return nil
	}

	out := make(map[string]string, len(items))
	for key, value := range items {
		out[key] = value
	}
	return out
}

func newStatusBenchmarkScheme(b *testing.B) *runtime.Scheme {
	b.Helper()

	scheme := runtime.NewScheme()
	benchmarkMustAddToScheme(b, scheme, corev1.AddToScheme)
	benchmarkMustAddToScheme(b, scheme, discoveryv1.AddToScheme)
	benchmarkMustAddToScheme(b, scheme, apiextensionsv1.AddToScheme)
	benchmarkMustAddToScheme(b, scheme, gatewayv1.Install)
	benchmarkMustAddToScheme(b, scheme, gatewayv1alpha2.Install)
	benchmarkMustAddToScheme(b, scheme, backendlbv1alpha2.Install)
	benchmarkMustAddToScheme(b, scheme, gatewayv1alpha3.Install)
	benchmarkMustAddToScheme(b, scheme, gatewayv1beta1.Install)
	benchmarkMustAddToScheme(b, scheme, mcsv1alpha1.AddToScheme)
	return scheme
}

func benchmarkMustAddToScheme(b *testing.B, scheme *runtime.Scheme, add func(*runtime.Scheme) error) {
	b.Helper()
	if err := add(scheme); err != nil {
		b.Fatalf("add to scheme: %v", err)
	}
}
