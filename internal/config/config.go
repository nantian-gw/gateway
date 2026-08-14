package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"github.com/nantian-gw/gateway/internal/constants"
)

const (
	defaultLeaderElectionLeaseDuration = 15 * time.Second
	defaultLeaderElectionRenewDeadline = 10 * time.Second
	defaultNodeStatusPersistDebounce   = 250 * time.Millisecond
	defaultNodeDriftWarningThreshold   = 15 * time.Second
	defaultSyncSettleDelay             = 200 * time.Millisecond
	defaultGRPCKeepaliveTimeout        = 10 * time.Second
	defaultGRPCMinPingInterval         = 15 * time.Second
)

type Config struct {
	GRPCAddr         string                 `yaml:"grpcAddr"`
	AdminAddr        string                 `yaml:"adminAddr"`
	MetricsAddr      string                 `yaml:"metricsAddr"`
	HealthProbeAddr  string                 `yaml:"healthProbeAddr"`
	ControllerName   string                 `yaml:"controllerName"`
	StatusAddress    string                 `yaml:"statusAddress"`
	StatusAddresses  []string               `yaml:"statusAddresses"`
	SyncPeriod       string                 `yaml:"syncPeriod"`
	SyncSettleDelay  string                 `yaml:"syncSettleDelay"`
	ReconcilerRunner ReconcilerRunnerConfig `yaml:"reconcilerRunner"`
	NodeStatus       NodeStatusConfig       `yaml:"nodeStatus"`
	AdminReadiness   AdminReadinessConfig   `yaml:"adminReadiness"`
	AdminLimits      AdminLimitsConfig      `yaml:"adminLimits"`
	AdminRuntime     AdminRuntimeConfig     `yaml:"adminRuntime"`
	TranslatorLimits TranslatorLimitsConfig `yaml:"translatorLimits"`
	NodeDrift        NodeDriftConfig        `yaml:"nodeDrift"`
	Log              LogConfig              `yaml:"log"`
	LeaderElection   LeaderElectionConfig   `yaml:"leaderElection"`
	AdminAuth        AdminAuthConfig        `yaml:"adminAuth"`
	Pprof            PprofConfig            `yaml:"pprof"`
	AdminTLS         AdminTLSConfig         `yaml:"adminTLS"`
	GRPCTLS          GRPCTLSConfig          `yaml:"grpcTLS"`
	GRPCRuntime      GRPCRuntimeConfig      `yaml:"grpcRuntime"`
	Namespace        string                 `yaml:"namespace"`
	Features         FeaturesConfig         `yaml:"features"`
	Infra            InfraConfig            `yaml:"infra"`
	Dashboard        DashboardConfig        `yaml:"dashboard"`
	Tracing          TracingConfig          `yaml:"tracing"`
	Controller       ControllerConfig       `yaml:"controller"`
}

type FeaturesConfig struct {
	EnableExperimentalGateway bool `yaml:"enableExperimentalGateway"`
	EnableAiGateway           bool `yaml:"enableAiGateway"`
}

type InfraConfig struct {
	DataplaneAdminPort     int32 `yaml:"dataplaneAdminPort"`
	NodePortBasePrivileged int32 `yaml:"nodePortBasePrivileged"`
	NodePortBaseUDP        int32 `yaml:"nodePortBaseUDP"`
	NodePortBaseDefault    int32 `yaml:"nodePortBaseDefault"`
	NodePortRangeMax       int32 `yaml:"nodePortRangeMax"`
}

type DashboardConfig struct {
	Enabled      *bool                       `yaml:"enabled"`
	Capabilities DashboardCapabilitiesConfig `yaml:"capabilities"`
}

type DashboardCapabilitiesConfig struct {
	AIOverview      *bool `yaml:"aiOverview"`
	AIServices      *bool `yaml:"aiServices"`
	AITokenPolicies *bool `yaml:"aiTokenPolicies"`
	AICost          *bool `yaml:"aiCost"`
	AITraces        *bool `yaml:"aiTraces"`
	AIUsage         *bool `yaml:"aiUsage"`
	WasmPlugins     *bool `yaml:"wasmPlugins"`
	Chatbot         *bool `yaml:"chatbot"`
}

