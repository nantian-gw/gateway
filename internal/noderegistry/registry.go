package noderegistry

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/observability"
)

const (
	defaultPersistTimeout  = 2 * time.Second
	defaultPersistDebounce = 250 * time.Millisecond
	persistQueueSize       = 128
)

type Repository interface {
	Get(ctx context.Context, nodeID string) (ir.NodeStatus, bool, error)
	List(ctx context.Context) ([]ir.NodeStatus, error)
	Upsert(ctx context.Context, status ir.NodeStatus) error
}

type Options struct {
	PersistTimeout  time.Duration
	PersistDebounce time.Duration
	Metrics         *observability.Metrics
	BaseContext     context.Context
}

type persistRequest struct {
	status    ir.NodeStatus
	immediate bool
}

type publishObservation struct {
	version string
	at      time.Time
}

type Registry struct {
	local            *ir.NodeStatusStore
	repository       Repository
	logger           *slog.Logger
	metrics          *observability.Metrics
	baseContext      context.Context
	persistTimeout   time.Duration
	persistDebounce  time.Duration
	onChange         func()
	persistSignal    chan struct{}
	persistImmediate []persistRequest
	persistDebounced map[string]persistRequest
	persistMu        sync.RWMutex
	persistClosed    bool
	persistDone      chan struct{}
	closeOnce        sync.Once
	publishMu        sync.Mutex
	published        map[string]publishObservation
}

func NewRegistry(local *ir.NodeStatusStore, repository Repository, logger *slog.Logger, opts Options) *Registry {
	if local == nil {
		local = ir.NewNodeStatusStore()
	}
	if logger == nil {
		logger = slog.Default()
	}
	if opts.PersistTimeout <= 0 {
		opts.PersistTimeout = defaultPersistTimeout
	}
	if opts.PersistDebounce <= 0 {
		opts.PersistDebounce = defaultPersistDebounce
	}
	if opts.BaseContext == nil {
		opts.BaseContext = context.Background()
	}

	registry := &Registry{
		local:           local,
		repository:      repository,
		logger:          logger,
		metrics:         opts.Metrics,
		baseContext:     opts.BaseContext,
		persistTimeout:  opts.PersistTimeout,
		persistDebounce: opts.PersistDebounce,
		published:       make(map[string]publishObservation),
	}
	if repository != nil {
		registry.persistSignal = make(chan struct{}, 1)
		registry.persistDebounced = make(map[string]persistRequest, persistQueueSize)
		registry.persistDone = make(chan struct{})
		go registry.runPersister()
	}

	return registry
}

func (r *Registry) Connect(ctx context.Context, nodeID, cluster string, subscriptions []string, now time.Time) {
	r.ConnectWithFeatures(ctx, nodeID, cluster, subscriptions, nil, now)
}

func (r *Registry) ConnectWithFeatures(
	ctx context.Context,
	nodeID, cluster string,
	subscriptions []string,
	supportedFeatures []string,
	now time.Time,
) {
	ctx, cancel := r.operationContext(ctx)
	defer cancel()

	r.seedNode(ctx, nodeID)
	previous, _ := r.local.Get(nodeID)
	status := r.local.ConnectWithFeatures(nodeID, cluster, subscriptions, supportedFeatures, now.UTC())
	r.clearPublishObservation(nodeID)
	r.persistUpdatedStatus(previous, status)
	r.notifyIfRoutingStateChanged(previous, status)
}

func (r *Registry) Disconnect(ctx context.Context, nodeID string, now time.Time) {
	r.DisconnectWithReason(ctx, nodeID, "", "", now)
}

func (r *Registry) DisconnectWithMessage(ctx context.Context, nodeID, message string, now time.Time) {
	r.DisconnectWithReason(ctx, nodeID, "", message, now)
}

func (r *Registry) DisconnectWithReason(ctx context.Context, nodeID, reason, message string, now time.Time) {
	ctx, cancel := r.operationContext(ctx)
	defer cancel()

	r.seedNode(ctx, nodeID)
	previous, _ := r.local.Get(nodeID)
	status, ok := r.local.DisconnectWithReason(nodeID, now.UTC(), reason, message)
	if !ok {
		return
	}
	r.clearPublishObservation(nodeID)
	r.persistUpdatedStatus(previous, status)
	r.notifyIfRoutingStateChanged(previous, status)
}

