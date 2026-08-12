package xds

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"

	"github.com/nantian-gw/gateway/internal/ir"
)

func (s *Server) DeltaStreamConfiguration(stream controlv1.DeltaDiscoveryService_DeltaStreamConfigurationServer) (err error) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("panic in delta stream handler", "panic", r)
			err = status.Error(codes.Internal, "internal server error")
		}
	}()

	ctx := stream.Context()
	req, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.Unavailable, "receive initial delta request: %v", err)
	}
	nodeID := req.GetNodeId()
	s.logger.Info("delta dataplane connected", "node_id", nodeID)

	reg := s.registerStream(nodeID)
	defer s.unregisterStream(reg)

	sub, unsubscribe := s.store.Subscribe()
	defer unsubscribe()

	ds := &deltaStream{
		logger:     s.logger,
		stream:     stream,
		snapshotCh: sub,
		unsub:      unsubscribe,
		subscribed: make(map[string]bool),
		versions:   make(map[string]string),
	}

	for _, sub := range req.GetResourceNamesSubscribe() {
		ds.subscribed[sub] = true
	}

	// Send initial full state for subscribed types
	initial := s.store.Current()
	if initial != nil {
		ds.pushDelta(ctx, nil, initial)
		ds.lastSnapshot = initial
	}

	// Main loop: handle client requests and snapshot changes
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("delta stream context: %w", ctx.Err())
		case snap, ok := <-ds.snapshotCh:
			if !ok {
				return nil
			}
			ds.pushDelta(ctx, ds.lastSnapshot, snap)
			ds.lastSnapshot = snap
		default:
		}

		req, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("delta stream recv: %w", err)
		}

		ds.handleAckNack(req)
	}
}

type deltaStream struct {
	logger       *slog.Logger
	stream       controlv1.DeltaDiscoveryService_DeltaStreamConfigurationServer
	snapshotCh   <-chan *ir.Snapshot
	unsub        func()
	subscribed   map[string]bool
	versions     map[string]string
	lastSnapshot *ir.Snapshot
	mu           sync.Mutex
	versionSeq   uint64
}

// pushDelta computes the delta between prev and curr, then sends a
// DeltaDiscoveryResponse per resource type. Added/changed resources are
// serialized into the Resources field as google.protobuf.Any payloads.
func (ds *deltaStream) pushDelta(ctx context.Context, prev, curr *ir.Snapshot) {
	ds.mu.Lock()
	ds.versionSeq++
	ver := fmt.Sprintf("%d", ds.versionSeq)
	ds.mu.Unlock()

	delta := SnapshotDelta(prev, curr)
	hasChanges := false

	typeResources := []struct {
		typeURL string
		rd      *ResourceDelta
		get     func(*ir.Snapshot, []string) ([]*controlv1.Resource, error)
	}{
		{typeURLListener, &delta.Listeners, deltaListenerResources},
		{typeURLHTTPRoute, &delta.HTTPRoutes, deltaHTTPRouteResources},
		{typeURLGRPCRoute, &delta.GRPCRoutes, deltaGRPCRouteResources},
		{typeURLStreamRoute, &delta.StreamRoutes, deltaStreamRouteResources},
		{typeURLBackend, &delta.Backends, deltaBackendResources},
		{typeURLSecret, &delta.Secrets, deltaSecretResources},
	}

	for _, tr := range typeResources {
		if !ds.subscribed[tr.typeURL] || tr.rd.IsEmpty() {
			continue
		}
		hasChanges = true

		nonce, _ := newNonce()
		resp := &controlv1.DeltaDiscoveryResponse{
			SystemVersionInfo: ver,
			Nonce:             nonce,
			TypeUrl:           tr.typeURL,
			RemovedResources:  tr.rd.Removed,
			NonIncremental:    tr.rd.HasNonIncremental(typeResourceCount(tr.typeURL, prev)),
		}

		// Serialize the added/changed resources into the payload.
		// Without this, the data plane would be told resources changed but
		// never receive their actual content, leaving it with an empty cache.
		if len(tr.rd.AddedChanged) > 0 && curr != nil {
			resources, err := tr.get(curr, tr.rd.AddedChanged)
			if err != nil {
				ds.logger.Error("delta resource serialize failed",
					"type_url", tr.typeURL, "error", err)
			} else if len(resources) > 0 {
				resp.Resources = resources
			}
		}

		if err := ds.stream.Send(resp); err != nil {
			ds.logger.Error("delta send failed", "type_url", tr.typeURL, "error", err)
		}
	}

	if !hasChanges {
		return
	}

	if curr != nil {
		_, span := otel.Tracer("").Start(ctx, "xds.push_delta_snapshot")
		span.SetAttributes(
			attribute.String("snapshot_id", curr.ID),
			attribute.String("system_version", ver),
		)
		span.End()
	}
}