type ResolvedDashboardCapabilities struct {
	AIOverview      bool
	AIServices      bool
	AITokenPolicies bool
	AICost          bool
	AITraces        bool
	AIUsage         bool
	WasmPlugins     bool
	Chatbot         bool
}

// ControllerConfig controls reconciliation behavior (concurrency, rate limiting).
type ControllerConfig struct {
	MaxConcurrentReconciles int    `yaml:"maxConcurrentReconciles"`
	RateLimiterBaseDelay    string `yaml:"rateLimiterBaseDelay"`
	RateLimiterMaxDelay     string `yaml:"rateLimiterMaxDelay"`
	RateLimiterQPS          int    `yaml:"rateLimiterQPS"`
	RateLimiterBucketSize   int    `yaml:"rateLimiterBucketSize"`
}

type LogConfig struct {
	Level     string `yaml:"level"`
	Format    string `yaml:"format"`
	AddSource bool   `yaml:"addSource"`
}

type LeaderElectionConfig struct {
	Enabled       bool   `yaml:"enabled"`
	ID            string `yaml:"id"`
	LeaseDuration string `yaml:"leaseDuration"`
	RenewDeadline string `yaml:"renewDeadline"`
	RetryPeriod   string `yaml:"retryPeriod"`
}

type AdminAuthConfig struct {
	AuthMode                string   `yaml:"authMode"` // "static" (default) or "kubernetes"
	BearerToken             string   `yaml:"bearerToken"`
	BearerTokenFile         string   `yaml:"bearerTokenFile"`
	ReadOnlyBearerToken     string   `yaml:"readOnlyBearerToken"`
	ReadOnlyBearerTokenFile string   `yaml:"readOnlyBearerTokenFile"`
	RateLimitRPS            int64    `yaml:"rateLimitRPS"`
	RateLimitBurst          int64    `yaml:"rateLimitBurst"`
	TokenReviewAudiences    []string `yaml:"tokenReviewAudiences"`
	AllowedUsers            []string `yaml:"allowedUsers"`
	AllowedGroups           []string `yaml:"allowedGroups"`
	TrustedProxies          []string `yaml:"trustedProxies"`
	AllowFromCIDRs          []string `yaml:"allowFromCIDRs"`
	// RBAC enables role-based access control on admin API endpoints.
	// When nil or empty, the existing binary auth model applies (backward compatible).
	RBAC *AdminRBACConfig `yaml:"rbac"`
}

// NormalizeAuthMode returns the effective auth mode, defaulting to "static".
func (c AdminAuthConfig) NormalizeAuthMode() string {
	mode := strings.TrimSpace(c.AuthMode)
	if mode == "" {
		return "static"
	}
	return mode
}

type PprofConfig struct {
	Enabled         bool   `yaml:"enabled"`
	Addr            string `yaml:"addr"`
	BearerToken     string `yaml:"bearerToken"`
	BearerTokenFile string `yaml:"bearerTokenFile"`
}

type NodeStatusConfig struct {
	Namespace       string `yaml:"namespace"`
	LeasePrefix     string `yaml:"leasePrefix"`
	PersistTimeout  string `yaml:"persistTimeout"`
	PersistDebounce string `yaml:"persistDebounce"`
}

type AdminReadinessConfig struct {
	Mode string `yaml:"mode"`
}

type AdminLimitsConfig struct {
	MaxRequestBodyBytes  int64 `yaml:"maxRequestBodyBytes"`
	MaxResponseBodyBytes int64 `yaml:"maxResponseBodyBytes"`
	MaxListItems         int   `yaml:"maxListItems"`
}

type TracingConfig struct {
	Enabled      bool              `yaml:"enabled"`
	Endpoint     string            `yaml:"endpoint"`
	Insecure     bool              `yaml:"insecure"`
	SamplerRatio *float64          `yaml:"samplerRatio"`
	Headers      map[string]string `yaml:"headers"`
}

