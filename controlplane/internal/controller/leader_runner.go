package controller

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/nantian-gw/gateway/controlplane/internal/observability"
)

type ReconcilerRunnerScope string

const (
	ReconcilerRunnerScopeFull          ReconcilerRunnerScope = "full"
	ReconcilerRunnerScopeInfra         ReconcilerRunnerScope = "infra"
	ReconcilerRunnerScopeGatewayStatus ReconcilerRunnerScope = "gateway-status"
	ReconcilerRunnerScopeRouteStatus   ReconcilerRunnerScope = "route-status"
	ReconcilerRunnerScopePolicyStatus  ReconcilerRunnerScope = "policy-status"
)

var reconcilerRunnerScopeOrder = []ReconcilerRunnerScope{
	ReconcilerRunnerScopeInfra,
	ReconcilerRunnerScopeGatewayStatus,
	ReconcilerRunnerScopeRouteStatus,
	ReconcilerRunnerScopePolicyStatus,
}

func (s ReconcilerRunnerScope) String() string {
	return string(normalizeReconcilerRunnerScope(s))
}

type ReconcilerRunner struct {
	interval    time.Duration
	logger      *slog.Logger
	metrics     *observability.Metrics
	mu          sync.Mutex
	reconcilers []reconcilerRunnerComponent

	trigger       chan struct{}
	pendingMu     sync.Mutex
	pendingScopes runnerScopeSet

	settleMu     sync.Mutex
	settleDelay  time.Duration
	settleTimer  *time.Timer
	settleSeq    uint64
	settleScopes runnerScopeSet

	retryMu      sync.Mutex
	retryBackoff time.Duration
	retryTimer   *time.Timer
	retrySeq     uint64
	retryScopes  runnerScopeSet
}

type runnerScopedReconciler interface {
	ReconcileScope(context.Context, ReconcilerRunnerScope) error
	ReconcilerRunnerScopes() []ReconcilerRunnerScope
}

type reconcilerRunnerComponent struct {
	name       string
	reconciler ComponentReconciler
	scoped     runnerScopedReconciler
	scopes     runnerScopeSet
}

type scopedComponentReconciler struct {
	name   string
	full   func(context.Context) error
	scoped func(context.Context, ReconcilerRunnerScope) error
	scopes runnerScopeSet
}

func NewScopedReconciler(
	name string,
	reconciler ComponentReconciler,
	scopes ...ReconcilerRunnerScope,
) ComponentReconciler {
	if reconciler == nil {
		return nil
	}
	return NewScopedReconcilerFunc(name, reconciler.Reconcile, nil, scopes...)
}

func NewScopedReconcilerFunc(
	name string,
	full func(context.Context) error,
	scoped func(context.Context, ReconcilerRunnerScope) error,
	scopes ...ReconcilerRunnerScope,
) ComponentReconciler {
	component := &scopedComponentReconciler{
		name:   name,
		full:   full,
		scoped: scoped,
		scopes: newRunnerScopeSet(scopes...),
	}
	return component
}

func (r *scopedComponentReconciler) Reconcile(ctx context.Context) error {
	if r == nil || r.full == nil {
		return nil
	}
	return r.full(ctx)
}

func (r *scopedComponentReconciler) ReconcileScope(ctx context.Context, scope ReconcilerRunnerScope) error {
	if r == nil {
		return nil
	}
	if r.scoped != nil {
		return r.scoped(ctx, normalizeReconcilerRunnerScope(scope))
	}
	return r.Reconcile(ctx)
}

func (r *scopedComponentReconciler) ReconcilerRunnerScopes() []ReconcilerRunnerScope {
	if r == nil {
		return nil
	}
	return r.scopes.sorted()
}

func NewReconcilerRunner(
	interval time.Duration,
	logger *slog.Logger,
	metrics *observability.Metrics,
	reconcilers ...ComponentReconciler,
) *ReconcilerRunner {
	runner := &ReconcilerRunner{
		interval: interval,
		logger:   logger,
		metrics:  metrics,
		trigger:  make(chan struct{}, 1),
	}
	for _, reconciler := range reconcilers {
		if component, ok := newReconcilerRunnerComponent(reconciler); ok {
			runner.reconcilers = append(runner.reconcilers, component)
		}
	}
	runner.updateQueueDepth()
	runner.updateSettlePending(false)
	runner.updateRetryPending(false)
	return runner
}

