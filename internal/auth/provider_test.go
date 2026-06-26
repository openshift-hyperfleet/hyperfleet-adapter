package auth

import (
	"context"
	"testing"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/configloader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

// TestStaticProvider verifies that StaticProvider returns its token unchanged.
func TestStaticProvider(t *testing.T) {
	p := &StaticProvider{token: "my-static-token"}
	token, err := p.GetToken(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "my-static-token", token)
}

// TestNewTokenProvider_Static verifies template rendering for the static type.
func TestNewTokenProvider_Static(t *testing.T) {
	tests := []struct {
		name      string
		tokenTmpl string
		params    map[string]any
		wantToken string
		wantErr   bool
	}{
		{
			name:      "literal token",
			tokenTmpl: "abc123",
			params:    map[string]any{},
			wantToken: "abc123",
		},
		{
			name:      "template-rendered token",
			tokenTmpl: "{{ .apiKey }}",
			params:    map[string]any{"apiKey": "rendered-key"},
			wantToken: "rendered-key",
		},
		{
			name:      "missing template variable returns error",
			tokenTmpl: "{{ .missing }}",
			params:    map[string]any{},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &configloader.APICallAuth{Type: "static", Token: tt.tokenTmpl}
			provider, err := NewTokenProvider(a, configloader.KubernetesConfig{}, tt.params)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			token, err := provider.GetToken(context.Background())
			require.NoError(t, err)
			assert.Equal(t, tt.wantToken, token)
		})
	}
}

// TestNewTokenProvider_Unknown verifies that an unknown type returns an error.
func TestNewTokenProvider_Unknown(t *testing.T) {
	a := &configloader.APICallAuth{Type: "oauth2"}
	_, err := NewTokenProvider(a, configloader.KubernetesConfig{}, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported authorization type")
}

// TestNewTokenProvider_Nil verifies that a nil auth config returns an error.
func TestNewTokenProvider_Nil(t *testing.T) {
	_, err := NewTokenProvider(nil, configloader.KubernetesConfig{}, map[string]any{})
	require.Error(t, err)
}

// TestKubernetesProvider_MountedToken_MissingFile verifies the error when
// the mounted SA token file is absent (expected outside a real cluster).
func TestKubernetesProvider_MountedToken_MissingFile(t *testing.T) {
	p := &KubernetesProvider{audience: ""} // no audience → reads mounted file
	_, err := p.GetToken(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read mounted ServiceAccount token")
}

// TestKubernetesProvider_TokenRequest verifies the TokenRequest API path
// using a fake Kubernetes clientset.
func TestKubernetesProvider_TokenRequest(t *testing.T) {
	const (
		namespace      = "hyperfleet"
		serviceAccount = "adapter-sa"
		audience       = "target-service"
		wantToken      = "test-bound-sa-token"
	)

	fakeCS := fake.NewClientset()
	fakeCS.PrependReactor("create", "serviceaccounts",
		func(action ktesting.Action) (bool, runtime.Object, error) {
			createAction, ok := action.(ktesting.CreateAction)
			if !ok || createAction.GetSubresource() != "token" {
				return false, nil, nil
			}
			return true, &authv1.TokenRequest{
				Status: authv1.TokenRequestStatus{Token: wantToken},
			}, nil
		},
	)

	p := &KubernetesProvider{
		audience:       audience,
		namespace:      namespace,
		serviceAccount: serviceAccount,
		expSeconds:     1800,
	}

	token, err := p.createTokenRequestWithClientset(context.Background(), fakeCS)
	require.NoError(t, err)
	assert.Equal(t, wantToken, token)
}

// TestResolveNamespace verifies namespace resolution.
func TestResolveNamespace(t *testing.T) {
	t.Run("configured namespace returned directly", func(t *testing.T) {
		ns, err := resolveNamespace("my-namespace")
		require.NoError(t, err)
		assert.Equal(t, "my-namespace", ns)
	})

	t.Run("empty namespace reads mounted file — fails outside cluster", func(t *testing.T) {
		_, err := resolveNamespace("")
		require.Error(t, err)
		assert.Contains(t, err.Error(), mountedNamespacePath)
	})
}
