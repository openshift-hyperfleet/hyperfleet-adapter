package hyperfleet_api

import (
	"context"
)

// MockClient implements Client for testing.
// It allows configuring mock responses for each method.
type MockClient struct {
	// BaseURLValue is the value returned by BaseURL()
	BaseURLValue string

	// DoResponse and DoError are returned by Do()
	DoResponse *Response
	DoError    error

	// GetResponse and GetError are returned by Get()
	GetResponse *Response
	GetError    error

	// PostResponse and PostError are returned by Post()
	PostResponse *Response
	PostError    error

	// PutResponse and PutError are returned by Put()
	PutResponse *Response
	PutError    error

	// PatchResponse and PatchError are returned by Patch()
	PatchResponse *Response
	PatchError    error

	// DeleteResponse and DeleteError are returned by Delete()
	DeleteResponse *Response
	DeleteError    error

	// Requests records all requests made to this mock for verification
	Requests []*Request
}

// NewMockClient creates a new mock API client for testing.
// By default, all methods return a 200 OK response.
func NewMockClient() *MockClient {
	defaultResponse := &Response{
		StatusCode: 200,
		Status:     "200 OK",
	}
	return &MockClient{
		BaseURLValue:   "http://mock-api.example.com",
		DoResponse:     defaultResponse,
		GetResponse:    defaultResponse,
		PostResponse:   defaultResponse,
		PutResponse:    defaultResponse,
		PatchResponse:  defaultResponse,
		DeleteResponse: defaultResponse,
		Requests:       make([]*Request, 0),
	}
}

// Do implements Client.Do
func (m *MockClient) Do(ctx context.Context, req *Request) (*Response, error) {
	m.Requests = append(m.Requests, req)
	if m.DoError != nil {
		return nil, m.DoError
	}
	return m.DoResponse, nil
}

// Get implements Client.Get
func (m *MockClient) Get(ctx context.Context, url string, opts ...RequestOption) (*Response, error) {
	req := &Request{Method: "GET", URL: url}
	m.Requests = append(m.Requests, req)
	if m.GetError != nil {
		return nil, m.GetError
	}
	return m.GetResponse, nil
}

// Post implements Client.Post
func (m *MockClient) Post(ctx context.Context, url string, body []byte, opts ...RequestOption) (*Response, error) {
	req := &Request{Method: "POST", URL: url, Body: body}
	m.Requests = append(m.Requests, req)
	if m.PostError != nil {
		return nil, m.PostError
	}
	return m.PostResponse, nil
}

// Put implements Client.Put
func (m *MockClient) Put(ctx context.Context, url string, body []byte, opts ...RequestOption) (*Response, error) {
	req := &Request{Method: "PUT", URL: url, Body: body}
	m.Requests = append(m.Requests, req)
	if m.PutError != nil {
		return nil, m.PutError
	}
	return m.PutResponse, nil
}

// Patch implements Client.Patch
func (m *MockClient) Patch(ctx context.Context, url string, body []byte, opts ...RequestOption) (*Response, error) {
	req := &Request{Method: "PATCH", URL: url, Body: body}
	m.Requests = append(m.Requests, req)
	if m.PatchError != nil {
		return nil, m.PatchError
	}
	return m.PatchResponse, nil
}

// Delete implements Client.Delete
func (m *MockClient) Delete(ctx context.Context, url string, opts ...RequestOption) (*Response, error) {
	req := &Request{Method: "DELETE", URL: url}
	m.Requests = append(m.Requests, req)
	if m.DeleteError != nil {
		return nil, m.DeleteError
	}
	return m.DeleteResponse, nil
}

// BaseURL implements Client.BaseURL
func (m *MockClient) BaseURL() string {
	return m.BaseURLValue
}

// Reset clears all recorded requests
func (m *MockClient) Reset() {
	m.Requests = make([]*Request, 0)
}

// GetLastRequest returns the most recent request, or nil if none
func (m *MockClient) GetLastRequest() *Request {
	if len(m.Requests) == 0 {
		return nil
	}
	return m.Requests[len(m.Requests)-1]
}

// Ensure MockClient implements Client
var _ Client = (*MockClient)(nil)
