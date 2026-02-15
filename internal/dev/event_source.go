package dev

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// EventSource provides events for development testing
type EventSource interface {
	// LoadEvent loads an event and returns the raw data
	LoadEvent() (map[string]interface{}, error)
}

// FileEventSource loads events from JSON or YAML files
type FileEventSource struct {
	path string
}

// NewFileEventSource creates a new FileEventSource
func NewFileEventSource(path string) *FileEventSource {
	return &FileEventSource{path: path}
}

// LoadEvent loads the event from the file
func (s *FileEventSource) LoadEvent() (map[string]interface{}, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil, fmt.Errorf("failed to read event file %s: %w", s.path, err)
	}

	var event map[string]interface{}

	ext := strings.ToLower(filepath.Ext(s.path))
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &event); err != nil {
			return nil, fmt.Errorf("failed to parse YAML event file: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(data, &event); err != nil {
			return nil, fmt.Errorf("failed to parse JSON event file: %w", err)
		}
	default:
		// Try JSON first, then YAML
		if err := json.Unmarshal(data, &event); err != nil {
			if err := yaml.Unmarshal(data, &event); err != nil {
				return nil, fmt.Errorf("failed to parse event file (tried JSON and YAML): %w", err)
			}
		}
	}

	return event, nil
}

// InlineEventSource parses events from inline JSON/YAML strings
type InlineEventSource struct {
	data string
}

// NewInlineEventSource creates a new InlineEventSource
func NewInlineEventSource(data string) *InlineEventSource {
	return &InlineEventSource{data: data}
}

// LoadEvent parses the inline event data
func (s *InlineEventSource) LoadEvent() (map[string]interface{}, error) {
	var event map[string]interface{}

	// Try JSON first (more common for inline data)
	if err := json.Unmarshal([]byte(s.data), &event); err != nil {
		// Try YAML as fallback
		if err := yaml.Unmarshal([]byte(s.data), &event); err != nil {
			return nil, fmt.Errorf("failed to parse inline event data: %w", err)
		}
	}

	return event, nil
}

// LoadEnvFile loads environment variables from a .env file
func LoadEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read env file %s: %w", path, err)
	}

	envVars := make(map[string]string)
	lines := strings.Split(string(data), "\n")

	for lineNum, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid env file format at line %d: %s", lineNum+1, line)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove surrounding quotes if present
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		envVars[key] = value
	}

	return envVars, nil
}

// ApplyEnvVars sets environment variables from a map
func ApplyEnvVars(envVars map[string]string) error {
	for key, value := range envVars {
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("failed to set env var %s: %w", key, err)
		}
	}
	return nil
}
