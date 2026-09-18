package sessionlifecycle

import (
	"context"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/game/game"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/game/session"
	internalrepo "github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"gorm.io/gorm"
)

// gameCurrentVersionReader is the narrow Game Management read capability
// Create depends on to resolve the Game's current playable
// Definition/Version to pin (GAME-ADR-0001, GAME-ADR-0004).
type gameCurrentVersionReader interface {
	GetPlayableGameWithCurrentVersion(ctx context.Context, gameUUID string) (*game.Game, error)
}

// createRepoAPI is Create's own narrow persistence contract. Even though one
// concrete internal/repo.Repo happens to satisfy createRepoAPI/joinRepoAPI/
// leaveRepoAPI today, each step keeps its own contract
// (`docs/engineering/standards/domain-logic-placement.md`'s Workflow
// Grouping Does Not Imply A Shared Repository Contract).
type createRepoAPI interface {
	ClaimSessionRequest(ctx context.Context, tx *gorm.DB, operation, userUUID, idempotencyKey string, sessionID *uint, requestPayload string) (requestID uint, claimed bool, existing *internalrepo.RequestRow, err error)
	CompleteSessionRequest(ctx context.Context, tx *gorm.DB, requestID uint, sessionID *uint, outcome string, responsePayload string) error
	CreateSessionWithHost(ctx context.Context, tx *gorm.DB, gameDefinitionUUID string, hostUserUUID string, lobbyExpiresAt time.Time, now time.Time) (internalrepo.CreatedSession, error)
	CreateJoinCode(ctx context.Context, tx *gorm.DB, sessionID uint, now time.Time) (uint, error)
}

// Create resolves/compiles/pins the Game's current playable Definition, then
// creates a LOBBY Session with a host SessionActor and an active JoinCode.
// See WORK-0001's Manager Operation Behavior - Create.
func (m *Manager) Create(ctx context.Context, gameUUID GameUUID, hostUserUUID UserUUID, idempotencyKey IdempotencyKey) (CreatedSession, error) {
	defer logging.Step(ctx, "SessionLifecycle.Create").Close()
	logging.LogFields(ctx,
		logging.Field("game_uuid", string(gameUUID)),
		logging.Field("host_user_uuid", string(hostUserUUID)),
	)

	if idempotencyKey == "" {
		return CreatedSession{}, session.ErrIdempotencyKeyRequired
	}

	playableGame, err := m.currentGameReader.GetPlayableGameWithCurrentVersion(ctx, string(gameUUID))
	if err != nil {
		return CreatedSession{}, err
	}
	if playableGame == nil {
		return CreatedSession{}, session.ErrGameNotFound
	}

	if _, diagnostics := engineservice.Compile(playableGame.Definition); diagnostics.HasErrors() {
		monitoring.Alert(ctx, fmt.Sprintf(
			"game definition failed to compile at session create: game_uuid=%s version_uuid=%s",
			gameUUID, playableGame.VersionUUID,
		))
		return CreatedSession{}, session.ErrDefinitionDoesNotCompile
	}

	incomingPayload := createRequestPayload{GameUUID: string(gameUUID)}
	payloadBytes, err := marshalPayload(incomingPayload)
	if err != nil {
		return CreatedSession{}, err
	}

	var result CreatedSession
	var bizErr error
	txErr := m.tx.RunInTransaction(ctx, func(ctx context.Context, tx *gorm.DB) error {
		requestID, claimed, existing, err := m.createRepo.ClaimSessionRequest(ctx, tx, operationCreate, string(hostUserUUID), string(idempotencyKey), nil, payloadBytes)
		if err != nil {
			return err
		}
		if !claimed {
			result, bizErr = interpretExistingCreateClaim(existing, incomingPayload)
			return nil
		}

		now := m.now()
		created, err := m.createRepo.CreateSessionWithHost(ctx, tx, playableGame.VersionUUID, string(hostUserUUID), now.Add(m.lobbyTTL), now)
		if err != nil {
			return err
		}

		joinCode, err := m.createRepo.CreateJoinCode(ctx, tx, created.SessionID, now)
		if err != nil {
			return err
		}

		result = CreatedSession{
			SessionUUID:    SessionUUID(created.SessionUUID),
			JoinCode:       JoinCode(joinCode),
			LobbyExpiresAt: created.LobbyExpiresAt,
		}
		responseBytes, err := marshalPayload(result)
		if err != nil {
			return err
		}

		sessionID := created.SessionID
		if err := m.createRepo.CompleteSessionRequest(ctx, tx, requestID, &sessionID, outcomeCreated, responseBytes); err != nil {
			return err
		}
		return nil
	})
	if txErr != nil {
		return CreatedSession{}, txErr
	}
	if bizErr != nil {
		return CreatedSession{}, bizErr
	}
	return result, nil
}