func (r *Registry) ObserveAck(
	ctx context.Context,
	nodeID, cluster, version, nonce string,
	subscriptions []string,
	now time.Time,
) {
	r.ObserveAckWithFeatures(ctx, nodeID, cluster, version, nonce, subscriptions, nil, now)
}

func (r *Registry) ObserveAckWithFeatures(
	ctx context.Context,
	nodeID, cluster, version, nonce string,
	subscriptions []string,
	supportedFeatures []string,
	now time.Time,
) {
	ctx, cancel := r.operationContext(ctx)
	defer cancel()

	r.seedNode(ctx, nodeID)
	previous, _ := r.local.Get(nodeID)
	status := r.local.ObserveAckWithFeatures(nodeID, cluster, version, nonce, subscriptions, supportedFeatures, now.UTC())
	r.observePublishAckLag(nodeID, version, now.UTC())
	r.persistUpdatedStatus(previous, status)
	r.notifyIfRoutingStateChanged(previous, status)
}

func (r *Registry) ObserveNack(
	ctx context.Context,
	nodeID, cluster, version, nonce, message string,
	subscriptions []string,
	now time.Time,
) {
	r.ObserveNackWithFeatures(ctx, nodeID, cluster, version, nonce, message, subscriptions, nil, now)
}

func (r *Registry) ObserveNackWithFeatures(
	ctx context.Context,
	nodeID, cluster, version, nonce, message string,
	subscriptions []string,
	supportedFeatures []string,
	now time.Time,
) {
	ctx, cancel := r.operationContext(ctx)
	defer cancel()

	r.seedNode(ctx, nodeID)
	previous, _ := r.local.Get(nodeID)
	status := r.local.ObserveNackWithFeatures(
		nodeID,
		cluster,
		version,
		nonce,
		message,
		subscriptions,
		supportedFeatures,
		now.UTC(),
	)
	r.observePublishNackLag(nodeID, version, now.UTC())
	r.persistUpdatedStatus(previous, status)
	r.notifyIfRoutingStateChanged(previous, status)
}

func (r *Registry) ObservePublished(ctx context.Context, nodeID, version string, now time.Time) {
	ctx, cancel := r.operationContext(ctx)
	defer cancel()

	r.seedNode(ctx, nodeID)
	previous, _ := r.local.Get(nodeID)
	status, ok := r.local.ObservePublished(nodeID, version, now.UTC())
	if !ok {
		return
	}
	r.recordPublishObservation(nodeID, version, now.UTC())
	r.persistUpdatedStatus(previous, status)
}

func (r *Registry) ObserveReport(
	ctx context.Context,
	nodeID, version string,
	ready bool,
	message string,
	observedAt time.Time,
) {
	ctx, cancel := r.operationContext(ctx)
	defer cancel()

	r.seedNode(ctx, nodeID)
	previous, _ := r.local.Get(nodeID)
	status := r.local.ObserveReport(nodeID, version, ready, message, observedAt.UTC())
	r.persistUpdatedStatus(previous, status)
	r.notifyIfRoutingStateChanged(previous, status)
}

func (r *Registry) SetOnChange(onChange func()) {
	r.onChange = onChange
}

func (r *Registry) Close() {
	if r.persistSignal == nil || r.persistDone == nil {
		return
	}

	r.closeOnce.Do(func() {
		r.persistMu.Lock()
		r.persistClosed = true
		close(r.persistSignal)
		r.persistMu.Unlock()
		<-r.persistDone
	})
}

func (r *Registry) Get(ctx context.Context, nodeID string) (ir.NodeStatus, bool) {
	items := r.List(ctx)
	for _, item := range items {
		if item.NodeID == strings.TrimSpace(nodeID) {
			return item, true
		}
	}

	return ir.NodeStatus{}, false
}

func (r *Registry) List(ctx context.Context) []ir.NodeStatus {
	local := r.local.List()
	if r.repository == nil {
		return filterStaleNodes(local, time.Now().UTC())
	}

	readCtx, cancel := r.operationContext(ctx)
	defer cancel()

	shared, err := r.repository.List(readCtx)
	if err != nil {
		r.logger.Warn("failed to list shared node status", "error", err)
		return filterStaleNodes(local, time.Now().UTC())
	}

	return filterStaleNodes(mergeSharedAuthoritative(shared, local), time.Now().UTC())
}

