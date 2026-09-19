package sessionlifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/game/session/internal/idempotency"
	"github.com/diegobermudez03/playhoot/game/session/internal/sessionlock"
	internalrepo "github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/utils"
	"gorm.io/gorm"
)

// outcomeActorNotFound is the outcome label persisted to
// session_requests.outcome for a deterministic post-claim decline that must
// survive as a replayable outcome (WORK-0001's Transaction Ownership).
// outcomeLeft is likewise recorded so a same-token replay can tell success
// from a decline.
const (
	outcomeLeft          = "LEFT"
	outcomeActorNotFound = "ACTOR_NOT_FOUND"
)

// leaveRepoAPI is Leave's own narrow persistence contract (see
// createRepoAPI's doc comment on why this is not shared verbatim with
// Create/Join despite some overlapping method shapes). The shared
// sessionlock/idempotency mechanism packages are called directly by this
// step instead of through repository forwarding methods
// (`docs/engineering/standards/repositories.md`'s Sharing Rule).
type leaveRepoAPI interface {
	expirationStore
	FindActor(ctx context.Context, tx *gorm.DB, sessionID uint, userUUID string) (*internalrepo.Actor, error)
	FindParticipant(ctx context.Context, tx *gorm.DB, actorID uint) (*internalrepo.Participant, error)
	DeactivateParticipant(ctx context.Context, tx *gorm.DB, participantID uint, leftAt time.Time) error
}

// leaveRequestPayload is LEAVE's meaningful-field idempotency payload.
type leaveRequestPayload struct {
	SessionUUID string `json:"session_uuid"`
	UserUUID    string `json:"user_uuid"`
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

	return utils.RunInDBTransaction(ctx, m, func(ctx context.Context, tx *gorm.DB) (LeaveResult, error) {
		return m.leaveSessionInTx(ctx, tx, sessionUUID, userUUID, idempotencyKey)
	})
}

// leaveSessionInTx is Leave's per-transaction business logic, split out from
// the public Leave so it can be exercised directly by mocked-collaborator
// unit tests without needing a real DB transaction - sessionlock/idempotency
// mechanism calls further down this same path require a real Postgres
// connection to run their SQL, which is instead proven by this package's
// repository-integration/concurrency tests.
func (m *Manager) leaveSessionInTx(ctx context.Context, tx *gorm.DB, sessionUUID SessionUUID, userUUID UserUUID, idempotencyKey IdempotencyKey) (LeaveResult, error) {
	incomingPayload := leaveRequestPayload{SessionUUID: string(sessionUUID), UserUUID: string(userUUID)}

	lockedSession, err := sessionlock.LockByUUID(ctx, tx, string(sessionUUID))
	if err != nil {
		return LeaveResult{}, err
	}
	if lockedSession == nil {
		return LeaveResult{}, session.ErrSessionNotFound
	}

	now := time.Now().UTC()
	if _, err := materializeExpirationIfDue(ctx, tx, m.leaveRepo, lockedSession, now); err != nil {
		return LeaveResult{}, err
	}
	if lockedSession.Phase != session.PhaseLobby {
		// Any materialization above must still commit even though this
		// attempted Leave is rejected - it already has, via this same
		// callback's eventual successful return (WORK-0001's Transaction
		// Ownership). This rejection is discovered before any idempotency
		// claim, so there is no token-scoped outcome to record.
		return LeaveResult{Outcome: LeaveOutcomeNotInLobby}, nil
	}

	payloadBytes, err := json.Marshal(incomingPayload)
	if err != nil {
		return LeaveResult{}, fmt.Errorf("marshaling leave request payload: %s", err)
	}
	requestID, existing, err := idempotency.Claim(ctx, tx, idempotency.ClaimInput{
		Operation:      operationLeave,
		UserUUID:       string(userUUID),
		IdempotencyKey: string(idempotencyKey),
		SessionID:      &lockedSession.ID,
		RequestPayload: string(payloadBytes),
	})
	if err != nil {
		return LeaveResult{}, fmt.Errorf("claiming leave session request: %s", err)
	}
	if existing != nil {
		return interpretExistingLeaveClaim(existing, incomingPayload)
	}

	actor, err := m.leaveRepo.FindActor(ctx, tx, lockedSession.ID, string(userUUID))
	if err != nil {
		return LeaveResult{}, err
	}
	if actor == nil {
		if err := idempotency.Complete(ctx, tx, requestID, &lockedSession.ID, outcomeActorNotFound, ""); err != nil {
			return LeaveResult{}, fmt.Errorf("completing leave session request: %s", err)
		}
		return LeaveResult{Outcome: LeaveOutcomeActorNotFound}, nil
	}

	participant, err := m.leaveRepo.FindParticipant(ctx, tx, actor.ID)
	if err != nil {
		return LeaveResult{}, err
	}
	if participant != nil && participant.Active {
		if err := m.leaveRepo.DeactivateParticipant(ctx, tx, participant.ID, now); err != nil {
			return LeaveResult{}, err
		}
	}
	// A missing or already-inactive Participant is a harmless idempotent
	// no-op: there is no active slot left to release.

	result := LeaveResult{Outcome: LeaveOutcomeLeft, SessionUUID: SessionUUID(lockedSession.UUID)}
	responseBytes, err := json.Marshal(result)
	if err != nil {
		return LeaveResult{}, fmt.Errorf("marshaling leave response payload: %s", err)
	}
	if err := idempotency.Complete(ctx, tx, requestID, &lockedSession.ID, outcomeLeft, string(responseBytes)); err != nil {
		return LeaveResult{}, fmt.Errorf("completing leave session request: %s", err)
	}
	return result, nil
}

// interpretExistingLeaveClaim decides what an already-claimed LEAVE identity
// means for the incoming request: replay or conflict
// (`docs/engineering/standards/idempotency.md`'s Token Semantics). A
// replayed decline is returned as the same outcome value it was originally
// recorded as (GAME-ADR-0022), never reconstructed as an error.
func interpretExistingLeaveClaim(existing *idempotency.Request, incoming leaveRequestPayload) (LeaveResult, error) {
	if existing.Status != idempotency.StatusCompleted {
		return LeaveResult{}, session.ErrIdempotencyInFlight
	}

	var stored leaveRequestPayload
	if err := json.Unmarshal([]byte(existing.RequestPayload), &stored); err != nil {
		return LeaveResult{}, fmt.Errorf("decoding stored leave request payload: %s", err)
	}
	if stored != incoming {
		return LeaveResult{}, session.ErrIdempotencyConflict
	}

	if existing.Outcome == outcomeActorNotFound {
		return LeaveResult{Outcome: LeaveOutcomeActorNotFound}, nil
	}

	if existing.ResponsePayload == nil {
		return LeaveResult{}, fmt.Errorf("completed leave idempotency record missing response payload")
	}
	var result LeaveResult
	if err := json.Unmarshal([]byte(*existing.ResponsePayload), &result); err != nil {
		return LeaveResult{}, fmt.Errorf("decoding stored leave response payload: %s", err)
	}
	return result, nil
}
