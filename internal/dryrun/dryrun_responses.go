package dryrun

import (
	"encoding/json"
	"fmt"
	"os"
)

// DryrunResponsesFile represents the top-level structure of a dryrun API responses JSON file.
type DryrunResponsesFile struct {
	Responses []DryrunEndpoint `json:"responses"`
}

// DryrunEndpoint defines a URL pattern matcher and its sequential responses.
type DryrunEndpoint struct {
	Match     DryrunMatch      `json:"match"`
	Responses []DryrunResponse `json:"responses"`
}

// DryrunMatch defines the HTTP method and URL pattern to match against.
type DryrunMatch struct {
	Method     string `json:"method"`     // HTTP method or "*" for any
	URLPattern string `json:"urlPattern"` // Go regexp
}

// DryrunResponse defines a single dryrun HTTP response.
type DryrunResponse struct {
	StatusCode int               `json:"statusCode"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       interface{}       `json:"body,omitempty"`
}

// LoadDryrunResponses reads and parses a dryrun API responses JSON file.
func LoadDryrunResponses(path string) (*DryrunResponsesFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read dryrun responses file %q: %w", path, err)
	}

	var mrf DryrunResponsesFile
	if err := json.Unmarshal(data, &mrf); err != nil {
		return nil, fmt.Errorf("failed to parse dryrun responses file %q: %w", path, err)
	}

	// Validate each endpoint has at least one response
	for i, ep := range mrf.Responses {
		if len(ep.Responses) == 0 {
			return nil, fmt.Errorf("dryrun responses file %q: endpoint %d has no responses defined", path, i)
		}
		if ep.Match.URLPattern == "" {
			return nil, fmt.Errorf("dryrun responses file %q: endpoint %d has empty urlPattern", path, i)
		}
	}

	return &mrf, nil
}
