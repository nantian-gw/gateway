package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlcfg "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/nantian-gw/gateway/internal/admin"
	"github.com/nantian-gw/gateway/internal/config"
	"github.com/nantian-gw/gateway/internal/controller"
	"github.com/nantian-gw/gateway/internal/xds"
	"github.com/nantian-gw/gateway/internal/infrastructure"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/lifecycle"
	"github.com/nantian-gw/gateway/internal/noderegistry"
	"github.com/nantian-gw/gateway/internal/observability"
	"github.com/nantian-gw/gateway/internal/status"
	"github.com/nantian-gw/gateway/internal/translator"
	"github.com/nantian-gw/gateway/internal/translator/shared"
)

const (
	defaultStartupTimeout  = 20 * time.Second
	defaultShutdownTimeout = 5 * time.Second

	defaultMetricsReadHeaderTimeout = 5 * time.Second
	defaultMetricsReadTimeout       = 30 * time.Second
	defaultMetricsWriteTimeout      = 30 * time.Second
	defaultMetricsIdleTimeout       = 2 * time.Minute
	defaultMetricsMaxHeaderBytes    = 32 << 10
)

func controlplaneManagerOptions(
	cfg *config.Config,
	scheme *runtime.Scheme,
	logger *slog.Logger,
) ctrl.Options {
	return ctrl.Options{
		Scheme: scheme,
		Client: client.Options{
			Cache: &client.CacheOptions{
				Unstructured: true,
			},
		},
		Logger: controllerRuntimeLogger(logger),
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
		HealthProbeBindAddress: cfg.HealthProbeAddr,
		LeaderElection:         cfg.LeaderElection.Enabled,
		LeaderElectionID:       cfg.LeaderElection.ID,
		LeaseDuration:          ptr(cfg.LeaderElectionLeaseDuration()),
		RenewDeadline:          ptr(cfg.LeaderElectionRenewDeadline()),
		RetryPeriod:            ptr(cfg.LeaderElectionRetryPeriod()),
		Controller: ctrlcfg.Controller{
			MaxConcurrentReconciles: cfg.Controller.MaxConcurrentReconciles,
		},
	}
}