func Merge(groups ...[]ir.NodeStatus) []ir.NodeStatus {
	merged := make(map[string]ir.NodeStatus)
	for _, group := range groups {
		for _, item := range group {
			if item.NodeID == "" {
				continue
			}
			current, ok := merged[item.NodeID]
			if !ok {
				merged[item.NodeID] = clone(item)
				continue
			}
			merged[item.NodeID] = prefer(current, item)
		}
	}

	out := make([]ir.NodeStatus, 0, len(merged))
	for _, item := range merged {
		out = append(out, clone(item))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].NodeID < out[j].NodeID
	})

	return out
}

func (r *Registry) seedNode(ctx context.Context, nodeID string) {
	if r.repository == nil || nodeID == "" {
		return
	}
	if _, ok := r.local.Get(nodeID); ok {
		return
	}

	status, ok, err := r.repository.Get(ctx, nodeID)
	if err != nil {
		r.logger.Warn("failed to read shared node status", "node_id", nodeID, "error", err)
		return
	}
	if !ok {
		return
	}

	r.local.Upsert(status)
}

func (r *Registry) operationContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = r.baseContext
	}
	if r.persistTimeout <= 0 {
		return context.WithCancel(parent)
	}

	return context.WithTimeout(parent, r.persistTimeout)
}

func (r *Registry) persistUpdatedStatus(previous, current ir.NodeStatus) {
	if r.repository == nil || current.NodeID == "" {
		return
	}

	request := persistRequest{
		status:    clone(current),
		immediate: routingStateChanged(previous, current),
	}

	if r.persistSignal == nil {
		r.recordPersistAccepted(request)
		r.persistNow(request.status)
		return
	}

	if !r.enqueuePersistRequest(request) {
		r.recordPersistDrop()
	}
}

func (r *Registry) persistNow(status ir.NodeStatus) {
	if r.repository == nil || status.NodeID == "" {
		return
	}

	ctx, cancel := r.operationContext(context.Background())
	defer cancel()

	if err := r.repository.Upsert(ctx, status); err != nil {
		if ctx != nil && ctx.Err() == context.Canceled {
			return
		}
		r.logger.Warn("failed to persist shared node status", "node_id", status.NodeID, "error", err)
	}
}

