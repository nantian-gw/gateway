package xds

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"
	"github.com/nantian-gw/gateway/internal/ir"
)

func TestToProtoRoutePolicy_FullConfig(t *testing.T) {
	config := &ir.RoutePolicyConfig{
		Timeout: &ir.RouteTimeoutConfig{
			Request:        30 * time.Second,
			BackendRequest: 25 * time.Second,
			Connect:        5 * time.Second,
			NextUpstream:   15 * time.Second,
		},
		BodyLimit: &ir.RouteBodyLimitConfig{
			MaxRequestBodyBytes:    1048576,
			RequestBodyBufferBytes: 65536,
			MaxRequestHeaderBytes:  16384,
		},
		Proxy: &ir.RouteProxyConfig{
			RequestBuffering:  true,
			ResponseBuffering: true,
			BufferSize:        4096,
			BufferCount:       4,
		},
		Connection: &ir.RouteConnectionConfig{
			KeepaliveRequests:         100,
			KeepaliveTime:             10 * time.Second,
			KeepaliveTimeout:          5 * time.Second,
			UpstreamKeepalivePoolSize: 10,
			UpstreamKeepaliveIdle:     60 * time.Second,
		},
	}

	result := toProtoRoutePolicy(config)
	if result == nil {
		t.Fatal("expected non-nil result for full config")
	}

	if result.Timeout == nil {
		t.Fatal("expected timeout to be non-nil")
	}
	if d := result.Timeout.Request; d == nil || d.AsDuration() != 30*time.Second {
		t.Errorf("expected timeout.request=30s, got %v", d)
	}
	if d := result.Timeout.BackendRequest; d == nil || d.AsDuration() != 25*time.Second {
		t.Errorf("expected timeout.backend_request=25s, got %v", d)
	}
	if d := result.Timeout.Connect; d == nil || d.AsDuration() != 5*time.Second {
		t.Errorf("expected timeout.connect=5s, got %v", d)
	}
	if d := result.Timeout.NextUpstream; d == nil || d.AsDuration() != 15*time.Second {
		t.Errorf("expected timeout.next_upstream=15s, got %v", d)
	}

	if result.BodyLimit == nil {
		t.Fatal("expected body_limit to be non-nil")
	}
	if result.BodyLimit.MaxRequestBodyBytes != 1048576 {
		t.Errorf("expected body_limit.max_request_body_bytes=1048576, got %d", result.BodyLimit.MaxRequestBodyBytes)
	}
	if result.BodyLimit.RequestBodyBufferBytes != 65536 {
		t.Errorf("expected body_limit.request_body_buffer_bytes=65536, got %d", result.BodyLimit.RequestBodyBufferBytes)
	}
	if result.BodyLimit.MaxRequestHeaderBytes != 16384 {
		t.Errorf("expected body_limit.max_request_header_bytes=16384, got %d", result.BodyLimit.MaxRequestHeaderBytes)
	}

	if result.Proxy == nil {
		t.Fatal("expected proxy to be non-nil")
	}
	if v := result.Proxy.RequestBuffering; v == nil || !v.GetValue() {
		t.Errorf("expected proxy.request_buffering=true, got %v", v)
	}
	if v := result.Proxy.ResponseBuffering; v == nil || !v.GetValue() {
		t.Errorf("expected proxy.response_buffering=true, got %v", v)
	}
	if result.Proxy.BufferSize != 4096 {
		t.Errorf("expected proxy.buffer_size=4096, got %d", result.Proxy.BufferSize)
	}
	if result.Proxy.BufferCount != 4 {
		t.Errorf("expected proxy.buffer_count=4, got %d", result.Proxy.BufferCount)
	}

	if result.Connection == nil {
		t.Fatal("expected connection to be non-nil")
	}
	if result.Connection.KeepaliveRequests != 100 {
		t.Errorf("expected connection.keepalive_requests=100, got %d", result.Connection.KeepaliveRequests)
	}
	if d := result.Connection.KeepaliveTime; d == nil || d.AsDuration() != 10*time.Second {
		t.Errorf("expected connection.keepalive_time=10s, got %v", d)
	}
	if d := result.Connection.KeepaliveTimeout; d == nil || d.AsDuration() != 5*time.Second {
		t.Errorf("expected connection.keepalive_timeout=5s, got %v", d)
	}
	if result.Connection.UpstreamKeepalivePoolSize != 10 {
		t.Errorf("expected connection.upstream_keepalive_pool_size=10, got %d", result.Connection.UpstreamKeepalivePoolSize)
	}
	if d := result.Connection.UpstreamKeepaliveIdle; d == nil || d.AsDuration() != 60*time.Second {
		t.Errorf("expected connection.upstream_keepalive_idle=60s, got %v", d)
	}
}

