package sessionlifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/game/management"
	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/game/session/internal/idempotency"
	internalrepo "github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/utils"
	"gorm.io/gorm"
)

// gameCurrentVersionReader is the narrow Game Management read capability
// Create depends on to resolve the Game's current playable
// Definition/Version to pin (GAME-ADR-0001, GAME-ADR-0004).
type gameCurrentVersionReader interface {
	GetPlayableGameWithCurrentVersion(ctx context.Context, gameUUID string) (*management.Game, error)
}

// createRepoAPI is Create's own narrow persistence contract. Even though one
// concrete internal/repo.Repo happens to satisfy createRepoAPI/joinRepoAPI/
// leaveRepoAPI today, each step keeps its own contract
// (`docs/engineering/standards/domain-logic-placement.md`'s Workflow
// Grouping Does Not Imply A Shared Repository Contract). The shared
// sessionlock/idempotency mechanism packages are called directly by this
// step instead of through repository forwarding methods
// (`docs/engineering/standards/repositories.md`'s Sharing Rule).
type createRepoAPI interface {
	CreateSessionWithHost(ctx context.Context, tx *gorm.DB, gameDefinitionUUID string, hostUserUUID string, lobbyExpiresAt time.Time) (internalrepo.CreatedSession, error)
	CreateJoinCode(ctx context.Context, tx *gorm.DB, sessionID uint) (uint, error)
}

// createRequestPayload is CREATE's meaningful-field idempotency payload (the
// host UserUUID is already implied by the idempotency identity itself).
type createRequestPayload struct {
	GameUUID string `json:"game_uuid"`
}

// outcomeCreated is CREATE's only recorded logical outcome label (Create has
// no deterministic decline outcome in this WORK's scope).
const outcomeCreated = "CREATED"

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

	return utils.RunInDBTransaction(ctx, m, func(ctx context.Context, tx *gorm.DB) (CreatedSession, error) {
		return m.createSessionInTx(ctx, tx, gameUUID, playableGame.VersionUUID, hostUserUUID, idempotencyKey)
	})
}

// createSessionInTx is Create's per-transaction business logic, split out
// from the public Create so it can be exercised directly by
// mocked-collaborator unit tests without needing a real DB transaction -
// sessionlock/idempotency mechanism calls further down this same path
// require a real Postgres connection to run their SQL, which is instead
// proven by this package's repository-integration/concurrency tests.
func (m *Manager) createSessionInTx(ctx context.Context, tx *gorm.DB, gameUUID GameUUID, gameDefinitionUUID string, hostUserUUID UserUUID, idempotencyKey IdempotencyKey) (CreatedSession, error) {
	incomingPayload := createRequestPayload{GameUUID: string(gameUUID)}
	payloadBytes, err := json.Marshal(incomingPayload)
	if err != nil {
		return CreatedSession{}, fmt.Errorf("marshaling create request payload: %s", err)
	}

	requestID, existing, err := idempotency.Claim(ctx, tx, idempotency.ClaimInput{
		Operation:      operationCreate,
		UserUUID:       string(hostUserUUID),
		IdempotencyKey: string(idempotencyKey),
		RequestPayload: string(payloadBytes),
	})
	if err != nil {
		return CreatedSession{}, fmt.Errorf("claiming create session request: %s", err)
	}
	if existing != nil {
		return interpretExistingCreateClaim(existing, incomingPayload)
	}

	now := time.Now().UTC()
	created, err := m.createRepo.CreateSessionWithHost(ctx, tx, gameDefinitionUUID, string(hostUserUUID), now.Add(m.lobbyTTL))
	if err != nil {
		return CreatedSession{}, err
	}

	joinCode, err := m.createRepo.CreateJoinCode(ctx, tx, created.SessionID)
	if err != nil {
		return CreatedSession{}, err
	}

	result := CreatedSession{
		SessionUUID:    SessionUUID(created.SessionUUID),
		JoinCode:       JoinCode(joinCode),
		LobbyExpiresAt: created.LobbyExpiresAt,
	}
	responseBytes, err := json.Marshal(result)
	if err != nil {
		return CreatedSession{}, fmt.Errorf("marshaling create response payload: %s", err)
	}

	sessionID := created.SessionID
	if err := idempotency.Complete(ctx, tx, requestID, &sessionID, outcomeCreated, string(responseBytes)); err != nil {
		return CreatedSession{}, fmt.Errorf("completing create session request: %s", err)
	}
	return result, nil
}

// interpretExistingCreateClaim decides what an already-claimed CREATE
// identity means for the incoming request: replay or conflict
// (`docs/engineering/standards/idempotency.md`'s Token Semantics). This is
// Manager policy - the shared idempotency mechanism only reports the
// existing request.
func interpretExistingCreateClaim(existing *idempotency.Request, incoming createRequestPayload) (CreatedSession, error) {
	if existing.Status != idempotency.StatusCompleted {
		return CreatedSession{}, session.ErrIdempotencyInFlight
	}

	var stored createRequestPayload
	if err := json.Unmarshal([]byte(existing.RequestPayload), &stored); err != nil {
		return CreatedSession{}, fmt.Errorf("decoding stored create request payload: %s", err)
	}
	if stored != incoming {
		return CreatedSession{}, session.ErrIdempotencyConflict
	}

	if existing.ResponsePayload == nil {
		return CreatedSession{}, fmt.Errorf("completed create idempotency record missing response payload")
	}
	var result CreatedSession
	if err := json.Unmarshal([]byte(*existing.ResponsePayload), &result); err != nil {
		return CreatedSession{}, fmt.Errorf("decoding stored create response payload: %s", err)
	}
	return result, nil
}