func (r *Registry) runPersister() {
	defer close(r.persistDone)

	pending := make(map[string]ir.NodeStatus)
	var timer *time.Timer
	var timerCh <-chan time.Time

	stopTimer := func() {
		if timer == nil {
			timerCh = nil
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timerCh = nil
	}

	scheduleFlush := func() {
		if r.persistDebounce <= 0 {
			return
		}
		if timer == nil {
			timer = time.NewTimer(r.persistDebounce)
		} else {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(r.persistDebounce)
		}
		timerCh = timer.C
	}

	flushPending := func() {
		if len(pending) == 0 {
			stopTimer()
			r.setPersistPendingNodes(0)
			return
		}

		batch := make([]ir.NodeStatus, 0, len(pending))
		for _, status := range pending {
			batch = append(batch, clone(status))
		}
		clear(pending)
		stopTimer()
		r.setPersistPendingNodes(0)

		started := time.Now()
		for _, status := range batch {
			r.persistNow(status)
		}
		r.observePersistFlush(time.Since(started))
	}

	drainBuffered := func() []persistRequest {
		r.persistMu.Lock()
		defer r.persistMu.Unlock()

		total := len(r.persistImmediate) + len(r.persistDebounced)
		if total == 0 {
			return nil
		}

		batch := make([]persistRequest, 0, total)
		for _, request := range r.persistImmediate {
			batch = append(batch, clonePersistRequest(request))
		}
		for _, request := range r.persistDebounced {
			batch = append(batch, clonePersistRequest(request))
		}
		r.persistImmediate = r.persistImmediate[:0]
		clear(r.persistDebounced)
		r.setPersistQueueDepthLocked(r.persistBacklogDepthLocked())
		return batch
	}

	handleRequest := func(request persistRequest) {
		if request.immediate {
			delete(pending, request.status.NodeID)
			r.setPersistPendingNodes(len(pending))
			if len(pending) == 0 {
				stopTimer()
			}
			r.persistNow(request.status)
			return
		}

		pending[request.status.NodeID] = clone(request.status)
		r.setPersistPendingNodes(len(pending))
		scheduleFlush()
	}

	for {
		select {
		case _, ok := <-r.persistSignal:
			if !ok {
				for _, request := range drainBuffered() {
					handleRequest(request)
				}
				flushPending()
				stopTimer()
				return
			}

			for _, request := range drainBuffered() {
				handleRequest(request)
			}
		case <-timerCh:
			flushPending()
		}
	}
}

func (r *Registry) enqueuePersistRequest(request persistRequest) bool {
	r.persistMu.Lock()
	defer r.persistMu.Unlock()

	if r.persistClosed {
		return false
	}

	if request.immediate {
		delete(r.persistDebounced, request.status.NodeID)
		if r.persistBacklogDepthLocked() >= persistQueueSize {
			r.setPersistQueueDepthLocked(r.persistBacklogDepthLocked())
			return false
		}
		r.persistImmediate = append(r.persistImmediate, clonePersistRequest(request))
	} else {
		if _, ok := r.persistDebounced[request.status.NodeID]; !ok && r.persistBacklogDepthLocked() >= persistQueueSize {
			r.setPersistQueueDepthLocked(r.persistBacklogDepthLocked())
			return false
		}
		r.persistDebounced[request.status.NodeID] = clonePersistRequest(request)
	}

	r.recordPersistAccepted(request)
	r.setPersistQueueDepthLocked(r.persistBacklogDepthLocked())

	select {
	case r.persistSignal <- struct{}{}:
	default:
	}
	return true
}

func clonePersistRequest(request persistRequest) persistRequest {
	return persistRequest{
		status:    clone(request.status),
		immediate: request.immediate,
	}
}

func (r *Registry) persistBacklogDepthLocked() int {
	return len(r.persistImmediate) + len(r.persistDebounced)
}

func (r *Registry) recordPersistAccepted(request persistRequest) {
	if r.metrics == nil {
		return
	}
	if r.metrics.NodeStatusPersistEnqueuedTotal != nil {
		r.metrics.NodeStatusPersistEnqueuedTotal.Inc()
	}
	if request.immediate {
		if r.metrics.NodeStatusPersistImmediateTotal != nil {
			r.metrics.NodeStatusPersistImmediateTotal.Inc()
		}
		return
	}
	if r.metrics.NodeStatusPersistDebouncedTotal != nil {
		r.metrics.NodeStatusPersistDebouncedTotal.Inc()
	}
}

func (r *Registry) recordPersistDrop() {
	if r.metrics == nil || r.metrics.NodeStatusPersistDroppedTotal == nil {
		return
	}
	r.metrics.NodeStatusPersistDroppedTotal.Inc()
}

func (r *Registry) observePersistFlush(duration time.Duration) {
	if r.metrics == nil || r.metrics.NodeStatusPersistFlushDurationSeconds == nil {
		return
	}
	r.metrics.NodeStatusPersistFlushDurationSeconds.Observe(duration.Seconds())
}

func (r *Registry) recordPublishObservation(nodeID, version string, now time.Time) {
	if nodeID == "" || version == "" {
		return
	}

	r.publishMu.Lock()
	defer r.publishMu.Unlock()
	r.published[nodeID] = publishObservation{
		version: version,
		at:      now,
	}
}

func (r *Registry) clearPublishObservation(nodeID string) {
	if nodeID == "" {
		return
	}

	r.publishMu.Lock()
	defer r.publishMu.Unlock()
	delete(r.published, nodeID)
}

func (r *Registry) observePublishAckLag(nodeID, version string, now time.Time) {
	r.observePublishLag(nodeID, version, now, func(metrics *observability.Metrics) prometheus.Observer {
		if metrics == nil {
			return nil
		}
		return metrics.XDSPublishAckLagSeconds
	})
}

func (r *Registry) observePublishNackLag(nodeID, version string, now time.Time) {
	r.observePublishLag(nodeID, version, now, func(metrics *observability.Metrics) prometheus.Observer {
		if metrics == nil {
			return nil
		}
		return metrics.XDSPublishNackLagSeconds
	})
}

func (r *Registry) observePublishLag(
	nodeID, version string,
	now time.Time,
	selectObserver func(*observability.Metrics) prometheus.Observer,
) {
	if nodeID == "" || version == "" || selectObserver == nil {
		return
	}

	r.publishMu.Lock()
	observation, ok := r.published[nodeID]
	if ok && observation.version == version {
		delete(r.published, nodeID)
	}
	r.publishMu.Unlock()
	if !ok || observation.version != version {
		return
	}

	observer := selectObserver(r.metrics)
	if observer == nil {
		return
	}

	lag := now.Sub(observation.at)
	if lag < 0 {
		lag = 0
	}
	observer.Observe(lag.Seconds())
}

func (r *Registry) setPersistPendingNodes(depth int) {
	if r.metrics == nil || r.metrics.NodeStatusPersistPendingNodes == nil {
		return
	}
	r.metrics.NodeStatusPersistPendingNodes.Set(float64(depth))
}

func (r *Registry) setPersistQueueDepthLocked(depth int) {
	if r.metrics == nil || r.metrics.NodeStatusPersistQueueDepth == nil {
		return
	}
	r.metrics.NodeStatusPersistQueueDepth.Set(float64(depth))
}

func (r *Registry) notifyIfRoutingStateChanged(previous, current ir.NodeStatus) {
	if r.onChange == nil {
		return
	}
	if !routingStateChanged(previous, current) {
		return
	}
	r.onChange()
}

func routingStateChanged(previous, current ir.NodeStatus) bool {
	return previous.Connected != current.Connected ||
		previous.Ready != current.Ready ||
		previous.LastAckVersion != current.LastAckVersion ||
		previous.LastConfigStatus != current.LastConfigStatus ||
		previous.LastNackVersion != current.LastNackVersion ||
		!stringSlicesEqual(previous.SupportedFeatures, current.SupportedFeatures)
}

func prefer(current, candidate ir.NodeStatus) ir.NodeStatus {
	switch {
	case candidate.LastSeenAt.After(current.LastSeenAt):
		return clone(candidate)
	case current.LastSeenAt.After(candidate.LastSeenAt):
		return clone(current)
	case completenessScore(candidate) > completenessScore(current):
		return clone(candidate)
	default:
		return clone(current)
	}
}

func completenessScore(status ir.NodeStatus) int {
	score := 0
	if status.Connected {
		score += 4
	}
	if status.Ready {
		score += 3
	}
	if status.Cluster != "" {
		score++
	}
	if !status.ConnectedAt.IsZero() {
		score++
	}
	if !status.DisconnectedAt.IsZero() {
		score++
	}
	if status.DisconnectReason != "" {
		score++
	}
	if status.LastSentVersion != "" {
		score++
	}
	if status.LastAckVersion != "" {
		score++
	}
	if status.LastNonce != "" {
		score++
	}
	if status.LastConfigStatus != "" {
		score++
	}
	if status.LastNackVersion != "" {
		score++
	}
	if status.LastNackNonce != "" {
		score++
	}
	if status.LastNackMessage != "" {
		score++
	}
	if status.Message != "" {
		score++
	}
	score += len(status.Subscriptions)
	score += len(status.SupportedFeatures)
	return score
}

func clone(status ir.NodeStatus) ir.NodeStatus {
	status.Subscriptions = append([]string(nil), status.Subscriptions...)
	status.SupportedFeatures = append([]string(nil), status.SupportedFeatures...)
	return status
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func mergeSharedAuthoritative(shared []ir.NodeStatus, local []ir.NodeStatus) []ir.NodeStatus {
	merged := make(map[string]ir.NodeStatus, len(shared)+len(local))
	for _, item := range shared {
		if item.NodeID == "" {
			continue
		}
		merged[item.NodeID] = clone(item)
	}
	for _, item := range local {
		if item.NodeID == "" {
			continue
		}
		if current, ok := merged[item.NodeID]; ok {
			merged[item.NodeID] = prefer(current, item)
			continue
		}
		merged[item.NodeID] = clone(item)
	}

	out := make([]ir.NodeStatus, 0, len(merged))
	for _, item := range merged {
		out = append(out, clone(item))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].NodeID < out[j].NodeID
	})

	return out
}

func filterStaleNodes(nodes []ir.NodeStatus, now time.Time) []ir.NodeStatus {
	if len(nodes) == 0 {
		return nil
	}

	cutoff := now.UTC().Add(-defaultLeaseDuration)
	out := make([]ir.NodeStatus, 0, len(nodes))
	for _, node := range nodes {
		if node.NodeID == "" {
			continue
		}
		if isStaleNode(node, cutoff) {
			continue
		}
		out = append(out, clone(node))
	}

	return out
}

func FilterStale(nodes []ir.NodeStatus, now time.Time) []ir.NodeStatus {
	return filterStaleNodes(nodes, now)
}

func isStaleNode(node ir.NodeStatus, cutoff time.Time) bool {
	if node.LastSeenAt.IsZero() {
		return false
	}
	return node.LastSeenAt.UTC().Before(cutoff)
}
