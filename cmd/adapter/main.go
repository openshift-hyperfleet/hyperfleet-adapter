package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/config_loader"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/executor"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/hyperfleet_api"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/k8s_client"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-broker/broker"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Build-time variables set via ldflags
var (
	version   = "0.1.0"
	commit    = "none"
	buildDate = "unknown"
	tag       = "none"
)

// Command-line flags
var (
	configPath string
	logLevel   string
	logFormat  string
	logOutput  string
)

// Environment variable names
const (
	EnvBrokerSubscriptionID = "BROKER_SUBSCRIPTION_ID"
	EnvBrokerTopic          = "BROKER_TOPIC"
)

func main() {
	// Root command
	rootCmd := &cobra.Command{
		Use:   "adapter",
		Short: "HyperFleet Adapter - event-driven Kubernetes resource manager",
		Long: `HyperFleet Adapter listens for events from a message broker and 
executes configured actions including Kubernetes resource management 
and HyperFleet API calls.`,
		// Disable default completion command
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}

	// Add flags to root command (so they work on all subcommands)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	// Serve command
	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the adapter and begin processing events",
		Long: `Start the HyperFleet adapter in serve mode. The adapter will:
- Connect to the configured message broker
- Subscribe to the specified topic
- Process incoming events according to the adapter configuration
- Execute Kubernetes operations and HyperFleet API calls`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe()
		},
	}

	// Add --config flag to serve command
	serveCmd.Flags().StringVarP(&configPath, "config", "c", "",
		fmt.Sprintf("Path to adapter configuration file (can also use %s env var)", config_loader.EnvConfigPath))

	// Add logging flags to serve command
	serveCmd.Flags().StringVar(&logLevel, "log-level", "",
		"Log level (debug, info, warn, error). Env: LOG_LEVEL")
	serveCmd.Flags().StringVar(&logFormat, "log-format", "",
		"Log format (text, json). Env: LOG_FORMAT")
	serveCmd.Flags().StringVar(&logOutput, "log-output", "",
		"Log output (stdout, stderr). Env: LOG_OUTPUT")

	// Version command
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("HyperFleet Adapter\n")
			fmt.Printf("  Version:    %s\n", version)
			fmt.Printf("  Commit:     %s\n", commit)
			fmt.Printf("  Built:      %s\n", buildDate)
			fmt.Printf("  Tag:        %s\n", tag)
		},
	}

	// Add subcommands
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(versionCmd)

	// Execute
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// buildLoggerConfig creates a logger configuration from environment variables
// buildLoggerConfig builds a logger.Config by loading defaults from the environment,
// applying any command-line flag overrides (flags take precedence), and setting the
// provided component name and the global version.
func buildLoggerConfig(component string) logger.Config {
	cfg := logger.ConfigFromEnv()

	// Override with command-line flags if provided
	if logLevel != "" {
		cfg.Level = logLevel
	}
	if logFormat != "" {
		cfg.Format = logFormat
	}
	if logOutput != "" {
		cfg.Output = logOutput
	}

	cfg.Component = component
	cfg.Version = version

	return cfg
}

// initTracer initializes OpenTelemetry TracerProvider for generating trace_id and span_id.
// These IDs are used for:
// 1. Log correlation (via logger.WithOTelTraceContext)
// initTracer creates and registers an OpenTelemetry TracerProvider configured with the
// provided service name and version. It sets the global TracerProvider and installs the
// W3C Trace Context text-map propagator for HTTP request propagation.
// Returns the created TracerProvider, or an error if the required resource cannot be built.
func initTracer(serviceName, serviceVersion string) (*sdktrace.TracerProvider, error) {
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp, nil
}

