package logger

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const (
	// Required fields (per logging spec)
	ComponentKey contextKey = "component"
	VersionKey   contextKey = "version"
	HostnameKey  contextKey = "hostname"

	// Error fields (per logging spec)
	ErrorKey      contextKey = "error"
	StackTraceKey contextKey = "stack_trace"

	// Correlation fields (distributed tracing)
	TraceIDKey contextKey = "trace_id"
	SpanIDKey  contextKey = "span_id"
	EventIDKey contextKey = "event_id"

	// Resource fields
	ClusterIDKey      contextKey = "cluster_id"
	ResourceTypeKey   contextKey = "resource_type"
	ResourceNameKey   contextKey = "resource_name"
	ResourceResultKey contextKey = "resource_result"

	// Adapter-specific fields
	AdapterKey            contextKey = "adapter"
	ObservedGenerationKey contextKey = "observed_generation"
	SubscriptionKey       contextKey = "subscription"

	// Dynamic log fields
	LogFieldsKey contextKey = "log_fields"
)

// LogFields holds dynamic key-value pairs for logging
type LogFields map[string]interface{}

// -----------------------------------------------------------------------------
// Context Setters
// -----------------------------------------------------------------------------

// WithLogField adds a single dynamic log field to the context
// These fields will be extracted and included in all log entries
func WithLogField(ctx context.Context, key string, value interface{}) context.Context {
	fields := GetLogFields(ctx)
	if fields == nil {
		fields = make(LogFields)
	}
	fields[key] = value
	return context.WithValue(ctx, LogFieldsKey, fields)
}

// WithLogFields adds multiple dynamic log fields to the context
// These fields will be extracted and included in all log entries
func WithLogFields(ctx context.Context, newFields LogFields) context.Context {
	fields := GetLogFields(ctx)
	if fields == nil {
		fields = make(LogFields)
	}
	for k, v := range newFields {
		fields[k] = v
	}
	return context.WithValue(ctx, LogFieldsKey, fields)
}

// WithDynamicResourceID adds a resource ID as a dynamic log field
// The field name is derived from the resource type (e.g., "Cluster" -> "cluster_id", "NodePool" -> "nodepool_id")
func WithDynamicResourceID(ctx context.Context, resourceType string, resourceID string) context.Context {
	fieldName := strings.ToLower(resourceType) + "_id"
	return WithLogField(ctx, fieldName, resourceID)
}

// WithTraceID returns a context with the trace ID set
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return WithLogField(ctx, string(TraceIDKey), traceID)
}

// WithSpanID returns a context with the span ID set
func WithSpanID(ctx context.Context, spanID string) context.Context {
	return WithLogField(ctx, string(SpanIDKey), spanID)
}

// WithEventID returns a context with the event ID set
func WithEventID(ctx context.Context, eventID string) context.Context {
	return WithLogField(ctx, string(EventIDKey), eventID)
}

// WithClusterID returns a context with the cluster ID set
func WithClusterID(ctx context.Context, clusterID string) context.Context {
	return WithLogField(ctx, string(ClusterIDKey), clusterID)
}

// WithResourceType returns a copy of the context that includes the provided resource type under the ResourceTypeKey log field.
func WithResourceType(ctx context.Context, resourceType string) context.Context {
	return WithLogField(ctx, string(ResourceTypeKey), resourceType)
}

// WithResourceName returns a context with the resource name set
func WithResourceName(ctx context.Context, resourceName string) context.Context {
	return WithLogField(ctx, string(ResourceNameKey), resourceName)
}

// WithResourceResult returns a context with the resource operation result set to the provided value (for example, "SUCCESS" or "FAILED").
func WithResourceResult(ctx context.Context, result string) context.Context {
	return WithLogField(ctx, string(ResourceResultKey), result)
}

// WithAdapter returns a context with the adapter name set
func WithAdapter(ctx context.Context, adapter string) context.Context {
	return WithLogField(ctx, string(AdapterKey), adapter)
}

// WithObservedGeneration returns a context with the observed generation set
func WithObservedGeneration(ctx context.Context, generation int64) context.Context {
	return WithLogField(ctx, string(ObservedGenerationKey), generation)
}

// WithSubscription adds the subscription name to the context under SubscriptionKey.
// It returns a new context containing that subscription value.
func WithSubscription(ctx context.Context, subscription string) context.Context {
	return WithLogField(ctx, string(SubscriptionKey), subscription)
}

