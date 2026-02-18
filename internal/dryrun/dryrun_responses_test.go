package dryrun

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDryrunResponses_ValidFile(t *testing.T) {
	t.Run("minimal valid file", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "responses.json")

		content := `{
  "responses": [
    {
      "match": {
        "method": "GET",
        "urlPattern": "/api/v1/things/.*"
      },
      "responses": [
        {
          "statusCode": 200,
          "headers": {"Content-Type": "application/json"},
          "body": {"id": "thing-1", "name": "Thing One"}
        }
      ]
    },
    {
      "match": {
        "method": "POST",
        "urlPattern": "/api/v1/actions"
      },
      "responses": [
        {
          "statusCode": 201,
          "body": {"status": "created"}
        },
        {
          "statusCode": 409,
          "body": {"error": "conflict"}
        }
      ]
    }
  ]
}`
		err := os.WriteFile(filePath, []byte(content), 0644)
		require.NoError(t, err)

		result, err := LoadDryrunResponses(filePath)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Len(t, result.Responses, 2)

		// First endpoint
		assert.Equal(t, "GET", result.Responses[0].Match.Method)
		assert.Equal(t, "/api/v1/things/.*", result.Responses[0].Match.URLPattern)
		assert.Len(t, result.Responses[0].Responses, 1)
		assert.Equal(t, 200, result.Responses[0].Responses[0].StatusCode)
		assert.Equal(t, "application/json", result.Responses[0].Responses[0].Headers["Content-Type"])
		require.NotNil(t, result.Responses[0].Responses[0].Body)

		// Second endpoint
		assert.Equal(t, "POST", result.Responses[1].Match.Method)
		assert.Equal(t, "/api/v1/actions", result.Responses[1].Match.URLPattern)
		assert.Len(t, result.Responses[1].Responses, 2)
		assert.Equal(t, 201, result.Responses[1].Responses[0].StatusCode)
		assert.Equal(t, 409, result.Responses[1].Responses[1].StatusCode)
	})

	t.Run("real testdata file", func(t *testing.T) {
		testdataPath := filepath.Join("..", "..", "test", "testdata", "dryrun", "dryrun-api-responses.json")
		if _, err := os.Stat(testdataPath); os.IsNotExist(err) {
			t.Skipf("testdata file not found at %s", testdataPath)
		}

		result, err := LoadDryrunResponses(testdataPath)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Greater(t, len(result.Responses), 0, "expected at least one endpoint in testdata file")

		for i, ep := range result.Responses {
			assert.NotEmpty(t, ep.Match.URLPattern, "endpoint %d should have a urlPattern", i)
			assert.NotEmpty(t, ep.Responses, "endpoint %d should have at least one response", i)
		}
	})
}

func TestLoadDryrunResponses_FileNotFound(t *testing.T) {
	_, err := LoadDryrunResponses("/nonexistent/path/responses.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read")
}

func TestLoadDryrunResponses_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "bad.json")

	err := os.WriteFile(filePath, []byte(`{not valid json}`), 0644)
	require.NoError(t, err)

	_, err = LoadDryrunResponses(filePath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}

func TestLoadDryrunResponses_NoResponses(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "no-responses.json")

	content := `{
  "responses": [
    {
      "match": {
        "method": "GET",
        "urlPattern": "/api/v1/things"
      },
      "responses": []
    }
  ]
}`
	err := os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err)

	_, err = LoadDryrunResponses(filePath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no responses defined")
}

func TestLoadDryrunResponses_EmptyURLPattern(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "empty-pattern.json")

	content := `{
  "responses": [
    {
      "match": {
        "method": "GET",
        "urlPattern": ""
      },
      "responses": [
        {
          "statusCode": 200
        }
      ]
    }
  ]
}`
	err := os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err)

	_, err = LoadDryrunResponses(filePath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty urlPattern")
}
