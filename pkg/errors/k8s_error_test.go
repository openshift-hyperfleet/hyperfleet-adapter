package errors

import (
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Helper function to create K8s API errors for testing
func newStatusError(status metav1.Status) *apierrors.StatusError {
	return &apierrors.StatusError{ErrStatus: status}
}

func TestIsRetryableDiscoveryError_Nil(t *testing.T) {
	assert.False(t, IsRetryableDiscoveryError(nil))
}

func TestIsRetryableDiscoveryError_RetryableK8sErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "Timeout error",
			err: newStatusError(metav1.Status{
				Reason: metav1.StatusReasonTimeout,
				Code:   408,
			}),
		},
		{
			name: "ServerTimeout error",
			err: newStatusError(metav1.Status{
				Reason: metav1.StatusReasonServerTimeout,
				Code:   504,
			}),
		},
		{
			name: "ServiceUnavailable error",
			err: newStatusError(metav1.Status{
				Reason: metav1.StatusReasonServiceUnavailable,
				Code:   503,
			}),
		},
		{
			name: "InternalError",
			err: newStatusError(metav1.Status{
				Reason: metav1.StatusReasonInternalError,
				Code:   500,
			}),
		},
		{
			name: "TooManyRequests error",
			err: newStatusError(metav1.Status{
				Reason: metav1.StatusReasonTooManyRequests,
				Code:   429,
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, IsRetryableDiscoveryError(tt.err),
				"Expected %s to be retryable", tt.name)
		})
	}
}

func TestIsRetryableDiscoveryError_NonRetryableK8sErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "Forbidden error",
			err: newStatusError(metav1.Status{
				Reason: metav1.StatusReasonForbidden,
				Code:   403,
			}),
		},
		{
			name: "Unauthorized error",
			err: newStatusError(metav1.Status{
				Reason: metav1.StatusReasonUnauthorized,
				Code:   401,
			}),
		},
		{
			name: "BadRequest error",
			err: newStatusError(metav1.Status{
				Reason: metav1.StatusReasonBadRequest,
				Code:   400,
			}),
		},
		{
			name: "Invalid error",
			err: newStatusError(metav1.Status{
				Reason: metav1.StatusReasonInvalid,
				Code:   422,
			}),
		},
		{
			name: "Gone error",
			err: newStatusError(metav1.Status{
				Reason: metav1.StatusReasonGone,
				Code:   410,
			}),
		},
		{
			name: "MethodNotAllowed error",
			err: newStatusError(metav1.Status{
				Reason: metav1.StatusReasonMethodNotAllowed,
				Code:   405,
			}),
		},
		{
			name: "NotAcceptable error",
			err: newStatusError(metav1.Status{
				Reason: metav1.StatusReasonNotAcceptable,
				Code:   406,
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.False(t, IsRetryableDiscoveryError(tt.err),
				"Expected %s to NOT be retryable (fatal error)", tt.name)
		})
	}
}

func TestIsRetryableDiscoveryError_NetworkErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "Connection refused",
			err:  syscall.ECONNREFUSED,
		},
		{
			name: "Connection reset",
			err:  syscall.ECONNRESET,
		},
		{
			name: "Connection timed out",
			err:  syscall.ETIMEDOUT,
		},
		{
			name: "Network unreachable",
			err:  syscall.ENETUNREACH,
		},
		{
			name: "Broken pipe",
			err:  syscall.EPIPE,
		},
		{
			name: "Wrapped connection refused",
			err: &net.OpError{
				Op:  "dial",
				Net: "tcp",
				Err: &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, IsRetryableDiscoveryError(tt.err),
				"Expected network error %s to be retryable", tt.name)
		})
	}
}

func TestIsRetryableDiscoveryError_UnknownErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "Generic error",
			err:  errors.New("some unknown error"),
		},
		{
			name: "Wrapped generic error",
			err:  fmt.Errorf("wrapped: %w", errors.New("inner error")),
		},
		{
			name: "Custom error type",
			err:  &customError{msg: "custom error"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Unknown errors should NOT be retryable to surface issues early
			assert.False(t, IsRetryableDiscoveryError(tt.err),
				"Expected unknown error to NOT be retryable (fail fast)")
		})
	}
}

func TestIsRetryableDiscoveryError_NotFoundError(t *testing.T) {
	// NotFound is a special case - it's not an error in discovery context
	// It just means the resource doesn't exist yet
	notFoundErr := newStatusError(metav1.Status{
		Reason: metav1.StatusReasonNotFound,
		Code:   404,
	})

	// NotFound should NOT be retryable (it's not transient, resource just doesn't exist)
	// The caller should handle NotFound separately
	assert.False(t, IsRetryableDiscoveryError(notFoundErr),
		"NotFound should not be retryable - it's a definitive answer")
}

