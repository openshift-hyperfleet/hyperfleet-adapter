package dryrun

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sync"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/hyperfleet_api"
)

// RequestRecord stores details of an API request made through the dryrun client.
type RequestRecord struct {
	Method     string
	URL        string
	Headers    map[string]string
	Body       []byte
	StatusCode int
	Response   []byte
}

// DryrunAPIClient implements hyperfleet_api.Client backed by file-defined dryrun responses.
// It matches requests by HTTP method and URL regex pattern, returning responses
// sequentially from a configured array per endpoint. All requests are recorded.
type DryrunAPIClient struct {
	endpoints []compiledEndpoint
	mu        sync.Mutex
	Requests  []RequestRecord
}

type compiledEndpoint struct {
	method  string
	pattern *regexp.Regexp
	resps   []DryrunResponse
	callIdx int
}

// NewDryrunAPIClient creates a DryrunAPIClient from a DryrunResponsesFile.
// If mrf is nil, a default client that returns 200 OK for all requests is created.
func NewDryrunAPIClient(mrf *DryrunResponsesFile) (*DryrunAPIClient, error) {
	client := &DryrunAPIClient{
		Requests: make([]RequestRecord, 0),
	}

	if mrf == nil {
		return client, nil
	}

	for i, ep := range mrf.Responses {
		compiled, err := regexp.Compile(ep.Match.URLPattern)
		if err != nil {
			return nil, fmt.Errorf("endpoint %d: invalid urlPattern %q: %w", i, ep.Match.URLPattern, err)
		}
		client.endpoints = append(client.endpoints, compiledEndpoint{
			method:  ep.Match.Method,
			pattern: compiled,
			resps:   ep.Responses,
		})
	}

	return client, nil
}

func (c *DryrunAPIClient) findEndpoint(method, url string) *compiledEndpoint {
	for i := range c.endpoints {
		ep := &c.endpoints[i]
		if ep.method != "*" && ep.method != method {
			continue
		}
		if ep.pattern.MatchString(url) {
			return ep
		}
	}
	return nil
}

func (c *DryrunAPIClient) nextResponse(ep *compiledEndpoint) DryrunResponse {
	idx := ep.callIdx
	if idx >= len(ep.resps) {
		idx = len(ep.resps) - 1 // repeat last response
	}
	ep.callIdx++
	return ep.resps[idx]
}

// Do executes a dryrun HTTP request, matching against configured endpoints.
func (c *DryrunAPIClient) Do(ctx context.Context, req *hyperfleet_api.Request) (*hyperfleet_api.Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ep := c.findEndpoint(req.Method, req.URL)

	var statusCode int
	var respBody []byte

	if ep == nil {
		// Default: 200 OK with empty body
		statusCode = http.StatusOK
		respBody = []byte("{}")
	} else {
		dryrunResp := c.nextResponse(ep)
		statusCode = dryrunResp.StatusCode
		if statusCode == 0 {
			statusCode = http.StatusOK
		}

		if dryrunResp.Body != nil {
			var err error
			respBody, err = json.Marshal(dryrunResp.Body)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal dryrun response body: %w", err)
			}
		} else {
			respBody = []byte("{}")
		}
	}

	record := RequestRecord{
		Method:     req.Method,
		URL:        req.URL,
		Headers:    req.Headers,
		Body:       req.Body,
		StatusCode: statusCode,
		Response:   respBody,
	}
	c.Requests = append(c.Requests, record)

	return &hyperfleet_api.Response{
		StatusCode: statusCode,
		Status:     fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode)),
		Body:       respBody,
		Headers:    make(map[string][]string),
		Attempts:   1,
	}, nil
}

// Get performs a dryrun GET request.
func (c *DryrunAPIClient) Get(ctx context.Context, url string, opts ...hyperfleet_api.RequestOption) (*hyperfleet_api.Response, error) {
	req := &hyperfleet_api.Request{Method: http.MethodGet, URL: url}
	for _, opt := range opts {
		opt(req)
	}
	return c.Do(ctx, req)
}

// Post performs a dryrun POST request.
func (c *DryrunAPIClient) Post(ctx context.Context, url string, body []byte, opts ...hyperfleet_api.RequestOption) (*hyperfleet_api.Response, error) {
	req := &hyperfleet_api.Request{Method: http.MethodPost, URL: url, Body: body}
	for _, opt := range opts {
		opt(req)
	}
	return c.Do(ctx, req)
}

// Put performs a dryrun PUT request.
func (c *DryrunAPIClient) Put(ctx context.Context, url string, body []byte, opts ...hyperfleet_api.RequestOption) (*hyperfleet_api.Response, error) {
	req := &hyperfleet_api.Request{Method: http.MethodPut, URL: url, Body: body}
	for _, opt := range opts {
		opt(req)
	}
	return c.Do(ctx, req)
}

// Patch performs a dryrun PATCH request.
func (c *DryrunAPIClient) Patch(ctx context.Context, url string, body []byte, opts ...hyperfleet_api.RequestOption) (*hyperfleet_api.Response, error) {
	req := &hyperfleet_api.Request{Method: http.MethodPatch, URL: url, Body: body}
	for _, opt := range opts {
		opt(req)
	}
	return c.Do(ctx, req)
}

// Delete performs a dryrun DELETE request.
func (c *DryrunAPIClient) Delete(ctx context.Context, url string, opts ...hyperfleet_api.RequestOption) (*hyperfleet_api.Response, error) {
	req := &hyperfleet_api.Request{Method: http.MethodDelete, URL: url}
	for _, opt := range opts {
		opt(req)
	}
	return c.Do(ctx, req)
}

// BaseURL returns a placeholder base URL for the dryrun client.
func (c *DryrunAPIClient) BaseURL() string {
	return "http://mock-api"
}
