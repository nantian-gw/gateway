package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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
	if cfg.LeaderElection.ID != "aether-gateway-controlplane-leader" {
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
	if cfg.NodeStatus.LeasePrefix != "aeg-node" {
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
