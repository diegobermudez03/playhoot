package sessionlifecycle

import (
	"context"
	"time"

	"github.com/diegobermudez03/playhoot/game/language/v1/program"
	"github.com/diegobermudez03/playhoot/game/session"
	internalrepo "github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"gorm.io/gorm"
)

// gamePinnedDefinitionReader is the narrow Game Management read capability
// Join depends on to load an already-pinned immutable Game Definition by its
// own Definition/Version UUID - never by re-resolving the Game's current
// version. See WORK-0001's Game Management Dependency / Pinned Game
// Definition Is Immutable For The Session.
type gamePinnedDefinitionReader interface {
	GetGameDefinition(ctx context.Context, gameDefinitionUUID string) (*program.Definition, error)
}

// joinRepoAPI is Join's own narrow persistence contract (see createRepoAPI's
// doc comment on why this is not shared verbatim with Create/Leave despite
// some overlapping method shapes).
type joinRepoAPI interface {
	ResolveActiveSessionForJoinCode(ctx context.Context, joinCode uint) (*internalrepo.JoinCodeResolution, error)
	LockSessionByID(ctx context.Context, tx *gorm.DB, sessionID uint) (*internalrepo.SessionRow, error)
	SetSessionTerminal(ctx context.Context, tx *gorm.DB, sessionID uint, terminalAt time.Time, terminalReason string, now time.Time) error
	RevokeActiveJoinCode(ctx context.Context, tx *gorm.DB, sessionID uint, now time.Time) error
	ClaimSessionRequest(ctx context.Context, tx *gorm.DB, operation, userUUID, idempotencyKey string, sessionID *uint, requestPayload string) (requestID uint, claimed bool, existing *internalrepo.RequestRow, err error)
	CompleteSessionRequest(ctx context.Context, tx *gorm.DB, requestID uint, sessionID *uint, outcome string, responsePayload string) error
	FindActor(ctx context.Context, tx *gorm.DB, sessionID uint, userUUID string) (*internalrepo.ActorRow, error)
	CreateActor(ctx context.Context, tx *gorm.DB, sessionID uint, userUUID string, now time.Time) (uint, error)
	FindParticipant(ctx context.Context, tx *gorm.DB, actorID uint) (*internalrepo.ParticipantRow, error)
	CountActiveParticipants(ctx context.Context, tx *gorm.DB, sessionID uint) (int, error)
	CreateParticipant(ctx context.Context, tx *gorm.DB, actorID uint, displayName string, now time.Time) error
	ActivateParticipant(ctx context.Context, tx *gorm.DB, participantID uint, displayName string, now time.Time) error
}

