package xds

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"

	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/constants"
)

var newStructPB = structpb.NewStruct

func toProtoSnapshot(snapshot *ir.Snapshot) *controlv1.ConfigSnapshot {
	return toProtoSnapshotWithLogger(snapshot, nil)
}

func toProtoSnapshotWithLogger(snapshot *ir.Snapshot, logger *slog.Logger) *controlv1.ConfigSnapshot {
	if snapshot == nil {
		return &controlv1.ConfigSnapshot{}
	}
	logger = snapshotProtoLogger(logger)

	out := &controlv1.ConfigSnapshot{
		Id:          snapshot.ID,
		GeneratedAt: timestamppb.New(snapshot.GeneratedAt),
		Extensions:  snapshotExtensionsWithLogger(snapshot, logger),
	}

	for _, item := range snapshot.Listeners {
		out.Listeners = append(out.Listeners, &controlv1.Listener{
			Name:           item.Name,
			Address:        item.Address,
			Addresses:      item.Addresses,
			Port:           item.Port,
			Protocol:       toListenerProtocol(item.Protocol),
			Hostnames:      item.Hostnames,
			AttachedRoutes: item.AttachedRoutes,
			Tls:            toProtoTLS(item.TLS),
			Metadata:       item.Metadata,
			BackendTls:     toProtoBackendTLS(item.BackendTLS),
		})
	}

	for _, item := range snapshot.HTTPRoutes {
		route := &controlv1.HttpRoute{
			Name:        item.Name,
			Namespace:   item.Namespace,
			Hostnames:   item.Hostnames,
			ParentRefs:  toProtoParents(item.ParentRefs),
			Labels:      item.Labels,
			Annotations: item.Annotations,
			RoutePolicy: toProtoRoutePolicy(item.RoutePolicy),
		}

		for _, rule := range item.Rules {
			protoRule := &controlv1.HttpRule{
				Name:               rule.Name,
				Filters:            toProtoFiltersWithLogger(rule.Filters, logger),
				BackendRefs:        toProtoBackendsWithLogger(rule.BackendRefs, logger),
				Timeouts:           toProtoRouteTimeouts(rule.Timeouts),
				Retry:              toProtoRetryPolicy(rule.Retry),
				SessionPersistence: toProtoSessionPersistence(rule.SessionPersistence),
			}
			for _, match := range rule.Matches {
				protoRule.Matches = append(protoRule.Matches, &controlv1.HttpMatch{
					Path:        match.Path,
					PathType:    match.PathType,
					Method:      match.Method,
					Headers:     toProtoHeaders(match.Headers),
					QueryParams: toProtoQueries(match.QueryParams),
				})
			}
			route.Rules = append(route.Rules, protoRule)
		}

		out.HttpRoutes = append(out.HttpRoutes, route)
	}

	for _, item := range snapshot.GRPCRoutes {
		route := &controlv1.GrpcRoute{
			Name:        item.Name,
			Namespace:   item.Namespace,
			Hostnames:   item.Hostnames,
			ParentRefs:  toProtoParents(item.ParentRefs),
			Labels:      item.Labels,
			Annotations: item.Annotations,
			RoutePolicy: toProtoRoutePolicy(item.RoutePolicy),
		}

		for _, rule := range item.Rules {
			protoRule := &controlv1.GrpcRule{
				Name:               rule.Name,
				Filters:            toProtoFiltersWithLogger(rule.Filters, logger),
				BackendRefs:        toProtoBackendsWithLogger(rule.BackendRefs, logger),
				SessionPersistence: toProtoSessionPersistence(rule.SessionPersistence),
			}
			for _, match := range rule.Matches {
				protoRule.Matches = append(protoRule.Matches, &controlv1.GrpcMatch{
					Service:   match.Service,
					Method:    match.Method,
					Headers:   toProtoHeaders(match.Headers),
					MatchType: match.MatchType,
				})
			}
			route.Rules = append(route.Rules, protoRule)
		}

		out.GrpcRoutes = append(out.GrpcRoutes, route)
	}

	for _, item := range snapshot.StreamRoutes {
		route := &controlv1.StreamRoute{
			Name:        item.Name,
			Namespace:   item.Namespace,
			Kind:        toRouteKind(item.Kind),
			ParentRefs:  toProtoParents(item.ParentRefs),
			Labels:      item.Labels,
			Annotations: item.Annotations,
		}

		for _, rule := range item.Rules {
			protoRule := &controlv1.StreamRule{
				Name:        rule.Name,
				BackendRefs: toProtoBackendsWithLogger(rule.BackendRefs, logger),
			}
			for _, match := range rule.Matches {
				protoMatch := &controlv1.StreamMatch{
					Port:        match.Port,
					SniHostname: match.SNIHostname,
				}
				switch match.Mode {
				case ir.TlsRouteModePassthrough:
					protoMatch.Mode = controlv1.TlsRouteMode_TLS_ROUTE_PASSTHROUGH
				case ir.TlsRouteModeTerminate:
					protoMatch.Mode = controlv1.TlsRouteMode_TLS_ROUTE_TERMINATE
				}
				protoRule.Matches = append(protoRule.Matches, protoMatch)
			}
			route.Rules = append(route.Rules, protoRule)
		}

		out.StreamRoutes = append(out.StreamRoutes, route)
	}

	for _, item := range snapshot.Backends {
		cluster := &controlv1.BackendCluster{
			Name:               item.Name,
			Namespace:          item.Namespace,
			Protocol:           item.Protocol,
			ConnectTimeout:     durationpb.New(item.ConnectTimeout),
			RequestTimeout:     nonZeroDurationOrNil(item.RequestTimeout),
			TlsValidation:      toProtoBackendTLSValidation(item.BackendTLSValidation),
			SessionPersistence: toProtoSessionPersistence(item.SessionPersistence),
			LoadBalancing:      toProtoLoadBalancing(item.LoadBalancing),
			CircuitBreaker:     toProtoCircuitBreaker(item.CircuitBreaker),
			Metadata:           item.Metadata,
		}

		for _, endpoint := range item.Endpoints {
			cluster.Endpoints = append(cluster.Endpoints, &controlv1.BackendEndpoint{
				Address: endpoint.Address,
				Port:    endpoint.Port,
				Healthy: endpoint.Healthy,
				Zone:    endpoint.Zone,
			})
		}

		cluster.AiService = toProtoAIService(item.AIService)
		cluster.TokenPolicy = toProtoTokenPolicy(item.TokenPolicy)
		cluster.WasmPlugin = toProtoWasmPlugin(item.WasmPlugin)

		out.Backends = append(out.Backends, cluster)
	}

	for _, item := range snapshot.Secrets {
		// NOTE: SecretMaterial includes private key material (KeyPem).
		// In production, the xDS channel must be encrypted with gRPC TLS
		// (grpcTLS.enabled=true) to protect keys in transit.
		out.Secrets = append(out.Secrets, &controlv1.SecretMaterial{
			Namespace: item.Namespace,
			Name:      item.Name,
			CertPem:   item.CertPEM,
			KeyPem:    item.KeyPEM,
		})
	}

	return out
}

