// Package findeditableversion answers which Play version a user may edit for
// a Game: its open edition's, when the user owns the Game. It authorizes
// edits and, for now, reads of the version being built (owner only).
package findeditableversion

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/game/internal/business/edition"
	"github.com/diegobermudez03/playhoot/game/usecases/findeditableversion/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
)

//go:generate mockgen -package=findeditableversion -destination=service_mock_test.go . repoAPI

// repoAPI is Service's own narrow persistence contract.
type repoAPI interface {
	FindGameEditing(ctx context.Context, gameUUID string, editionStatus string) (*repo.GameEditing, error)
}

// Service is the find-editable-version use case.
type Service struct {
	repo repoAPI
}

// New constructs a Service backed by db.
func New(db *gorm.DB) *Service {
	return &Service{repo: repo.New(db)}
}

// FindEditableVersion answers the Play version of gameUUID's open edition when
// userUUID owns the Game. A Game someone else owns is not found, so its
// existence is never revealed.
func (s *Service) FindEditableVersion(ctx context.Context, gameUUID string, userUUID string) (game.EditableVersion, error) {
	defer logging.Step(ctx, "findeditableversion.find_editable_version").Close()

	gameID, err := uuid.Parse(gameUUID)
	if err != nil {
		return game.EditableVersion{Outcome: game.EditableVersionNotFound}, nil
	}
	user, err := uuid.Parse(userUUID)
	if err != nil {
		return game.EditableVersion{Outcome: game.EditableVersionNotFound}, nil
	}
	editing, err := s.repo.FindGameEditing(ctx, gameID.String(), string(edition.Editing))
	if err != nil {
		return game.EditableVersion{}, err
	}
	answer := decide(editing, user)
	logging.LogFields(ctx, logging.Field("editable_version_outcome", string(answer.Outcome)))
	return answer, nil
}

func decide(editing *repo.GameEditing, user uuid.UUID) game.EditableVersion {
	if editing == nil {
		return game.EditableVersion{Outcome: game.EditableVersionNotFound}
	}
	owner, err := uuid.Parse(editing.OwnerUUID)
	if err != nil || owner != user {
		return game.EditableVersion{Outcome: game.EditableVersionNotFound}
	}
	if editing.VersionUUID == nil {
		return game.EditableVersion{Outcome: game.EditableVersionNotEditable}
	}
	return game.EditableVersion{Outcome: game.EditableVersionFound, VersionUUID: *editing.VersionUUID}
}
