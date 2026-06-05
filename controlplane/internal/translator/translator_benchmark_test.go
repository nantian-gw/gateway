package translator

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
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/nantian-gw/gateway/controlplane/internal/ir"
)

func BenchmarkBuildSnapshotRouteFanout(b *testing.B) {
	for _, routeCount := range []int{50, 200} {
		b.Run(fmt.Sprintf("routes_%d", routeCount), func(b *testing.B) {
			b.ReportAllocs()
			ctx := context.Background()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				xlator, k8sClient := newTranslatorBenchmarkFixture(b, routeCount)
				b.StartTimer()

				snapshot, err := xlator.Build(ctx, k8sClient)
				if err != nil {
					b.Fatalf("Build returned error: %v", err)
				}
				if len(snapshot.HTTPRoutes) != routeCount {
					b.Fatalf("expected %d http routes, got %d", routeCount, len(snapshot.HTTPRoutes))
				}
				if len(snapshot.Backends) != routeCount {
					b.Fatalf("expected %d backends, got %d", routeCount, len(snapshot.Backends))
				}
			}
		})
	}
}

func BenchmarkBuildSnapshotAttachDetachStorm(b *testing.B) {
	for _, routeCount := range []int{50, 200} {
		b.Run(fmt.Sprintf("routes_%d", routeCount), func(b *testing.B) {
			b.ReportAllocs()
			ctx := context.Background()
			fixture := newTranslatorAttachDetachStormFixture(b, routeCount)
			attached := false

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				if err := fixture.setRoutesAttached(ctx, attached); err != nil {
					b.Fatalf("setRoutesAttached returned error: %v", err)
				}
				b.StartTimer()

				snapshot, err := fixture.translator.Build(ctx, fixture.client)
				if err != nil {
					b.Fatalf("Build returned error: %v", err)
				}
				wantAttached := 0
				if attached {
					wantAttached = routeCount
				}
				if got := totalAttachedRoutes(snapshot); got != wantAttached {
					b.Fatalf("attached route count = %d, want %d", got, wantAttached)
				}
				attached = !attached
			}
		})
	}
}

func BenchmarkBuildBackendsForSnapshotEndpointSliceStorm(b *testing.B) {
	for _, routeCount := range []int{50, 200} {
		b.Run(fmt.Sprintf("routes_%d", routeCount), func(b *testing.B) {
			b.ReportAllocs()
			ctx := context.Background()
			fixture := newTranslatorBackendStormFixture(b, routeCount)

			current, err := fixture.translator.Build(ctx, fixture.client)
			if err != nil {
				b.Fatalf("initial Build returned error: %v", err)
			}
			suffix := 11

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				address := fmt.Sprintf("10.1.0.%d", suffix)

				b.StopTimer()
				if err := fixture.setEndpointAddress(ctx, address); err != nil {
					b.Fatalf("setEndpointAddress returned error: %v", err)
				}
				b.StartTimer()

				backends, err := fixture.translator.BuildBackendsForSnapshot(
					ctx,
					fixture.client,
					current,
					[]client.ObjectKey{{Namespace: "default", Name: "echo"}},
					nil,
				)
				if err != nil {
					b.Fatalf("BuildBackendsForSnapshot returned error: %v", err)
				}
				if len(backends) != 1 {
					b.Fatalf("expected 1 backend, got %d", len(backends))
				}
				if got := backends[0].Endpoints[0].Address; got != address {
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

type translatorBackendStormFixture struct {
	translator *Translator
	client     client.Client
	sliceKey   client.ObjectKey
}

type translatorAttachDetachStormFixture struct {
	translator *Translator
	client     client.Client
	routeKeys  []client.ObjectKey
}

func newTranslatorBenchmarkFixture(b *testing.B, routeCount int) (*Translator, client.Client) {
	b.Helper()

	scheme := newTranslatorBenchmarkScheme(b)
	k8sClient := newTranslatorClientBuilder(scheme).
		WithObjects(translatorBenchmarkObjects(routeCount)...).
		Build()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New("gateway.networking.k8s.io/nantian-gw", logger), k8sClient
}

func newTranslatorBackendStormFixture(b *testing.B, routeCount int) *translatorBackendStormFixture {
	b.Helper()

	scheme := newTranslatorBenchmarkScheme(b)
	k8sClient := newTranslatorClientBuilder(scheme).
		WithObjects(translatorBackendStormObjects(routeCount)...).
		Build()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &translatorBackendStormFixture{
		translator: New("gateway.networking.k8s.io/nantian-gw", logger),
		client:     k8sClient,
		sliceKey: client.ObjectKey{
			Namespace: "default",
			Name:      "echo-1",
		},
	}
}

func newTranslatorAttachDetachStormFixture(b *testing.B, routeCount int) *translatorAttachDetachStormFixture {
	b.Helper()

	xlator, k8sClient := newTranslatorBenchmarkFixture(b, routeCount)
	return &translatorAttachDetachStormFixture{
		translator: xlator,
		client:     k8sClient,
		routeKeys:  translatorBenchmarkRouteKeys(routeCount),
	}
}

func translatorBenchmarkObjects(routeCount int) []client.Object {
	objects := []client.Object{
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
	}

	for i := 0; i < routeCount; i++ {
		serviceName := translatorBenchmarkServiceName(i)
		routeName := translatorBenchmarkRouteName(i)
		port := gatewayv1.PortNumber(8080)

		objects = append(objects,
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      routeName,
					Namespace: "default",
				},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Name: "gw",
						}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: gatewayv1.ObjectName(serviceName),
									Port: &port,
								},
							},
						}},
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: "default",
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
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName + "-1",
					Namespace: "default",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: serviceName,
					},
				},
				Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](8080)}},
				Endpoints: []discoveryv1.Endpoint{{
					Addresses: []string{fmt.Sprintf("10.0.%d.%d", i/250, (i%250)+1)},
				}},
			},
		)
	}

	return objects
}