// runServe executes the main application lifecycle for the serve subcommand, including logger and configuration setup, OpenTelemetry initialization, HyperFleet API and Kubernetes client creation, executor and broker subscription setup, and graceful shutdown handling.
// It returns an error if initialization fails or if an unrecoverable runtime condition occurs.
func runServe() error {
	// Create context that cancels on system signals
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create bootstrap logger (before config is loaded)
	log, err := logger.NewLogger(buildLoggerConfig("hyperfleet-adapter"))
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}

	log.Infof(ctx, "Starting Hyperfleet Adapter version=%s commit=%s built=%s tag=%s", version, commit, buildDate, tag)

	// Load adapter configuration
	// If configPath flag is empty, config_loader.Load will read from ADAPTER_CONFIG_PATH env var
	log.Info(ctx, "Loading adapter configuration...")
	adapterConfig, err := config_loader.Load(configPath, config_loader.WithAdapterVersion(version))
	if err != nil {
		errCtx := logger.WithStackTraceField(logger.WithErrorField(ctx, err), logger.CaptureStackTrace(1))
		log.Errorf(errCtx, "Failed to load adapter configuration")
		return fmt.Errorf("failed to load adapter configuration: %w", err)
	}

	// Recreate logger with component name from adapter config
	log, err = logger.NewLogger(buildLoggerConfig(adapterConfig.Metadata.Name))
	if err != nil {
		return fmt.Errorf("failed to create logger with adapter config: %w", err)
	}

	log.Infof(ctx, "Adapter configuration loaded successfully: name=%s namespace=%s",
		adapterConfig.Metadata.Name, adapterConfig.Metadata.Namespace)
	log.Infof(ctx, "HyperFleet API client configured: timeout=%s retryAttempts=%d",
		adapterConfig.Spec.HyperfleetAPI.Timeout,
		adapterConfig.Spec.HyperfleetAPI.RetryAttempts)

	// Initialize OpenTelemetry for trace_id/span_id generation and HTTP propagation
	tp, err := initTracer(adapterConfig.Metadata.Name, version)
	if err != nil {
		errCtx := logger.WithStackTraceField(logger.WithErrorField(ctx, err), logger.CaptureStackTrace(1))
		log.Errorf(errCtx, "Failed to initialize OpenTelemetry")
		return fmt.Errorf("failed to initialize OpenTelemetry: %w", err)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = tp.Shutdown(shutdownCtx)
	}()

	// Create HyperFleet API client from config
	log.Info(ctx, "Creating HyperFleet API client...")
	apiClient, err := createAPIClient(adapterConfig.Spec.HyperfleetAPI, log)
	if err != nil {
		errCtx := logger.WithStackTraceField(logger.WithErrorField(ctx, err), logger.CaptureStackTrace(1))
		log.Errorf(errCtx, "Failed to create HyperFleet API client")
		return fmt.Errorf("failed to create HyperFleet API client: %w", err)
	}

	// Create Kubernetes client
	// Uses KUBECONFIG env var if set, otherwise uses in-cluster config
	log.Info(ctx, "Creating Kubernetes client...")
	k8sClient, err := k8s_client.NewClient(ctx, k8s_client.ClientConfig{}, log)
	if err != nil {
		errCtx := logger.WithStackTraceField(logger.WithErrorField(ctx, err), logger.CaptureStackTrace(1))
		log.Errorf(errCtx, "Failed to create Kubernetes client")
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Create the executor using the builder pattern
	log.Info(ctx, "Creating event executor...")
	exec, err := executor.NewBuilder().
		WithAdapterConfig(adapterConfig).
		WithAPIClient(apiClient).
		WithK8sClient(k8sClient).
		WithLogger(log).
		Build()
	if err != nil {
		errCtx := logger.WithStackTraceField(logger.WithErrorField(ctx, err), logger.CaptureStackTrace(1))
		log.Errorf(errCtx, "Failed to create executor")
		return fmt.Errorf("failed to create executor: %w", err)
	}

	// Create the event handler from the executor
	// This handler will:
	// 1. Extract params from event data
	// 2. Execute preconditions (API calls, condition checks)
	// 3. Create/update Kubernetes resources
	// 4. Execute post actions (status reporting)
	handler := exec.CreateHandler()

	// Handle signals for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Infof(ctx, "Received signal %s, initiating graceful shutdown...", sig)
		cancel()

		// Second signal forces immediate exit
		sig = <-sigCh
		log.Infof(ctx, "Received second signal %s, forcing immediate exit", sig)
		os.Exit(1)
	}()

	// Get subscription ID from environment
	subscriptionID := os.Getenv(EnvBrokerSubscriptionID)
	if subscriptionID == "" {
		err := fmt.Errorf("%s environment variable is required", EnvBrokerSubscriptionID)
		errCtx := logger.WithStackTraceField(logger.WithErrorField(ctx, err), logger.CaptureStackTrace(1))
		log.Errorf(errCtx, "Missing required environment variable")
		return err
	}

	// Get topic from environment
	topic := os.Getenv(EnvBrokerTopic)
	if topic == "" {
		err := fmt.Errorf("%s environment variable is required", EnvBrokerTopic)
		errCtx := logger.WithStackTraceField(logger.WithErrorField(ctx, err), logger.CaptureStackTrace(1))
		log.Errorf(errCtx, "Missing required environment variable")
		return err
	}

	// Create broker subscriber
	// Configuration is loaded from environment variables by the broker library:
	//   - BROKER_TYPE: "rabbitmq" or "googlepubsub"
	//   - BROKER_GOOGLEPUBSUB_PROJECT_ID: GCP project ID (for googlepubsub)
	//   - BROKER_RABBITMQ_URL: RabbitMQ URL (for rabbitmq)
	//   - SUBSCRIBER_PARALLELISM: number of parallel workers (default: 1)
	log.Info(ctx, "Creating broker subscriber...")
	subscriber, err := broker.NewSubscriber(log, subscriptionID)
	if err != nil {
		errCtx := logger.WithStackTraceField(logger.WithErrorField(ctx, err), logger.CaptureStackTrace(1))
		log.Errorf(errCtx, "Failed to create subscriber")
		return fmt.Errorf("failed to create subscriber: %w", err)
	}
	log.Info(ctx, "Broker subscriber created successfully")

	// Subscribe to topic - this is NON-BLOCKING, it returns immediately after setup
	log.Info(ctx, "Subscribing to broker topic...")
	err = subscriber.Subscribe(ctx, topic, handler)
	if err != nil {
		errCtx := logger.WithStackTraceField(logger.WithErrorField(ctx, err), logger.CaptureStackTrace(1))
		log.Errorf(errCtx, "Failed to subscribe to topic")
		return fmt.Errorf("failed to subscribe to topic: %w", err)
	}
	log.Info(ctx, "Successfully subscribed to broker topic")

	// Channel to signal fatal errors from the errors goroutine
	fatalErrCh := make(chan error, 1)

	// Monitor subscription errors channel in a separate goroutine
	go func() {
		for subErr := range subscriber.Errors() {
			errCtx := logger.WithStackTraceField(logger.WithErrorField(ctx, subErr), logger.CaptureStackTrace(1))
			log.Errorf(errCtx, "Subscription error")
			// For critical errors, signal shutdown
			select {
			case fatalErrCh <- subErr:
				// Signal sent, trigger shutdown
			default:
				// Channel already has an error, don't block
			}
		}
	}()

	log.Info(ctx, "Adapter started, waiting for events...")

	// Wait for shutdown signal or fatal subscription error
	select {
	case <-ctx.Done():
		log.Info(ctx, "Context cancelled, shutting down...")
	case err := <-fatalErrCh:
		errCtx := logger.WithStackTraceField(logger.WithErrorField(ctx, err), logger.CaptureStackTrace(1))
		log.Errorf(errCtx, "Fatal subscription error, shutting down")
		cancel() // Cancel context to trigger graceful shutdown
	}

	// Close subscriber gracefully with timeout
	log.Info(ctx, "Closing broker subscriber...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Close subscriber in a goroutine with timeout
	closeDone := make(chan error, 1)
	go func() {
		closeDone <- subscriber.Close()
	}()

	select {
	case err := <-closeDone:
		if err != nil {
			errCtx := logger.WithStackTraceField(logger.WithErrorField(ctx, err), logger.CaptureStackTrace(1))
			log.Errorf(errCtx, "Error closing subscriber")
		} else {
			log.Info(ctx, "Subscriber closed successfully")
		}
	case <-shutdownCtx.Done():
		err := fmt.Errorf("subscriber close timed out after 30 seconds")
		errCtx := logger.WithStackTraceField(logger.WithErrorField(ctx, err), logger.CaptureStackTrace(1))
		log.Error(errCtx, "Subscriber close timed out")
	}

	log.Info(ctx, "Adapter shutdown complete")

	return nil
}

