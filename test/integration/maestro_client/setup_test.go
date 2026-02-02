package maestro_client_integration

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// Database configuration
	dbName     = "maestro"
	dbUser     = "maestro"
	dbPassword = "maestro-test-password"
)

// setupMaestroTestEnv starts PostgreSQL and Maestro containers
func setupMaestroTestEnv() (*MaestroTestEnv, error) {
	ctx := context.Background()
	env := &MaestroTestEnv{}

	// Step 1: Start PostgreSQL
	println("   📦 Starting PostgreSQL container...")
	pgContainer, err := startPostgresContainer(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start PostgreSQL: %w", err)
	}
	env.PostgresContainer = pgContainer

	// Get PostgreSQL connection info
	host, err := pgContainer.Host(ctx)
	if err != nil {
		_ = pgContainer.Terminate(ctx)
		return nil, fmt.Errorf("failed to get PostgreSQL host: %w", err)
	}
	env.PostgresHost = host

	port, err := pgContainer.MappedPort(ctx, nat.Port(PostgresPort))
	if err != nil {
		_ = pgContainer.Terminate(ctx)
		return nil, fmt.Errorf("failed to get PostgreSQL port: %w", err)
	}
	env.PostgresPort = port.Port()
	println(fmt.Sprintf("   ✅ PostgreSQL ready at %s:%s", env.PostgresHost, env.PostgresPort))

	// Step 2: Run Maestro migration
	println("   🔄 Running Maestro database migration...")
	if err := runMaestroMigration(ctx, env); err != nil {
		_ = pgContainer.Terminate(ctx)
		return nil, fmt.Errorf("failed to run Maestro migration: %w", err)
	}
	println("   ✅ Database migration complete")

	// Step 3: Start Maestro server
	println("   📦 Starting Maestro server container...")
	maestroContainer, err := startMaestroServer(ctx, env)
	if err != nil {
		_ = pgContainer.Terminate(ctx)
		return nil, fmt.Errorf("failed to start Maestro server: %w", err)
	}
	env.MaestroContainer = maestroContainer

	// Get Maestro connection info
	env.MaestroHost, err = maestroContainer.Host(ctx)
	if err != nil {
		cleanupMaestroTestEnv(env)
		return nil, fmt.Errorf("failed to get Maestro host: %w", err)
	}

	httpPort, err := maestroContainer.MappedPort(ctx, nat.Port(MaestroHTTPPort))
	if err != nil {
		cleanupMaestroTestEnv(env)
		return nil, fmt.Errorf("failed to get Maestro HTTP port: %w", err)
	}
	env.MaestroHTTPPort = httpPort.Port()

	grpcPort, err := maestroContainer.MappedPort(ctx, nat.Port(MaestroGRPCPort))
	if err != nil {
		cleanupMaestroTestEnv(env)
		return nil, fmt.Errorf("failed to get Maestro gRPC port: %w", err)
	}
	env.MaestroGRPCPort = grpcPort.Port()

	// Build connection strings
	env.MaestroServerAddr = fmt.Sprintf("http://%s:%s", env.MaestroHost, env.MaestroHTTPPort)
	env.MaestroGRPCAddr = fmt.Sprintf("%s:%s", env.MaestroHost, env.MaestroGRPCPort)

	println("   ✅ Maestro server ready")

	return env, nil
}

// startPostgresContainer starts a PostgreSQL container
func startPostgresContainer(ctx context.Context) (testcontainers.Container, error) {
	req := testcontainers.ContainerRequest{
		Image:        PostgresImage,
		ExposedPorts: []string{PostgresPort},
		Env: map[string]string{
			"POSTGRES_DB":       dbName,
			"POSTGRES_USER":     dbUser,
			"POSTGRES_PASSWORD": dbPassword,
		},
		WaitingFor: wait.ForAll(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
			wait.ForListeningPort(nat.Port(PostgresPort)).
				WithStartupTimeout(60*time.Second),
		),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, err
	}

	return container, nil
}

