package otel

import (
	"context"
	"testing"

	"github.com/cloudevents/sdk-go/v2/event"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func init() {
	// Ensure the global propagator is set for tests
	otel.SetTextMapPropagator(propagation.TraceContext{})
}

func TestExtractTraceContextFromCloudEvent(t *testing.T) {
	// Valid W3C trace context values for testing
	// Format: version-traceid-parentid-flags
	const (
		validTraceID     = "0af7651916cd43dd8448eb211c80319c"
		validSpanID      = "b7ad6b7169203331"
		validTraceparent = "00-" + validTraceID + "-" + validSpanID + "-01"
		validTracestate  = "vendor1=value1,vendor2=value2"
	)

	t.Run("nil_event_returns_unchanged_context", func(t *testing.T) {
		ctx := context.Background()
		result := ExtractTraceContextFromCloudEvent(ctx, nil)

		// Context should be unchanged
		if result != ctx {
			t.Error("Expected context to be unchanged for nil event")
		}

		// No span context should be present
		spanCtx := trace.SpanContextFromContext(result)
		if spanCtx.IsValid() {
			t.Error("Expected no valid span context for nil event")
		}
	})

	t.Run("event_without_extensions_returns_unchanged_context", func(t *testing.T) {
		ctx := context.Background()
		evt := event.New()
		evt.SetID("test-id")
		evt.SetType("test.type")
		evt.SetSource("/test")

		result := ExtractTraceContextFromCloudEvent(ctx, &evt)

		// No span context should be present
		spanCtx := trace.SpanContextFromContext(result)
		if spanCtx.IsValid() {
			t.Error("Expected no valid span context for event without extensions")
		}
	})

	t.Run("event_with_empty_traceparent_returns_unchanged_context", func(t *testing.T) {
		ctx := context.Background()
		evt := event.New()
		evt.SetID("test-id")
		evt.SetType("test.type")
		evt.SetSource("/test")
		evt.SetExtension("traceparent", "")

		result := ExtractTraceContextFromCloudEvent(ctx, &evt)

		// No span context should be present
		spanCtx := trace.SpanContextFromContext(result)
		if spanCtx.IsValid() {
			t.Error("Expected no valid span context for empty traceparent")
		}
	})

	t.Run("event_with_valid_traceparent_extracts_trace_context", func(t *testing.T) {
		ctx := context.Background()
		evt := event.New()
		evt.SetID("test-id")
		evt.SetType("test.type")
		evt.SetSource("/test")
		evt.SetExtension("traceparent", validTraceparent)

		result := ExtractTraceContextFromCloudEvent(ctx, &evt)

		// Span context should be present and valid
		spanCtx := trace.SpanContextFromContext(result)
		if !spanCtx.IsValid() {
			t.Fatal("Expected valid span context")
		}

		// Verify trace ID
		if spanCtx.TraceID().String() != validTraceID {
			t.Errorf("Expected trace ID %s, got %s", validTraceID, spanCtx.TraceID().String())
		}

		// Verify span ID (parent span ID from traceparent)
		if spanCtx.SpanID().String() != validSpanID {
			t.Errorf("Expected span ID %s, got %s", validSpanID, spanCtx.SpanID().String())
		}

		// Verify sampled flag (01 means sampled)
		if !spanCtx.IsSampled() {
			t.Error("Expected span context to be sampled")
		}
	})

	t.Run("event_with_traceparent_and_tracestate_extracts_both", func(t *testing.T) {
		ctx := context.Background()
		evt := event.New()
		evt.SetID("test-id")
		evt.SetType("test.type")
		evt.SetSource("/test")
		evt.SetExtension("traceparent", validTraceparent)
		evt.SetExtension("tracestate", validTracestate)

		result := ExtractTraceContextFromCloudEvent(ctx, &evt)

		// Span context should be present and valid
		spanCtx := trace.SpanContextFromContext(result)
		if !spanCtx.IsValid() {
			t.Fatal("Expected valid span context")
		}

		// Verify trace ID
		if spanCtx.TraceID().String() != validTraceID {
			t.Errorf("Expected trace ID %s, got %s", validTraceID, spanCtx.TraceID().String())
		}

		// Verify tracestate is preserved
		traceState := spanCtx.TraceState()
		if traceState.Len() == 0 {
			t.Error("Expected tracestate to be preserved")
		}

		// Verify vendor1 value
		if val := traceState.Get("vendor1"); val != "value1" {
			t.Errorf("Expected tracestate vendor1=value1, got vendor1=%s", val)
		}
	})

	t.Run("event_with_invalid_traceparent_handles_gracefully", func(t *testing.T) {
		testCases := []struct {
			name        string
			traceparent string
		}{
			{"malformed_format", "not-a-valid-traceparent"},
			{"wrong_version", "ff-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"},
			{"short_trace_id", "00-0af7651916cd43dd-b7ad6b7169203331-01"},
			{"short_span_id", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b71-01"},
			{"missing_parts", "00-0af7651916cd43dd8448eb211c80319c"},
			{"all_zeros_trace_id", "00-00000000000000000000000000000000-b7ad6b7169203331-01"},
			{"all_zeros_span_id", "00-0af7651916cd43dd8448eb211c80319c-0000000000000000-01"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx := context.Background()
				evt := event.New()
				evt.SetID("test-id")
				evt.SetType("test.type")
				evt.SetSource("/test")
				evt.SetExtension("traceparent", tc.traceparent)

				// Should not panic
				result := ExtractTraceContextFromCloudEvent(ctx, &evt)

				// Invalid traceparent should result in invalid span context
				spanCtx := trace.SpanContextFromContext(result)
				if spanCtx.IsValid() {
					t.Errorf("Expected invalid span context for malformed traceparent %q", tc.traceparent)
				}
			})
		}
	})

	t.Run("event_with_non_string_traceparent_returns_unchanged_context", func(t *testing.T) {
		ctx := context.Background()
		evt := event.New()
		evt.SetID("test-id")
		evt.SetType("test.type")
		evt.SetSource("/test")
		// Set traceparent as non-string type
		evt.SetExtension("traceparent", 12345)

		result := ExtractTraceContextFromCloudEvent(ctx, &evt)

		// No span context should be present
		spanCtx := trace.SpanContextFromContext(result)
		if spanCtx.IsValid() {
			t.Error("Expected no valid span context for non-string traceparent")
		}
	})

	t.Run("event_with_traceparent_only_no_tracestate", func(t *testing.T) {
		ctx := context.Background()
		evt := event.New()
		evt.SetID("test-id")
		evt.SetType("test.type")
		evt.SetSource("/test")
		evt.SetExtension("traceparent", validTraceparent)
		// No tracestate set

		result := ExtractTraceContextFromCloudEvent(ctx, &evt)

		// Span context should still be valid
		spanCtx := trace.SpanContextFromContext(result)
		if !spanCtx.IsValid() {
			t.Fatal("Expected valid span context")
		}

		// Tracestate should be empty
		if spanCtx.TraceState().Len() != 0 {
			t.Error("Expected empty tracestate when not provided")
		}
	})

	t.Run("unsampled_trace_context_is_extracted", func(t *testing.T) {
		ctx := context.Background()
		evt := event.New()
		evt.SetID("test-id")
		evt.SetType("test.type")
		evt.SetSource("/test")
		// flags=00 means not sampled
		unsampledTraceparent := "00-" + validTraceID + "-" + validSpanID + "-00"
		evt.SetExtension("traceparent", unsampledTraceparent)

		result := ExtractTraceContextFromCloudEvent(ctx, &evt)

		spanCtx := trace.SpanContextFromContext(result)
		if !spanCtx.IsValid() {
			t.Fatal("Expected valid span context")
		}

		// Should NOT be sampled
		if spanCtx.IsSampled() {
			t.Error("Expected span context to NOT be sampled (flags=00)")
		}
	})
}