func TestIsRetryableDiscoveryError_WrappedK8sErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name: "Wrapped timeout error",
			err: fmt.Errorf("discovery failed: %w", newStatusError(metav1.Status{
				Reason: metav1.StatusReasonTimeout,
				Code:   408,
			})),
			expected: true,
		},
		{
			name: "Wrapped forbidden error",
			err: fmt.Errorf("discovery failed: %w", newStatusError(metav1.Status{
				Reason: metav1.StatusReasonForbidden,
				Code:   403,
			})),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryableDiscoveryError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsRetryableDiscoveryError_RetryBehaviorMatrix(t *testing.T) {
	// This test documents the expected retry behavior for different error types
	// It serves as documentation for operators and developers

	type testCase struct {
		name           string
		err            error
		shouldRetry    bool
		expectedAction string
	}

	tests := []testCase{
		// Transient errors - SHOULD RETRY
		{
			name:           "API Server overloaded (429)",
			err:            newStatusError(metav1.Status{Reason: metav1.StatusReasonTooManyRequests, Code: 429}),
			shouldRetry:    true,
			expectedAction: "Wait and retry - server is rate limiting",
		},
		{
			name:           "API Server timeout (408)",
			err:            newStatusError(metav1.Status{Reason: metav1.StatusReasonTimeout, Code: 408}),
			shouldRetry:    true,
			expectedAction: "Retry - request timed out",
		},
		{
			name:           "Service unavailable (503)",
			err:            newStatusError(metav1.Status{Reason: metav1.StatusReasonServiceUnavailable, Code: 503}),
			shouldRetry:    true,
			expectedAction: "Retry - API server temporarily unavailable",
		},
		{
			name:           "Internal server error (500)",
			err:            newStatusError(metav1.Status{Reason: metav1.StatusReasonInternalError, Code: 500}),
			shouldRetry:    true,
			expectedAction: "Retry - transient server error",
		},
		{
			name:           "Network connection refused",
			err:            syscall.ECONNREFUSED,
			shouldRetry:    true,
			expectedAction: "Retry - network connectivity issue",
		},

		// Fatal errors - SHOULD NOT RETRY
		{
			name:           "Permission denied (403)",
			err:            newStatusError(metav1.Status{Reason: metav1.StatusReasonForbidden, Code: 403}),
			shouldRetry:    false,
			expectedAction: "FAIL FAST - fix RBAC permissions",
		},
		{
			name:           "Authentication failed (401)",
			err:            newStatusError(metav1.Status{Reason: metav1.StatusReasonUnauthorized, Code: 401}),
			shouldRetry:    false,
			expectedAction: "FAIL FAST - fix authentication",
		},
		{
			name:           "Invalid request (400)",
			err:            newStatusError(metav1.Status{Reason: metav1.StatusReasonBadRequest, Code: 400}),
			shouldRetry:    false,
			expectedAction: "FAIL FAST - fix request format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryableDiscoveryError(tt.err)
			assert.Equal(t, tt.shouldRetry, result,
				"Error: %s\nExpected action: %s", tt.name, tt.expectedAction)
		})
	}
}

// customError is a test helper for unknown error types
type customError struct {
	msg string
}

func (e *customError) Error() string {
	return e.msg
}

// TestK8sOperationError tests the K8sOperationError type
func TestK8sOperationError(t *testing.T) {
	t.Run("error with namespace", func(t *testing.T) {
		err := &K8sOperationError{
			Operation: "create",
			Resource:  "my-pod",
			Kind:      "Pod",
			Namespace: "default",
			Message:   "already exists",
		}

		assert.Contains(t, err.Error(), "create")
		assert.Contains(t, err.Error(), "Pod")
		assert.Contains(t, err.Error(), "my-pod")
		assert.Contains(t, err.Error(), "default")
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("error without namespace", func(t *testing.T) {
		err := &K8sOperationError{
			Operation: "delete",
			Resource:  "my-namespace",
			Kind:      "Namespace",
			Namespace: "",
			Message:   "not found",
		}

		errStr := err.Error()
		assert.Contains(t, errStr, "delete")
		assert.Contains(t, errStr, "Namespace")
		assert.NotContains(t, errStr, "(namespace:")
	})

	t.Run("unwrap underlying error", func(t *testing.T) {
		underlyingErr := errors.New("connection refused")
		err := &K8sOperationError{
			Operation: "get",
			Resource:  "my-pod",
			Kind:      "Pod",
			Message:   "failed",
			Err:       underlyingErr,
		}

		assert.True(t, errors.Is(err, underlyingErr))
	})
}

func TestIsK8sOperationError(t *testing.T) {
	t.Run("returns true for K8sOperationError", func(t *testing.T) {
		err := &K8sOperationError{
			Operation: "create",
			Resource:  "test",
			Kind:      "Pod",
			Message:   "failed",
		}

		k8sErr, ok := IsK8sOperationError(err)
		assert.True(t, ok)
		assert.NotNil(t, k8sErr)
		assert.Equal(t, "create", k8sErr.Operation)
	})

	t.Run("returns true for wrapped K8sOperationError", func(t *testing.T) {
		innerErr := &K8sOperationError{
			Operation: "update",
			Resource:  "test",
			Kind:      "ConfigMap",
			Message:   "conflict",
		}
		wrappedErr := fmt.Errorf("operation failed: %w", innerErr)

		k8sErr, ok := IsK8sOperationError(wrappedErr)
		assert.True(t, ok)
		assert.NotNil(t, k8sErr)
		assert.Equal(t, "update", k8sErr.Operation)
	})

	t.Run("returns false for other errors", func(t *testing.T) {
		err := errors.New("some error")

		k8sErr, ok := IsK8sOperationError(err)
		assert.False(t, ok)
		assert.Nil(t, k8sErr)
	})
}
