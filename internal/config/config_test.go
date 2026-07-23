package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLoadAppliesProductionDefaults(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(""), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.HealthProbeAddr != ":18083" {
		t.Fatalf("unexpected health probe addr: %s", cfg.HealthProbeAddr)
	}
	if cfg.LeaderElection.ID != "nantian-controlplane-leader" {
		t.Fatalf("unexpected leader election id: %s", cfg.LeaderElection.ID)
	}
	if cfg.LeaderElectionLeaseDuration() != 15*time.Second {
		t.Fatalf("unexpected lease duration: %s", cfg.LeaderElectionLeaseDuration())
	}
	if cfg.LeaderElectionRenewDeadline() != 10*time.Second {
		t.Fatalf("unexpected renew deadline: %s", cfg.LeaderElectionRenewDeadline())
	}
	if cfg.LeaderElectionRetryPeriod() != 2*time.Second {
		t.Fatalf("unexpected retry period: %s", cfg.LeaderElectionRetryPeriod())
	}
	if cfg.NodeStatus.LeasePrefix != "nantian-gw-node" {
		t.Fatalf("unexpected node status lease prefix: %s", cfg.NodeStatus.LeasePrefix)
	}
	if cfg.NodeStatusPersistTimeout() != 2*time.Second {
		t.Fatalf("unexpected node status timeout: %s", cfg.NodeStatusPersistTimeout())
	}
	if cfg.NodeStatusPersistDebounce() != 250*time.Millisecond {
		t.Fatalf("unexpected node status debounce: %s", cfg.NodeStatusPersistDebounce())
	}
	if cfg.SyncSettleDelayDuration() != 200*time.Millisecond {
		t.Fatalf("unexpected sync settle delay: %s", cfg.SyncSettleDelayDuration())
	}
	if cfg.ReconcilerRunnerSettleDelayDuration() != 300*time.Millisecond {
		t.Fatalf("unexpected reconciler runner settle delay: %s", cfg.ReconcilerRunnerSettleDelayDuration())
	}
	if cfg.ReconcilerRunnerRetryBackoffDuration() != time.Second {
		t.Fatalf("unexpected reconciler runner retry backoff: %s", cfg.ReconcilerRunnerRetryBackoffDuration())
	}
	if cfg.GRPCKeepaliveTimeDuration() != 30*time.Second {
		t.Fatalf("unexpected grpc keepalive time: %s", cfg.GRPCKeepaliveTimeDuration())
	}
	if cfg.GRPCKeepaliveTimeoutDuration() != 10*time.Second {
		t.Fatalf("unexpected grpc keepalive timeout: %s", cfg.GRPCKeepaliveTimeoutDuration())
	}
	if cfg.GRPCMinPingIntervalDuration() != 15*time.Second {
		t.Fatalf("unexpected grpc min ping interval: %s", cfg.GRPCMinPingIntervalDuration())
	}
	if cfg.GRPCMaxConnectionIdleDuration() != 2*time.Minute {
		t.Fatalf("unexpected grpc max connection idle: %s", cfg.GRPCMaxConnectionIdleDuration())
	}
	if cfg.GRPCMaxConnectionAgeDuration() != 30*time.Minute {
		t.Fatalf("unexpected grpc max connection age: %s", cfg.GRPCMaxConnectionAgeDuration())
	}
	if cfg.GRPCMaxConnectionAgeGraceDuration() != 30*time.Second {
		t.Fatalf("unexpected grpc max connection age grace: %s", cfg.GRPCMaxConnectionAgeGraceDuration())
	}
	if cfg.GRPCSnapshotSendTimeoutDuration() != 5*time.Second {
		t.Fatalf("unexpected grpc snapshot send timeout: %s", cfg.GRPCSnapshotSendTimeoutDuration())
	}
	if cfg.GRPCSnapshotAckTimeoutDuration() != 30*time.Second {
		t.Fatalf("unexpected grpc snapshot ack timeout: %s", cfg.GRPCSnapshotAckTimeoutDuration())
	}
	if cfg.StatusAddress != "" {
		t.Fatalf("unexpected default status address: %q", cfg.StatusAddress)
	}
	if got := cfg.AdvertisedAddresses(); got != nil {
		t.Fatalf("unexpected default advertised addresses: %#v", got)
	}
	if cfg.GRPCTLS.RequireClientCert {
		t.Fatal("grpc mTLS should be disabled by default")
	}
	if cfg.GRPCRuntime.PermitWithoutStream {
		t.Fatal("grpc keepalive without stream should be disabled by default")
	}
	if cfg.AdminReadiness.Mode != "snapshot" {
		t.Fatalf("unexpected admin readiness mode: %s", cfg.AdminReadiness.Mode)
	}
	if cfg.NodeDriftWarningThreshold() != 15*time.Second {
		t.Fatalf("unexpected node drift warning threshold: %s", cfg.NodeDriftWarningThreshold())
	}
	if cfg.AdminMaxRequestBodyBytes() != 2<<20 {
		t.Fatalf("unexpected admin max request body bytes: %d", cfg.AdminMaxRequestBodyBytes())
	}
	if cfg.AdminMaxResponseBodyBytes() != 8<<20 {
		t.Fatalf("unexpected admin max response body bytes: %d", cfg.AdminMaxResponseBodyBytes())
	}
	if cfg.AdminReadHeaderTimeoutDuration() != 5*time.Second {
		t.Fatalf("unexpected admin read header timeout: %s", cfg.AdminReadHeaderTimeoutDuration())
	}
	if cfg.AdminReadTimeoutDuration() != 30*time.Second {
		t.Fatalf("unexpected admin read timeout: %s", cfg.AdminReadTimeoutDuration())
	}
	if cfg.AdminWriteTimeoutDuration() != 30*time.Second {
		t.Fatalf("unexpected admin write timeout: %s", cfg.AdminWriteTimeoutDuration())
	}
	if cfg.AdminIdleTimeoutDuration() != 2*time.Minute {
		t.Fatalf("unexpected admin idle timeout: %s", cfg.AdminIdleTimeoutDuration())
	}
	if cfg.Pprof.Enabled {
		t.Fatal("pprof should be disabled by default")
	}
	if cfg.Pprof.Addr != "127.0.0.1:6060" {
		t.Fatalf("unexpected pprof addr: %s", cfg.Pprof.Addr)
	}
	if cfg.GRPCTLSEnabled() {
		t.Fatal("grpc tls should be disabled by default")
	}
}