func newReconcilerRunnerComponent(reconciler ComponentReconciler) (reconcilerRunnerComponent, bool) {
	if reconciler == nil {
		return reconcilerRunnerComponent{}, false
	}
	component := reconcilerRunnerComponent{
		reconciler: reconciler,
		scopes:     newRunnerScopeSet(ReconcilerRunnerScopeFull),
	}
	if scoped, ok := reconciler.(runnerScopedReconciler); ok {
		component.scoped = scoped
		component.scopes = newRunnerScopeSet(scoped.ReconcilerRunnerScopes()...)
	}
	if named, ok := reconciler.(*scopedComponentReconciler); ok {
		component.name = named.name
	}
	return component, true
}

func (r *ReconcilerRunner) Start(ctx context.Context) error {
	defer r.stopAsyncTriggers()
	r.runOnce(ctx, ReconcilerRunnerScopeFull)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			r.runOnce(ctx, ReconcilerRunnerScopeFull)
		case <-r.trigger:
			r.updateQueueDepth()
			scopes := r.consumePendingScopes()
			r.runOnce(ctx, scopes.sortedOrFull()...)
		}
	}
}

func (r *ReconcilerRunner) NeedLeaderElection() bool {
return true
}

func (r *ReconcilerRunner) runOnce(ctx context.Context, scopes ...ReconcilerRunnerScope) {
	r.mu.Lock()
	defer r.mu.Unlock()

	requestedScopes := newRunnerScopeSet(scopes...)
	incCounter(r.metricsCounter(func(m *observability.Metrics) prometheus.Counter {
		return m.ReconcilerRunnerRunsTotal
	}))

	successfulScopes := runnerScopeSet{}
	failedScopes := runnerScopeSet{}
	for _, scope := range requestedScopes.sortedOrFull() {
		started := time.Now()
		scopeFailed := r.runScope(ctx, scope)
		observeHistogramVec(
			r.metricsHistogramVec(func(m *observability.Metrics) *prometheus.HistogramVec {
				return m.ReconcilerRunnerRunDurationSeconds
			}),
			scope.String(),
			time.Since(started).Seconds(),
		)

		if scopeFailed {
			failedScopes.merge(scope)
			continue
		}
		successfulScopes.merge(scope)
	}

	if !failedScopes.empty() {
		r.clearRetryScopes(successfulScopes)
		incCounter(r.metricsCounter(func(m *observability.Metrics) prometheus.Counter {
			return m.ReconcilerRunnerFailuresTotal
		}))
		setGauge(r.metricsGauge(func(m *observability.Metrics) prometheus.Gauge {
			return m.ReconcilerRunnerLastRunSuccess
		}), 0)
		r.scheduleRetry(failedScopes.sortedOrFull()...)
		return
	}

	r.clearRetryScopes(successfulScopes)
	setGauge(r.metricsGauge(func(m *observability.Metrics) prometheus.Gauge {
		return m.ReconcilerRunnerLastRunSuccess
	}), 1)
}

func (r *ReconcilerRunner) runScope(ctx context.Context, scope ReconcilerRunnerScope) bool {
	failed := false
	for _, reconciler := range r.reconcilers {
		if !reconciler.supports(scope) {
			continue
		}
		if err := reconciler.reconcile(ctx, scope); err != nil {
			failed = true
			attrs := []any{"scope", scope.String(), "error", err}
			if reconciler.name != "" {
				attrs = append(attrs, "reconciler", reconciler.name)
			}
			r.logger.Error("failed to reconcile controlplane state", attrs...)
		}
	}
	return failed
}

func (c reconcilerRunnerComponent) supports(scope ReconcilerRunnerScope) bool {
	scope = normalizeReconcilerRunnerScope(scope)
	if scope == ReconcilerRunnerScopeFull {
		return true
	}
	return c.scopes.contains(scope)
}

func (c reconcilerRunnerComponent) reconcile(ctx context.Context, scope ReconcilerRunnerScope) error {
	scope = normalizeReconcilerRunnerScope(scope)
	if scope == ReconcilerRunnerScopeFull || c.scoped == nil {
		return c.reconciler.Reconcile(ctx)
	}
	return c.scoped.ReconcileScope(ctx, scope)
}

func (r *ReconcilerRunner) metricsCounter(selectMetric func(*observability.Metrics) prometheus.Counter) prometheus.Counter {
	if r == nil || r.metrics == nil || selectMetric == nil {
		return nil
	}
	return selectMetric(r.metrics)
}

