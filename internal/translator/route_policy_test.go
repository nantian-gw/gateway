package translator

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	routepolicy "github.com/nantian-gw/gateway/internal/gatewayexp/routepolicy"
	"github.com/nantian-gw/gateway/internal/ir"
)

func TestTranslateRoutePolicyDefault_NilSpec(t *testing.T) {
	result := translateRoutePolicyDefault(nil)
	if result != nil {
		t.Fatalf("expected nil for nil spec, got %#v", result)
	}
}

func TestTranslateRoutePolicyDefault_Timeout(t *testing.T) {
	requestDur := metav1.Duration{Duration: 10 * time.Second}
	backendDur := metav1.Duration{Duration: 20 * time.Second}
	connectDur := metav1.Duration{Duration: 30 * time.Second}
	nextDur := metav1.Duration{Duration: 40 * time.Second}

	spec := &routepolicy.RoutePolicyDefault{
		Timeout: &routepolicy.TimeoutConfig{
			Request:        &requestDur,
			BackendRequest: &backendDur,
			Connect:        &connectDur,
			NextUpstream:   &nextDur,
		},
	}

	result := translateRoutePolicyDefault(spec)
	if result == nil {
		t.Fatal("expected config, got nil")
	}
	if result.Timeout == nil {
		t.Fatal("expected timeout config")
	}
	if result.Timeout.Request != 10*time.Second {
		t.Fatalf("unexpected request timeout: %v", result.Timeout.Request)
	}
	if result.Timeout.BackendRequest != 20*time.Second {
		t.Fatalf("unexpected backend request timeout: %v", result.Timeout.BackendRequest)
	}
	if result.Timeout.Connect != 30*time.Second {
		t.Fatalf("unexpected connect timeout: %v", result.Timeout.Connect)
	}
	if result.Timeout.NextUpstream != 40*time.Second {
		t.Fatalf("unexpected next upstream timeout: %v", result.Timeout.NextUpstream)
	}
	if result.BodyLimit != nil || result.Proxy != nil || result.Connection != nil {
		t.Fatalf("unexpected non-nil sub-configs")
	}
}

func TestTranslateRoutePolicyDefault_BodyLimit(t *testing.T) {
	maxBody := uint64(1048576)
	bufBytes := uint64(65536)
	hdrBytes := uint64(8192)

	spec := &routepolicy.RoutePolicyDefault{
		BodyLimit: &routepolicy.BodyLimitConfig{
			MaxRequestBodyBytes:    &maxBody,
			RequestBodyBufferBytes: &bufBytes,
			MaxRequestHeaderBytes:  &hdrBytes,
		},
	}

	result := translateRoutePolicyDefault(spec)
	if result == nil {
		t.Fatal("expected config, got nil")
	}
	if result.BodyLimit == nil {
		t.Fatal("expected body limit config")
	}
	if result.BodyLimit.MaxRequestBodyBytes != 1048576 {
		t.Fatalf("unexpected max request body: %d", result.BodyLimit.MaxRequestBodyBytes)
	}
	if result.BodyLimit.RequestBodyBufferBytes != 65536 {
		t.Fatalf("unexpected request body buffer: %d", result.BodyLimit.RequestBodyBufferBytes)
	}
	if result.BodyLimit.MaxRequestHeaderBytes != 8192 {
		t.Fatalf("unexpected max request header: %d", result.BodyLimit.MaxRequestHeaderBytes)
	}
}

func TestTranslateRoutePolicyDefault_Proxy(t *testing.T) {
	reqBuf := true
	respBuf := false
	bufSize := uint64(32768)
	bufCount := uint32(16)

	spec := &routepolicy.RoutePolicyDefault{
		Proxy: &routepolicy.ProxyConfig{
			RequestBuffering:  &reqBuf,
			ResponseBuffering: &respBuf,
			BufferSize:        &bufSize,
			BufferCount:       &bufCount,
		},
	}

	result := translateRoutePolicyDefault(spec)
	if result == nil {
		t.Fatal("expected config, got nil")
	}
	if result.Proxy == nil {
		t.Fatal("expected proxy config")
	}
	if result.Proxy.RequestBuffering != true {
		t.Fatal("expected request buffering true")
	}
	if result.Proxy.ResponseBuffering != false {
		t.Fatal("expected response buffering false")
	}
	if result.Proxy.BufferSize != 32768 {
		t.Fatalf("unexpected buffer size: %d", result.Proxy.BufferSize)
	}
	if result.Proxy.BufferCount != 16 {
		t.Fatalf("unexpected buffer count: %d", result.Proxy.BufferCount)
	}
}

