// Package otel provides OpenTelemetry tracing utilities for the hyperfleet-adapter.
package otel

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

// Tracing configuration constants
const (
	// EnvTraceSampleRatio is the environment variable for trace sampling ratio
	EnvTraceSampleRatio = "TRACE_SAMPLE_RATIO"

	// DefaultTraceSampleRatio is the default trace sampling ratio (10% of traces)
	// Can be overridden via TRACE_SAMPLE_RATIO env var
	DefaultTraceSampleRatio = 0.1
)

// GetTraceSampleRatio reads the trace sample ratio from TRACE_SAMPLE_RATIO env var.
// Returns DefaultTraceSampleRatio (0.1 = 10%) if not set or invalid.
// Valid range is 0.0 to 1.0 where:
//   - 0.0 = sample no traces (not recommended, use for debugging only)
//   - 0.01 = sample 1% of traces (high volume systems)
//   - 0.1 = sample 10% of traces (default, moderate volume)
//   - 1.0 = sample all traces (development/debugging only)
func GetTraceSampleRatio(log logger.Logger, ctx context.Context) float64 {
	ratioStr := os.Getenv(EnvTraceSampleRatio)
	if ratioStr == "" {
		log.Infof(ctx, "Using default trace sample ratio: %.2f (set %s to override)", DefaultTraceSampleRatio, EnvTraceSampleRatio)
		return DefaultTraceSampleRatio
	}

	ratio, err := strconv.ParseFloat(ratioStr, 64)
	if err != nil {
		log.Warnf(ctx, "Invalid %s value %q, using default %.2f: %v", EnvTraceSampleRatio, ratioStr, DefaultTraceSampleRatio, err)
		return DefaultTraceSampleRatio
	}

	if ratio < 0.0 || ratio > 1.0 {
		log.Warnf(ctx, "Invalid %s value %.4f (must be 0.0-1.0), using default %.2f", EnvTraceSampleRatio, ratio, DefaultTraceSampleRatio)
		return DefaultTraceSampleRatio
	}

	log.Infof(ctx, "Trace sample ratio configured: %.4f (%.2f%% of traces will be sampled)", ratio, ratio*100)
	return ratio
}

// InitTracer initializes OpenTelemetry TracerProvider for generating trace_id and span_id.
// These IDs are used for:
// 1. Log correlation (via logger.WithOTelTraceContext)
// 2. HTTP request propagation (via W3C Trace Context headers)
//
// The sampler uses ParentBased(TraceIDRatioBased(sampleRatio)) which:
// - Respects the parent span's sampling decision when present (from traceparent header)
// - Applies probabilistic sampling for root spans based on sampleRatio
// This allows distributed tracing visibility while controlling observability costs.
func InitTracer(serviceName, serviceVersion string, sampleRatio float64) (*sdktrace.TracerProvider, error) {
	// Create resource with service attributes.
	// Note: We don't merge with resource.Default() to avoid schema URL conflicts
	// between the SDK's bundled semconv version and our imported version.
	res, err := resource.New(
		context.Background(),
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
		),
		resource.WithProcessRuntimeDescription(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Use ParentBased sampler with TraceIDRatioBased for root spans:
	// - If parent span exists: inherit parent's sampling decision
	// - If no parent (root span): apply probabilistic sampling based on trace ID
	// This enables proper sampling propagation across service boundaries
	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(sampleRatio))

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)
	otel.SetTracerProvider(tp)
	// TraceContext propagator handles W3C traceparent/tracestate headers
	// ensuring sampling decisions propagate through message headers
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp, nil
}
