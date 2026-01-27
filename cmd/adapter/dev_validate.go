package main

import (
	"fmt"
	"os"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/dev"
	"github.com/spf13/cobra"
)

var (
	validateConfigPath string
	validateVerbose    bool
	validateOutput     string
	validateStrict     bool
	validateEnvFile    string
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate adapter configuration file",
	Long: `Validate an adapter configuration file offline.

This command checks:
  - YAML syntax and schema structure
  - Parameter definitions and references
  - CEL expression syntax
  - Go template variable references
  - Kubernetes manifest structure

Examples:
  # Basic validation
  adapter dev validate --config ./adapter-config.yaml

  # Verbose output with details
  adapter dev validate --config ./adapter-config.yaml --verbose

  # JSON output for CI pipelines
  adapter dev validate --config ./adapter-config.yaml --output json

  # Strict mode (treat warnings as errors)
  adapter dev validate --config ./adapter-config.yaml --strict

  # With environment variables from .env file
  adapter dev validate --config ./adapter-config.yaml --env-file .env.local`,
	RunE: runValidate,
}

func init() {
	validateCmd.Flags().StringVarP(&validateConfigPath, "config", "c", "",
		"Path to adapter configuration file (required)")
	validateCmd.Flags().BoolVarP(&validateVerbose, "verbose", "v", false,
		"Show detailed validation results")
	validateCmd.Flags().StringVarP(&validateOutput, "output", "o", "text",
		"Output format: text or json")
	validateCmd.Flags().BoolVar(&validateStrict, "strict", false,
		"Treat warnings as errors")
	validateCmd.Flags().StringVar(&validateEnvFile, "env-file", "",
		"Path to .env file for required environment variables")

	_ = validateCmd.MarkFlagRequired("config")

	devCmd.AddCommand(validateCmd)
}

func runValidate(cmd *cobra.Command, args []string) error {
	// Load environment variables from .env file if provided
	if validateEnvFile != "" {
		envVars, err := dev.LoadEnvFile(validateEnvFile)
		if err != nil {
			return fmt.Errorf("failed to load env file: %w", err)
		}
		if err := dev.ApplyEnvVars(envVars); err != nil {
			return fmt.Errorf("failed to apply env vars: %w", err)
		}
	}

	// Determine output format
	format := dev.OutputFormatText
	if validateOutput == "json" {
		format = dev.OutputFormatJSON
	}

	writer := dev.NewOutputWriter(os.Stdout, format, validateVerbose)

	// Validate the configuration
	result, err := dev.ValidateConfigWithOpts(validateConfigPath, dev.ValidateOptions{
		Strict:  validateStrict,
		Verbose: validateVerbose,
	})
	if err != nil {
		return fmt.Errorf("validation error: %w", err)
	}

	// Write the results
	if err := writer.WriteValidationResult(result); err != nil {
		return fmt.Errorf("failed to write results: %w", err)
	}

	// Exit with error code if validation failed
	if !result.Valid {
		os.Exit(1)
	}

	return nil
}