func toProtoAIService(item *ir.AIServiceConfig) *controlv1.AIServiceConfig {
	if item == nil {
		return nil
	}

	return &controlv1.AIServiceConfig{
		Provider: item.Provider,
		Format:   item.Format,
		Model:    item.Model,
		Auth: &controlv1.AIServiceAuthConfig{
			Type:      item.Auth.Type,
			SecretRef: item.Auth.SecretRef,
			Header:    item.Auth.Header,
		},
		Timeout: nonZeroDurationOrNil(item.Timeout),
	}
}

func toProtoTokenPolicy(item *ir.TokenPolicyConfig) *controlv1.TokenPolicyConfig {
	if item == nil {
		return nil
	}

	return &controlv1.TokenPolicyConfig{
		TokensPerMinute:   item.TokensPerMinute,
		TokensPerHour:     item.TokensPerHour,
		RequestsPerMinute: item.RequestsPerMinute,
		Scope:             item.Scope,
		Burst:             item.Burst,
		OnLimit:           item.OnLimit,
	}
}

func toProtoWasmPlugin(item *ir.WasmPluginConfig) *controlv1.WasmPluginConfig {
	if item == nil {
		return nil
	}

	return &controlv1.WasmPluginConfig{
		Name:       item.Name,
		Namespace:  item.Namespace,
		WasmBytes:  append([]byte{}, item.WasmBytes...),
		Sha256:     item.SHA256,
		Hooks:      item.Hooks,
		ConfigJson: item.ConfigJSON,
		SourceUrl:  item.SourceURL,
		Sandbox: &controlv1.WasmSandboxConfig{
			MaxMemoryBytes:     item.Sandbox.MaxMemoryBytes,
			MaxExecutionTimeMs: item.Sandbox.MaxExecutionTimeMs,
			AllowNetwork:       item.Sandbox.AllowNetwork,
			AllowFileSystem:    item.Sandbox.AllowFileSystem,
		},
	}
}

