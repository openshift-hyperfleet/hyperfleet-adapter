package executor

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/hyperfleet_api"
	apierrors "github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestValidateAPIResponse_NilError_SuccessResponse(t *testing.T) {
	resp := &hyperfleet_api.Response{
		StatusCode: 200,
		Status:     "200 OK",
		Body:       []byte(`{"status":"ok"}`),
		Attempts:   1,
		Duration:   100 * time.Millisecond,
	}

	err := ValidateAPIResponse(resp, nil, "GET", "http://example.com/api")

	assert.NoError(t, err)
}

func TestValidateAPIResponse_NilError_NilResponse(t *testing.T) {
	err := ValidateAPIResponse(nil, nil, "GET", "http://example.com/api")

	require.Error(t, err)

	// Should be wrapped as APIError
	apiErr, ok := apierrors.IsAPIError(err)
	require.True(t, ok, "Expected error to be APIError")
	assert.Equal(t, "GET", apiErr.Method)
	assert.Equal(t, "http://example.com/api", apiErr.URL)
	assert.Equal(t, 0, apiErr.StatusCode)
	assert.Contains(t, apiErr.Error(), "nil")
}

func TestValidateAPIResponse_WithError_AlreadyAPIError(t *testing.T) {
	// If error is already an APIError, it should be returned as-is
	originalErr := apierrors.NewAPIError(
		"POST",
		"http://example.com/api/create",
		503,
		"503 Service Unavailable",
		[]byte("service down"),
		3,
		5*time.Second,
		errors.New("connection refused"),
	)

	err := ValidateAPIResponse(nil, originalErr, "GET", "http://other.com")

	require.Error(t, err)

	// Should be the same error, not re-wrapped
	apiErr, ok := apierrors.IsAPIError(err)
	require.True(t, ok)
	assert.Equal(t, "POST", apiErr.Method) // Original method preserved
	assert.Equal(t, "http://example.com/api/create", apiErr.URL)
	assert.Equal(t, 503, apiErr.StatusCode)
}

func TestValidateAPIResponse_WithError_NonAPIError(t *testing.T) {
	// Non-APIError should be wrapped
	originalErr := errors.New("network timeout")

	err := ValidateAPIResponse(nil, originalErr, "PUT", "http://example.com/api/update")

	require.Error(t, err)

	apiErr, ok := apierrors.IsAPIError(err)
	require.True(t, ok, "Expected error to be wrapped as APIError")
	assert.Equal(t, "PUT", apiErr.Method)
	assert.Equal(t, "http://example.com/api/update", apiErr.URL)
	assert.Equal(t, 0, apiErr.StatusCode) // No status code for network errors
	assert.True(t, errors.Is(err, originalErr), "Original error should be unwrappable")
}