func translatorBackendStormObjects(routeCount int) []client.Object {
	objects := []client.Object{
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
				Addresses: []string{"10.1.0.10"},
			}},
		},
	}

	for i := 0; i < routeCount; i++ {
		routeName := translatorBenchmarkRouteName(i)
		port := gatewayv1.PortNumber(8080)

		objects = append(objects, &gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      routeName,
				Namespace: "default",
			},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{
						Name: "gw",
					}},
				},
				Rules: []gatewayv1.HTTPRouteRule{{
					BackendRefs: []gatewayv1.HTTPBackendRef{{
						BackendRef: gatewayv1.BackendRef{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Name: "echo",
								Port: &port,
							},
						},
					}},
				}},
			},
		})
	}

	return objects
}

func (f *translatorBackendStormFixture) setEndpointAddress(ctx context.Context, address string) error {
	var slice discoveryv1.EndpointSlice
	if err := f.client.Get(ctx, f.sliceKey, &slice); err != nil {
		return err
	}

	slice.Endpoints = []discoveryv1.Endpoint{{
		Addresses: []string{address},
	}}
	return f.client.Update(ctx, &slice)
}

func translatorBenchmarkServiceName(index int) string {
	return "translator-bench-svc-" + strconv.Itoa(index)
}

func translatorBenchmarkRouteName(index int) string {
	return "translator-bench-route-" + strconv.Itoa(index)
}

func translatorBenchmarkRouteKeys(routeCount int) []client.ObjectKey {
	keys := make([]client.ObjectKey, 0, routeCount)
	for i := 0; i < routeCount; i++ {
		keys = append(keys, client.ObjectKey{
			Namespace: "default",
			Name:      translatorBenchmarkRouteName(i),
		})
	}
	return keys
}

func (f *translatorAttachDetachStormFixture) setRoutesAttached(ctx context.Context, attached bool) error {
	for _, key := range f.routeKeys {
		var route gatewayv1.HTTPRoute
		if err := f.client.Get(ctx, key, &route); err != nil {
			return err
		}

		route.Generation++
		if attached {
			route.Spec.CommonRouteSpec.ParentRefs = []gatewayv1.ParentReference{{
				Name: "gw",
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

func totalAttachedRoutes(snapshot *ir.Snapshot) int {
	total := 0
	for _, listener := range snapshot.Listeners {
		total += len(listener.AttachedRoutes)
	}
	return total
}

func newTranslatorBenchmarkScheme(b *testing.B) *runtime.Scheme {
	b.Helper()

	scheme := runtime.NewScheme()
	translatorBenchmarkMustAddToScheme(b, corev1.AddToScheme(scheme))
	translatorBenchmarkMustAddToScheme(b, discoveryv1.AddToScheme(scheme))
	translatorBenchmarkMustAddToScheme(b, gatewayv1.Install(scheme))
	translatorBenchmarkMustAddToScheme(b, gatewayv1alpha2.Install(scheme))
	translatorBenchmarkMustAddToScheme(b, gatewayv1beta1.Install(scheme))
	return scheme
}

func translatorBenchmarkMustAddToScheme(b *testing.B, err error) {
	b.Helper()
	if err != nil {
		b.Fatalf("add to scheme: %v", err)
	}
}
