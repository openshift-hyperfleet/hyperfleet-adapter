package logger

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestNewLogger(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
	}{
		{
			name: "create_logger_with_empty_context",
			ctx:  context.Background(),
		},
		{
			name: "create_logger_with_nil_context",
			ctx:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := NewLogger(tt.ctx)
			if log == nil {
				t.Fatal("NewLogger returned nil")
			}

			// Type assertion to check implementation
			if _, ok := log.(*logger); !ok {
				t.Error("NewLogger didn't return *logger type")
			}
		})
	}
}

func TestInitLogger(t *testing.T) {
	t.Run("initialize_default_logger", func(t *testing.T) {
		// Reset default logger
		defaultLogger = nil

		err := InitLogger()
		if err != nil {
			t.Fatalf("InitLogger failed: %v", err)
		}

		if defaultLogger == nil {
			t.Error("InitLogger didn't set defaultLogger")
		}
	})
}

func TestLoggerV(t *testing.T) {
	ctx := context.Background()
	log := NewLogger(ctx)

	tests := []struct {
		name  string
		level int32
	}{
		{"verbosity_level_0", 0},
		{"verbosity_level_1", 1},
		{"verbosity_level_2", 2},
		{"verbosity_level_5", 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vLog := log.V(tt.level)
			if vLog == nil {
				t.Fatal("V() returned nil")
			}

			// Type assertion
			if vLogger, ok := vLog.(*logger); ok {
				if vLogger.level != tt.level {
					t.Errorf("Expected level %d, got %d", tt.level, vLogger.level)
				}
			} else {
				t.Error("V() didn't return *logger type")
			}
		})
	}
}

func TestLoggerExtra(t *testing.T) {
	ctx := context.Background()
	log := NewLogger(ctx)

	tests := []struct {
		name  string
		key   string
		value interface{}
	}{
		{"add_string_extra", "key1", "value1"},
		{"add_int_extra", "key2", 123},
		{"add_bool_extra", "key3", true},
		{"add_nil_extra", "key4", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extraLog := log.Extra(tt.key, tt.value)
			if extraLog == nil {
				t.Fatal("Extra() returned nil")
			}

			// Type assertion to check extra field
			if eLogger, ok := extraLog.(*logger); ok {
				if val, exists := eLogger.extra[tt.key]; !exists {
					t.Errorf("Extra key %s not found", tt.key)
				} else if val != tt.value {
					t.Errorf("Expected value %v, got %v", tt.value, val)
				}
			} else {
				t.Error("Extra() didn't return *logger type")
			}
		})
	}
}

func TestLoggerExtraChaining(t *testing.T) {
	ctx := context.Background()
	log := NewLogger(ctx)

	t.Run("chain_multiple_extra_calls", func(t *testing.T) {
		chainedLog := log.Extra("key1", "value1").
			Extra("key2", 456).
			Extra("key3", true)

		if eLogger, ok := chainedLog.(*logger); ok {
			if len(eLogger.extra) != 3 {
				t.Errorf("Expected 3 extra fields, got %d", len(eLogger.extra))
			}
		} else {
			t.Error("Chained Extra() didn't return *logger type")
		}
	})
}

func TestLoggerContextHandling(t *testing.T) {
	// Setup test logger with observer
	core, observed := observer.New(zapcore.InfoLevel)
	testLogger := zap.New(core)
	defaultLogger = testLogger

	tests := []struct {
		name             string
		ctx              context.Context
		message          string
		expectedContains []string
	}{
		{
			name:             "log_with_txid_int",
			ctx:              context.WithValue(context.Background(), "txid", int64(12345)),
			message:          "test message",
			expectedContains: []string{"tx_id=12345", "test message"},
		},
		{
			name:             "log_with_txid_string",
			ctx:              context.WithValue(context.Background(), "txid", "txid-abc"),
			message:          "test message",
			expectedContains: []string{"tx_id=txid-abc", "test message"},
		},
		{
			name:             "log_with_adapter_id",
			ctx:              context.WithValue(context.Background(), AdapterIDKey, "validation"),
			message:          "test message",
			expectedContains: []string{"adapter_id=validation", "test message"},
		},
		{
			name:             "log_with_cluster_id",
			ctx:              context.WithValue(context.Background(), ClusterIDKey, "cluster-123"),
			message:          "test message",
			expectedContains: []string{"cluster_id=cluster-123", "test message"},
		},
		{
			name:             "log_with_opid",
			ctx:              context.WithValue(context.Background(), OpIDKey, "op-456"),
			message:          "test message",
			expectedContains: []string{"opid=op-456", "test message"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observed.TakeAll() // Clear previous logs

			log := NewLogger(tt.ctx)
			log.Info(tt.message)

			entries := observed.TakeAll()
			if len(entries) == 0 {
				t.Fatal("No log entries captured")
			}

			logMessage := entries[0].Message
			for _, expected := range tt.expectedContains {
				if !contains(logMessage, expected) {
					t.Errorf("Log message '%s' doesn't contain '%s'", logMessage, expected)
				}
			}
		})
	}
}

func TestLoggerMultipleContextValues(t *testing.T) {
	// Setup test logger with observer
	core, observed := observer.New(zapcore.InfoLevel)
	testLogger := zap.New(core)
	defaultLogger = testLogger

	t.Run("log_with_multiple_context_values", func(t *testing.T) {
		observed.TakeAll() // Clear previous logs

		ctx := context.Background()
		ctx = context.WithValue(ctx, "txid", "tx-999")
		ctx = context.WithValue(ctx, AdapterIDKey, "dns")
		ctx = context.WithValue(ctx, ClusterIDKey, "prod-cluster")
		ctx = context.WithValue(ctx, OpIDKey, "create-namespace")

		log := NewLogger(ctx)
		log.Info("multiple context test")

		entries := observed.TakeAll()
		if len(entries) == 0 {
			t.Fatal("No log entries captured")
		}

		logMessage := entries[0].Message
		expectedValues := []string{
			"tx_id=tx-999",
			"adapter_id=dns",
			"cluster_id=prod-cluster",
			"opid=create-namespace",
			"multiple context test",
		}

		for _, expected := range expectedValues {
			if !contains(logMessage, expected) {
				t.Errorf("Log message '%s' doesn't contain '%s'", logMessage, expected)
			}
		}
	})
}