func TestLoadAppliesAdminOperabilityDefaults(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(""), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if got := cfg.AdminMaxListItems(); got != 1000 {
		t.Fatalf("unexpected admin max list items: %d", got)
	}
	if got := cfg.AdminAuth.RateLimitBurst; got != 0 {
		t.Fatalf("unexpected raw admin rate limit burst default: %d", got)
	}
	if got := cfg.AdminRateLimitBurst(); got != cfg.AdminAuth.RateLimitRPS {
		t.Fatalf("unexpected effective admin rate limit burst default: %d", got)
	}
	if cfg.Tracing.Enabled {
		t.Fatal("tracing should be disabled by default")
	}
	if got := cfg.TracingSamplerRatio(); got != 1.0 {
		t.Fatalf("unexpected tracing sampler ratio default: %v", got)
	}
	if got := cfg.TracingHeaders(); len(got) != 0 {
		t.Fatalf("unexpected tracing headers default: %#v", got)
	}
}

func TestAdminRateLimitBurstFallsBackToRPSWhenUnset(t *testing.T) {
	t.Parallel()

	cfg := &Config{AdminAuth: AdminAuthConfig{RateLimitRPS: 7}}
	if got := cfg.AdminRateLimitBurst(); got != 7 {
		t.Fatalf("unexpected effective admin rate limit burst: %d", got)
	}
}