func TestToProtoRoutePolicy_NilInput(t *testing.T) {
	result := toProtoRoutePolicy(nil)
	if result != nil {
		t.Errorf("expected nil for nil input, got %v", result)
	}
}

func TestToProtoRoutePolicy_ZeroValue(t *testing.T) {
	config := &ir.RoutePolicyConfig{
		Timeout:    &ir.RouteTimeoutConfig{},
		BodyLimit:  &ir.RouteBodyLimitConfig{},
		Proxy:      &ir.RouteProxyConfig{},
		Connection: &ir.RouteConnectionConfig{},
	}

	result := toProtoRoutePolicy(config)
	if result != nil {
		t.Errorf("expected nil for all-zero config, got %v", result)
	}
}

func TestToProtoRoutePolicy_PartialTimeoutOnly(t *testing.T) {
	config := &ir.RoutePolicyConfig{
		Timeout: &ir.RouteTimeoutConfig{
			Request:    10 * time.Second,
			Connect:    3 * time.Second,
		},
	}

	result := toProtoRoutePolicy(config)
	if result == nil {
		t.Fatal("expected non-nil for partial config")
	}

	if result.Timeout == nil {
		t.Fatal("expected timeout to be non-nil")
	}
	if d := result.Timeout.Request; d == nil || d.AsDuration() != 10*time.Second {
		t.Errorf("expected timeout.request=10s, got %v", d)
	}
	if result.Timeout.BackendRequest != nil {
		t.Errorf("expected timeout.backend_request to be nil, got %v", result.Timeout.BackendRequest)
	}
	if d := result.Timeout.Connect; d == nil || d.AsDuration() != 3*time.Second {
		t.Errorf("expected timeout.connect=3s, got %v", d)
	}
	if result.Timeout.NextUpstream != nil {
		t.Errorf("expected timeout.next_upstream to be nil, got %v", result.Timeout.NextUpstream)
	}

	if result.BodyLimit != nil {
		t.Errorf("expected body_limit to be nil, got %v", result.BodyLimit)
	}
	if result.Proxy != nil {
		t.Errorf("expected proxy to be nil, got %v", result.Proxy)
	}
	if result.Connection != nil {
		t.Errorf("expected connection to be nil, got %v", result.Connection)
	}
}

