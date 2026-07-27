package xds

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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
	}

	// Main loop: handle client requests and snapshot changes
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case snap, ok := <-ds.snapshotCh:
			if !ok {
				return nil
			}
			ds.pushDelta(ctx, ds.lastSnapshot, snap)
			ds.lastSnapshot = snap
		default:
		}

		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		ds.handleAckNack(req)
	}
}

type deltaStream struct {
	logger     *slog.Logger
	stream     controlv1.DeltaDiscoveryService_DeltaStreamConfigurationServer
	store      *ir.SnapshotStore
	snapshotCh <-chan *ir.Snapshot
	unsub      func()
	subscribed map[string]bool
	versions   map[string]string
	lastSnapshot *ir.Snapshot
	mu         sync.Mutex
	versionSeq uint64
}

func (ds *deltaStream) pushDelta(ctx context.Context, old, new *ir.Snapshot) {
	ds.mu.Lock()
	ds.versionSeq++
	ver := fmt.Sprintf("%d", ds.versionSeq)
	ds.mu.Unlock()

	delta := SnapshotDelta(old, new)
	hasChanges := false

	typeResources := []struct {
		typeURL string
		rd      *ResourceDelta
	}{
		{typeURLListener, &delta.Listeners},
		{typeURLHTTPRoute, &delta.HTTPRoutes},
		{typeURLGRPCRoute, &delta.GRPCRoutes},
		{typeURLStreamRoute, &delta.StreamRoutes},
		{typeURLBackend, &delta.Backends},
		{typeURLSecret, &delta.Secrets},
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
			NonIncremental:    tr.rd.HasNonIncremental(typeResourceCount(tr.typeURL, old)),
		}

		if err := ds.stream.Send(resp); err != nil {
			ds.logger.Error("delta send failed", "type_url", tr.typeURL, "error", err)
		}
	}

	if !hasChanges {
		return
	}

	if new != nil {
		_, span := otel.Tracer("").Start(ctx, "xds.push_delta_snapshot")
		span.SetAttributes(
			attribute.String("snapshot_id", new.ID),
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
