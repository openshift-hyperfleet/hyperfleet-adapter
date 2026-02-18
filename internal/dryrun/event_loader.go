package dryrun

import (
	"encoding/json"
	"fmt"
	"os"

	cloudevents "github.com/cloudevents/sdk-go/v2/event"
)

// LoadCloudEvent reads a CloudEvent from a JSON file in standard CloudEvents
// JSON format and returns the parsed event.
func LoadCloudEvent(path string) (*cloudevents.Event, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read event file %q: %w", path, err)
	}

	var evt cloudevents.Event
	if err := json.Unmarshal(data, &evt); err != nil {
		return nil, fmt.Errorf("failed to parse CloudEvent from %q: %w", path, err)
	}

	// Basic validation
	if err := evt.Validate(); err != nil {
		return nil, fmt.Errorf("invalid CloudEvent in %q: %w", path, err)
	}

	return &evt, nil
}
