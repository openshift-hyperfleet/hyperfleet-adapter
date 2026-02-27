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
	"debug_config":                                     "DEBUG_CONFIG",
	"clients::maestro::grpc_server_address":            "MAESTRO_GRPC_SERVER_ADDRESS",
	"clients::maestro::http_server_address":            "MAESTRO_HTTP_SERVER_ADDRESS",
	"clients::maestro::source_id":                      "MAESTRO_SOURCE_ID",
	"clients::maestro::client_id":                      "MAESTRO_CLIENT_ID",
	"clients::maestro::auth::tls_config::ca_file":      "MAESTRO_CA_FILE",
	"clients::maestro::auth::tls_config::cert_file":    "MAESTRO_CERT_FILE",
	"clients::maestro::auth::tls_config::key_file":     "MAESTRO_KEY_FILE",
	"clients::maestro::auth::tls_config::http_ca_file": "MAESTRO_HTTP_CA_FILE",
	"clients::maestro::timeout":                        "MAESTRO_TIMEOUT",
	"clients::maestro::server_healthiness_timeout":     "MAESTRO_SERVER_HEALTHINESS_TIMEOUT",
	"clients::maestro::retry_attempts":                 "MAESTRO_RETRY_ATTEMPTS",
	"clients::maestro::insecure":                       "MAESTRO_INSECURE",
	"clients::hyperfleet_api::base_url":                "API_BASE_URL",
	"clients::hyperfleet_api::version":                 "API_VERSION",
	"clients::hyperfleet_api::timeout":                 "API_TIMEOUT",
	"clients::hyperfleet_api::retry_attempts":          "API_RETRY_ATTEMPTS",
	"clients::hyperfleet_api::retry_backoff":           "API_RETRY_BACKOFF",
	"clients::broker::subscription_id":                 "BROKER_SUBSCRIPTION_ID",
	"clients::broker::topic":                           "BROKER_TOPIC",
}

// cliFlags defines mappings from CLI flag names to config paths
// Note: Uses "::" as key delimiter to avoid conflicts with dots in YAML keys
var cliFlags = map[string]string{
	"debug-config":                "debug_config",
	"maestro-grpc-server-address": "clients::maestro::grpc_server_address",
	"maestro-http-server-address": "clients::maestro::http_server_address",
	"maestro-source-id":           "clients::maestro::source_id",
	"maestro-client-id":           "clients::maestro::client_id",
	"maestro-ca-file":             "clients::maestro::auth::tls_config::ca_file",
	"maestro-cert-file":           "clients::maestro::auth::tls_config::cert_file",
	"maestro-key-file":            "clients::maestro::auth::tls_config::key_file",
	"maestro-timeout":             "clients::maestro::timeout",
	"maestro-insecure":            "clients::maestro::insecure",
	"hyperfleet-api-timeout":      "clients::hyperfleet_api::timeout",
	"hyperfleet-api-retry":        "clients::hyperfleet_api::retry_attempts",
}

// standardConfigPaths are tried when no explicit config path is provided
var standardConfigPaths = []string{
	"/etc/hyperfleet/config.yaml", // production
	"./configs/config.yaml",       // development
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

	// Try standard paths if no path configured
	if filePath == "" {
		for _, p := range standardConfigPaths {
			if _, err := os.Stat(p); err == nil {
				filePath = p
				break
			}
		}
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
			v.Set("clients::broker::subscription_id", val)
		}
	}
	if os.Getenv(EnvPrefix+"_BROKER_TOPIC") == "" {
		if val := os.Getenv("BROKER_TOPIC"); val != "" {
			v.Set("clients::broker::topic", val)
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