func TestTranslateRoutePolicyDefault_Connection(t *testing.T) {
	keepaliveReqs := uint32(100)
	poolSize := uint32(32)
	keepTime := metav1.Duration{Duration: 2 * time.Hour}
	keepTimeout := metav1.Duration{Duration: 20 * time.Second}
	keepIdle := metav1.Duration{Duration: 60 * time.Second}

	spec := &routepolicy.RoutePolicyDefault{
		Connection: &routepolicy.ConnectionConfig{
			KeepaliveRequests:         &keepaliveReqs,
			UpstreamKeepalivePoolSize: &poolSize,
			KeepaliveTime:             &keepTime,
			KeepaliveTimeout:          &keepTimeout,
			UpstreamKeepaliveIdle:     &keepIdle,
		},
	}

	result := translateRoutePolicyDefault(spec)
	if result == nil {
		t.Fatal("expected config, got nil")
	}
	if result.Connection == nil {
		t.Fatal("expected connection config")
	}
	if result.Connection.KeepaliveRequests != 100 {
		t.Fatalf("unexpected keepalive requests: %d", result.Connection.KeepaliveRequests)
	}
	if result.Connection.UpstreamKeepalivePoolSize != 32 {
		t.Fatalf("unexpected upstream keepalive pool size: %d", result.Connection.UpstreamKeepalivePoolSize)
	}
	if result.Connection.KeepaliveTime != 2*time.Hour {
		t.Fatalf("unexpected keepalive time: %v", result.Connection.KeepaliveTime)
	}
	if result.Connection.KeepaliveTimeout != 20*time.Second {
		t.Fatalf("unexpected keepalive timeout: %v", result.Connection.KeepaliveTimeout)
	}
	if result.Connection.UpstreamKeepaliveIdle != 60*time.Second {
		t.Fatalf("unexpected upstream keepalive idle: %v", result.Connection.UpstreamKeepaliveIdle)
	}
}

func TestBuildRoutePolicyIndexes_EmptyTargetRefs_NamespaceLevel(t *testing.T) {
	requestDur := metav1.Duration{Duration: 5 * time.Second}

	policies := []routepolicy.RoutePolicy{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "ns-policy", Namespace: "default"},
			Spec: routepolicy.RoutePolicySpec{
				Default: &routepolicy.RoutePolicyDefault{
					Timeout: &routepolicy.TimeoutConfig{
						Request: &requestDur,
					},
				},
			},
		},
	}

	httpRoutes := []ir.HTTPRoute{
		{Name: "route1", Namespace: "default"},
		{Name: "route2", Namespace: "default"},
		{Name: "route3", Namespace: "other"},
	}

	result := buildRoutePolicyIndexes(policies, httpRoutes, nil)
	if len(result) != 2 {
		t.Fatalf("expected 2 route configs, got %d", len(result))
	}

	cfg, ok := result["default/route1"]
	if !ok {
		t.Fatal("expected route1 in result")
	}
	if cfg.Timeout == nil || cfg.Timeout.Request != 5*time.Second {
		t.Fatalf("unexpected timeout for route1: %#v", cfg.Timeout)
	}

	cfg, ok = result["default/route2"]
	if !ok {
		t.Fatal("expected route2 in result")
	}
	if cfg.Timeout == nil || cfg.Timeout.Request != 5*time.Second {
		t.Fatalf("unexpected timeout for route2: %#v", cfg.Timeout)
	}

	if _, ok := result["other/route3"]; ok {
		t.Fatal("route3 should not have policy from different namespace")
	}
}

