package maestro_client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
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
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/utils"
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

// --- TransportClient implementation ---
// The following methods implement transport_client.TransportClient,
// enabling the maestro_client to be used as a transport backend.

// ApplyResources bundles manifests into a ManifestWork and applies it to a target cluster.
// It extracts Maestro-specific settings from opts.TransportConfig:
//   - "targetCluster" (string, required): the consumer name for Maestro
//   - "manifestWorkName" (string, optional): the ManifestWork name
//   - "manifestWorkRefContent" (map[string]interface{}, optional): labels, annotations, deleteOption settings
//   - "resourceName" (string, optional): the resource name from config (for auto-naming)
//   - "params" (map[string]interface{}, optional): template params for rendering refContent values
func (c *Client) ApplyResources(ctx context.Context, resources []transport_client.ResourceToApply, opts transport_client.ApplyOptions) (*transport_client.ApplyResourcesResult, error) {
	// Extract transport config
	tc := opts.TransportConfig
	if tc == nil {
		return nil, apperrors.MaestroError("TransportConfig is required for Maestro transport")
	}

	targetCluster, _ := tc["targetCluster"].(string) //nolint:errcheck // type assertion with zero-value default
	if targetCluster == "" {
		return nil, apperrors.MaestroError("targetCluster is required in TransportConfig")
	}

	manifestWorkName, _ := tc["manifestWorkName"].(string)                 //nolint:errcheck // optional, zero-value default
	resourceName, _ := tc["resourceName"].(string)                         //nolint:errcheck // optional, zero-value default
	refContent, _ := tc["manifestWorkRefContent"].(map[string]interface{}) //nolint:errcheck // optional, nil default
	params, _ := tc["params"].(map[string]interface{})                     //nolint:errcheck // optional, nil default

	// Collect manifests from resources
	manifests := make([]*unstructured.Unstructured, 0, len(resources))
	for _, res := range resources {
		manifests = append(manifests, res.Manifest)
	}

	// Build ManifestWork
	work, err := buildManifestWork(ctx, c.log, manifests, manifestWorkName, resourceName, refContent, params)
	if err != nil {
		return nil, fmt.Errorf("failed to build ManifestWork: %w", err)
	}

	c.log.Infof(ctx, "Applying ManifestWork via Maestro: name=%s targetCluster=%s manifestCount=%d",
		work.Name, targetCluster, len(manifests))

	// Apply ManifestWork
	_, err = c.ApplyManifestWork(ctx, targetCluster, work)
	if err != nil {
		return nil, fmt.Errorf("failed to apply ManifestWork: %w", err)
	}

	// Build results
	results := &transport_client.ApplyResourcesResult{
		Results: make([]transport_client.ApplyResult, 0, len(resources)),
	}
	for _, res := range resources {
		results.Results = append(results.Results, transport_client.ApplyResult{
			Operation: string(manifest.OperationCreate),
			Reason:    fmt.Sprintf("ManifestWork applied to cluster %s with %d manifests", targetCluster, len(manifests)),
			Resource:  res.Manifest,
		})
	}

	return results, nil
}

// GetResource retrieves a resource by GVK, namespace, and name.
// For Maestro transport, individual resource lookup is not supported;
// returns NotFound so that the executor treats it as a new resource.
func (c *Client) GetResource(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) (*unstructured.Unstructured, error) {
	return nil, apierrors.NewNotFound(
		schema.GroupResource{Group: gvk.Group, Resource: gvk.Kind},
		name,
	)
}

// DiscoverResources discovers resources by GVK and discovery config.
// For Maestro transport, discovery returns an empty list.
func (c *Client) DiscoverResources(ctx context.Context, gvk schema.GroupVersionKind, discovery transport_client.Discovery) (*unstructured.UnstructuredList, error) {
	return &unstructured.UnstructuredList{}, nil
}

