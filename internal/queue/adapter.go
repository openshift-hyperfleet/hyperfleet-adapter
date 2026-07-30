package queue

import (
	"encoding/json"

	"github.com/cloudevents/sdk-go/v2/event"
)

func QueueMessageToCloudEvent(msg *QueueMessage) (*event.Event, error) {
	e := event.New()
	e.SetID(msg.ID)
	e.SetType(msg.EventType)
	e.SetSource("hyperfleet-sentinel")
	e.SetTime(msg.CreatedAt)

	data := map[string]interface{}{
		"id":         msg.ResourceID,
		"kind":       msg.Kind,
		"href":       msg.Href,
		"generation": msg.Generation,
	}
	if msg.OwnerReferences != nil {
		var ownerRef map[string]interface{}
		if err := json.Unmarshal(msg.OwnerReferences, &ownerRef); err == nil {
			data["owner_references"] = ownerRef
		}
	}
	if err := e.SetData(event.ApplicationJSON, data); err != nil {
		return nil, err
	}
	return &e, nil
}
