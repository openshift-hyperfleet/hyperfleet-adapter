package maestro_client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/manifest"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/transport_client"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/constants"
	apperrors "github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/version"
	"github.com/openshift-online/maestro/pkg/api/openapi"
	"github.com/openshift-online/maestro/pkg/client/cloudevents/grpcsource"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	workv1client "open-cluster-management.io/api/client/work/clientset/versioned/typed/work/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options/cert"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options/grpc"
)

// Default configuration values
const (
	DefaultHTTPTimeout              = 10 * time.Second
	DefaultServerHealthinessTimeout = 20 * time.Second
)

// Client is the Maestro client for managing ManifestWorks via CloudEvents gRPC
type Client struct {
	workClient       workv1client.WorkV1Interface
	maestroAPIClient *openapi.APIClient
	config           *Config
	log              logger.Logger
	grpcOptions      *grpc.GRPCOptions
}

// Config holds configuration for creating a Maestro client
// Following the official Maestro client pattern:
// https://github.com/openshift-online/maestro/blob/main/examples/manifestwork/client.go
type Config struct {
	// MaestroServerAddr is the Maestro HTTP API server address (e.g., "https://maestro.example.com:8000")
	// This is used for the OpenAPI client to communicate with Maestro's REST API
	MaestroServerAddr string

	// GRPCServerAddr is the Maestro gRPC server address (e.g., "maestro-grpc.example.com:8090")
	// This is used for CloudEvents communication
	GRPCServerAddr string

	// SourceID is a unique identifier for this client (used for CloudEvents routing)
	// This identifies the source of ManifestWork operations
	SourceID string

	// TLS Configuration for gRPC (optional - for secure connections)
	// CAFile is the path to the CA certificate file for verifying the gRPC server
	CAFile string
	// ClientCertFile is the path to the client certificate file for mutual TLS (gRPC)
	ClientCertFile string
	// ClientKeyFile is the path to the client key file for mutual TLS (gRPC)
	ClientKeyFile string
	// TokenFile is the path to a token file for token-based authentication (alternative to cert auth)
	TokenFile string

	// TLS Configuration for HTTP API (optional - may use different CA than gRPC)
	// HTTPCAFile is the path to the CA certificate file for verifying the HTTPS API server
	// If not set, falls back to CAFile for backwards compatibility
	HTTPCAFile string

	// Insecure disables TLS verification and allows plaintext connections
	// Use this for local testing without TLS or with self-signed certificates
	// WARNING: NOT recommended for production
	Insecure bool

	// HTTPTimeout is the timeout for HTTP requests to Maestro API (default: 10s)
	HTTPTimeout time.Duration
	// ServerHealthinessTimeout is the timeout for gRPC server health checks (default: 20s)
	ServerHealthinessTimeout time.Duration

	// TransportClient configuration for implementing transport_client.TransportClient
	// ConsumerName is the target cluster name (Maestro consumer) - required for TransportClient
	ConsumerName string
	// WorkName is the name of the ManifestWork to manage - required for TransportClient
	WorkName string
	// WorkLabels are optional labels to add to the ManifestWork
	WorkLabels map[string]string
}

