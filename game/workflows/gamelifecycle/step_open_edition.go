package gamelifecycle

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/game/internal/business/edition"
	internalrepo "github.com/diegobermudez03/playhoot/game/workflows/gamelifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=gamelifecycle -destination=step_open_edition_mock_test.go . openEditionRepo

// openEditionRepo is OpenEdition's own narrow persistence contract.
type openEditionRepo interface {
	LockGame(ctx context.Context, tx *gorm.DB, gameUUID string) (*internalrepo.OwnedGame, error)
	FindLatestEditionInStatus(ctx context.Context, tx *gorm.DB, gameID uint, status string) (*internalrepo.Edition, error)
	FindPendingRelease(ctx context.Context, tx *gorm.DB, gameID uint) (*internalrepo.ReleaseRecord, error)
	FindEditionRequest(ctx context.Context, tx *gorm.DB, gameID uint, idempotencyKey string) (*internalrepo.EditionRequest, error)
	ClaimEditionRequest(ctx context.Context, tx *gorm.DB, gameID uint, idempotencyKey string, at time.Time) (uint, error)
	CompleteEditionRequest(ctx context.Context, tx *gorm.DB, requestID uint, editionID uint) error
	FindEditionByID(ctx context.Context, tx *gorm.DB, editionID uint) (*internalrepo.Edition, error)
	OpenEdition(ctx context.Context, tx *gorm.DB, gameID uint, editionUUID string, status string, at time.Time) (uint, error)
}

var (
	editionOutcomeLabel = metrics.NewLabel("outcome", "opened", "not_found", "not_published", "already_open", "publishing")

	editionOpeningsTotal = metrics.NewCounter(
		"game_edition_openings_total",
		"Requests to open a new edition of a published Game, by outcome.",
		editionOutcomeLabel,
	)
)

// OpenEdition opens a new edition of ownerUUID's published Game gameUUID,
// with a new uuid for the Play version it is built in, starting from the
// version of the Game's latest published edition, which stays the one played
// until the new edition is published. It is idempotent per (game,
// idempotencyKey).
func (m *Manager) OpenEdition(ctx context.Context, ownerUUID string, gameUUID string, idempotencyKey string) (game.Edition, error) {
	defer logging.Step(ctx, "gamelifecycle.open_edition").Close()

	owner, err := uuid.Parse(ownerUUID)
	if err != nil {
		return game.Edition{Outcome: game.EditionNotFound}, nil
	}
	parsedGame, err := uuid.Parse(gameUUID)
	if err != nil {
		return game.Edition{Outcome: game.EditionNotFound}, nil
	}
	if idempotencyKey == "" {
		return game.Edition{}, fmt.Errorf("idempotency key is required")
	}
	opened, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (game.Edition, error) {
		return m.openEditionInTx(ctx, tx, owner, parsedGame.String(), idempotencyKey)
	})
	if err != nil {
		return game.Edition{}, err
	}
	editionOpeningsTotal.Inc(ctx, editionOutcomeLabel.Value(string(opened.Outcome)))
	logging.LogFields(ctx, logging.Field("edition_outcome", string(opened.Outcome)), logging.Field("version_uuid", opened.VersionUUID))
	return opened, nil
}

func (m *Manager) openEditionInTx(ctx context.Context, tx *gorm.DB, owner uuid.UUID, gameUUID string, idempotencyKey string) (game.Edition, error) {
	owned, err := m.openEditionRepo.LockGame(ctx, tx, gameUUID)
	if err != nil {
		return game.Edition{}, err
	}
	if owned == nil {
		return game.Edition{Outcome: game.EditionNotFound}, nil
	}
	if recorded, err := uuid.Parse(owned.OwnerUUID); err != nil || recorded != owner {
		return game.Edition{Outcome: game.EditionNotFound}, nil
	}
	base, err := m.openEditionRepo.FindLatestEditionInStatus(ctx, tx, owned.ID, string(edition.Published))
	if err != nil {
		return game.Edition{}, err
	}
	request, err := m.openEditionRepo.FindEditionRequest(ctx, tx, owned.ID, idempotencyKey)
	if err != nil {
		return game.Edition{}, err
	}
	if request != nil {
		return m.replayEdition(ctx, tx, request, base)
	}
	if owned.PublishedAt == nil || base == nil {
		return game.Edition{Outcome: game.EditionNotPublished}, nil
	}
	open, err := m.openEditionRepo.FindLatestEditionInStatus(ctx, tx, owned.ID, string(edition.Editing))
	if err != nil {
		return game.Edition{}, err
	}
	if open != nil {
		return game.Edition{Outcome: game.EditionAlreadyOpen, VersionUUID: open.UUID}, nil
	}
	pending, err := m.openEditionRepo.FindPendingRelease(ctx, tx, owned.ID)
	if err != nil {
		return game.Edition{}, err
	}
	if pending != nil {
		return game.Edition{Outcome: game.EditionPublishing}, nil
	}
	now := m.now().UTC().Truncate(time.Microsecond)
	requestID, err := m.openEditionRepo.ClaimEditionRequest(ctx, tx, owned.ID, idempotencyKey, now)
	if err != nil {
		return game.Edition{}, err
	}
	versionUUID := m.newUUID()
	editionID, err := m.openEditionRepo.OpenEdition(ctx, tx, owned.ID, versionUUID, string(edition.Editing), now)
	if err != nil {
		return game.Edition{}, err
	}
	if err := m.openEditionRepo.CompleteEditionRequest(ctx, tx, requestID, editionID); err != nil {
		return game.Edition{}, err
	}
	return game.Edition{Outcome: game.EditionOpened, VersionUUID: versionUUID, BaseVersionUUID: base.UUID}, nil
}

// replayEdition answers a retry with the edition its request opened, and the
// published version it started from.
func (m *Manager) replayEdition(ctx context.Context, tx *gorm.DB, request *internalrepo.EditionRequest, base *internalrepo.Edition) (game.Edition, error) {
	if request.EditionID == nil {
		monitoring.Alert(ctx, "game edition request has no edition")
		return game.Edition{}, fmt.Errorf("edition request %d has no edition", request.ID)
	}
	opened, err := m.openEditionRepo.FindEditionByID(ctx, tx, *request.EditionID)
	if err != nil {
		return game.Edition{}, err
	}
	if opened == nil || base == nil {
		monitoring.Alert(ctx, "game edition request references an edition that does not exist")
		return game.Edition{}, fmt.Errorf("edition request %d references a missing edition", request.ID)
	}
	return game.Edition{Outcome: game.EditionOpened, VersionUUID: opened.UUID, BaseVersionUUID: base.UUID}, nil
}
