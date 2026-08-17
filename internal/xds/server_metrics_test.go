package xds

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/observability"
)

func TestRecordStreamTerminationNormalizesUnknownReason(t *testing.T) {
	metrics := observability.NewMetrics()
	server := &Server{metrics: metrics}

	server.recordStreamTermination("node/default/pod-123")

	if got := testutil.ToFloat64(metrics.XDSStreamTerminationsTotal.WithLabelValues("other")); got != 1 {
		t.Fatalf("other stream termination count = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.XDSStreamTerminationsTotal.WithLabelValues("node/default/pod-123")); got != 0 {
		t.Fatalf("raw stream termination count = %v, want 0", got)
	}
}

func TestRecordStatusReportRejectionNormalizesUnknownReason(t *testing.T) {
	metrics := observability.NewMetrics()
	server := &Server{metrics: metrics}

	server.recordStatusReportRejection("node/default/pod-123")

	if got := testutil.ToFloat64(metrics.XDSStatusReportRejectionsTotal.WithLabelValues("other")); got != 1 {
		t.Fatalf("other status report rejection count = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.XDSStatusReportRejectionsTotal.WithLabelValues("node/default/pod-123")); got != 0 {
		t.Fatalf("raw status report rejection count = %v, want 0", got)
	}
}

func TestObserveSnapshotDivergence(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	now := time.Now().UTC()
	store.Publish(&ir.Snapshot{
		GeneratedAt: now,
		Workloads:   []ir.Workload{{Namespace: "default", Name: "dp-1", IP: "10.0.0.1"}},
	})
	currentID := store.Current().ID
	if currentID == "" {
		t.Fatal("expected published snapshot to have a non-empty ID")
	}

	metrics := observability.NewMetrics()
	server := &Server{metrics: metrics, store: store}

	// A report matching the current snapshot is not divergence.
	server.observeSnapshotDivergence(currentID)
	if got := testutil.ToFloat64(metrics.SnapshotDivergenceTotal); got != 0 {
		t.Fatalf("matching snapshot version incremented divergence: %v, want 0", got)
	}

	// A stale version diverges from the current snapshot.
	server.observeSnapshotDivergence("stale-version")
	if got := testutil.ToFloat64(metrics.SnapshotDivergenceTotal); got != 1 {
		t.Fatalf("stale snapshot version divergence count = %v, want 1", got)
	}

	// Empty reported versions are ignored.
	server.observeSnapshotDivergence("")
	if got := testutil.ToFloat64(metrics.SnapshotDivergenceTotal); got != 1 {
		t.Fatalf("empty snapshot version divergence count = %v, want 1", got)
	}
}

func TestObserveSnapshotDivergenceHandlesNilStore(t *testing.T) {
	metrics := observability.NewMetrics()
	server := &Server{metrics: metrics}

	server.observeSnapshotDivergence("anything")
	if got := testutil.ToFloat64(metrics.SnapshotDivergenceTotal); got != 0 {
		t.Fatalf("nil store divergence count = %v, want 0", got)
	}
}
