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

	t.Run("out of range", func(t *testing.T) {
		t.Setenv(EnvTraceSampleRatio, "2.0")
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

	t.Run("nil exporter when no endpoint set", func(t *testing.T) {
		t.Setenv(envOtelExporterOtlpEndpoint, "")
		exporter, err := createExporter(ctx, log)
		require.NoError(t, err)
		assert.Nil(t, exporter)
	})
}

func TestInitTracer(t *testing.T) {
	log := testLogger()

	t.Run("initializes without exporter when no endpoint", func(t *testing.T) {
		t.Setenv(envOtelExporterOtlpEndpoint, "")
		tp, err := InitTracer(log, "test-service", "0.0.1", 1.0)
		require.NoError(t, err)
		require.NotNil(t, tp)
		assert.NoError(t, tp.Shutdown(context.Background()))
	})
}