func TestGRPCTLSEnabledWhenCertificatesConfigured(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		GRPCTLS: GRPCTLSConfig{
			CertPath: "/certs/tls.crt",
			KeyPath:  "/certs/tls.key",
		},
	}

	if !cfg.GRPCTLSEnabled() {
		t.Fatal("expected grpc tls to be enabled when cert and key are configured")
	}
}

func TestAdminLimitsRespectConfiguredValues(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		AdminLimits: AdminLimitsConfig{
			MaxRequestBodyBytes:  4096,
			MaxResponseBodyBytes: 16384,
		},
	}

	if got := cfg.AdminMaxRequestBodyBytes(); got != 4096 {
		t.Fatalf("unexpected admin max request body bytes: %d", got)
	}
	if got := cfg.AdminMaxResponseBodyBytes(); got != 16384 {
		t.Fatalf("unexpected admin max response body bytes: %d", got)
	}
}

func TestAdminOperabilitySettingsRespectConfiguredValues(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		AdminLimits: AdminLimitsConfig{
			MaxListItems:         250,
			MaxRequestBodyBytes:  4096,
			MaxResponseBodyBytes: 16384,
		},
		AdminAuth: AdminAuthConfig{
			RateLimitRPS:   12,
			RateLimitBurst: 36,
		},
		Tracing: TracingConfig{
			Enabled:      true,
			Endpoint:     "otel-collector:4317",
			Insecure:     true,
			SamplerRatio: float64Ptr(0.35),
			Headers: map[string]string{
				"authorization": "Bearer token",
			},
		},
	}

	if got := cfg.AdminMaxListItems(); got != 250 {
		t.Fatalf("unexpected admin max list items: %d", got)
	}
	if got := cfg.AdminRateLimitBurst(); got != 36 {
		t.Fatalf("unexpected admin rate limit burst: %d", got)
	}
	if got := cfg.TracingSamplerRatio(); got != 0.35 {
		t.Fatalf("unexpected tracing sampler ratio: %v", got)
	}
	if got := cfg.TracingHeaders()["authorization"]; got != "Bearer token" {
		t.Fatalf("unexpected tracing header value: %q", got)
	}
}

func float64Ptr(v float64) *float64 {
	return &v
}

func TestTracingSamplerRatioClampsOutOfRangeValues(t *testing.T) {
	t.Parallel()

	high := 7.0
	cfg := &Config{Tracing: TracingConfig{SamplerRatio: &high}}
	if got := cfg.TracingSamplerRatio(); got != 1.0 {
		t.Fatalf("unexpected clamped high tracing sampler ratio: %v", got)
	}

	low := -2.0
	cfg = &Config{Tracing: TracingConfig{SamplerRatio: &low}}
	if got := cfg.TracingSamplerRatio(); got != 0.0 {
		t.Fatalf("unexpected clamped low tracing sampler ratio: %v", got)
	}
}

func TestLoadPreservesExplicitZeroTracingSamplerRatio(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw := []byte("tracing:\n  samplerRatio: 0\n")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if got := cfg.TracingSamplerRatio(); got != 0.0 {
		t.Fatalf("unexpected explicit zero tracing sampler ratio: %v", got)
	}
}