type AdminRuntimeConfig struct {
	ReadHeaderTimeout    string                           `yaml:"readHeaderTimeout"`
	ReadTimeout          string                           `yaml:"readTimeout"`
	WriteTimeout         string                           `yaml:"writeTimeout"`
	IdleTimeout          string                           `yaml:"idleTimeout"`
	DataplaneAggregation *DataplaneAdminAggregationConfig `yaml:"dataplaneAggregation"`
}

type DataplaneAdminAggregationConfig struct {
	ServiceName     string `yaml:"serviceName"`
	Namespace       string `yaml:"namespace"`
	PortName        string `yaml:"portName"`
	Timeout         string `yaml:"timeout"`
	BearerTokenFile string `yaml:"bearerTokenFile"`
}

type TranslatorLimitsConfig struct {
	MaxInputObjects      int `yaml:"maxInputObjects"`
	MaxSnapshotObjects   int `yaml:"maxSnapshotObjects"`
	MaxSnapshotEndpoints int `yaml:"maxSnapshotEndpoints"`
}

type ReconcilerRunnerConfig struct {
	SettleDelay  string `yaml:"settleDelay"`
	RetryBackoff string `yaml:"retryBackoff"`
}

type NodeDriftConfig struct {
	WarningThreshold string `yaml:"warningThreshold"`
}

type AdminTLSConfig struct {
	Enabled           bool   `yaml:"enabled"`
	CertPath          string `yaml:"certPath"`
	KeyPath           string `yaml:"keyPath"`
	ClientCAPath      string `yaml:"clientCAPath"`
	RequireClientCert bool   `yaml:"requireClientCert"`
}

type GRPCTLSConfig struct {
	Enabled           bool   `yaml:"enabled"`
	CertPath          string `yaml:"certPath"`
	KeyPath           string `yaml:"keyPath"`
	ClientCAPath      string `yaml:"clientCAPath"`
	RequireClientCert bool   `yaml:"requireClientCert"`
}

