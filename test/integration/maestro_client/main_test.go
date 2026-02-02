// main_test.go provides shared test setup for Maestro integration tests.
// It starts PostgreSQL and Maestro server containers that are reused across all test functions.

package maestro_client_integration

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
)

const (
	// MaestroImage is the Maestro server container image
	MaestroImage = "quay.io/redhat-user-workloads/maestro-rhtap-tenant/maestro/maestro:latest"

	// PostgresImage is the PostgreSQL container image
	PostgresImage = "postgres:15-alpine"

	// Default ports
	PostgresPort      = "5432/tcp"
	MaestroHTTPPort   = "8000/tcp"
	MaestroGRPCPort   = "8090/tcp"
	MaestroHealthPort = "8083/tcp"
)

// MaestroTestEnv holds the test environment configuration
type MaestroTestEnv struct {
	// PostgreSQL
	PostgresContainer testcontainers.Container
	PostgresHost      string
	PostgresPort      string

	// Maestro
	MaestroContainer testcontainers.Container
	MaestroHost      string
	MaestroHTTPPort  string
	MaestroGRPCPort  string

	// Connection strings
	MaestroServerAddr string // HTTP API address (e.g., "http://localhost:32000")
	MaestroGRPCAddr   string // gRPC address (e.g., "localhost:32001")
}

// sharedEnv holds the shared test environment for all integration tests
var sharedEnv *MaestroTestEnv

// setupErr holds any error that occurred during setup
var setupErr error

// TestMain runs before all tests to set up the shared containers
func TestMain(m *testing.M) {
	flag.Parse()

	// Check if we should skip integration tests
	if testing.Short() {
		os.Exit(m.Run())
	}

	// Check if SKIP_MAESTRO_INTEGRATION_TESTS is set
	if os.Getenv("SKIP_MAESTRO_INTEGRATION_TESTS") == "true" {
		println("⚠️  SKIP_MAESTRO_INTEGRATION_TESTS is set, skipping maestro_client integration tests")
		os.Exit(m.Run())
	}

	// Quick check if testcontainers can work
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	provider, err := testcontainers.NewDockerProvider()
	if err != nil {
		setupErr = err
		println("⚠️  Warning: Could not connect to container runtime:", err.Error())
		println("   Tests will be skipped")
	} else {
		info, err := provider.DaemonHost(ctx)
		_ = provider.Close()

		if err != nil {
			setupErr = err
			println("⚠️  Warning: Could not get container runtime info:", err.Error())
			println("   Tests will be skipped")
		} else {
			println("✅ Container runtime available:", info)
			println("🚀 Starting Maestro test environment...")

			// Set up the shared environment
			env, err := setupMaestroTestEnv()
			if err != nil {
				setupErr = err
				println("❌ Failed to set up Maestro environment:", err.Error())
				println("   Tests will be skipped")
			} else {
				sharedEnv = env
				println("✅ Maestro test environment ready!")
				println(fmt.Sprintf("   HTTP API: %s", env.MaestroServerAddr))
				println(fmt.Sprintf("   gRPC:     %s", env.MaestroGRPCAddr))
			}
		}
	}
	println()

	// Run tests
	exitCode := m.Run()

	// Cleanup after all tests
	if sharedEnv != nil {
		println()
		println("🧹 Cleaning up Maestro test environment...")
		cleanupMaestroTestEnv(sharedEnv)
	}

	os.Exit(exitCode)
}

// GetSharedEnv returns the shared test environment.
// If setup failed or environment is not initialized (e.g., short mode), the test will be skipped.
func GetSharedEnv(t *testing.T) *MaestroTestEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	if setupErr != nil {
		t.Skipf("Maestro environment setup failed: %v", setupErr)
	}
	if sharedEnv == nil {
		t.Skip("Shared test environment is not initialized")
	}
	return sharedEnv
}