func TestToProtoRoutePolicy_RoundTripSemantics(t *testing.T) {
	original := &ir.RoutePolicyConfig{
		Timeout: &ir.RouteTimeoutConfig{
			Request:    60 * time.Second,
			BackendRequest: 45 * time.Second,
			Connect:    10 * time.Second,
			NextUpstream:   30 * time.Second,
		},
		BodyLimit: &ir.RouteBodyLimitConfig{
			MaxRequestBodyBytes:    2097152,
			MaxRequestHeaderBytes:  32768,
		},
		Proxy: &ir.RouteProxyConfig{
			RequestBuffering:  false,
			ResponseBuffering: true,
			BufferSize:        8192,
			BufferCount:       8,
		},
		Connection: &ir.RouteConnectionConfig{
			KeepaliveTime:     30 * time.Second,
			KeepaliveTimeout:  10 * time.Second,
		},
	}

	result := toProtoRoutePolicy(original)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	expected := &controlv1.RoutePolicy{
		Timeout: &controlv1.RoutePolicyTimeout{
			Request:        durationpb.New(60 * time.Second),
			BackendRequest: durationpb.New(45 * time.Second),
			Connect:        durationpb.New(10 * time.Second),
			NextUpstream:   durationpb.New(30 * time.Second),
		},
		BodyLimit: &controlv1.RoutePolicyBodyLimit{
			MaxRequestBodyBytes:    2097152,
			MaxRequestHeaderBytes:  32768,
		},
		Proxy: &controlv1.RoutePolicyProxy{
			ResponseBuffering: wrapperspb.Bool(true),
			BufferSize:        8192,
			BufferCount:       8,
		},
		Connection: &controlv1.RoutePolicyConnection{
			KeepaliveTime:    durationpb.New(30 * time.Second),
			KeepaliveTimeout: durationpb.New(10 * time.Second),
		},
	}

	checkRoutePolicyEqual(t, expected, result)
}

func checkRoutePolicyEqual(t *testing.T, expected, actual *controlv1.RoutePolicy) {
	t.Helper()

	if (expected == nil) != (actual == nil) {
		t.Fatalf("nil mismatch: expected=%v, actual=%v", expected != nil, actual != nil)
	}
	if expected == nil {
		return
	}

	checkRoutePolicyTimeoutEqual(t, expected.Timeout, actual.Timeout)
	checkRoutePolicyBodyLimitEqual(t, expected.BodyLimit, actual.BodyLimit)
	checkRoutePolicyProxyEqual(t, expected.Proxy, actual.Proxy)
	checkRoutePolicyConnectionEqual(t, expected.Connection, actual.Connection)
}

func checkRoutePolicyTimeoutEqual(t *testing.T, expected, actual *controlv1.RoutePolicyTimeout) {
	t.Helper()
	if (expected == nil) != (actual == nil) {
		t.Fatalf("timeout nil mismatch: expected=%v, actual=%v", expected != nil, actual != nil)
	}
	if expected == nil {
		return
	}
	if e, a := expected.Request.AsDuration(), actual.Request.AsDuration(); e != a {
		t.Errorf("timeout.request: expected=%v, actual=%v", e, a)
	}
	if e, a := expected.BackendRequest.AsDuration(), actual.BackendRequest.AsDuration(); e != a {
		t.Errorf("timeout.backend_request: expected=%v, actual=%v", e, a)
	}
	if e, a := expected.Connect.AsDuration(), actual.Connect.AsDuration(); e != a {
		t.Errorf("timeout.connect: expected=%v, actual=%v", e, a)
	}
	if e, a := expected.NextUpstream.AsDuration(), actual.NextUpstream.AsDuration(); e != a {
		t.Errorf("timeout.next_upstream: expected=%v, actual=%v", e, a)
	}
}

func checkRoutePolicyBodyLimitEqual(t *testing.T, expected, actual *controlv1.RoutePolicyBodyLimit) {
	t.Helper()
	if (expected == nil) != (actual == nil) {
		t.Fatalf("body_limit nil mismatch: expected=%v, actual=%v", expected != nil, actual != nil)
	}
	if expected == nil {
		return
	}
	if expected.MaxRequestBodyBytes != actual.MaxRequestBodyBytes {
		t.Errorf("body_limit.max_request_body_bytes: expected=%d, actual=%d", expected.MaxRequestBodyBytes, actual.MaxRequestBodyBytes)
	}
	if expected.RequestBodyBufferBytes != actual.RequestBodyBufferBytes {
		t.Errorf("body_limit.request_body_buffer_bytes: expected=%d, actual=%d", expected.RequestBodyBufferBytes, actual.RequestBodyBufferBytes)
	}
	if expected.MaxRequestHeaderBytes != actual.MaxRequestHeaderBytes {
		t.Errorf("body_limit.max_request_header_bytes: expected=%d, actual=%d", expected.MaxRequestHeaderBytes, actual.MaxRequestHeaderBytes)
	}
}

