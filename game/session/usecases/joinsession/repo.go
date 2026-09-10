package joinsession

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/game/session/internal/actors"
	"github.com/diegobermudez03/playhoot/game/session/internal/idempotency"
	"github.com/diegobermudez03/playhoot/game/session/internal/sessionlock"
	"github.com/diegobermudez03/playhoot/utils"
	"gorm.io/gorm"
)

const (
	operationJoin = "JOIN"
	outcomeJoined = "JOINED"
)

type repoAPI interface {
	resolveActiveSessionForJoinCode(ctx context.Context, joinCode uint) (*joinCodeResolution, error)
	joinSession(ctx context.Context, params joinParams) (Result, error)
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

type joinCodeResolution struct {
	SessionID          uint
	GameDefinitionUUID string
}

// resolveActiveSessionForJoinCode is an unlocked lookup: it identifies which
// Session an active JoinCode currently belongs to, and that Session's pinned
// Definition/Version UUID, so the caller can read the Game Management
// pinned-definition capability before opening the mutation transaction. The
// Session's admissibility itself is always re-validated under lock inside
// joinSession - see WORK-0001's Game Management Dependency (either order is
// correctness-safe since the pinned definition cannot change underneath the
// Session).
func (r *repo) resolveActiveSessionForJoinCode(ctx context.Context, joinCode uint) (*joinCodeResolution, error) {
	var resolution joinCodeResolution
	result := r.db.WithContext(ctx).Raw(`
		SELECT s.id AS session_id, s.game_definition_uuid
		FROM sessions s
		INNER JOIN join_codes jc ON jc.session_id = s.id
		WHERE jc.code = ? AND jc.revoked_at IS NULL
	`, joinCode).Scan(&resolution)
	if result.Error != nil {
		return nil, fmt.Errorf("resolving join code: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &resolution, nil
}

type joinParams struct {
	SessionID      uint
	JoinCode       uint
	UserUUID       string
	DisplayName    string
	IdempotencyKey string
	PlayersMax     int
}

// joinRequestPayload is JOIN's meaningful-field idempotency payload.
type joinRequestPayload struct {
	JoinCode    uint   `json:"join_code"`
	UserUUID    string `json:"user_uuid"`
	DisplayName string `json:"display_name"`
}

// joinSession obtains the Session's per-Session DB mutation lock, revalidates
// LOBBY admissibility (lazily materializing an already-expired lobby),
// enforces the pinned Definition's players.max, and activates a Participant
// - reusing an existing SessionActor/Participant where one already exists
// for (session, user_uuid). See WORK-0001's Session/Actor/Participant
// Operation Behavior - JoinSession.
func (r *repo) joinSession(ctx context.Context, params joinParams) (Result, error) {
	return utils.RunInDBTransaction(ctx, r, func(ctx context.Context, tx *gorm.DB) (Result, error) {
		row, err := sessionlock.LockByID(ctx, tx, params.SessionID)
		if err != nil {
			return Result{}, err
		}
		if row == nil {
			return Result{}, session.ErrSessionNotFound
		}

		now := time.Now().UTC()
		expired, err := sessionlock.MaterializeExpirationIfDue(ctx, tx, row, now)
		if err != nil {
			return Result{}, err
		}
		if expired || row.Phase != session.PhaseLobby {
			return Result{}, session.ErrLobbyExpired
		}

		incomingPayload := joinRequestPayload{
			JoinCode:    params.JoinCode,
			UserUUID:    params.UserUUID,
			DisplayName: params.DisplayName,
		}
		payloadBytes, err := json.Marshal(incomingPayload)
		if err != nil {
			return Result{}, fmt.Errorf("marshaling join request payload: %s", err)
		}

		sessionID := row.ID
		requestID, claimed, existing, err := idempotency.Claim(ctx, tx, idempotency.ClaimInput{
			Operation:      operationJoin,
			UserUUID:       params.UserUUID,
			IdempotencyKey: params.IdempotencyKey,
			SessionID:      &sessionID,
			RequestPayload: string(payloadBytes),
		})
		if err != nil {
			return Result{}, fmt.Errorf("claiming join idempotency key: %s", err)
		}
		if !claimed {
			return interpretExistingJoinClaim(existing, incomingPayload)
		}

		actorID, err := findOrCreateActor(ctx, tx, row.ID, params.UserUUID, now)
		if err != nil {
			return Result{}, err
		}

		if err := admitParticipant(ctx, tx, row.ID, actorID, params.DisplayName, params.PlayersMax, now); err != nil {
			return Result{}, err
		}

		result := Result{SessionUUID: row.UUID, DisplayName: params.DisplayName}
		responseBytes, err := json.Marshal(result)
		if err != nil {
			return Result{}, fmt.Errorf("marshaling join response payload: %s", err)
		}
		if err := idempotency.Complete(ctx, tx, requestID, &sessionID, outcomeJoined, string(responseBytes)); err != nil {
			return Result{}, err
		}

		return result, nil
	})
}

func findOrCreateActor(ctx context.Context, tx *gorm.DB, sessionID uint, userUUID string, now time.Time) (uint, error) {
	actor, err := actors.Find(ctx, tx, sessionID, userUUID)
	if err != nil {
		return 0, err
	}
	if actor != nil {
		return actor.ID, nil
	}
	return actors.Create(ctx, tx, sessionID, userUUID, now)
}

func admitParticipant(ctx context.Context, tx *gorm.DB, sessionID, actorID uint, displayName string, playersMax int, now time.Time) error {
	participant, err := actors.FindParticipant(ctx, tx, actorID)
	if err != nil {
		return err
	}

	if participant != nil && participant.Active {
		// Repeated identical active Join is the same logical admission, not
		// a new slot - just refresh the display-name snapshot.
		return actors.RefreshActiveDisplayName(ctx, tx, participant.ID, displayName)
	}

	activeCount, err := actors.CountActive(ctx, tx, sessionID)
	if err != nil {
		return err
	}
	if playersMax > 0 && activeCount >= playersMax {
		return session.ErrLobbyFull
	}

	if participant == nil {
		return actors.CreateParticipant(ctx, tx, actorID, displayName, now)
	}
	return actors.ReactivateParticipant(ctx, tx, participant.ID, displayName, now)
}

func interpretExistingJoinClaim(existing *idempotency.Row, incoming joinRequestPayload) (Result, error) {
	if existing.Status != idempotency.StatusCompleted {
		return Result{}, session.ErrIdempotencyInFlight
	}

	var storedPayload joinRequestPayload
	if err := json.Unmarshal([]byte(existing.RequestPayload), &storedPayload); err != nil {
		return Result{}, fmt.Errorf("decoding stored join request payload: %s", err)
	}
	if storedPayload != incoming {
		return Result{}, session.ErrIdempotencyConflict
	}

	if existing.ResponsePayload == nil {
		return Result{}, fmt.Errorf("completed join idempotency record missing response payload")
	}
	var result Result
	if err := json.Unmarshal([]byte(*existing.ResponsePayload), &result); err != nil {
		return Result{}, fmt.Errorf("decoding stored join response payload: %s", err)
	}
	return result, nil
}
