package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/admin"
	"github.com/nantian-gw/gateway/internal/config"
	"github.com/nantian-gw/gateway/internal/controller"
	aiservicev1alpha1 "github.com/nantian-gw/gateway/internal/gatewayapiexperimental/aiservicev1alpha1"
	backendlbv1alpha2 "github.com/nantian-gw/gateway/internal/gatewayapiexperimental/backendlbv1alpha2"
	tokenpolicyv1alpha1 "github.com/nantian-gw/gateway/internal/gatewayapiexperimental/tokenpolicyv1alpha1"
	wasmpluginv1alpha1 "github.com/nantian-gw/gateway/internal/gatewayapiexperimental/wasmpluginv1alpha1"
	"github.com/nantian-gw/gateway/internal/grpcserver"
	"github.com/nantian-gw/gateway/internal/infrastructure"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/lifecycle"
	"github.com/nantian-gw/gateway/internal/nodestatus"
	"github.com/nantian-gw/gateway/internal/observability"
	"github.com/nantian-gw/gateway/internal/status"
	"github.com/nantian-gw/gateway/internal/translator"
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
	nodeRepository := nodestatus.NewLeaseRepository(
		mgr.GetAPIReader(),
		mgr.GetClient(),
		cfg.NodeStatus.Namespace,
		cfg.NodeStatus.LeasePrefix,
		logger,
	)
	nodes := nodestatus.NewRegistry(
		ir.NewNodeStatusStore(),
		nodeRepository,
		logger,
		nodestatus.Options{
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
		translator.Options{
			Limits: translator.Limits{
				MaxInputObjects:      translatorLimits.MaxInputObjects,
				MaxSnapshotObjects:   translatorLimits.MaxSnapshotObjects,
				MaxSnapshotEndpoints: translatorLimits.MaxSnapshotEndpoints,
			},
		},
	)
	statusOptions := status.Options{EnableExperimentalGateway: cfg.Features.EnableExperimentalGateway}
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
	syncer.SetOptions(controller.SyncerOptions{EnableExperimentalGateway: cfg.Features.EnableExperimentalGateway})
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
			BearerToken:               cfg.AdminAuth.BearerToken,
			BearerTokenFile:           cfg.AdminAuth.BearerTokenFile,
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
		},
	)
	adminServer.SetInfrastructureInspector(infra)

	if dpCfg := cfg.DataplaneAggregationConfig(); dpCfg != nil {
		namespace := cfg.Namespace
		if namespace == "" {
			namespace = "nantian-gw"
		}
		discovery := admin.NewDataplaneAdminDiscovery(mgr.GetClient(), admin.DataplaneAdminDiscoveryConfig{
			Namespace:   namespace,
			ServiceName: dpCfg.ServiceName,
			PortName:    dpCfg.PortName,
		})

		token, err := dpCfg.BearerToken()
		if err != nil {
			return fmt.Errorf("configure dataplane admin aggregation: %w", err)
		}
		client := admin.NewDataplaneAdminClient(admin.DataplaneAdminClientConfig{
			Timeout:     dpCfg.TimeoutDuration(),
			BearerToken: token,
		})

		adminServer.SetDataplaneComponents(discovery, client)
		logger.Info("configured dataplane admin aggregation", "service", dpCfg.ServiceName, "namespace", namespace)
	}

	grpcServer, err := grpcserver.New(cfg.GRPCAddr, cfg.GRPCTLS, cfg.GRPCRuntime, store, nodes, logger, metrics)
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