func run(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config %s: %w", configPath, err)
	}

	logger := config.NewLogger(cfg.Log).With(
		"component", "controlplane",
		"controller_name", cfg.ControllerName,
	)
	configureKubernetesLogging(logger)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	tracingCfg := controlplaneTracingConfig(cfg)
	tracingShutdown, err := observability.ConfigureTracing(ctx, tracingCfg)
	if err != nil {
		return fmt.Errorf("configure tracing: %w", err)
	}
	logControlplaneTracingStatus(logger, tracingCfg)
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), defaultShutdownTimeout)
		defer shutdownCancel()
		_ = tracingShutdown(shutdownCtx)
	}()

	scheme, err := buildScheme(cfg)
	if err != nil {
		return err
	}

	restCfg, err := ctrl.GetConfig()
	if err != nil {
		return fmt.Errorf("load kubernetes rest config: %w", err)
	}

	startupGate := lifecycle.NewStartupGate("controlplane startup in progress")
	mgr, err := ctrl.NewManager(restCfg, controlplaneManagerOptions(cfg, scheme, logger))
	if err != nil {
		return fmt.Errorf("create controller manager: %w", err)
	}
	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		return fmt.Errorf("register healthz check: %w", err)
	}
	if err := mgr.AddReadyzCheck("startup", startupGate.Check); err != nil {
		return fmt.Errorf("register startup readyz check: %w", err)
	}
	if err := mgr.AddReadyzCheck("ping", healthz.Ping); err != nil {
		return fmt.Errorf("register ping readyz check: %w", err)
	}

	metrics := observability.NewMetrics()
	store := ir.NewSnapshotStore(logger)
	store.SetHooks(ir.SnapshotStoreHooks{
		OnSubscriberQueueReplace: func(_ string, replaced int) {
			if replaced <= 0 || metrics == nil || metrics.XDSSnapshotFanoutCoalescedTotal == nil {
				return
			}
			metrics.XDSSnapshotFanoutCoalescedTotal.Add(float64(replaced))
		},
	})
	nodeRepository := noderegistry.NewLeaseRepository(
		mgr.GetAPIReader(),
		mgr.GetClient(),
		cfg.NodeStatus.Namespace,
		cfg.NodeStatus.LeasePrefix,
		logger,
	)
	nodes := noderegistry.NewRegistry(
		ir.NewNodeStatusStore(),
		nodeRepository,
		logger,
		noderegistry.Options{
			BaseContext:     ctx,
			PersistTimeout:  cfg.NodeStatusPersistTimeout(),
			PersistDebounce: cfg.NodeStatusPersistDebounce(),
			Metrics:         metrics,
		},
	)
	defer nodes.Close()
	logger.Info(
		"configured shared node status storage",
		"namespace",
		nodeRepository.Namespace(),
		"lease_prefix",
		nodeRepository.Prefix(),
	)

	translatorLimits := cfg.TranslatorResourceLimits()
	xlator := translator.NewWithOptions(
		cfg.ControllerName,
		logger,
		shared.Options{
			Limits: shared.Limits{
				MaxInputObjects:      translatorLimits.MaxInputObjects,
				MaxSnapshotObjects:   translatorLimits.MaxSnapshotObjects,
				MaxSnapshotEndpoints: translatorLimits.MaxSnapshotEndpoints,
			},
		},
	)
	statusOptions := status.Options{
		EnableExperimentalGateway: cfg.Features.EnableExperimentalGateway,
		MaxConcurrentReconciles:   cfg.Controller.MaxConcurrentReconciles,
		RateLimiterBaseDelay:      cfg.RateLimiterBaseDelayDuration(),
		RateLimiterMaxDelay:       cfg.RateLimiterMaxDelayDuration(),
		RateLimiterQPS:            cfg.Controller.RateLimiterQPS,
		RateLimiterBucketSize:     cfg.Controller.RateLimiterBucketSize,
	}
	statuser := status.NewWithAddressesAndReaderOptions(
		mgr.GetClient(),
		mgr.GetAPIReader(),
		cfg.ControllerName,
		cfg.AdvertisedAddresses(),
		logger,
		statusOptions,
	)
	statuser.SetEventRecorder(mgr.GetEventRecorderFor("gateway-status"))
	if err := translator.SetupIndexes(ctx, mgr.GetFieldIndexer()); err != nil {
		return fmt.Errorf("set up translator indexes: %w", err)
	}
	if err := infrastructure.SetupIndexes(
		ctx,
		mgr.GetFieldIndexer(),
		infrastructure.Options{EnableExperimentalGateway: cfg.Features.EnableExperimentalGateway},
	); err != nil {
		return fmt.Errorf("set up infrastructure indexes: %w", err)
	}
	if err := status.SetupIndexes(ctx, mgr.GetFieldIndexer(), statusOptions); err != nil {
		return fmt.Errorf("set up status indexes: %w", err)
	}

	infraOptions := infrastructure.DefaultOptions()
	infraOptions.SnapshotStore = store
	infraOptions.NodeStatus = nodes
	infraOptions.EnableExperimentalGateway = cfg.Features.EnableExperimentalGateway
	if v := cfg.Infra.DataplaneAdminPort; v != 0 {
		infraOptions.DataplaneAdminPort = v
	}
	if v := cfg.Infra.NodePortBasePrivileged; v != 0 {
		infraOptions.NodePortBasePrivileged = v
	}
	if v := cfg.Infra.NodePortBaseUDP; v != 0 {
		infraOptions.NodePortBaseUDP = v
	}
	if v := cfg.Infra.NodePortBaseDefault; v != 0 {
		infraOptions.NodePortBaseDefault = v
	}
	if v := cfg.Infra.NodePortRangeMax; v != 0 {
		infraOptions.NodePortRangeMax = v
	}
	infra := infrastructure.NewWithOptions(mgr.GetClient(), mgr.GetAPIReader(), cfg.ControllerName, infraOptions, logger)
	statusScopedReconcile := func(ctx context.Context, scope controller.ReconcilerRunnerScope) error {
		switch scope {
		case controller.ReconcilerRunnerScopeGatewayStatus:
			return statuser.ReconcileGatewayStatuses(ctx)
		case controller.ReconcilerRunnerScopeRouteStatus:
			return statuser.ReconcileRouteStatuses(ctx)
		case controller.ReconcilerRunnerScopePolicyStatus:
			return statuser.ReconcilePolicyStatuses(ctx)
		default:
			return statuser.Reconcile(ctx)
		}
	}

	reconcilerRunner := controller.NewReconcilerRunner(
		cfg.SyncPeriodDuration(),
		logger,
		metrics,
		controller.NewScopedReconciler("infrastructure", infra, controller.ReconcilerRunnerScopeInfra),
		controller.NewScopedReconcilerFunc(
			"status",
			statuser.Reconcile,
			statusScopedReconcile,
			controller.ReconcilerRunnerScopeGatewayStatus,
			controller.ReconcilerRunnerScopeRouteStatus,
			controller.ReconcilerRunnerScopePolicyStatus,
		),
	)
	reconcilerRunner.SetSettleDelay(cfg.ReconcilerRunnerSettleDelayDuration())
	reconcilerRunner.SetRetryBackoff(cfg.ReconcilerRunnerRetryBackoffDuration())
	nodes.SetOnChange(func() {
		reconcilerRunner.QueueRunImmediateForScope(controller.ReconcilerRunnerScopeInfra)
	})
	statuser.SetTriggerInfrastructure(func() {
		reconcilerRunner.QueueRunImmediateForScope(controller.ReconcilerRunnerScopeInfra)
	})

	syncer := controller.NewSyncer(
		mgr.GetClient(),
		xlator,
		store,
		metrics,
		cfg.SyncPeriodDuration(),
		logger,
		reconcilerRunner.QueueRunForScopes,
	)
	syncer.SetOptions(controller.SyncerOptions{
		EnableExperimentalGateway: cfg.Features.EnableExperimentalGateway,
		MaxConcurrentReconciles:   cfg.Controller.MaxConcurrentReconciles,
		RateLimiterBaseDelay:      cfg.RateLimiterBaseDelayDuration(),
		RateLimiterMaxDelay:       cfg.RateLimiterMaxDelayDuration(),
		RateLimiterQPS:            cfg.Controller.RateLimiterQPS,
		RateLimiterBucketSize:     cfg.Controller.RateLimiterBucketSize,
	})
	syncer.SetSettleDelay(cfg.SyncSettleDelayDuration())
	if err := syncer.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("set up snapshot sync controller: %w", err)
	}
	if err := mgr.Add(syncer); err != nil {
		return fmt.Errorf("add syncer runnable: %w", err)
	}
	if err := status.SetupControllers(mgr, statuser, statusOptions); err != nil {
		return fmt.Errorf("set up status controllers: %w", err)
	}
	if err := mgr.Add(reconcilerRunner); err != nil {
		return fmt.Errorf("add controlplane reconciler runner: %w", err)
	}
	if _, err := cfg.AdminAuth.ResolveBearerToken(); err != nil {
		return fmt.Errorf("resolve admin auth configuration: %w", err)
	}

	var adminTLSConfig *tls.Config
	if cfg.AdminTLS.Enabled {
		if cfg.AdminTLS.CertPath == "" || cfg.AdminTLS.KeyPath == "" {
			return fmt.Errorf("admin TLS enabled but certPath or keyPath is empty")
		}
		certificate, err := tls.LoadX509KeyPair(cfg.AdminTLS.CertPath, cfg.AdminTLS.KeyPath)
		if err != nil {
			return fmt.Errorf("load admin TLS certificate: %w", err)
		}
		if cfg.AdminTLS.RequireClientCert && cfg.AdminTLS.ClientCAPath == "" {
			return fmt.Errorf("admin TLS requireClientCert is enabled but clientCAPath is empty")
		}
		adminTLSConfig = &tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{certificate},
			CipherSuites: []uint16{
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			},
		}
		if caPath := cfg.AdminTLS.ClientCAPath; caPath != "" {
			raw, err := os.ReadFile(caPath)
			if err != nil {
				return fmt.Errorf("read admin TLS client CA: %w", err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(raw) {
				return fmt.Errorf("parse admin TLS client CA bundle")
			}
			adminTLSConfig.ClientCAs = pool
			if cfg.AdminTLS.RequireClientCert {
				adminTLSConfig.ClientAuth = tls.RequireAndVerifyClientCert
			} else {
				adminTLSConfig.ClientAuth = tls.VerifyClientCertIfGiven
			}
		}
	}

	adminServer := admin.NewServer(
		cfg.AdminAddr,
		store,
		nodes,
		admin.NewResourceManager(mgr.GetClient(), logger),
		logger,
		admin.Options{
			AuthMode:                  cfg.AdminAuth.NormalizeAuthMode(),
			BearerToken:               cfg.AdminAuth.BearerToken,
			BearerTokenFile:           cfg.AdminAuth.BearerTokenFile,
			ReadOnlyBearerToken:       cfg.AdminAuth.ReadOnlyBearerToken,
			ReadOnlyBearerTokenFile:   cfg.AdminAuth.ReadOnlyBearerTokenFile,
			RestConfig:                restCfg,
			ReadinessMode:             cfg.AdminReadiness.Mode,
			NodeDriftWarningThreshold: cfg.NodeDriftWarningThreshold(),
			MaxRequestBodyBytes:       cfg.AdminMaxRequestBodyBytes(),
			MaxResponseBodyBytes:      cfg.AdminMaxResponseBodyBytes(),
			ReadHeaderTimeout:         cfg.AdminReadHeaderTimeoutDuration(),
			ReadTimeout:               cfg.AdminReadTimeoutDuration(),
			WriteTimeout:              cfg.AdminWriteTimeoutDuration(),
			IdleTimeout:               cfg.AdminIdleTimeoutDuration(),
			Metrics:                   metrics,
			TLSConfig:                 adminTLSConfig,
			Logger:                    logger,
			RateLimitRPS:              cfg.AdminAuth.RateLimitRPS,
			RateLimitBurst:            cfg.AdminAuth.RateLimitBurst,
			TokenReviewAudiences:      cfg.AdminAuth.TokenReviewAudiences,
			AllowedUsers:              cfg.AdminAuth.AllowedUsers,
			AllowedGroups:             cfg.AdminAuth.AllowedGroups,
			TrustedProxies:            cfg.AdminAuth.TrustedProxies,
			DashboardCapabilities:     admin.ResolveDashboardCapabilities(cfg),
			AllowFromCIDRs:            cfg.AdminAuth.AllowFromCIDRs,
		},
	)
	adminServer.SetInfrastructureInspector(infra)

	if dpCfg := cfg.DataplaneAggregationConfig(); dpCfg != nil {
		namespace := cfg.Namespace
		if namespace == "" {
			namespace = "nantian-gw"
		}
		discovery := admin.NewDataplaneDiscovery(mgr.GetClient(), admin.DataplaneDiscoveryConfig{
			Namespace:   namespace,
			ServiceName: dpCfg.ServiceName,
			PortName:    dpCfg.PortName,
		})

		token, err := dpCfg.BearerToken()
		if err != nil {
			return fmt.Errorf("configure dataplane admin aggregation: %w", err)
		}
		client := admin.NewDataplaneClient(admin.DataplaneClientConfig{
			Timeout:     dpCfg.TimeoutDuration(),
			BearerToken: token,
		})

		adminServer.SetDataplaneComponents(discovery, client)
		logger.Info("configured dataplane admin aggregation", "service", dpCfg.ServiceName, "namespace", namespace)
	}

	grpcServer, err := xds.New(cfg.GRPCAddr, cfg.GRPCTLS, cfg.GRPCRuntime, store, nodes, logger, metrics)
	if err != nil {
		return fmt.Errorf("configure grpc server: %w", err)
	}

	metricsServer := newMetricsServer(cfg.MetricsAddr, metrics, cfg.AdminAuth.BearerToken, cfg.AdminAuth.BearerTokenFile, logger)

	components := []lifecycle.Component{
		newManagerComponent(mgr, cfg.LeaderElection.Enabled),
		newHTTPComponent(
			"admin",
			cfg.AdminAddr,
			adminServer.Serve,
			adminServer.Shutdown,
			adminServer.Close,
			defaultShutdownTimeout,
			logger,
		),
		newHTTPComponent(
			"metrics",
			cfg.MetricsAddr,
			metricsServer.Serve,
			metricsServer.Shutdown,
			metricsServer.Close,
			defaultShutdownTimeout,
			logger,
		),
		newGRPCComponent("grpc", cfg.GRPCAddr, grpcServer),
	}
	if cfg.Pprof.Enabled {
		logger.Info("pprof debug server enabled", "addr", cfg.Pprof.Addr)
		pprofServer := newPprofServer(cfg.Pprof.Addr, cfg.Pprof.BearerToken, cfg.Pprof.BearerTokenFile, logger)
		components = append(
			components,
			newHTTPComponent(
				"pprof",
				cfg.Pprof.Addr,
				pprofServer.Serve,
				pprofServer.Shutdown,
				pprofServer.Close,
				defaultShutdownTimeout,
				logger,
			),
		)
	}

	supervisor := lifecycle.NewSupervisor(
		logger,
		defaultStartupTimeout,
		startupGate,
		components...,
	)

	if err := supervisor.Run(ctx); err != nil {
		return err
	}

	logger.Info("shutdown complete")
	return nil
}

func newMetricsServer(addr string, metrics *observability.Metrics, bearerToken, bearerTokenFile string, logger *slog.Logger) *http.Server {
	handler := observability.Handler(metrics)
	if tok := resolvePprofToken(bearerToken, bearerTokenFile); tok != "" {
		handler = pprofAuthMiddleware(handler, tok)
	} else if logger != nil {
		logger.Warn("metrics server running without authentication — set adminAuth.bearerToken or adminAuth.bearerTokenFile in config")
	}

	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: defaultMetricsReadHeaderTimeout,
		ReadTimeout:       defaultMetricsReadTimeout,
		WriteTimeout:      defaultMetricsWriteTimeout,
		IdleTimeout:       defaultMetricsIdleTimeout,
		MaxHeaderBytes:    defaultMetricsMaxHeaderBytes,
	}
}
