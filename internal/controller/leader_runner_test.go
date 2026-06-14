package controller

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/nantian-gw/gateway/internal/observability"
)

func TestReconcilerRunnerQueueMetricsDeduplicateTriggers(t *testing.T) {
	runner := NewReconcilerRunner(
		time.Second,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		testReconcilerRunnerMetrics(),
	)

	if got := testutil.ToFloat64(runner.metrics.ReconcilerRunnerQueueDepth); got != 0 {
		t.Fatalf("initial queue depth = %v, want 0", got)
	}

	runner.QueueRun()
	runner.QueueRun()

	if got := testutil.ToFloat64(runner.metrics.ReconcilerRunnerTriggerEnqueuedTotal); got != 1 {
		t.Fatalf("enqueued triggers = %v, want 1", got)
	}
	if got := testutil.ToFloat64(runner.metrics.ReconcilerRunnerTriggerDedupedTotal); got != 1 {
		t.Fatalf("deduped triggers = %v, want 1", got)
	}
	if got := testutil.ToFloat64(runner.metrics.ReconcilerRunnerQueueDepth); got != 1 {
		t.Fatalf("queue depth after enqueue = %v, want 1", got)
	}

	<-runner.trigger
	runner.updateQueueDepth()

	if got := testutil.ToFloat64(runner.metrics.ReconcilerRunnerQueueDepth); got != 0 {
		t.Fatalf("queue depth after dequeue = %v, want 0", got)
	}
}

func TestReconcilerRunnerSettleDelayCoalescesBurstTriggers(t *testing.T) {
	runner := NewReconcilerRunner(
		time.Second,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		testReconcilerRunnerMetrics(),
	)
	runner.SetSettleDelay(20 * time.Millisecond)
	defer runner.stopAsyncTriggers()

	runner.QueueRun()
	runner.QueueRun()

	if got := testutil.ToFloat64(runner.metrics.ReconcilerRunnerQueueDepth); got != 0 {
		t.Fatalf("queue depth before settle fires = %v, want 0", got)
	}
	if got := testutil.ToFloat64(runner.metrics.ReconcilerRunnerSettlePending); got != 1 {
		t.Fatalf("settle pending = %v, want 1", got)
	}
	if got := testutil.ToFloat64(runner.metrics.ReconcilerRunnerTriggerSettledTotal); got != 2 {
		t.Fatalf("settled triggers = %v, want 2", got)
	}

	waitForCondition(t, 200*time.Millisecond, func() bool {
		return testutil.ToFloat64(runner.metrics.ReconcilerRunnerQueueDepth) == 1
	}, "expected settle timer to enqueue a single trigger")

	if got := testutil.ToFloat64(runner.metrics.ReconcilerRunnerTriggerEnqueuedTotal); got != 1 {
		t.Fatalf("enqueued triggers after settle = %v, want 1", got)
	}
	if got := testutil.ToFloat64(runner.metrics.ReconcilerRunnerSettlePending); got != 0 {
		t.Fatalf("settle pending after fire = %v, want 0", got)
	}
}

func TestReconcilerRunnerQueueRunImmediateBypassesSettleDelay(t *testing.T) {
	runner := NewReconcilerRunner(
		time.Second,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		testReconcilerRunnerMetrics(),
	)
	runner.SetSettleDelay(20 * time.Millisecond)
	defer runner.stopAsyncTriggers()

	runner.QueueRunImmediate()

	if got := testutil.ToFloat64(runner.metrics.ReconcilerRunnerQueueDepth); got != 1 {
		t.Fatalf("queue depth after immediate enqueue = %v, want 1", got)
	}
	if got := testutil.ToFloat64(runner.metrics.ReconcilerRunnerTriggerEnqueuedTotal); got != 1 {
		t.Fatalf("enqueued triggers after immediate enqueue = %v, want 1", got)
	}
	if got := testutil.ToFloat64(runner.metrics.ReconcilerRunnerSettlePending); got != 0 {
		t.Fatalf("settle pending after immediate enqueue = %v, want 0", got)
	}
}

