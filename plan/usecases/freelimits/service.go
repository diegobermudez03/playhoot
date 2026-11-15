// Package freelimits is admin's free-limit configuration: reading what is in
// force, scheduled and superseded, and appending a new entry.
package freelimits

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/plan"
	"github.com/diegobermudez03/playhoot/plan/internal/business/limits"
	internalrepo "github.com/diegobermudez03/playhoot/plan/internal/repo"
)

// pastTolerance absorbs the clock difference between admin and server.
const pastTolerance = time.Minute

var (
	outcomeLabel = metrics.NewLabel("outcome",
		string(plan.FreeLimitsRecorded), string(plan.FreeLimitsInvalid))
	setLimitsTotal = metrics.NewCounter(
		"plan_free_limit_changes_total",
		"Admin changes to the free limits by what was decided.",
		outcomeLabel,
	)
)

// Service is the free-limit admin use case.
type Service struct {
	repo *internalrepo.Repo
	now  func() time.Time
}

// New constructs a Service on db.
func New(db *gorm.DB) *Service {
	return &Service{repo: internalrepo.New(db), now: time.Now}
}

// ReadFreeLimits returns the configuration in force, the ones scheduled and
// the superseded ones, newest first.
func (s *Service) ReadFreeLimits(ctx context.Context) (plan.FreeLimitConfigs, error) {
	defer logging.Step(ctx, "plan.read_free_limits").Close()
	rows, err := s.repo.FreeLimitConfigs(ctx)
	if err != nil {
		return plan.FreeLimitConfigs{}, err
	}
	now := s.now()
	result := plan.FreeLimitConfigs{Scheduled: []plan.FreeLimitConfig{}, History: []plan.FreeLimitConfig{}}
	foundCurrent := false
	for _, row := range rows {
		config := toConfig(row)
		switch {
		case row.EffectiveAt.After(now):
			result.Scheduled = append(result.Scheduled, config)
		case !foundCurrent:
			foundCurrent = true
			result.Current = config
		default:
			result.History = append(result.History, config)
		}
	}
	return result, nil
}

// AdminSetFreeLimits appends a configuration by adminUUID, effective now
// when effectiveAt is zero. Nothing is deleted or reduced retroactively:
// accounts over a lowered capacity only lose the ability to add more.
func (s *Service) AdminSetFreeLimits(ctx context.Context, adminUUID string, newLimits plan.Limits, effectiveAt time.Time, reason string) (plan.SetFreeLimitsOutcome, error) {
	defer logging.Step(ctx, "plan.admin_set_free_limits").Close()
	if _, err := uuid.Parse(adminUUID); err != nil {
		return "", fmt.Errorf("invalid admin uuid: %w", err)
	}
	now := s.now()
	if effectiveAt.IsZero() {
		effectiveAt = now
	}
	outcome := plan.FreeLimitsRecorded
	if !limits.Valid(newLimits) || reason == "" || len(reason) > limits.MaxReasonLen || effectiveAt.Before(now.Add(-pastTolerance)) {
		outcome = plan.FreeLimitsInvalid
	}
	if outcome == plan.FreeLimitsRecorded {
		err := s.repo.InsertFreeLimitConfig(ctx, internalrepo.FreeLimitConfig{
			SavedGames: newLimits.SavedGames, CreationsPerWindow: newLimits.CreationsPerWindow, SessionsPerWindow: newLimits.SessionsPerWindow,
			WindowDays: newLimits.WindowDays, PlayersPerSession: newLimits.PlayersPerSession, SimultaneousRooms: newLimits.SimultaneousRooms,
			StorageMB: newLimits.StorageMB, EffectiveAt: effectiveAt, CreatedByUserUUID: &adminUUID, Reason: reason, CreatedAt: now,
		})
		if err != nil {
			return "", err
		}
	}
	setLimitsTotal.Inc(ctx, outcomeLabel.Value(string(outcome)))
	logging.LogFields(ctx, logging.Field("free_limits", string(outcome)))
	return outcome, nil
}

func toConfig(row internalrepo.FreeLimitConfig) plan.FreeLimitConfig {
	config := plan.FreeLimitConfig{
		Limits: plan.Limits{
			SavedGames: row.SavedGames, CreationsPerWindow: row.CreationsPerWindow, SessionsPerWindow: row.SessionsPerWindow,
			WindowDays: row.WindowDays, PlayersPerSession: row.PlayersPerSession, SimultaneousRooms: row.SimultaneousRooms,
			StorageMB: row.StorageMB,
		},
		EffectiveAt: row.EffectiveAt, Reason: row.Reason, CreatedAt: row.CreatedAt,
	}
	if row.CreatedByUserUUID != nil {
		config.CreatedByUserUUID = *row.CreatedByUserUUID
	}
	return config
}
