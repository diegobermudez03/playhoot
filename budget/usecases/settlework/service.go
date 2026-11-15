// Package settlework closes admitted work: the measured cost enters the
// spend record and the reservation is released, once. A periodic task
// releases reservations whose work window ended without settlement.
package settlework

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/budget"
	"github.com/diegobermudez03/playhoot/budget/internal/business/controls"
	internalrepo "github.com/diegobermudez03/playhoot/budget/internal/repo"
	"github.com/diegobermudez03/playhoot/budget/internal/snapshot"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

// SweepTaskType is the expired-reservation sweep on Budget's queue, run
// every SweepEvery; sweepBatch bounds one statement.
const (
	SweepTaskType = "sweep-expired-reservations"
	SweepEvery    = time.Minute
	sweepBatch    = 500
)

// sourceDomain is the source of the measured events settlement records;
// their key is derived from the reservation, so settling twice records once.
const sourceDomain = "budget"

var (
	settleLabel = metrics.NewLabel("outcome",
		string(budget.Settled), string(budget.AlreadyReleased), string(budget.UnknownReservation))
	settlementsTotal = metrics.NewCounter(
		"budget_settlements_total",
		"Reservations settled by what was found.",
		settleLabel,
	)
	estimateLabel  = metrics.NewLabel("estimate", "under", "exact", "over")
	estimatesTotal = metrics.NewCounter(
		"budget_estimate_accuracy_total",
		"Settled reservations by whether the measured cost was under, equal to or over the estimate.",
		estimateLabel,
	)
	expiredTotal = metrics.NewCounter(
		"budget_reservations_expired_total",
		"Reservations released because their work window ended unsettled.",
	)
)

// Service is the settlement and the expiry sweep.
type Service struct {
	repo *internalrepo.Repo
	now  func() time.Time
}

// New constructs a Service on db.
func New(db *gorm.DB) *Service {
	return &Service{repo: internalrepo.New(db), now: time.Now}
}

// Register makes the expiry sweep a periodic task of Budget's queue.
func (s *Service) Register(queue *txqueue.Queue) {
	queue.HandlePeriodic(SweepTaskType, SweepEvery, func(ctx context.Context, _ txqueue.Task) error {
		_, err := s.SweepExpired(ctx)
		return err
	})
}

// SettleWork records settlement's measured cost against the reservation and
// releases it. Settling again changes nothing; a reservation already
// released by expiry stays released, but what was spent is still recorded.
func (s *Service) SettleWork(ctx context.Context, reservationUUID string, settlement budget.Settlement) (budget.SettleOutcome, error) {
	defer logging.Step(ctx, "budget.settle_work").Close()
	if _, err := uuid.Parse(reservationUUID); err != nil {
		return budget.UnknownReservation, nil
	}
	if settlement.Quantity < 0 || math.IsNaN(settlement.Quantity) || math.IsInf(settlement.Quantity, 0) ||
		len(settlement.Service) > controls.MaxServiceLen || (settlement.Quantity > 0 && settlement.Service == "") {
		return "", fmt.Errorf("invalid settlement for reservation %s", reservationUUID)
	}
	outcome, err := utils.RunInDBTransaction(ctx, s.repo, func(ctx context.Context, tx *gorm.DB) (budget.SettleOutcome, error) {
		reservation, err := s.repo.ReservationForUpdate(ctx, tx, reservationUUID)
		if err != nil {
			return "", err
		}
		if reservation == nil {
			return budget.UnknownReservation, nil
		}
		now := s.now().UTC()
		measuredUSD := 0.0
		if settlement.Quantity > 0 {
			rate, err := s.repo.RateFor(ctx, tx, reservation.Category, settlement.Service, now)
			if err != nil {
				return "", err
			}
			event := internalrepo.SpendEvent{
				SourceDomain: sourceDomain, Category: reservation.Category, Service: settlement.Service,
				Reference: reservation.Reference, Quantity: settlement.Quantity, Measured: true,
				IdempotencyKey: "settle:" + reservationUUID, OccurredAt: now, CreatedAt: now,
			}
			if rate == nil {
				event.RateMissing, event.Unit = true, "unknown"
			} else {
				event.Unit, event.USD = rate.Unit, settlement.Quantity*rate.USDPerUnit
				measuredUSD = event.USD
			}
			if _, err := s.repo.InsertSpendEvent(ctx, tx, event); err != nil {
				return "", err
			}
		}
		released, err := s.repo.ReleaseReservation(ctx, tx, reservationUUID, measuredUSD, now)
		if err != nil {
			return "", err
		}
		if !released {
			return budget.AlreadyReleased, nil
		}
		estimatesTotal.Inc(ctx, estimateLabel.Value(accuracy(reservation.USDReserved, measuredUSD)))
		return budget.Settled, nil
	})
	if err != nil {
		return "", err
	}
	settlementsTotal.Inc(ctx, settleLabel.Value(string(outcome)))
	logging.LogFields(ctx, logging.Field("budget_settlement", string(outcome)))
	return outcome, nil
}

// SweepExpired releases every reservation whose work window ended without
// settlement, answering how many. Running it again releases nothing new.
func (s *Service) SweepExpired(ctx context.Context) (int64, error) {
	defer logging.Step(ctx, "budget.sweep_expired_reservations").Close()
	now := s.now().UTC()
	var total int64
	for {
		released, err := s.repo.ReleaseExpired(ctx, s.repo.GetDB(), snapshot.CategoryNames(), now, sweepBatch)
		if err != nil {
			return total, err
		}
		total += released
		if released < sweepBatch {
			break
		}
	}
	if total > 0 {
		expiredTotal.Add(ctx, float64(total))
	}
	logging.LogFields(ctx, logging.Field("budget_reservations_expired", total))
	return total, nil
}

func accuracy(estimated, measured float64) string {
	switch {
	case measured > estimated:
		return "over"
	case measured < estimated:
		return "under"
	}
	return "exact"
}
