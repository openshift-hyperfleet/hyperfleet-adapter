package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestMetricsServer(t *testing.T) *MetricsServer {
	t.Helper()
	return NewMetricsServer(&mockLogger{}, "0", MetricsConfig{
		Component: "test-adapter",
		Version:   "v0.0.1-test",
		Commit:    "abc123",
	})
}

func getGaugeValue(t *testing.T, g prometheus.Gauge) float64 {
	t.Helper()
	ch := make(chan prometheus.Metric, 1)
	g.Collect(ch)
	m := <-ch
	metric := &dto.Metric{}
	require.NoError(t, m.Write(metric))
	return metric.GetGauge().GetValue()
}

func TestMetricsServer_RecordMessageProcessed_UpdatesTimestamp(t *testing.T) {
	ms := newTestMetricsServer(t)

	before := float64(time.Now().Unix())
	ms.RecordMessageProcessed()
	after := float64(time.Now().Unix())

	val := getGaugeValue(t, ms.lastProcessedGauge)
	assert.GreaterOrEqual(t, val, before, "timestamp should be >= time before call")
	assert.LessOrEqual(t, val, after+1, "timestamp should be <= time after call")
}

func TestMetricsServer_RecordMessageProcessed_AdvancesTimestamp(t *testing.T) {
	ms := newTestMetricsServer(t)

	ms.RecordMessageProcessed()
	first := getGaugeValue(t, ms.lastProcessedGauge)

	time.Sleep(10 * time.Millisecond)

	ms.RecordMessageProcessed()
	second := getGaugeValue(t, ms.lastProcessedGauge)

	assert.GreaterOrEqual(t, second, first, "second call should produce >= timestamp")
}

func TestMetricsServer_LastProcessedGauge_ZeroBeforeFirstCall(t *testing.T) {
	ms := newTestMetricsServer(t)
	val := getGaugeValue(t, ms.lastProcessedGauge)
	assert.Equal(t, float64(0), val, "gauge should be 0 before any message is processed")
}

func TestMetricsServer_RecordMessageSuccess_UpdatesTimestamp(t *testing.T) {
	ms := newTestMetricsServer(t)

	before := float64(time.Now().Unix())
	ms.RecordMessageSuccess()
	after := float64(time.Now().Unix())

	val := getGaugeValue(t, ms.lastSuccessGauge)
	assert.GreaterOrEqual(t, val, before)
	assert.LessOrEqual(t, val, after+1)
}

func TestMetricsServer_RecordMessageFailure_UpdatesTimestamp(t *testing.T) {
	ms := newTestMetricsServer(t)

	before := float64(time.Now().Unix())
	ms.RecordMessageFailure()
	after := float64(time.Now().Unix())

	val := getGaugeValue(t, ms.lastFailureGauge)
	assert.GreaterOrEqual(t, val, before)
	assert.LessOrEqual(t, val, after+1)
}

func TestMetricsServer_SuccessAndFailure_Independent(t *testing.T) {
	ms := newTestMetricsServer(t)

	ms.RecordMessageSuccess()
	successVal := getGaugeValue(t, ms.lastSuccessGauge)
	failureVal := getGaugeValue(t, ms.lastFailureGauge)

	assert.Greater(t, successVal, float64(0), "success gauge should be updated")
	assert.Equal(t, float64(0), failureVal, "failure gauge should remain 0")

	ms.RecordMessageFailure()
	failureVal = getGaugeValue(t, ms.lastFailureGauge)
	assert.Greater(t, failureVal, float64(0), "failure gauge should now be updated")
}

func TestMetricsServer_MetricsEndpoint_ExposesAllMetrics(t *testing.T) {
	ms := newTestMetricsServer(t)

	ms.RecordMessageProcessed()
	ms.RecordMessageSuccess()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	ms.server.Handler.ServeHTTP(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body := w.Body.String()
	assert.True(t, strings.Contains(body, "hyperfleet_adapter_up"), "should expose up metric")
	assert.True(t, strings.Contains(body, "hyperfleet_adapter_build_info"), "should expose build_info metric")
	assert.True(t, strings.Contains(body, "hyperfleet_adapter_last_message_processed_timestamp"),
		"should expose last_message_processed_timestamp metric")
	assert.True(t, strings.Contains(body, "hyperfleet_adapter_last_message_success_timestamp"),
		"should expose last_message_success_timestamp metric")
	assert.True(t, strings.Contains(body, `component="test-adapter"`),
		"metric should include component label")
}

func TestMetricsServer_MetricsEndpoint_ExposesDefaultCollectors(t *testing.T) {
	ms := newTestMetricsServer(t)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	ms.server.Handler.ServeHTTP(w, req)

	body := w.Body.String()
	assert.True(t, strings.Contains(body, "go_goroutines"), "should expose Go runtime metrics")
	assert.True(t, strings.Contains(body, "process_cpu_seconds_total"), "should expose process metrics")
}

func TestMetricsServer_Shutdown_SetsUpToZero(t *testing.T) {
	ms := newTestMetricsServer(t)

	val := getGaugeValue(t, ms.upGauge)
	assert.Equal(t, float64(1), val, "up gauge should be 1 before shutdown")

	err := ms.Shutdown(context.Background())
	require.NoError(t, err)

	val = getGaugeValue(t, ms.upGauge)
	assert.Equal(t, float64(0), val, "up gauge should be 0 after shutdown")
}

func TestMetricsServer_Lifecycle(t *testing.T) {
	port := "19090"
	ms := NewMetricsServer(&mockLogger{}, port, MetricsConfig{
		Component: "lifecycle-test",
		Version:   "v0.0.1",
		Commit:    "def456",
	})

	ctx := context.Background()
	err := ms.Start(ctx)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	ms.RecordMessageProcessed()
	ms.RecordMessageSuccess()

	resp, err := http.Get("http://localhost:" + port + "/metrics")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = ms.Shutdown(shutdownCtx)
	require.NoError(t, err)
}