// createAPIClient creates a HyperFleet API client from the config
func createAPIClient(apiConfig config_loader.HyperfleetAPIConfig, log logger.Logger) (hyperfleet_api.Client, error) {
	var opts []hyperfleet_api.ClientOption

	// Parse and set timeout using the accessor method
	timeout, err := apiConfig.ParseTimeout()
	if err != nil {
		return nil, fmt.Errorf("invalid timeout %q: %w", apiConfig.Timeout, err)
	}
	if timeout > 0 {
		opts = append(opts, hyperfleet_api.WithTimeout(timeout))
	}

	// Set retry attempts
	if apiConfig.RetryAttempts > 0 {
		opts = append(opts, hyperfleet_api.WithRetryAttempts(apiConfig.RetryAttempts))
	}

	// Parse and set retry backoff strategy
	if apiConfig.RetryBackoff != "" {
		backoff := hyperfleet_api.BackoffStrategy(apiConfig.RetryBackoff)
		switch backoff {
		case hyperfleet_api.BackoffExponential, hyperfleet_api.BackoffLinear, hyperfleet_api.BackoffConstant:
			opts = append(opts, hyperfleet_api.WithRetryBackoff(backoff))
		default:
			return nil, fmt.Errorf("invalid retry backoff strategy %q (supported: exponential, linear, constant)", apiConfig.RetryBackoff)
		}
	}

	return hyperfleet_api.NewClient(log, opts...)
}