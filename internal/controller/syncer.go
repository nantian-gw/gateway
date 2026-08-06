package controller

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/observability"
	"github.com/nantian-gw/gateway/internal/translator"
)

type Syncer struct {
	client                         client.Client
	translator                     *translator.Translator
	store                          *ir.SnapshotStore
	metrics                        *observability.Metrics
	interval                       time.Duration
	logger                         *slog.Logger
	leaderRun                      func(...ReconcilerRunnerScope)
	mu                             sync.Mutex
	indexMu                        sync.Mutex
	settleMu                       sync.Mutex
	settleDelay                    time.Duration
	settleTimer                    *time.Timer
	settleRun                      func(context.Context)
	lifecycleCtx                   context.Context
	settlePending                  snapshotPendingBuild
	retryMu                        sync.Mutex
	retryPending                   snapshotPendingBuild
	backendTLSPolicyConfigMapIndex bool
	missingFieldIndexFallbacks     map[missingFieldIndexFallbackLogKey]struct{}
	options                        SyncerOptions
}

type ComponentReconciler interface {
	Reconcile(context.Context) error
}

type SyncerOptions struct {
	EnableExperimentalGateway bool
	MaxConcurrentReconciles   int
	RateLimiterBaseDelay      time.Duration
	RateLimiterMaxDelay       time.Duration
	RateLimiterQPS            int
	RateLimiterBucketSize     int
}

func defaultSyncerOptions() SyncerOptions {
	return SyncerOptions{
		EnableExperimentalGateway: true,
		MaxConcurrentReconciles:   1,
		RateLimiterBaseDelay:      200 * time.Millisecond,
		RateLimiterMaxDelay:       30 * time.Second,
		RateLimiterQPS:            10,
		RateLimiterBucketSize:     100,
	}
}

func NewSyncer(
	client client.Client,
	translator *translator.Translator,
	store *ir.SnapshotStore,
	metrics *observability.Metrics,
	interval time.Duration,
	logger *slog.Logger,
	leaderRuns ...func(...ReconcilerRunnerScope),
) *Syncer {
	var leaderRun func(...ReconcilerRunnerScope)
	if len(leaderRuns) > 0 {
		leaderRun = leaderRuns[0]
	}

	return &Syncer{
		client:      client,
		translator:  translator,
		store:       store,
		metrics:     metrics,
		interval:    interval,
		logger:      logger,
		leaderRun:   leaderRun,
		settleDelay: 100 * time.Millisecond,
		options:     defaultSyncerOptions(),
	}
}

func (s *Syncer) SetOptions(options SyncerOptions) {
	if s == nil {
		return
	}
	s.options = options
}

func (s *Syncer) Run(ctx context.Context) {
	s.setLifecycleContext(ctx)
	defer s.clearLifecycleContext()

	if published, err := s.publishSnapshot(ctx); err != nil {
		s.mergeRetryPendingBuild(snapshotBuildScopeFull, nil, nil, nil, nil, nil, snapshotRouteObjectKeys{})
	} else if published {
		s.queueLeaderRun(snapshotBuildScopeFull)
	}

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.stopSettleRun()
			return
		case <-ticker.C:
			s.runRetryBuild(ctx)
		}
	}
}

func (s *Syncer) Start(ctx context.Context) error {
	s.Run(ctx)
	return nil
}

func (s *Syncer) NeedLeaderElection() bool {
	return false
}

func (s *Syncer) backendTLSPolicyConfigMapIndexAvailable() bool {
	if s == nil {
		return false
	}
	s.indexMu.Lock()
	defer s.indexMu.Unlock()
	return s.backendTLSPolicyConfigMapIndex
}

func (s *Syncer) setBackendTLSPolicyConfigMapIndexAvailable(available bool) {
	if s == nil {
		return
	}
	s.indexMu.Lock()
	defer s.indexMu.Unlock()
	s.backendTLSPolicyConfigMapIndex = available
}

func (s *Syncer) SetSettleDelay(delay time.Duration) {
	s.settleMu.Lock()
	defer s.settleMu.Unlock()
	s.settleDelay = delay
}

func (s *Syncer) stopSettleRun() {
	s.settleMu.Lock()
	defer s.settleMu.Unlock()

	if s.settleTimer != nil {
		s.settleTimer.Stop()
		s.settleTimer = nil
	}
	s.clearPendingBuildLocked()
}

func (s *Syncer) queueLeaderRun(scope snapshotBuildScope) {
	if s.leaderRun == nil {
		return
	}
	s.leaderRun(reconcilerRunnerScopesForSnapshotBuildScope(scope)...)
}

func (s *Syncer) setLifecycleContext(ctx context.Context) {
	s.settleMu.Lock()
	defer s.settleMu.Unlock()
	s.lifecycleCtx = ctx
}

func (s *Syncer) clearLifecycleContext() {
	s.settleMu.Lock()
	defer s.settleMu.Unlock()
	s.lifecycleCtx = nil
}

