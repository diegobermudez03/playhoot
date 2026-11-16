// Package analyticsingestion delivers the analytics-event tasks domains leave
// in their own queues to Analytics' event store (flow ingest-analytics-events).
package analyticsingestion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/analytics"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

// eventIngester is Analytics' ingestion.
type eventIngester interface {
	IngestEvents(ctx context.Context, sourceDomain string, events []analytics.Event) (analytics.IngestResult, error)
}

// Payload is one event of an analytics-event task as a domain enqueues it.
// A task carries one Payload or an array of them. The idempotency key is
// unique within the enqueuing domain.
type Payload struct {
	EventType      string         `json:"event_type"`
	UserRef        string         `json:"user_ref"`
	AccountKind    string         `json:"account_kind"`
	PlanKind       string         `json:"plan_kind"`
	EntityRef      string         `json:"entity_ref"`
	Properties     map[string]any `json:"properties"`
	IdempotencyKey string         `json:"idempotency_key"`
	OccurredAt     time.Time      `json:"occurred_at"`
}

// Ingestion is the analytics-event delivery.
type Ingestion struct {
	events eventIngester
}

// New constructs an Ingestion into Analytics' event store.
func New(events eventIngester) *Ingestion {
	return &Ingestion{events: events}
}

// Register handles analytics-event tasks on each queue; the queue's domain is
// the event's source, whatever the payload says.
func (i *Ingestion) Register(queues ...*txqueue.Queue) {
	for _, queue := range queues {
		source := queue.Domain()
		queue.HandleDelivery(txqueue.TypeAnalyticsEvent, func(ctx context.Context, task txqueue.Task) error {
			return i.Deliver(ctx, source, task.Payload)
		})
	}
}

// Deliver stores the events of one payload from source. The valid events are
// stored even when others in the task are not; a task with an unreadable or
// invalid event then turns dead at once, an admin alert and never a silent
// loss. Retrying a stored event is harmless.
func (i *Ingestion) Deliver(ctx context.Context, source string, raw json.RawMessage) error {
	payloads, err := decode(raw)
	if err != nil {
		return txqueue.Permanent(fmt.Errorf("reading analytics event from %s: %w", source, err))
	}
	events := make([]analytics.Event, len(payloads))
	for n, p := range payloads {
		events[n] = analytics.Event{
			EventType: p.EventType, UserRef: p.UserRef, AccountKind: p.AccountKind, PlanKind: p.PlanKind,
			EntityRef: p.EntityRef, Properties: p.Properties, IdempotencyKey: p.IdempotencyKey, OccurredAt: p.OccurredAt,
		}
	}
	result, err := i.events.IngestEvents(ctx, source, events)
	if err != nil {
		return err
	}
	if result.Invalid > 0 {
		return txqueue.Permanent(fmt.Errorf("%d invalid analytics events from %s", result.Invalid, source))
	}
	return nil
}

func decode(raw json.RawMessage) ([]Payload, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var batch []Payload
		return batch, json.Unmarshal(trimmed, &batch)
	}
	var one Payload
	if err := json.Unmarshal(trimmed, &one); err != nil {
		return nil, err
	}
	return []Payload{one}, nil
}
