package ir

import (
	"fmt"
	"testing"
	"time"
)

var benchmarkSnapshotIDSink string

func BenchmarkSnapshotNormalizeLarge(b *testing.B) {
	base := benchmarkLargeSnapshotFixture()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		snapshot := base.Clone()
		b.StartTimer()

		if err := snapshot.Normalize(); err != nil {
			b.Fatalf("Normalize returned error: %v", err)
		}
		benchmarkSnapshotIDSink = snapshot.ID
	}
}

func benchmarkLargeSnapshotFixture() *Snapshot {
	const (
		listenerCount       = 96
		httpRouteCount      = 600
		grpcRouteCount      = 300
		streamRouteCount    = 300
		backendCount        = 600
		endpointsPerBackend = 4
		workloadCount       = 1_200
	)

	snapshot := &Snapshot{
		GeneratedAt: time.Unix(1_777_500_000, 123_000_000).UTC(),
		Listeners:   make([]Listener, 0, listenerCount),
		HTTPRoutes:  make([]HTTPRoute, 0, httpRouteCount),
		GRPCRoutes:  make([]GRPCRoute, 0, grpcRouteCount),
		StreamRoutes: make(
			[]StreamRoute,
			0,
			streamRouteCount,
		),
		Backends:  make([]BackendCluster, 0, backendCount),
		Secrets:   make([]SecretMaterial, 0, listenerCount),
		Workloads: make([]Workload, 0, workloadCount),
	}

	for i := listenerCount - 1; i >= 0; i-- {
		snapshot.Listeners = append(snapshot.Listeners, benchmarkLargeSnapshotListener(i))
		snapshot.Secrets = append(snapshot.Secrets, SecretMaterial{
			Namespace: benchmarkNamespace(i),
			Name:      fmt.Sprintf("tls-%03d", i),
			CertPEM:   fmt.Sprintf("cert-%03d", i),
			KeyPEM:    fmt.Sprintf("key-%03d", i),
		})
	}
	for i := httpRouteCount - 1; i >= 0; i-- {
		snapshot.HTTPRoutes = append(snapshot.HTTPRoutes, benchmarkLargeSnapshotHTTPRoute(i, backendCount))
	}
	for i := grpcRouteCount - 1; i >= 0; i-- {
		snapshot.GRPCRoutes = append(snapshot.GRPCRoutes, benchmarkLargeSnapshotGRPCRoute(i, backendCount))
	}
	for i := streamRouteCount - 1; i >= 0; i-- {
		snapshot.StreamRoutes = append(snapshot.StreamRoutes, benchmarkLargeSnapshotStreamRoute(i, backendCount))
	}
	for i := backendCount - 1; i >= 0; i-- {
		snapshot.Backends = append(snapshot.Backends, benchmarkLargeSnapshotBackend(i, endpointsPerBackend))
	}
	for i := workloadCount - 1; i >= 0; i-- {
		snapshot.Workloads = append(snapshot.Workloads, Workload{
			Namespace: benchmarkNamespace(i),
			Name:      fmt.Sprintf("workload-%04d", i),
			IP:        fmt.Sprintf("10.%d.%d.%d", (i/65_536)%256, (i/256)%256, i%256),
		})
	}

	return snapshot
}

func benchmarkLargeSnapshotListener(index int) Listener {
	protocol := "HTTP"
	port := uint32(80 + index%16)
	if index%5 == 0 {
		protocol = "HTTPS"
		port = 443
	}
	listener := Listener{
		Name:      fmt.Sprintf("%s/gw-%03d/%s", benchmarkNamespace(index), index, benchmarkListenerName(index)),
		Address:   "0.0.0.0",
		Addresses: []string{fmt.Sprintf("192.0.2.%d", (index%200)+1), "127.0.0.1"},
		Port:      port,
		Protocol:  protocol,
		Hostnames: []string{
			fmt.Sprintf("z-%03d.example.com", index),
			fmt.Sprintf("a-%03d.example.com", index),
		},
		AttachedRoutes: []string{
			fmt.Sprintf("%s/http-%04d", benchmarkNamespace(index), index%600),
			fmt.Sprintf("%s/grpc-%04d", benchmarkNamespace(index), index%300),
		},
		Metadata: map[string]string{
			"gateway": fmt.Sprintf("gw-%03d", index),
			"profile": "large-normalize-benchmark",
		},
		Status: &ListenerStatus{
			AttachedRoutes: index % 32,
			Conditions: []ConditionStatus{
				{Type: "Programmed", Status: "True", ObservedGeneration: int64(index + 1)},
				{Type: "Accepted", Status: "True", ObservedGeneration: int64(index + 1)},
				{Type: "ResolvedRefs", Status: "True", ObservedGeneration: int64(index + 1)},
			},
		},
	}
	if protocol == "HTTPS" {
		listener.TLS = &TLSConfig{
			Enabled:    true,
			SecretRefs: []string{fmt.Sprintf("%s/tls-%03d", benchmarkNamespace(index), index)},
			SNIHosts: []string{
				fmt.Sprintf("z-%03d.example.com", index),
				fmt.Sprintf("a-%03d.example.com", index),
			},
			MinVersion: "1.2",
			MaxVersion: "1.3",
		}
	}
	return listener
}

