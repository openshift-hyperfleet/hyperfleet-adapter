package errors

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockNetError implements net.Error for testing
type mockNetError struct {
	timeout   bool
	temporary bool
	msg       string
}

func (e *mockNetError) Error() string   { return e.msg }
func (e *mockNetError) Timeout() bool   { return e.timeout }
func (e *mockNetError) Temporary() bool { return e.temporary }

func TestIsNetworkError_Nil(t *testing.T) {
	assert.False(t, IsNetworkError(nil))
}

func TestIsNetworkError_SyscallErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "ECONNREFUSED - connection refused",
			err:      syscall.ECONNREFUSED,
			expected: true,
		},
		{
			name:     "ECONNRESET - connection reset",
			err:      syscall.ECONNRESET,
			expected: true,
		},
		{
			name:     "ETIMEDOUT - connection timed out",
			err:      syscall.ETIMEDOUT,
			expected: true,
		},
		{
			name:     "ENETUNREACH - network unreachable",
			err:      syscall.ENETUNREACH,
			expected: true,
		},
		{
			name:     "EHOSTUNREACH - no route to host",
			err:      syscall.EHOSTUNREACH,
			expected: true,
		},
		{
			name:     "ECONNABORTED - connection aborted",
			err:      syscall.ECONNABORTED,
			expected: true,
		},
		{
			name:     "EPIPE - broken pipe",
			err:      syscall.EPIPE,
			expected: true,
		},
		{
			name:     "ENOENT - not a network error",
			err:      syscall.ENOENT,
			expected: false,
		},
		{
			name:     "EACCES - not a network error",
			err:      syscall.EACCES,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNetworkError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsNetworkError_WrappedSyscallErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "wrapped ECONNREFUSED",
			err:      fmt.Errorf("operation failed: %w", syscall.ECONNREFUSED),
			expected: true,
		},
		{
			name:     "double wrapped ECONNRESET",
			err:      fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", syscall.ECONNRESET)),
			expected: true,
		},
		{
			name:     "os.SyscallError wrapping ETIMEDOUT",
			err:      &os.SyscallError{Syscall: "connect", Err: syscall.ETIMEDOUT},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNetworkError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsNetworkError_NetOpError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name: "OpError with ECONNREFUSED",
			err: &net.OpError{
				Op:  "dial",
				Net: "tcp",
				Err: &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED},
			},
			expected: true,
		},
		{
			name: "OpError with ECONNRESET",
			err: &net.OpError{
				Op:  "read",
				Net: "tcp",
				Err: &os.SyscallError{Syscall: "read", Err: syscall.ECONNRESET},
			},
			expected: true,
		},
		{
			name: "OpError with timeout",
			err: &net.OpError{
				Op:  "dial",
				Net: "tcp",
				Err: &mockNetError{timeout: true, msg: "i/o timeout"},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNetworkError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsNetworkError_URLError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name: "URL error with ECONNREFUSED",
			err: &url.Error{
				Op:  "Get",
				URL: "http://localhost:9999",
				Err: &net.OpError{
					Op:  "dial",
					Net: "tcp",
					Err: &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED},
				},
			},
			expected: true,
		},
		{
			name: "URL error with timeout",
			err: &url.Error{
				Op:  "Get",
				URL: "http://example.com",
				Err: &mockNetError{timeout: true, msg: "timeout"},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNetworkError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsNetworkError_TimeoutErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "net.Error with timeout",
			err:      &mockNetError{timeout: true, msg: "connection timeout"},
			expected: true,
		},
		{
			name:     "net.Error without timeout",
			err:      &mockNetError{timeout: false, msg: "some error"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNetworkError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsNetworkError_EOFErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "io.EOF",
			err:      io.EOF,
			expected: true,
		},
		{
			name:     "io.ErrUnexpectedEOF",
			err:      io.ErrUnexpectedEOF,
			expected: true,
		},
		{
			// Note: utilnet.IsProbableEOF checks error message, not wrapped errors
			// Wrapped EOF is not detected by utilnet as it checks the error directly
			name:     "wrapped io.EOF - not detected by utilnet",
			err:      fmt.Errorf("read failed: %w", io.EOF),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNetworkError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsNetworkError_NonNetworkErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "simple error",
			err:  errors.New("some error"),
		},
		{
			name: "file not found",
			err:  os.ErrNotExist,
		},
		{
			name: "permission denied error",
			err:  os.ErrPermission,
		},
		{
			name: "custom error",
			err:  fmt.Errorf("custom error: %s", "details"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.False(t, IsNetworkError(tt.err), "Expected %v to NOT be a network error", tt.err)
		})
	}
}

func TestIsNetworkError_RealWorldScenarios(t *testing.T) {
	t.Run("simulated connection refused", func(t *testing.T) {
		// This simulates what happens when connecting to a closed port
		err := &net.OpError{
			Op:  "dial",
			Net: "tcp",
			Addr: &net.TCPAddr{
				IP:   net.ParseIP("127.0.0.1"),
				Port: 59999,
			},
			Err: &os.SyscallError{
				Syscall: "connect",
				Err:     syscall.ECONNREFUSED,
			},
		}
		assert.True(t, IsNetworkError(err))
	})

	t.Run("simulated connection reset by peer", func(t *testing.T) {
		err := &net.OpError{
			Op:  "read",
			Net: "tcp",
			Err: &os.SyscallError{
				Syscall: "read",
				Err:     syscall.ECONNRESET,
			},
		}
		assert.True(t, IsNetworkError(err))
	})

	t.Run("simulated DNS lookup failure", func(t *testing.T) {
		// DNS errors are typically wrapped in net.DNSError
		err := &net.DNSError{
			Err:        "no such host",
			Name:       "nonexistent.invalid",
			IsNotFound: true,
		}
		// DNS errors are not timeout errors, so should return false
		// unless they have a specific network cause
		assert.False(t, IsNetworkError(err))
	})
}
