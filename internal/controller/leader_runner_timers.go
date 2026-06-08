package controller

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/nantian-gw/gateway/internal/observability"
)

func (r *ReconcilerRunner) scheduleRetry(scopes ...ReconcilerRunnerScope) {
	r.retryMu.Lock()
	defer r.retryMu.Unlock()

	if r.retryBackoff <= 0 {
		return
	}
	r.retryScopes.merge(scopes...)
	if r.retryTimer != nil {
		r.updateRetryPending(!r.retryScopes.empty())
		return
	}

	incCounter(r.metricsCounter(func(m *observability.Metrics) prometheus.Counter {
		return m.ReconcilerRunnerRetriesScheduledTotal
	}))
	r.updateRetryPending(true)
	r.retrySeq++
	seq := r.retrySeq

	r.retryTimer = time.AfterFunc(r.retryBackoff, func() {
		retryScopes, ok := r.completeRetryTimer(seq)
		if !ok || retryScopes.empty() {
			return
		}
		r.QueueRunForScopes(retryScopes.sortedOrFull()...)
	})
}

func (r *ReconcilerRunner) clearRetryScopes(scopes runnerScopeSet) {
	r.retryMu.Lock()
	defer r.retryMu.Unlock()

	if r.retryScopes.empty() {
		return
	}
	if scopes.contains(ReconcilerRunnerScopeFull) {
		r.stopRetryTimerLocked()
		return
	}
	for _, scope := range scopes.sorted() {
		r.retryScopes.remove(scope)
	}
	if r.retryScopes.empty() {
		r.stopRetryTimerLocked()
		return
	}
	r.updateRetryPending(true)
}

func (r *ReconcilerRunner) stopAsyncTriggers() {
	r.stopSettleRun()
	r.stopRetryRun()
}

func (r *ReconcilerRunner) stopSettleRun() {
	r.settleMu.Lock()
	defer r.settleMu.Unlock()
	r.stopSettleTimerLocked()
}

func (r *ReconcilerRunner) stopSettleTimerLocked() {
	if r.settleTimer != nil {
		r.settleTimer.Stop()
		r.settleTimer = nil
	}
	r.settleSeq++
	r.settleScopes.clear()
	r.updateSettlePending(false)
}

func (r *ReconcilerRunner) completeSettleTimer(seq uint64) (runnerScopeSet, bool) {
	r.settleMu.Lock()
	defer r.settleMu.Unlock()

	if r.settleSeq == seq {
		r.settleTimer = nil
		scopes := r.settleScopes.clone()
		r.settleScopes.clear()
		r.updateSettlePending(false)
		return scopes, true
	}
	return runnerScopeSet{}, false
}

func (r *ReconcilerRunner) stopRetryRun() {
	r.retryMu.Lock()
	defer r.retryMu.Unlock()
	r.stopRetryTimerLocked()
}

func (r *ReconcilerRunner) stopRetryTimerLocked() {
	if r.retryTimer != nil {
		r.retryTimer.Stop()
		r.retryTimer = nil
	}
	r.retrySeq++
	r.retryScopes.clear()
	r.updateRetryPending(false)
}

func (r *ReconcilerRunner) completeRetryTimer(seq uint64) (runnerScopeSet, bool) {
	r.retryMu.Lock()
	defer r.retryMu.Unlock()

	if r.retrySeq == seq {
		r.retryTimer = nil
		scopes := r.retryScopes.clone()
		r.retryScopes.clear()
		r.updateRetryPending(false)
		return scopes, true
	}
	return runnerScopeSet{}, false
}