func (s *Syncer) publishSnapshotWithScope(
	ctx context.Context,
	scope snapshotBuildScope,
	attachmentNamespaces []string,
	backendNamespaces []string,
	gatewayKeys []client.ObjectKey,
	serviceKeys []client.ObjectKey,
	serviceImportKeys []client.ObjectKey,
	routeKeys snapshotRouteObjectKeys,
) (bool, error) {
	tracer := otel.Tracer("github.com/nantian-gw/gateway/internal/controller")
	ctx, span := tracer.Start(ctx, "controlplane.syncer.publish_snapshot")
	defer span.End()
	span.SetAttributes(
		attribute.String("snapshot.scope", scope.String()),
		attribute.Int("snapshot.attachment_namespace_count", len(attachmentNamespaces)),
		attribute.Int("snapshot.backend_namespace_count", len(backendNamespaces)),
		attribute.Int("snapshot.gateway_key_count", len(gatewayKeys)),
		attribute.Int("snapshot.service_key_count", len(serviceKeys)),
		attribute.Int("snapshot.service_import_key_count", len(serviceImportKeys)),
		attribute.Int("snapshot.route_key_count", routeKeys.count()),
	)

	s.mu.Lock()
	defer s.mu.Unlock()

	metrics := s.metrics
	if metrics != nil {
		incCounter(metrics.BuildsTotal)
	}
	startedAt := time.Now()

	snapshot, err := s.buildSnapshot(
		ctx,
		scope,
		attachmentNamespaces,
		backendNamespaces,
		gatewayKeys,
		serviceKeys,
		serviceImportKeys,
		routeKeys,
	)
	if metrics != nil {
		observeHistogram(metrics.SnapshotBuildDurationSeconds, time.Since(startedAt).Seconds())
	}
	if err != nil {
		if metrics != nil {
			incCounter(metrics.BuildFailures)
			setGauge(metrics.LastBuildSuccess, 0)
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		s.logger.Error("failed to rebuild snapshot", "error", err)
		return false, err
	}

	if metrics != nil {
		setGauge(metrics.LastBuildSuccess, 1)
	}
	s.observeSnapshotBuildShape(snapshot)
	if s.store.Publish(snapshot) {
		if metrics != nil {
			incCounter(metrics.PublishedTotal)
		}
		span.SetAttributes(attribute.Bool("snapshot.published", true))
		return true, nil
	}
	span.SetAttributes(attribute.Bool("snapshot.published", false))
	return false, nil
}

func (s *Syncer) publishSnapshot(ctx context.Context) (bool, error) {
	return s.publishSnapshotWithScope(ctx, snapshotBuildScopeFull, nil, nil, nil, nil, nil, snapshotRouteObjectKeys{})
}

func (s *Syncer) runRetryBuild(ctx context.Context) bool {
	scope, attachmentNamespaces, backendNamespaces, gatewayKeys, serviceKeys, serviceImportKeys, routeKeys := s.consumeRetryPendingBuild()
	if scope == snapshotBuildScopeNone {
		return false
	}

	published, err := s.publishSnapshotWithScope(
		ctx,
		scope,
		attachmentNamespaces,
		backendNamespaces,
		gatewayKeys,
		serviceKeys,
		serviceImportKeys,
		routeKeys,
	)
	if err != nil {
		s.mergeRetryPendingBuild(
			scope,
			attachmentNamespaces,
			backendNamespaces,
			gatewayKeys,
			serviceKeys,
			serviceImportKeys,
			routeKeys,
		)
		return true
	}
	if published {
		s.queueLeaderRun(scope)
	}
	return true
}

func (s *Syncer) observeSnapshotBuildShape(snapshot *ir.Snapshot) {
	if s == nil || s.metrics == nil || snapshot == nil {
		return
	}
	if s.metrics.SnapshotResourceCount != nil {
		s.metrics.SnapshotResourceCount.WithLabelValues("listeners").Observe(float64(len(snapshot.Listeners)))
		s.metrics.SnapshotResourceCount.WithLabelValues("http_routes").Observe(float64(len(snapshot.HTTPRoutes)))
		s.metrics.SnapshotResourceCount.WithLabelValues("grpc_routes").Observe(float64(len(snapshot.GRPCRoutes)))
		s.metrics.SnapshotResourceCount.WithLabelValues("stream_routes").Observe(float64(len(snapshot.StreamRoutes)))
		s.metrics.SnapshotResourceCount.WithLabelValues("backends").Observe(float64(len(snapshot.Backends)))
		s.metrics.SnapshotResourceCount.WithLabelValues("secrets").Observe(float64(len(snapshot.Secrets)))
		s.metrics.SnapshotResourceCount.WithLabelValues("workloads").Observe(float64(len(snapshot.Workloads)))
	}
	if s.metrics.SnapshotListenerAttachedRoutes != nil {
		for _, listener := range snapshot.Listeners {
			s.metrics.SnapshotListenerAttachedRoutes.Observe(float64(len(listener.AttachedRoutes)))
		}
	}
}