// NewMaestroClient creates a new Maestro client using the official Maestro client pattern
//
// The client uses:
//   - Maestro HTTP API (OpenAPI client) for resource management
//   - CloudEvents over gRPC for ManifestWork operations
//
// Example Usage:
//
//	config := &Config{
//	    MaestroServerAddr: "https://maestro.example.com:8000",
//	    GRPCServerAddr:    "maestro-grpc.example.com:8090",
//	    SourceID:          "hyperfleet-adapter",
//	    CAFile:            "/etc/maestro/certs/ca.crt",
//	    ClientCertFile:    "/etc/maestro/certs/client.crt",
//	    ClientKeyFile:     "/etc/maestro/certs/client.key",
//	}
//	client, err := NewMaestroClient(ctx, config, log)
func NewMaestroClient(ctx context.Context, config *Config, log logger.Logger) (*Client, error) {
	if config == nil {
		return nil, apperrors.ConfigurationError("maestro config is required")
	}
	if config.MaestroServerAddr == "" {
		return nil, apperrors.ConfigurationError("maestro server address is required")
	}

	// Validate MaestroServerAddr URL scheme
	serverURL, err := url.Parse(config.MaestroServerAddr)
	if err != nil {
		return nil, apperrors.ConfigurationError("invalid MaestroServerAddr URL: %v", err)
	}
	// Require http or https scheme (reject schemeless or other schemes like ftp://, grpc://, etc.)
	if serverURL.Scheme != "http" && serverURL.Scheme != "https" {
		return nil, apperrors.ConfigurationError(
			"MaestroServerAddr must use http:// or https:// scheme (got scheme %q in %q)",
			serverURL.Scheme, config.MaestroServerAddr)
	}
	// Enforce https when Insecure=false
	if !config.Insecure && serverURL.Scheme != "https" {
		return nil, apperrors.ConfigurationError(
			"MaestroServerAddr must use https:// scheme when Insecure=false (got %q); "+
				"use https:// URL or set Insecure=true for http:// connections",
			serverURL.Scheme)
	}

	if config.GRPCServerAddr == "" {
		return nil, apperrors.ConfigurationError("maestro gRPC server address is required")
	}
	if config.SourceID == "" {
		return nil, apperrors.ConfigurationError("maestro sourceID is required")
	}

	// Apply defaults
	httpTimeout := config.HTTPTimeout
	if httpTimeout == 0 {
		httpTimeout = DefaultHTTPTimeout
	}
	serverHealthinessTimeout := config.ServerHealthinessTimeout
	if serverHealthinessTimeout == 0 {
		serverHealthinessTimeout = DefaultServerHealthinessTimeout
	}

	log.WithFields(map[string]interface{}{
		"maestroServer": config.MaestroServerAddr,
		"grpcServer":    config.GRPCServerAddr,
		"sourceID":      config.SourceID,
	}).Info(ctx, "Creating Maestro client")

	// Create HTTP client with appropriate TLS configuration
	httpTransport, err := createHTTPTransport(config)
	if err != nil {
		return nil, apperrors.ConfigurationError("failed to create HTTP transport: %v", err)
	}

	// Create Maestro HTTP API client (OpenAPI)
	maestroAPIClient := openapi.NewAPIClient(&openapi.Configuration{
		DefaultHeader: make(map[string]string),
		UserAgent:     version.UserAgent(),
		Debug:         false,
		Servers: openapi.ServerConfigurations{
			{
				URL:         config.MaestroServerAddr,
				Description: "Maestro API Server",
			},
		},
		OperationServers: map[string]openapi.ServerConfigurations{},
		HTTPClient: &http.Client{
			Transport: httpTransport,
			Timeout:   httpTimeout,
		},
	})

	// Create gRPC options
	grpcOptions := &grpc.GRPCOptions{
		Dialer:                   &grpc.GRPCDialer{},
		ServerHealthinessTimeout: &serverHealthinessTimeout,
	}
	grpcOptions.Dialer.URL = config.GRPCServerAddr

	// Configure TLS if certificates are provided
	if err := configureTLS(config, grpcOptions); err != nil {
		return nil, apperrors.ConfigurationError("failed to configure TLS: %v", err)
	}

	// Create the Maestro gRPC work client using the official pattern
	// This returns a workv1client.WorkV1Interface with Kubernetes-style API
	workClient, err := grpcsource.NewMaestroGRPCSourceWorkClient(
		ctx,
		newOCMLoggerAdapter(log),
		maestroAPIClient,
		grpcOptions,
		config.SourceID,
	)
	if err != nil {
		return nil, apperrors.MaestroError("failed to create Maestro work client: %v", err)
	}

	log.WithFields(map[string]interface{}{
		"sourceID": config.SourceID,
	}).Info(ctx, "Maestro client created successfully")

	return &Client{
		workClient:       workClient,
		maestroAPIClient: maestroAPIClient,
		config:           config,
		log:              log,
		grpcOptions:      grpcOptions,
	}, nil
}

