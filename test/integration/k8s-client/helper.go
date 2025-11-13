// +build integration

package k8sclient_integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	k8sclient "github.com/openshift-hyperfleet/hyperfleet-adapter/internal/k8s-client"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// TestEnv holds the test environment for integration tests
type TestEnv struct {
	Env     *envtest.Environment
	Config  *rest.Config
	Client  *k8sclient.Client
	Manager *k8sclient.ResourceManager
	Ctx     context.Context
	Log     logger.Logger
}

// SetupTestEnv creates a new test environment with envtest
func SetupTestEnv(t *testing.T) *TestEnv {
	t.Helper()

	ctx := context.Background()
	log := logger.NewLogger(ctx)

	// Get KUBEBUILDER_ASSETS from environment
	// This should be set by `make test-integration` or setup-envtest
	kubebuilderAssets := os.Getenv("KUBEBUILDER_ASSETS")
	if kubebuilderAssets == "" {
		t.Fatal("KUBEBUILDER_ASSETS environment variable not set. " +
			"Run 'setup-envtest use 1.31.x' and export KUBEBUILDER_ASSETS=$(setup-envtest use -i -p path 1.31.x)")
	}

	// Setup envtest with explicit binary path
	testEnv := &envtest.Environment{
		BinaryAssetsDirectory: kubebuilderAssets,
		CRDDirectoryPaths:     []string{},
		ErrorIfCRDPathMissing: false,
	}

	cfg, err := testEnv.Start()
	require.NoError(t, err, "Failed to start test environment")
	require.NotNil(t, cfg, "Test environment config is nil")

	// Create k8s client using the test config
	client, err := k8sclient.NewClientFromConfig(ctx, cfg, log)
	require.NoError(t, err, "Failed to create k8s client")
	require.NotNil(t, client, "Client is nil")

	// Create resource manager
	manager := k8sclient.NewResourceManager(client, log)
	require.NotNil(t, manager, "Manager is nil")

	return &TestEnv{
		Env:     testEnv,
		Config:  cfg,
		Client:  client,
		Manager: manager,
		Ctx:     ctx,
		Log:     log,
	}
}

// Cleanup stops the test environment
func (e *TestEnv) Cleanup(t *testing.T) {
	t.Helper()
	if e.Env != nil {
		err := e.Env.Stop()
		require.NoError(t, err, "Failed to stop test environment")
	}
}

// WriteKubeconfig writes the test environment's kubeconfig to a temp file
func (e *TestEnv) WriteKubeconfig(t *testing.T) string {
	t.Helper()

	// Create a temp directory
	tmpDir := t.TempDir()
	kubeconfigPath := filepath.Join(tmpDir, "kubeconfig")

	// Write kubeconfig
	user, err := e.Env.AddUser(envtest.User{
		Name:   "test-user",
		Groups: []string{"system:masters"},
	}, e.Config)
	require.NoError(t, err, "Failed to create test user")

	kubeconfigBytes, err := user.KubeConfig()
	require.NoError(t, err, "Failed to generate kubeconfig")

	err = os.WriteFile(kubeconfigPath, kubeconfigBytes, 0600)
	require.NoError(t, err, "Failed to write kubeconfig")

	return kubeconfigPath
}

