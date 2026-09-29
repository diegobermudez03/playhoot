// Package checkvisibility is Game Management's narrow cross-domain read
// capability: whether a Game currently exists and is playable/visible.
// It resolves this entirely from the games table's own visibility column -
// no version/definition/script concept is involved, since Game Management
// owns none anymore (see game/docs/DATA_MODEL.md).
package checkvisibility

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/game/internal/businessservice"
	storage "github.com/diegobermudez03/playhoot/game/internal/storage"
	"gorm.io/gorm"
)

//go:generate mockgen -package=checkvisibility -destination=repo_mock_test.go . repoAPI

// repoAPI is Service's own narrow persistence contract.
type repoAPI interface {
	ResolveVisibility(ctx context.Context, gameUUID string) (*string, error)
}

// Service is Game Management's visibility-check use case.
type Service struct {
	repo repoAPI
}

// New constructs a Service backed by db.
func New(db *gorm.DB) *Service {
	return &Service{repo: storage.New(db)}
}

// IsVisible reports whether gameUUID identifies an existing, currently
// playable/visible Game. found is false when no such Game exists (deleted
// or never existed); visible is only meaningful when found is true.
func (s *Service) IsVisible(ctx context.Context, gameUUID string) (visible bool, found bool, err error) {
	raw, err := s.repo.ResolveVisibility(ctx, gameUUID)
	if err != nil {
		return false, false, fmt.Errorf("resolving game visibility: %w", err)
	}
	if raw == nil {
		return false, false, nil
	}
	return businessservice.IsPlayableVisibility(game.VisibilityType(*raw)), true, nil
}
