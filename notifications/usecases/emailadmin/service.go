// Package emailadmin is the administrator's outbound email panel: the mode,
// consumption and estimated cost per provider, deliveries by state, and
// switching the mode during operation without deploying.
package emailadmin

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/notifications"
	internalrepo "github.com/diegobermudez03/playhoot/notifications/internal/repo"
)

// Delivery records live this long; the status counts the same window.
const retention = 30 * 24 * time.Hour

const maxReasonLen = 500

var (
	modeLabel   = metrics.NewLabel("mode", string(notifications.ModeAutomatic), string(notifications.ModeResendOnly), string(notifications.ModeSESOnly))
	modeChanges = metrics.NewCounter("notifications_email_mode_changes_total", "Outbound email mode changes by the mode chosen.", modeLabel)
)

// Service is the panel.
type Service struct {
	repo      *internalrepo.Repo
	providers []notifications.EmailProvider
	now       func() time.Time
}

// New constructs a Service on db; providers give the reference prices.
func New(db *gorm.DB, providers ...notifications.EmailProvider) *Service {
	return &Service{repo: internalrepo.New(db), providers: providers, now: time.Now}
}

// AdminEmailStatus returns the panel.
func (s *Service) AdminEmailStatus(ctx context.Context) (notifications.EmailStatus, error) {
	defer logging.Step(ctx, "notifications.admin_email_status").Close()
	db := s.repo.GetDB()
	now := s.now().UTC()
	status := notifications.EmailStatus{Mode: notifications.ModeAutomatic, Deliveries: map[notifications.DeliveryState]int{}}
	row, err := s.repo.LatestMode(ctx, db)
	if err != nil {
		return status, err
	}
	if row != nil {
		status.Mode, status.ModeSince, status.ModeReason = notifications.Mode(row.Mode), row.CreatedAt, row.Reason
	}
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	for _, p := range s.providers {
		today, err := s.repo.UsageBetween(ctx, db, string(p.Provider()), day, day.AddDate(0, 0, 1))
		if err != nil {
			return status, err
		}
		month, err := s.repo.UsageBetween(ctx, db, string(p.Provider()), monthStart, monthStart.AddDate(0, 1, 0))
		if err != nil {
			return status, err
		}
		status.Usage = append(status.Usage, notifications.ProviderUsage{Provider: p.Provider(), Today: today, Month: month, EstimatedUSD: float64(month) * p.USDPerEmail()})
	}
	counts, err := s.repo.CountDeliveriesSince(ctx, db, now.Add(-retention))
	if err != nil {
		return status, err
	}
	for _, state := range notifications.DeliveryStates {
		status.Deliveries[state] = 0
	}
	for _, c := range counts {
		status.Deliveries[notifications.DeliveryState(c.State)] = c.Count
	}
	return status, nil
}

// AdminSetEmailMode appends mode with the administrator and reason; the
// current mode again changes nothing.
func (s *Service) AdminSetEmailMode(ctx context.Context, adminUUID string, mode notifications.Mode, reason string) (notifications.SetModeOutcome, error) {
	defer logging.Step(ctx, "notifications.admin_set_email_mode").Close()
	if _, err := uuid.Parse(adminUUID); err != nil || !notifications.ValidMode(mode) || reason == "" || len(reason) > maxReasonLen {
		return notifications.ModeInvalid, nil
	}
	db := s.repo.GetDB()
	current, err := s.repo.LatestMode(ctx, db)
	if err != nil {
		return "", err
	}
	if current != nil && current.Mode == string(mode) {
		return notifications.ModeUnchanged, nil
	}
	if err := s.repo.InsertMode(ctx, db, &internalrepo.ModeChange{Mode: string(mode), ChangedByUserUUID: &adminUUID, Reason: reason, CreatedAt: s.now().UTC()}); err != nil {
		return "", err
	}
	modeChanges.Inc(ctx, modeLabel.Value(string(mode)))
	return notifications.ModeChanged, nil
}
