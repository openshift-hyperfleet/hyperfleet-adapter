package config_loader

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// EnvPrefix is the prefix for all environment variables that override deployment config
const EnvPrefix = "HYPERFLEET"

// viperKeyMappings defines mappings from config paths to env variable suffixes
// The full env var name is EnvPrefix + "_" + suffix
// Note: Uses "::" as key delimiter to avoid conflicts with dots in YAML keys
var viperKeyMappings = map[string]string{
	"spec::debugConfig":                                 "DEBUG_CONFIG",
	"spec::clients::maestro::grpcServerAddress":         "MAESTRO_GRPC_SERVER_ADDRESS",
	"spec::clients::maestro::httpServerAddress":         "MAESTRO_HTTP_SERVER_ADDRESS",
	"spec::clients::maestro::sourceId":                  "MAESTRO_SOURCE_ID",
	"spec::clients::maestro::clientId":                  "MAESTRO_CLIENT_ID",
	"spec::clients::maestro::auth::tlsConfig::caFile":   "MAESTRO_CA_FILE",
	"spec::clients::maestro::auth::tlsConfig::certFile": "MAESTRO_CERT_FILE",
	"spec::clients::maestro::auth::tlsConfig::keyFile":  "MAESTRO_KEY_FILE",
	"spec::clients::maestro::timeout":                   "MAESTRO_TIMEOUT",
	"spec::clients::maestro::retryAttempts":             "MAESTRO_RETRY_ATTEMPTS",
	"spec::clients::maestro::insecure":                  "MAESTRO_INSECURE",
	"spec::clients::hyperfleetApi::baseUrl":             "API_BASE_URL",
	"spec::clients::hyperfleetApi::version":             "API_VERSION",
	"spec::clients::hyperfleetApi::timeout":             "API_TIMEOUT",
	"spec::clients::hyperfleetApi::retryAttempts":       "API_RETRY_ATTEMPTS",
	"spec::clients::hyperfleetApi::retryBackoff":        "API_RETRY_BACKOFF",
	"spec::clients::broker::subscriptionId":             "BROKER_SUBSCRIPTION_ID",
	"spec::clients::broker::topic":                      "BROKER_TOPIC",
}

// cliFlags defines mappings from CLI flag names to config paths
// Note: Uses "::" as key delimiter to avoid conflicts with dots in YAML keys
var cliFlags = map[string]string{
	"debug-config":                "spec::debugConfig",
	"maestro-grpc-server-address": "spec::clients::maestro::grpcServerAddress",
	"maestro-http-server-address": "spec::clients::maestro::httpServerAddress",
	"maestro-source-id":           "spec::clients::maestro::sourceId",
	"maestro-client-id":           "spec::clients::maestro::clientId",
	"maestro-ca-file":             "spec::clients::maestro::auth::tlsConfig::caFile",
	"maestro-cert-file":           "spec::clients::maestro::auth::tlsConfig::certFile",
	"maestro-key-file":            "spec::clients::maestro::auth::tlsConfig::keyFile",
	"maestro-timeout":             "spec::clients::maestro::timeout",
	"maestro-insecure":            "spec::clients::maestro::insecure",
	"hyperfleet-api-timeout":      "spec::clients::hyperfleetApi::timeout",
	"hyperfleet-api-retry":        "spec::clients::hyperfleetApi::retryAttempts",
}

// loadAdapterConfigWithViper loads the deployment configuration from a YAML file
// with environment variable and CLI flag overrides using Viper.
// Priority: CLI flags > Environment variables > Config file > Defaults
func loadAdapterConfigWithViper(filePath string, flags *pflag.FlagSet) (*AdapterConfig, error) {
	// Use "::" as key delimiter to avoid conflicts with dots in YAML keys
	// (e.g., "hyperfleet.io/component" in metadata.labels)
	v := viper.NewWithOptions(viper.KeyDelimiter("::"))

	// Set config file path
	if filePath == "" {
		filePath = os.Getenv(EnvAdapterConfig)
	}

	if filePath == "" {
		return nil, fmt.Errorf("adapter config file path is required (use --config flag or %s env var)",
			EnvAdapterConfig)
	}

	// Read the YAML file first to get base configuration
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read adapter config file %q: %w", filePath, err)
	}

	// Parse YAML into a map for Viper
	var configMap map[string]interface{}

	reader := bytes.NewReader(data)
	decoder := yaml.NewDecoder(reader)

	decoder.KnownFields(true)

	if err := decoder.Decode(&configMap); err != nil {
		//	if err := yaml.Unmarshal(data, &configMap); err != nil {
		return nil, fmt.Errorf("failed to parse adapter config YAML: %w", err)
	}

	// Load the map into Viper
	if err := v.MergeConfigMap(configMap); err != nil {
		return nil, fmt.Errorf("failed to merge config map: %w", err)
	}

	// Bind environment variables
	v.SetEnvPrefix(EnvPrefix)
	v.AutomaticEnv()
	// Replace "::" (our key delimiter) and "-" with "_" for env var lookups
	v.SetEnvKeyReplacer(strings.NewReplacer("::", "_", "-", "_"))

	// Bind specific environment variables
	for configPath, envSuffix := range viperKeyMappings {
		envVar := EnvPrefix + "_" + envSuffix
		if val := os.Getenv(envVar); val != "" {
			v.Set(configPath, val)
		}
	}

	// Legacy broker env vars without HYPERFLEET_ prefix (kept for compatibility)
	if os.Getenv(EnvPrefix+"_BROKER_SUBSCRIPTION_ID") == "" {
		if val := os.Getenv("BROKER_SUBSCRIPTION_ID"); val != "" {
			v.Set("spec::clients::broker::subscriptionId", val)
		}
	}
	if os.Getenv(EnvPrefix+"_BROKER_TOPIC") == "" {
		if val := os.Getenv("BROKER_TOPIC"); val != "" {
			v.Set("spec::clients::broker::topic", val)
		}
	}

	// Bind CLI flags if provided
	if flags != nil {
		for flagName, configPath := range cliFlags {
			if flag := flags.Lookup(flagName); flag != nil && flag.Changed {
				v.Set(configPath, flag.Value.String())
			}
		}
	}

	// Unmarshal into AdapterConfig struct
	var config AdapterConfig
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal adapter config: %w", err)
	}

	return &config, nil
}

// loadTaskConfig loads the task configuration from a YAML file without Viper overrides.
// Task config is purely static YAML configuration.
func loadTaskConfig(filePath string) (*AdapterTaskConfig, error) {
	if filePath == "" {
		filePath = os.Getenv(EnvTaskConfigPath)
	}

	if filePath == "" {
		return nil, fmt.Errorf("task config file path is required (use --task-config flag or %s env var)",
			EnvTaskConfigPath)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read task config file %q: %w", filePath, err)
	}

	var config AdapterTaskConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse task config YAML: %w", err)
	}

	return &config, nil
}

// getBaseDir returns the base directory for a config file path
func getBaseDir(filePath string) (string, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path for %q: %w", filePath, err)
	}
	return filepath.Dir(absPath), nil
}

// loadAdapterConfigWithViperGeneric wraps loadAdapterConfigWithViper, binding CLI flags if provided and of correct type.
func loadAdapterConfigWithViperGeneric(filePath string, flags interface{}) (*AdapterConfig, error) {
	if pflags, ok := flags.(*pflag.FlagSet); ok && pflags != nil {
		return loadAdapterConfigWithViper(filePath, pflags)
	}
	return loadAdapterConfigWithViper(filePath, nil)
}