// Join resolves an active JoinCode to its Session, loads that Session's
// pinned immutable Game Definition, and admits the caller as an active
// Participant under the Session's per-Session DB mutation lock. See
// WORK-0001's Manager Operation Behavior - Join and GAME-ADR-0021.
func (m *Manager) Join(ctx context.Context, joinCode JoinCode, userUUID UserUUID, displayName DisplayName, idempotencyKey IdempotencyKey) (JoinResult, error) {
	defer logging.Step(ctx, "SessionLifecycle.Join").Close()
	logging.LogFields(ctx,
		logging.Field("join_code", uint(joinCode)),
		logging.Field("user_uuid", string(userUUID)),
	)

	if idempotencyKey == "" {
		return JoinResult{}, session.ErrIdempotencyKeyRequired
	}

	resolution, err := m.joinRepo.ResolveActiveSessionForJoinCode(ctx, uint(joinCode))
	if err != nil {
		return JoinResult{}, err
	}
	if resolution == nil {
		return JoinResult{}, session.ErrJoinCodeInvalid
	}

	// Loads the Session's pinned Definition/Version UUID directly - never
	// the Game's current version - before opening the mutation
	// transaction/row lock (GAME-ADR-0001), so lobby capacity stays governed
	// by the exact version this Session was pinned to at Create.
	definition, err := m.pinnedGameReader.GetGameDefinition(ctx, resolution.GameDefinitionUUID)
	if err != nil {
		return JoinResult{}, err
	}
	if definition == nil {
		monitoring.Alert(ctx, "session pinned game definition is missing")
		return JoinResult{}, session.ErrPinnedDefinitionMissing
	}
	playersMax := definition.Players.Max

	incomingPayload := joinRequestPayload{JoinCode: uint(joinCode), UserUUID: string(userUUID), DisplayName: string(displayName)}
	payloadBytes, err := marshalPayload(incomingPayload)
	if err != nil {
		return JoinResult{}, err
	}

	var result JoinResult
	var bizErr error
	txErr := m.tx.RunInTransaction(ctx, func(ctx context.Context, tx *gorm.DB) error {
		row, err := m.joinRepo.LockSessionByID(ctx, tx, resolution.SessionID)
		if err != nil {
			return err
		}
		if row == nil {
			bizErr = session.ErrSessionNotFound
			return nil
		}

		now := m.now()
		expired, err := materializeExpirationIfDue(ctx, tx, m.joinRepo, row, now)
		if err != nil {
			return err
		}
		if expired || row.Phase != session.PhaseLobby {
			// The materialization above (if it ran) must still commit even
			// though this attempted Join is rejected - see
			// WORK-0001's Transaction Ownership.
			bizErr = session.ErrLobbyExpired
			return nil
		}

		sessionID := row.ID
		requestID, claimed, existing, err := m.joinRepo.ClaimSessionRequest(ctx, tx, operationJoin, string(userUUID), string(idempotencyKey), &sessionID, payloadBytes)
		if err != nil {
			return err
		}
		if !claimed {
			result, bizErr = interpretExistingJoinClaim(existing, incomingPayload)
			return nil
		}

		actor, err := m.joinRepo.FindActor(ctx, tx, row.ID, string(userUUID))
		if err != nil {
			return err
		}

		var actorID uint
		var participant *internalrepo.ParticipantRow
		if actor != nil {
			actorID = actor.ID
			participant, err = m.joinRepo.FindParticipant(ctx, tx, actorID)
			if err != nil {
				return err
			}
		} else {
			actorID, err = m.joinRepo.CreateActor(ctx, tx, row.ID, string(userUUID), now)
			if err != nil {
				return err
			}
		}

		if participant != nil && participant.Active {
			// A *different* idempotency token than any previously used for
			// this user's admission, evaluated against current state, while
			// already an active Participant: a new command, rejected - not
			// silently replayed or treated as success (GAME-ADR-0021). The
			// rejection is itself the token's completed logical outcome, so
			// it still commits together with the just-created claim.
			if err := m.joinRepo.CompleteSessionRequest(ctx, tx, requestID, &sessionID, outcomeAlreadyJoined, ""); err != nil {
				return err
			}
			bizErr = session.ErrAlreadyJoined
			return nil
		}

		activeCount, err := m.joinRepo.CountActiveParticipants(ctx, tx, row.ID)
		if err != nil {
			return err
		}
		if playersMax > 0 && activeCount >= playersMax {
			if err := m.joinRepo.CompleteSessionRequest(ctx, tx, requestID, &sessionID, outcomeLobbyFull, ""); err != nil {
				return err
			}
			bizErr = session.ErrLobbyFull
			return nil
		}

		if participant == nil {
			if err := m.joinRepo.CreateParticipant(ctx, tx, actorID, string(displayName), now); err != nil {
				return err
			}
		} else {
			if err := m.joinRepo.ActivateParticipant(ctx, tx, participant.ID, string(displayName), now); err != nil {
				return err
			}
		}

		result = JoinResult{SessionUUID: SessionUUID(row.UUID), DisplayName: displayName}
		responseBytes, err := marshalPayload(result)
		if err != nil {
			return err
		}
		if err := m.joinRepo.CompleteSessionRequest(ctx, tx, requestID, &sessionID, outcomeJoined, responseBytes); err != nil {
			return err
		}
		return nil
	})
	if txErr != nil {
		return JoinResult{}, txErr
	}
	if bizErr != nil {
		return JoinResult{}, bizErr
	}
	return result, nil
}
