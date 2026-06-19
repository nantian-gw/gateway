package main

import (
	"log/slog"
	"strings"

	"github.com/nantian-gw/gateway/internal/config"
	"github.com/nantian-gw/gateway/internal/observability"
)

func controlplaneTracingConfig(cfg *config.Config) observability.TracingConfig {
	if cfg == nil {
		return observability.TracingConfig{}
	}

	return observability.TracingConfig{
		Enabled:      cfg.Tracing.Enabled,
		Endpoint:     strings.TrimSpace(cfg.Tracing.Endpoint),
		Insecure:     cfg.Tracing.Insecure,
		SamplerRatio: cfg.TracingSamplerRatio(),
		Headers:      cfg.TracingHeaders(),
	}
}

func logControlplaneTracingStatus(logger *slog.Logger, cfg observability.TracingConfig) {
	if logger == nil {
		return
	}

	summary := observability.SummarizeTracing(cfg)
	logger.Info(
		"configured controlplane tracing",
		"enabled", summary.Enabled,
		"endpoint", summary.Endpoint,
		"insecure", summary.Insecure,
		"sampler_ratio", summary.SamplerRatio,
		"header_count", summary.HeaderCount,
	)
}