func TestBuildRoutePolicyIndexes_RouteLevel_TargetsSpecificRoute(t *testing.T) {
	requestDur := metav1.Duration{Duration: 8 * time.Second}

	policies := []routepolicy.RoutePolicy{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "route-policy", Namespace: "default"},
			Spec: routepolicy.RoutePolicySpec{
				TargetRefs: []gatewayv1.LocalPolicyTargetReference{
					{Kind: "HTTPRoute", Name: "route1"},
				},
				Default: &routepolicy.RoutePolicyDefault{
					Timeout: &routepolicy.TimeoutConfig{
						Request: &requestDur,
					},
				},
			},
		},
	}

	httpRoutes := []ir.HTTPRoute{
		{Name: "route1", Namespace: "default"},
		{Name: "route2", Namespace: "default"},
	}

	result := buildRoutePolicyIndexes(policies, httpRoutes, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 route config, got %d", len(result))
	}

	cfg, ok := result["default/route1"]
	if !ok {
		t.Fatal("expected route1 in result")
	}
	if cfg.Timeout == nil || cfg.Timeout.Request != 8*time.Second {
		t.Fatalf("unexpected timeout for route1: %#v", cfg.Timeout)
	}

	if _, ok := result["default/route2"]; ok {
		t.Fatal("route2 should not have the route-level policy")
	}
}

func TestBuildRoutePolicyIndexes_GatewayLevel_ResolvesToRoutes(t *testing.T) {
	requestDur := metav1.Duration{Duration: 12 * time.Second}

	policies := []routepolicy.RoutePolicy{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "gw-policy", Namespace: "default"},
			Spec: routepolicy.RoutePolicySpec{
				TargetRefs: []gatewayv1.LocalPolicyTargetReference{
					{Kind: "Gateway", Name: "my-gw"},
				},
				Default: &routepolicy.RoutePolicyDefault{
					Timeout: &routepolicy.TimeoutConfig{
						Request: &requestDur,
					},
				},
			},
		},
	}

	httpRoutes := []ir.HTTPRoute{
		{
			Name:      "route1",
			Namespace: "default",
			ParentRefs: []ir.ParentRef{
				{Namespace: "default", Name: "my-gw"},
			},
		},
		{
			Name:      "route2",
			Namespace: "default",
			ParentRefs: []ir.ParentRef{
				{Namespace: "default", Name: "other-gw"},
			},
		},
		{
			Name:      "route3",
			Namespace: "default",
			ParentRefs: []ir.ParentRef{
				{Namespace: "default", Name: "my-gw"},
			},
		},
	}

	gateways := []gatewayv1.Gateway{
		{ObjectMeta: metav1.ObjectMeta{Name: "my-gw", Namespace: "default"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "other-gw", Namespace: "default"}},
	}

	result := buildRoutePolicyIndexes(policies, httpRoutes, gateways)
	if len(result) != 2 {
		t.Fatalf("expected 2 route configs, got %d", len(result))
	}

	if _, ok := result["default/route1"]; !ok {
		t.Fatal("expected route1 in result")
	}
	if _, ok := result["default/route3"]; !ok {
		t.Fatal("expected route3 in result")
	}
	if _, ok := result["default/route2"]; ok {
		t.Fatal("route2 should not match (references other-gw, not my-gw)")
	}
}

func TestBuildRoutePolicyIndexes_ThreeLevelInheritance(t *testing.T) {
	nsDur := metav1.Duration{Duration: 5 * time.Second}
	gwDur := metav1.Duration{Duration: 15 * time.Second}
	routeDur := metav1.Duration{Duration: 30 * time.Second}

	gwBody := uint64(1048576)
	routeBody := uint64(2097152)

	nsPolicy := routepolicy.RoutePolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "ns-policy", Namespace: "default"},
		Spec: routepolicy.RoutePolicySpec{
			Default: &routepolicy.RoutePolicyDefault{
				Timeout: &routepolicy.TimeoutConfig{Request: &nsDur},
			},
		},
	}
	gwPolicy := routepolicy.RoutePolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "gw-policy", Namespace: "default"},
		Spec: routepolicy.RoutePolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReference{
				{Kind: "Gateway", Name: "my-gw"},
			},
			Default: &routepolicy.RoutePolicyDefault{
				Timeout: &routepolicy.TimeoutConfig{Request: &gwDur},
				BodyLimit: &routepolicy.BodyLimitConfig{
					MaxRequestBodyBytes: &gwBody,
				},
			},
		},
	}
	routePolicy := routepolicy.RoutePolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "route-policy", Namespace: "default"},
		Spec: routepolicy.RoutePolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReference{
				{Kind: "HTTPRoute", Name: "route1"},
			},
			Default: &routepolicy.RoutePolicyDefault{
				Timeout: &routepolicy.TimeoutConfig{BackendRequest: &routeDur},
				BodyLimit: &routepolicy.BodyLimitConfig{
					MaxRequestBodyBytes: &routeBody,
				},
			},
		},
	}

	policies := []routepolicy.RoutePolicy{nsPolicy, gwPolicy, routePolicy}

	httpRoutes := []ir.HTTPRoute{
		{
			Name:      "route1",
			Namespace: "default",
			ParentRefs: []ir.ParentRef{
				{Namespace: "default", Name: "my-gw"},
			},
		},
	}

	gateways := []gatewayv1.Gateway{
		{ObjectMeta: metav1.ObjectMeta{Name: "my-gw", Namespace: "default"}},
	}

	result := buildRoutePolicyIndexes(policies, httpRoutes, gateways)
	if len(result) != 1 {
		t.Fatalf("expected 1 route config, got %d", len(result))
	}

	cfg := result["default/route1"]

	if cfg.BodyLimit == nil {
		t.Fatal("expected body limit config")
	}
	if cfg.BodyLimit.MaxRequestBodyBytes != routeBody {
		t.Fatalf("expected route-level body limit %d, got %d", routeBody, cfg.BodyLimit.MaxRequestBodyBytes)
	}

	if cfg.Timeout == nil {
		t.Fatal("expected timeout config")
	}
	if cfg.Timeout.BackendRequest != routeDur.Duration {
		t.Fatalf("expected route-level backend request %v, got %v", routeDur.Duration, cfg.Timeout.BackendRequest)
	}

	if cfg.Timeout.Request != gwDur.Duration {
		t.Fatalf("expected gateway-level request timeout %v, got %v", gwDur.Duration, cfg.Timeout.Request)
	}
}