// buildManifestWork creates a ManifestWork containing the given manifests.
// log may be nil (e.g., when called from mock).
func buildManifestWork(_ context.Context, log logger.Logger, manifests []*unstructured.Unstructured, workName, resourceName string, refContent map[string]interface{}, params map[string]interface{}) (*workv1.ManifestWork, error) {
	// Determine ManifestWork name
	if workName == "" {
		if len(manifests) > 0 {
			workName = fmt.Sprintf("%s-%s", resourceName, manifests[0].GetName())
		} else {
			workName = resourceName
		}
	}

	// Build manifests array for ManifestWork
	manifestEntries := make([]workv1.Manifest, len(manifests))
	for i, m := range manifests {
		manifestBytes, err := m.MarshalJSON()
		if err != nil {
			return nil, fmt.Errorf("failed to marshal manifest[%d]: %w", i, err)
		}
		manifestEntries[i] = workv1.Manifest{
			RawExtension: runtime.RawExtension{Raw: manifestBytes},
		}
	}

	work := &workv1.ManifestWork{
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: manifestEntries,
			},
		},
	}
	work.SetName(workName)

	// Copy the generation annotation from the first manifest to the ManifestWork
	if len(manifests) > 0 {
		manifestAnnotations := manifests[0].GetAnnotations()
		if manifestAnnotations != nil {
			if gen, ok := manifestAnnotations[constants.AnnotationGeneration]; ok {
				work.SetAnnotations(map[string]string{
					constants.AnnotationGeneration: gen,
				})
			}
		}
	}

	// Apply any additional settings from refContent if present
	if refContent != nil {
		if err := applyManifestWorkSettings(log, work, refContent, params); err != nil {
			return nil, fmt.Errorf("failed to apply manifestWork settings: %w", err)
		}
	}

	return work, nil
}

// applyManifestWorkSettings applies settings from the manifestWork ref file to the ManifestWork.
// The ref file can contain metadata (labels, annotations) and spec fields.
// Template variables in string values are rendered using the provided params.
// log may be nil (e.g., when called from mock).
func applyManifestWorkSettings(_ logger.Logger, work *workv1.ManifestWork, settings map[string]interface{}, params map[string]interface{}) error {
	// Apply metadata if present
	if metadata, ok := settings["metadata"].(map[string]interface{}); ok {
		// Apply labels from metadata
		if labels, ok := metadata["labels"].(map[string]interface{}); ok {
			labelMap := make(map[string]string)
			for k, v := range labels {
				if str, ok := v.(string); ok {
					rendered, err := utils.RenderTemplate(str, params)
					if err != nil {
						return fmt.Errorf("failed to render label value for key %s: %w", k, err)
					}
					labelMap[k] = rendered
				}
			}
			work.SetLabels(labelMap)
		}

		// Apply annotations from metadata
		if annotations, ok := metadata["annotations"].(map[string]interface{}); ok {
			annotationMap := make(map[string]string)
			for k, v := range annotations {
				if str, ok := v.(string); ok {
					rendered, err := utils.RenderTemplate(str, params)
					if err != nil {
						return fmt.Errorf("failed to render annotation value for key %s: %w", k, err)
					}
					annotationMap[k] = rendered
				}
			}
			work.SetAnnotations(annotationMap)
		}
	}

	// Also check for labels/annotations at root level (backwards compatibility)
	if labels, ok := settings["labels"].(map[string]interface{}); ok {
		labelMap := make(map[string]string)
		for k, v := range labels {
			if str, ok := v.(string); ok {
				rendered, err := utils.RenderTemplate(str, params)
				if err != nil {
					return fmt.Errorf("failed to render label value for key %s: %w", k, err)
				}
				labelMap[k] = rendered
			}
		}
		// Merge with existing labels
		existing := work.GetLabels()
		if existing == nil {
			existing = make(map[string]string)
		}
		for k, v := range labelMap {
			existing[k] = v
		}
		work.SetLabels(existing)
	}

	if annotations, ok := settings["annotations"].(map[string]interface{}); ok {
		annotationMap := make(map[string]string)
		for k, v := range annotations {
			if str, ok := v.(string); ok {
				rendered, err := utils.RenderTemplate(str, params)
				if err != nil {
					return fmt.Errorf("failed to render annotation value for key %s: %w", k, err)
				}
				annotationMap[k] = rendered
			}
		}
		// Merge with existing annotations
		existing := work.GetAnnotations()
		if existing == nil {
			existing = make(map[string]string)
		}
		for k, v := range annotationMap {
			existing[k] = v
		}
		work.SetAnnotations(existing)
	}

	// Apply spec fields if present
	if spec, ok := settings["spec"].(map[string]interface{}); ok {
		// Apply deleteOption if present
		if deleteOption, ok := spec["deleteOption"].(map[string]interface{}); ok {
			if propagationPolicy, ok := deleteOption["propagationPolicy"].(string); ok {
				work.Spec.DeleteOption = &workv1.DeleteOption{
					PropagationPolicy: workv1.DeletePropagationPolicyType(propagationPolicy),
				}
			}
		}
	}

	return nil
}
