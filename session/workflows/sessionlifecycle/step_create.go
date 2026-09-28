package sessionlifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/session"
	internalrepo "github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
	"gorm.io/gorm"
)

// createRepoAPI is Create's own narrow persistence contract. Even though one
// concrete internal/repo.Repo happens to satisfy createRepoAPI/joinRepoAPI/
// leaveRepoAPI today, each step keeps its own contract naming only the
// methods that step actually calls, so a method added for one step never
// forces every other step's interface, mock, and test to change with it.
// ResolveCurrentGameDefinitionUUID resolves entirely from Session Runtime's
// own tables, so Create no longer depends on Game Management at all.
// ClaimSessionRequest/CompleteSessionRequest are this workflow's own
// idempotency-claim mechanics, not a domain-wide protocol; Create has no
// pre-existing Session row to lock, unlike every other step below.
type createRepoAPI interface {
	ResolveCurrentGameDefinitionUUID(ctx context.Context, gameUUID string) (*string, error)
	CreateSessionWithHost(ctx context.Context, tx *gorm.DB, gameDefinitionUUID string, hostUserUUID string, lobbyExpiresAt time.Time) (internalrepo.CreatedSession, error)
	CreateJoinCode(ctx context.Context, tx *gorm.DB, sessionID uint) (uint, error)
	ClaimSessionRequest(ctx context.Context, tx *gorm.DB, input internalrepo.ClaimSessionRequestInput) (requestID uint, existing *internalrepo.Request, err error)
	CompleteSessionRequest(ctx context.Context, tx *gorm.DB, requestID uint, sessionID *uint, outcome string, responsePayload string) error
}

// createRequestPayload is CREATE's meaningful-field idempotency payload (the
// host UserUUID is already implied by the idempotency identity itself).
type createRequestPayload struct {
	GameUUID string `json:"game_uuid"`
}

// outcomeCreated is CREATE's only recorded logical outcome label; Create has
// no deterministic decline outcome.
const outcomeCreated = "CREATED"

// Create resolves the Game's currently pinnable version from Session
// Runtime's own tables, then creates a LOBBY Session with a host
// SessionActor and an active JoinCode. Unlike the retired Game Language
// path, no compile-time script validation happens here: an authored
// JavaScript BackendScript is opaque to Session Runtime until the separately
// deployed JavaScript Executor actually evaluates it - there is no
// equivalent local check to perform at Create.
func (m *Manager) Create(ctx context.Context, gameUUID session.GameUUID, hostUserUUID session.UserUUID, idempotencyKey session.IdempotencyKey) (session.CreatedSession, error) {
	defer logging.Step(ctx, "SessionLifecycle.Create").Close()
	logging.LogFields(ctx,
		logging.Field("game_uuid", string(gameUUID)),
		logging.Field("host_user_uuid", string(hostUserUUID)),
	)

	if idempotencyKey == "" {
		return session.CreatedSession{}, session.ErrIdempotencyKeyRequired
	}

	currentDefinitionUUID, err := m.createRepo.ResolveCurrentGameDefinitionUUID(ctx, string(gameUUID))
	if err != nil {
		return session.CreatedSession{}, err
	}
	if currentDefinitionUUID == nil {
		return session.CreatedSession{}, session.ErrGameNotFound
	}

	return utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (session.CreatedSession, error) {
		return m.createSessionInTx(ctx, tx, gameUUID, *currentDefinitionUUID, hostUserUUID, idempotencyKey)
	})
}

// createSessionInTx is Create's per-transaction business logic, split out
// from the public Create so it can be exercised directly by
// mocked-collaborator unit tests without needing a real DB transaction -
// sessionlock/idempotency mechanism calls further down this same path
// require a real Postgres connection to run their SQL, which is instead
// proven by this package's repository-integration/concurrency tests.
func (m *Manager) createSessionInTx(ctx context.Context, tx *gorm.DB, gameUUID session.GameUUID, gameDefinitionUUID string, hostUserUUID session.UserUUID, idempotencyKey session.IdempotencyKey) (session.CreatedSession, error) {
	incomingPayload := createRequestPayload{GameUUID: string(gameUUID)}
	payloadBytes, err := json.Marshal(incomingPayload)
	if err != nil {
		return session.CreatedSession{}, fmt.Errorf("marshaling create request payload: %s", err)
	}

	requestID, existing, err := m.createRepo.ClaimSessionRequest(ctx, tx, internalrepo.ClaimSessionRequestInput{
		Operation:      operationCreate,
		UserUUID:       string(hostUserUUID),
		IdempotencyKey: string(idempotencyKey),
		RequestPayload: string(payloadBytes),
	})
	if err != nil {
		return session.CreatedSession{}, fmt.Errorf("claiming create session request: %s", err)
	}
	if existing != nil {
		return interpretExistingCreateClaim(existing, incomingPayload)
	}

	now := time.Now().UTC()
	created, err := m.createRepo.CreateSessionWithHost(ctx, tx, gameDefinitionUUID, string(hostUserUUID), now.Add(m.lobbyTTL))
	if err != nil {
		return session.CreatedSession{}, err
	}

	joinCode, err := m.createRepo.CreateJoinCode(ctx, tx, created.SessionID)
	if err != nil {
		return session.CreatedSession{}, err
	}

	result := session.CreatedSession{
		SessionUUID:    session.SessionUUID(created.SessionUUID),
		JoinCode:       session.JoinCode(joinCode),
		LobbyExpiresAt: created.LobbyExpiresAt,
	}
	responseBytes, err := json.Marshal(result)
	if err != nil {
		return session.CreatedSession{}, fmt.Errorf("marshaling create response payload: %s", err)
	}

	sessionID := created.SessionID
	if err := m.createRepo.CompleteSessionRequest(ctx, tx, requestID, &sessionID, outcomeCreated, string(responseBytes)); err != nil {
		return session.CreatedSession{}, fmt.Errorf("completing create session request: %s", err)
	}
	return result, nil
}

// interpretExistingCreateClaim decides what an already-claimed CREATE
// identity means for the incoming request: replay or conflict. This is
// Manager policy - the shared idempotency mechanism only reports the
// existing request.
func interpretExistingCreateClaim(existing *internalrepo.Request, incoming createRequestPayload) (session.CreatedSession, error) {
	if existing.Status != internalrepo.RequestStatusCompleted {
		return session.CreatedSession{}, session.ErrIdempotencyInFlight
	}

	var stored createRequestPayload
	if err := json.Unmarshal([]byte(existing.RequestPayload), &stored); err != nil {
		return session.CreatedSession{}, fmt.Errorf("decoding stored create request payload: %s", err)
	}
	if stored != incoming {
		return session.CreatedSession{}, session.ErrIdempotencyConflict
	}

	if existing.ResponsePayload == nil {
		return session.CreatedSession{}, fmt.Errorf("completed create idempotency record missing response payload")
	}
	var result session.CreatedSession
	if err := json.Unmarshal([]byte(*existing.ResponsePayload), &result); err != nil {
		return session.CreatedSession{}, fmt.Errorf("decoding stored create response payload: %s", err)
	}
	return result, nil
}
