package main

import (
	"github.com/spf13/cobra"
)

// devCmd is the parent command for development tools
var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Development tools for adapter testing and debugging",
	Long: `Development tools for testing and debugging adapters locally.

These commands help adapter developers:
  - Validate configuration files offline
  - Test event processing without a message broker
  - Preview what would happen without connecting to Kubernetes

Examples:
  # Validate an adapter configuration
  adapter dev validate --config ./adapter-config.yaml

  # Dry-run an event through the adapter
  adapter dev run --config ./adapter-config.yaml --event ./test-event.json`,
}

func init() {
	// Subcommands are added in their respective files
}
