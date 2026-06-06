package status

import (
	"context"
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestRetryStatusUpdateRecordsConflictRetryAndErrorMetrics(t *testing.T) {
	t.Run("conflict_then_success", func(t *testing.T) {
		resetStatusUpdateMetricsForTest()

		attempts := 0
		err := (&Reconciler{}).retryStatusUpdate(context.Background(), statusUpdateResourceGateway, func() error {
			attempts++
			if attempts == 1 {
				return apierrors.NewConflict(
					schema.GroupResource{Group: "gateway.networking.k8s.io", Resource: "gateways"},
					"example",
					errors.New("optimistic lock"),
				)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("retryStatusUpdate returned error: %v", err)
		}
		if attempts != 2 {
			t.Fatalf("attempts = %d, want 2", attempts)
		}
		if got := testutil.ToFloat64(statusUpdateConflictsTotal.WithLabelValues(statusUpdateResourceGateway)); got != 1 {
			t.Fatalf("conflicts_total = %v, want 1", got)
		}
		if got := testutil.ToFloat64(statusUpdateRetriesTotal.WithLabelValues(statusUpdateResourceGateway)); got != 1 {
			t.Fatalf("retries_total = %v, want 1", got)
		}
		if got := testutil.ToFloat64(statusUpdateErrorsTotal.WithLabelValues(statusUpdateResourceGateway, statusUpdateErrorConflict)); got != 0 {
			t.Fatalf("errors_total{reason=conflict} = %v, want 0", got)
		}
	})

	t.Run("terminal_error", func(t *testing.T) {
		resetStatusUpdateMetricsForTest()

		wantErr := errors.New("boom")
		err := (&Reconciler{}).retryStatusUpdate(context.Background(), statusUpdateResourceGateway, func() error {
			return wantErr
		})
		if !errors.Is(err, wantErr) {
			t.Fatalf("retryStatusUpdate error = %v, want %v", err, wantErr)
		}
		if got := testutil.ToFloat64(statusUpdateConflictsTotal.WithLabelValues(statusUpdateResourceGateway)); got != 0 {
			t.Fatalf("conflicts_total = %v, want 0", got)
		}
		if got := testutil.ToFloat64(statusUpdateRetriesTotal.WithLabelValues(statusUpdateResourceGateway)); got != 0 {
			t.Fatalf("retries_total = %v, want 0", got)
		}
		if got := testutil.ToFloat64(statusUpdateErrorsTotal.WithLabelValues(statusUpdateResourceGateway, statusUpdateErrorOther)); got != 1 {
			t.Fatalf("errors_total{reason=other} = %v, want 1", got)
		}
	})
}

func TestRetryStatusUpdateNormalizesUnknownResourceMetricLabel(t *testing.T) {
	resetStatusUpdateMetricsForTest()

	wantErr := errors.New("boom")
	err := (&Reconciler{}).retryStatusUpdate(context.Background(), "tenant-specific-resource", func() error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("retryStatusUpdate error = %v, want %v", err, wantErr)
	}
	if got := testutil.ToFloat64(statusUpdateErrorsTotal.WithLabelValues(statusUpdateResourceOther, statusUpdateErrorOther)); got != 1 {
		t.Fatalf("errors_total{resource=other,reason=other} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(statusUpdateErrorsTotal.WithLabelValues("tenant-specific-resource", statusUpdateErrorOther)); got != 0 {
		t.Fatalf("errors_total{resource=tenant-specific-resource,reason=other} = %v, want 0", got)
	}
}

func resetStatusUpdateMetricsForTest() {
	statusUpdateConflictsTotal.Reset()
	statusUpdateRetriesTotal.Reset()
	statusUpdateErrorsTotal.Reset()
}
