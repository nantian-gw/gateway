package xds

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"

	"github.com/nantian-gw/gateway/internal/config"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/noderegistry"
	"github.com/nantian-gw/gateway/internal/observability"
)

func TestReportStatusRejectsMissingNodeID(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	metrics := observability.NewMetrics()
	nodes := noderegistry.NewRegistry(ir.NewNodeStatusStore(), nil, logger, noderegistry.Options{})
	defer nodes.Close()

	server, err := New(":18080", config.GRPCTLSConfig{}, config.GRPCRuntimeConfig{}, store, nodes, logger, metrics)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	_, err = server.ReportStatus(context.Background(), &controlv1.StatusReport{
		Version: "v1",
		Ready:   true,
		Message: "ready",
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected invalid argument status on empty report node_id, got %v", err)
	}
	if _, ok := nodes.Get(context.Background(), ""); ok {
		t.Fatal("expected empty node_id report to avoid recording node status")
	}
	if got := testutil.ToFloat64(metrics.XDSStatusReportRejectionsTotal.WithLabelValues("invalid_request")); got != 1 {
		t.Fatalf("invalid request status report rejection count = %v, want 1", got)
	}
}

func TestReportStatusRejectsDuringShutdown(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	metrics := observability.NewMetrics()
	nodes := noderegistry.NewRegistry(ir.NewNodeStatusStore(), nil, logger, noderegistry.Options{})
	defer nodes.Close()

	server, err := New(":18080", config.GRPCTLSConfig{}, config.GRPCRuntimeConfig{}, store, nodes, logger, metrics)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	server.signalShutdown()

	_, err = server.ReportStatus(context.Background(), &controlv1.StatusReport{
		NodeId:  "dp-1",
		Version: "v1",
		Ready:   true,
		Message: "ready",
	})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("expected unavailable status on shutdown report, got %v", err)
	}
	if _, ok := nodes.Get(context.Background(), "dp-1"); ok {
		t.Fatal("expected shutdown report to avoid recording node status")
	}
	if got := testutil.ToFloat64(metrics.XDSStatusReportRejectionsTotal.WithLabelValues("shutdown")); got != 1 {
		t.Fatalf("shutdown status report rejection count = %v, want 1", got)
	}
}

