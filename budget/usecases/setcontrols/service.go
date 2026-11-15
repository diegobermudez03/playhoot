// Package setcontrols is the administrator's change of caps and rates:
// rows appended with effective date, actor and reason, old ones kept as
// history. It is reached only through a flow that spent a step-up proof.
package setcontrols

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/budget"
	"github.com/diegobermudez03/playhoot/budget/internal/business/controls"
	internalrepo "github.com/diegobermudez03/playhoot/budget/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
)

// pastTolerance absorbs the clock difference between admin and server;
// maxEntries bounds one change.
const (
	pastTolerance = time.Minute
	maxEntries    = 50
)

var (
	outcomeLabel = metrics.NewLabel("outcome", string(budget.ControlsRecorded), string(budget.ControlsInvalid))
	changesTotal = metrics.NewCounter(
		"budget_control_changes_total",
		"Admin changes of caps and rates by what was decided.",
		outcomeLabel,
	)
)

// Service is the controls change.
type Service struct {
	repo *internalrepo.Repo
	now  func() time.Time
}

// New constructs a Service on db.
func New(db *gorm.DB) *Service {
	return &Service{repo: internalrepo.New(db), now: time.Now}
}

// AdminSetControls appends the caps and rates of change by adminUUID in one
// transaction, effective now when EffectiveAt is zero. Caps are manual:
// nothing raises them from revenue.
func (s *Service) AdminSetControls(ctx context.Context, adminUUID string, change budget.Controls) (budget.SetControlsOutcome, error) {
	defer logging.Step(ctx, "budget.admin_set_controls").Close()
	if _, err := uuid.Parse(adminUUID); err != nil {
		return "", fmt.Errorf("invalid admin uuid: %w", err)
	}
	now := s.now().UTC()
	effectiveAt := change.EffectiveAt.UTC()
	if change.EffectiveAt.IsZero() {
		effectiveAt = now
	}
	if !valid(change, effectiveAt, now) {
		changesTotal.Inc(ctx, outcomeLabel.Value(string(budget.ControlsInvalid)))
		return budget.ControlsInvalid, nil
	}
	caps := make([]internalrepo.Cap, len(change.Caps))
	for i, c := range change.Caps {
		caps[i] = internalrepo.Cap{
			Scope: c.Scope, CapWindow: string(c.Window), USDLimit: c.USDLimit, MaxConcurrency: optional(c.MaxConcurrency),
			MaxVolume: optional(c.MaxVolume), EffectiveAt: effectiveAt, CreatedByUserUUID: &adminUUID, Reason: change.Reason, CreatedAt: now,
		}
	}
	rates := make([]internalrepo.Rate, len(change.Rates))
	for i, r := range change.Rates {
		rates[i] = internalrepo.Rate{
			Category: string(r.Category), Service: r.Service, Unit: r.Unit, USDPerUnit: r.USDPerUnit,
			EffectiveAt: effectiveAt, CreatedByUserUUID: &adminUUID, Reason: change.Reason, CreatedAt: now,
		}
	}
	err := s.repo.GetDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.repo.InsertControls(ctx, tx, caps, rates)
	})
	if err != nil {
		return "", err
	}
	changesTotal.Inc(ctx, outcomeLabel.Value(string(budget.ControlsRecorded)))
	logging.LogFields(ctx, logging.Field("budget_caps_changed", len(caps)), logging.Field("budget_rates_changed", len(rates)))
	return budget.ControlsRecorded, nil
}

func valid(change budget.Controls, effectiveAt, now time.Time) bool {
	entries := len(change.Caps) + len(change.Rates)
	if entries == 0 || entries > maxEntries || change.Reason == "" || len(change.Reason) > controls.MaxReasonLen || effectiveAt.Before(now.Add(-pastTolerance)) {
		return false
	}
	for _, c := range change.Caps {
		if !controls.ValidCap(c) {
			return false
		}
	}
	for _, r := range change.Rates {
		if !controls.ValidRate(r) {
			return false
		}
	}
	return true
}

func optional(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}