func TestBuildRoutePolicyIndexes_ConflictDetection_TwoNamespacePolicies(t *testing.T) {
	reqDur1 := metav1.Duration{Duration: 5 * time.Second}
	reqDur2 := metav1.Duration{Duration: 10 * time.Second}

	policies := []routepolicy.RoutePolicy{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "ns-policy-1", Namespace: "default"},
			Spec: routepolicy.RoutePolicySpec{
				Default: &routepolicy.RoutePolicyDefault{
					Timeout: &routepolicy.TimeoutConfig{Request: &reqDur1},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "ns-policy-2", Namespace: "default"},
			Spec: routepolicy.RoutePolicySpec{
				Default: &routepolicy.RoutePolicyDefault{
					Timeout: &routepolicy.TimeoutConfig{Request: &reqDur2},
				},
			},
		},
	}

	httpRoutes := []ir.HTTPRoute{
		{Name: "route1", Namespace: "default"},
	}

	result := buildRoutePolicyIndexes(policies, httpRoutes, nil)
	if len(result) != 0 {
		t.Fatalf("expected 0 route configs (conflict), got %d", len(result))
	}
}

func TestBuildRoutePolicyIndexes_ConflictDetection_TwoRoutePoliciesForSameRoute(t *testing.T) {
	reqDur1 := metav1.Duration{Duration: 5 * time.Second}
	reqDur2 := metav1.Duration{Duration: 10 * time.Second}

	policies := []routepolicy.RoutePolicy{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "route-policy-1", Namespace: "default"},
			Spec: routepolicy.RoutePolicySpec{
				TargetRefs: []gatewayv1.LocalPolicyTargetReference{
					{Kind: "HTTPRoute", Name: "route1"},
				},
				Default: &routepolicy.RoutePolicyDefault{
					Timeout: &routepolicy.TimeoutConfig{Request: &reqDur1},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "route-policy-2", Namespace: "default"},
			Spec: routepolicy.RoutePolicySpec{
				TargetRefs: []gatewayv1.LocalPolicyTargetReference{
					{Kind: "HTTPRoute", Name: "route1"},
				},
				Default: &routepolicy.RoutePolicyDefault{
					Timeout: &routepolicy.TimeoutConfig{Request: &reqDur2},
				},
			},
		},
	}

	httpRoutes := []ir.HTTPRoute{
		{Name: "route1", Namespace: "default"},
	}

	result := buildRoutePolicyIndexes(policies, httpRoutes, nil)
	if len(result) != 0 {
		t.Fatalf("expected 0 route configs (conflict), got %d", len(result))
	}
}