func TestLoadParsesAdminOperabilityAndTracingSettings(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw := []byte(`
adminLimits:
  maxListItems: 250
adminAuth:
  rateLimitRPS: 12
  rateLimitBurst: 36
tracing:
  enabled: true
  endpoint: otel-collector:4317
  insecure: true
  samplerRatio: 0.25
  headers:
    " authorization ": " Bearer token "
`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if got := cfg.AdminLimits.MaxListItems; got != 250 {
		t.Fatalf("unexpected raw admin max list items: %d", got)
	}
	if got := cfg.AdminMaxListItems(); got != 250 {
		t.Fatalf("unexpected effective admin max list items: %d", got)
	}
	if got := cfg.AdminAuth.RateLimitBurst; got != 36 {
		t.Fatalf("unexpected raw admin rate limit burst: %d", got)
	}
	if got := cfg.AdminRateLimitBurst(); got != 36 {
		t.Fatalf("unexpected effective admin rate limit burst: %d", got)
	}
	if !cfg.Tracing.Enabled {
		t.Fatal("tracing should be enabled")
	}
	if got := cfg.Tracing.Endpoint; got != "otel-collector:4317" {
		t.Fatalf("unexpected tracing endpoint: %q", got)
	}
	if !cfg.Tracing.Insecure {
		t.Fatal("tracing insecure should be true")
	}
	if cfg.Tracing.SamplerRatio == nil {
		t.Fatal("tracing sampler ratio pointer should be populated")
	}
	if got := *cfg.Tracing.SamplerRatio; got != 0.25 {
		t.Fatalf("unexpected raw tracing sampler ratio: %v", got)
	}
	if got := cfg.TracingSamplerRatio(); got != 0.25 {
		t.Fatalf("unexpected effective tracing sampler ratio: %v", got)
	}
	if got := cfg.Tracing.Headers[" authorization "]; got != " Bearer token " {
		t.Fatalf("unexpected raw tracing header value: %q", got)
	}
	if got := cfg.TracingHeaders()["authorization"]; got != "Bearer token" {
		t.Fatalf("unexpected effective tracing header value: %q", got)
	}
}

func TestTracingHeadersTrimAndCopy(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Tracing: TracingConfig{
			Headers: map[string]string{
				" x-api-key ": " secret ",
			},
		},
	}

	headers := cfg.TracingHeaders()
	if got := headers["x-api-key"]; got != "secret" {
		t.Fatalf("unexpected trimmed tracing header: %q", got)
	}
	headers["x-api-key"] = "mutated"
	if got := cfg.Tracing.Headers[" x-api-key "]; got != " secret " {
		t.Fatalf("unexpected source tracing header after mutation: %q", got)
	}
}

func TestAdminRuntimeDurationsRespectConfiguredValues(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		AdminRuntime: AdminRuntimeConfig{
			ReadHeaderTimeout: "7s",
			ReadTimeout:       "45s",
			WriteTimeout:      "50s",
			IdleTimeout:       "4m",
		},
	}

	if got := cfg.AdminReadHeaderTimeoutDuration(); got != 7*time.Second {
		t.Fatalf("unexpected admin read header timeout: %s", got)
	}
	if got := cfg.AdminReadTimeoutDuration(); got != 45*time.Second {
		t.Fatalf("unexpected admin read timeout: %s", got)
	}
	if got := cfg.AdminWriteTimeoutDuration(); got != 50*time.Second {
		t.Fatalf("unexpected admin write timeout: %s", got)
	}
	if got := cfg.AdminIdleTimeoutDuration(); got != 4*time.Minute {
		t.Fatalf("unexpected admin idle timeout: %s", got)
	}
}

func TestLoadParsesTranslatorLimits(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw := []byte(`
translatorLimits:
  maxInputObjects: 128
  maxSnapshotObjects: 64
  maxSnapshotEndpoints: 512
`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	limits := cfg.TranslatorResourceLimits()
	if limits.MaxInputObjects != 128 {
		t.Fatalf("unexpected max input objects: %d", limits.MaxInputObjects)
	}
	if limits.MaxSnapshotObjects != 64 {
		t.Fatalf("unexpected max snapshot objects: %d", limits.MaxSnapshotObjects)
	}
	if limits.MaxSnapshotEndpoints != 512 {
		t.Fatalf("unexpected max snapshot endpoints: %d", limits.MaxSnapshotEndpoints)
	}
}

func TestTranslatorLimitsClampNonPositiveValues(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		TranslatorLimits: TranslatorLimitsConfig{
			MaxInputObjects:      -1,
			MaxSnapshotObjects:   0,
			MaxSnapshotEndpoints: -10,
		},
	}

	limits := cfg.TranslatorResourceLimits()
	if limits.MaxInputObjects != 0 {
		t.Fatalf("unexpected clamped max input objects: %d", limits.MaxInputObjects)
	}
	if limits.MaxSnapshotObjects != 0 {
		t.Fatalf("unexpected clamped max snapshot objects: %d", limits.MaxSnapshotObjects)
	}
	if limits.MaxSnapshotEndpoints != 0 {
		t.Fatalf("unexpected clamped max snapshot endpoints: %d", limits.MaxSnapshotEndpoints)
	}
}