func TestLoggerLevels(t *testing.T) {
	// Setup test logger with observer (capture all levels)
	core, observed := observer.New(zapcore.DebugLevel)
	testLogger := zap.New(core)
	defaultLogger = testLogger

	ctx := context.Background()
	log := NewLogger(ctx)

	tests := []struct {
		name         string
		logFunc      func(string)
		message      string
		expectedLevel zapcore.Level
	}{
		{
			name:          "info_level",
			logFunc:       log.Info,
			message:       "info message",
			expectedLevel: zapcore.InfoLevel,
		},
		{
			name:          "warning_level",
			logFunc:       log.Warning,
			message:       "warning message",
			expectedLevel: zapcore.WarnLevel,
		},
		{
			name:          "error_level",
			logFunc:       log.Error,
			message:       "error message",
			expectedLevel: zapcore.ErrorLevel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observed.TakeAll() // Clear previous logs

			tt.logFunc(tt.message)

			entries := observed.TakeAll()
			if len(entries) == 0 {
				t.Fatal("No log entries captured")
			}

			entry := entries[0]
			if entry.Level != tt.expectedLevel {
				t.Errorf("Expected log level %v, got %v", tt.expectedLevel, entry.Level)
			}

			if !contains(entry.Message, tt.message) {
				t.Errorf("Log message '%s' doesn't contain '%s'", entry.Message, tt.message)
			}
		})
	}
}

func TestLoggerInfof(t *testing.T) {
	// Setup test logger with observer
	core, observed := observer.New(zapcore.InfoLevel)
	testLogger := zap.New(core)
	defaultLogger = testLogger

	ctx := context.Background()
	log := NewLogger(ctx)

	tests := []struct {
		name   string
		format string
		args   []interface{}
		expect string
	}{
		{
			name:   "formatted_string",
			format: "User %s logged in",
			args:   []interface{}{"john"},
			expect: "User john logged in",
		},
		{
			name:   "formatted_multiple_args",
			format: "Processing %d items in namespace %s",
			args:   []interface{}{5, "default"},
			expect: "Processing 5 items in namespace default",
		},
		{
			name:   "no_formatting",
			format: "Simple message",
			args:   []interface{}{},
			expect: "Simple message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observed.TakeAll() // Clear previous logs

			log.Infof(tt.format, tt.args...)

			entries := observed.TakeAll()
			if len(entries) == 0 {
				t.Fatal("No log entries captured")
			}

			if !contains(entries[0].Message, tt.expect) {
				t.Errorf("Log message '%s' doesn't contain '%s'", entries[0].Message, tt.expect)
			}
		})
	}
}

func TestLoggerInfofWithContext(t *testing.T) {
	// Setup test logger with observer
	core, observed := observer.New(zapcore.InfoLevel)
	testLogger := zap.New(core)
	defaultLogger = testLogger

	t.Run("infof_with_context_values", func(t *testing.T) {
		observed.TakeAll() // Clear previous logs

		ctx := context.WithValue(context.Background(), OpIDKey, "op-123")
		log := NewLogger(ctx)

		log.Infof("Created %d resources", 3)

		entries := observed.TakeAll()
		if len(entries) == 0 {
			t.Fatal("No log entries captured")
		}

		logMessage := entries[0].Message
		if !contains(logMessage, "opid=op-123") {
			t.Errorf("Log message '%s' doesn't contain opid", logMessage)
		}
		if !contains(logMessage, "Created 3 resources") {
			t.Errorf("Log message '%s' doesn't contain formatted message", logMessage)
		}
	})
}

func TestLoggerExtraInLogMessage(t *testing.T) {
	// Setup test logger with observer
	core, observed := observer.New(zapcore.InfoLevel)
	testLogger := zap.New(core)
	defaultLogger = testLogger

	t.Run("extra_fields_appear_in_log", func(t *testing.T) {
		observed.TakeAll() // Clear previous logs

		ctx := context.Background()
		log := NewLogger(ctx)

		log.Extra("resource", "namespace").Extra("operation", "create").Info("Resource created")

		entries := observed.TakeAll()
		if len(entries) == 0 {
			t.Fatal("No log entries captured")
		}

		logMessage := entries[0].Message
		if !contains(logMessage, "resource=namespace") {
			t.Errorf("Log message '%s' doesn't contain extra field 'resource=namespace'", logMessage)
		}
		if !contains(logMessage, "operation=create") {
			t.Errorf("Log message '%s' doesn't contain extra field 'operation=create'", logMessage)
		}
		if !contains(logMessage, "Resource created") {
			t.Errorf("Log message '%s' doesn't contain message", logMessage)
		}
	})
}

func TestContextKeys(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected string
	}{
		{"txid_key", TxIDKey, "txid"},
		{"adapter_id_key", AdapterIDKey, "adapter_id"},
		{"cluster_id_key", ClusterIDKey, "cluster_id"},
		{"opid_key", OpIDKey, "opid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.key != tt.expected {
				t.Errorf("Expected constant %s to equal '%s', got '%s'", tt.name, tt.expected, tt.key)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || 
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

