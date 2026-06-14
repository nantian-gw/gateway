package observability

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

type TracingConfig struct {
	Enabled      bool
	Endpoint     string
	Insecure     bool
	SamplerRatio float64
	Headers      map[string]string
}

type TracingSummary struct {
	Enabled      bool
	Endpoint     string
	Insecure     bool
	SamplerRatio float64
	HeaderCount  int
}

func SummarizeTracing(cfg TracingConfig) TracingSummary {
	headers := traceHeaders(cfg.Headers)
	return TracingSummary{
		Enabled:      cfg.Enabled,
		Endpoint:     strings.TrimSpace(cfg.Endpoint),
		Insecure:     cfg.Insecure,
		SamplerRatio: clampSamplerRatio(cfg.SamplerRatio),
		HeaderCount:  len(headers),
	}
}

func ConfigureTracing(ctx context.Context, cfg TracingConfig) (func(context.Context) error, error) {
	if !cfg.Enabled {
		return func(context.Context) error { return nil }, nil
	}

	exporterOptions, err := buildTraceExporterOptions(cfg)
	if err != nil {
		return nil, err
	}

	exporter, err := otlptracegrpc.New(ctx, exporterOptions...)
	if err != nil {
		return nil, err
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(clampSamplerRatio(cfg.SamplerRatio))),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(sdkresource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("nantian-gw-controlplane"),
		)),
	)

	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return provider.Shutdown, nil
}

func buildTraceExporterOptions(cfg TracingConfig) ([]otlptracegrpc.Option, error) {
	options := make([]otlptracegrpc.Option, 0, 3)
	if endpoint := strings.TrimSpace(cfg.Endpoint); endpoint != "" {
		options = append(options, otlptracegrpc.WithEndpoint(endpoint))
	}
	if cfg.Insecure {
		options = append(options, otlptracegrpc.WithInsecure())
	}
	if headers := traceHeaders(cfg.Headers); len(headers) > 0 {
		options = append(options, otlptracegrpc.WithHeaders(headers))
	}
	return options, nil
}

func clampSamplerRatio(ratio float64) float64 {
	switch {
	case ratio < 0:
		return 0
	case ratio > 1:
		return 1
	default:
		return ratio
	}
}

func traceHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(headers))
	for key, value := range headers {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		cloned[key] = strings.TrimSpace(value)
	}
	if len(cloned) == 0 {
		return nil
	}
	return cloned
}
