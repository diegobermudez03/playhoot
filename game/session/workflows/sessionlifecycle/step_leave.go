package sessionlifecycle

import (
	"context"
	"time"

	"github.com/diegobermudez03/playhoot/game/session"
	internalrepo "github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"gorm.io/gorm"
)

// leaveRepoAPI is Leave's own narrow persistence contract (see
// createRepoAPI's doc comment on why this is not shared verbatim with
// Create/Join despite some overlapping method shapes).
type leaveRepoAPI interface {
	LockSessionByUUID(ctx context.Context, tx *gorm.DB, sessionUUID string) (*internalrepo.SessionRow, error)
	SetSessionTerminal(ctx context.Context, tx *gorm.DB, sessionID uint, terminalAt time.Time, terminalReason string, now time.Time) error
	RevokeActiveJoinCode(ctx context.Context, tx *gorm.DB, sessionID uint, now time.Time) error
	ClaimSessionRequest(ctx context.Context, tx *gorm.DB, operation, userUUID, idempotencyKey string, sessionID *uint, requestPayload string) (requestID uint, claimed bool, existing *internalrepo.RequestRow, err error)
	CompleteSessionRequest(ctx context.Context, tx *gorm.DB, requestID uint, sessionID *uint, outcome string, responsePayload string) error
	FindActor(ctx context.Context, tx *gorm.DB, sessionID uint, userUUID string) (*internalrepo.ActorRow, error)
	FindParticipant(ctx context.Context, tx *gorm.DB, actorID uint) (*internalrepo.ParticipantRow, error)
	DeactivateParticipant(ctx context.Context, tx *gorm.DB, participantID uint, now time.Time) error
}

// Leave deactivates the caller's Participant, releasing their lobby slot
// while keeping the underlying SessionActor durable and host authority
// unaffected. See WORK-0001's Manager Operation Behavior - Leave.
func (m *Manager) Leave(ctx context.Context, sessionUUID SessionUUID, userUUID UserUUID, idempotencyKey IdempotencyKey) (LeaveResult, error) {
	defer logging.Step(ctx, "SessionLifecycle.Leave").Close()
	logging.LogFields(ctx,
		logging.Field("session_uuid", string(sessionUUID)),
		logging.Field("user_uuid", string(userUUID)),
	)

	if idempotencyKey == "" {
		return LeaveResult{}, session.ErrIdempotencyKeyRequired
	}

	incomingPayload := leaveRequestPayload{SessionUUID: string(sessionUUID), UserUUID: string(userUUID)}
	payloadBytes, err := marshalPayload(incomingPayload)
	if err != nil {
		return LeaveResult{}, err
	}

	var result LeaveResult
	var bizErr error
	txErr := m.tx.RunInTransaction(ctx, func(ctx context.Context, tx *gorm.DB) error {
		row, err := m.leaveRepo.LockSessionByUUID(ctx, tx, string(sessionUUID))
		if err != nil {
			return err
		}
		if row == nil {
			bizErr = session.ErrSessionNotFound
			return nil
		}

		now := m.now()
		if _, err := materializeExpirationIfDue(ctx, tx, m.leaveRepo, row, now); err != nil {
			return err
		}
		if row.Phase != session.PhaseLobby {
			// Any materialization above must still commit even though this
			// attempted Leave is rejected - see WORK-0001's Transaction
			// Ownership.
			bizErr = session.ErrNotInLobbyPhase
			return nil
		}

		sessionID := row.ID
		requestID, claimed, existing, err := m.leaveRepo.ClaimSessionRequest(ctx, tx, operationLeave, string(userUUID), string(idempotencyKey), &sessionID, payloadBytes)
		if err != nil {
			return err
		}
		if !claimed {
			result, bizErr = interpretExistingLeaveClaim(existing, incomingPayload)
			return nil
		}

		actor, err := m.leaveRepo.FindActor(ctx, tx, row.ID, string(userUUID))
		if err != nil {
			return err
		}
		if actor == nil {
			if err := m.leaveRepo.CompleteSessionRequest(ctx, tx, requestID, &sessionID, outcomeActorNotFound, ""); err != nil {
				return err
			}
			bizErr = session.ErrActorNotFound
			return nil
		}

		participant, err := m.leaveRepo.FindParticipant(ctx, tx, actor.ID)
		if err != nil {
			return err
		}
		if participant != nil && participant.Active {
			if err := m.leaveRepo.DeactivateParticipant(ctx, tx, participant.ID, now); err != nil {
				return err
			}
		}
		// A missing or already-inactive Participant is a harmless idempotent
		// no-op: there is no active slot left to release.

		result = LeaveResult{SessionUUID: SessionUUID(row.UUID)}
		responseBytes, err := marshalPayload(result)
		if err != nil {
			return err
		}
		if err := m.leaveRepo.CompleteSessionRequest(ctx, tx, requestID, &sessionID, outcomeLeft, responseBytes); err != nil {
			return err
		}
		return nil
	})
	if txErr != nil {
		return LeaveResult{}, txErr
	}
	if bizErr != nil {
		return LeaveResult{}, bizErr
	}
	return result, nil
}