func buildScheme(cfg *config.Config) (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	registrations := []struct {
		name string
		fn   func(*runtime.Scheme) error
	}{
		{name: "client-go", fn: clientgoscheme.AddToScheme},
		{name: "core/v1", fn: corev1.AddToScheme},
		{name: "coordination/v1", fn: coordinationv1.AddToScheme},
		{name: "discovery/v1", fn: discoveryv1.AddToScheme},
		{name: "apiextensions/v1", fn: apiextensionsv1.AddToScheme},
		{name: "gateway/v1", fn: gatewayv1.Install},
		{name: "gateway/v1alpha2", fn: gatewayv1alpha2.Install},
		{name: "gateway/v1alpha3", fn: gatewayv1alpha3.Install},
		{name: "gateway/v1beta1", fn: gatewayv1beta1.Install},
		{name: "mcs/v1alpha1", fn: mcsv1alpha1.AddToScheme},
	}

	if cfg.Features.EnableExperimentalGateway {
		registrations = append(registrations,
			[]struct {
				name string
				fn   func(*runtime.Scheme) error
			}{
				{name: "gateway.experimental/v1alpha2", fn: backendlbv1alpha2.Install},
				{name: "wasmplugin/v1alpha1", fn: wasmpluginv1alpha1.AddToScheme},
				{name: "tokenpolicy/v1alpha1", fn: tokenpolicyv1alpha1.AddToScheme},
			}...,
		)
	}

	if cfg.Features.EnableAiGateway {
		registrations = append(registrations,
			struct {
				name string
				fn   func(*runtime.Scheme) error
			}{name: "aiservice/v1alpha1", fn: aiservicev1alpha1.AddToScheme},
		)
	}

	for _, registration := range registrations {
		if err := registration.fn(scheme); err != nil {
			return nil, fmt.Errorf("register %s scheme: %w", registration.name, err)
		}
	}

	return scheme, nil
}

func newManagerComponent(mgr ctrl.Manager, leaderElectionEnabled bool) lifecycle.Component {
	return lifecycle.Component{
		Name: "controller-manager",
		Run: func(ctx context.Context, markStarted func()) error {
			errCh := make(chan error, 1)
			go func() {
				errCh <- mgr.Start(ctx)
			}()

			// Standby replicas with leader election enabled may intentionally wait
			// for leadership before caches sync; treat process startup as complete
			// once the manager goroutine is running in that mode.
			if leaderElectionEnabled {
				markStarted()
				return waitManagerExit(ctx, errCh, false)
			}

			syncedCh := make(chan bool, 1)
			go func() {
				syncedCh <- mgr.GetCache().WaitForCacheSync(ctx)
			}()

			for {
				select {
				case synced := <-syncedCh:
					if !synced {
						if ctx.Err() != nil {
							return waitManagerExit(ctx, errCh, true)
						}
						select {
						case err := <-errCh:
							if err != nil {
								return err
							}
						default:
						}
						return errors.New("controller manager cache sync did not complete")
					}

					markStarted()
					return waitManagerExit(ctx, errCh, false)
				case err := <-errCh:
					if ctx.Err() != nil && err == nil {
						return nil
					}
					if err != nil {
						return err
					}
					return errors.New("controller manager stopped before cache sync completed")
				case <-ctx.Done():
					return waitManagerExit(ctx, errCh, true)
				}
			}
		},
	}
}

func waitManagerExit(ctx context.Context, errCh <-chan error, shuttingDown bool) error {
	err := <-errCh
	if ctx.Err() != nil {
		return nil
	}
	if err != nil {
		return err
	}
	if shuttingDown {
		return nil
	}
	return errors.New("controller manager stopped unexpectedly")
}

func newHTTPComponent(
	name string,
	addr string,
	serve func(net.Listener) error,
	shutdown func(context.Context) error,
	closeServer func() error,
	shutdownTimeout time.Duration,
	logger *slog.Logger,
) lifecycle.Component {
	return lifecycle.Component{
		Name: name,
		Run: func(ctx context.Context, markStarted func()) error {
			listener, err := net.Listen("tcp", addr)
			if err != nil {
				return fmt.Errorf("listen on %s: %w", addr, err)
			}
			defer listener.Close()

			go func() {
				<-ctx.Done()

				shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
				defer cancel()

				if err := shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
					logger.Warn("http component shutdown returned error", "component", name, "error", err)
					_ = closeServer()
				}
				if shutdownCtx.Err() == context.DeadlineExceeded {
					logger.Warn("http component shutdown timed out, forcing close", "component", name)
					_ = closeServer()
				}
			}()

			markStarted()
			err = serve(listener)
			switch {
			case ctx.Err() != nil:
				return nil
			case err == nil:
				return fmt.Errorf("%s server stopped unexpectedly", name)
			default:
				return err
			}
		},
	}
}

func newGRPCComponent(name, addr string, server *grpcserver.Server) lifecycle.Component {
	return lifecycle.Component{
		Name: name,
		Run: func(ctx context.Context, markStarted func()) error {
			listener, err := net.Listen("tcp", addr)
			if err != nil {
				return fmt.Errorf("listen on %s: %w", addr, err)
			}
			defer listener.Close()
			return server.Serve(ctx, listener, markStarted)
		},
	}
}

func ptr[T any](value T) *T {
	return &value
}