// createHTTPTransport creates an HTTP transport with appropriate TLS configuration.
// It clones http.DefaultTransport to preserve important defaults like ProxyFromEnvironment,
// connection pooling, timeouts, etc., and only overrides TLS settings.
func createHTTPTransport(config *Config) (*http.Transport, error) {
	// Clone default transport to preserve ProxyFromEnvironment, DialContext,
	// MaxIdleConns, IdleConnTimeout, TLSHandshakeTimeout, etc.
	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, apperrors.ConfigurationError("http.DefaultTransport is not *http.Transport").AsError()
	}
	transport := defaultTransport.Clone()

	// Build TLS config
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if config.Insecure {
		// Insecure mode: skip TLS verification (works for both http:// and https://)
		tlsConfig.InsecureSkipVerify = true //nolint:gosec // Intentional: user explicitly set Insecure=true
	} else {
		// Secure mode: load CA certificate if provided
		// HTTPCAFile takes precedence, falls back to CAFile for backwards compatibility
		httpCAFile := config.HTTPCAFile
		if httpCAFile == "" {
			httpCAFile = config.CAFile
		}

		if httpCAFile != "" {
			caCert, err := os.ReadFile(httpCAFile)
			if err != nil {
				return nil, err
			}
			caCertPool := x509.NewCertPool()
			if !caCertPool.AppendCertsFromPEM(caCert) {
				return nil, apperrors.ConfigurationError("failed to parse CA certificate from %s", httpCAFile).AsError()
			}
			tlsConfig.RootCAs = caCertPool
		}
	}

	transport.TLSClientConfig = tlsConfig
	return transport, nil
}

// configureTLS sets up TLS configuration for the gRPC connection
func configureTLS(config *Config, grpcOptions *grpc.GRPCOptions) error {
	// Insecure mode: plaintext gRPC connection (no TLS)
	// Note: Unlike HTTP where InsecureSkipVerify allows both http:// and https://,
	// gRPC TLS always requires a TLS handshake on the server side.
	// For self-signed certs with gRPC, use CAFile instead of Insecure=true.
	if config.Insecure {
		grpcOptions.Dialer.TLSConfig = nil
		return nil
	}

	// Option 1: Mutual TLS with certificates
	if config.CAFile != "" && config.ClientCertFile != "" && config.ClientKeyFile != "" {
		certConfig := cert.CertConfig{
			CAFile:         config.CAFile,
			ClientCertFile: config.ClientCertFile,
			ClientKeyFile:  config.ClientKeyFile,
		}
		if err := certConfig.EmbedCerts(); err != nil {
			return err
		}

		tlsConfig, err := cert.AutoLoadTLSConfig(
			certConfig,
			func() (*cert.CertConfig, error) {
				c := cert.CertConfig{
					CAFile:         config.CAFile,
					ClientCertFile: config.ClientCertFile,
					ClientKeyFile:  config.ClientKeyFile,
				}
				if err := c.EmbedCerts(); err != nil {
					return nil, err
				}
				return &c, nil
			},
			grpcOptions.Dialer,
		)
		if err != nil {
			return err
		}
		grpcOptions.Dialer.TLSConfig = tlsConfig
		return nil
	}

	// Option 2: Token-based authentication with CA
	if config.CAFile != "" && config.TokenFile != "" {
		token, err := readTokenFile(config.TokenFile)
		if err != nil {
			return err
		}
		grpcOptions.Dialer.Token = token

		certConfig := cert.CertConfig{
			CAFile: config.CAFile,
		}
		if err := certConfig.EmbedCerts(); err != nil {
			return err
		}

		tlsConfig, err := cert.AutoLoadTLSConfig(
			certConfig,
			func() (*cert.CertConfig, error) {
				c := cert.CertConfig{
					CAFile: config.CAFile,
				}
				if err := c.EmbedCerts(); err != nil {
					return nil, err
				}
				return &c, nil
			},
			grpcOptions.Dialer,
		)
		if err != nil {
			return err
		}
		grpcOptions.Dialer.TLSConfig = tlsConfig
		return nil
	}

	// Option 3: CA only (server verification without client auth)
	if config.CAFile != "" {
		certConfig := cert.CertConfig{
			CAFile: config.CAFile,
		}
		if err := certConfig.EmbedCerts(); err != nil {
			return err
		}

		tlsConfig, err := cert.AutoLoadTLSConfig(
			certConfig,
			func() (*cert.CertConfig, error) {
				c := cert.CertConfig{
					CAFile: config.CAFile,
				}
				if err := c.EmbedCerts(); err != nil {
					return nil, err
				}
				return &c, nil
			},
			grpcOptions.Dialer,
		)
		if err != nil {
			return err
		}
		grpcOptions.Dialer.TLSConfig = tlsConfig
		return nil
	}

	// Fail fast: Insecure=false but no TLS configuration was provided
	// This prevents silently falling back to plaintext connections
	return fmt.Errorf("no TLS configuration provided: set CAFile (with optional ClientCertFile/ClientKeyFile or TokenFile) or set Insecure=true for plaintext connections")
}