// runMaestroMigration runs the Maestro database migration
func runMaestroMigration(ctx context.Context, env *MaestroTestEnv) error {
	// Get the PostgreSQL container's IP on the default bridge network
	pgInspect, err := env.PostgresContainer.Inspect(ctx)
	if err != nil {
		return fmt.Errorf("failed to inspect PostgreSQL container: %w", err)
	}

	// Try to get the container IP from the bridge network
	pgIP := ""
	for _, network := range pgInspect.NetworkSettings.Networks {
		if network.IPAddress != "" {
			pgIP = network.IPAddress
			break
		}
	}

	if pgIP == "" {
		// Fallback to host.docker.internal for Docker Desktop
		pgIP = "host.docker.internal"
	}

	req := testcontainers.ContainerRequest{
		Image: MaestroImage,
		Cmd: []string{
			"/usr/local/bin/maestro",
			"migration",
			"--db-host", pgIP,
			"--db-port", "5432",
			"--db-user", dbUser,
			"--db-password", dbPassword,
			"--db-name", dbName,
			"--db-sslmode", "disable",
			"--alsologtostderr",
			"-v=2",
		},
		WaitingFor: wait.ForExit().WithExitTimeout(120 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return fmt.Errorf("failed to run migration container: %w", err)
	}
	defer func() {
		_ = container.Terminate(ctx)
	}()

	// Check exit code
	state, err := container.State(ctx)
	if err != nil {
		return fmt.Errorf("failed to get migration container state: %w", err)
	}

	if state.ExitCode != 0 {
		// Get logs for debugging
		logs, _ := container.Logs(ctx)
		if logs != nil {
			defer logs.Close() //nolint:errcheck
			buf := make([]byte, 4096)
			n, _ := logs.Read(buf)
			println(fmt.Sprintf("      Migration logs: %s", string(buf[:n])))
		}
		return fmt.Errorf("migration failed with exit code %d", state.ExitCode)
	}

	return nil
}

// startMaestroServer starts the Maestro server container
func startMaestroServer(ctx context.Context, env *MaestroTestEnv) (testcontainers.Container, error) {
	// Get PostgreSQL container IP
	pgInspect, err := env.PostgresContainer.Inspect(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect PostgreSQL container: %w", err)
	}

	pgIP := ""
	for _, network := range pgInspect.NetworkSettings.Networks {
		if network.IPAddress != "" {
			pgIP = network.IPAddress
			break
		}
	}

	if pgIP == "" {
		pgIP = "host.docker.internal"
	}

	req := testcontainers.ContainerRequest{
		Image:        MaestroImage,
		ExposedPorts: []string{MaestroHTTPPort, MaestroGRPCPort, MaestroHealthPort},
		Cmd: []string{
			"/usr/local/bin/maestro",
			"server",
			"--db-host", pgIP,
			"--db-port", "5432",
			"--db-user", dbUser,
			"--db-password", dbPassword,
			"--db-name", dbName,
			"--db-sslmode", "disable",
			"--enable-grpc-server=true",
			"--grpc-server-bindport=8090",
			"--http-server-bindport=8000",
			"--health-check-server-bindport=8083",
			"--message-broker-type=grpc",
			"--alsologtostderr",
			"-v=2",
		},
		WaitingFor: wait.ForAll(
			wait.ForListeningPort(nat.Port(MaestroHTTPPort)).WithStartupTimeout(120*time.Second),
			wait.ForListeningPort(nat.Port(MaestroGRPCPort)).WithStartupTimeout(120*time.Second),
			wait.ForHTTP("/api/maestro/v1").
				WithPort(nat.Port(MaestroHTTPPort)).
				WithStartupTimeout(120*time.Second),
		),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, err
	}

	return container, nil
}

// cleanupMaestroTestEnv cleans up all containers
func cleanupMaestroTestEnv(env *MaestroTestEnv) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if env.MaestroContainer != nil {
		println("   Stopping Maestro server...")
		if err := env.MaestroContainer.Terminate(ctx); err != nil {
			println(fmt.Sprintf("   ⚠️  Warning: Failed to terminate Maestro: %v", err))
		}
	}

	if env.PostgresContainer != nil {
		println("   Stopping PostgreSQL...")
		if err := env.PostgresContainer.Terminate(ctx); err != nil {
			println(fmt.Sprintf("   ⚠️  Warning: Failed to terminate PostgreSQL: %v", err))
		}
	}

	println("   ✅ Cleanup complete")
}
