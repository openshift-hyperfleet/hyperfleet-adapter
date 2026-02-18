package dryrun

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeEventFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	err := os.WriteFile(p, []byte(content), 0644)
	require.NoError(t, err)
	return p
}

func TestLoadCloudEvent_ValidFile(t *testing.T) {
	t.Run("loads a valid CloudEvent from file", func(t *testing.T) {
		dir := t.TempDir()
		path := writeEventFile(t, dir, "event.json", `{"specversion":"1.0","id":"test-123","type":"com.example.test","source":"/test"}`)

		evt, err := LoadCloudEvent(path)

		require.NoError(t, err)
		assert.Equal(t, "test-123", evt.ID())
		assert.Equal(t, "com.example.test", evt.Type())
		assert.Equal(t, "/test", evt.Source())
	})
}

func TestLoadCloudEvent_FileNotFound(t *testing.T) {
	t.Run("returns error when file does not exist", func(t *testing.T) {
		_, err := LoadCloudEvent("/nonexistent/path/event.json")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read")
	})
}

func TestLoadCloudEvent_InvalidJSON(t *testing.T) {
	t.Run("returns error for invalid JSON", func(t *testing.T) {
		dir := t.TempDir()
		path := writeEventFile(t, dir, "bad.json", `{not json}`)

		_, err := LoadCloudEvent(path)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse")
	})
}