func toProtoCircuitBreaker(item *ir.CircuitBreakerConfig) *controlv1.CircuitBreakerConfig {
	if item == nil {
		return nil
	}
	return &controlv1.CircuitBreakerConfig{
		MaxInflightRequests: uint32(item.MaxInflightRequests), //nolint:gosec // G115: conversion is safe — value validated as non-negative
	}
}

func snapshotExtensionsWithLogger(snapshot *ir.Snapshot, logger *slog.Logger) *structpb.Struct {
	if snapshot == nil || len(snapshot.Workloads) == 0 {
		return nil
	}
	logger = snapshotProtoLogger(logger)

	workloads := make([]any, 0, len(snapshot.Workloads))
	for _, item := range snapshot.Workloads {
		workloads = append(workloads, map[string]any{
			"namespace": item.Namespace,
			"name":      item.Name,
			"ip":        item.IP,
		})
	}

	out, err := newStructPB(map[string]any{
		"workloads": workloads,
	})
	if err != nil {
		logger.Warn("failed to build snapshot extensions struct", "workload_count", len(snapshot.Workloads), "error", err)
		return nil
	}
	return out
}

func toProtoTLS(tls *ir.TLSConfig) *controlv1.TlsConfig {
	if tls == nil {
		return nil
	}

	return &controlv1.TlsConfig{
		Enabled:            tls.Enabled,
		Passthrough:        tls.Passthrough,
		SecretRefs:         tls.SecretRefs,
		SniHosts:           tls.SNIHosts,
		MinVersion:         tls.MinVersion,
		MaxVersion:         tls.MaxVersion,
		FrontendValidation: toProtoFrontendValidation(tls.FrontendValidation),
	}
}

func toProtoRouteTimeouts(timeouts *ir.RouteTimeouts) *controlv1.HttpRouteTimeouts {
	if timeouts == nil {
		return nil
	}

	return &controlv1.HttpRouteTimeouts{
		Request:        durationOrNil(timeouts.Request),
		BackendRequest: durationOrNil(timeouts.BackendRequest),
	}
}

func toProtoParents(refs []ir.ParentRef) []*controlv1.ParentRef {
	out := make([]*controlv1.ParentRef, 0, len(refs))
	for _, ref := range refs {
		out = append(out, &controlv1.ParentRef{
			Group:       ref.Group,
			Kind:        ref.Kind,
			Namespace:   ref.Namespace,
			Name:        ref.Name,
			SectionName: ref.SectionName,
			Port:        ref.Port,
		})
	}

	return out
}

func toProtoRetryPolicy(retry *ir.RetryPolicy) *controlv1.HttpRouteRetry {
	if retry == nil {
		return nil
	}

	out := &controlv1.HttpRouteRetry{
		Codes:    append([]uint32(nil), retry.Codes...),
		Attempts: retry.Attempts,
	}
	if retry.Backoff != nil {
		out.Backoff = durationpb.New(*retry.Backoff)
	}

	if len(out.Codes) == 0 && out.Attempts == 0 && out.Backoff == nil {
		return nil
	}

	return out
}

func toProtoBackends(refs []ir.BackendRef) []*controlv1.BackendRef {
	return toProtoBackendsWithLogger(refs, nil)
}

func toProtoBackendsWithLogger(refs []ir.BackendRef, logger *slog.Logger) []*controlv1.BackendRef {
	out := make([]*controlv1.BackendRef, 0, len(refs))
	for _, ref := range refs {
		out = append(out, &controlv1.BackendRef{
			Group:     ref.Group,
			Kind:      ref.Kind,
			Namespace: ref.Namespace,
			Name:      ref.Name,
			Port:      ref.Port,
			Weight:    ref.Weight,
			Metadata:  ref.Metadata,
			Filters:   toProtoFiltersWithLogger(ref.Filters, logger),
		})
	}

	return out
}