func TestAdvertisedAddressesPrefersExplicitList(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		StatusAddress:   "127.0.0.1",
		StatusAddresses: []string{" 192.0.2.10 ", "2001:db8::10", "192.0.2.10", ""},
	}

	if got := cfg.AdvertisedAddresses(); len(got) != 2 || got[0] != "192.0.2.10" || got[1] != "2001:db8::10" {
		t.Fatalf("unexpected advertised addresses: %#v", got)
	}
}

func TestResolveBearerTokenUsesFileWhenConfigured(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("  secret-token \n"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}

	token, err := (AdminAuthConfig{BearerTokenFile: path}).ResolveBearerToken()
	if err != nil {
		t.Fatalf("resolve bearer token: %v", err)
	}
	if token != "secret-token" {
		t.Fatalf("unexpected token: %q", token)
	}
}

func TestReconcilerRunnerDurationsRespectConfiguredValues(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		ReconcilerRunner: ReconcilerRunnerConfig{
			SettleDelay:  "750ms",
			RetryBackoff: "5s",
		},
	}

	if got := cfg.ReconcilerRunnerSettleDelayDuration(); got != 750*time.Millisecond {
		t.Fatalf("unexpected settle delay: %s", got)
	}
	if got := cfg.ReconcilerRunnerRetryBackoffDuration(); got != 5*time.Second {
		t.Fatalf("unexpected retry backoff: %s", got)
	}
}

func TestGRPCRuntimeDurationsRespectConfiguredValues(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		GRPCRuntime: GRPCRuntimeConfig{
			KeepaliveTime:         "45s",
			KeepaliveTimeout:      "12s",
			MinPingInterval:       "20s",
			MaxConnectionIdle:     "3m",
			MaxConnectionAge:      "40m",
			MaxConnectionAgeGrace: "90s",
			SnapshotSendTimeout:   "35s",
			SnapshotAckTimeout:    "45s",
			PermitWithoutStream:   true,
		},
	}

	if got := cfg.GRPCKeepaliveTimeDuration(); got != 45*time.Second {
		t.Fatalf("unexpected grpc keepalive time: %s", got)
	}
	if got := cfg.GRPCKeepaliveTimeoutDuration(); got != 12*time.Second {
		t.Fatalf("unexpected grpc keepalive timeout: %s", got)
	}
	if got := cfg.GRPCMinPingIntervalDuration(); got != 20*time.Second {
		t.Fatalf("unexpected grpc min ping interval: %s", got)
	}
	if got := cfg.GRPCMaxConnectionIdleDuration(); got != 3*time.Minute {
		t.Fatalf("unexpected grpc max connection idle: %s", got)
	}
	if got := cfg.GRPCMaxConnectionAgeDuration(); got != 40*time.Minute {
		t.Fatalf("unexpected grpc max connection age: %s", got)
	}
	if got := cfg.GRPCMaxConnectionAgeGraceDuration(); got != 90*time.Second {
		t.Fatalf("unexpected grpc max connection age grace: %s", got)
	}
	if got := cfg.GRPCSnapshotSendTimeoutDuration(); got != 35*time.Second {
		t.Fatalf("unexpected grpc snapshot send timeout: %s", got)
	}
	if got := cfg.GRPCSnapshotAckTimeoutDuration(); got != 45*time.Second {
		t.Fatalf("unexpected grpc snapshot ack timeout: %s", got)
	}
	if !cfg.GRPCRuntime.PermitWithoutStream {
		t.Fatal("expected grpc permitWithoutStream to respect configured value")
	}
}

