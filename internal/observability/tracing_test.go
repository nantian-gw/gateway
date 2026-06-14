package observability

import (
	"context"
	"testing"
)

func TestConfigureTracingDisabledReturnsNoopShutdown(t *testing.T) {
	t.Parallel()

	shutdown, err := ConfigureTracing(context.Background(), TracingConfig{
		Enabled: false,
	})
	if err != nil {
		t.Fatalf("ConfigureTracing returned error: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("disabled tracing shutdown returned error: %v", err)
	}
}

func TestConfigureTracingClampsSamplerAndAppliesHeaders(t *testing.T) {
	t.Parallel()

	cfg := TracingConfig{
		Enabled:      true,
		Endpoint:     "127.0.0.1:4317",
		Insecure:     true,
		SamplerRatio: 5,
		Headers: map[string]string{
			"authorization": "Bearer token",
		},
	}

	opts, err := buildTraceExporterOptions(cfg)
	if err != nil {
		t.Fatalf("buildTraceExporterOptions returned error: %v", err)
	}
	if len(opts) != 3 {
		t.Fatalf("unexpected exporter option count: %d", len(opts))
	}
	if got := clampSamplerRatio(cfg.SamplerRatio); got != 1.0 {
		t.Fatalf("unexpected clamped sampler ratio: %v", got)
	}
}