func toProtoFilters(filters []ir.Filter) []*controlv1.Filter {
	return toProtoFiltersWithLogger(filters, nil)
}

func toProtoFiltersWithLogger(filters []ir.Filter, logger *slog.Logger) []*controlv1.Filter {
	logger = snapshotProtoLogger(logger)
	out := make([]*controlv1.Filter, 0, len(filters))
	for _, filter := range filters {
		payload, err := newStructPB(filter.Config)
		if err != nil {
			logger.Warn("failed to build filter config struct", "filter_type", filter.Type, "error", err)
			payload = emptyStructFallback()
		}
		out = append(out, &controlv1.Filter{
			Type:   filter.Type,
			Config: payload,
		})
	}

	return out
}

func toProtoHeaders(headers []ir.HeaderMatch) []*controlv1.HeaderMatch {
	out := make([]*controlv1.HeaderMatch, 0, len(headers))
	for _, header := range headers {
		out = append(out, &controlv1.HeaderMatch{
			Name:      header.Name,
			Value:     header.Value,
			MatchType: header.MatchType,
		})
	}

	return out
}

func toProtoQueries(queries []ir.QueryMatch) []*controlv1.QueryMatch {
	out := make([]*controlv1.QueryMatch, 0, len(queries))
	for _, query := range queries {
		out = append(out, &controlv1.QueryMatch{
			Name:      query.Name,
			Value:     query.Value,
			MatchType: query.MatchType,
		})
	}

	return out
}

func toListenerProtocol(protocol string) controlv1.ListenerProtocol {
	switch protocol {
	case constants.ProtocolHTTP:
		return controlv1.ListenerProtocol_LISTENER_HTTP
	case "HTTPS":
		return controlv1.ListenerProtocol_LISTENER_HTTPS
	case "GRPC":
		return controlv1.ListenerProtocol_LISTENER_GRPC
	case "HTTP3":
		return controlv1.ListenerProtocol_LISTENER_HTTP3
	case constants.ProtocolTCP:
		return controlv1.ListenerProtocol_LISTENER_TCP
	case "UDP":
		return controlv1.ListenerProtocol_LISTENER_UDP
	case "TLS_PASSTHROUGH":
		return controlv1.ListenerProtocol_LISTENER_TLS_PASSTHROUGH
	case "TLS":
		return controlv1.ListenerProtocol_LISTENER_TLS
	default:
		return controlv1.ListenerProtocol_LISTENER_UNSPECIFIED
	}
}

func toRouteKind(kind string) controlv1.RouteKind {
	switch kind {
	case constants.ProtocolHTTP:
		return controlv1.RouteKind_ROUTE_KIND_HTTP
	case "GRPC":
		return controlv1.RouteKind_ROUTE_KIND_GRPC
	case constants.ProtocolTCP:
		return controlv1.RouteKind_ROUTE_KIND_TCP
	case "UDP":
		return controlv1.RouteKind_ROUTE_KIND_UDP
	case "TLS":
		return controlv1.RouteKind_ROUTE_KIND_TLS
	default:
		return controlv1.RouteKind_ROUTE_KIND_UNSPECIFIED
	}
}

func toEmptyStructWithLogger(logger *slog.Logger) *structpb.Struct {
	logger = snapshotProtoLogger(logger)
	payload, err := newStructPB(map[string]any{})
	if err != nil {
		logger.Warn("failed to build empty struct", "error", err)
		return emptyStructFallback()
	}
	return payload
}

func emptyStructFallback() *structpb.Struct {
	return &structpb.Struct{Fields: map[string]*structpb.Value{}}
}

func snapshotProtoLogger(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}
	return slog.Default()
}

func durationOrNil(value *time.Duration) *durationpb.Duration {
	if value == nil {
		return nil
	}

	return durationpb.New(*value)
}

func nonZeroDurationOrNil(value time.Duration) *durationpb.Duration {
	if value == 0 {
		return nil
	}

	return durationpb.New(value)
}

// injectTraceparent extracts the W3C traceparent header from the current span
// context and writes it into the snapshot proto so the data plane can create
// child spans linked to the snapshot generation trace.
func injectTraceparent(ctx context.Context, snapshot *controlv1.ConfigSnapshot) {
	prop := propagation.TraceContext{}
	carrier := propagation.MapCarrier{}
	prop.Inject(ctx, carrier)
	snapshot.Traceparent = carrier.Get("traceparent")
}