func TestGRPCSnapshotSendTimeoutFallsBackToDefaultWhenNonPositive(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		GRPCRuntime: GRPCRuntimeConfig{
			SnapshotSendTimeout: "0s",
		},
	}

	if got := cfg.GRPCSnapshotSendTimeoutDuration(); got != 5*time.Second {
		t.Fatalf("unexpected grpc snapshot send timeout fallback: %s", got)
	}
}

func TestGRPCSnapshotAckTimeoutFallsBackToDefaultWhenNonPositive(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		GRPCRuntime: GRPCRuntimeConfig{
			SnapshotAckTimeout: "0s",
		},
	}

	if got := cfg.GRPCSnapshotAckTimeoutDuration(); got != 30*time.Second {
		t.Fatalf("unexpected grpc snapshot ack timeout fallback: %s", got)
	}
}

func TestAdminRuntimeTimeoutsFallBackToDefaultsWhenNonPositive(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		AdminRuntime: AdminRuntimeConfig{
			ReadHeaderTimeout: "0s",
			ReadTimeout:       "-1s",
			WriteTimeout:      "0s",
			IdleTimeout:       "-5s",
		},
	}

	if got := cfg.AdminReadHeaderTimeoutDuration(); got != 5*time.Second {
		t.Fatalf("unexpected admin read header timeout fallback: %s", got)
	}
	if got := cfg.AdminReadTimeoutDuration(); got != 30*time.Second {
		t.Fatalf("unexpected admin read timeout fallback: %s", got)
	}
	if got := cfg.AdminWriteTimeoutDuration(); got != 30*time.Second {
		t.Fatalf("unexpected admin write timeout fallback: %s", got)
	}
	if got := cfg.AdminIdleTimeoutDuration(); got != 2*time.Minute {
		t.Fatalf("unexpected admin idle timeout fallback: %s", got)
	}
}

func TestLoadRespectsAdminRuntimeSettings(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw := `
adminRuntime:
  readHeaderTimeout: 6s
  readTimeout: 41s
  writeTimeout: 42s
  idleTimeout: 150s
`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if got := cfg.AdminReadHeaderTimeoutDuration(); got != 6*time.Second {
		t.Fatalf("unexpected admin read header timeout: %s", got)
	}
	if got := cfg.AdminReadTimeoutDuration(); got != 41*time.Second {
		t.Fatalf("unexpected admin read timeout: %s", got)
	}
	if got := cfg.AdminWriteTimeoutDuration(); got != 42*time.Second {
		t.Fatalf("unexpected admin write timeout: %s", got)
	}
	if got := cfg.AdminIdleTimeoutDuration(); got != 150*time.Second {
		t.Fatalf("unexpected admin idle timeout: %s", got)
	}
}

func TestFeaturesConfigDefaultsDisabled(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(""), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Features.EnableExperimentalGateway {
		t.Fatal("enableExperimentalGateway should default to false")
	}
	if cfg.Features.EnableAiGateway {
		t.Fatal("enableAiGateway should default to false")
	}
}

func TestFeaturesConfigEnabled(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw := `
features:
  enableExperimentalGateway: true
  enableAiGateway: true
`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if !cfg.Features.EnableExperimentalGateway {
		t.Fatal("enableExperimentalGateway should be true")
	}
	if !cfg.Features.EnableAiGateway {
		t.Fatal("enableAiGateway should be true")
	}
}

