package auth

import (
	"context"
	"fmt"
	"os"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/configloader"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// KubernetesProvider generates a bearer token from Kubernetes.
//
// When audience is empty, it reads the pod's mounted ServiceAccount token file.
// When audience is set, it calls the Kubernetes TokenRequest API to create a
// short-lived bound token for the specified audience.
type KubernetesProvider struct {
	serviceAccount string
	namespace      string
	audience       string
	k8sCfg         configloader.KubernetesConfig
	expSeconds     int64
}

// GetToken returns a Kubernetes ServiceAccount bearer token.
func (p *KubernetesProvider) GetToken(ctx context.Context) (string, error) {
	if p.audience == "" {
		return readMountedToken()
	}
	return p.createTokenRequest(ctx)
}

// readMountedToken reads the projected ServiceAccount token from the standard mount path.
func readMountedToken() (string, error) {
	data, err := os.ReadFile(mountedTokenPath)
	if err != nil {
		return "", fmt.Errorf("failed to read mounted ServiceAccount token from %s: %w", mountedTokenPath, err)
	}
	return string(data), nil
}

// createTokenRequest calls the Kubernetes TokenRequest API to create a bound token.
func (p *KubernetesProvider) createTokenRequest(ctx context.Context) (string, error) {
	restCfg, err := buildRestConfig(p.k8sCfg)
	if err != nil {
		return "", fmt.Errorf("failed to build Kubernetes REST config: %w", err)
	}

	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return "", fmt.Errorf("failed to create Kubernetes clientset: %w", err)
	}

	return p.createTokenRequestWithClientset(ctx, cs)
}

// createTokenRequestWithClientset creates a bound token using the provided clientset.
// Separated from createTokenRequest to allow unit testing with a fake clientset.
func (p *KubernetesProvider) createTokenRequestWithClientset(
	ctx context.Context, cs kubernetes.Interface,
) (string, error) {
	expSeconds := p.expSeconds
	tr, err := cs.CoreV1().ServiceAccounts(p.namespace).CreateToken(ctx, p.serviceAccount,
		&authv1.TokenRequest{
			Spec: authv1.TokenRequestSpec{
				Audiences:         []string{p.audience},
				ExpirationSeconds: &expSeconds,
			},
		},
		metav1.CreateOptions{},
	)
	if err != nil {
		return "", fmt.Errorf("failed to create token for ServiceAccount %s/%s (audience=%s): %w",
			p.namespace, p.serviceAccount, p.audience, err)
	}

	return tr.Status.Token, nil
}

// buildRestConfig mirrors the logic in k8sclient.NewClient to produce a *rest.Config.
func buildRestConfig(cfg configloader.KubernetesConfig) (*rest.Config, error) {
	if cfg.KubeConfigPath != "" {
		restCfg, err := clientcmd.BuildConfigFromFlags("", cfg.KubeConfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load kubeconfig from %s: %w", cfg.KubeConfigPath, err)
		}
		return restCfg, nil
	}

	restCfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create in-cluster config: %w", err)
	}
	return restCfg, nil
}
