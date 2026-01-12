package otel

import (
	"context"

	"github.com/cloudevents/sdk-go/v2/event"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// ExtractTraceContextFromCloudEvent extracts W3C trace context from CloudEvent extensions.
// CloudEvents can carry traceparent and tracestate as extension attributes,
// allowing distributed tracing across event-driven systems.
//
// If trace context is present, the returned context will have the parent span
// information, making any subsequent spans children of the upstream trace.
// If no trace context is found, the original context is returned unchanged,
// and any new spans will be root spans.
//
// Example CloudEvent with trace context:
//
//	{
//	  "specversion": "1.0",
//	  "type": "cluster.created",
//	  "source": "/sentinel",
//	  "id": "abc-123",
//	  "traceparent": "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
//	  "tracestate": "vendor=value"
//	}
func ExtractTraceContextFromCloudEvent(ctx context.Context, evt *event.Event) context.Context {
	if evt == nil {
		return ctx
	}

	extensions := evt.Extensions()
	if extensions == nil {
		return ctx
	}

	carrier := propagation.MapCarrier{}

	// Extract traceparent (required for W3C Trace Context)
	if traceparent, ok := extensions["traceparent"].(string); ok && traceparent != "" {
		carrier["traceparent"] = traceparent
	} else {
		// No traceparent means no trace context to extract
		return ctx
	}

	// Extract tracestate (optional, carries vendor-specific trace data)
	if tracestate, ok := extensions["tracestate"].(string); ok && tracestate != "" {
		carrier["tracestate"] = tracestate
	}

	// Use the global propagator to extract trace context into the context
	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}