type GRPCRuntimeConfig struct {
	KeepaliveTime         string `yaml:"keepaliveTime"`
	KeepaliveTimeout      string `yaml:"keepaliveTimeout"`
	MinPingInterval       string `yaml:"minPingInterval"`
	MaxConnectionIdle     string `yaml:"maxConnectionIdle"`
	MaxConnectionAge      string `yaml:"maxConnectionAge"`
	MaxConnectionAgeGrace string `yaml:"maxConnectionAgeGrace"`
	SnapshotSendTimeout   string `yaml:"snapshotSendTimeout"`
	SnapshotAckTimeout    string `yaml:"snapshotAckTimeout"`
	GracefulStopTimeout   string `yaml:"gracefulStopTimeout"`
	InitialWindowSize     int32  `yaml:"initialWindowSize"`
	InitialConnWindowSize int32  `yaml:"initialConnWindowSize"`
	MaxConcurrentStreams  uint32 `yaml:"maxConcurrentStreams"`
	MaxRecvMsgSize        int    `yaml:"maxRecvMsgSize"`
	PermitWithoutStream   bool   `yaml:"permitWithoutStream"`
	EnableCompression     bool   `yaml:"enableCompression"`
	XDSProtocol           string `yaml:"xdsProtocol"`
}

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}

	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = ":18080"
	}
	if cfg.AdminAddr == "" {
		cfg.AdminAddr = ":18081"
	}
	if cfg.MetricsAddr == "" {
		cfg.MetricsAddr = ":18082"
	}
	if cfg.HealthProbeAddr == "" {
		cfg.HealthProbeAddr = ":18083"
	}
	if cfg.SyncPeriod == "" {
		cfg.SyncPeriod = constants.DefaultTimeout
	}
	if cfg.SyncSettleDelay == "" {
		cfg.SyncSettleDelay = "200ms"
	}
	if cfg.ReconcilerRunner.SettleDelay == "" {
		cfg.ReconcilerRunner.SettleDelay = "300ms"
	}
	if cfg.ReconcilerRunner.RetryBackoff == "" {
		cfg.ReconcilerRunner.RetryBackoff = "1s"
	}
	if cfg.ControllerName == "" {
		cfg.ControllerName = "gateway.networking.k8s.io/nantian-gw"
	}
	if cfg.LeaderElection.ID == "" {
		cfg.LeaderElection.ID = "nantian-controlplane-leader"
	}
	if cfg.LeaderElection.LeaseDuration == "" {
		cfg.LeaderElection.LeaseDuration = "15s"
	}
	if cfg.LeaderElection.RenewDeadline == "" {
		cfg.LeaderElection.RenewDeadline = "10s"
	}
	if cfg.LeaderElection.RetryPeriod == "" {
		cfg.LeaderElection.RetryPeriod = "2s"
	}
	if cfg.NodeStatus.LeasePrefix == "" {
		cfg.NodeStatus.LeasePrefix = "nantian-gw-node"
	}
	if cfg.NodeStatus.PersistTimeout == "" {
		cfg.NodeStatus.PersistTimeout = "2s"
	}
	if cfg.NodeStatus.PersistDebounce == "" {
		cfg.NodeStatus.PersistDebounce = "250ms"
	}
	if cfg.AdminReadiness.Mode == "" {
		cfg.AdminReadiness.Mode = "snapshot"
	}
	if cfg.AdminRuntime.ReadHeaderTimeout == "" {
		cfg.AdminRuntime.ReadHeaderTimeout = "5s"
	}
	if cfg.AdminRuntime.ReadTimeout == "" {
		cfg.AdminRuntime.ReadTimeout = constants.DefaultTimeout
	}
	if cfg.AdminRuntime.WriteTimeout == "" {
		cfg.AdminRuntime.WriteTimeout = constants.DefaultTimeout
	}
	if cfg.AdminRuntime.IdleTimeout == "" {
		cfg.AdminRuntime.IdleTimeout = "2m"
	}
	if cfg.NodeDrift.WarningThreshold == "" {
		cfg.NodeDrift.WarningThreshold = "15s"
	}
	if cfg.GRPCRuntime.KeepaliveTime == "" {
		cfg.GRPCRuntime.KeepaliveTime = constants.DefaultTimeout
	}
	if cfg.GRPCRuntime.KeepaliveTimeout == "" {
		cfg.GRPCRuntime.KeepaliveTimeout = "10s"
	}
	if cfg.GRPCRuntime.MinPingInterval == "" {
		cfg.GRPCRuntime.MinPingInterval = "15s"
	}
	if cfg.GRPCRuntime.MaxConnectionIdle == "" {
		cfg.GRPCRuntime.MaxConnectionIdle = "2m"
	}
	if cfg.GRPCRuntime.MaxConnectionAge == "" {
		cfg.GRPCRuntime.MaxConnectionAge = "30m"
	}
	if cfg.GRPCRuntime.MaxConnectionAgeGrace == "" {
		cfg.GRPCRuntime.MaxConnectionAgeGrace = constants.DefaultTimeout
	}
	if cfg.GRPCRuntime.SnapshotSendTimeout == "" {
		cfg.GRPCRuntime.SnapshotSendTimeout = "5s"
	}
	if cfg.GRPCRuntime.SnapshotAckTimeout == "" {
		cfg.GRPCRuntime.SnapshotAckTimeout = constants.DefaultTimeout
	}
	if cfg.GRPCRuntime.GracefulStopTimeout == "" {
		cfg.GRPCRuntime.GracefulStopTimeout = "3s"
	}
	if cfg.GRPCRuntime.XDSProtocol == "" {
		cfg.GRPCRuntime.XDSProtocol = "delta"
	}
	if cfg.Pprof.Addr == "" {
		cfg.Pprof.Addr = "127.0.0.1:6060"
	}
	if cfg.Tracing.SamplerRatio == nil {
		ratio := 1.0
		cfg.Tracing.SamplerRatio = &ratio
	}
	if cfg.Controller.MaxConcurrentReconciles <= 0 {
		cfg.Controller.MaxConcurrentReconciles = 5
	}
	if cfg.Controller.RateLimiterBaseDelay == "" {
		cfg.Controller.RateLimiterBaseDelay = "200ms"
	}
	if cfg.Controller.RateLimiterMaxDelay == "" {
		cfg.Controller.RateLimiterMaxDelay = constants.DefaultTimeout
	}
	if cfg.Controller.RateLimiterQPS <= 0 {
		cfg.Controller.RateLimiterQPS = 10
	}
	if cfg.Controller.RateLimiterBucketSize <= 0 {
		cfg.Controller.RateLimiterBucketSize = 100
	}

	return &cfg, nil
}