func benchmarkLargeSnapshotHTTPRoute(index int, backendCount int) HTTPRoute {
	requestTimeout := time.Duration(2+index%7) * time.Second
	backendTimeout := time.Duration(100+index%50) * time.Millisecond
	backoff := time.Duration(10+index%10) * time.Millisecond
	return HTTPRoute{
		Name:      fmt.Sprintf("http-%04d", index),
		Namespace: benchmarkNamespace(index),
		Hostnames: []string{
			fmt.Sprintf("z-%04d.example.com", index),
			fmt.Sprintf("a-%04d.example.com", index),
		},
		ParentRefs: benchmarkRouteParents(index),
		Labels: map[string]string{
			"app":     fmt.Sprintf("app-%04d", index),
			"version": fmt.Sprintf("v%d", index%8),
		},
		Annotations: map[string]string{
			"nantian.dev/access-log-template": "default",
			"benchmark-index":                         fmt.Sprintf("%04d", index),
		},
		Rules: []HTTPRule{{
			Matches: []HTTPMatch{{
				Path:     fmt.Sprintf("/api/%04d", index),
				PathType: "PathPrefix",
				Method:   "GET",
				Headers: []HeaderMatch{
					{Name: "x-tenant", Value: fmt.Sprintf("tenant-%02d", index%16), MatchType: "Exact"},
					{Name: "x-shard", Value: fmt.Sprintf("%02d", index%32), MatchType: "Exact"},
				},
				QueryParams: []QueryMatch{
					{Name: "region", Value: fmt.Sprintf("r%d", index%4), MatchType: "Exact"},
					{Name: "version", Value: fmt.Sprintf("v%d", index%8), MatchType: "Exact"},
				},
			}},
			Filters: []Filter{{
				Type: "RequestHeaderModifier",
				Config: map[string]any{
					"add": map[string]any{
						"x-benchmark-route": fmt.Sprintf("http-%04d", index),
					},
				},
			}},
			BackendRefs: []BackendRef{
				benchmarkBackendRef(index, backendCount, 80),
				benchmarkBackendRef(index+1, backendCount, 20),
			},
			Timeouts: &RouteTimeouts{
				Request:        &requestTimeout,
				BackendRequest: &backendTimeout,
			},
			Retry: &RetryPolicy{
				Codes:    []uint32{503, 502},
				Attempts: 2,
				Backoff:  &backoff,
			},
		}},
		Status: benchmarkRouteStatus(index),
	}
}

func benchmarkLargeSnapshotGRPCRoute(index int, backendCount int) GRPCRoute {
	return GRPCRoute{
		Name:       fmt.Sprintf("grpc-%04d", index),
		Namespace:  benchmarkNamespace(index),
		Hostnames:  []string{fmt.Sprintf("grpc-z-%04d.example.com", index), fmt.Sprintf("grpc-a-%04d.example.com", index)},
		ParentRefs: benchmarkRouteParents(index),
		Labels: map[string]string{
			"app":     fmt.Sprintf("grpc-app-%04d", index),
			"version": fmt.Sprintf("v%d", index%8),
		},
		Rules: []GRPCRule{{
			Matches: []GRPCMatch{{
				Service:   fmt.Sprintf("pkg.Service%d", index%64),
				Method:    fmt.Sprintf("Method%d", index%32),
				MatchType: "Exact",
				Headers: []HeaderMatch{
					{Name: "x-tenant", Value: fmt.Sprintf("tenant-%02d", index%16), MatchType: "Exact"},
					{Name: "x-shard", Value: fmt.Sprintf("%02d", index%32), MatchType: "Exact"},
				},
			}},
			BackendRefs: []BackendRef{
				benchmarkBackendRef(index, backendCount, 70),
				benchmarkBackendRef(index+3, backendCount, 30),
			},
		}},
		Status: benchmarkRouteStatus(index),
	}
}

func benchmarkLargeSnapshotStreamRoute(index int, backendCount int) StreamRoute {
	kind := "TCP"
	if index%3 == 0 {
		kind = "TLS"
	}
	return StreamRoute{
		Name:       fmt.Sprintf("stream-%04d", index),
		Namespace:  benchmarkNamespace(index),
		Kind:       kind,
		ParentRefs: benchmarkRouteParents(index),
		Labels: map[string]string{
			"app": fmt.Sprintf("stream-app-%04d", index),
		},
		Rules: []StreamRule{{
			Matches: []StreamMatch{{
				Port:        uint32(10_000 + index%1_000),
				SNIHostname: fmt.Sprintf("stream-%04d.example.com", index),
			}},
			BackendRefs: []BackendRef{
				benchmarkBackendRef(index, backendCount, 100),
			},
		}},
		Status: benchmarkRouteStatus(index),
	}
}

