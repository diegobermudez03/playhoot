package sessionlifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/game/language/v1/program"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/session"
	internalrepo "github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
	"gorm.io/gorm"
)

// Join outcome labels persisted to session_requests.outcome for a
// deterministic post-claim decline that must survive as a replayable
// outcome. JOINED itself is also recorded so a same-token replay can tell
// success from a decline.
const (
	outcomeJoined        = "JOINED"
	outcomeAlreadyJoined = "ALREADY_JOINED"
	outcomeLobbyFull     = "LOBBY_FULL"
)

// gamePinnedDefinitionReader is the narrow Game Management read capability
// Join depends on to load an already-pinned immutable Game Definition by its
// own Definition/Version UUID - never by re-resolving the Game's current
// version.
type gamePinnedDefinitionReader interface {
	GetGameDefinition(ctx context.Context, gameDefinitionUUID string) (*program.Definition, error)
}

// joinRepoAPI is Join's own narrow persistence contract. LockSessionByID/
// ClaimSessionRequest/CompleteSessionRequest are this workflow's own
// locking/idempotency-claim mechanics, not a domain-wide protocol.
type joinRepoAPI interface {
	ResolveSessionForJoinCode(ctx context.Context, joinCode uint) (*internalrepo.JoinCodeResolution, error)
	LockSessionByID(ctx context.Context, tx *gorm.DB, sessionID uint) (*internalrepo.Session, error)
	FindActor(ctx context.Context, tx *gorm.DB, sessionID uint, userUUID string) (*internalrepo.Actor, error)
	CreateActor(ctx context.Context, tx *gorm.DB, sessionID uint, userUUID string) (uint, error)
	FindParticipant(ctx context.Context, tx *gorm.DB, actorID uint) (*internalrepo.Participant, error)
	CountActiveParticipants(ctx context.Context, tx *gorm.DB, sessionID uint) (int, error)
	CreateParticipant(ctx context.Context, tx *gorm.DB, actorID uint, displayName string, joinedAt time.Time) error
	ActivateParticipant(ctx context.Context, tx *gorm.DB, participantID uint, displayName string, joinedAt time.Time) error
	ClaimSessionRequest(ctx context.Context, tx *gorm.DB, input internalrepo.ClaimSessionRequestInput) (requestID uint, existing *internalrepo.Request, err error)
	CompleteSessionRequest(ctx context.Context, tx *gorm.DB, requestID uint, sessionID *uint, outcome string, responsePayload string) error
}

// joinRequestPayload is JOIN's meaningful-field idempotency payload.
type joinRequestPayload struct {
	JoinCode    uint   `json:"join_code"`
	UserUUID    string `json:"user_uuid"`
	DisplayName string `json:"display_name"`
}

// Join resolves an active JoinCode to its Session, loads that Session's
// pinned immutable Game Definition, and admits the caller as an active
// Participant under the Session's per-Session DB mutation lock.
func (m *Manager) Join(ctx context.Context, joinCode session.JoinCode, userUUID session.UserUUID, displayName session.DisplayName, idempotencyKey session.IdempotencyKey) (session.JoinResult, error) {
	defer logging.Step(ctx, "SessionLifecycle.Join").Close()
	logging.LogFields(ctx,
		logging.Field("join_code", uint(joinCode)),
		logging.Field("user_uuid", string(userUUID)),
	)

	if idempotencyKey == "" {
		return session.JoinResult{}, session.ErrIdempotencyKeyRequired
	}

	// A currently-active assignment can still be concurrently revoked by
	// another operation's lazy lobby-expiration materialization between this
	// unlocked read and the lock below; that race surfaces as the ordinary
	// LobbyExpired outcome once locked, not a hard error here - see
	// joinSessionInTx.
	resolution, err := m.joinRepo.ResolveSessionForJoinCode(ctx, uint(joinCode))
	if err != nil {
		return session.JoinResult{}, err
	}
	if resolution == nil {
		return session.JoinResult{}, session.ErrJoinCodeInvalid
	}

	// Read before the mutation transaction/row lock opens, so lobby capacity
	// stays governed by the exact version this Session was pinned to at
	// Create, not whatever the lock might observe by the time it opens.
	definition, err := m.pinnedGameReader.GetGameDefinition(ctx, resolution.GameDefinitionUUID)
	if err != nil {
		return session.JoinResult{}, err
	}
	if definition == nil {
		monitoring.Alert(ctx, "session pinned game definition is missing")
		return session.JoinResult{}, session.ErrPinnedDefinitionMissing
	}
	playersMax := definition.Players.Max

	return utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (session.JoinResult, error) {
		return m.joinSessionInTx(ctx, tx, resolution.SessionID, playersMax, joinCode, userUUID, displayName, idempotencyKey)
	})
}

