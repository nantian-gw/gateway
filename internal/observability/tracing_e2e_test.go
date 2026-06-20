package observability

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestEndToEndSpanHierarchy(t *testing.T) {
	t.Parallel()

	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSyncer(exporter),
	)
	otel.SetTracerProvider(provider)
	defer func() { _ = provider.Shutdown(context.Background()) }()

	tracer := otel.Tracer("test-tracer")
	ctx, rootSpan := tracer.Start(context.Background(), "controlplane.syncer.publish_snapshot")
	rootSpan.SetAttributes(
		attribute.String("snapshot.scope", "full"),
		attribute.Int("snapshot.gateway_key_count", 3),
	)

	_, buildSpan := tracer.Start(ctx, "controlplane.syncer.build_snapshot")
	buildSpan.SetAttributes(attribute.Int("snapshot.route_key_count", 150))
	buildSpan.End()

	_, infraSpan := tracer.Start(ctx, "controlplane.infrastructure.reconcile")
	infraSpan.SetAttributes(attribute.Int("infrastructure.managed_gateways", 2))
	infraSpan.End()

	rootSpan.RecordError(errors.New("test error"))
	rootSpan.SetStatus(codes.Error, "test error")
	rootSpan.End()

	provider.ForceFlush(context.Background())
	spans := exporter.GetSpans()

	if len(spans) < 3 {
		t.Fatalf("expected >= 3 spans, got %d", len(spans))
	}

	var hasRoot, hasBuild, hasInfra bool
	for _, s := range spans {
		switch s.Name {
		case "controlplane.syncer.publish_snapshot":
			hasRoot = true
			if s.Status.Code != codes.Error {
				t.Error("root span should have error status")
			}
		case "controlplane.syncer.build_snapshot":
			hasBuild = true
			if s.Parent.SpanID() != rootSpan.SpanContext().SpanID() {
				t.Error("build span should be child of root span")
			}
		case "controlplane.infrastructure.reconcile":
			hasInfra = true
		}
	}

	if !hasRoot {
		t.Error("root span not found")
	}
	if !hasBuild {
		t.Error("build span not found")
	}
	if !hasInfra {
		t.Error("infra span not found")
	}
}

func TestEndToEndAIInferenceSpan(t *testing.T) {
	t.Parallel()

	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSyncer(exporter),
	)
	otel.SetTracerProvider(provider)
	defer func() { _ = provider.Shutdown(context.Background()) }()

	tracer := otel.Tracer("test-ai-tracer")
	_, span := tracer.Start(context.Background(), "ai.inference")
	span.SetAttributes(
		attribute.String("ai.model", "gpt-4o"),
		attribute.String("ai.format", "openai"),
		attribute.Bool("ai.stream", false),
		attribute.Int64("ai.prompt_tokens", 150),
		attribute.Int64("ai.completion_tokens", 80),
	)
	span.End()

	provider.ForceFlush(context.Background())
	spans := exporter.GetSpans()

	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name != "ai.inference" {
		t.Errorf("span name = %q, want ai.inference", spans[0].Name)
	}

	attrs := spans[0].Attributes
	if v := attrVal(attrs, "ai.model"); v != "gpt-4o" {
		t.Errorf("ai.model = %q, want gpt-4o", v)
	}
	if v := attrIntVal(attrs, "ai.prompt_tokens"); v != 150 {
		t.Errorf("ai.prompt_tokens = %d, want 150", v)
	}
}

func TestEndToEndW3CPropagationEnabled(t *testing.T) {
	t.Parallel()

	otel.SetTextMapPropagator(propagation.TraceContext{})
	prop := otel.GetTextMapPropagator()
	fields := prop.Fields()
	if len(fields) == 0 {
		t.Error("text map propagator should have fields for W3C traceparent")
	}
}

func attrVal(attrs []attribute.KeyValue, key string) string {
	for _, a := range attrs {
		if string(a.Key) == key {
			return a.Value.AsString()
		}
	}
	return ""
}

func attrIntVal(attrs []attribute.KeyValue, key string) int64 {
	for _, a := range attrs {
		if string(a.Key) == key {
			return a.Value.AsInt64()
		}
	}
	return -1
}
