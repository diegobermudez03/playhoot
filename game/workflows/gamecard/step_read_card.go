package gamecard

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/game/internal/business/card"
	"github.com/diegobermudez03/playhoot/game/internal/business/edition"
	internalrepo "github.com/diegobermudez03/playhoot/game/workflows/gamecard/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
)

//go:generate mockgen -package=gamecard -destination=step_read_card_mock_test.go . readCardRepo

// readCardRepo is ReadCard's own narrow persistence contract.
type readCardRepo interface {
	FindGame(ctx context.Context, tx *gorm.DB, gameUUID string, lock bool) (*internalrepo.CardGame, error)
	FindActiveImages(ctx context.Context, tx *gorm.DB, gameID uint) ([]internalrepo.Image, error)
	FindLifecycleFacts(ctx context.Context, tx *gorm.DB, gameID uint, editingStatus string) (internalrepo.LifecycleFacts, error)
}

// ReadCard returns ownerUUID's Game gameUUID's card, with signed links to
// its images, and where its lifecycle stands.
func (m *Manager) ReadCard(ctx context.Context, ownerUUID string, gameUUID string) (game.CardRead, error) {
	defer logging.Step(ctx, "gamecard.read_card").Close()

	owner, err := uuid.Parse(ownerUUID)
	if err != nil {
		return game.CardRead{Outcome: game.CardNotFound}, nil
	}
	parsedGame, err := uuid.Parse(gameUUID)
	if err != nil {
		return game.CardRead{Outcome: game.CardNotFound}, nil
	}
	db := m.dbServicer.GetDB()
	found, err := m.readCardRepo.FindGame(ctx, db, parsedGame.String(), false)
	if err != nil {
		return game.CardRead{}, err
	}
	if found == nil || !isOwner(found.OwnerUUID, owner) {
		return game.CardRead{Outcome: game.CardNotFound}, nil
	}
	images, err := m.readCardRepo.FindActiveImages(ctx, db, found.ID)
	if err != nil {
		return game.CardRead{}, err
	}
	facts, err := m.readCardRepo.FindLifecycleFacts(ctx, db, found.ID, string(edition.Editing))
	if err != nil {
		return game.CardRead{}, err
	}
	read := game.Card{
		GameUUID: found.UUID, Name: found.Name, Description: found.Description,
		Visibility: game.VisibilityType(found.Visibility), Published: found.PublishedAt != nil,
		PublishedAt: found.PublishedAt, Editing: facts.Editing, Publishing: facts.Publishing,
		Photos: []game.CardImage{},
	}
	expiresAt := m.now().Add(imageLinkTTL)
	for _, image := range images {
		url, err := m.images.PresignGet(ctx, image.ObjectKey, expiresAt)
		if err != nil {
			return game.CardRead{}, fmt.Errorf("signing a card image link: %w", err)
		}
		shown := game.CardImage{UUID: image.UUID, MIMEType: image.MIMEType, Size: image.Size, URL: url, URLExpiresAt: expiresAt}
		switch card.ImageKind(image.Kind) {
		case card.Logo:
			read.Logo = &shown
		case card.Photo:
			read.Photos = append(read.Photos, shown)
		default:
			return game.CardRead{}, fmt.Errorf("card image %s has unknown kind %q", image.UUID, image.Kind)
		}
	}
	return game.CardRead{Outcome: game.CardDone, Card: read}, nil
}