func TestValidateAPIResponse_NonSuccessStatusCodes(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		status      string
		body        []byte
		expectError bool
		expectBody  bool
	}{
		{
			name:        "400 Bad Request",
			statusCode:  400,
			status:      "400 Bad Request",
			body:        []byte(`{"error":"invalid input"}`),
			expectError: true,
			expectBody:  true,
		},
		{
			name:        "401 Unauthorized",
			statusCode:  401,
			status:      "401 Unauthorized",
			body:        []byte(`{"error":"invalid token"}`),
			expectError: true,
			expectBody:  true,
		},
		{
			name:        "403 Forbidden",
			statusCode:  403,
			status:      "403 Forbidden",
			body:        nil,
			expectError: true,
			expectBody:  false,
		},
		{
			name:        "404 Not Found",
			statusCode:  404,
			status:      "404 Not Found",
			body:        []byte(`{"message":"resource not found"}`),
			expectError: true,
			expectBody:  true,
		},
		{
			name:        "429 Too Many Requests",
			statusCode:  429,
			status:      "429 Too Many Requests",
			body:        []byte(`{"retry_after":60}`),
			expectError: true,
			expectBody:  true,
		},
		{
			name:        "500 Internal Server Error",
			statusCode:  500,
			status:      "500 Internal Server Error",
			body:        []byte(`{"error":"internal error"}`),
			expectError: true,
			expectBody:  true,
		},
		{
			name:        "502 Bad Gateway",
			statusCode:  502,
			status:      "502 Bad Gateway",
			body:        nil,
			expectError: true,
			expectBody:  false,
		},
		{
			name:        "503 Service Unavailable",
			statusCode:  503,
			status:      "503 Service Unavailable",
			body:        []byte("service temporarily unavailable"),
			expectError: true,
			expectBody:  true,
		},
		{
			name:        "504 Gateway Timeout",
			statusCode:  504,
			status:      "504 Gateway Timeout",
			body:        nil,
			expectError: true,
			expectBody:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &hyperfleet_api.Response{
				StatusCode: tt.statusCode,
				Status:     tt.status,
				Body:       tt.body,
				Attempts:   1,
				Duration:   50 * time.Millisecond,
			}

			err := ValidateAPIResponse(resp, nil, "GET", "http://example.com/api")

			if tt.expectError {
				require.Error(t, err)

				apiErr, ok := apierrors.IsAPIError(err)
				require.True(t, ok, "Expected error to be APIError")

				assert.Equal(t, tt.statusCode, apiErr.StatusCode)
				assert.Equal(t, tt.status, apiErr.Status)
				assert.Equal(t, "GET", apiErr.Method)
				assert.Equal(t, "http://example.com/api", apiErr.URL)

				if tt.expectBody {
					assert.Equal(t, tt.body, apiErr.ResponseBody)
					assert.Contains(t, apiErr.Error(), string(tt.body))
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateAPIResponse_SuccessStatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		status     string
	}{
		{
			name:       "200 OK",
			statusCode: 200,
			status:     "200 OK",
		},
		{
			name:       "201 Created",
			statusCode: 201,
			status:     "201 Created",
		},
		{
			name:       "202 Accepted",
			statusCode: 202,
			status:     "202 Accepted",
		},
		{
			name:       "204 No Content",
			statusCode: 204,
			status:     "204 No Content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &hyperfleet_api.Response{
				StatusCode: tt.statusCode,
				Status:     tt.status,
				Body:       nil,
				Attempts:   1,
				Duration:   30 * time.Millisecond,
			}

			err := ValidateAPIResponse(resp, nil, "POST", "http://example.com/api/create")

			assert.NoError(t, err)
		})
	}
}

func TestValidateAPIResponse_PreservesAttempts(t *testing.T) {
	resp := &hyperfleet_api.Response{
		StatusCode: 500,
		Status:     "500 Internal Server Error",
		Body:       []byte("error"),
		Attempts:   5,
		Duration:   10 * time.Second,
	}

	err := ValidateAPIResponse(resp, nil, "GET", "http://example.com")

	require.Error(t, err)
	apiErr, ok := apierrors.IsAPIError(err)
	require.True(t, ok)

	assert.Equal(t, 5, apiErr.Attempts)
	assert.Equal(t, 10*time.Second, apiErr.Duration)
}

func TestValidateAPIResponse_AllHTTPMethods(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			resp := &hyperfleet_api.Response{
				StatusCode: 404,
				Status:     "404 Not Found",
			}

			err := ValidateAPIResponse(resp, nil, method, "http://example.com")

			require.Error(t, err)
			apiErr, ok := apierrors.IsAPIError(err)
			require.True(t, ok)
			assert.Equal(t, method, apiErr.Method)
		})
	}
}

func TestValidateAPIResponse_URLPreserved(t *testing.T) {
	urls := []string{
		"http://localhost:8080/api/v1/clusters",
		"https://api.example.com/resources/123",
		"http://service.namespace.svc.cluster.local:9090/health",
		"http://10.0.0.1:3000/path/to/resource?query=value",
	}

	for _, url := range urls {
		t.Run(url, func(t *testing.T) {
			resp := &hyperfleet_api.Response{
				StatusCode: 500,
				Status:     "500 Internal Server Error",
			}

			err := ValidateAPIResponse(resp, nil, "GET", url)

			require.Error(t, err)
			apiErr, ok := apierrors.IsAPIError(err)
			require.True(t, ok)
			assert.Equal(t, url, apiErr.URL)
			assert.Contains(t, apiErr.Error(), url)
		})
	}
}

func TestValidateAPIResponse_WrappedErrorChain(t *testing.T) {
	// Test that error wrapping works correctly for error inspection
	innerErr := fmt.Errorf("connection reset by peer")
	wrappedErr := fmt.Errorf("dial failed: %w", innerErr)

	err := ValidateAPIResponse(nil, wrappedErr, "GET", "http://example.com")

	require.Error(t, err)

	// Should be an APIError
	apiErr, ok := apierrors.IsAPIError(err)
	require.True(t, ok)

	// The underlying error chain should be preserved
	assert.Contains(t, apiErr.Error(), "connection reset")
}

