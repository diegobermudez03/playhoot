// Package getuserlimits answers the capacity limits of a user.
package getuserlimits

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/plan"
	"github.com/diegobermudez03/playhoot/plan/internal/business/limits"
	internalrepo "github.com/diegobermudez03/playhoot/plan/internal/repo"
)

// Service is the GetUserLimits use case.
type Service struct {
	repo *internalrepo.Repo
	now  func() time.Time
}

// New constructs a Service on db.
func New(db *gorm.DB) *Service {
	return &Service{repo: internalrepo.New(db), now: time.Now}
}

// GetUserLimits returns the capacities of userUUID. Until subscriptions
// exist everyone gets the free configuration in force; an unknown or empty
// user is never an error. A pure read: lowering the limits never removes
// anything an account already has.
func (s *Service) GetUserLimits(ctx context.Context, userUUID string) (plan.Limits, error) {
	defer logging.Step(ctx, "plan.get_user_limits").Close()
	config, err := s.repo.CurrentFreeLimitConfig(ctx, s.now())
	if err != nil {
		return plan.Limits{}, err
	}
	if config == nil {
		return limits.Defaults, nil
	}
	return toLimits(*config), nil
}

func toLimits(row internalrepo.FreeLimitConfig) plan.Limits {
	return plan.Limits{
		SavedGames: row.SavedGames, CreationsPerWindow: row.CreationsPerWindow, SessionsPerWindow: row.SessionsPerWindow,
		WindowDays: row.WindowDays, PlayersPerSession: row.PlayersPerSession, SimultaneousRooms: row.SimultaneousRooms,
		StorageMB: row.StorageMB,
	}
}
