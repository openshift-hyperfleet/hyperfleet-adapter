package main

import (
	"context"
	"fmt"
	"os"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/dev"
	"github.com/spf13/cobra"
)

var (
	runConfigPath       string
	runEventPath        string
	runMockAPIResponses string
	runEnvFile          string
	runVerbose          bool
	runOutput           string
	runShowManifests    bool
	runShowPayloads     bool
	runShowParams       bool
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Dry-run an event through the adapter",
	Long: `Process an event through the adapter in dry-run mode.

This command executes the full adapter pipeline without:
  - Connecting to a real message broker
  - Making actual Kubernetes API calls
  - Making actual HyperFleet API calls

It shows what WOULD happen if the event were processed.

Examples:
  # Basic dry-run
  adapter dev run --config ./adapter-config.yaml --event ./test-event.json

  # With mock API responses
  adapter dev run --config ./adapter-config.yaml --event ./test-event.json \
    --mock-api-responses ./mock-responses.yaml

  # Show rendered manifests
  adapter dev run --config ./adapter-config.yaml --event ./test-event.json \
    --show-manifests --verbose

  # Load environment from .env file
  adapter dev run --config ./adapter-config.yaml --event ./test-event.json \
    --env-file .env.local`,
	RunE: runDryRun,
}

func init() {
	runCmd.Flags().StringVarP(&runConfigPath, "config", "c", "",
		"Path to adapter configuration file (required)")
	runCmd.Flags().StringVarP(&runEventPath, "event", "e", "",
		"Path to event file in JSON or YAML format (required)")
	runCmd.Flags().StringVar(&runMockAPIResponses, "mock-api-responses", "",
		"Path to YAML file with mock API responses")
	runCmd.Flags().StringVar(&runEnvFile, "env-file", "",
		"Path to .env file for environment variables")
	runCmd.Flags().BoolVarP(&runVerbose, "verbose", "v", false,
		"Show detailed execution trace")
	runCmd.Flags().StringVarP(&runOutput, "output", "o", "text",
		"Output format: text or json")
	runCmd.Flags().BoolVar(&runShowManifests, "show-manifests", false,
		"Display rendered Kubernetes manifests")
	runCmd.Flags().BoolVar(&runShowPayloads, "show-payloads", false,
		"Display built payloads")
	runCmd.Flags().BoolVar(&runShowParams, "show-params", false,
		"Display extracted parameters")

	_ = runCmd.MarkFlagRequired("config")
	_ = runCmd.MarkFlagRequired("event")

	devCmd.AddCommand(runCmd)
}

func runDryRun(cmd *cobra.Command, args []string) error {
	// Load environment variables from .env file if provided
	if runEnvFile != "" {
		envVars, err := dev.LoadEnvFile(runEnvFile)
		if err != nil {
			return fmt.Errorf("failed to load env file: %w", err)
		}
		if err := dev.ApplyEnvVars(envVars); err != nil {
			return fmt.Errorf("failed to apply env vars: %w", err)
		}
	}

	// Load the event
	eventSource := dev.NewFileEventSource(runEventPath)
	eventData, err := eventSource.LoadEvent()
	if err != nil {
		return fmt.Errorf("failed to load event: %w", err)
	}

	// Configure run options
	opts := &dev.RunOptions{
		MockAPIResponsesPath: runMockAPIResponses,
		ShowManifests:        runShowManifests,
		ShowPayloads:         runShowPayloads,
		ShowParams:           runShowParams,
		EnvFilePath:          runEnvFile,
	}

	// Execute with trace
	ctx := context.Background()
	result, err := dev.ExecuteWithTrace(ctx, runConfigPath, eventData, opts)
	if err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	// Determine output format
	format := dev.OutputFormatText
	if runOutput == "json" {
		format = dev.OutputFormatJSON
	}

	// Enable verbose if show flags are set
	verbose := runVerbose || runShowManifests || runShowPayloads || runShowParams

	writer := dev.NewOutputWriter(os.Stdout, format, verbose)

	// Write the results
	if err := writer.WriteTraceResult(result); err != nil {
		return fmt.Errorf("failed to write results: %w", err)
	}

	// Exit with error code if execution failed
	if !result.Success {
		os.Exit(1)
	}

	return nil
}
