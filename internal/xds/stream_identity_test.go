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

	"github.com/nantian-gw/gateway/internal/config"
	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/nodeinfo"
	"github.com/nantian-gw/gateway/internal/observability"
)

func TestStreamConfigurationRejectsInitialRequestWithoutNodeID(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	metrics := observability.NewMetrics()
	nodes := nodeinfo.NewRegistry(ir.NewNodeStatusStore(), nil, logger, nodeinfo.Options{})
	defer nodes.Close()

	server, err := New(":18080", config.GRPCTLSConfig{}, config.GRPCRuntimeConfig{}, store, nodes, logger, metrics)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	stream := newFakeConfigStream()
	stream.initialRecv <- &controlv1.DiscoveryRequest{
		Cluster:       "default",
		Subscriptions: []string{"*"},
	}

	result := make(chan error, 1)
	go func() {
		result <- server.StreamConfiguration(stream)
	}()

	select {
	case err := <-result:
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("expected invalid argument status on empty initial node_id, got %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		stream.release()
		t.Fatal("StreamConfiguration did not reject empty initial node_id")
	}

	stream.release()
	if _, ok := nodes.Get(context.Background(), ""); ok {
		t.Fatal("expected empty node_id stream to avoid recording node status")
	}
	if got := testutil.ToFloat64(metrics.XDSStreamTerminationsTotal.WithLabelValues("invalid_request")); got != 1 {
		t.Fatalf("invalid request stream termination count = %v, want 1", got)
	}
}

func TestStreamConfigurationRejectsNodeIDChangeMidStream(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	metrics := observability.NewMetrics()
	nodes := nodeinfo.NewRegistry(ir.NewNodeStatusStore(), nil, logger, nodeinfo.Options{})
	defer nodes.Close()

	server, err := New(":18080", config.GRPCTLSConfig{}, config.GRPCRuntimeConfig{}, store, nodes, logger, metrics)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	stream := newFakeConfigStream()
	result := make(chan error, 1)
	go func() {
		result <- server.StreamConfiguration(stream)
	}()

	waitForNodeConnection(t, nodes, "dp-1")
	stream.pushRecv(&controlv1.DiscoveryRequest{
		NodeId:        "dp-2",
		Cluster:       "default",
		Subscriptions: []string{"*"},
	})

	select {
	case err := <-result:
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("expected invalid argument status on mid-stream node_id change, got %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		stream.release()
		t.Fatal("StreamConfiguration did not reject mid-stream node_id change")
	}

	stream.release()
	nodeStatus, ok := nodes.Get(context.Background(), "dp-1")
	if !ok {
		t.Fatal("expected original node status to be recorded")
	}
	if nodeStatus.Connected {
		t.Fatalf("expected original node to be disconnected after invalid node_id change, got %#v", nodeStatus)
	}
	if nodeStatus.Ready {
		t.Fatalf("expected invalid request disconnect to clear readiness, got %#v", nodeStatus)
	}
	if nodeStatus.Message != "invalid xds discovery request" {
		t.Fatalf("expected invalid request disconnect message, got %#v", nodeStatus)
	}
	if _, ok := nodes.Get(context.Background(), "dp-2"); ok {
		t.Fatal("expected mismatched node_id to avoid creating a new node status")
	}
	if got := testutil.ToFloat64(metrics.XDSStreamTerminationsTotal.WithLabelValues("invalid_request")); got != 1 {
		t.Fatalf("invalid request stream termination count = %v, want 1", got)
	}
}
