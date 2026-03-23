// Package otel provides OpenTelemetry tracing utilities for the hyperfleet-adapter.
package otel

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
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

	// envOtelExporterOtlpEndpoint is the standard OTel env var for the OTLP endpoint
	envOtelExporterOtlpEndpoint = "OTEL_EXPORTER_OTLP_ENDPOINT"

	// envOtelExporterOtlpProtocol is the standard OTel env var for the OTLP protocol
	envOtelExporterOtlpProtocol = "OTEL_EXPORTER_OTLP_PROTOCOL"

	// defaultOtlpProtocol is the default OTLP protocol when none is specified
	defaultOtlpProtocol = "grpc"
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
		log.Infof(
			ctx, "Using default trace sample ratio: %.2f (set %s to override)",
			DefaultTraceSampleRatio, EnvTraceSampleRatio,
		)
		return DefaultTraceSampleRatio
	}

	ratio, err := strconv.ParseFloat(ratioStr, 64)
	if err != nil {
		log.Warnf(
			ctx, "Invalid %s value %q, using default %.2f: %v",
			EnvTraceSampleRatio, ratioStr, DefaultTraceSampleRatio, err,
		)
		return DefaultTraceSampleRatio
	}

	if ratio < 0.0 || ratio > 1.0 {
		log.Warnf(
			ctx, "Invalid %s value %.4f (must be 0.0-1.0), using default %.2f",
			EnvTraceSampleRatio, ratio, DefaultTraceSampleRatio,
		)
		return DefaultTraceSampleRatio
	}

	log.Infof(
		ctx, "Trace sample ratio configured: %.4f (%.2f%% of traces will be sampled)", ratio, ratio*100,
	)
	return ratio
}

// createExporter creates an OTLP SpanExporter when OTEL_EXPORTER_OTLP_ENDPOINT is set.
// Returns nil when no endpoint is configured (spans remain local-only for trace ID generation).
// The protocol defaults to gRPC, configurable via OTEL_EXPORTER_OTLP_PROTOCOL.
func createExporter(ctx context.Context, log logger.Logger) (sdktrace.SpanExporter, error) {
	otlpEndpoint := os.Getenv(envOtelExporterOtlpEndpoint)
	if otlpEndpoint == "" {
		log.Infof(ctx, "No %s configured, traces will not be exported (trace IDs still generated for log correlation)",
			envOtelExporterOtlpEndpoint)
		return nil, nil
	}

	protocol := os.Getenv(envOtelExporterOtlpProtocol)
	var exporter sdktrace.SpanExporter
	var err error

	switch strings.ToLower(protocol) {
	case "http", "http/protobuf":
		exporter, err = otlptracehttp.New(ctx)
	case defaultOtlpProtocol, "":
		protocol = defaultOtlpProtocol
		exporter, err = otlptracegrpc.New(ctx)
	default:
		log.Warnf(ctx, "Unrecognized %s value %q, using default %s",
			envOtelExporterOtlpProtocol, protocol, defaultOtlpProtocol)
		protocol = defaultOtlpProtocol
		exporter, err = otlptracegrpc.New(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter (protocol=%s): %w", protocol, err)
	}

	log.Infof(ctx, "OTLP trace exporter configured: protocol=%s", protocol)
	return exporter, nil
}

// InitTracer initializes OpenTelemetry TracerProvider with optional span export.
//
// When OTEL_EXPORTER_OTLP_ENDPOINT is set, spans are batched and exported via OTLP
// (gRPC by default, or HTTP via OTEL_EXPORTER_OTLP_PROTOCOL).
// When no endpoint is configured, the TracerProvider still generates trace IDs and span IDs
// for log correlation and W3C context propagation, but spans are not exported.
//
// The sampler uses ParentBased(TraceIDRatioBased(sampleRatio)) which:
//   - Respects the parent span's sampling decision when present (from traceparent header)
//   - Applies probabilistic sampling for root spans based on sampleRatio
func InitTracer(
	log logger.Logger, serviceName, serviceVersion string, sampleRatio float64,
) (*sdktrace.TracerProvider, error) {
	ctx := context.Background()

	// Create exporter (nil when no OTLP endpoint configured)
	exporter, err := createExporter(ctx, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	// Create resource with service attributes.
	// Note: We don't merge with resource.Default() to avoid schema URL conflicts
	// between the SDK's bundled semconv version and our imported version.
	res, err := resource.New(
		ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
		),
		resource.WithProcessRuntimeDescription(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
	)
	if err != nil {
		if exporter != nil {
			if shutdownErr := exporter.Shutdown(ctx); shutdownErr != nil {
				log.Warnf(ctx, "Failed to shutdown exporter during cleanup: %v", shutdownErr)
			}
		}
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Use ParentBased sampler with TraceIDRatioBased for root spans:
	// - If parent span exists: inherit parent's sampling decision
	// - If no parent (root span): apply probabilistic sampling based on trace ID
	// This enables proper sampling propagation across service boundaries
	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(sampleRatio))

	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	}
	if exporter != nil {
		opts = append(opts, sdktrace.WithBatcher(exporter))
	}

	tp := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)
	// TraceContext propagator handles W3C traceparent/tracestate headers
	// ensuring sampling decisions propagate through message headers
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp, nil
}
