// Package recordspend ingests the spend events domains report: each is
// stored once, priced with the rate registered for its service, and the
// ladder evaluation is asked to run after it.
package recordspend

import (
	"context"
	"math"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/budget"
	"github.com/diegobermudez03/playhoot/budget/internal/business/controls"
	internalrepo "github.com/diegobermudez03/playhoot/budget/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils"
)

// EvaluateAfterSpendTask is the ladder evaluation a recorded spend asks for;
// a burst of events collapses into one pending task.
const EvaluateAfterSpendTask = "evaluate-ladder-after-spend"

// maxKeyLen bounds identifiers coming from other domains.
const maxKeyLen = 200

var (
	outcomeLabel = metrics.NewLabel("outcome",
		string(budget.SpendRecorded), string(budget.SpendRateMissing), string(budget.SpendDuplicate), string(budget.SpendInvalid))
	spendEventsTotal = metrics.NewCounter(
		"budget_spend_events_total",
		"Spend events ingested by what was decided.",
		outcomeLabel,
	)
)

// enqueuer leaves a task in Budget's queue inside a transaction.
type enqueuer interface {
	Enqueue(tx *gorm.DB, taskType string, payload any, runAt time.Time, idempotencyKey string) error
}

// Service is the spend ingestion.
type Service struct {
	repo  *internalrepo.Repo
	queue enqueuer
	now   func() time.Time
}

// New constructs a Service on db that asks queue for the evaluation.
func New(db *gorm.DB, queue enqueuer) *Service {
	return &Service{repo: internalrepo.New(db), queue: queue, now: time.Now}
}

// RecordSpend stores event once per source domain and idempotency key. With
// no rate for its service and unit, it is kept flagged and unpriced, and its
// category refuses admission until the rate exists.
func (s *Service) RecordSpend(ctx context.Context, event budget.SpendEvent) (budget.RecordSpendOutcome, error) {
	defer logging.Step(ctx, "budget.record_spend").Close()
	if !valid(event) {
		spendEventsTotal.Inc(ctx, outcomeLabel.Value(string(budget.SpendInvalid)))
		return budget.SpendInvalid, nil
	}
	now := s.now().UTC()
	if event.OccurredAt.IsZero() || event.OccurredAt.After(now) {
		event.OccurredAt = now
	}
	outcome, err := utils.RunInDBTransaction(ctx, s.repo, func(ctx context.Context, tx *gorm.DB) (budget.RecordSpendOutcome, error) {
		rate, err := s.repo.RateFor(ctx, tx, string(event.Category), event.Service, event.OccurredAt)
		if err != nil {
			return "", err
		}
		row := internalrepo.SpendEvent{
			SourceDomain: event.SourceDomain, Category: string(event.Category), Service: event.Service, Reference: event.Reference,
			Quantity: event.Quantity, Unit: event.Unit, Measured: event.Measured, IdempotencyKey: event.IdempotencyKey,
			OccurredAt: event.OccurredAt.UTC(), CreatedAt: now,
		}
		outcome := budget.SpendRecorded
		if rate == nil || rate.Unit != event.Unit {
			row.RateMissing = true
			outcome = budget.SpendRateMissing
		} else {
			row.USD = event.Quantity * rate.USDPerUnit
		}
		inserted, err := s.repo.InsertSpendEvent(ctx, tx, row)
		if err != nil {
			return "", err
		}
		if !inserted {
			return budget.SpendDuplicate, nil
		}
		if err := s.queue.Enqueue(tx, EvaluateAfterSpendTask, struct{}{}, time.Time{}, EvaluateAfterSpendTask); err != nil {
			return "", err
		}
		return outcome, nil
	})
	if err != nil {
		return "", err
	}
	spendEventsTotal.Inc(ctx, outcomeLabel.Value(string(outcome)))
	logging.LogFields(ctx, logging.Field("spend_event", string(outcome)), logging.Field("spend_category", string(event.Category)))
	return outcome, nil
}

func valid(e budget.SpendEvent) bool {
	return budget.ValidCategory(e.Category) &&
		e.SourceDomain != "" && len(e.SourceDomain) <= maxKeyLen &&
		e.IdempotencyKey != "" && len(e.IdempotencyKey) <= maxKeyLen &&
		e.Service != "" && len(e.Service) <= controls.MaxServiceLen &&
		e.Unit != "" && len(e.Unit) <= controls.MaxServiceLen &&
		len(e.Reference) <= maxKeyLen &&
		e.Quantity >= 0 && !math.IsNaN(e.Quantity) && !math.IsInf(e.Quantity, 0)
}
