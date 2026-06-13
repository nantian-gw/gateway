package admin

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/nantian-gw/gateway/internal/infrastructure"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/nodestatus"
)

const (
	defaultMaxRequestBodyBytes  int64 = 2 << 20
	defaultMaxResponseBodyBytes int64 = 8 << 20
	defaultReadHeaderTimeout          = 5 * time.Second
	defaultReadTimeout                = 30 * time.Second
	defaultWriteTimeout               = 30 * time.Second
	defaultIdleTimeout                = 2 * time.Minute
)

type Summary struct {
	SnapshotVersion          string    `json:"snapshotVersion,omitempty"`
	GeneratedAt              time.Time `json:"generatedAt,omitempty"`
	ListenerCount            int       `json:"listenerCount"`
	HTTPRouteCount           int       `json:"httpRouteCount"`
	GRPCRouteCount           int       `json:"grpcRouteCount"`
	StreamRouteCount         int       `json:"streamRouteCount"`
	RouteCount               int       `json:"routeCount"`
	BackendCount             int       `json:"backendCount"`
	SecretCount              int       `json:"secretCount"`
	NodeCount                int       `json:"nodeCount"`
	ConnectedNodeCount       int       `json:"connectedNodeCount"`
	ReadyNodeCount           int       `json:"readyNodeCount"`
	CurrentVersionNodeCount  int       `json:"currentVersionNodeCount"`
	CurrentVersionReadyCount int       `json:"currentVersionReadyCount"`
	DriftedNodeCount         int       `json:"driftedNodeCount"`
	ReadyListenerCount       int       `json:"readyListenerCount"`
	WarningListenerCount     int       `json:"warningListenerCount"`
	FailedListenerCount      int       `json:"failedListenerCount"`
	Warnings                 []string  `json:"warnings,omitempty"`
}

type routeContract struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	Auth        string `json:"auth"`
	ContentType string `json:"contentType"`
}

type routeBinding struct {
	contract routeContract
	handler  func(*Server) http.HandlerFunc
}

type Server struct {
	store                 *ir.SnapshotStore
	nodes                 *nodestatus.Registry
	resources             *ResourceManager
	dashboardCapabilities DashboardCapabilities
	logger                *slog.Logger
	server                *http.Server
	tlsConfig             *tls.Config
	readinessMode         string
	driftWarningThreshold time.Duration
	maxRequestBodyBytes   int64
	maxResponseBodyBytes  int64
	now                   func() time.Time
	infra                 *infrastructure.Reconciler
	detailIndex           *snapshotDetailIndexCache
	dataplaneDiscovery    *DataplaneAdminDiscovery
	dataplaneClient       *DataplaneAdminClient
}

func NewServer(
	addr string,
	store *ir.SnapshotStore,
	nodes *nodestatus.Registry,
	resources *ResourceManager,
	logger *slog.Logger,
	opts Options,
) *Server {
	s := &Server{
		store:                 store,
		nodes:                 nodes,
		resources:             resources,
		dashboardCapabilities: opts.DashboardCapabilities,
		logger:                logger,
		tlsConfig:             opts.TLSConfig,
		readinessMode:         normalizeReadinessMode(opts.ReadinessMode),
		driftWarningThreshold: opts.NodeDriftWarningThreshold,
		maxRequestBodyBytes:   positiveOrDefault(opts.MaxRequestBodyBytes, defaultMaxRequestBodyBytes),
		maxResponseBodyBytes:  positiveOrDefault(opts.MaxResponseBodyBytes, defaultMaxResponseBodyBytes),
		now:                   func() time.Time { return time.Now().UTC() },
		detailIndex:           newSnapshotDetailIndexCache(),
	}

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	handler := wrapAuthHandler(mux, opts)
	handler = wrapMetricsHandler(handler, opts.Metrics)
	if rl := newRateLimiter(opts.RateLimitRPS, 1*time.Second); rl != nil {
		handler = rl.middleware(handler)
	}

	s.server = &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: positiveDurationOrDefault(opts.ReadHeaderTimeout, defaultReadHeaderTimeout),
		ReadTimeout:       positiveDurationOrDefault(opts.ReadTimeout, defaultReadTimeout),
		WriteTimeout:      positiveDurationOrDefault(opts.WriteTimeout, defaultWriteTimeout),
		IdleTimeout:       positiveDurationOrDefault(opts.IdleTimeout, defaultIdleTimeout),
	}

	return s
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	for _, binding := range adminRouteBindings() {
		mux.HandleFunc(binding.contract.Method+" "+binding.contract.Path, binding.handler(s))
	}
}

func adminRouteContracts() []routeContract {
	bindings := adminRouteBindings()
	contracts := make([]routeContract, 0, len(bindings))
	for _, binding := range bindings {
		contracts = append(contracts, binding.contract)
	}
	return contracts
}

