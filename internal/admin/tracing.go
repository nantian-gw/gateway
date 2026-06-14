package admin

import (
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
)

func wrapTracingHandler(next http.Handler, route string) http.Handler {
	if next == nil {
		return nil
	}

	tracer := otel.Tracer("github.com/nantian-gw/gateway/internal/admin")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		routeName := route
		if routeName == "" {
			routeName = classifyAdminRoute(r.URL.Path)
		}

		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := tracer.Start(ctx, "admin "+r.Method+" "+r.URL.Path)
		defer span.End()

		recorder := &statusRecordingResponseWriter{ResponseWriter: w}
		next.ServeHTTP(recorder, r.WithContext(ctx))

		span.SetAttributes(
			attribute.String("http.method", normalizeAdminMethod(r.Method)),
			attribute.String("http.route", routeName),
			attribute.Int("http.status_code", recorder.statusCode()),
		)
	})
}
