package gamecard

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/game/internal/business/card"
	internalrepo "github.com/diegobermudez03/playhoot/game/workflows/gamecard/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=gamecard -destination=step_remove_image_mock_test.go . removeImageRepo

// removeImageRepo is RemoveImage's own narrow persistence contract.
type removeImageRepo interface {
	FindGame(ctx context.Context, tx *gorm.DB, gameUUID string, lock bool) (*internalrepo.CardGame, error)
	FindActiveImages(ctx context.Context, tx *gorm.DB, gameID uint) ([]internalrepo.Image, error)
	RemoveImage(ctx context.Context, tx *gorm.DB, imageID uint, at time.Time) (bool, error)
}

// RemoveImage stops showing an image on ownerUUID's Game gameUUID's card: its
// logo, or the photo imageUUID. Removing an image no longer shown answers
// game.CardImageNotFound; its record and content stay.
func (m *Manager) RemoveImage(ctx context.Context, ownerUUID string, gameUUID string, kind card.ImageKind, imageUUID string) (game.CardEdit, error) {
	defer logging.Step(ctx, "gamecard.remove_image").Close()

	if kind != card.Logo && kind != card.Photo {
		return game.CardEdit{}, fmt.Errorf("unknown card image kind %q", kind)
	}
	owner, err := uuid.Parse(ownerUUID)
	if err != nil {
		return game.CardEdit{Outcome: game.CardNotFound}, nil
	}
	parsedGame, err := uuid.Parse(gameUUID)
	if err != nil {
		return game.CardEdit{Outcome: game.CardNotFound}, nil
	}
	edited, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (game.CardEdit, error) {
		found, err := m.removeImageRepo.FindGame(ctx, tx, parsedGame.String(), true)
		if err != nil {
			return game.CardEdit{}, err
		}
		if found == nil || !isOwner(found.OwnerUUID, owner) {
			return game.CardEdit{Outcome: game.CardNotFound}, nil
		}
		images, err := m.removeImageRepo.FindActiveImages(ctx, tx, found.ID)
		if err != nil {
			return game.CardEdit{}, err
		}
		for _, image := range images {
			if image.Kind != string(kind) || (kind == card.Photo && image.UUID != imageUUID) {
				continue
			}
			if _, err := m.removeImageRepo.RemoveImage(ctx, tx, image.ID, m.now().UTC().Truncate(time.Microsecond)); err != nil {
				return game.CardEdit{}, err
			}
			return game.CardEdit{Outcome: game.CardDone}, nil
		}
		return game.CardEdit{Outcome: game.CardImageNotFound}, nil
	})
	if err != nil {
		return game.CardEdit{}, err
	}
	countImageEdit(ctx, edited, false)
	return edited, nil
}
