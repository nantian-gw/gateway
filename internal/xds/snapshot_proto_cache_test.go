package xds

import (
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nantian-gw/gateway/internal/ir"
	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"
)

func TestSnapshotProtoCacheBuildsOncePerSnapshotID(t *testing.T) {
	t.Parallel()

	builds := 0
	cache := newSnapshotProtoCache(func(snapshot *ir.Snapshot, _ projectionProfile, _ *slog.Logger) *controlv1.ConfigSnapshot {
		builds++
		return &controlv1.ConfigSnapshot{
			Id: snapshot.ID,
			Listeners: []*controlv1.Listener{{
				Name: "listener-" + snapshot.ID,
			}},
		}
	})

	snapshot := &ir.Snapshot{ID: "v1", GeneratedAt: time.Now().UTC()}
	full := effectiveProjectionProfile([]string{featureCoreV1, featureRouteLabelsV1, featureBackendAIServiceV1, featureBackendTokenPolicyV1, featureBackendWasmPluginV1})
	first := cache.get(snapshot, full, nil)
	second := cache.get(snapshot, full, nil)
	if builds != 1 {
		t.Fatalf("expected one proto build for repeated snapshot ID, got %d", builds)
	}
	if first != second {
		t.Fatal("expected repeated snapshot ID to reuse cached proto object")
	}

	cache.get(&ir.Snapshot{ID: "v2", GeneratedAt: time.Now().UTC()}, full, nil)
	if builds != 2 {
		t.Fatalf("expected cache miss for a new snapshot ID, got %d builds", builds)
	}
}

func TestSnapshotProtoCacheSeparatesProjectionKeys(t *testing.T) {
	t.Parallel()

	builds := 0
	cache := newSnapshotProtoCache(func(snapshot *ir.Snapshot, profile projectionProfile, _ *slog.Logger) *controlv1.ConfigSnapshot {
		builds++
		return &controlv1.ConfigSnapshot{
			Id:                   snapshot.ID,
			CompatibilityProfile: profile.compatibilityProfile,
		}
	})

	snapshot := &ir.Snapshot{ID: "v1", GeneratedAt: time.Now().UTC()}
	full := effectiveProjectionProfile([]string{featureCoreV1, featureRouteLabelsV1, featureBackendAIServiceV1, featureBackendTokenPolicyV1, featureBackendWasmPluginV1})
	coreOnly := effectiveProjectionProfile([]string{featureCoreV1})

	first := cache.get(snapshot, full, nil)
	second := cache.get(snapshot, full, nil)
	third := cache.get(snapshot, coreOnly, nil)
	fourth := cache.get(snapshot, coreOnly, nil)

	if builds != 2 {
		t.Fatalf("expected one build per projection key, got %d", builds)
	}
	if first != second {
		t.Fatal("expected repeated full-profile lookups to reuse the cached object")
	}
	if third != fourth {
		t.Fatal("expected repeated core-only lookups to reuse the cached object")
	}
	if first == third {
		t.Fatal("expected different projection keys to return different cached objects")
	}
	if first.GetCompatibilityProfile() == third.GetCompatibilityProfile() {
		t.Fatalf("expected distinct compatibility profiles, got %q and %q", first.GetCompatibilityProfile(), third.GetCompatibilityProfile())
	}
}

func TestSnapshotProtoCacheBuildsOnceForConcurrentReaders(t *testing.T) {
	t.Parallel()

	var builds atomic.Int32
	cache := newSnapshotProtoCache(func(snapshot *ir.Snapshot, _ projectionProfile, _ *slog.Logger) *controlv1.ConfigSnapshot {
		builds.Add(1)
		// sleep to simulate slow config snapshot build
		time.Sleep(10 * time.Millisecond)
		return &controlv1.ConfigSnapshot{Id: snapshot.ID}
	})
	snapshot := &ir.Snapshot{ID: "v-concurrent", GeneratedAt: time.Now().UTC()}
	full := effectiveProjectionProfile([]string{featureCoreV1, featureRouteLabelsV1, featureBackendAIServiceV1, featureBackendTokenPolicyV1, featureBackendWasmPluginV1})

	const readers = 32
	results := make(chan *controlv1.ConfigSnapshot, readers)
	var wg sync.WaitGroup
	wg.Add(readers)
	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			results <- cache.get(snapshot, full, nil)
		}()
	}
	wg.Wait()
	close(results)

	var first *controlv1.ConfigSnapshot
	for result := range results {
		if result.GetId() != "v-concurrent" {
			t.Fatalf("unexpected snapshot ID: %q", result.GetId())
		}
		if first == nil {
			first = result
			continue
		}
		if first != result {
			t.Fatal("expected concurrent readers to reuse the cached proto object")
		}
	}
	if got := builds.Load(); got != 1 {
		t.Fatalf("expected one proto build for concurrent readers, got %d", got)
	}
}