func TestReconcilerRunnerRunsOnlyMatchingScopedReconcilers(t *testing.T) {
	infra := &recordingReconciler{}
	status := &recordingReconciler{}
	runner := NewReconcilerRunner(
		time.Second,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		testReconcilerRunnerMetrics(),
		NewScopedReconciler("infra", infra, ReconcilerRunnerScopeInfra),
		NewScopedReconciler("status", status, ReconcilerRunnerScopeGatewayStatus, ReconcilerRunnerScopeRouteStatus, ReconcilerRunnerScopePolicyStatus),
	)

	runner.runOnce(context.Background(), ReconcilerRunnerScopeInfra)

	if got := infra.calls(); got != 1 {
		t.Fatalf("infra calls after infra scope = %d, want 1", got)
	}
	if got := status.calls(); got != 0 {
		t.Fatalf("status calls after infra scope = %d, want 0", got)
	}

	runner.runOnce(context.Background(), ReconcilerRunnerScopeRouteStatus)

	if got := infra.calls(); got != 1 {
		t.Fatalf("infra calls after route-status scope = %d, want 1", got)
	}
	if got := status.calls(); got != 1 {
		t.Fatalf("status calls after route-status scope = %d, want 1", got)
	}

	runner.runOnce(context.Background(), ReconcilerRunnerScopeFull)

	if got := infra.calls(); got != 2 {
		t.Fatalf("infra calls after full scope = %d, want 2", got)
	}
	if got := status.calls(); got != 2 {
		t.Fatalf("status calls after full scope = %d, want 2", got)
	}
}

func TestReconcilerRunnerSettleDelayMergesScopedTriggers(t *testing.T) {
	infra := &recordingReconciler{}
	status := &recordingReconciler{}
	runner := NewReconcilerRunner(
		time.Second,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		testReconcilerRunnerMetrics(),
		NewScopedReconciler("infra", infra, ReconcilerRunnerScopeInfra),
		NewScopedReconciler("status", status, ReconcilerRunnerScopeRouteStatus),
	)
	runner.SetSettleDelay(20 * time.Millisecond)
	defer runner.stopAsyncTriggers()

	runner.QueueRunForScope(ReconcilerRunnerScopeInfra)
	runner.QueueRunForScope(ReconcilerRunnerScopeRouteStatus)

	waitForCondition(t, 200*time.Millisecond, func() bool {
		return testutil.ToFloat64(runner.metrics.ReconcilerRunnerQueueDepth) == 1
	}, "expected settle timer to enqueue merged scoped trigger")

	<-runner.trigger
	runner.updateQueueDepth()
	runner.runOnce(context.Background(), runner.consumePendingScopes().sorted()...)

	if got := infra.calls(); got != 1 {
		t.Fatalf("infra calls after merged settled trigger = %d, want 1", got)
	}
	if got := status.calls(); got != 1 {
		t.Fatalf("status calls after merged settled trigger = %d, want 1", got)
	}
}

func TestReconcilerRunnerRetryPreservesFailedScope(t *testing.T) {
	infra := &recordingReconciler{}
	status := &recordingReconciler{err: errors.New("boom")}
	runner := NewReconcilerRunner(
		time.Second,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		testReconcilerRunnerMetrics(),
		NewScopedReconciler("infra", infra, ReconcilerRunnerScopeInfra),
		NewScopedReconciler("status", status, ReconcilerRunnerScopeRouteStatus),
	)
	runner.SetRetryBackoff(20 * time.Millisecond)
	defer runner.stopAsyncTriggers()

	runner.runOnce(context.Background(), ReconcilerRunnerScopeRouteStatus)
	status.setErr(nil)

	waitForCondition(t, 200*time.Millisecond, func() bool {
		return testutil.ToFloat64(runner.metrics.ReconcilerRunnerQueueDepth) == 1
	}, "expected retry backoff to enqueue route-status trigger")

	<-runner.trigger
	runner.updateQueueDepth()
	runner.runOnce(context.Background(), runner.consumePendingScopes().sorted()...)

	if got := infra.calls(); got != 0 {
		t.Fatalf("infra calls after route-status retry = %d, want 0", got)
	}
	if got := status.calls(); got != 2 {
		t.Fatalf("status calls after route-status retry = %d, want 2", got)
	}
}

