package grpcserver

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/nantian-gw/gateway/controlplane/internal/observability"
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