func BenchmarkToProtoSnapshotFanout(b *testing.B) {
	snapshot := benchmarkProtoSnapshot()

	for _, nodes := range []int{1, 10, 100} {
		b.Run(fmt.Sprintf("uncached/nodes_%d", nodes), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				for node := 0; node < nodes; node++ {
					_ = toProtoSnapshotWithLogger(snapshot, nil)
				}
			}
		})

		b.Run(fmt.Sprintf("cached/nodes_%d", nodes), func(b *testing.B) {
			cache := newSnapshotProtoCache(nil)
			full := effectiveProjectionProfile([]string{featureCoreV1, featureRouteLabelsV1, featureBackendAIServiceV1, featureBackendTokenPolicyV1, featureBackendWasmPluginV1})
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				for node := 0; node < nodes; node++ {
					_ = cache.get(snapshot, full, nil)
				}
			}
		})
	}
}

func benchmarkProtoSnapshot() *ir.Snapshot {
	const (
		listeners        = 8
		routesPerListen  = 8
		backendsPerRoute = 4
		endpoints        = 4
	)

	snapshot := &ir.Snapshot{
		ID:          "benchmark-snapshot",
		GeneratedAt: time.Now().UTC(),
	}

	for listener := 0; listener < listeners; listener++ {
		snapshot.Listeners = append(snapshot.Listeners, ir.Listener{
			Name:      fmt.Sprintf("listener-%02d", listener),
			Address:   "0.0.0.0",
			Port:      uint32(10_080 + listener),
			Protocol:  "HTTP",
			Hostnames: []string{fmt.Sprintf("app-%02d.example.com", listener)},
		})
		for route := 0; route < routesPerListen; route++ {
			routeName := fmt.Sprintf("route-%02d-%02d", listener, route)
			httpRoute := ir.HTTPRoute{
				Name:      routeName,
				Namespace: "default",
				Hostnames: []string{fmt.Sprintf("app-%02d.example.com", listener)},
				ParentRefs: []ir.ParentRef{{
					Name:        fmt.Sprintf("gateway-%02d", listener),
					SectionName: fmt.Sprintf("listener-%02d", listener),
				}},
				Annotations: map[string]string{
					"gateway.nantian.dev/access-log-mode": "json",
				},
			}
			rule := ir.HTTPRule{
				Matches: []ir.HTTPMatch{{
					Path:     fmt.Sprintf("/service-%02d", route),
					PathType: "PathPrefix",
					Method:   "GET",
					Headers: []ir.HeaderMatch{{
						Name:      "x-tenant",
						Value:     fmt.Sprintf("tenant-%02d", listener),
						MatchType: "Exact",
					}},
				}},
				Filters: []ir.Filter{{
					Type: "RequestHeaderModifier",
					Config: map[string]any{
						"set": []any{map[string]any{
							"name":  "x-gateway",
							"value": "nantian-gw",
						}},
					},
				}},
			}
			for backend := 0; backend < backendsPerRoute; backend++ {
				backendName := fmt.Sprintf("backend-%02d-%02d-%02d", listener, route, backend)
				rule.BackendRefs = append(rule.BackendRefs, ir.BackendRef{
					Name:      backendName,
					Namespace: "default",
					Port:      8080,
					Weight:    1,
				})
				snapshot.Backends = append(snapshot.Backends, ir.BackendCluster{
					Name:      backendName,
					Namespace: "default",
					Protocol:  "HTTP",
					Endpoints: benchmarkEndpoints(listener, route, backend, endpoints),
				})
			}
			httpRoute.Rules = []ir.HTTPRule{rule}
			snapshot.HTTPRoutes = append(snapshot.HTTPRoutes, httpRoute)
		}
	}

	return snapshot
}

func benchmarkEndpoints(listener, route, backend, endpoints int) []ir.BackendEndpoint {
	out := make([]ir.BackendEndpoint, 0, endpoints)
	for endpoint := 0; endpoint < endpoints; endpoint++ {
		out = append(out, ir.BackendEndpoint{
			Address: fmt.Sprintf("10.%d.%d.%d", listener+1, route+1, backend*endpoints+endpoint+1),
			Port:    8080,
			Healthy: true,
			Zone:    fmt.Sprintf("zone-%d", endpoint%3),
		})
	}
	return out
}