func TestReconcilerRunnerRecordsRunMetrics(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		runner := NewReconcilerRunner(
			time.Second,
			slog.New(slog.NewTextHandler(io.Discard, nil)),
			testReconcilerRunnerMetrics(),
			staticReconciler{},
		)

		runner.runOnce(context.Background())

		if got := testutil.ToFloat64(runner.metrics.ReconcilerRunnerRunsTotal); got != 1 {
			t.Fatalf("runs total = %v, want 1", got)
		}
		if got := testutil.ToFloat64(runner.metrics.ReconcilerRunnerFailuresTotal); got != 0 {
			t.Fatalf("failures total = %v, want 0", got)
		}
		if got := testutil.ToFloat64(runner.metrics.ReconcilerRunnerLastRunSuccess); got != 1 {
			t.Fatalf("last run success = %v, want 1", got)
		}
		if got := histogramVecSampleCount(t, runner.metrics.ReconcilerRunnerRunDurationSeconds, ReconcilerRunnerScopeFull.String()); got != 1 {
			t.Fatalf("run duration sample count = %d, want 1", got)
		}
	})

	t.Run("failure", func(t *testing.T) {
		runner := NewReconcilerRunner(
			time.Second,
			slog.New(slog.NewTextHandler(io.Discard, nil)),
			testReconcilerRunnerMetrics(),
			staticReconciler{err: errors.New("boom")},
		)

		runner.runOnce(context.Background())

		if got := testutil.ToFloat64(runner.metrics.ReconcilerRunnerRunsTotal); got != 1 {
			t.Fatalf("runs total = %v, want 1", got)
		}
		if got := testutil.ToFloat64(runner.metrics.ReconcilerRunnerFailuresTotal); got != 1 {
			t.Fatalf("failures total = %v, want 1", got)
		}
		if got := testutil.ToFloat64(runner.metrics.ReconcilerRunnerLastRunSuccess); got != 0 {
			t.Fatalf("last run success = %v, want 0", got)
		}
		if got := histogramVecSampleCount(t, runner.metrics.ReconcilerRunnerRunDurationSeconds, ReconcilerRunnerScopeFull.String()); got != 1 {
			t.Fatalf("run duration sample count = %d, want 1", got)
		}
	})
}

func TestReconcilerRunnerRecordsDurationMetricsByScope(t *testing.T) {
	runner := NewReconcilerRunner(
		time.Second,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		testReconcilerRunnerMetrics(),
		NewScopedReconciler("infra", staticReconciler{}, ReconcilerRunnerScopeInfra),
		NewScopedReconciler("status", staticReconciler{}, ReconcilerRunnerScopeRouteStatus),
	)

	runner.runOnce(context.Background(), ReconcilerRunnerScopeInfra, ReconcilerRunnerScopeRouteStatus)

	if got := histogramVecSampleCount(t, runner.metrics.ReconcilerRunnerRunDurationSeconds, ReconcilerRunnerScopeInfra.String()); got != 1 {
		t.Fatalf("infra duration sample count = %d, want 1", got)
	}
	if got := histogramVecSampleCount(t, runner.metrics.ReconcilerRunnerRunDurationSeconds, ReconcilerRunnerScopeRouteStatus.String()); got != 1 {
		t.Fatalf("route-status duration sample count = %d, want 1", got)
	}
	if got := histogramVecSampleCount(t, runner.metrics.ReconcilerRunnerRunDurationSeconds, ReconcilerRunnerScopeFull.String()); got != 0 {
		t.Fatalf("full duration sample count = %d, want 0", got)
	}
}

