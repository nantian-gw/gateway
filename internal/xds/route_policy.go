package xds

import (
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"

	"github.com/nantian-gw/gateway/internal/ir"
)

func toProtoRoutePolicy(item *ir.RoutePolicyConfig) *controlv1.RoutePolicy {
	if item == nil {
		return nil
	}

	timeout := toProtoRoutePolicyTimeout(item.Timeout)
	bodyLimit := toProtoRoutePolicyBodyLimit(item.BodyLimit)
	proxy := toProtoRoutePolicyProxy(item.Proxy)
	connection := toProtoRoutePolicyConnection(item.Connection)

	if timeout == nil && bodyLimit == nil && proxy == nil && connection == nil {
		return nil
	}

	return &controlv1.RoutePolicy{
		Timeout:    timeout,
		BodyLimit:  bodyLimit,
		Proxy:      proxy,
		Connection: connection,
	}
}

func toProtoRoutePolicyTimeout(item *ir.RouteTimeoutConfig) *controlv1.RoutePolicyTimeout {
	if item == nil {
		return nil
	}

	out := &controlv1.RoutePolicyTimeout{}
	anySet := false

	if item.Request != 0 {
		out.Request = durationpb.New(item.Request)
		anySet = true
	}
	if item.BackendRequest != 0 {
		out.BackendRequest = durationpb.New(item.BackendRequest)
		anySet = true
	}
	if item.Connect != 0 {
		out.Connect = durationpb.New(item.Connect)
		anySet = true
	}
	if item.NextUpstream != 0 {
		out.NextUpstream = durationpb.New(item.NextUpstream)
		anySet = true
	}

	if !anySet {
		return nil
	}

	return out
}

func toProtoRoutePolicyBodyLimit(item *ir.RouteBodyLimitConfig) *controlv1.RoutePolicyBodyLimit {
	if item == nil {
		return nil
	}

	if item.MaxRequestBodyBytes == 0 &&
		item.RequestBodyBufferBytes == 0 &&
		item.MaxRequestHeaderBytes == 0 {
		return nil
	}

	return &controlv1.RoutePolicyBodyLimit{
		MaxRequestBodyBytes:    item.MaxRequestBodyBytes,
		RequestBodyBufferBytes: item.RequestBodyBufferBytes,
		MaxRequestHeaderBytes:  item.MaxRequestHeaderBytes,
	}
}

func toProtoRoutePolicyProxy(item *ir.RouteProxyConfig) *controlv1.RoutePolicyProxy {
	if item == nil {
		return nil
	}

	out := &controlv1.RoutePolicyProxy{
		BufferSize:  item.BufferSize,
		BufferCount: item.BufferCount,
	}
	anySet := false

	if item.RequestBuffering {
		out.RequestBuffering = wrapperspb.Bool(true)
		anySet = true
	}
	if item.ResponseBuffering {
		out.ResponseBuffering = wrapperspb.Bool(true)
		anySet = true
	}
	if item.BufferSize != 0 {
		anySet = true
	}
	if item.BufferCount != 0 {
		anySet = true
	}

	if !anySet {
		return nil
	}

	return out
}

func toProtoRoutePolicyConnection(item *ir.RouteConnectionConfig) *controlv1.RoutePolicyConnection {
	if item == nil {
		return nil
	}

	out := &controlv1.RoutePolicyConnection{
		KeepaliveRequests:         item.KeepaliveRequests,
		UpstreamKeepalivePoolSize: item.UpstreamKeepalivePoolSize,
	}
	anySet := item.KeepaliveRequests != 0

	if item.UpstreamKeepalivePoolSize != 0 {
		anySet = true
	}
	if item.KeepaliveTime != 0 {
		out.KeepaliveTime = durationpb.New(item.KeepaliveTime)
		anySet = true
	}
	if item.KeepaliveTimeout != 0 {
		out.KeepaliveTimeout = durationpb.New(item.KeepaliveTimeout)
		anySet = true
	}
	if item.UpstreamKeepaliveIdle != 0 {
		out.UpstreamKeepaliveIdle = durationpb.New(item.UpstreamKeepaliveIdle)
		anySet = true
	}

	if !anySet {
		return nil
	}

	return out
}
