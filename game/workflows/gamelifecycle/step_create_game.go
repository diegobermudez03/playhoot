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

//go:generate mockgen -package=gamelifecycle -destination=step_create_game_mock_test.go . createGameRepo

// createGameRepo is CreateGame's own narrow persistence contract.
type createGameRepo interface {
	ClaimCreateGameRequest(ctx context.Context, tx *gorm.DB, ownerUUID string, idempotencyKey string) (uint, *internalrepo.CreateGameRequest, error)
	CompleteCreateGameRequest(ctx context.Context, tx *gorm.DB, requestID uint, gameID uint) error
	CreateGame(ctx context.Context, tx *gorm.DB, game internalrepo.NewGame) (uint, error)
	CreateEdition(ctx context.Context, tx *gorm.DB, gameID uint, editionUUID string, status string, createdAt time.Time) error
	FindCreatedGame(ctx context.Context, tx *gorm.DB, gameID uint) (*internalrepo.CreatedGame, error)
}

var (
	createGameOutcomeLabel = metrics.NewLabel("outcome", "created", "replayed")

	createGameOutcomeCreated  = createGameOutcomeLabel.Value("created")
	createGameOutcomeReplayed = createGameOutcomeLabel.Value("replayed")

	gameCreateTotal = metrics.NewCounter(
		"game_create_total",
		"Completed Game creation requests, by outcome; replayed counts retries answered from a previous request.",
		createGameOutcomeLabel,
	)
)

// createGameAttempt is what one creation transaction decided, and whether it
// was replayed from an earlier request with the same idempotency key.
type createGameAttempt struct {
	created  game.CreatedGame
	replayed bool
}

// CreateGame creates an unpublished, private Game owned by ownerUUID, its card
// empty, with its first edition open for editing. It is idempotent per
// (ownerUUID, idempotencyKey): a retry answers the same Game and edition.
func (m *Manager) CreateGame(ctx context.Context, ownerUUID string, idempotencyKey string) (game.CreatedGame, error) {
	defer logging.Step(ctx, "gamelifecycle.create_game").Close()

	owner, err := uuid.Parse(ownerUUID)
	if err != nil {
		return game.CreatedGame{}, fmt.Errorf("owner uuid is not a valid UUID")
	}
	if idempotencyKey == "" {
		return game.CreatedGame{}, fmt.Errorf("idempotency key is required")
	}

	attempt, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (createGameAttempt, error) {
		return m.createGameInTx(ctx, tx, owner.String(), idempotencyKey)
	})
	if err != nil {
		return game.CreatedGame{}, err
	}

	// Counted only once the transaction committed.
	if attempt.replayed {
		gameCreateTotal.Inc(ctx, createGameOutcomeReplayed)
	} else {
		gameCreateTotal.Inc(ctx, createGameOutcomeCreated)
	}
	logging.LogFields(ctx,
		logging.Field("game_uuid", attempt.created.GameUUID),
		logging.Field("edition_version_uuid", attempt.created.EditionVersionUUID),
		logging.Field("create_game_replayed", attempt.replayed),
	)
	return attempt.created, nil
}

// createGameInTx is CreateGame's work inside its transaction; its arguments
// are already validated and normalized.
func (m *Manager) createGameInTx(ctx context.Context, tx *gorm.DB, ownerUUID string, idempotencyKey string) (createGameAttempt, error) {
	requestID, existing, err := m.createGameRepo.ClaimCreateGameRequest(ctx, tx, ownerUUID, idempotencyKey)
	if err != nil {
		return createGameAttempt{}, err
	}
	if existing != nil {
		// The claim and the Game it created commit together, so a request
		// read back always has its Game.
		if existing.GameID == nil {
			monitoring.Alert(ctx, "game create request has no game")
			return createGameAttempt{}, fmt.Errorf("create game request %d has no game", existing.ID)
		}
		created, err := m.createGameRepo.FindCreatedGame(ctx, tx, *existing.GameID)
		if err != nil {
			return createGameAttempt{}, err
		}
		if created == nil {
			monitoring.Alert(ctx, "game create request references a game without editions")
			return createGameAttempt{}, fmt.Errorf("create game request %d references game %d, which has no edition", existing.ID, *existing.GameID)
		}
		return createGameAttempt{created: game.CreatedGame(*created), replayed: true}, nil
	}

	now := m.now().UTC().Truncate(time.Microsecond)
	newGame := internalrepo.NewGame{
		UUID:       m.newUUID(),
		OwnerUUID:  ownerUUID,
		Visibility: string(game.Private),
		CreatedAt:  now,
	}
	gameID, err := m.createGameRepo.CreateGame(ctx, tx, newGame)
	if err != nil {
		return createGameAttempt{}, err
	}
	editionUUID := m.newUUID()
	if err := m.createGameRepo.CreateEdition(ctx, tx, gameID, editionUUID, string(edition.Editing), now); err != nil {
		return createGameAttempt{}, err
	}
	if err := m.createGameRepo.CompleteCreateGameRequest(ctx, tx, requestID, gameID); err != nil {
		return createGameAttempt{}, err
	}
	return createGameAttempt{created: game.CreatedGame{
		GameUUID:           newGame.UUID,
		EditionVersionUUID: editionUUID,
		CreatedAt:          now,
	}}, nil
}
