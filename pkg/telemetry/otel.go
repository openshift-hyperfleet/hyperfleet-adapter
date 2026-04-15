// Package telemetry provides OpenTelemetry tracing utilities for the hyperfleet-adapter.
package telemetry

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
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

// Tracing configuration constants
const (
	// envOtelTracesSampler is the standard OTel env var for selecting the sampler type.
	// See: https://opentelemetry.io/docs/specs/otel/configuration/sdk-environment-variables/
	envOtelTracesSampler = "OTEL_TRACES_SAMPLER"

	// envOtelTracesSamplerArg is the standard OTel env var for the sampler argument.
	// For ratio-based samplers, this is a float64 between 0.0 and 1.0.
	envOtelTracesSamplerArg = "OTEL_TRACES_SAMPLER_ARG"

	// Sampler type values for OTEL_TRACES_SAMPLER.
	// See: https://opentelemetry.io/docs/specs/otel/configuration/sdk-environment-variables/#general-sdk-configuration
	samplerAlwaysOn         = "always_on"
	samplerAlwaysOff        = "always_off"
	samplerTraceIDRatio     = "traceidratio"
	parentBasedTraceIDRatio = "parentbased_traceidratio"
	parentBasedAlwaysOn     = "parentbased_always_on"
	parentBasedAlwaysOff    = "parentbased_always_off"

	// envOtelExporterOtlpEndpoint is the standard OTel env var for the OTLP endpoint
	envOtelExporterOtlpEndpoint = "OTEL_EXPORTER_OTLP_ENDPOINT"

	// envOtelExporterOtlpTracesEndpoint is the signal-specific OTel env var for the traces endpoint
	envOtelExporterOtlpTracesEndpoint = "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"

	// envOtelExporterOtlpProtocol is the standard OTel env var for the OTLP protocol
	envOtelExporterOtlpProtocol = "OTEL_EXPORTER_OTLP_PROTOCOL"

	// envOtelExporterOtlpTracesProtocol is the signal-specific OTel env var for the traces protocol
	envOtelExporterOtlpTracesProtocol = "OTEL_EXPORTER_OTLP_TRACES_PROTOCOL"

	// defaultOtlpProtocol is the default OTLP protocol when none is specified.
	// Per HyperFleet tracing standard, the default is "grpc".
	defaultOtlpProtocol = "grpc"

	// defaultSamplingRate is the default sampling ratio for OTel
	defaultSamplingRate = 1.0
)