func TestValidateAPIResponse_ErrorMessageContainsContext(t *testing.T) {
	resp := &hyperfleet_api.Response{
		StatusCode: 503,
		Status:     "503 Service Unavailable",
		Body:       []byte(`{"message":"database connection failed","retry_after":30}`),
		Attempts:   3,
		Duration:   9 * time.Second,
	}

	err := ValidateAPIResponse(resp, nil, "POST", "http://api.example.com/clusters")

	require.Error(t, err)

	errMsg := err.Error()
	assert.Contains(t, errMsg, "POST")
	assert.Contains(t, errMsg, "http://api.example.com/clusters")
	assert.Contains(t, errMsg, "503")
	assert.Contains(t, errMsg, "3") // attempts
}

func TestValidateAPIResponse_APIErrorHelpers(t *testing.T) {
	t.Run("IsServerError", func(t *testing.T) {
		resp := &hyperfleet_api.Response{StatusCode: 500, Status: "500 Internal Server Error"}
		err := ValidateAPIResponse(resp, nil, "GET", "http://example.com")

		apiErr, _ := apierrors.IsAPIError(err)
		assert.True(t, apiErr.IsServerError())
		assert.False(t, apiErr.IsClientError())
	})

	t.Run("IsClientError", func(t *testing.T) {
		resp := &hyperfleet_api.Response{StatusCode: 400, Status: "400 Bad Request"}
		err := ValidateAPIResponse(resp, nil, "GET", "http://example.com")

		apiErr, _ := apierrors.IsAPIError(err)
		assert.True(t, apiErr.IsClientError())
		assert.False(t, apiErr.IsServerError())
	})

	t.Run("IsNotFound", func(t *testing.T) {
		resp := &hyperfleet_api.Response{StatusCode: 404, Status: "404 Not Found"}
		err := ValidateAPIResponse(resp, nil, "GET", "http://example.com")

		apiErr, _ := apierrors.IsAPIError(err)
		assert.True(t, apiErr.IsNotFound())
	})

	t.Run("IsUnauthorized", func(t *testing.T) {
		resp := &hyperfleet_api.Response{StatusCode: 401, Status: "401 Unauthorized"}
		err := ValidateAPIResponse(resp, nil, "GET", "http://example.com")

		apiErr, _ := apierrors.IsAPIError(err)
		assert.True(t, apiErr.IsUnauthorized())
	})

	t.Run("IsForbidden", func(t *testing.T) {
		resp := &hyperfleet_api.Response{StatusCode: 403, Status: "403 Forbidden"}
		err := ValidateAPIResponse(resp, nil, "GET", "http://example.com")

		apiErr, _ := apierrors.IsAPIError(err)
		assert.True(t, apiErr.IsForbidden())
	})

	t.Run("IsRateLimited", func(t *testing.T) {
		resp := &hyperfleet_api.Response{StatusCode: 429, Status: "429 Too Many Requests"}
		err := ValidateAPIResponse(resp, nil, "GET", "http://example.com")

		apiErr, _ := apierrors.IsAPIError(err)
		assert.True(t, apiErr.IsRateLimited())
	})

	t.Run("IsBadRequest", func(t *testing.T) {
		resp := &hyperfleet_api.Response{StatusCode: 400, Status: "400 Bad Request"}
		err := ValidateAPIResponse(resp, nil, "GET", "http://example.com")

		apiErr, _ := apierrors.IsAPIError(err)
		assert.True(t, apiErr.IsBadRequest())
	})

	t.Run("IsConflict", func(t *testing.T) {
		resp := &hyperfleet_api.Response{StatusCode: 409, Status: "409 Conflict"}
		err := ValidateAPIResponse(resp, nil, "POST", "http://example.com")

		apiErr, _ := apierrors.IsAPIError(err)
		assert.True(t, apiErr.IsConflict())
	})
}

func TestValidateAPIResponse_ResponseBodyString(t *testing.T) {
	resp := &hyperfleet_api.Response{
		StatusCode: 500,
		Status:     "500 Internal Server Error",
		Body:       []byte(`{"error":"database timeout","code":"DB_TIMEOUT"}`),
	}

	err := ValidateAPIResponse(resp, nil, "GET", "http://example.com")

	apiErr, _ := apierrors.IsAPIError(err)

	assert.True(t, apiErr.HasResponseBody())
	assert.Equal(t, `{"error":"database timeout","code":"DB_TIMEOUT"}`, apiErr.ResponseBodyString())
}