// joinSessionInTx is Join's per-transaction business logic, split out from
// the public Join so it can be exercised directly by mocked-collaborator
// unit tests without needing a real DB transaction - sessionlock/idempotency
// mechanism calls further down this same path require a real Postgres
// connection to run their SQL, which is instead proven by this package's
// repository-integration/concurrency tests.
func (m *Manager) joinSessionInTx(ctx context.Context, tx *gorm.DB, sessionID uint, playersMax int, joinCode session.JoinCode, userUUID session.UserUUID, displayName session.DisplayName, idempotencyKey session.IdempotencyKey) (session.JoinResult, error) {
	incomingPayload := joinRequestPayload{JoinCode: uint(joinCode), UserUUID: string(userUUID), DisplayName: string(displayName)}

	lockedSession, err := m.joinRepo.LockSessionByID(ctx, tx, sessionID)
	if err != nil {
		return session.JoinResult{}, err
	}
	if lockedSession == nil {
		return session.JoinResult{}, session.ErrSessionNotFound
	}

	now := time.Now().UTC()
	if _, err := m.lobbyExpirer.MaterializeIfDue(ctx, tx, lockedSession, now); err != nil {
		return session.JoinResult{}, err
	}
	if lockedSession.Phase != session.PhaseLobby {
		// A rejection discovered before any idempotency claim is attempted
		// never reaches session_requests at all - there is no token-scoped
		// outcome to record. Any materialization above must still commit
		// even though this attempted Join is rejected - it already has, via
		// this same callback's eventual successful return.
		return session.JoinResult{Outcome: session.JoinOutcomeLobbyExpired}, nil
	}

	payloadBytes, err := json.Marshal(incomingPayload)
	if err != nil {
		return session.JoinResult{}, fmt.Errorf("marshaling join request payload: %s", err)
	}
	requestID, existing, err := m.joinRepo.ClaimSessionRequest(ctx, tx, internalrepo.ClaimSessionRequestInput{
		Operation:      operationJoin,
		UserUUID:       string(userUUID),
		IdempotencyKey: string(idempotencyKey),
		SessionID:      &lockedSession.ID,
		RequestPayload: string(payloadBytes),
	})
	if err != nil {
		return session.JoinResult{}, fmt.Errorf("claiming join session request: %s", err)
	}
	if existing != nil {
		return interpretExistingJoinClaim(existing, incomingPayload)
	}

	actor, err := m.joinRepo.FindActor(ctx, tx, lockedSession.ID, string(userUUID))
	if err != nil {
		return session.JoinResult{}, err
	}

	var actorID uint
	var participant *internalrepo.Participant
	if actor != nil {
		actorID = actor.ID
		participant, err = m.joinRepo.FindParticipant(ctx, tx, actorID)
		if err != nil {
			return session.JoinResult{}, err
		}
	} else {
		actorID, err = m.joinRepo.CreateActor(ctx, tx, lockedSession.ID, string(userUUID))
		if err != nil {
			return session.JoinResult{}, err
		}
	}

	if participant != nil && participant.Active {
		// A *different* idempotency token than any previously used for
		// this user's admission, evaluated against current state, while
		// already an active Participant: a new command, rejected - not
		// silently replayed or treated as success. The rejection is itself
		// the token's completed logical outcome, so it still commits
		// together with the just-created claim.
		if err := m.joinRepo.CompleteSessionRequest(ctx, tx, requestID, &lockedSession.ID, outcomeAlreadyJoined, ""); err != nil {
			return session.JoinResult{}, fmt.Errorf("completing join session request: %s", err)
		}
		return session.JoinResult{Outcome: session.JoinOutcomeAlreadyJoined}, nil
	}

	activeCount, err := m.joinRepo.CountActiveParticipants(ctx, tx, lockedSession.ID)
	if err != nil {
		return session.JoinResult{}, err
	}
	if playersMax > 0 && activeCount >= playersMax {
		if err := m.joinRepo.CompleteSessionRequest(ctx, tx, requestID, &lockedSession.ID, outcomeLobbyFull, ""); err != nil {
			return session.JoinResult{}, fmt.Errorf("completing join session request: %s", err)
		}
		return session.JoinResult{Outcome: session.JoinOutcomeLobbyFull}, nil
	}

	if participant == nil {
		if err := m.joinRepo.CreateParticipant(ctx, tx, actorID, string(displayName), now); err != nil {
			return session.JoinResult{}, err
		}
	} else {
		if err := m.joinRepo.ActivateParticipant(ctx, tx, participant.ID, string(displayName), now); err != nil {
			return session.JoinResult{}, err
		}
	}

	result := session.JoinResult{Outcome: session.JoinOutcomeJoined, SessionUUID: session.SessionUUID(lockedSession.UUID), DisplayName: displayName}
	responseBytes, err := json.Marshal(result)
	if err != nil {
		return session.JoinResult{}, fmt.Errorf("marshaling join response payload: %s", err)
	}
	if err := m.joinRepo.CompleteSessionRequest(ctx, tx, requestID, &lockedSession.ID, outcomeJoined, string(responseBytes)); err != nil {
		return session.JoinResult{}, fmt.Errorf("completing join session request: %s", err)
	}
	return result, nil
}

// interpretExistingJoinClaim decides what an already-claimed JOIN identity
// means for the incoming request: replay or conflict. A replayed decline is
// returned as the same outcome value it was originally recorded as, never
// reconstructed as an error.
func interpretExistingJoinClaim(existing *internalrepo.Request, incoming joinRequestPayload) (session.JoinResult, error) {
	if existing.Status != internalrepo.RequestStatusCompleted {
		return session.JoinResult{}, session.ErrIdempotencyInFlight
	}

	var stored joinRequestPayload
	if err := json.Unmarshal([]byte(existing.RequestPayload), &stored); err != nil {
		return session.JoinResult{}, fmt.Errorf("decoding stored join request payload: %s", err)
	}
	if stored != incoming {
		return session.JoinResult{}, session.ErrIdempotencyConflict
	}

	switch existing.Outcome {
	case outcomeAlreadyJoined:
		return session.JoinResult{Outcome: session.JoinOutcomeAlreadyJoined}, nil
	case outcomeLobbyFull:
		return session.JoinResult{Outcome: session.JoinOutcomeLobbyFull}, nil
	}

	if existing.ResponsePayload == nil {
		return session.JoinResult{}, fmt.Errorf("completed join idempotency record missing response payload")
	}
	var result session.JoinResult
	if err := json.Unmarshal([]byte(*existing.ResponsePayload), &result); err != nil {
		return session.JoinResult{}, fmt.Errorf("decoding stored join response payload: %s", err)
	}
	return result, nil
}
