package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// Tracer wraps OpenTelemetry tracer
type Tracer struct {
	tracer oteltrace.Tracer
}

// New creates a new tracer
func New(serviceName string, enabled bool) (*Tracer, error) {
	if !enabled {
		// Return a no-op tracer
		return &Tracer{
			tracer: otel.Tracer(serviceName),
		}, nil
	}

	// Create trace provider
	tp := trace.NewTracerProvider()
	otel.SetTracerProvider(tp)

	return &Tracer{
		tracer: tp.Tracer(serviceName),
	}, nil
}

// Start starts a new span
func (t *Tracer) Start(ctx context.Context, spanName string, opts ...oteltrace.SpanStartOption) (context.Context, oteltrace.Span) {
	return t.tracer.Start(ctx, spanName, opts...)
}

// GetTracer returns the underlying OpenTelemetry tracer
func (t *Tracer) GetTracer() oteltrace.Tracer {
	return t.tracer
}

// StartSpan is a helper to start a span
func StartSpan(ctx context.Context, spanName string) (context.Context, oteltrace.Span) {
	tracer := otel.Tracer("gomenarr")
	return tracer.Start(ctx, spanName)
}