// TestRenderTemplateBytes tests template rendering to bytes
func TestRenderTemplateBytes(t *testing.T) {
	tests := []struct {
		name        string
		template    string
		data        map[string]interface{}
		expected    []byte
		expectError bool
	}{
		{
			name:     "simple template",
			template: "Hello {{ .name }}!",
			data:     map[string]interface{}{"name": "World"},
			expected: []byte("Hello World!"),
		},
		{
			name:     "no template markers",
			template: "plain text",
			data:     map[string]interface{}{},
			expected: []byte("plain text"),
		},
		{
			name:     "JSON body template",
			template: `{"cluster_id": "{{ .clusterId }}", "region": "{{ .region }}"}`,
			data:     map[string]interface{}{"clusterId": "cluster-123", "region": "us-east-1"},
			expected: []byte(`{"cluster_id": "cluster-123", "region": "us-east-1"}`),
		},
		{
			name:        "missing variable error",
			template:    "{{ .missing }}",
			data:        map[string]interface{}{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := renderTemplateBytes(tt.template, tt.data)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestExecutionErrorToMap tests conversion of ExecutionError to map
func TestExecutionErrorToMap(t *testing.T) {
	tests := []struct {
		name     string
		execErr  *ExecutionError
		expected interface{}
	}{
		{
			name:     "nil error",
			execErr:  nil,
			expected: nil,
		},
		{
			name: "error with all fields",
			execErr: &ExecutionError{
				Step:    "check-cluster",
				Message: "Cluster not found",
			},
			expected: map[string]interface{}{
				"step":    "check-cluster",
				"message": "Cluster not found",
			},
		},
		{
			name: "error with empty fields",
			execErr: &ExecutionError{
				Step:    "",
				Message: "",
			},
			expected: map[string]interface{}{
				"step":    "",
				"message": "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := executionErrorToMap(tt.execErr)

			if tt.expected == nil {
				assert.Nil(t, result)
				return
			}

			expectedMap := tt.expected.(map[string]interface{})
			resultMap := result.(map[string]interface{})
			assert.Equal(t, expectedMap["step"], resultMap["step"])
			assert.Equal(t, expectedMap["message"], resultMap["message"])
		})
	}
}

// TestAdapterMetadataToMap tests conversion of AdapterMetadata to map
func TestAdapterMetadataToMap(t *testing.T) {
	tests := []struct {
		name     string
		adapter  *AdapterMetadata
		expected map[string]interface{}
	}{
		{
			name:     "nil adapter",
			adapter:  nil,
			expected: map[string]interface{}{},
		},
		{
			name: "success status",
			adapter: &AdapterMetadata{
				ExecutionStatus: "success",
				ErrorReason:     "",
				ErrorMessage:    "",
				ExecutionError:  nil,
			},
			expected: map[string]interface{}{
				"executionStatus": "success",
				"errorReason":     "",
				"errorMessage":    "",
				"executionError":  nil,
			},
		},
		{
			name: "failed status with error",
			adapter: &AdapterMetadata{
				ExecutionStatus: "failed",
				ErrorReason:     "APIError",
				ErrorMessage:    "API returned 500",
				ExecutionError: &ExecutionError{
					Step:    "fetch-cluster",
					Message: "Connection refused",
				},
			},
			expected: map[string]interface{}{
				"executionStatus": "failed",
				"errorReason":     "APIError",
				"errorMessage":    "API returned 500",
				"executionError": map[string]interface{}{
					"step":    "fetch-cluster",
					"message": "Connection refused",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := adapterMetadataToMap(tt.adapter)

			assert.Equal(t, tt.expected["executionStatus"], result["executionStatus"])
			assert.Equal(t, tt.expected["errorReason"], result["errorReason"])
			assert.Equal(t, tt.expected["errorMessage"], result["errorMessage"])

			if tt.expected["executionError"] == nil {
				assert.Nil(t, result["executionError"])
			} else {
				expectedErr := tt.expected["executionError"].(map[string]interface{})
				resultErr := result["executionError"].(map[string]interface{})
				assert.Equal(t, expectedErr["step"], resultErr["step"])
				assert.Equal(t, expectedErr["message"], resultErr["message"])
			}
		})
	}
}

// TestConvertToStringKeyMap tests map key conversion
func TestConvertToStringKeyMap(t *testing.T) {
	tests := []struct {
		name     string
		input    map[interface{}]interface{}
		expected map[string]interface{}
	}{
		{
			name:     "empty map",
			input:    map[interface{}]interface{}{},
			expected: map[string]interface{}{},
		},
		{
			name: "simple string keys",
			input: map[interface{}]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
			expected: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			name: "integer keys",
			input: map[interface{}]interface{}{
				1: "one",
				2: "two",
			},
			expected: map[string]interface{}{
				"1": "one",
				"2": "two",
			},
		},
		{
			name: "nested map",
			input: map[interface{}]interface{}{
				"outer": map[interface{}]interface{}{
					"inner": "value",
				},
			},
			expected: map[string]interface{}{
				"outer": map[string]interface{}{
					"inner": "value",
				},
			},
		},
		{
			name: "nested slice",
			input: map[interface{}]interface{}{
				"items": []interface{}{"a", "b", "c"},
			},
			expected: map[string]interface{}{
				"items": []interface{}{"a", "b", "c"},
			},
		},
		{
			name: "deeply nested structure",
			input: map[interface{}]interface{}{
				"level1": map[interface{}]interface{}{
					"level2": map[interface{}]interface{}{
						"level3": "deep value",
					},
				},
			},
			expected: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"level3": "deep value",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToStringKeyMap(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestConvertSlice tests slice element conversion
func TestConvertSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    []interface{}
		expected []interface{}
	}{
		{
			name:     "empty slice",
			input:    []interface{}{},
			expected: []interface{}{},
		},
		{
			name:     "simple values",
			input:    []interface{}{"a", "b", "c"},
			expected: []interface{}{"a", "b", "c"},
		},
		{
			name:     "numeric values",
			input:    []interface{}{1, 2, 3},
			expected: []interface{}{1, 2, 3},
		},
		{
			name: "nested maps in slice",
			input: []interface{}{
				map[interface{}]interface{}{"key": "value1"},
				map[interface{}]interface{}{"key": "value2"},
			},
			expected: []interface{}{
				map[string]interface{}{"key": "value1"},
				map[string]interface{}{"key": "value2"},
			},
		},
		{
			name: "nested slices",
			input: []interface{}{
				[]interface{}{"a", "b"},
				[]interface{}{"c", "d"},
			},
			expected: []interface{}{
				[]interface{}{"a", "b"},
				[]interface{}{"c", "d"},
			},
		},
		{
			name: "mixed types",
			input: []interface{}{
				"string",
				123,
				map[interface{}]interface{}{"nested": "map"},
				[]interface{}{"nested", "slice"},
			},
			expected: []interface{}{
				"string",
				123,
				map[string]interface{}{"nested": "map"},
				[]interface{}{"nested", "slice"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertSlice(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestBuildResourcesMap tests building resources map for CEL
func TestBuildResourcesMap(t *testing.T) {
	tests := []struct {
		name      string
		resources map[string]*unstructured.Unstructured
		expected  map[string]interface{}
	}{
		{
			name:      "nil resources",
			resources: nil,
			expected:  map[string]interface{}{},
		},
		{
			name:      "empty resources",
			resources: map[string]*unstructured.Unstructured{},
			expected:  map[string]interface{}{},
		},
		{
			name: "single resource",
			resources: map[string]*unstructured.Unstructured{
				"cluster": {
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name": "test-cluster",
						},
					},
				},
			},
			expected: map[string]interface{}{
				"cluster": map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name": "test-cluster",
					},
				},
			},
		},
		{
			name: "multiple resources",
			resources: map[string]*unstructured.Unstructured{
				"configmap": {
					Object: map[string]interface{}{
						"kind": "ConfigMap",
						"data": map[string]interface{}{"key": "value"},
					},
				},
				"secret": {
					Object: map[string]interface{}{
						"kind": "Secret",
						"data": map[string]interface{}{"password": "encoded"},
					},
				},
			},
			expected: map[string]interface{}{
				"configmap": map[string]interface{}{
					"kind": "ConfigMap",
					"data": map[string]interface{}{"key": "value"},
				},
				"secret": map[string]interface{}{
					"kind": "Secret",
					"data": map[string]interface{}{"password": "encoded"},
				},
			},
		},
		{
			name: "nil resource in map",
			resources: map[string]*unstructured.Unstructured{
				"valid": {
					Object: map[string]interface{}{"kind": "ConfigMap"},
				},
				"nil_resource": nil,
			},
			expected: map[string]interface{}{
				"valid": map[string]interface{}{"kind": "ConfigMap"},
				// nil_resource is not included
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildResourcesMap(tt.resources)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetResourceAsMap tests resource to map conversion
func TestGetResourceAsMap(t *testing.T) {
	tests := []struct {
		name     string
		resource *unstructured.Unstructured
		expected map[string]interface{}
	}{
		{
			name:     "nil resource",
			resource: nil,
			expected: nil,
		},
		{
			name: "simple resource",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"name":      "test-pod",
						"namespace": "default",
					},
				},
			},
			expected: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"name":      "test-pod",
					"namespace": "default",
				},
			},
		},
		{
			name: "resource with status",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind": "Deployment",
					"status": map[string]interface{}{
						"replicas":          3,
						"availableReplicas": 3,
						"conditions": []interface{}{
							map[string]interface{}{"type": "Available", "status": "True"},
						},
					},
				},
			},
			expected: map[string]interface{}{
				"kind": "Deployment",
				"status": map[string]interface{}{
					"replicas":          3,
					"availableReplicas": 3,
					"conditions": []interface{}{
						map[string]interface{}{"type": "Available", "status": "True"},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetResourceAsMap(tt.resource)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestStepResult tests step result creation and methods
func TestStepResult(t *testing.T) {
	t.Run("NewStepResult creates success result", func(t *testing.T) {
		result := NewStepResult("testStep", StepTypeParam, "value")

		assert.Equal(t, "testStep", result.Name)
		assert.Equal(t, StepTypeParam, result.Type)
		assert.Equal(t, "value", result.Result)
		assert.Nil(t, result.Error)
		assert.False(t, result.Skipped)
		assert.True(t, result.IsSuccess())
	})

	t.Run("NewStepResultSkipped creates skipped result", func(t *testing.T) {
		result := NewStepResultSkipped("testStep", StepTypeResource, "when clause false")

		assert.Equal(t, "testStep", result.Name)
		assert.Equal(t, StepTypeResource, result.Type)
		assert.True(t, result.Skipped)
		assert.Equal(t, "when clause false", result.SkipReason)
		assert.False(t, result.IsSuccess())
	})

	t.Run("NewStepResultError creates error result", func(t *testing.T) {
		result := NewStepResultError("testStep", StepTypeAPICall, "APIFailed", "connection refused")

		assert.Equal(t, "testStep", result.Name)
		assert.Equal(t, StepTypeAPICall, result.Type)
		assert.NotNil(t, result.Error)
		assert.Equal(t, "APIFailed", result.Error.Reason)
		assert.Equal(t, "connection refused", result.Error.Message)
		assert.False(t, result.IsSuccess())
	})

	t.Run("ToMap converts result correctly", func(t *testing.T) {
		result := NewStepResult("testStep", StepTypeParam, map[string]interface{}{"key": "value"})
		m := result.ToMap()

		assert.Equal(t, false, m["skipped"])
		assert.NotNil(t, m["result"])
		assert.Nil(t, m["error"])
	})
}

// TestStepExecutionResult tests step execution result management
func TestStepExecutionResult(t *testing.T) {
	t.Run("NewStepExecutionResult creates empty result", func(t *testing.T) {
		result := NewStepExecutionResult()

		assert.Equal(t, StatusSuccess, result.Status)
		assert.Empty(t, result.StepResults)
		assert.Empty(t, result.StepResultsByName)
		assert.False(t, result.HasErrors)
	})

	t.Run("AddStepResult adds and indexes results", func(t *testing.T) {
		result := NewStepExecutionResult()

		step1 := NewStepResult("step1", StepTypeParam, "value1")
		step2 := NewStepResult("step2", StepTypeParam, "value2")

		result.AddStepResult(step1)
		result.AddStepResult(step2)

		assert.Len(t, result.StepResults, 2)
		assert.Equal(t, step1, result.GetStepResult("step1"))
		assert.Equal(t, step2, result.GetStepResult("step2"))
	})

	t.Run("AddStepResult tracks errors", func(t *testing.T) {
		result := NewStepExecutionResult()

		successStep := NewStepResult("success", StepTypeParam, "value")
		errorStep := NewStepResultError("error", StepTypeAPICall, "Failed", "error message")

		result.AddStepResult(successStep)
		assert.False(t, result.HasErrors)

		result.AddStepResult(errorStep)
		assert.True(t, result.HasErrors)
		assert.Equal(t, "Failed", result.FirstError.Reason)
	})

	t.Run("GetStepResult returns nil for unknown step", func(t *testing.T) {
		result := NewStepExecutionResult()

		assert.Nil(t, result.GetStepResult("unknown"))
	})
}
