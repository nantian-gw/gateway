package admin

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strconv"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/aether-gateway/aether-gateway/controlplane/internal/ir"
)

func BenchmarkFilterRoutesQueryRouteFanout(b *testing.B) {
	for _, routeCount := range []int{50, 200} {
		b.Run(fmt.Sprintf("routes_%d", routeCount), func(b *testing.B) {
			b.ReportAllocs()

			snapshot := adminBenchmarkSnapshot(routeCount)
			query := url.Values{
				"kind":   []string{"http"},
				"sort":   []string{"name"},
				"order":  []string{"desc"},
				"limit":  []string{"25"},
				"offset": []string{"10"},
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				response, err := filterRoutes(snapshot, query)
				if err != nil {
					b.Fatalf("filterRoutes returned error: %v", err)
				}
				if len(response.HTTP) != 25 {
					b.Fatalf("expected 25 http routes, got %d", len(response.HTTP))
				}
			}
		})
	}
}

func BenchmarkFilterBackendsQueryRouteFanout(b *testing.B) {
	for _, routeCount := range []int{50, 200} {
		b.Run(fmt.Sprintf("routes_%d", routeCount), func(b *testing.B) {
			b.ReportAllocs()

			snapshot := adminBenchmarkSnapshot(routeCount)
			query := url.Values{
				"sort":   []string{"protocol"},
				"order":  []string{"desc"},
				"limit":  []string{"25"},
				"offset": []string{"10"},
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				backends, err := filterBackends(snapshot, query)
				if err != nil {
					b.Fatalf("filterBackends returned error: %v", err)
				}
				if len(backends) != 25 {
					b.Fatalf("expected 25 backends, got %d", len(backends))
				}
			}
		})
	}
}

func BenchmarkListResourcesQueryPaths(b *testing.B) {
	for _, routeCount := range []int{50, 200} {
		b.Run(fmt.Sprintf("routes_%d", routeCount), func(b *testing.B) {
			b.ReportAllocs()

			manager := newAdminBenchmarkResourceManager(b, routeCount)
			ctx := context.Background()

			b.Run("kind_list_cache_miss", func(b *testing.B) {
				filter := ResourceListFilter{
					Kind:      "HTTPRoute",
					Namespace: "default",
					Offset:    10,
					Limit:     25,
					HasLimit:  true,
				}

				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					manager.invalidateListCache()
					items, err := manager.List(ctx, filter)
					if err != nil {
						b.Fatalf("ResourceManager.List returned error: %v", err)
					}
					if len(items) != 25 {
						b.Fatalf("expected 25 resources, got %d", len(items))
					}
				}
			})

			b.Run("kind_list_cache_hit", func(b *testing.B) {
				filter := ResourceListFilter{
					Kind:      "HTTPRoute",
					Namespace: "default",
					Offset:    10,
					Limit:     25,
					HasLimit:  true,
				}
				manager.invalidateListCache()
				if _, err := manager.List(ctx, filter); err != nil {
					b.Fatalf("warm ResourceManager.List cache returned error: %v", err)
				}

				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					items, err := manager.List(ctx, filter)
					if err != nil {
						b.Fatalf("ResourceManager.List cached returned error: %v", err)
					}
					if len(items) != 25 {
						b.Fatalf("expected 25 cached resources, got %d", len(items))
					}
				}
			})

			b.Run("exact_match", func(b *testing.B) {
				filter := ResourceListFilter{
					Kind:      "HTTPRoute",
					Namespace: "default",
					Name:      adminBenchmarkRouteName(routeCount / 2),
				}

				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					items, err := manager.List(ctx, filter)
					if err != nil {
						b.Fatalf("ResourceManager.List exact match returned error: %v", err)
					}
					if len(items) != 1 {
						b.Fatalf("expected 1 exact-match resource, got %d", len(items))
					}
				}
			})
		})
	}
}