func TestBuildRoutePolicyIndexes_Conflict_NamespaceConflictDoesNotBlockNonConflicting(t *testing.T) {
	nsDur1 := metav1.Duration{Duration: 5 * time.Second}
	nsDur2 := metav1.Duration{Duration: 10 * time.Second}
	routeDur := metav1.Duration{Duration: 20 * time.Second}

	policies := []routepolicy.RoutePolicy{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "ns-policy-1", Namespace: "default"},
			Spec: routepolicy.RoutePolicySpec{
				Default: &routepolicy.RoutePolicyDefault{
					Timeout: &routepolicy.TimeoutConfig{Request: &nsDur1},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "ns-policy-2", Namespace: "default"},
			Spec: routepolicy.RoutePolicySpec{
				Default: &routepolicy.RoutePolicyDefault{
					Timeout: &routepolicy.TimeoutConfig{Request: &nsDur2},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "route-policy", Namespace: "default"},
			Spec: routepolicy.RoutePolicySpec{
				TargetRefs: []gatewayv1.LocalPolicyTargetReference{
					{Kind: "HTTPRoute", Name: "route1"},
				},
				Default: &routepolicy.RoutePolicyDefault{
					Timeout: &routepolicy.TimeoutConfig{Request: &routeDur},
				},
			},
		},
	}

	httpRoutes := []ir.HTTPRoute{
		{Name: "route1", Namespace: "default"},
	}

	result := buildRoutePolicyIndexes(policies, httpRoutes, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 route config, got %d", len(result))
	}
	cfg := result["default/route1"]
	if cfg.Timeout == nil || cfg.Timeout.Request != routeDur.Duration {
		t.Fatalf("expected route-level timeout %v, got %#v", routeDur.Duration, cfg.Timeout)
	}
}

func TestBuildRoutePolicyIndexes_EmptyPolicies(t *testing.T) {
	httpRoutes := []ir.HTTPRoute{
		{Name: "route1", Namespace: "default"},
	}
	result := buildRoutePolicyIndexes(nil, httpRoutes, nil)
	if len(result) != 0 {
		t.Fatalf("expected empty result, got %d", len(result))
	}
}

func TestMergeRoutePolicyConfig_ChildWins(t *testing.T) {
	parent := &ir.RoutePolicyConfig{
		Timeout: &ir.RouteTimeoutConfig{Request: 10 * time.Second},
	}
	child := &ir.RoutePolicyConfig{
		Timeout: &ir.RouteTimeoutConfig{Request: 5 * time.Second, Connect: 2 * time.Second},
	}

	merged := mergeRoutePolicyConfig(parent, child)
	if merged.Timeout == nil {
		t.Fatal("expected timeout config")
	}
	if merged.Timeout.Request != 5*time.Second {
		t.Fatalf("expected child timeout 5s, got %v", merged.Timeout.Request)
	}
	if merged.Timeout.Connect != 2*time.Second {
		t.Fatalf("expected child connect 2s, got %v", merged.Timeout.Connect)
	}
}

func TestMergeRoutePolicyConfig_ParentFallback(t *testing.T) {
	parent := &ir.RoutePolicyConfig{
		Timeout:   &ir.RouteTimeoutConfig{Request: 10 * time.Second},
		BodyLimit: &ir.RouteBodyLimitConfig{MaxRequestBodyBytes: 1000},
	}
	child := &ir.RoutePolicyConfig{
		Proxy: &ir.RouteProxyConfig{RequestBuffering: true},
	}

	merged := mergeRoutePolicyConfig(parent, child)
	if merged.Timeout == nil || merged.Timeout.Request != 10*time.Second {
		t.Fatal("expected parent timeout to fall through")
	}
	if merged.BodyLimit == nil || merged.BodyLimit.MaxRequestBodyBytes != 1000 {
		t.Fatal("expected parent body limit to fall through")
	}
	if merged.Proxy == nil || !merged.Proxy.RequestBuffering {
		t.Fatal("expected child proxy to apply")
	}
}

func TestMergeRoutePolicyConfig_Nils(t *testing.T) {
	parent := &ir.RoutePolicyConfig{
		Timeout: &ir.RouteTimeoutConfig{Request: 10 * time.Second},
	}
	merged := mergeRoutePolicyConfig(parent, nil)
	if merged.Timeout == nil || merged.Timeout.Request != 10*time.Second {
		t.Fatal("expected parent to be returned when child is nil")
	}

	merged = mergeRoutePolicyConfig(nil, parent)
	if merged.Timeout == nil || merged.Timeout.Request != 10*time.Second {
		t.Fatal("expected child to be returned when parent is nil")
	}

	merged = mergeRoutePolicyConfig(nil, nil)
	if merged != nil {
		t.Fatal("expected nil when both are nil")
	}
}

