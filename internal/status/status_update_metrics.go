package status

import (
	"context"
	"errors"

	"github.com/prometheus/client_golang/prometheus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	statusUpdateResourceGatewayClass     = "gatewayclass"
	statusUpdateResourceGateway          = "gateway"
	statusUpdateResourceHTTPRoute        = "httproute"
	statusUpdateResourceGRPCRoute        = "grpcroute"
	statusUpdateResourceTCPRoute         = "tcproute"
	statusUpdateResourceUDPRoute         = "udproute"
	statusUpdateResourceTLSRoute         = "tlsroute"
	statusUpdateResourceBackendLBPolicy  = "backendlbpolicy"
	statusUpdateResourceBackendTLSPolicy = "backendtlspolicy"
	statusUpdateResourceListenerSet     = "listenerset"
	statusUpdateResourceOther            = "other"

	statusUpdateErrorConflict         = "conflict"
	statusUpdateErrorCanceled         = "canceled"
	statusUpdateErrorDeadlineExceeded = "deadline_exceeded"
	statusUpdateErrorOther            = "other"
)

var (
	statusUpdateConflictsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nantian_gateway_controlplane_status_update_conflicts_total",
			Help: "Total number of controlplane status update conflicts partitioned by resource type.",
		},
		[]string{"resource"},
	)
	statusUpdateRetriesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nantian_gateway_controlplane_status_update_retries_total",
			Help: "Total number of additional controlplane status update retry attempts entered after the initial attempt, partitioned by resource type.",
		},
		[]string{"resource"},
	)
	statusUpdateErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nantian_gateway_controlplane_status_update_errors_total",
			Help: "Total number of terminal controlplane status update errors partitioned by resource type and normalized error class.",
		},
		[]string{"resource", "reason"},
	)
	statusUpdatesWrittenTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nantian_gateway_controlplane_status_updates_written_total",
			Help: "Total number of actual Patch calls for status updates.",
		},
		[]string{"resource"},
	)
	statusUpdatesSkippedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nantian_gateway_controlplane_status_updates_skipped_total",
			Help: "Total number of status updates skipped via DeepEqual.",
		},
		[]string{"resource"},
	)
	statusBatchDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "nantian_gateway_controlplane_status_batch_duration_seconds",
			Help:    "Duration of status update batch write phase.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"resource"},
	)
)

func init() {
	ctrlmetrics.Registry.MustRegister(
		statusUpdateConflictsTotal,
		statusUpdateRetriesTotal,
		statusUpdateErrorsTotal,
		statusUpdatesWrittenTotal,
		statusUpdatesSkippedTotal,
		statusBatchDurationSeconds,
	)
}

func (r *Reconciler) retryStatusUpdate(
	resource string,
	update func() error,
) error {
	if update == nil {
		return nil
	}
	resource = normalizeStatusUpdateResource(resource)

	attempt := 0
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if attempt > 0 {
			statusUpdateRetriesTotal.WithLabelValues(resource).Inc()
		}
		attempt++

		err := update()
		if apierrors.IsConflict(err) {
			statusUpdateConflictsTotal.WithLabelValues(resource).Inc()
		}
		return err
	})
	if err != nil {
		statusUpdateErrorsTotal.WithLabelValues(resource, classifyStatusUpdateError(err)).Inc()
	}
	return err
}

func normalizeStatusUpdateResource(resource string) string {
	switch resource {
	case statusUpdateResourceGatewayClass,
		statusUpdateResourceGateway,
		statusUpdateResourceHTTPRoute,
		statusUpdateResourceGRPCRoute,
		statusUpdateResourceTCPRoute,
		statusUpdateResourceUDPRoute,
		statusUpdateResourceTLSRoute,
		statusUpdateResourceBackendLBPolicy,
		statusUpdateResourceBackendTLSPolicy,
		statusUpdateResourceListenerSet:
		return resource
	default:
		return statusUpdateResourceOther
	}
}

func classifyStatusUpdateError(err error) string {
	switch {
	case err == nil:
		return ""
	case apierrors.IsConflict(err):
		return statusUpdateErrorConflict
	case errors.Is(err, context.Canceled):
		return statusUpdateErrorCanceled
	case errors.Is(err, context.DeadlineExceeded):
		return statusUpdateErrorDeadlineExceeded
	default:
		return statusUpdateErrorOther
	}
}