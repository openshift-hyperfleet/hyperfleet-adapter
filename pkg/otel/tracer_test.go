package otel

import (
	"context"
	"testing"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() logger.Logger {
	log, _ := logger.NewLogger(logger.Config{Level: "error", Output: "stdout", Format: "json"})
	return log
}

func TestGetTraceSampleRatio(t *testing.T) {
	log := testLogger()
	ctx := context.Background()

	t.Run("default when not set", func(t *testing.T) {
		t.Setenv(EnvTraceSampleRatio, "")
		ratio := GetTraceSampleRatio(log, ctx)
		assert.Equal(t, DefaultTraceSampleRatio, ratio)
	})

	t.Run("valid ratio", func(t *testing.T) {
		t.Setenv(EnvTraceSampleRatio, "0.5")
		ratio := GetTraceSampleRatio(log, ctx)
		assert.Equal(t, 0.5, ratio)
	})

	t.Run("invalid string", func(t *testing.T) {
		t.Setenv(EnvTraceSampleRatio, "notanumber")
		ratio := GetTraceSampleRatio(log, ctx)
		assert.Equal(t, DefaultTraceSampleRatio, ratio)
	})

	t.Run("out of range positive", func(t *testing.T) {
		t.Setenv(EnvTraceSampleRatio, "2.0")
		ratio := GetTraceSampleRatio(log, ctx)
		assert.Equal(t, DefaultTraceSampleRatio, ratio)
	})

	t.Run("out of range negative", func(t *testing.T) {
		t.Setenv(EnvTraceSampleRatio, "-0.5")
		ratio := GetTraceSampleRatio(log, ctx)
		assert.Equal(t, DefaultTraceSampleRatio, ratio)
	})

	t.Run("zero is valid", func(t *testing.T) {
		t.Setenv(EnvTraceSampleRatio, "0.0")
		ratio := GetTraceSampleRatio(log, ctx)
		assert.Equal(t, 0.0, ratio)
	})

	t.Run("one is valid", func(t *testing.T) {
		t.Setenv(EnvTraceSampleRatio, "1.0")
		ratio := GetTraceSampleRatio(log, ctx)
		assert.Equal(t, 1.0, ratio)
	})
}

func TestCreateExporter(t *testing.T) {
	log := testLogger()
	ctx := context.Background()

	// clearOtelEnv ensures all 4 OTel env vars are cleared to prevent
	// interference from the local shell environment.
	clearOtelEnv := func(t *testing.T) {
		t.Setenv(envOtelExporterOtlpEndpoint, "")
		t.Setenv(envOtelExporterOtlpTracesEndpoint, "")
		t.Setenv(envOtelExporterOtlpProtocol, "")
		t.Setenv(envOtelExporterOtlpTracesProtocol, "")
	}

	t.Run("nil exporter when no endpoint set", func(t *testing.T) {
		clearOtelEnv(t)
		exporter, err := createExporter(ctx, log)
		require.NoError(t, err)
		assert.Nil(t, exporter)
	})

	t.Run("http exporter when endpoint set with default protocol", func(t *testing.T) {
		clearOtelEnv(t)
		t.Setenv(envOtelExporterOtlpEndpoint, "http://localhost:4318")
		exporter, err := createExporter(ctx, log)
		require.NoError(t, err)
		assert.NotNil(t, exporter)
		assert.NoError(t, exporter.Shutdown(ctx))
	})

	t.Run("grpc exporter when protocol is grpc", func(t *testing.T) {
		clearOtelEnv(t)
		t.Setenv(envOtelExporterOtlpEndpoint, "localhost:4317")
		t.Setenv(envOtelExporterOtlpProtocol, "grpc")
		exporter, err := createExporter(ctx, log)
		require.NoError(t, err)
		assert.NotNil(t, exporter)
		assert.NoError(t, exporter.Shutdown(ctx))
	})

	t.Run("falls back to http/protobuf for unrecognized protocol", func(t *testing.T) {
		clearOtelEnv(t)
		t.Setenv(envOtelExporterOtlpEndpoint, "http://localhost:4318")
		t.Setenv(envOtelExporterOtlpProtocol, "unknown-protocol")
		exporter, err := createExporter(ctx, log)
		require.NoError(t, err)
		assert.NotNil(t, exporter)
		assert.NoError(t, exporter.Shutdown(ctx))
	})

	t.Run("traces-specific endpoint takes precedence", func(t *testing.T) {
		clearOtelEnv(t)
		t.Setenv(envOtelExporterOtlpTracesEndpoint, "http://localhost:4318")
		exporter, err := createExporter(ctx, log)
		require.NoError(t, err)
		assert.NotNil(t, exporter)
		assert.NoError(t, exporter.Shutdown(ctx))
	})

	t.Run("traces-specific protocol takes precedence", func(t *testing.T) {
		clearOtelEnv(t)
		t.Setenv(envOtelExporterOtlpEndpoint, "http://localhost:4318")
		t.Setenv(envOtelExporterOtlpProtocol, "grpc")
		t.Setenv(envOtelExporterOtlpTracesProtocol, "http/protobuf")
		exporter, err := createExporter(ctx, log)
		require.NoError(t, err)
		assert.NotNil(t, exporter)
		assert.NoError(t, exporter.Shutdown(ctx))
	})

	t.Run("nil when neither endpoint is set", func(t *testing.T) {
		clearOtelEnv(t)
		exporter, err := createExporter(ctx, log)
		require.NoError(t, err)
		assert.Nil(t, exporter)
	})
}

func TestInitTracer(t *testing.T) {
	log := testLogger()

	clearOtelEnv := func(t *testing.T) {
		t.Setenv(envOtelExporterOtlpEndpoint, "")
		t.Setenv(envOtelExporterOtlpTracesEndpoint, "")
		t.Setenv(envOtelExporterOtlpProtocol, "")
		t.Setenv(envOtelExporterOtlpTracesProtocol, "")
	}

	t.Run("initializes without exporter when no endpoint", func(t *testing.T) {
		clearOtelEnv(t)
		tp, err := InitTracer(log, "test-service", "0.0.1", 1.0)
		require.NoError(t, err)
		require.NotNil(t, tp)
		assert.NoError(t, tp.Shutdown(context.Background()))
	})

	t.Run("initializes with exporter when endpoint is set", func(t *testing.T) {
		clearOtelEnv(t)
		t.Setenv(envOtelExporterOtlpEndpoint, "http://localhost:4318")
		tp, err := InitTracer(log, "test-service", "0.0.1", 1.0)
		require.NoError(t, err)
		require.NotNil(t, tp)
		assert.NoError(t, tp.Shutdown(context.Background()))
	})
}
