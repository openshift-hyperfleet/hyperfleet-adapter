package k8s_client

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDiscoveryConfig(t *testing.T) {
	t.Run("single resource discovery", func(t *testing.T) {
		d := &DiscoveryConfig{
			Namespace: "default",
			ByName:    "my-resource",
		}

		assert.Equal(t, "default", d.GetNamespace())
		assert.Equal(t, "my-resource", d.GetName())
		assert.Equal(t, "", d.GetLabelSelector())
		assert.True(t, d.IsSingleResource())
	})

	t.Run("list resource discovery", func(t *testing.T) {
		d := &DiscoveryConfig{
			Namespace:     "kube-system",
			LabelSelector: "app=myapp,env=prod",
		}

		assert.Equal(t, "kube-system", d.GetNamespace())
		assert.Equal(t, "", d.GetName())
		assert.Equal(t, "app=myapp,env=prod", d.GetLabelSelector())
		assert.False(t, d.IsSingleResource())
	})

	t.Run("cluster-scoped discovery", func(t *testing.T) {
		d := &DiscoveryConfig{
			Namespace:     "",
			LabelSelector: "type=cluster",
		}

		// Verify all four interface methods consistently
		assert.Equal(t, "", d.GetNamespace())
		assert.Equal(t, "", d.GetName())
		assert.Equal(t, "type=cluster", d.GetLabelSelector())
		assert.False(t, d.IsSingleResource())
	})
}

func TestBuildLabelSelector(t *testing.T) {
	tests := []struct {
		name         string
		labels       map[string]string
		want         string        // exact match (used when wantContains is nil)
		wantContains []string      // substrings that must be present
		requireComma bool          // whether result must contain a comma separator
	}{
		{
			name:   "empty labels",
			labels: nil,
			want:   "",
		},
		{
			name:   "empty map",
			labels: map[string]string{},
			want:   "",
		},
		{
			name:   "single label",
			labels: map[string]string{"app": "myapp"},
			want:   "app=myapp",
		},
		{
			name: "multiple labels",
			labels: map[string]string{
				"app": "myapp",
				"env": "prod",
			},
			// Map iteration order is not guaranteed, so use contains checks
			wantContains: []string{"app=myapp", "env=prod"},
			requireComma: true,
		},
		{
			name: "three labels",
			labels: map[string]string{
				"app":     "myapp",
				"env":     "prod",
				"version": "v1",
			},
			wantContains: []string{"app=myapp", "env=prod", "version=v1"},
			requireComma: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildLabelSelector(tt.labels)

			if len(tt.wantContains) > 0 {
				// Use contains-based validation for non-deterministic order
				for _, substr := range tt.wantContains {
					assert.Contains(t, got, substr, "selector should contain %q", substr)
				}
				if tt.requireComma {
					assert.Contains(t, got, ",", "selector with multiple labels should contain comma separator")
				}
			} else {
				// Use exact match for deterministic cases
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