func TestBuildRoutePolicyIndexes_RouteLevelConflict(t *testing.T) {
	reqDur1 := metav1.Duration{Duration: 5 * time.Second}
	reqDur2 := metav1.Duration{Duration: 15 * time.Second}

	policies := []routepolicy.RoutePolicy{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "rp-1", Namespace: "default"},
			Spec: routepolicy.RoutePolicySpec{
				TargetRefs: []gatewayv1.LocalPolicyTargetReference{
					{Kind: "HTTPRoute", Name: "route1"},
				},
				Default: &routepolicy.RoutePolicyDefault{
					Timeout: &routepolicy.TimeoutConfig{Request: &reqDur1},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "rp-2", Namespace: "default"},
			Spec: routepolicy.RoutePolicySpec{
				TargetRefs: []gatewayv1.LocalPolicyTargetReference{
					{Kind: "HTTPRoute", Name: "route1"},
				},
				Default: &routepolicy.RoutePolicyDefault{
					Timeout: &routepolicy.TimeoutConfig{Request: &reqDur2},
				},
			},
		},
	}
	httpRoutes := []ir.HTTPRoute{
		{Name: "route1", Namespace: "default"},
	}
	result := buildRoutePolicyIndexes(policies, httpRoutes, nil)
	if _, ok := result["default/route1"]; ok {
		t.Fatal("route1 should NOT have a policy; conflict between rp-1 and rp-2")
	}
}

func TestBuildRoutePolicyIndexes_GatewayLevelConflict(t *testing.T) {
	reqDur1 := metav1.Duration{Duration: 5 * time.Second}
	reqDur2 := metav1.Duration{Duration: 15 * time.Second}

	policies := []routepolicy.RoutePolicy{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "gw-1", Namespace: "default"},
			Spec: routepolicy.RoutePolicySpec{
				TargetRefs: []gatewayv1.LocalPolicyTargetReference{
					{Kind: "Gateway", Name: "my-gw"},
				},
				Default: &routepolicy.RoutePolicyDefault{
					Timeout: &routepolicy.TimeoutConfig{Request: &reqDur1},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "gw-2", Namespace: "default"},
			Spec: routepolicy.RoutePolicySpec{
				TargetRefs: []gatewayv1.LocalPolicyTargetReference{
					{Kind: "Gateway", Name: "my-gw"},
				},
				Default: &routepolicy.RoutePolicyDefault{
					Timeout: &routepolicy.TimeoutConfig{Request: &reqDur2},
				},
			},
		},
	}
	httpRoutes := []ir.HTTPRoute{
		{
			Name: "route1", Namespace: "default",
			ParentRefs: []ir.ParentRef{{Namespace: "default", Name: "my-gw"}},
		},
	}
	gateways := []gatewayv1.Gateway{
		{ObjectMeta: metav1.ObjectMeta{Name: "my-gw", Namespace: "default"}},
	}
	result := buildRoutePolicyIndexes(policies, httpRoutes, gateways)
	if _, ok := result["default/route1"]; ok {
		t.Fatal("route1 should NOT have a policy; conflict between gw-1 and gw-2")
	}
}

