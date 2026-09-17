package joinsession

import (
	"context"

	"github.com/diegobermudez03/playhoot/game/language/v1/program"
	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"gorm.io/gorm"
)

//go:generate mockgen -package=joinsession -destination=repo_mock_test.go . repoAPI,gameDefinitionResolver

// Input is JoinSession's public/application-facing command. UserUUID is an
// already-authenticated caller identity (GAME-ADR-0005); DisplayName is
// supplied by the trusted application/integration layer.
type Input struct {
	JoinCode       uint
	UserUUID       string
	DisplayName    string
	IdempotencyKey string
}

// Result is JoinSession's logical outcome, also the shape persisted as the
// idempotency record's replayable response payload.
type Result struct {
	SessionUUID string `json:"session_uuid"`
	DisplayName string `json:"display_name"`
}

// gameDefinitionResolver is the narrow Game Management read capability that
// loads an already-pinned immutable Game Definition by its own
// Definition/Version UUID - never by re-resolving the Game's current
// version. See WORK-0001's Game Management Dependency / Pinned Game
// Definition Is Immutable For The Session.
type gameDefinitionResolver interface {
	GetGameDefinition(ctx context.Context, gameDefinitionUUID string) (*program.Definition, error)
}

type UseCase struct {
	repo       repoAPI
	gameReader gameDefinitionResolver
}

func New(db *gorm.DB, gameReader gameDefinitionResolver) *UseCase {
	return &UseCase{
		repo:       newRepo(db),
		gameReader: gameReader,
	}
}

// JoinSession resolves an active JoinCode to its Session, loads that
// Session's pinned immutable Game Definition, and admits the caller as an
// active Participant under the Session's per-Session DB mutation lock. See
// WORK-0001's Session/Actor/Participant Operation Behavior - JoinSession.
func (c *UseCase) JoinSession(ctx context.Context, input Input) (Result, error) {
	defer logging.Step(ctx, "JoinSession").Close()
	logging.LogFields(ctx,
		logging.Field("join_code", input.JoinCode),
		logging.Field("user_uuid", input.UserUUID),
	)

	resolution, err := c.repo.resolveActiveSessionForJoinCode(ctx, input.JoinCode)
	if err != nil {
		return Result{}, err
	}
	if resolution == nil {
		return Result{}, session.ErrJoinCodeInvalid
	}

	// Loads the Session's pinned Definition/Version UUID directly - never
	// the Game's current version - so lobby capacity stays governed by the
	// exact version this Session was pinned to at Create.
	definition, err := c.gameReader.GetGameDefinition(ctx, resolution.GameDefinitionUUID)
	if err != nil {
		return Result{}, err
	}
	if definition == nil {
		monitoring.Alert(ctx, "session pinned game definition is missing")
		return Result{}, session.ErrPinnedDefinitionMissing
	}

	return c.repo.joinSession(ctx, joinParams{
		SessionID:      resolution.SessionID,
		JoinCode:       input.JoinCode,
		UserUUID:       input.UserUUID,
		DisplayName:    input.DisplayName,
		IdempotencyKey: input.IdempotencyKey,
		PlayersMax:     definition.Players.Max,
	})
}