func (c *Config) SyncPeriodDuration() time.Duration {
	return parseDurationOrDefault(c.SyncPeriod, 30*time.Second)
}

func (c *Config) SyncSettleDelayDuration() time.Duration {
	return parseDurationOrDefault(c.SyncSettleDelay, defaultSyncSettleDelay)
}

func (c *Config) ReconcilerRunnerSettleDelayDuration() time.Duration {
	return parseDurationOrDefault(c.ReconcilerRunner.SettleDelay, 300*time.Millisecond)
}

func (c *Config) ReconcilerRunnerRetryBackoffDuration() time.Duration {
	return parseDurationOrDefault(c.ReconcilerRunner.RetryBackoff, time.Second)
}

func (c *Config) LeaderElectionLeaseDuration() time.Duration {
	return parseDurationOrDefault(c.LeaderElection.LeaseDuration, defaultLeaderElectionLeaseDuration)
}

func (c *Config) LeaderElectionRenewDeadline() time.Duration {
	return parseDurationOrDefault(c.LeaderElection.RenewDeadline, defaultLeaderElectionRenewDeadline)
}

func (c *Config) LeaderElectionRetryPeriod() time.Duration {
	return parseDurationOrDefault(c.LeaderElection.RetryPeriod, 2*time.Second)
}

func (c *Config) NodeStatusPersistTimeout() time.Duration {
	return parseDurationOrDefault(c.NodeStatus.PersistTimeout, 2*time.Second)
}

func (c *Config) NodeStatusPersistDebounce() time.Duration {
	return parseDurationOrDefault(c.NodeStatus.PersistDebounce, defaultNodeStatusPersistDebounce)
}

func (c *Config) NodeDriftWarningThreshold() time.Duration {
	return parseDurationOrDefault(c.NodeDrift.WarningThreshold, defaultNodeDriftWarningThreshold)
}

func (c *Config) AdminMaxRequestBodyBytes() int64 {
	return positiveInt64OrDefault(c.AdminLimits.MaxRequestBodyBytes, 2<<20)
}

func (c *Config) AdminMaxResponseBodyBytes() int64 {
	return positiveInt64OrDefault(c.AdminLimits.MaxResponseBodyBytes, 8<<20)
}

func (c *Config) AdminMaxListItems() int {
	if c.AdminLimits.MaxListItems > 0 {
		return c.AdminLimits.MaxListItems
	}
	return 1000
}

func (c *Config) AdminRateLimitBurst() int64 {
	if c.AdminAuth.RateLimitBurst > 0 {
		return c.AdminAuth.RateLimitBurst
	}
	if c.AdminAuth.RateLimitRPS > 0 {
		return c.AdminAuth.RateLimitRPS
	}
	return 0
}

func (c *Config) AdminReadHeaderTimeoutDuration() time.Duration {
	return parsePositiveDurationOrDefault(c.AdminRuntime.ReadHeaderTimeout, 5*time.Second)
}

func (c *Config) AdminReadTimeoutDuration() time.Duration {
	return parsePositiveDurationOrDefault(c.AdminRuntime.ReadTimeout, 30*time.Second)
}

func (c *Config) AdminWriteTimeoutDuration() time.Duration {
	return parsePositiveDurationOrDefault(c.AdminRuntime.WriteTimeout, 30*time.Second)
}

func (c *Config) AdminIdleTimeoutDuration() time.Duration {
	return parsePositiveDurationOrDefault(c.AdminRuntime.IdleTimeout, 2*time.Minute)
}