func TestReconcilerRunnerCreatesScopeSpans(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	original := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer func() { otel.SetTracerProvider(original) }()

	runner := NewReconcilerRunner(
		time.Second,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		testReconcilerRunnerMetrics(),
		staticReconciler{},
	)

	runner.runOnce(context.Background(), ReconcilerRunnerScopeInfra)

	names := spanNames(exporter.GetSpans())
	if !slices.Contains(names, "controlplane.reconciler_runner.run") {
		t.Fatalf("expected run span, got %v", names)
	}
	if !slices.Contains(names, "controlplane.reconciler_runner.scope") {
		t.Fatalf("expected scope span, got %v", names)
	}
}

func TestReconcilerRunnerScopesForSnapshotBuildScope(t *testing.T) {
	tests := []struct {
		name  string
		scope snapshotBuildScope
		want  []ReconcilerRunnerScope
	}{
		{
			name:  "full",
			scope: snapshotBuildScopeFull,
			want:  []ReconcilerRunnerScope{ReconcilerRunnerScopeFull},
		},
		{
			name:  "gateway listeners",
			scope: snapshotBuildScopeGatewayListeners,
			want:  []ReconcilerRunnerScope{ReconcilerRunnerScopeInfra, ReconcilerRunnerScopeGatewayStatus, ReconcilerRunnerScopeRouteStatus},
		},
		{
			name:  "route change",
			scope: snapshotBuildScopeRoutes,
			want:  []ReconcilerRunnerScope{ReconcilerRunnerScopeInfra, ReconcilerRunnerScopeGatewayStatus, ReconcilerRunnerScopeRouteStatus, ReconcilerRunnerScopePolicyStatus},
		},
		{
			name:  "backend policy metadata",
			scope: snapshotBuildScopeBackends | snapshotBuildScopeRouteBackendRefs,
			want:  []ReconcilerRunnerScope{ReconcilerRunnerScopeRouteStatus, ReconcilerRunnerScopePolicyStatus},
		},
		{
			name:  "mesh listeners",
			scope: snapshotBuildScopeMeshListeners,
			want:  []ReconcilerRunnerScope{ReconcilerRunnerScopeInfra, ReconcilerRunnerScopeGatewayStatus, ReconcilerRunnerScopeRouteStatus},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reconcilerRunnerScopesForSnapshotBuildScope(tt.scope)
			if !sameRunnerScopes(got, tt.want) {
				t.Fatalf("runner scopes = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReconcilerRunnerFailureSchedulesRetry(t *testing.T) {
	runner := NewReconcilerRunner(
		time.Second,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		testReconcilerRunnerMetrics(),
		staticReconciler{err: errors.New("boom")},
	)
	runner.SetRetryBackoff(20 * time.Millisecond)
	defer runner.stopAsyncTriggers()

	runner.runOnce(context.Background())

	if got := testutil.ToFloat64(runner.metrics.ReconcilerRunnerRetriesScheduledTotal); got != 1 {
		t.Fatalf("scheduled retries = %v, want 1", got)
	}
	if got := testutil.ToFloat64(runner.metrics.ReconcilerRunnerRetryPending); got != 1 {
		t.Fatalf("retry pending after failure = %v, want 1", got)
	}

	waitForCondition(t, 200*time.Millisecond, func() bool {
		return testutil.ToFloat64(runner.metrics.ReconcilerRunnerQueueDepth) == 1
	}, "expected retry backoff to enqueue a trigger")

	if got := testutil.ToFloat64(runner.metrics.ReconcilerRunnerRetryPending); got != 0 {
		t.Fatalf("retry pending after enqueue = %v, want 0", got)
	}
}

func TestReconcilerRunnerSuccessCancelsPendingRetry(t *testing.T) {
	reconciler := &mutableReconciler{err: errors.New("boom")}
	runner := NewReconcilerRunner(
		time.Second,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		testReconcilerRunnerMetrics(),
		reconciler,
	)
	runner.SetRetryBackoff(40 * time.Millisecond)
	defer runner.stopAsyncTriggers()

	runner.runOnce(context.Background())
	if got := testutil.ToFloat64(runner.metrics.ReconcilerRunnerRetryPending); got != 1 {
		t.Fatalf("retry pending after failure = %v, want 1", got)
	}

	reconciler.setErr(nil)
	runner.runOnce(context.Background())

	if got := testutil.ToFloat64(runner.metrics.ReconcilerRunnerRetryPending); got != 0 {
		t.Fatalf("retry pending after success = %v, want 0", got)
	}

	time.Sleep(80 * time.Millisecond)
	if got := testutil.ToFloat64(runner.metrics.ReconcilerRunnerQueueDepth); got != 0 {
		t.Fatalf("queue depth after canceled retry = %v, want 0", got)
	}
}

type staticReconciler struct {
	err error
}

func (r staticReconciler) Reconcile(context.Context) error {
	return r.err
}

type mutableReconciler struct {
	mu  sync.Mutex
	err error
}

func (r *mutableReconciler) Reconcile(context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}

func (r *mutableReconciler) setErr(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.err = err
}

type recordingReconciler struct {
	mu  sync.Mutex
	err error
	n   int
}

func (r *recordingReconciler) Reconcile(context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.n++
	return r.err
}

func (r *recordingReconciler) calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.n
}

func (r *recordingReconciler) setErr(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.err = err
}

func spanNames(spans tracetest.SpanStubs) []string {
	names := make([]string, 0, len(spans))
	for _, span := range spans {
		names = append(names, span.Name)
	}
	return names
}

func testReconcilerRunnerMetrics() *observability.Metrics {
	return &observability.Metrics{
		ReconcilerRunnerRunsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "test_reconciler_runner_runs_total",
			Help: "test",
		}),
		ReconcilerRunnerFailuresTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "test_reconciler_runner_failures_total",
			Help: "test",
		}),
		ReconcilerRunnerLastRunSuccess: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "test_reconciler_runner_last_run_success",
			Help: "test",
		}),
		ReconcilerRunnerRunDurationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "test_reconciler_runner_duration_seconds",
				Help:    "test",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"scope"},
		),
		ReconcilerRunnerQueueDepth: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "test_reconciler_runner_queue_depth",
			Help: "test",
		}),
		ReconcilerRunnerTriggerEnqueuedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "test_reconciler_runner_triggers_enqueued_total",
			Help: "test",
		}),
		ReconcilerRunnerTriggerDedupedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "test_reconciler_runner_triggers_deduped_total",
			Help: "test",
		}),
		ReconcilerRunnerTriggerSettledTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "test_reconciler_runner_triggers_settled_total",
			Help: "test",
		}),
		ReconcilerRunnerSettlePending: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "test_reconciler_runner_settle_pending",
			Help: "test",
		}),
		ReconcilerRunnerRetriesScheduledTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "test_reconciler_runner_retries_scheduled_total",
			Help: "test",
		}),
		ReconcilerRunnerRetryPending: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "test_reconciler_runner_retry_pending",
			Help: "test",
		}),
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool, message string) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal(message)
}

func histogramVecSampleCount(t *testing.T, histogram *prometheus.HistogramVec, label string) uint64 {
	t.Helper()

	observer, err := histogram.GetMetricWithLabelValues(label)
	if err != nil {
		t.Fatalf("get histogram metric: %v", err)
	}
	metric, ok := observer.(prometheus.Metric)
	if !ok {
		t.Fatal("histogram observer does not implement prometheus.Metric")
	}

	dtoMetric := &dto.Metric{}
	if err := metric.Write(dtoMetric); err != nil {
		t.Fatalf("write histogram metric: %v", err)
	}
	return dtoMetric.GetHistogram().GetSampleCount()
}

func sameRunnerScopes(left, right []ReconcilerRunnerScope) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