func checkRoutePolicyProxyEqual(t *testing.T, expected, actual *controlv1.RoutePolicyProxy) {
	t.Helper()
	if (expected == nil) != (actual == nil) {
		t.Fatalf("proxy nil mismatch: expected=%v, actual=%v", expected != nil, actual != nil)
	}
	if expected == nil {
		return
	}
	if e, a := expected.RequestBuffering, actual.RequestBuffering; (e == nil) != (a == nil) {
		t.Fatalf("proxy.request_buffering nil mismatch: expected=%v, actual=%v", e != nil, a != nil)
	}
	if expected.RequestBuffering != nil && expected.RequestBuffering.GetValue() != actual.RequestBuffering.GetValue() {
		t.Errorf("proxy.request_buffering: expected=%v, actual=%v", expected.RequestBuffering.GetValue(), actual.RequestBuffering.GetValue())
	}
	if e, a := expected.ResponseBuffering, actual.ResponseBuffering; (e == nil) != (a == nil) {
		t.Fatalf("proxy.response_buffering nil mismatch: expected=%v, actual=%v", e != nil, a != nil)
	}
	if expected.ResponseBuffering != nil && expected.ResponseBuffering.GetValue() != actual.ResponseBuffering.GetValue() {
		t.Errorf("proxy.response_buffering: expected=%v, actual=%v", expected.ResponseBuffering.GetValue(), actual.ResponseBuffering.GetValue())
	}
	if expected.BufferSize != actual.BufferSize {
		t.Errorf("proxy.buffer_size: expected=%d, actual=%d", expected.BufferSize, actual.BufferSize)
	}
	if expected.BufferCount != actual.BufferCount {
		t.Errorf("proxy.buffer_count: expected=%d, actual=%d", expected.BufferCount, actual.BufferCount)
	}
}

func checkRoutePolicyConnectionEqual(t *testing.T, expected, actual *controlv1.RoutePolicyConnection) {
	t.Helper()
	if (expected == nil) != (actual == nil) {
		t.Fatalf("connection nil mismatch: expected=%v, actual=%v", expected != nil, actual != nil)
	}
	if expected == nil {
		return
	}
	if expected.KeepaliveRequests != actual.KeepaliveRequests {
		t.Errorf("connection.keepalive_requests: expected=%d, actual=%d", expected.KeepaliveRequests, actual.KeepaliveRequests)
	}
	if e, a := expected.KeepaliveTime, actual.KeepaliveTime; (e == nil) != (a == nil) {
		t.Fatalf("connection.keepalive_time nil mismatch: expected=%v, actual=%v", e != nil, a != nil)
	}
	if expected.KeepaliveTime != nil && expected.KeepaliveTime.AsDuration() != actual.KeepaliveTime.AsDuration() {
		t.Errorf("connection.keepalive_time: expected=%v, actual=%v", expected.KeepaliveTime.AsDuration(), actual.KeepaliveTime.AsDuration())
	}
	if e, a := expected.KeepaliveTimeout, actual.KeepaliveTimeout; (e == nil) != (a == nil) {
		t.Fatalf("connection.keepalive_timeout nil mismatch: expected=%v, actual=%v", e != nil, a != nil)
	}
	if expected.KeepaliveTimeout != nil && expected.KeepaliveTimeout.AsDuration() != actual.KeepaliveTimeout.AsDuration() {
		t.Errorf("connection.keepalive_timeout: expected=%v, actual=%v", expected.KeepaliveTimeout.AsDuration(), actual.KeepaliveTimeout.AsDuration())
	}
	if expected.UpstreamKeepalivePoolSize != actual.UpstreamKeepalivePoolSize {
		t.Errorf("connection.upstream_keepalive_pool_size: expected=%d, actual=%d", expected.UpstreamKeepalivePoolSize, actual.UpstreamKeepalivePoolSize)
	}
	if e, a := expected.UpstreamKeepaliveIdle, actual.UpstreamKeepaliveIdle; (e == nil) != (a == nil) {
		t.Fatalf("connection.upstream_keepalive_idle nil mismatch: expected=%v, actual=%v", e != nil, a != nil)
	}
	if expected.UpstreamKeepaliveIdle != nil && expected.UpstreamKeepaliveIdle.AsDuration() != actual.UpstreamKeepaliveIdle.AsDuration() {
		t.Errorf("connection.upstream_keepalive_idle: expected=%v, actual=%v", expected.UpstreamKeepaliveIdle.AsDuration(), actual.UpstreamKeepaliveIdle.AsDuration())
	}
}