func TestLoadAppliesDashboardCapabilityDefaults(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(""), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if !cfg.DashboardEnabled() {
		t.Fatal("dashboard should be enabled by default")
	}
	for _, check := range []struct {
		name string
		got  bool
	}{
		{"aiOverview", cfg.DashboardCapabilities().AIOverview},
		{"aiServices", cfg.DashboardCapabilities().AIServices},
		{"aiTokenPolicies", cfg.DashboardCapabilities().AITokenPolicies},
		{"aiCost", cfg.DashboardCapabilities().AICost},
		{"aiTraces", cfg.DashboardCapabilities().AITraces},
		{"aiUsage", cfg.DashboardCapabilities().AIUsage},
		{"wasmPlugins", cfg.DashboardCapabilities().WasmPlugins},
		{"chatbot", cfg.DashboardCapabilities().Chatbot},
	} {
		if !check.got {
			t.Fatalf("dashboard capability %s should default to true", check.name)
		}
	}
}

func TestLoadRespectsExplicitDashboardCapabilityOverrides(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw := []byte(`
dashboard:
  enabled: false
  capabilities:
    aiOverview: false
    wasmPlugins: false
`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.DashboardEnabled() {
		t.Fatal("dashboard.enabled=false must be respected")
	}
	caps := cfg.DashboardCapabilities()
	if caps.AIOverview || caps.WasmPlugins {
		t.Fatalf("explicit dashboard capability overrides not applied: %+v", caps)
	}
	if !caps.AIServices {
		t.Fatal("unset dashboard capability should still default to true")
	}
}

func TestAdminRBACConfigParsing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw := []byte(`
nodeId: n1
cluster: c1
adminAddr: :18081
adminAuth:
  authMode: static
  bearerToken: secret
  rbac:
    roles:
      - name: admin
        permissions: [admin:*]
        matchUsers: [admin-user]
      - name: reader
        permissions: [read:*]
        matchGroups: [readers]
`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	assert.NotNil(t, cfg.AdminAuth.RBAC)
	assert.True(t, cfg.AdminAuth.RBAC.IsEnabled())
	assert.Len(t, cfg.AdminAuth.RBAC.Roles, 2)
	assert.Equal(t, "admin", cfg.AdminAuth.RBAC.Roles[0].Name)
	assert.Equal(t, PermissionAdmin, cfg.AdminAuth.RBAC.Roles[0].Permissions[0])
}

func TestAdminRBACConfigBackwardCompatible(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw := []byte(`
nodeId: n1
cluster: c1
adminAddr: :18081
adminAuth:
  authMode: static
  bearerToken: secret
`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	assert.Nil(t, cfg.AdminAuth.RBAC)
	assert.False(t, cfg.AdminAuth.RBAC.IsEnabled()) // nil receiver safe
}

func TestAdminRBACConfigValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "empty role name",
			yaml: `nodeId: n1
cluster: c1
adminAddr: :18081
adminAuth:
  authMode: static
  bearerToken: secret
  rbac:
    roles:
      - name: ''
        permissions: [read:*]
        matchUsers: [u]
`,
			wantErr: "name is required",
		},
		{
			name: "no permissions",
			yaml: `nodeId: n1
cluster: c1
adminAddr: :18081
adminAuth:
  authMode: static
  bearerToken: secret
  rbac:
    roles:
      - name: r
        permissions: []
        matchUsers: [u]
`,
			wantErr: "must have at least one permission",
		},
		{
			name: "invalid permission",
			yaml: `nodeId: n1
cluster: c1
adminAddr: :18081
adminAuth:
  authMode: static
  bearerToken: secret
  rbac:
    roles:
      - name: r
        permissions: [bad:perm]
        matchUsers: [u]
`,
			wantErr: "is not a valid permission",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			if err := os.WriteFile(path, []byte(tt.yaml), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}

			cfg, err := Load(path)
			assert.NoError(t, err, "Load should succeed; validation is separate")

			err = cfg.Validate()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