func adminRouteBindings() []routeBinding {
	return []routeBinding{
		{
			contract: routeContract{Method: http.MethodGet, Path: "/livez", Auth: "none", ContentType: "text/plain"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleLiveness },
		},
		{
			contract: routeContract{Method: http.MethodGet, Path: "/readyz", Auth: "none", ContentType: "text/plain"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleReadiness },
		},
		{
			contract: routeContract{Method: http.MethodGet, Path: "/v1/summary", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleSummary },
		},
		{
			contract: routeContract{Method: http.MethodGet, Path: "/v1/dashboard/capabilities", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleDashboardCapabilities },
		},
		{
			contract: routeContract{Method: http.MethodGet, Path: "/v1/snapshot-sync", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleSnapshotSync },
		},
		{
			contract: routeContract{Method: http.MethodGet, Path: "/v1/snapshot", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleSnapshot },
		},
		{
			contract: routeContract{Method: http.MethodGet, Path: "/v1/listeners", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleListeners },
		},
		{
			contract: routeContract{Method: http.MethodGet, Path: "/v1/listeners/{name}", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleListenerDetail },
		},
		{
			contract: routeContract{Method: http.MethodGet, Path: "/v1/routes", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleRoutes },
		},
		{
			contract: routeContract{Method: http.MethodGet, Path: "/v1/routes/{kind}/{namespace}/{name}", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleRouteDetail },
		},
		{
			contract: routeContract{Method: http.MethodGet, Path: "/v1/backends", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleBackends },
		},
		{
			contract: routeContract{Method: http.MethodGet, Path: "/v1/backends/{namespace}/{name}", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleBackendDetail },
		},
		{
			contract: routeContract{Method: http.MethodGet, Path: "/v1/nodes", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleNodes },
		},
		{
			contract: routeContract{Method: http.MethodGet, Path: "/v1/nodes/{nodeId}", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleNodeDetail },
		},
		{
			contract: routeContract{Method: http.MethodGet, Path: "/v1/infrastructure", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleInfrastructure },
		},
		{
			contract: routeContract{Method: http.MethodGet, Path: "/v1/service-catalog", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleServiceCatalog },
		},
		{
			contract: routeContract{Method: http.MethodGet, Path: "/v1/namespaces", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleNamespaces },
		},
		{
			contract: routeContract{Method: http.MethodGet, Path: "/v1/resource-kinds", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleResourceKinds },
		},
		{
			contract: routeContract{Method: http.MethodGet, Path: "/v1/resources", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleResources },
		},
		{
			contract: routeContract{Method: http.MethodPost, Path: "/v1/resources", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleResourceApply },
		},
		{
			contract: routeContract{Method: http.MethodGet, Path: "/v1/resources/{kind}/{namespace}/{name}", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleResourceDetail },
		},
		{
			contract: routeContract{Method: http.MethodPut, Path: "/v1/resources/{kind}/{namespace}/{name}", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleResourceApply },
		},
		{
			contract: routeContract{Method: http.MethodDelete, Path: "/v1/resources/{kind}/{namespace}/{name}", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleResourceDelete },
		},
		{
			contract: routeContract{Method: http.MethodGet, Path: "/v1/topology", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleTopology },
		},
		{
			contract: routeContract{Method: http.MethodGet, Path: "/v1/dataplanes", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleDataplanes },
		},
		{
			contract: routeContract{Method: http.MethodGet, Path: "/v1/dataplanes/{nodeId}/summary", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleDataplaneSummary },
		},
		{
			contract: routeContract{Method: http.MethodGet, Path: "/v1/chatbot/config", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleChatbotConfig },
		},
		{
			contract: routeContract{Method: http.MethodPut, Path: "/v1/chatbot/config", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleChatbotConfigPut },
		},
		{
			contract: routeContract{Method: http.MethodPost, Path: "/v1/chatbot/chat", Auth: "bearer-when-configured", ContentType: "text/event-stream"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleChatbotChat },
		},
		{
			contract: routeContract{Method: http.MethodGet, Path: "/v1/metrics/config", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleMetricsConfigGet },
		},
		{
			contract: routeContract{Method: http.MethodPut, Path: "/v1/metrics/config", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleMetricsConfigPut },
		},
		{
			contract: routeContract{Method: http.MethodPost, Path: "/v1/metrics/query", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleMetricsQuery },
		},
		{
			contract: routeContract{Method: http.MethodPost, Path: "/v1/metrics/query_range", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleMetricsRangeQuery },
		},
		// AI Gateway endpoints
		{
			contract: routeContract{Method: http.MethodGet, Path: "/v1/ai/overview", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleAIOverview },
		},
		{
			contract: routeContract{Method: http.MethodGet, Path: "/v1/ai/services", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleAIServices },
		},
		{
			contract: routeContract{Method: http.MethodGet, Path: "/v1/ai/token-usage", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleAITokenUsage },
		},
		{
			contract: routeContract{Method: http.MethodGet, Path: "/v1/ai/traces", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleAITraces },
		},
		{
			contract: routeContract{Method: http.MethodGet, Path: "/v1/ai/cost", Auth: "bearer-when-configured", ContentType: "application/json"},
			handler:  func(s *Server) http.HandlerFunc { return s.handleAICost },
		},
	}
}

func (s *Server) ListenAndServe() error {
	s.logger.Info("starting admin server", "addr", s.server.Addr)
	if s.tlsConfig != nil {
		return s.server.ListenAndServeTLS("", "")
	}
	return s.server.ListenAndServe()
}

func (s *Server) Serve(listener net.Listener) error {
	s.logger.Info("starting admin server", "addr", listener.Addr().String())
	if s.tlsConfig != nil {
		listener = tls.NewListener(listener, s.tlsConfig)
	}
	return s.server.Serve(listener)
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *Server) Close() error {
	return s.server.Close()
}

func (s *Server) SetInfrastructureInspector(reconciler *infrastructure.Reconciler) {
	s.infra = reconciler
}

func (s *Server) SetDataplaneComponents(discovery *DataplaneAdminDiscovery, client *DataplaneAdminClient) {
	s.dataplaneDiscovery = discovery
	s.dataplaneClient = client
}

func (s *Server) handleDashboardCapabilities(w http.ResponseWriter, r *http.Request) {
	s.respondJSON(w, s.dashboardCapabilities)
}

func positiveOrDefault(value, fallback int64) int64 {
	if value > 0 {
		return value
	}
	return fallback
}

func positiveDurationOrDefault(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}