func (ds *deltaStream) handleAckNack(req *controlv1.DeltaDiscoveryRequest) {
	if req.GetResultStatus() == controlv1.DiscoveryResultStatus_DISCOVERY_RESULT_STATUS_NACK {
		ds.logger.Warn("delta NACK",
			"node_id", req.GetNodeId(),
			"type_url", req.GetTypeUrl(),
			"error_detail", req.GetErrorDetail(),
		)
	}

	// Handle subscription changes
	for _, sub := range req.GetResourceNamesSubscribe() {
		ds.subscribed[sub] = true
	}
	for _, unsub := range req.GetResourceNamesUnsubscribe() {
		delete(ds.subscribed, unsub)
	}
}

// ---------------------------------------------------------------------------
// Resource serialization helpers for delta responses
// ---------------------------------------------------------------------------

// resourceToDelta wraps a proto message in google.protobuf.Any and returns a
// *controlv1.Resource suitable for inclusion in a DeltaDiscoveryResponse.
func resourceToDelta(name, version string, msg proto.Message) *controlv1.Resource {
	anyPayload, err := anypb.New(msg)
	if err != nil {
		return nil
	}
	return &controlv1.Resource{
		Name:     name,
		Version:  version,
		Resource: anyPayload,
	}
}

// deltaListenerResources serializes the listeners whose names appear in
// addedChanged into delta Resource objects. Names are formatted as "name/port".
func deltaListenerResources(snap *ir.Snapshot, addedChanged []string) ([]*controlv1.Resource, error) {
	if len(addedChanged) == 0 || snap == nil {
		return nil, nil
	}
	nameSet := make(map[string]bool, len(addedChanged))
	for _, name := range addedChanged {
		nameSet[name] = true
	}
	var resources []*controlv1.Resource
	for _, item := range snap.Listeners {
		key := fmt.Sprintf("%s/%d", item.Name, item.Port)
		if !nameSet[key] {
			continue
		}
		proto := &controlv1.Listener{
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
		}
		if r := resourceToDelta(key, ResourceVersion(item), proto); r != nil {
			resources = append(resources, r)
		}
	}
	return resources, nil
}

// deltaHTTPRouteResources serializes the HTTP routes whose names appear in
// addedChanged into delta Resource objects. Names are formatted as "namespace/name".
func deltaHTTPRouteResources(snap *ir.Snapshot, addedChanged []string) ([]*controlv1.Resource, error) {
	if len(addedChanged) == 0 || snap == nil {
		return nil, nil
	}
	nameSet := make(map[string]bool, len(addedChanged))
	for _, name := range addedChanged {
		nameSet[name] = true
	}
	var resources []*controlv1.Resource
	for _, item := range snap.HTTPRoutes {
		key := fmt.Sprintf("%s/%s", item.Namespace, item.Name)
		if !nameSet[key] {
			continue
		}
		proto := &controlv1.HttpRoute{
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
				Filters:            toProtoFiltersWithLogger(rule.Filters, nil),
				BackendRefs:        toProtoBackendsWithLogger(rule.BackendRefs, nil),
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
			proto.Rules = append(proto.Rules, protoRule)
		}
		if r := resourceToDelta(key, ResourceVersion(item), proto); r != nil {
			resources = append(resources, r)
		}
	}
	return resources, nil
}

// deltaGRPCRouteResources serializes the gRPC routes whose names appear in
// addedChanged into delta Resource objects. Names are formatted as "namespace/name".
func deltaGRPCRouteResources(snap *ir.Snapshot, addedChanged []string) ([]*controlv1.Resource, error) {
	if len(addedChanged) == 0 || snap == nil {
		return nil, nil
	}
	nameSet := make(map[string]bool, len(addedChanged))
	for _, name := range addedChanged {
		nameSet[name] = true
	}
	var resources []*controlv1.Resource
	for _, item := range snap.GRPCRoutes {
		key := fmt.Sprintf("%s/%s", item.Namespace, item.Name)
		if !nameSet[key] {
			continue
		}
		proto := &controlv1.GrpcRoute{
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
				Filters:            toProtoFiltersWithLogger(rule.Filters, nil),
				BackendRefs:        toProtoBackendsWithLogger(rule.BackendRefs, nil),
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
			proto.Rules = append(proto.Rules, protoRule)
		}
		if r := resourceToDelta(key, ResourceVersion(item), proto); r != nil {
			resources = append(resources, r)
		}
	}
	return resources, nil
}

