package tracing

import (
	"context"

	"github.com/amaumene/gomenarr/internal/platform/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
)

// Setup initializes OpenTelemetry tracing
func Setup(cfg config.TracingConfig) (*tracesdk.TracerProvider, error) {
	if !cfg.Enabled {
		// Return a no-op tracer provider
		return trace.NewTracerProvider(), nil
	}

	// For now, return a simple tracer provider
	// In production, you would configure exporters here
	tp := tracesdk.NewTracerProvider(
		tracesdk.WithSampler(tracesdk.AlwaysSample()),
	)

	otel.SetTracerProvider(tp)
	return tp, nil
}

// Shutdown gracefully shuts down the tracer provider
func Shutdown(ctx context.Context, tp *tracesdk.TracerProvider) error {
	if tp == nil {
		return nil
	}
	return tp.Shutdown(ctx)
}