func (r *ReconcilerRunner) metricsGauge(selectMetric func(*observability.Metrics) prometheus.Gauge) prometheus.Gauge {
	if r == nil || r.metrics == nil || selectMetric == nil {
		return nil
	}
	return selectMetric(r.metrics)
}

func (r *ReconcilerRunner) metricsHistogramVec(selectMetric func(*observability.Metrics) *prometheus.HistogramVec) *prometheus.HistogramVec {
	if r == nil || r.metrics == nil || selectMetric == nil {
		return nil
	}
	return selectMetric(r.metrics)
}

type runnerScopeSet struct {
	values map[ReconcilerRunnerScope]struct{}
}

func newRunnerScopeSet(scopes ...ReconcilerRunnerScope) runnerScopeSet {
	set := runnerScopeSet{}
	set.merge(scopes...)
	if set.empty() {
		set.merge(ReconcilerRunnerScopeFull)
	}
	return set
}

func (s *runnerScopeSet) merge(scopes ...ReconcilerRunnerScope) {
	for _, scope := range scopes {
		s.add(scope)
	}
}

func (s *runnerScopeSet) add(scope ReconcilerRunnerScope) {
	scope = normalizeReconcilerRunnerScope(scope)
	if s.values == nil {
		s.values = make(map[ReconcilerRunnerScope]struct{})
	}
	if scope == ReconcilerRunnerScopeFull {
		clear(s.values)
		s.values[ReconcilerRunnerScopeFull] = struct{}{}
		return
	}
	if _, ok := s.values[ReconcilerRunnerScopeFull]; ok {
		return
	}
	s.values[scope] = struct{}{}
}

func (s *runnerScopeSet) remove(scope ReconcilerRunnerScope) {
	if s == nil || s.values == nil {
		return
	}
	delete(s.values, normalizeReconcilerRunnerScope(scope))
}

func (s runnerScopeSet) contains(scope ReconcilerRunnerScope) bool {
	if s.values == nil {
		return false
	}
	_, ok := s.values[normalizeReconcilerRunnerScope(scope)]
	return ok
}

func (s runnerScopeSet) empty() bool {
	return len(s.values) == 0
}

func (s runnerScopeSet) clone() runnerScopeSet {
	if len(s.values) == 0 {
		return runnerScopeSet{}
	}
	out := runnerScopeSet{values: make(map[ReconcilerRunnerScope]struct{}, len(s.values))}
	for scope := range s.values {
		out.values[scope] = struct{}{}
	}
	return out
}

func (s *runnerScopeSet) clear() {
	if s != nil {
		s.values = nil
	}
}

func (s runnerScopeSet) sorted() []ReconcilerRunnerScope {
	if len(s.values) == 0 {
		return nil
	}
	if s.contains(ReconcilerRunnerScopeFull) {
		return []ReconcilerRunnerScope{ReconcilerRunnerScopeFull}
	}
	out := make([]ReconcilerRunnerScope, 0, len(s.values))
	for _, scope := range reconcilerRunnerScopeOrder {
		if s.contains(scope) {
			out = append(out, scope)
		}
	}
	return out
}

func (s runnerScopeSet) sortedOrFull() []ReconcilerRunnerScope {
	scopes := s.sorted()
	if len(scopes) == 0 {
		return []ReconcilerRunnerScope{ReconcilerRunnerScopeFull}
	}
	return scopes
}

func normalizeReconcilerRunnerScope(scope ReconcilerRunnerScope) ReconcilerRunnerScope {
	switch scope {
	case ReconcilerRunnerScopeInfra,
		ReconcilerRunnerScopeGatewayStatus,
		ReconcilerRunnerScopeRouteStatus,
		ReconcilerRunnerScopePolicyStatus,
		ReconcilerRunnerScopeFull:
		return scope
	default:
		return ReconcilerRunnerScopeFull
	}
}

func incCounter(counter prometheus.Counter) {
	if counter != nil {
		counter.Inc()
	}
}

func setGauge(gauge prometheus.Gauge, value float64) {
	if gauge != nil {
		gauge.Set(value)
	}
}

func observeHistogramVec(histogram *prometheus.HistogramVec, label string, value float64) {
	if histogram != nil {
		histogram.WithLabelValues(label).Observe(value)
	}
}

func observeHistogram(histogram prometheus.Histogram, value float64) {
	if histogram != nil {
		histogram.Observe(value)
	}
}