// createExporter creates a SpanExporter based on OTLP environment variables.
// When no endpoint is configured, returns a stdout exporter for local development.
// The protocol defaults to grpc (per HyperFleet tracing standard), configurable via OTEL_EXPORTER_OTLP_PROTOCOL.
func createExporter(ctx context.Context, log logger.Logger) (sdktrace.SpanExporter, error) {
	// Check if an OTLP endpoint is configured (presence check only).
	// The actual endpoint value is read by the OTel SDK from env vars directly,
	// so we don't pass otlpEndpoint to the exporter constructors.
	otlpEndpoint := os.Getenv(envOtelExporterOtlpTracesEndpoint)
	if otlpEndpoint == "" {
		otlpEndpoint = os.Getenv(envOtelExporterOtlpEndpoint)
	}
	if otlpEndpoint == "" {
		log.Infof(ctx, "No %s or %s configured, using stdout exporter",
			envOtelExporterOtlpTracesEndpoint, envOtelExporterOtlpEndpoint)
		return stdouttrace.New(stdouttrace.WithPrettyPrint())
	}

	protocol := os.Getenv(envOtelExporterOtlpTracesProtocol)
	protocolSource := envOtelExporterOtlpTracesProtocol
	if protocol == "" {
		protocol = os.Getenv(envOtelExporterOtlpProtocol)
		protocolSource = envOtelExporterOtlpProtocol
	}
	var exporter sdktrace.SpanExporter
	var err error

	switch strings.ToLower(protocol) {
	case "http/protobuf", "http":
		exporter, err = otlptracehttp.New(ctx)
	case defaultOtlpProtocol, "": // gRPC (default per HyperFleet tracing standard), or unset
		protocol = defaultOtlpProtocol
		exporter, err = otlptracegrpc.New(ctx)
	default:
		log.Warnf(ctx, "Unrecognized %s value %q, using default %s",
			protocolSource, protocol, defaultOtlpProtocol)
		protocol = defaultOtlpProtocol
		exporter, err = otlptracegrpc.New(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter (protocol=%s): %w", protocol, err)
	}

	log.Infof(ctx, "OTLP trace exporter configured: protocol=%s", protocol)
	return exporter, nil
}

// InitTraceProvider initializes OpenTelemetry TracerProvider.
//
// Configuration is driven by standard OpenTelemetry environment variables:
//   - OTEL_EXPORTER_OTLP_ENDPOINT / OTEL_EXPORTER_OTLP_TRACES_ENDPOINT: OTLP endpoint (stdout if unset)
//   - OTEL_EXPORTER_OTLP_PROTOCOL / OTEL_EXPORTER_OTLP_TRACES_PROTOCOL: "grpc" (default) or "http/protobuf"
//   - OTEL_TRACES_SAMPLER: sampler type (default: "parentbased_traceidratio")
//   - OTEL_TRACES_SAMPLER_ARG: sampling rate 0.0-1.0 (default: 1.0)
func InitTraceProvider(
	ctx context.Context, log logger.Logger, serviceName, serviceVersion string,
) (*sdktrace.TracerProvider, error) {
	// Create exporter (nil when no OTLP endpoint configured)
	exporter, err := createExporter(ctx, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	// Create resource with service attributes.
	// Note: We don't merge with resource.Default() to avoid schema URL conflicts
	// between the SDK's bundled semconv version and our imported version.
	res, err := resource.New(ctx, resource.WithFromEnv(), resource.WithAttributes(
		semconv.ServiceName(serviceName),
		semconv.ServiceVersion(serviceVersion),
	),
		resource.WithProcessRuntimeDescription(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
	)

	if err != nil {
		if shutdownErr := exporter.Shutdown(ctx); shutdownErr != nil {
			log.Warnf(ctx, "Failed to shutdown exporter during cleanup: %v", shutdownErr)
		}
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Use ParentBased sampler with TraceIDRatioBased for root spans:
	// - If parent span exists: inherit parent's sampling decision
	// - If no parent (root span): apply probabilistic sampling based on trace ID
	// This enables proper sampling propagation across service boundaries
	sampler := selectSampler(ctx, log)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)
	otel.SetTracerProvider(tp)
	// TraceContext propagator handles W3C traceparent/tracestate headers
	// ensuring sampling decisions propagate through message headers
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	return tp, nil
}

func selectSampler(ctx context.Context, log logger.Logger) sdktrace.Sampler {
	samplerType := strings.ToLower(os.Getenv(envOtelTracesSampler))
	switch samplerType {
	case samplerAlwaysOn:
		return sdktrace.AlwaysSample()
	case samplerAlwaysOff:
		return sdktrace.NeverSample()
	case samplerTraceIDRatio:
		return sdktrace.TraceIDRatioBased(parseSamplingRate(ctx, log))
	case parentBasedTraceIDRatio, "":
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(parseSamplingRate(ctx, log)))
	case parentBasedAlwaysOn:
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	case parentBasedAlwaysOff:
		return sdktrace.ParentBased(sdktrace.NeverSample())
	default:
		log.Warnf(ctx, "Unrecognized %s value %q, using default parentbased_traceidratio",
			envOtelTracesSampler, samplerType)
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(parseSamplingRate(ctx, log)))
	}
}

func parseSamplingRate(ctx context.Context, log logger.Logger) float64 {
	rate := defaultSamplingRate
	if arg := os.Getenv(envOtelTracesSamplerArg); arg != "" {
		if parsedRate, err := strconv.ParseFloat(arg, 64); err == nil &&
			parsedRate >= 0.0 && parsedRate <= 1.0 {
			rate = parsedRate
		} else {
			log.Warnf(ctx, "Invalid %s value %q, using default %.1f",
				envOtelTracesSamplerArg, arg, defaultSamplingRate)
		}
	}
	return rate
}
