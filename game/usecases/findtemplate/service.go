// Package findtemplate answers whether a Game may be started from: published,
// not deleted and allowed as a template by its owner. Anyone may read or start
// from a template; its owner keeps the Game, and a copy never changes it.
package findtemplate

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/game/internal/business/edition"
	"github.com/diegobermudez03/playhoot/game/usecases/findtemplate/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
)

//go:generate mockgen -package=findtemplate -destination=service_mock_test.go . repoAPI

// repoAPI is Service's own narrow persistence contract.
type repoAPI interface {
	FindTemplate(ctx context.Context, gameUUID string, publishedStatus string) (*repo.Template, error)
}

// Service is the find-template use case.
type Service struct {
	repo repoAPI
}

// New constructs a Service backed by db.
func New(db *gorm.DB) *Service {
	return &Service{repo: repo.New(db)}
}

// FindTemplate answers gameUUID as a template, with the Play version of its
// latest published edition, or not found.
func (s *Service) FindTemplate(ctx context.Context, gameUUID string) (game.Template, error) {
	defer logging.Step(ctx, "findtemplate.find_template").Close()

	parsed, err := uuid.Parse(gameUUID)
	if err != nil {
		return game.Template{Outcome: game.TemplateNotFound}, nil
	}
	found, err := s.repo.FindTemplate(ctx, parsed.String(), string(edition.Published))
	if err != nil {
		return game.Template{}, err
	}
	if found == nil {
		logging.LogFields(ctx, logging.Field("template_outcome", string(game.TemplateNotFound)))
		return game.Template{Outcome: game.TemplateNotFound}, nil
	}
	if found.VersionUUID == nil {
		// published_at is set only when an edition is published.
		monitoring.Alert(ctx, "game is published without a published edition")
		return game.Template{Outcome: game.TemplateNotFound}, nil
	}
	logging.LogFields(ctx, logging.Field("template_outcome", string(game.TemplateFound)))
	return game.Template{
		Outcome: game.TemplateFound, GameUUID: found.GameUUID, VersionUUID: *found.VersionUUID,
		Name: found.Name, Description: found.Description, Featured: found.Featured,
	}, nil
}
