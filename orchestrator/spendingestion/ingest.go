// Package spendingestion delivers the spend-event tasks domains leave in
// their own queues to Budget's spend record (flow ingest-spend-events).
package spendingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/budget"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

// spendRecorder is Budget's ingestion.
type spendRecorder interface {
	RecordSpend(ctx context.Context, event budget.SpendEvent) (budget.RecordSpendOutcome, error)
}

// Payload is a spend-event task as a domain enqueues it. Quantity is in
// Unit of Service; Reference names the work, so a measured event replaces
// the estimate with the same reference.
type Payload struct {
	Category       string    `json:"category"`
	Service        string    `json:"service"`
	Reference      string    `json:"reference"`
	Quantity       float64   `json:"quantity"`
	Unit           string    `json:"unit"`
	Measured       bool      `json:"measured"`
	IdempotencyKey string    `json:"idempotency_key"`
	OccurredAt     time.Time `json:"occurred_at"`
}

// Ingestion is the spend-event delivery.
type Ingestion struct {
	spend spendRecorder
}

// New constructs an Ingestion into Budget's spend record.
func New(spend spendRecorder) *Ingestion {
	return &Ingestion{spend: spend}
}

// Register handles spend-event tasks on each queue; the queue's domain is
// the event's source, whatever the payload says.
func (i *Ingestion) Register(queues ...*txqueue.Queue) {
	for _, queue := range queues {
		source := queue.Domain()
		queue.HandleDelivery(txqueue.TypeSpendEvent, func(ctx context.Context, task txqueue.Task) error {
			return i.Deliver(ctx, source, task.Payload)
		})
	}
}

// Deliver records one spend-event payload from source. A payload Budget
// cannot read turns the task dead at once: an admin alert, never a silent
// loss.
func (i *Ingestion) Deliver(ctx context.Context, source string, raw json.RawMessage) error {
	var p Payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return txqueue.Permanent(fmt.Errorf("reading spend event from %s: %w", source, err))
	}
	outcome, err := i.spend.RecordSpend(ctx, budget.SpendEvent{
		SourceDomain: source, Category: budget.Category(p.Category), Service: p.Service, Reference: p.Reference,
		Quantity: p.Quantity, Unit: p.Unit, Measured: p.Measured, IdempotencyKey: p.IdempotencyKey, OccurredAt: p.OccurredAt,
	})
	if err != nil {
		return err
	}
	if outcome == budget.SpendInvalid {
		return txqueue.Permanent(fmt.Errorf("invalid spend event from %s with key %q", source, p.IdempotencyKey))
	}
	return nil
}
