package leavesession

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/game/session/internal/idempotency"
	"github.com/diegobermudez03/playhoot/game/session/internal/sessionlock"
	"github.com/diegobermudez03/playhoot/utils"
	"gorm.io/gorm"
)

const (
	operationLeave = "LEAVE"
	outcomeLeft    = "LEFT"
)

type repoAPI interface {
	leaveSession(ctx context.Context, params leaveParams) (Result, error)
}

type repo struct {
	db *gorm.DB
}

func newRepo(db *gorm.DB) *repo {
	return &repo{db: db}
}

// GetDB satisfies utils.DBServicer so repo can own its own transaction
// boundary via utils.RunInDBTransaction.
func (r *repo) GetDB() *gorm.DB { return r.db }

type leaveParams struct {
	SessionUUID    string
	UserUUID       string
	IdempotencyKey string
}

// leaveRequestPayload is LEAVE's meaningful-field idempotency payload.
type leaveRequestPayload struct {
	SessionUUID string `json:"session_uuid"`
	UserUUID    string `json:"user_uuid"`
}

// leaveSession obtains the Session's per-Session DB mutation lock, rejects
// unless the Session is still LOBBY (lazily materializing an already-expired
// lobby first), and deactivates the caller's Participant without touching
// SessionActor durability or host authority. See WORK-0001's
// Session/Actor/Participant Operation Behavior - LeaveSession.
func (r *repo) leaveSession(ctx context.Context, params leaveParams) (Result, error) {
	return utils.RunInDBTransaction(ctx, r, func(ctx context.Context, tx *gorm.DB) (Result, error) {
		row, err := sessionlock.LockByUUID(ctx, tx, params.SessionUUID)
		if err != nil {
			return Result{}, err
		}
		if row == nil {
			return Result{}, session.ErrSessionNotFound
		}

		now := time.Now().UTC()
		if _, err := sessionlock.MaterializeExpirationIfDue(ctx, tx, row, now); err != nil {
			return Result{}, err
		}
		if row.Phase != session.PhaseLobby {
			return Result{}, session.ErrNotInLobbyPhase
		}

		incomingPayload := leaveRequestPayload{SessionUUID: params.SessionUUID, UserUUID: params.UserUUID}
		payloadBytes, err := json.Marshal(incomingPayload)
		if err != nil {
			return Result{}, fmt.Errorf("marshaling leave request payload: %s", err)
		}

		sessionID := row.ID
		requestID, claimed, existing, err := idempotency.Claim(ctx, tx, idempotency.ClaimInput{
			Operation:      operationLeave,
			UserUUID:       params.UserUUID,
			IdempotencyKey: params.IdempotencyKey,
			SessionID:      &sessionID,
			RequestPayload: string(payloadBytes),
		})
		if err != nil {
			return Result{}, fmt.Errorf("claiming leave idempotency key: %s", err)
		}
		if !claimed {
			return interpretExistingLeaveClaim(existing, incomingPayload)
		}

		actor, err := findActor(ctx, tx, row.ID, params.UserUUID)
		if err != nil {
			return Result{}, err
		}
		if actor == nil {
			return Result{}, session.ErrActorNotFound
		}

		participant, err := findParticipant(ctx, tx, actor.ID)
		if err != nil {
			return Result{}, err
		}
		if participant != nil && participant.Active {
			if err := deactivateParticipant(ctx, tx, participant.ID, now); err != nil {
				return Result{}, err
			}
		}
		// A missing or already-inactive Participant is a harmless idempotent
		// no-op: there is no active slot left to release.

		result := Result{SessionUUID: row.UUID}
		responseBytes, err := json.Marshal(result)
		if err != nil {
			return Result{}, fmt.Errorf("marshaling leave response payload: %s", err)
		}
		if err := idempotency.Complete(ctx, tx, requestID, &sessionID, outcomeLeft, string(responseBytes)); err != nil {
			return Result{}, err
		}

		return result, nil
	})
}

// actorRow is Leave's local view of a persisted session_actors record - a
// small persistence-code duplication with joinsession's own actorRow is
// preferred here over a shared horizontal actors package, since Join and
// Leave are separate behavior-local consumers whose query/locking needs may
// evolve independently (see repositories.md's Sharing Rule). Leave never
// creates/admits an actor, so it only needs the read side.
type actorRow struct {
	ID        uint
	SessionID uint
	UserUUID  string
}

// findActor returns the SessionActor for (sessionID, userUUID), or nil, nil
// if none exists yet.
func findActor(ctx context.Context, tx *gorm.DB, sessionID uint, userUUID string) (*actorRow, error) {
	var row actorRow
	result := tx.WithContext(ctx).Raw(`
		SELECT id, session_id, user_uuid
		FROM session_actors
		WHERE session_id = ? AND user_uuid = ?
	`, sessionID, userUUID).Scan(&row)
	if result.Error != nil {
		return nil, fmt.Errorf("finding session actor: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &row, nil
}

// participantRow is Leave's local view of a persisted session_participants
// record.
type participantRow struct {
	ID             uint
	SessionActorID uint
	DisplayName    string
	Active         bool
}

// findParticipant returns the Participant owned by actorID, or nil, nil if
// none has ever been created for it.
func findParticipant(ctx context.Context, tx *gorm.DB, actorID uint) (*participantRow, error) {
	var row participantRow
	result := tx.WithContext(ctx).Raw(`
		SELECT id, session_actor_id, display_name, active
		FROM session_participants
		WHERE session_actor_id = ?
	`, actorID).Scan(&row)
	if result.Error != nil {
		return nil, fmt.Errorf("finding participant: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &row, nil
}

// deactivateParticipant releases participantID's lobby slot.
func deactivateParticipant(ctx context.Context, tx *gorm.DB, participantID uint, now time.Time) error {
	if err := tx.WithContext(ctx).Exec(`
		UPDATE session_participants
		SET active = FALSE, left_at = ?
		WHERE id = ?
	`, now, participantID).Error; err != nil {
		return fmt.Errorf("deactivating participant: %s", err)
	}
	return nil
}

func interpretExistingLeaveClaim(existing *idempotency.Row, incoming leaveRequestPayload) (Result, error) {
	if existing.Status != idempotency.StatusCompleted {
		return Result{}, session.ErrIdempotencyInFlight
	}

	var storedPayload leaveRequestPayload
	if err := json.Unmarshal([]byte(existing.RequestPayload), &storedPayload); err != nil {
		return Result{}, fmt.Errorf("decoding stored leave request payload: %s", err)
	}
	if storedPayload != incoming {
		return Result{}, session.ErrIdempotencyConflict
	}

	if existing.ResponsePayload == nil {
		return Result{}, fmt.Errorf("completed leave idempotency record missing response payload")
	}
	var result Result
	if err := json.Unmarshal([]byte(*existing.ResponsePayload), &result); err != nil {
		return Result{}, fmt.Errorf("decoding stored leave response payload: %s", err)
	}
	return result, nil
}