func (c *Config) TranslatorResourceLimits() TranslatorLimitsConfig {
	return TranslatorLimitsConfig{
		MaxInputObjects:      positiveIntOrZero(c.TranslatorLimits.MaxInputObjects),
		MaxSnapshotObjects:   positiveIntOrZero(c.TranslatorLimits.MaxSnapshotObjects),
		MaxSnapshotEndpoints: positiveIntOrZero(c.TranslatorLimits.MaxSnapshotEndpoints),
	}
}

func (c *Config) DashboardEnabled() bool {
	return boolValueOrDefault(c.Dashboard.Enabled)
}

func (c *Config) DashboardCapabilities() ResolvedDashboardCapabilities {
	return ResolvedDashboardCapabilities{
		AIOverview:      boolValueOrDefault(c.Dashboard.Capabilities.AIOverview),
		AIServices:      boolValueOrDefault(c.Dashboard.Capabilities.AIServices),
		AITokenPolicies: boolValueOrDefault(c.Dashboard.Capabilities.AITokenPolicies),
		AICost:          boolValueOrDefault(c.Dashboard.Capabilities.AICost),
		AITraces:        boolValueOrDefault(c.Dashboard.Capabilities.AITraces),
		AIUsage:         boolValueOrDefault(c.Dashboard.Capabilities.AIUsage),
		WasmPlugins:     boolValueOrDefault(c.Dashboard.Capabilities.WasmPlugins),
		Chatbot:         boolValueOrDefault(c.Dashboard.Capabilities.Chatbot),
	}
}

func (c *Config) TracingSamplerRatio() float64 {
	switch {
	case c.Tracing.SamplerRatio == nil:
		return 1
	case *c.Tracing.SamplerRatio < 0:
		return 0
	case *c.Tracing.SamplerRatio > 1:
		return 1
	default:
		return *c.Tracing.SamplerRatio
	}
}

func (c *Config) TracingHeaders() map[string]string {
	if len(c.Tracing.Headers) == 0 {
		return nil
	}

	out := make(map[string]string, len(c.Tracing.Headers))
	for k, v := range c.Tracing.Headers {
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}

	return out
}

func (c *Config) GRPCKeepaliveTimeDuration() time.Duration {
	return parseDurationOrDefault(c.GRPCRuntime.KeepaliveTime, 30*time.Second)
}

func (c *Config) GRPCKeepaliveTimeoutDuration() time.Duration {
	return parseDurationOrDefault(c.GRPCRuntime.KeepaliveTimeout, defaultGRPCKeepaliveTimeout)
}

func (c *Config) GRPCMinPingIntervalDuration() time.Duration {
	return parseDurationOrDefault(c.GRPCRuntime.MinPingInterval, defaultGRPCMinPingInterval)
}

func (c *Config) GRPCMaxConnectionIdleDuration() time.Duration {
	return parseDurationOrDefault(c.GRPCRuntime.MaxConnectionIdle, 2*time.Minute)
}

func (c *Config) GRPCMaxConnectionAgeDuration() time.Duration {
	return parseDurationOrDefault(c.GRPCRuntime.MaxConnectionAge, 30*time.Minute)
}

func (c *Config) GRPCMaxConnectionAgeGraceDuration() time.Duration {
	return parseDurationOrDefault(c.GRPCRuntime.MaxConnectionAgeGrace, 30*time.Second)
}

func (c *Config) GRPCSnapshotSendTimeoutDuration() time.Duration {
	return parsePositiveDurationOrDefault(c.GRPCRuntime.SnapshotSendTimeout, 5*time.Second)
}

func (c *Config) GRPCSnapshotAckTimeoutDuration() time.Duration {
	return parsePositiveDurationOrDefault(c.GRPCRuntime.SnapshotAckTimeout, 30*time.Second)
}

func (c *Config) GRPCTLSEnabled() bool {
	return c.GRPCTLS.Enabled || (c.GRPCTLS.CertPath != "" && c.GRPCTLS.KeyPath != "")
}