func TestToProtoRoutePolicy_BodyLimitOnly(t *testing.T) {
	config := &ir.RoutePolicyConfig{
		BodyLimit: &ir.RouteBodyLimitConfig{
			MaxRequestBodyBytes: 52428800,
		},
	}
	result := toProtoRoutePolicy(config)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result.BodyLimit == nil {
		t.Fatal("expected body_limit")
	}
	if result.BodyLimit.MaxRequestBodyBytes != 52428800 {
		t.Errorf("expected 52428800, got %d", result.BodyLimit.MaxRequestBodyBytes)
	}
	if result.BodyLimit.RequestBodyBufferBytes != 0 {
		t.Errorf("expected 0 request_body_buffer_bytes, got %d", result.BodyLimit.RequestBodyBufferBytes)
	}
	if result.Timeout != nil {
		t.Error("expected timeout nil")
	}
	if result.Proxy != nil {
		t.Error("expected proxy nil")
	}
	if result.Connection != nil {
		t.Error("expected connection nil")
	}
}

func TestToProtoRoutePolicy_ConnectionOnly(t *testing.T) {
	config := &ir.RoutePolicyConfig{
		Connection: &ir.RouteConnectionConfig{
			KeepaliveRequests:         200,
			UpstreamKeepalivePoolSize: 50,
		},
	}
	result := toProtoRoutePolicy(config)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result.Connection == nil {
		t.Fatal("expected connection")
	}
	if result.Connection.KeepaliveRequests != 200 {
		t.Errorf("expected 200, got %d", result.Connection.KeepaliveRequests)
	}
	if result.Connection.UpstreamKeepalivePoolSize != 50 {
		t.Errorf("expected 50, got %d", result.Connection.UpstreamKeepalivePoolSize)
	}
	if result.Connection.KeepaliveTime != nil {
		t.Error("expected keepalive_time nil")
	}
	if result.Timeout != nil {
		t.Error("expected timeout nil")
	}
	if result.BodyLimit != nil {
		t.Error("expected body_limit nil")
	}
}

func TestToProtoRoutePolicy_ProxyBothFalse(t *testing.T) {
	config := &ir.RoutePolicyConfig{
		Proxy: &ir.RouteProxyConfig{
			RequestBuffering:  false,
			ResponseBuffering: false,
			BufferSize:        8192,
			BufferCount:       4,
		},
	}
	result := toProtoRoutePolicy(config)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result.Proxy == nil {
		t.Fatal("expected proxy")
	}
	if result.Proxy.RequestBuffering != nil {
		t.Errorf("expected request_buffering nil (false not emitted), got %v", result.Proxy.RequestBuffering)
	}
	if result.Proxy.ResponseBuffering != nil {
		t.Errorf("expected response_buffering nil (false not emitted), got %v", result.Proxy.ResponseBuffering)
	}
	if result.Proxy.BufferSize != 8192 {
		t.Errorf("expected buffer_size=8192, got %d", result.Proxy.BufferSize)
	}
	if result.Proxy.BufferCount != 4 {
		t.Errorf("expected buffer_count=4, got %d", result.Proxy.BufferCount)
	}
}