func TestBuildRoutePolicyIndexes_RouteOverridesSingleFieldFromGateway(t *testing.T) {
	gwReq := metav1.Duration{Duration: 15 * time.Second}
	routeConn := metav1.Duration{Duration: 20 * time.Second}
	gwBody := uint64(52428800)

	gwPolicy := routepolicy.RoutePolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "gw-policy", Namespace: "default"},
		Spec: routepolicy.RoutePolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReference{{Kind: "Gateway", Name: "my-gw"}},
			Default: &routepolicy.RoutePolicyDefault{
				Timeout:   &routepolicy.TimeoutConfig{Request: &gwReq},
				BodyLimit: &routepolicy.BodyLimitConfig{MaxRequestBodyBytes: &gwBody},
			},
		},
	}
	routePolicy := routepolicy.RoutePolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "route-policy", Namespace: "default"},
		Spec: routepolicy.RoutePolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReference{{Kind: "HTTPRoute", Name: "route1"}},
			Default: &routepolicy.RoutePolicyDefault{
				Connection: &routepolicy.ConnectionConfig{KeepaliveTimeout: &routeConn},
			},
		},
	}
	policies := []routepolicy.RoutePolicy{gwPolicy, routePolicy}
	httpRoutes := []ir.HTTPRoute{
		{
			Name: "route1", Namespace: "default",
			ParentRefs: []ir.ParentRef{{Namespace: "default", Name: "my-gw"}},
		},
	}
	gateways := []gatewayv1.Gateway{
		{ObjectMeta: metav1.ObjectMeta{Name: "my-gw", Namespace: "default"}},
	}
	result := buildRoutePolicyIndexes(policies, httpRoutes, gateways)
	cfg := result["default/route1"]
	// Route-level connection should be present
	if cfg.Connection == nil || cfg.Connection.KeepaliveTimeout != routeConn.Duration {
		t.Fatalf("expected route-level keepalive timeout %v, got %v", routeConn.Duration, cfg.Connection)
	}
	// Gateway-level timeout should still apply (not overridden)
	if cfg.Timeout == nil || cfg.Timeout.Request != gwReq.Duration {
		t.Fatalf("expected gateway-level request timeout %v, got %v", gwReq.Duration, cfg.Timeout)
	}
	// Gateway-level body limit should still apply
	if cfg.BodyLimit == nil || cfg.BodyLimit.MaxRequestBodyBytes != gwBody {
		t.Fatalf("expected gateway-level body limit %d, got %v", gwBody, cfg.BodyLimit)
	}
}

func TestTranslateRoutePolicyDefault_PartialBodyLimit(t *testing.T) {
	maxBody := uint64(1048576)
	spec := &routepolicy.RoutePolicyDefault{
		BodyLimit: &routepolicy.BodyLimitConfig{
			MaxRequestBodyBytes: &maxBody,
			// RequestBodyBufferBytes and MaxRequestHeaderBytes NOT set
		},
	}
	result := translateRoutePolicyDefault(spec)
	if result == nil || result.BodyLimit == nil {
		t.Fatal("expected body limit config")
	}
	if result.BodyLimit.MaxRequestBodyBytes != 1048576 {
		t.Fatalf("unexpected max request body: %d", result.BodyLimit.MaxRequestBodyBytes)
	}
	if result.BodyLimit.RequestBodyBufferBytes != 0 {
		t.Fatalf("expected zero request body buffer bytes, got %d", result.BodyLimit.RequestBodyBufferBytes)
	}
	if result.BodyLimit.MaxRequestHeaderBytes != 0 {
		t.Fatalf("expected zero max request header bytes, got %d", result.BodyLimit.MaxRequestHeaderBytes)
	}
}

func TestTranslateRoutePolicyDefault_ProxyExplicitFalse(t *testing.T) {
	reqBuf := false
	respBuf := false
	bufSize := uint64(0)
	bufCount := uint32(0)

	spec := &routepolicy.RoutePolicyDefault{
		Proxy: &routepolicy.ProxyConfig{
			RequestBuffering:  &reqBuf,
			ResponseBuffering: &respBuf,
			BufferSize:        &bufSize,
			BufferCount:       &bufCount,
		},
	}
	result := translateRoutePolicyDefault(spec)
	if result == nil || result.Proxy == nil {
		t.Fatal("expected proxy config even when all values are explicit false/zero")
	}
	if result.Proxy.RequestBuffering != false {
		t.Fatal("expected request buffering false")
	}
	if result.Proxy.ResponseBuffering != false {
		t.Fatal("expected response buffering false")
	}
}

func TestBuildRoutePolicyIndexes_NamespacePolicyDoesNotCrossNamespace(t *testing.T) {
	reqDur := metav1.Duration{Duration: 5 * time.Second}
	policies := []routepolicy.RoutePolicy{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "ns-policy", Namespace: "team-a"},
			Spec: routepolicy.RoutePolicySpec{
				Default: &routepolicy.RoutePolicyDefault{
					Timeout: &routepolicy.TimeoutConfig{Request: &reqDur},
				},
			},
		},
	}
	httpRoutes := []ir.HTTPRoute{
		{Name: "route1", Namespace: "team-b"},
	}
	result := buildRoutePolicyIndexes(policies, httpRoutes, nil)
	if _, ok := result["team-b/route1"]; ok {
		t.Fatal("route in team-b should not inherit namespace policy from team-a")
	}
}
