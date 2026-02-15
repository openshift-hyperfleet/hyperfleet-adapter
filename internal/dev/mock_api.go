package dev

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sync"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/hyperfleet_api"
	"gopkg.in/yaml.v3"
)

// MockAPIClient implements hyperfleet_api.Client for dry-run mode.
// It records all API calls and returns configurable mock responses.
type MockAPIClient struct {
	mu sync.Mutex
	// Requests records all API requests made
	Requests []APIRequest
	// CallRecords records all API calls with their responses
	CallRecords []APICallRecord
	// Responses maps URL patterns to mock responses
	Responses map[string]*MockResponse
	// DefaultResponse is returned when no matching pattern is found
	DefaultResponse *MockResponse
	// baseURL is the mock base URL
	baseURL string
}

// APIRequest represents a recorded API request
type APIRequest struct {
	Method  string
	URL     string
	Body    []byte
	Headers map[string]string
}

// APICallRecord represents a recorded API request with its response
type APICallRecord struct {
	Request  APIRequest
	Response *MockResponse
}

// MockResponse defines a mock response for API calls
type MockResponse struct {
	StatusCode int               `yaml:"statusCode" json:"statusCode"`
	Body       interface{}       `yaml:"body" json:"body"`
	Headers    map[string]string `yaml:"headers" json:"headers"`
}

// NewMockAPIClient creates a new MockAPIClient
func NewMockAPIClient() *MockAPIClient {
	return &MockAPIClient{
		Requests:    make([]APIRequest, 0),
		CallRecords: make([]APICallRecord, 0),
		Responses:   make(map[string]*MockResponse),
		DefaultResponse: &MockResponse{
			StatusCode: 200,
			Body:       map[string]interface{}{"status": "ok"},
			Headers:    map[string]string{"Content-Type": "application/json"},
		},
		baseURL: "https://api.example.com",
	}
}

// SetResponse configures a mock response for a URL pattern
func (c *MockAPIClient) SetResponse(urlPattern string, resp *MockResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Responses[urlPattern] = resp
}

// LoadResponsesFromFile loads mock responses from a YAML or JSON file
func (c *MockAPIClient) LoadResponsesFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read mock responses file: %w", err)
	}

	var responses map[string]*MockResponse

	// Try YAML first, then JSON
	if err := yaml.Unmarshal(data, &responses); err != nil {
		if err := json.Unmarshal(data, &responses); err != nil {
			return fmt.Errorf("failed to parse mock responses file: %w", err)
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	for pattern, resp := range responses {
		c.Responses[pattern] = resp
	}

	return nil
}

// findResponse finds a matching response for a URL
func (c *MockAPIClient) findResponse(url string) *MockResponse {
	// Try exact match first
	if resp, exists := c.Responses[url]; exists {
		return resp
	}

	// Try pattern matching
	for pattern, resp := range c.Responses {
		if matched, _ := regexp.MatchString(pattern, url); matched {
			return resp
		}
	}

	return c.DefaultResponse
}

// Do executes an HTTP request and returns a mock response
func (c *MockAPIClient) Do(ctx context.Context, req *hyperfleet_api.Request) (*hyperfleet_api.Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Record the request
	apiReq := APIRequest{
		Method:  req.Method,
		URL:     req.URL,
		Body:    req.Body,
		Headers: req.Headers,
	}
	c.Requests = append(c.Requests, apiReq)

	// Find matching response
	mockResp := c.findResponse(req.URL)

	// Record the call with its response
	c.CallRecords = append(c.CallRecords, APICallRecord{
		Request:  apiReq,
		Response: mockResp,
	})

	// Convert body to JSON bytes
	var bodyBytes []byte
	if mockResp.Body != nil {
		var err error
		bodyBytes, err = json.Marshal(mockResp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal mock response body: %w", err)
		}
	}

	// Build response headers
	headers := make(map[string][]string)
	for k, v := range mockResp.Headers {
		headers[k] = []string{v}
	}

	return &hyperfleet_api.Response{
		StatusCode: mockResp.StatusCode,
		Status:     fmt.Sprintf("%d OK", mockResp.StatusCode),
		Headers:    headers,
		Body:       bodyBytes,
		Attempts:   1,
	}, nil
}

// Get performs a GET request
func (c *MockAPIClient) Get(ctx context.Context, url string, opts ...hyperfleet_api.RequestOption) (*hyperfleet_api.Response, error) {
	req := &hyperfleet_api.Request{
		Method: "GET",
		URL:    url,
	}
	for _, opt := range opts {
		opt(req)
	}
	return c.Do(ctx, req)
}

// Post performs a POST request
func (c *MockAPIClient) Post(ctx context.Context, url string, body []byte, opts ...hyperfleet_api.RequestOption) (*hyperfleet_api.Response, error) {
	req := &hyperfleet_api.Request{
		Method: "POST",
		URL:    url,
		Body:   body,
	}
	for _, opt := range opts {
		opt(req)
	}
	return c.Do(ctx, req)
}

// Put performs a PUT request
func (c *MockAPIClient) Put(ctx context.Context, url string, body []byte, opts ...hyperfleet_api.RequestOption) (*hyperfleet_api.Response, error) {
	req := &hyperfleet_api.Request{
		Method: "PUT",
		URL:    url,
		Body:   body,
	}
	for _, opt := range opts {
		opt(req)
	}
	return c.Do(ctx, req)
}

// Patch performs a PATCH request
func (c *MockAPIClient) Patch(ctx context.Context, url string, body []byte, opts ...hyperfleet_api.RequestOption) (*hyperfleet_api.Response, error) {
	req := &hyperfleet_api.Request{
		Method: "PATCH",
		URL:    url,
		Body:   body,
	}
	for _, opt := range opts {
		opt(req)
	}
	return c.Do(ctx, req)
}

// Delete performs a DELETE request
func (c *MockAPIClient) Delete(ctx context.Context, url string, opts ...hyperfleet_api.RequestOption) (*hyperfleet_api.Response, error) {
	req := &hyperfleet_api.Request{
		Method: "DELETE",
		URL:    url,
	}
	for _, opt := range opts {
		opt(req)
	}
	return c.Do(ctx, req)
}

// BaseURL returns the mock base URL
func (c *MockAPIClient) BaseURL() string {
	return c.baseURL
}

// GetRequests returns all recorded requests
func (c *MockAPIClient) GetRequests() []APIRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]APIRequest{}, c.Requests...)
}

// GetCallRecords returns all recorded API calls with their responses
func (c *MockAPIClient) GetCallRecords() []APICallRecord {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]APICallRecord{}, c.CallRecords...)
}

// Ensure MockAPIClient implements Client
var _ hyperfleet_api.Client = (*MockAPIClient)(nil)