func BenchmarkListServiceCatalogQueryPaths(b *testing.B) {
	for _, serviceCount := range []int{50, 200} {
		b.Run(fmt.Sprintf("services_%d", serviceCount), func(b *testing.B) {
			b.ReportAllocs()

			manager := newAdminBenchmarkResourceManager(b, serviceCount)
			ctx := context.Background()

			b.Run("namespace_list_cache_miss", func(b *testing.B) {
				filter := ServiceCatalogFilter{
					Namespace: "default",
					Sort:      serviceCatalogSortByName,
					Order:     sortOrderDescending,
					Offset:    10,
					Limit:     25,
					HasLimit:  true,
				}

				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					manager.invalidateListCache()
					items, err := manager.ListServiceCatalog(ctx, filter)
					if err != nil {
						b.Fatalf("ListServiceCatalog returned error: %v", err)
					}
					if len(items) != 25 {
						b.Fatalf("expected 25 service catalog entries, got %d", len(items))
					}
				}
			})

			b.Run("namespace_list_cache_hit", func(b *testing.B) {
				filter := ServiceCatalogFilter{
					Namespace: "default",
					Sort:      serviceCatalogSortByName,
					Order:     sortOrderDescending,
					Offset:    10,
					Limit:     25,
					HasLimit:  true,
				}
				manager.invalidateListCache()
				if _, err := manager.ListServiceCatalog(ctx, filter); err != nil {
					b.Fatalf("warm ListServiceCatalog cache returned error: %v", err)
				}

				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					items, err := manager.ListServiceCatalog(ctx, filter)
					if err != nil {
						b.Fatalf("ListServiceCatalog cached returned error: %v", err)
					}
					if len(items) != 25 {
						b.Fatalf("expected 25 cached service catalog entries, got %d", len(items))
					}
				}
			})

			b.Run("exact_match", func(b *testing.B) {
				filter := ServiceCatalogFilter{
					Namespace: "default",
					Name:      adminBenchmarkServiceName(serviceCount / 2),
					Protocol:  string(corev1.ProtocolTCP),
					Port:      8080,
					HasPort:   true,
				}

				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					items, err := manager.ListServiceCatalog(ctx, filter)
					if err != nil {
						b.Fatalf("ListServiceCatalog exact match returned error: %v", err)
					}
					if len(items) != 1 {
						b.Fatalf("expected 1 exact-match service catalog entry, got %d", len(items))
					}
				}
			})
		})
	}
}

func adminBenchmarkSnapshot(routeCount int) *ir.Snapshot {
	snapshot := &ir.Snapshot{
		GeneratedAt: time.Unix(1_700_000_000, 0).UTC(),
		HTTPRoutes:  make([]ir.HTTPRoute, 0, routeCount),
		Backends:    make([]ir.BackendCluster, 0, routeCount+(routeCount/4)),
	}

	protocols := []string{"HTTP", "GRPC", "TCP", "UDP"}
	for i := 0; i < routeCount; i++ {
		serviceName := adminBenchmarkServiceName(i)
		routeName := adminBenchmarkRouteName(i)

		snapshot.HTTPRoutes = append(snapshot.HTTPRoutes, ir.HTTPRoute{
			Name:      routeName,
			Namespace: "default",
			Hostnames: []string{fmt.Sprintf("%s.example.com", routeName)},
			Rules: []ir.HTTPRule{{
				BackendRefs: []ir.BackendRef{{
					Name:      serviceName,
					Namespace: "default",
					Port:      80,
				}},
			}},
		})
		snapshot.Backends = append(snapshot.Backends, ir.BackendCluster{
			Name:      serviceName + ":80",
			Namespace: "default",
			Protocol:  protocols[i%len(protocols)],
			Metadata:  map[string]string{"service": serviceName},
			Endpoints: []ir.BackendEndpoint{{
				Address: fmt.Sprintf("10.0.%d.%d", i/250, (i%250)+1),
				Port:    80,
				Healthy: true,
			}},
		})
	}

	// Include some unreferenced backends so the benchmark exercises referenced-backend filtering.
	for i := 0; i < routeCount/4; i++ {
		serviceName := "orphan-" + strconv.Itoa(i)
		snapshot.Backends = append(snapshot.Backends, ir.BackendCluster{
			Name:      serviceName + ":80",
			Namespace: "default",
			Protocol:  protocols[(i+1)%len(protocols)],
			Metadata:  map[string]string{"service": serviceName},
			Endpoints: []ir.BackendEndpoint{{
				Address: fmt.Sprintf("10.1.%d.%d", i/250, (i%250)+1),
				Port:    80,
				Healthy: true,
			}},
		})
	}

	return snapshot
}

func newAdminBenchmarkResourceManager(b *testing.B, itemCount int) *ResourceManager {
	b.Helper()

	scheme := runtime.NewScheme()
	adminBenchmarkMustAddToScheme(b, gatewayv1.Install(scheme))
	adminBenchmarkMustAddToScheme(b, corev1.AddToScheme(scheme))

	objects := make([]client.Object, 0, itemCount*2)
	for i := 0; i < itemCount; i++ {
		serviceName := adminBenchmarkServiceName(i)
		routeName := adminBenchmarkRouteName(i)

		objects = append(objects,
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      routeName,
					Namespace: "default",
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Port:       8080,
							Protocol:   corev1.ProtocolTCP,
							TargetPort: intstr.FromInt(8080),
						},
						{
							Name:       "https",
							Port:       8443,
							Protocol:   corev1.ProtocolTCP,
							TargetPort: intstr.FromString("https"),
						},
					},
				},
			},
		)
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		Build()

	return NewResourceManager(k8sClient, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func adminBenchmarkRouteName(index int) string {
	return "bench-route-" + strconv.Itoa(index)
}

func adminBenchmarkServiceName(index int) string {
	return "bench-service-" + strconv.Itoa(index)
}

func adminBenchmarkMustAddToScheme(b *testing.B, err error) {
	b.Helper()
	if err != nil {
		b.Fatalf("add to scheme: %v", err)
	}
}