// WithErrorField returns a context with the error message set.
// WithErrorField adds the error message from err to the context under the error key.
// If err is nil, it returns the original context unchanged.
func WithErrorField(ctx context.Context, err error) context.Context {
	if err == nil {
		return ctx
	}
	return WithLogField(ctx, string(ErrorKey), err.Error())
}

// WithStackTraceField returns a context with the stack trace set.
// WithStackTraceField adds the given stack trace frames to the context's log fields under the stack trace key.
// If frames is nil or empty, the original context is returned unchanged.
func WithStackTraceField(ctx context.Context, frames []string) context.Context {
	if len(frames) == 0 {
		return ctx
	}
	return WithLogField(ctx, string(StackTraceKey), frames)
}

// CaptureStackTrace captures the current call stack and returns it as a slice of strings.
// Each string contains the file path, line number, and function name.
// The skip parameter specifies how many stack frames to skip:
//   - skip=0 starts from the caller of CaptureStackTrace
// CaptureStackTrace captures the current goroutine's stack frames and returns them as formatted strings.
// The skip parameter omits that many additional caller frames from the result (skip=0 omits the runtime callers and CaptureStackTrace itself; skip=1 omits one additional frame, etc.).
// Each returned entry is formatted as "file:line function". The function returns nil if no frames are captured.
func CaptureStackTrace(skip int) []string {
	const maxFrames = 32
	pcs := make([]uintptr, maxFrames)
	// +2 to skip runtime.Callers and CaptureStackTrace itself
	n := runtime.Callers(skip+2, pcs)
	if n == 0 {
		return nil
	}

	frames := runtime.CallersFrames(pcs[:n])
	var stack []string
	for {
		frame, more := frames.Next()
		stack = append(stack, fmt.Sprintf("%s:%d %s", frame.File, frame.Line, frame.Function))
		if !more {
			break
		}
	}
	return stack
}

// WithOTelTraceID extracts only trace_id from OpenTelemetry span context.
// Use this at the event/request entry point where you only need trace correlation.
// If no active span exists, returns the context unchanged.
//
// Example usage:
//
//	ctx = logger.WithOTelTraceID(ctx)
//	log.Info(ctx, "Processing event")
//
// This will produce logs with trace_id field only:
//
// WithOTelTraceID adds the OpenTelemetry span's trace ID to the context's log fields when present.
// If the current context contains a valid span with a trace ID, the `trace_id` log field is set;
// otherwise the original context is returned unchanged.
func WithOTelTraceID(ctx context.Context) context.Context {
	spanCtx := trace.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		return ctx
	}

	// Add trace_id if valid
	if spanCtx.HasTraceID() {
		ctx = WithLogField(ctx, string(TraceIDKey), spanCtx.TraceID().String())
	}

	return ctx
}

// WithOTelTraceContext extracts OpenTelemetry trace context (trace_id, span_id)
// from the context and adds them as log fields for distributed tracing correlation.
// Use this for HTTP requests where span_id helps identify specific operations.
// If no active span exists, returns the context unchanged.
//
// Example usage:
//
//	ctx = logger.WithOTelTraceContext(ctx)
//	log.Info(ctx, "Making HTTP request")
//
// This will produce logs with trace_id and span_id fields:
//
// WithOTelTraceContext adds OpenTelemetry trace and span identifiers from ctx's current span to the context's log fields.
// If the span contains a trace ID and/or span ID those values are set under TraceIDKey and SpanIDKey; if no valid span context exists the original context is returned unchanged.
func WithOTelTraceContext(ctx context.Context) context.Context {
	spanCtx := trace.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		return ctx
	}

	// Add trace_id if valid
	if spanCtx.HasTraceID() {
		ctx = WithLogField(ctx, string(TraceIDKey), spanCtx.TraceID().String())
	}

	// Add span_id if valid
	if spanCtx.HasSpanID() {
		ctx = WithLogField(ctx, string(SpanIDKey), spanCtx.SpanID().String())
	}

	return ctx
}

// -----------------------------------------------------------------------------
// Context Getters
// -----------------------------------------------------------------------------

// GetLogFields returns the dynamic log fields from the context, or nil if not set
func GetLogFields(ctx context.Context) LogFields {
	if ctx == nil {
		return nil
	}
	if v, ok := ctx.Value(LogFieldsKey).(LogFields); ok {
		// Return a copy to avoid mutation
		fields := make(LogFields, len(v))
		for k, val := range v {
			fields[k] = val
		}
		return fields
	}
	return nil
}