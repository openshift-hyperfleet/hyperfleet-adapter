package health

import (
	"context"
	"net/http"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsServer provides HTTP metrics endpoint for Prometheus.
type MetricsServer struct {
	server             *http.Server
	log                logger.Logger
	port               string
	upGauge            prometheus.Gauge
	buildInfo          *prometheus.GaugeVec
	lastProcessedGauge prometheus.Gauge
	lastSuccessGauge   prometheus.Gauge
	lastFailureGauge   prometheus.Gauge
}

// MetricsConfig holds configuration for metrics registration.
type MetricsConfig struct {
	Component string
	Version   string
	Commit    string
}

// NewMetricsServer creates a new metrics server with required HyperFleet metrics.
// Each server uses its own Prometheus registry to avoid conflicts.
func NewMetricsServer(log logger.Logger, port string, cfg MetricsConfig) *MetricsServer {
	registry := prometheus.NewRegistry()
	registry.MustRegister(collectors.NewGoCollector())
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	// Create build_info metric per HyperFleet metrics standard
	buildInfo := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "hyperfleet_adapter_build_info",
			Help: "Build information for the adapter",
		},
		[]string{"component", "version", "commit"},
	)

	// Create up metric per HyperFleet metrics standard
	upGauge := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "hyperfleet_adapter_up",
			Help: "Whether the adapter is up and running",
			ConstLabels: prometheus.Labels{
				"component": cfg.Component,
				"version":   cfg.Version,
			},
		},
	)

	// Dead man's switch: timestamp of last processed CloudEvent (regardless of outcome)
	lastProcessedGauge := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "hyperfleet_adapter_last_message_processed_timestamp",
			Help: "Unix timestamp of the last processed CloudEvent (dead man's switch)",
			ConstLabels: prometheus.Labels{
				"component": cfg.Component,
			},
		},
	)

	lastSuccessGauge := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "hyperfleet_adapter_last_message_success_timestamp",
			Help: "Unix timestamp of the last CloudEvent that was processed successfully",
			ConstLabels: prometheus.Labels{
				"component": cfg.Component,
			},
		},
	)

	lastFailureGauge := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "hyperfleet_adapter_last_message_failure_timestamp",
			Help: "Unix timestamp of the last CloudEvent that failed processing",
			ConstLabels: prometheus.Labels{
				"component": cfg.Component,
			},
		},
	)

	// Register metrics
	registry.MustRegister(buildInfo)
	registry.MustRegister(upGauge)
	registry.MustRegister(lastProcessedGauge)
	registry.MustRegister(lastSuccessGauge)
	registry.MustRegister(lastFailureGauge)

	// Set build_info to 1 (this is an info metric)
	buildInfo.WithLabelValues(cfg.Component, cfg.Version, cfg.Commit).Set(1)

	// Set up to 1 (adapter is running)
	upGauge.Set(1)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	return &MetricsServer{
		log:                log,
		port:               port,
		upGauge:            upGauge,
		buildInfo:          buildInfo,
		lastProcessedGauge: lastProcessedGauge,
		lastSuccessGauge:   lastSuccessGauge,
		lastFailureGauge:   lastFailureGauge,
		server: &http.Server{
			Addr:              ":" + port,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
}

// Start starts the metrics server in a goroutine.
func (s *MetricsServer) Start(ctx context.Context) error {
	s.log.Infof(ctx, "Starting metrics server on port %s", s.port)

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCtx := logger.WithErrorField(ctx, err)
			s.log.Errorf(errCtx, "Metrics server error")
		}
	}()

	return nil
}

// RecordMessageProcessed updates the dead man's switch metric to the current time.
// Call this after every CloudEvent is processed, regardless of outcome.
func (s *MetricsServer) RecordMessageProcessed() {
	s.lastProcessedGauge.SetToCurrentTime()
}

// RecordMessageSuccess updates the last success timestamp to the current time.
func (s *MetricsServer) RecordMessageSuccess() {
	s.lastSuccessGauge.SetToCurrentTime()
}

// RecordMessageFailure updates the last failure timestamp to the current time.
func (s *MetricsServer) RecordMessageFailure() {
	s.lastFailureGauge.SetToCurrentTime()
}

// Shutdown gracefully shuts down the metrics server.
func (s *MetricsServer) Shutdown(ctx context.Context) error {
	s.log.Info(ctx, "Shutting down metrics server...")
	// Set up to 0 during shutdown
	s.upGauge.Set(0)
	return s.server.Shutdown(ctx)
}
