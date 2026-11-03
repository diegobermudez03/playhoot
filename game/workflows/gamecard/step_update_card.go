package gamecard

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/game/internal/business/card"
	internalrepo "github.com/diegobermudez03/playhoot/game/workflows/gamecard/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=gamecard -destination=step_update_card_mock_test.go . updateCardRepo

// updateCardRepo is UpdateCard's own narrow persistence contract.
type updateCardRepo interface {
	FindGame(ctx context.Context, tx *gorm.DB, gameUUID string, lock bool) (*internalrepo.CardGame, error)
	UpdateTexts(ctx context.Context, tx *gorm.DB, game internalrepo.CardGame, name string, description string, at time.Time) error
}

// UpdateCard changes the texts of ownerUUID's Game gameUUID's card, published
// or not, recording what changed in the Game's history. A change that leaves
// both texts as they are answers game.CardUnchanged.
func (m *Manager) UpdateCard(ctx context.Context, ownerUUID string, gameUUID string, change game.CardChange) (game.CardEdit, error) {
	defer logging.Step(ctx, "gamecard.update_card").Close()

	owner, err := uuid.Parse(ownerUUID)
	if err != nil {
		return game.CardEdit{Outcome: game.CardNotFound}, nil
	}
	parsedGame, err := uuid.Parse(gameUUID)
	if err != nil {
		return game.CardEdit{Outcome: game.CardNotFound}, nil
	}
	return utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (game.CardEdit, error) {
		found, err := m.updateCardRepo.FindGame(ctx, tx, parsedGame.String(), true)
		if err != nil {
			return game.CardEdit{}, err
		}
		if found == nil || !isOwner(found.OwnerUUID, owner) {
			return game.CardEdit{Outcome: game.CardNotFound}, nil
		}
		name, description := found.Name, found.Description
		if change.Name != nil {
			if name, err = card.NormalizeName(*change.Name); err != nil {
				return invalidText(err)
			}
		}
		if change.Description != nil {
			if description, err = card.NormalizeDescription(*change.Description); err != nil {
				return invalidText(err)
			}
		}
		if name == found.Name && description == found.Description {
			return game.CardEdit{Outcome: game.CardUnchanged}, nil
		}
		if err := m.updateCardRepo.UpdateTexts(ctx, tx, *found, name, description, m.now().UTC().Truncate(time.Microsecond)); err != nil {
			return game.CardEdit{}, err
		}
		return game.CardEdit{Outcome: game.CardDone}, nil
	})
}

func invalidText(err error) (game.CardEdit, error) {
	if errors.Is(err, card.ErrInvalidText) {
		return game.CardEdit{Outcome: game.CardInvalid, Message: err.Error()}, nil
	}
	return game.CardEdit{}, err
}
