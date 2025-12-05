package k8s_client

import (
	"errors"
	"fmt"
)

// K8sOperationError represents a structured Kubernetes operation error with detailed context.
// This allows callers to handle K8s errors with full information about what failed.
type K8sOperationError struct {
	// Operation is the operation that failed: "create", "update", "delete", "get", "patch", "list"
	Operation string
	// Resource is the resource name
	Resource string
	// Kind is the Kubernetes resource kind
	Kind string
	// Namespace is the resource namespace
	Namespace string
	// Message is the error message
	Message string
	// Err is the underlying error
	Err error
}

// Error implements the error interface
func (e *K8sOperationError) Error() string {
	if e.Namespace != "" {
		return fmt.Sprintf("K8s %s operation failed: %s/%s (namespace: %s): %s",
			e.Operation, e.Kind, e.Resource, e.Namespace, e.Message)
	}
	return fmt.Sprintf("K8s %s operation failed: %s/%s: %s",
		e.Operation, e.Kind, e.Resource, e.Message)
}

// Unwrap returns the underlying error for errors.Is/As support
func (e *K8sOperationError) Unwrap() error {
	return e.Err
}


// IsK8sOperationError checks if an error is a K8sOperationError and returns it.
// This function supports wrapped errors.
func IsK8sOperationError(err error) (*K8sOperationError, bool) {
	var k8sErr *K8sOperationError
	if errors.As(err, &k8sErr) {
		return k8sErr, true
	}
	return nil, false
}

