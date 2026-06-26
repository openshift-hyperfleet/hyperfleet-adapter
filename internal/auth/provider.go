package auth

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/configloader"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/utils"
)

const (
	mountedTokenPath     = "/var/run/secrets/kubernetes.io/serviceaccount/token"     //nolint:gosec
	mountedNamespacePath = "/var/run/secrets/kubernetes.io/serviceaccount/namespace" //nolint:gosec
	defaultExpSeconds    = int64(3600)
)

// TokenProvider generates a bearer token for an API call Authorization header.
type TokenProvider interface {
	GetToken(ctx context.Context) (string, error)
}

// StaticProvider returns a pre-rendered static token.
type StaticProvider struct {
	token string
}

func (s *StaticProvider) GetToken(_ context.Context) (string, error) {
	return s.token, nil
}

// NewTokenProvider builds a TokenProvider from the given APICallAuth config.
// For type=static, the token field is rendered as a Go template against params.
// For type=kubernetes, a KubernetesProvider is built using the k8s client config.
func NewTokenProvider(
	auth *configloader.APICallAuth,
	k8sCfg configloader.KubernetesConfig,
	params map[string]any,
) (TokenProvider, error) {
	if auth == nil {
		return nil, fmt.Errorf("auth config is nil")
	}

	switch auth.Type {
	case "static":
		rendered, err := utils.RenderTemplate(auth.Token, params)
		if err != nil {
			return nil, fmt.Errorf("failed to render static token template: %w", err)
		}
		return &StaticProvider{token: rendered}, nil

	case "kubernetes":
		ns, err := resolveNamespace(auth.Namespace)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve namespace: %w", err)
		}

		expSeconds := defaultExpSeconds
		if auth.ExpirationSeconds != nil {
			expSeconds = *auth.ExpirationSeconds
		}

		return &KubernetesProvider{
			k8sCfg:         k8sCfg,
			serviceAccount: auth.ServiceAccount,
			namespace:      ns,
			audience:       auth.Audience,
			expSeconds:     expSeconds,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported authorization type %q", auth.Type)
	}
}

// resolveNamespace returns the configured namespace, or reads it from the mounted SA namespace file.
func resolveNamespace(configured string) (string, error) {
	if configured != "" {
		return configured, nil
	}
	data, err := os.ReadFile(mountedNamespacePath)
	if err != nil {
		return "", fmt.Errorf("namespace not configured and could not read %s: %w", mountedNamespacePath, err)
	}
	return string(bytes.TrimSpace(data)), nil
}
