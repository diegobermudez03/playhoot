package createsession

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/game/session/internal/idempotency"
	"github.com/diegobermudez03/playhoot/game/session/internal/pgerrs"
	"github.com/diegobermudez03/playhoot/utils"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	operationCreate = "CREATE"
	outcomeCreated  = "CREATED"

	// maxJoinCodeAttempts bounds retries generating a numeric JoinCode that
	// does not collide with another currently-active code.
	maxJoinCodeAttempts = 10
)

type repoAPI interface {
	createSession(ctx context.Context, params createParams) (Result, error)
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

type createParams struct {
	GameUUID           string
	HostUserUUID       string
	IdempotencyKey     string
	GameDefinitionUUID string
	LobbyTTL           time.Duration
}

// createRequestPayload is CREATE's meaningful-field idempotency payload
// (the host UserUUID is already implied by the idempotency identity itself).
type createRequestPayload struct {
	GameUUID string `json:"game_uuid"`
}

// createSession claims the (user_uuid, CREATE, idempotency_key) identity and,
// if this call owns the claim, performs the Host/SessionActor Creation Cycle
// and issues an active JoinCode, all within one transaction - see WORK-0001's
// Concurrent Create Correctness and Host/SessionActor Creation Cycle.
func (r *repo) createSession(ctx context.Context, params createParams) (Result, error) {
	return utils.RunInDBTransaction(ctx, r, func(ctx context.Context, tx *gorm.DB) (Result, error) {
		incomingPayload := createRequestPayload{GameUUID: params.GameUUID}
		payloadBytes, err := json.Marshal(incomingPayload)
		if err != nil {
			return Result{}, fmt.Errorf("marshaling create request payload: %s", err)
		}

		requestID, claimed, existing, err := idempotency.Claim(ctx, tx, idempotency.ClaimInput{
			Operation:      operationCreate,
			UserUUID:       params.HostUserUUID,
			IdempotencyKey: params.IdempotencyKey,
			RequestPayload: string(payloadBytes),
		})
		if err != nil {
			return Result{}, fmt.Errorf("claiming create idempotency key: %s", err)
		}

		if !claimed {
			return interpretExistingCreateClaim(existing, incomingPayload)
		}

		now := time.Now().UTC()

		sessionRow := sessionInsert{
			UUID:               uuid.NewString(),
			GameDefinitionUUID: params.GameDefinitionUUID,
			Phase:              session.PhaseLobby,
			LobbyExpiresAt:     now.Add(params.LobbyTTL),
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if err := tx.Create(&sessionRow).Error; err != nil {
			return Result{}, fmt.Errorf("creating session: %s", err)
		}

		actorRow := actorInsert{
			SessionID:        sessionRow.ID,
			UserUUID:         params.HostUserUUID,
			SemanticPresence: session.PresenceConnected,
			CreatedAt:        now,
		}
		if err := tx.Create(&actorRow).Error; err != nil {
			return Result{}, fmt.Errorf("creating host session actor: %s", err)
		}

		if err := tx.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, actorRow.ID, sessionRow.ID).Error; err != nil {
			return Result{}, fmt.Errorf("assigning host actor to session: %s", err)
		}

		joinCode, err := createActiveJoinCode(ctx, tx, sessionRow.ID, now)
		if err != nil {
			return Result{}, err
		}

		result := Result{
			SessionUUID:    sessionRow.UUID,
			JoinCode:       joinCode,
			LobbyExpiresAt: sessionRow.LobbyExpiresAt,
		}
		responseBytes, err := json.Marshal(result)
		if err != nil {
			return Result{}, fmt.Errorf("marshaling create response payload: %s", err)
		}

		sessionID := sessionRow.ID
		if err := idempotency.Complete(ctx, tx, requestID, &sessionID, outcomeCreated, string(responseBytes)); err != nil {
			return Result{}, err
		}

		return result, nil
	})
}

func interpretExistingCreateClaim(existing *idempotency.Row, incoming createRequestPayload) (Result, error) {
	if existing.Status != idempotency.StatusCompleted {
		return Result{}, session.ErrIdempotencyInFlight
	}

	var storedPayload createRequestPayload
	if err := json.Unmarshal([]byte(existing.RequestPayload), &storedPayload); err != nil {
		return Result{}, fmt.Errorf("decoding stored create request payload: %s", err)
	}
	if storedPayload != incoming {
		return Result{}, session.ErrIdempotencyConflict
	}

	if existing.ResponsePayload == nil {
		return Result{}, fmt.Errorf("completed create idempotency record missing response payload")
	}
	var result Result
	if err := json.Unmarshal([]byte(*existing.ResponsePayload), &result); err != nil {
		return Result{}, fmt.Errorf("decoding stored create response payload: %s", err)
	}
	return result, nil
}

func createActiveJoinCode(ctx context.Context, tx *gorm.DB, sessionID uint, now time.Time) (uint, error) {
	for attempt := 0; attempt < maxJoinCodeAttempts; attempt++ {
		code := uint(rand.Intn(9000) + 1000) //nolint:gosec // not security sensitive, just a human-facing lobby code
		row := joinCodeInsert{
			SessionID: sessionID,
			Code:      code,
			CreatedAt: now,
		}
		err := tx.Create(&row).Error
		if err == nil {
			return code, nil
		}
		if !pgerrs.IsUniqueViolation(err) {
			return 0, fmt.Errorf("creating join code: %s", err)
		}
	}
	return 0, fmt.Errorf("creating join code: exhausted %d attempts generating a unique active code", maxJoinCodeAttempts)
}

type sessionInsert struct {
	ID                 uint      `gorm:"column:id"`
	UUID               string    `gorm:"column:uuid"`
	GameDefinitionUUID string    `gorm:"column:game_definition_uuid"`
	Phase              string    `gorm:"column:phase"`
	LobbyExpiresAt     time.Time `gorm:"column:lobby_expires_at"`
	CreatedAt          time.Time `gorm:"column:created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}

func (sessionInsert) TableName() string { return "sessions" }

type actorInsert struct {
	ID               uint      `gorm:"column:id"`
	SessionID        uint      `gorm:"column:session_id"`
	UserUUID         string    `gorm:"column:user_uuid"`
	SemanticPresence string    `gorm:"column:semantic_presence"`
	CreatedAt        time.Time `gorm:"column:created_at"`
}

func (actorInsert) TableName() string { return "session_actors" }

type joinCodeInsert struct {
	ID        uint      `gorm:"column:id"`
	SessionID uint      `gorm:"column:session_id"`
	Code      uint      `gorm:"column:code"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (joinCodeInsert) TableName() string { return "join_codes" }