func TestReportStatusRejectsUnknownNodeWithoutPriorXDSIdentity(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	metrics := observability.NewMetrics()
	nodes := noderegistry.NewRegistry(ir.NewNodeStatusStore(), nil, logger, noderegistry.Options{})
	defer nodes.Close()

	server, err := New(":18080", config.GRPCTLSConfig{}, config.GRPCRuntimeConfig{}, store, nodes, logger, metrics)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	_, err = server.ReportStatus(context.Background(), &controlv1.StatusReport{
		NodeId:  "dp-unknown",
		Version: "v1",
		Ready:   true,
		Message: "ready",
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected failed precondition status for unknown node report, got %v", err)
	}
	if _, ok := nodes.Get(context.Background(), "dp-unknown"); ok {
		t.Fatal("expected unknown node report to avoid recording node status")
	}
	if got := testutil.ToFloat64(metrics.XDSStatusReportRejectionsTotal.WithLabelValues("unknown_node")); got != 1 {
		t.Fatalf("unknown-node status report rejection count = %v, want 1", got)
	}
}

func TestReportStatusClampsFutureObservedAt(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	nodes := noderegistry.NewRegistry(ir.NewNodeStatusStore(), nil, logger, noderegistry.Options{})
	defer nodes.Close()

	server, err := New(":18080", config.GRPCTLSConfig{}, config.GRPCRuntimeConfig{}, store, nodes, logger, nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	start := time.Now().UTC()
	nodes.Connect(context.Background(), "dp-1", "kind", []string{"*"}, start.Add(-time.Second))
	_, err = server.ReportStatus(context.Background(), &controlv1.StatusReport{
		NodeId:     "dp-1",
		Version:    "v1",
		Ready:      true,
		Message:    "ready",
		ObservedAt: timestamppb.New(start.Add(24 * time.Hour)),
	})
	if err != nil {
		t.Fatalf("ReportStatus returned error: %v", err)
	}
	end := time.Now().UTC()

	nodeStatus, ok := nodes.Get(context.Background(), "dp-1")
	if !ok {
		t.Fatal("expected node status to be recorded")
	}
	if nodeStatus.LastSeenAt.Before(start) || nodeStatus.LastSeenAt.After(end) {
		t.Fatalf(
			"expected future observedAt to clamp into [%s, %s], got %s",
			start,
			end,
			nodeStatus.LastSeenAt,
		)
	}
}

func TestReportStatusIgnoresObservedAtThatWouldRegressNodeState(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	nodes := noderegistry.NewRegistry(ir.NewNodeStatusStore(), nil, logger, noderegistry.Options{})
	defer nodes.Close()

	server, err := New(":18080", config.GRPCTLSConfig{}, config.GRPCRuntimeConfig{}, store, nodes, logger, nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	now := time.Now().UTC().Add(-3 * time.Second)
	nodes.Connect(context.Background(), "dp-1", "kind", []string{"*"}, now)
	nodes.ObserveReport(context.Background(), "dp-1", "v2", true, "ready", now.Add(time.Second))
	nodes.Disconnect(context.Background(), "dp-1", now.Add(2*time.Second))

	_, err = server.ReportStatus(context.Background(), &controlv1.StatusReport{
		NodeId:     "dp-1",
		Version:    "v1",
		Ready:      false,
		Message:    "stale heartbeat",
		ObservedAt: timestamppb.New(now.Add(time.Second)),
	})
	if err != nil {
		t.Fatalf("ReportStatus returned error: %v", err)
	}

	nodeStatus, ok := nodes.Get(context.Background(), "dp-1")
	if !ok {
		t.Fatal("expected node status to be recorded")
	}
	if nodeStatus.Connected {
		t.Fatalf("expected node to remain disconnected, got %+v", nodeStatus)
	}
	if !nodeStatus.LastSeenAt.Equal(now.Add(2 * time.Second)) {
		t.Fatalf("expected stale report to preserve disconnect timestamp, got %+v", nodeStatus)
	}
	if nodeStatus.LastAckVersion != "v2" {
		t.Fatalf("expected stale report to preserve ack version, got %+v", nodeStatus)
	}
	if nodeStatus.Ready {
		t.Fatalf("expected stale report to preserve disconnected readiness=false state, got %+v", nodeStatus)
	}
	if nodeStatus.Message != "" {
		t.Fatalf("expected disconnect without explicit reason to preserve empty message, got %+v", nodeStatus)
	}
}

func TestReportStatusKeepsDisconnectedNodeOfflineAfterNewerHeartbeat(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	nodes := noderegistry.NewRegistry(ir.NewNodeStatusStore(), nil, logger, noderegistry.Options{})
	defer nodes.Close()

	server, err := New(":18080", config.GRPCTLSConfig{}, config.GRPCRuntimeConfig{}, store, nodes, logger, nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	now := time.Now().UTC().Add(-4 * time.Second)
	nodes.Connect(context.Background(), "dp-1", "kind", []string{"*"}, now)
	nodes.ObserveReport(context.Background(), "dp-1", "v2", true, "ready", now.Add(time.Second))
	nodes.DisconnectWithReason(
		context.Background(),
		"dp-1",
		"ack_timeout",
		"timed out waiting for dataplane snapshot ack",
		now.Add(2*time.Second),
	)

	_, err = server.ReportStatus(context.Background(), &controlv1.StatusReport{
		NodeId:     "dp-1",
		Version:    "v9",
		Ready:      true,
		Message:    "ready-again",
		ObservedAt: timestamppb.New(now.Add(3 * time.Second)),
	})
	if err != nil {
		t.Fatalf("ReportStatus returned error: %v", err)
	}

	nodeStatus, ok := nodes.Get(context.Background(), "dp-1")
	if !ok {
		t.Fatal("expected node status to be recorded")
	}
	if nodeStatus.Connected {
		t.Fatalf("expected node to remain disconnected, got %+v", nodeStatus)
	}
	if nodeStatus.Ready {
		t.Fatalf("expected node to remain not ready while disconnected, got %+v", nodeStatus)
	}
	if nodeStatus.LastAckVersion != "v2" {
		t.Fatalf("expected disconnected heartbeat to preserve ack version, got %+v", nodeStatus)
	}
	if nodeStatus.DisconnectReason != "ack_timeout" {
		t.Fatalf("expected disconnected heartbeat to preserve disconnect reason, got %+v", nodeStatus)
	}
	if nodeStatus.Message != "timed out waiting for dataplane snapshot ack" {
		t.Fatalf("expected disconnected heartbeat to preserve disconnect message, got %+v", nodeStatus)
	}
	if !nodeStatus.DisconnectedAt.Equal(now.Add(2 * time.Second)) {
		t.Fatalf("expected disconnected heartbeat to preserve disconnectedAt, got %+v", nodeStatus)
	}
	if !nodeStatus.LastSeenAt.Equal(now.Add(3 * time.Second)) {
		t.Fatalf("expected disconnected heartbeat to refresh only lastSeenAt, got %+v", nodeStatus)
	}
}
