package createsession

import (
	"context"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/game/game"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"gorm.io/gorm"
)

//go:generate mockgen -package=createsession -destination=repo_mock_test.go . repoAPI,gameDefinitionResolver

// defaultLobbyTTL is V1 Session Runtime policy (see WORK-0001's Known
// Risks/Questions - no repository-wide configuration surface exists yet to
// reuse for this).
const defaultLobbyTTL = 10 * time.Minute

// Input is CreateSession's public/application-facing command. GameUUID is a
// public Game identity; HostUserUUID is an already-authenticated caller
// identity (see GAME-ADR-0005 - Session Runtime does not resolve
// credentials).
type Input struct {
	GameUUID       string
	HostUserUUID   string
	IdempotencyKey string
}

// Result is CreateSession's logical outcome, also the shape persisted as the
// idempotency record's replayable response payload.
type Result struct {
	SessionUUID    string    `json:"session_uuid"`
	JoinCode       uint      `json:"join_code"`
	LobbyExpiresAt time.Time `json:"lobby_expires_at"`
}

// gameDefinitionResolver is the narrow Game Management read capability
// CreateSession depends on to resolve the Game's current playable
// Definition/Version to pin (GAME-ADR-0001, GAME-ADR-0004).
type gameDefinitionResolver interface {
	GetPlayableGameWithCurrentVersion(ctx context.Context, gameUUID string) (*game.Game, error)
}

type UseCase struct {
	repo       repoAPI
	gameReader gameDefinitionResolver
	lobbyTTL   time.Duration
}

func New(db *gorm.DB, gameReader gameDefinitionResolver) *UseCase {
	return &UseCase{
		repo:       newRepo(db),
		gameReader: gameReader,
		lobbyTTL:   defaultLobbyTTL,
	}
}

// CreateSession resolves/compiles/pins the Game's current playable
// Definition, then creates a LOBBY Session with a host SessionActor and an
// active JoinCode. See WORK-0001's Session/Actor/Participant Operation
// Behavior - CreateSession.
func (c *UseCase) CreateSession(ctx context.Context, input Input) (Result, error) {
	defer logging.Step(ctx, "CreateSession").Close()
	logging.LogFields(ctx,
		logging.Field("game_uuid", input.GameUUID),
		logging.Field("host_user_uuid", input.HostUserUUID),
	)

	playableGame, err := c.gameReader.GetPlayableGameWithCurrentVersion(ctx, input.GameUUID)
	if err != nil {
		return Result{}, err
	}
	if playableGame == nil {
		return Result{}, session.ErrGameNotFound
	}

	if _, diagnostics := engineservice.Compile(playableGame.Definition); diagnostics.HasErrors() {
		monitoring.Alert(ctx, fmt.Sprintf(
			"game definition failed to compile at session create: game_uuid=%s version_uuid=%s",
			input.GameUUID, playableGame.VersionUUID,
		))
		return Result{}, session.ErrDefinitionDoesNotCompile
	}

	result, err := c.repo.createSession(ctx, createParams{
		GameUUID:           input.GameUUID,
		HostUserUUID:       input.HostUserUUID,
		IdempotencyKey:     input.IdempotencyKey,
		GameDefinitionUUID: playableGame.VersionUUID,
		LobbyTTL:           c.lobbyTTL,
	})
	if err != nil {
		return Result{}, err
	}

	return result, nil
}