func benchmarkLargeSnapshotBackend(index int, endpointsPerBackend int) BackendCluster {
	connectTimeout := time.Duration(50+index%25) * time.Millisecond
	requestTimeout := time.Duration(2+index%5) * time.Second
	backend := BackendCluster{
		Name:           benchmarkBackendName(index),
		Namespace:      benchmarkNamespace(index),
		Protocol:       benchmarkBackendProtocol(index),
		ConnectTimeout: connectTimeout,
		RequestTimeout: requestTimeout,
		Endpoints:      make([]BackendEndpoint, 0, endpointsPerBackend),
		Metadata: map[string]string{
			"service": fmt.Sprintf("svc-%04d", index),
			"zone":    fmt.Sprintf("zone-%d", index%3),
		},
		LoadBalancing: &LoadBalancingPolicy{
			Type: "RoundRobin",
		},
	}
	if index%4 == 0 {
		backend.BackendTLSValidation = &BackendTLSValidation{
			Hostname:     fmt.Sprintf("svc-%04d.%s.svc.cluster.local", index, benchmarkNamespace(index)),
			UseSystemCAs: true,
			SubjectAltNames: []BackendSubjectName{
				{Type: "Hostname", Value: fmt.Sprintf("svc-%04d.%s.svc", index, benchmarkNamespace(index))},
				{Type: "URI", Value: fmt.Sprintf("spiffe://cluster.local/ns/%s/sa/svc-%04d", benchmarkNamespace(index), index)},
			},
			MinVersion: "1.2",
			MaxVersion: "1.3",
		}
	}
	for endpoint := endpointsPerBackend - 1; endpoint >= 0; endpoint-- {
		backend.Endpoints = append(backend.Endpoints, BackendEndpoint{
			Address: fmt.Sprintf("10.%d.%d.%d", (index/256)%256, index%256, endpoint+1),
			Port:    8_000 + uint32(index%1_000),
			Healthy: endpoint%5 != 0,
			Zone:    fmt.Sprintf("zone-%d", endpoint%3),
		})
	}
	return backend
}

func benchmarkBackendRef(index int, backendCount int, weight uint32) BackendRef {
	backendIndex := index % backendCount
	return BackendRef{
		Kind:      "Service",
		Namespace: benchmarkNamespace(backendIndex),
		Name:      benchmarkBackendName(backendIndex),
		Port:      8_000 + uint32(backendIndex%1_000),
		Weight:    weight,
		Metadata: map[string]string{
			"backend-index": fmt.Sprintf("%04d", backendIndex),
		},
	}
}

func benchmarkRouteParents(index int) []ParentRef {
	return []ParentRef{
		{
			Group:       "gateway.networking.k8s.io",
			Kind:        "Gateway",
			Namespace:   benchmarkNamespace(index),
			Name:        fmt.Sprintf("gw-%03d", index%96),
			SectionName: benchmarkListenerName(index),
			Port:        uint32(80 + index%16),
		},
		{
			Group:       "gateway.networking.k8s.io",
			Kind:        "Gateway",
			Namespace:   benchmarkNamespace(index + 1),
			Name:        fmt.Sprintf("gw-%03d", (index+1)%96),
			SectionName: benchmarkListenerName(index + 1),
			Port:        uint32(80 + (index+1)%16),
		},
	}
}

func benchmarkRouteStatus(index int) *RouteStatus {
	return &RouteStatus{
		Parents: []RouteParentStatus{
			{
				ControllerName: "gateway.networking.k8s.io/aether-gateway",
				ParentRef:      benchmarkRouteParents(index)[1],
				Conditions: []ConditionStatus{
					{Type: "ResolvedRefs", Status: "True", ObservedGeneration: int64(index + 1)},
					{Type: "Accepted", Status: "True", ObservedGeneration: int64(index + 1)},
				},
			},
			{
				ControllerName: "gateway.networking.k8s.io/aether-gateway",
				ParentRef:      benchmarkRouteParents(index)[0],
				Conditions: []ConditionStatus{
					{Type: "Accepted", Status: "True", ObservedGeneration: int64(index + 1)},
					{Type: "ResolvedRefs", Status: "True", ObservedGeneration: int64(index + 1)},
				},
			},
		},
	}
}

func benchmarkNamespace(index int) string {
	return fmt.Sprintf("ns-%02d", index%24)
}

func benchmarkListenerName(index int) string {
	return fmt.Sprintf("listener-%02d", index%16)
}

func benchmarkBackendName(index int) string {
	return fmt.Sprintf("svc-%04d", index)
}

func benchmarkBackendProtocol(index int) string {
	switch index % 4 {
	case 0:
		return "GRPC"
	case 1:
		return "HTTP"
	case 2:
		return "TCP"
	default:
		return "UDP"
	}
}
