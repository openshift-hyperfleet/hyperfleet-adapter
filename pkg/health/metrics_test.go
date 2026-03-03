package health

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openshift-hyperfleet/hyperfleet-broker/broker"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBrokerMetricsExposedOnMetricsEndpoint(t *testing.T) {
	// Use an isolated registry to avoid polluting the global one
	registry := prometheus.NewRegistry()

	// Register adapter baseline metrics (same as NewMetricsServer)
	buildInfo := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "hyperfleet_adapter_build_info",
			Help: "Build information for the adapter",
		},
		[]string{"component", "version", "commit"},
	)
	upGauge := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "hyperfleet_adapter_up",
			Help: "Whether the adapter is up and running",
			ConstLabels: prometheus.Labels{
				"component": "test-adapter",
				"version":   "v0.1.0-test",
			},
		},
	)
	registry.MustRegister(buildInfo)
	registry.MustRegister(upGauge)
	buildInfo.WithLabelValues("test-adapter", "v0.1.0-test", "abc123").Set(1)
	upGauge.Set(1)

	// Register broker metrics with the same registry (same as main.go does via DefaultRegisterer)
	brokerMetrics := broker.NewMetricsRecorder("test-adapter", "v0.1.0-test", registry)

	// Simulate broker activity so Vec metrics emit at least one time series
	brokerMetrics.RecordConsumed("test-topic")
	brokerMetrics.RecordError("test-topic", "handler")
	brokerMetrics.RecordDuration("test-topic", 1)

	// Serve metrics from the shared registry
	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	metricsOutput := string(body)

	// Verify adapter baseline metrics are present
	assert.Contains(t, metricsOutput, "hyperfleet_adapter_build_info")
	assert.Contains(t, metricsOutput, "hyperfleet_adapter_up")

	// Verify broker metrics are registered and exposed
	assert.Contains(t, metricsOutput, "hyperfleet_broker_messages_consumed_total",
		"messages_consumed_total metric should be exposed on /metrics")
	assert.Contains(t, metricsOutput, "hyperfleet_broker_errors_total",
		"errors_total metric should be exposed on /metrics")
	assert.Contains(t, metricsOutput, "hyperfleet_broker_message_duration_seconds",
		"message_duration_seconds metric should be exposed on /metrics")
}