func (c *Config) AdvertisedAddresses() []string {
	if len(c.StatusAddresses) == 0 {
		if value := strings.TrimSpace(c.StatusAddress); value != "" {
			return []string{value}
		}
		return nil
	}

	out := make([]string, 0, len(c.StatusAddresses))
	seen := make(map[string]struct{}, len(c.StatusAddresses))
	for _, raw := range c.StatusAddresses {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		if value := strings.TrimSpace(c.StatusAddress); value != "" {
			return []string{value}
		}
	}
	return out
}

func (c AdminAuthConfig) ResolveBearerToken() (string, error) {
	if path := strings.TrimSpace(c.BearerTokenFile); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read admin bearer token file: %w", err)
		}

		return strings.TrimSpace(string(raw)), nil
	}

	return strings.TrimSpace(c.BearerToken), nil
}

func NewLogger(cfg LogConfig) *slog.Logger {
	level := new(slog.LevelVar)
	level.Set(parseLevel(cfg.Level))

	options := &slog.HandlerOptions{
		Level:     level,
		AddSource: cfg.AddSource,
	}
	if strings.EqualFold(cfg.Format, "json") {
		return slog.New(slog.NewJSONHandler(os.Stdout, options))
	}

	return slog.New(slog.NewTextHandler(os.Stdout, options))
}

func parseLevel(raw string) slog.Level {
	switch strings.ToLower(raw) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func parseDurationOrDefault(raw string, fallback time.Duration) time.Duration {
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}

	return d
}

func parsePositiveDurationOrDefault(raw string, fallback time.Duration) time.Duration {
	d := parseDurationOrDefault(raw, fallback)
	if d <= 0 {
		return fallback
	}

	return d
}

func positiveInt64OrDefault(value, fallback int64) int64 {
	if value > 0 {
		return value
	}
	return fallback
}

func positiveIntOrZero(value int) int {
	if value > 0 {
		return value
	}
	return 0
}

func boolValueOrDefault(value *bool) bool {
	if value == nil {
		return true
	}
	return *value
}

func (c *Config) DataplaneAggregationConfig() *DataplaneAdminAggregationConfig {
	if c.AdminRuntime.DataplaneAggregation == nil {
		return nil
	}
	return c.AdminRuntime.DataplaneAggregation
}

func (c *DataplaneAdminAggregationConfig) TimeoutDuration() time.Duration {
	return parsePositiveDurationOrDefault(c.Timeout, 2*time.Second)
}

func (c *DataplaneAdminAggregationConfig) BearerToken() (string, error) {
	if c.BearerTokenFile == "" {
		return "", nil
	}
	token, err := os.ReadFile(c.BearerTokenFile)
	if err != nil {
		return "", fmt.Errorf("read dataplane admin bearer token file: %w", err)
	}
	return strings.TrimSpace(string(token)), nil
}

func (c *Config) RateLimiterBaseDelayDuration() time.Duration {
	return parseDurationOrDefault(c.Controller.RateLimiterBaseDelay, 200*time.Millisecond)
}

func (c *Config) RateLimiterMaxDelayDuration() time.Duration {
	return parseDurationOrDefault(c.Controller.RateLimiterMaxDelay, 30*time.Second)
}

func (c *Config) Validate() error {
	var errs []error

	if c.AdminAddr != "" {
		_, portStr, err := net.SplitHostPort(c.AdminAddr)
		if err != nil {
			errs = append(errs, fmt.Errorf("adminAddr: %w", err))
		} else if port, _ := strconv.Atoi(portStr); port < 1 || port > 65535 {
			errs = append(errs, fmt.Errorf("adminAddr port out of range: %d", port))
		}
	}

	if c.AdminAuth.BearerToken != "" && c.AdminAuth.BearerTokenFile != "" {
		errs = append(errs, fmt.Errorf("adminAuth: bearerToken and bearerTokenFile are mutually exclusive"))
	}

	if c.AdminAuth.RBAC != nil {
		if err := c.AdminAuth.RBAC.Validate(); err != nil {
			return err
		}
	}

	level := strings.ToLower(c.Log.Level)
	switch level {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, fmt.Errorf("log.level: invalid level %q (expected debug/info/warn/error)", c.Log.Level))
	}

	return errors.Join(errs...)
}