// deltaStreamRouteResources serializes the stream routes whose names appear in
// addedChanged into delta Resource objects. Names are formatted as "namespace/name".
func deltaStreamRouteResources(snap *ir.Snapshot, addedChanged []string) ([]*controlv1.Resource, error) {
	if len(addedChanged) == 0 || snap == nil {
		return nil, nil
	}
	nameSet := make(map[string]bool, len(addedChanged))
	for _, name := range addedChanged {
		nameSet[name] = true
	}
	var resources []*controlv1.Resource
	for _, item := range snap.StreamRoutes {
		key := fmt.Sprintf("%s/%s", item.Namespace, item.Name)
		if !nameSet[key] {
			continue
		}
		proto := &controlv1.StreamRoute{
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
				BackendRefs: toProtoBackendsWithLogger(rule.BackendRefs, nil),
			}
			for _, match := range rule.Matches {
				protoMatch := &controlv1.StreamMatch{
					Port:        match.Port,
					SniHostname: match.SNIHostname,
				}
				switch match.Mode {
				case ir.TlsRouteModePassthrough:
					protoMatch.Mode = controlv1.TlsRouteMode_TLS_ROUTE_MODE_PASSTHROUGH
				case ir.TlsRouteModeTerminate:
					protoMatch.Mode = controlv1.TlsRouteMode_TLS_ROUTE_MODE_TERMINATE
				}
				protoRule.Matches = append(protoRule.Matches, protoMatch)
			}
			proto.Rules = append(proto.Rules, protoRule)
		}
		if r := resourceToDelta(key, ResourceVersion(item), proto); r != nil {
			resources = append(resources, r)
		}
	}
	return resources, nil
}

// deltaBackendResources serializes the backends whose names appear in
// addedChanged into delta Resource objects. Names are the backend's Name field.
func deltaBackendResources(snap *ir.Snapshot, addedChanged []string) ([]*controlv1.Resource, error) {
	if len(addedChanged) == 0 || snap == nil {
		return nil, nil
	}
	nameSet := make(map[string]bool, len(addedChanged))
	for _, name := range addedChanged {
		nameSet[name] = true
	}
	var resources []*controlv1.Resource
	for _, item := range snap.Backends {
		if !nameSet[item.Name] {
			continue
		}
		proto := &controlv1.BackendCluster{
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
			proto.Endpoints = append(proto.Endpoints, &controlv1.BackendEndpoint{
				Address: endpoint.Address,
				Port:    endpoint.Port,
				Healthy: endpoint.Healthy,
				Zone:    endpoint.Zone,
			})
		}
		proto.AiService = toProtoAIService(item.AIService)
		proto.TokenPolicy = toProtoTokenPolicy(item.TokenPolicy)
		proto.WasmPlugin = toProtoWasmPlugin(item.WasmPlugin)
		if r := resourceToDelta(item.Name, ResourceVersion(item), proto); r != nil {
			resources = append(resources, r)
		}
	}
	return resources, nil
}

// deltaSecretResources serializes the secrets whose names appear in
// addedChanged into delta Resource objects. Names are the secret's Name field.
func deltaSecretResources(snap *ir.Snapshot, addedChanged []string) ([]*controlv1.Resource, error) {
	if len(addedChanged) == 0 || snap == nil {
		return nil, nil
	}
	nameSet := make(map[string]bool, len(addedChanged))
	for _, name := range addedChanged {
		nameSet[name] = true
	}
	var resources []*controlv1.Resource
	for _, item := range snap.Secrets {
		if !nameSet[item.Name] {
			continue
		}
		proto := &controlv1.SecretMaterial{
			Namespace: item.Namespace,
			Name:      item.Name,
			CertPem:   item.CertPEM,
			KeyPem:    item.KeyPEM,
		}
		if r := resourceToDelta(item.Name, ResourceVersion(item), proto); r != nil {
			resources = append(resources, r)
		}
	}
	return resources, nil
}