package dryrun

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeOverrideFile writes content to a file in dir and returns the full path.
func writeOverrideFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	err := os.WriteFile(p, []byte(content), 0644)
	require.NoError(t, err)
	return p
}

func TestLoadDiscoveryOverrides_ValidFile(t *testing.T) {
	t.Run("single entry with apiVersion and kind", func(t *testing.T) {
		dir := t.TempDir()
		path := writeOverrideFile(t, dir, "overrides.json", `{
			"my-resource": {
				"apiVersion": "v1",
				"kind": "ConfigMap",
				"metadata": {"name": "my-resource"}
			}
		}`)

		overrides, err := LoadDiscoveryOverrides(path)

		require.NoError(t, err)
		assert.Len(t, overrides, 1)
		assert.Equal(t, "v1", overrides["my-resource"]["apiVersion"])
		assert.Equal(t, "ConfigMap", overrides["my-resource"]["kind"])
	})
}

func TestLoadDiscoveryOverrides_FileNotFound(t *testing.T) {
	t.Run("non-existent file returns read error", func(t *testing.T) {
		_, err := LoadDiscoveryOverrides("/no/such/file.json")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read")
	})
}

func TestLoadDiscoveryOverrides_MissingAPIVersion(t *testing.T) {
	t.Run("entry missing apiVersion returns validation error", func(t *testing.T) {
		dir := t.TempDir()
		path := writeOverrideFile(t, dir, "overrides.json", `{
			"bad-resource": {
				"kind": "ConfigMap"
			}
		}`)

		_, err := LoadDiscoveryOverrides(path)

		require.Error(t, err)
		assert.Contains(t, err.Error(), `missing required field "apiVersion"`)
	})
}

func TestLoadDiscoveryOverrides_MissingKind(t *testing.T) {
	t.Run("entry missing kind returns validation error", func(t *testing.T) {
		dir := t.TempDir()
		path := writeOverrideFile(t, dir, "overrides.json", `{
			"bad-resource": {
				"apiVersion": "v1"
			}
		}`)

		_, err := LoadDiscoveryOverrides(path)

		require.Error(t, err)
		assert.Contains(t, err.Error(), `missing required field "kind"`)
	})
}
