package controller

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/nantian-gw/gateway/controlplane/internal/observability"
)

func (r *ReconcilerRunner) SetSettleDelay(delay time.Duration) {
	r.settleMu.Lock()
	defer r.settleMu.Unlock()

	r.settleDelay = delay
	if delay <= 0 {
		r.stopSettleTimerLocked()
	}
}

func (r *ReconcilerRunner) SetRetryBackoff(backoff time.Duration) {
	r.retryMu.Lock()
	defer r.retryMu.Unlock()

	r.retryBackoff = backoff
	if backoff <= 0 {
		r.stopRetryTimerLocked()
	}
}

func (r *ReconcilerRunner) QueueRun() {
	r.QueueRunForScopes(ReconcilerRunnerScopeFull)
}

func (r *ReconcilerRunner) QueueRunForScope(scope ReconcilerRunnerScope) {
	r.QueueRunForScopes(scope)
}

func (r *ReconcilerRunner) QueueRunForScopes(scopes ...ReconcilerRunnerScope) {
	r.settleMu.Lock()
	delay := r.settleDelay
	r.settleMu.Unlock()
	if delay > 0 {
		r.queueSettledRun(delay, scopes...)
		return
	}
	r.enqueueTriggerForScopes(scopes...)
}

func (r *ReconcilerRunner) QueueRunImmediate() {
	r.QueueRunImmediateForScopes(ReconcilerRunnerScopeFull)
}

func (r *ReconcilerRunner) QueueRunImmediateForScope(scope ReconcilerRunnerScope) {
	r.QueueRunImmediateForScopes(scope)
}

func (r *ReconcilerRunner) QueueRunImmediateForScopes(scopes ...ReconcilerRunnerScope) {
	r.enqueueTriggerForScopes(scopes...)
}

func (r *ReconcilerRunner) enqueueTriggerForScopes(scopes ...ReconcilerRunnerScope) {
	r.mergePendingScopes(scopes...)

	select {
	case r.trigger <- struct{}{}:
		incCounter(r.metricsCounter(func(m *observability.Metrics) prometheus.Counter {
			return m.ReconcilerRunnerTriggerEnqueuedTotal
		}))
		r.updateQueueDepth()
	default:
		incCounter(r.metricsCounter(func(m *observability.Metrics) prometheus.Counter {
			return m.ReconcilerRunnerTriggerDedupedTotal
		}))
	}
}

func (r *ReconcilerRunner) queueSettledRun(delay time.Duration, scopes ...ReconcilerRunnerScope) {
	r.settleMu.Lock()
	defer r.settleMu.Unlock()

	if delay <= 0 {
		r.enqueueTriggerForScopes(scopes...)
		return
	}

	r.settleScopes.merge(scopes...)
	if r.settleTimer != nil {
		r.settleTimer.Stop()
	}
	r.settleSeq++
	seq := r.settleSeq
	incCounter(r.metricsCounter(func(m *observability.Metrics) prometheus.Counter {
		return m.ReconcilerRunnerTriggerSettledTotal
	}))
	r.updateSettlePending(true)

	r.settleTimer = time.AfterFunc(delay, func() {
		settledScopes, ok := r.completeSettleTimer(seq)
		if !ok {
			return
		}
		r.enqueueTriggerForScopes(settledScopes.sortedOrFull()...)
	})
}

func (r *ReconcilerRunner) mergePendingScopes(scopes ...ReconcilerRunnerScope) {
	r.pendingMu.Lock()
	defer r.pendingMu.Unlock()
	r.pendingScopes.merge(scopes...)
}

func (r *ReconcilerRunner) consumePendingScopes() runnerScopeSet {
	r.pendingMu.Lock()
	defer r.pendingMu.Unlock()

	scopes := r.pendingScopes.clone()
	r.pendingScopes.clear()
	return scopes
}

func (r *ReconcilerRunner) updateQueueDepth() {
	setGauge(r.metricsGauge(func(m *observability.Metrics) prometheus.Gauge {
		return m.ReconcilerRunnerQueueDepth
	}), float64(len(r.trigger)))
}

func (r *ReconcilerRunner) updateSettlePending(pending bool) {
	value := 0.0
	if pending {
		value = 1
	}
	setGauge(r.metricsGauge(func(m *observability.Metrics) prometheus.Gauge {
		return m.ReconcilerRunnerSettlePending
	}), value)
}

func (r *ReconcilerRunner) updateRetryPending(pending bool) {
	value := 0.0
	if pending {
		value = 1
	}
	setGauge(r.metricsGauge(func(m *observability.Metrics) prometheus.Gauge {
		return m.ReconcilerRunnerRetryPending
	}), value)
}