// readTokenFile reads a token from a file and trims whitespace.
// Returns an error if the file is empty or contains only whitespace.
func readTokenFile(path string) (string, error) {
	token, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(string(token))
	if trimmed == "" {
		return "", fmt.Errorf("token file %s is empty or contains only whitespace", path)
	}
	return trimmed, nil
}

// Close closes the gRPC connection
func (c *Client) Close() error {
	if c.grpcOptions != nil && c.grpcOptions.Dialer != nil {
		return c.grpcOptions.Dialer.Close()
	}
	return nil
}

// WorkClient returns the underlying WorkV1Interface for ManifestWork operations
func (c *Client) WorkClient() workv1client.WorkV1Interface {
	return c.workClient
}

// SourceID returns the configured source ID
func (c *Client) SourceID() string {
	return c.config.SourceID
}

// ConsumerName returns the target cluster name
func (c *Client) ConsumerName() string {
	return c.config.ConsumerName
}

// WorkName returns the ManifestWork name
func (c *Client) WorkName() string {
	return c.config.WorkName
}

// =============================================================================
// TransportClient Interface Implementation
// =============================================================================

// Ensure Client implements transport_client.TransportClient
var _ transport_client.TransportClient = (*Client)(nil)

// ApplyResources applies multiple resources by bundling them into a ManifestWork.
// All resources are stored in a single ManifestWork for the target cluster.
func (c *Client) ApplyResources(
	ctx context.Context,
	resources []transport_client.ResourceToApply,
) (*transport_client.ApplyResourcesResult, error) {
	result := &transport_client.ApplyResourcesResult{
		Results: make([]*transport_client.ResourceApplyResult, 0, len(resources)),
	}

	if len(resources) == 0 {
		return result, nil
	}

	// Build ManifestWork from resources
	work, err := c.buildManifestWork(resources)
	if err != nil {
		return nil, fmt.Errorf("failed to build ManifestWork: %w", err)
	}

	c.log.Infof(ctx, "Applying %d resources via ManifestWork %s/%s",
		len(resources), c.config.ConsumerName, c.config.WorkName)

	// Apply the ManifestWork (create or update)
	appliedWork, err := c.ApplyManifestWork(ctx, c.config.ConsumerName, work)
	if err != nil {
		// Convert to result with error
		for _, r := range resources {
			resourceResult := &transport_client.ResourceApplyResult{
				Name:         r.Name,
				Kind:         r.Manifest.GetKind(),
				Namespace:    r.Manifest.GetNamespace(),
				ResourceName: r.Manifest.GetName(),
				Error:        err,
			}
			result.Results = append(result.Results, resourceResult)
			result.FailedCount++
		}
		return result, fmt.Errorf("failed to apply ManifestWork: %w", err)
	}

	// Determine operation based on ManifestWork result
	op := c.determineOperation(appliedWork)

	// Build success results for all resources
	for _, r := range resources {
		resourceResult := &transport_client.ResourceApplyResult{
			Name:         r.Name,
			Kind:         r.Manifest.GetKind(),
			Namespace:    r.Manifest.GetNamespace(),
			ResourceName: r.Manifest.GetName(),
			ApplyResult: &transport_client.ApplyResult{
				Resource:  r.Manifest,
				Operation: op,
				Reason:    fmt.Sprintf("applied via ManifestWork %s", c.config.WorkName),
			},
		}
		result.Results = append(result.Results, resourceResult)
		result.SuccessCount++
	}

	c.log.Infof(ctx, "Successfully applied %d resources via ManifestWork", result.SuccessCount)
	return result, nil
}

