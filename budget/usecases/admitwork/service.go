// Package admitwork admits or refuses cost-bearing work. Under the budget
// lock it applies the ladder, prices the estimate, checks every window of
// the category and of global keeping a margin of each remainder free, and
// reserves the priced estimate.
package admitwork

import (
	"context"
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
)

// Bounds of a request: how long admitted work may hold its reservation, and
// the length of its reference.
const (
	DefaultMaxDuration = 15 * time.Minute
	MaxMaxDuration     = 24 * time.Hour
	maxReferenceLen    = 200
)

var (
	decisionLabel = metrics.NewLabel("decision",
		string(budget.Admitted), string(budget.RefusedShutdown), string(budget.RefusedPaused), string(budget.RefusedRestricted),
		string(budget.RefusedNoControl), string(budget.RefusedCapReached), string(budget.RefusedConcurrency),
		string(budget.RefusedVolume), string(budget.RefusedInvalidRequest))
	admissionsTotal = metrics.NewCounter(
		"budget_admissions_total",
		"Admission decisions on cost-bearing work.",
		decisionLabel,
	)
)

// Service is the admission.
type Service struct {
	repo    *internalrepo.Repo
	now     func() time.Time
	newUUID func() string
}

// New constructs a Service on db.
func New(db *gorm.DB) *Service {
	return &Service{repo: internalrepo.New(db), now: time.Now, newUUID: uuid.NewString}
}

// AdmitWork admits request when the ladder lets its category through, its
// service has a rate, and the priced estimate fits every window of the
// category and of global after spend, reservations and the margin. A
// missing cap or rate, or a zero cap, refuses: never unlimited.
func (s *Service) AdmitWork(ctx context.Context, request budget.WorkRequest) (budget.Admission, error) {
	defer logging.Step(ctx, "budget.admit_work").Close()
	if !valid(request) {
		return s.decide(ctx, request, budget.Admission{Outcome: budget.Refused, Reason: budget.RefusedInvalidRequest}), nil
	}
	if request.MaxDuration == 0 {
		request.MaxDuration = DefaultMaxDuration
	}
	admission, err := utils.RunInDBTransaction(ctx, s.repo, func(ctx context.Context, tx *gorm.DB) (budget.Admission, error) {
		if err := s.repo.Lock(ctx, tx); err != nil {
			return budget.Admission{}, err
		}
		now := s.now().UTC()
		snap, err := snapshot.Load(ctx, s.repo, tx, now)
		if err != nil {
			return budget.Admission{}, err
		}
		if reason, ok := controls.Gate(snap.Ladder, request.Category, request.Discretionary); !ok {
			return refused(reason), nil
		}
		for _, missing := range snap.RateMissing {
			if missing == request.Category {
				return refused(budget.RefusedNoControl), nil
			}
		}
		rate, err := s.repo.RateFor(ctx, tx, string(request.Category), request.Service, now)
		if err != nil {
			return budget.Admission{}, err
		}
		if rate == nil {
			return refused(budget.RefusedNoControl), nil
		}
		cost := request.Quantity * rate.USDPerUnit
		if reason, ok := controls.Admit(cost, snap.Categories[request.Category], snap.CategoryCap[request.Category]); !ok {
			return refused(reason), nil
		}
		if reason, ok := controls.Admit(cost, snap.Global, snap.GlobalCaps); !ok {
			return refused(reason), nil
		}
		reservationUUID := s.newUUID()
		err = s.repo.InsertReservation(ctx, tx, internalrepo.Reservation{
			ReservationUUID: reservationUUID, Category: string(request.Category), Reference: request.Reference,
			USDReserved: cost, ExpiresAt: now.Add(request.MaxDuration), CreatedAt: now,
		})
		if err != nil {
			return budget.Admission{}, err
		}
		return budget.Admission{Outcome: budget.Admitted, ReservationUUID: reservationUUID}, nil
	})
	if err != nil {
		return budget.Admission{}, err
	}
	return s.decide(ctx, request, admission), nil
}

func (s *Service) decide(ctx context.Context, request budget.WorkRequest, admission budget.Admission) budget.Admission {
	decision := string(admission.Reason)
	if admission.Outcome == budget.Admitted {
		decision = string(budget.Admitted)
	} else {
		admission.Outcome = budget.Refused
		admission.Message = budget.RestrictionMessage
	}
	admissionsTotal.Inc(ctx, decisionLabel.Value(decision))
	logging.LogFields(ctx, logging.Field("budget_admission", decision), logging.Field("budget_category", string(request.Category)))
	return admission
}

func refused(reason budget.RefusalReason) budget.Admission {
	return budget.Admission{Outcome: budget.Refused, Reason: reason}
}

func valid(r budget.WorkRequest) bool {
	return budget.ValidCategory(r.Category) && r.Service != "" && len(r.Service) <= controls.MaxServiceLen &&
		len(r.Reference) <= maxReferenceLen && r.Quantity >= 0 && !math.IsNaN(r.Quantity) && !math.IsInf(r.Quantity, 0) &&
		r.MaxDuration >= 0 && r.MaxDuration <= MaxMaxDuration
}