// GetResource retrieves a resource from the ManifestWork's manifest list.
func (c *Client) GetResource(
	ctx context.Context,
	gvk schema.GroupVersionKind,
	namespace, name string,
) (*unstructured.Unstructured, error) {
	work, err := c.GetManifestWork(ctx, c.config.ConsumerName, c.config.WorkName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			gr := schema.GroupResource{Group: gvk.Group, Resource: gvk.Kind}
			return nil, apierrors.NewNotFound(gr, name)
		}
		return nil, err
	}

	// Search for the resource in the manifests
	for _, m := range work.Spec.Workload.Manifests {
		obj, err := manifestToUnstructured(m)
		if err != nil {
			continue
		}

		if obj.GetKind() == gvk.Kind &&
			obj.GetAPIVersion() == gvk.GroupVersion().String() &&
			obj.GetNamespace() == namespace &&
			obj.GetName() == name {
			return obj, nil
		}
	}

	gr := schema.GroupResource{Group: gvk.Group, Resource: gvk.Kind}
	return nil, apierrors.NewNotFound(gr, name)
}

// DiscoverResources discovers resources within the ManifestWork that match the discovery criteria.
func (c *Client) DiscoverResources(
	ctx context.Context,
	gvk schema.GroupVersionKind,
	discovery manifest.Discovery,
) (*unstructured.UnstructuredList, error) {
	return c.DiscoverManifest(ctx, c.config.ConsumerName, c.config.WorkName, discovery)
}

// buildManifestWork creates a ManifestWork containing all resources as manifests.
func (c *Client) buildManifestWork(resources []transport_client.ResourceToApply) (*workv1.ManifestWork, error) {
	manifests := make([]workv1.Manifest, 0, len(resources))

	// Find the highest generation among all resources
	var maxGeneration int64
	for _, r := range resources {
		gen := manifest.GetGenerationFromUnstructured(r.Manifest)
		if gen > maxGeneration {
			maxGeneration = gen
		}
	}

	// Convert each resource to a Manifest
	for _, r := range resources {
		raw, err := json.Marshal(r.Manifest.Object)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal manifest %s: %w", r.Name, err)
		}
		manifests = append(manifests, workv1.Manifest{
			RawExtension: runtime.RawExtension{Raw: raw},
		})
	}

	// Build the ManifestWork
	work := &workv1.ManifestWork{}
	work.Name = c.config.WorkName
	work.Namespace = c.config.ConsumerName

	if c.config.WorkLabels != nil {
		work.Labels = c.config.WorkLabels
	}

	if work.Annotations == nil {
		work.Annotations = make(map[string]string)
	}
	work.Annotations[constants.AnnotationGeneration] = fmt.Sprintf("%d", maxGeneration)

	work.Spec.Workload.Manifests = manifests

	return work, nil
}

// determineOperation determines the operation that was performed based on the ManifestWork.
func (c *Client) determineOperation(work *workv1.ManifestWork) manifest.Operation {
	if work == nil {
		return manifest.OperationCreate
	}
	if work.ResourceVersion == "" {
		return manifest.OperationCreate
	}
	return manifest.OperationUpdate
}

// manifestToUnstructured converts a workv1.Manifest to an unstructured object.
func manifestToUnstructured(m workv1.Manifest) (*unstructured.Unstructured, error) {
	if m.Raw == nil {
		return nil, fmt.Errorf("manifest has no raw data")
	}

	var obj map[string]interface{}
	if err := json.Unmarshal(m.Raw, &obj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal manifest: %w", err)
	}

	return &unstructured.Unstructured{Object: obj}, nil
}
