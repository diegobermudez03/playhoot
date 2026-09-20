package sessionlifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/game/language/v1/program"
	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/game/session/internal/idempotency"
	"github.com/diegobermudez03/playhoot/game/session/internal/sessionlock"
	internalrepo "github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
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

// joinRepoAPI is Join's own narrow persistence contract. The shared
// sessionlock/idempotency mechanism packages are called directly by this
// step instead of through repository forwarding methods.
type joinRepoAPI interface {
	expirationStore
	ResolveSessionForJoinCode(ctx context.Context, joinCode uint) (*internalrepo.JoinCodeResolution, error)
	FindActor(ctx context.Context, tx *gorm.DB, sessionID uint, userUUID string) (*internalrepo.Actor, error)
	CreateActor(ctx context.Context, tx *gorm.DB, sessionID uint, userUUID string) (uint, error)
	FindParticipant(ctx context.Context, tx *gorm.DB, actorID uint) (*internalrepo.Participant, error)
	CountActiveParticipants(ctx context.Context, tx *gorm.DB, sessionID uint) (int, error)
	CreateParticipant(ctx context.Context, tx *gorm.DB, actorID uint, displayName string, joinedAt time.Time) error
	ActivateParticipant(ctx context.Context, tx *gorm.DB, participantID uint, displayName string, joinedAt time.Time) error
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
func (m *Manager) Join(ctx context.Context, joinCode JoinCode, userUUID UserUUID, displayName DisplayName, idempotencyKey IdempotencyKey) (JoinResult, error) {
	defer logging.Step(ctx, "SessionLifecycle.Join").Close()
	logging.LogFields(ctx,
		logging.Field("join_code", uint(joinCode)),
		logging.Field("user_uuid", string(userUUID)),
	)

	if idempotencyKey == "" {
		return JoinResult{}, session.ErrIdempotencyKeyRequired
	}

	// ResolveSessionForJoinCode resolves the most recent join_codes row for
	// this code number regardless of revocation status (never filtered to
	// revoked_at IS NULL): a code that was active a moment ago can be
	// concurrently revoked by another operation's lazy lobby-expiration
	// materialization before this call reaches the lock below, and that race
	// must still surface as the ordinary LobbyExpired outcome value once
	// locked, not as a hard "invalid code" error discovered here and never
	// re-validated under lock. A code already revoked independently of the
	// Session it names remaining LOBBY (resolution.RevokedAt set) is instead
	// rejected once locked, immediately below - see joinSessionInTx.
	resolution, err := m.joinRepo.ResolveSessionForJoinCode(ctx, uint(joinCode))
	if err != nil {
		return JoinResult{}, err
	}
	if resolution == nil {
		return JoinResult{}, session.ErrJoinCodeInvalid
	}

	// Loads the Session's pinned Definition/Version UUID directly - never
	// the Game's current version - before opening the mutation
	// transaction/row lock, so lobby capacity stays governed by the exact
	// version this Session was pinned to at Create.
	definition, err := m.pinnedGameReader.GetGameDefinition(ctx, resolution.GameDefinitionUUID)
	if err != nil {
		return JoinResult{}, err
	}
	if definition == nil {
		monitoring.Alert(ctx, "session pinned game definition is missing")
		return JoinResult{}, session.ErrPinnedDefinitionMissing
	}
	playersMax := definition.Players.Max

	codeWasRevokedAtResolution := resolution.RevokedAt != nil

	return utils.RunInDBTransaction(ctx, m, func(ctx context.Context, tx *gorm.DB) (JoinResult, error) {
		return m.joinSessionInTx(ctx, tx, resolution.SessionID, codeWasRevokedAtResolution, playersMax, joinCode, userUUID, displayName, idempotencyKey)
	})
}

// joinSessionInTx is Join's per-transaction business logic, split out from
// the public Join so it can be exercised directly by mocked-collaborator
// unit tests without needing a real DB transaction - sessionlock/idempotency
// mechanism calls further down this same path require a real Postgres
// connection to run their SQL, which is instead proven by this package's
// repository-integration/concurrency tests.
func (m *Manager) joinSessionInTx(ctx context.Context, tx *gorm.DB, sessionID uint, codeWasRevokedAtResolution bool, playersMax int, joinCode JoinCode, userUUID UserUUID, displayName DisplayName, idempotencyKey IdempotencyKey) (JoinResult, error) {
	incomingPayload := joinRequestPayload{JoinCode: uint(joinCode), UserUUID: string(userUUID), DisplayName: string(displayName)}

	lockedSession, err := sessionlock.LockByID(ctx, tx, sessionID)
	if err != nil {
		return JoinResult{}, err
	}
	if lockedSession == nil {
		return JoinResult{}, session.ErrSessionNotFound
	}

	now := time.Now().UTC()
	if _, err := materializeExpirationIfDue(ctx, tx, m.joinRepo, lockedSession, now); err != nil {
		return JoinResult{}, err
	}
	if lockedSession.Phase != session.PhaseLobby {
		// A rejection discovered before any idempotency claim is attempted
		// never reaches session_requests at all - there is no token-scoped
		// outcome to record. Any materialization above must still commit
		// even though this attempted Join is rejected - it already has, via
		// this same callback's eventual successful return.
		return JoinResult{Outcome: JoinOutcomeLobbyExpired}, nil
	}
	if codeWasRevokedAtResolution {
		// The Session itself is still LOBBY (the check above already ruled
		// out expiration), so this JoinCode's revocation cannot be this
		// call's own lazy-expiration materialization - it was already
		// revoked independently of the Session it names still being open,
		// a genuinely invalid code rather than a race with a concurrent
		// expiration.
		return JoinResult{}, session.ErrJoinCodeInvalid
	}

	payloadBytes, err := json.Marshal(incomingPayload)
	if err != nil {
		return JoinResult{}, fmt.Errorf("marshaling join request payload: %s", err)
	}
	requestID, existing, err := idempotency.Claim(ctx, tx, idempotency.ClaimInput{
		Operation:      operationJoin,
		UserUUID:       string(userUUID),
		IdempotencyKey: string(idempotencyKey),
		SessionID:      &lockedSession.ID,
		RequestPayload: string(payloadBytes),
	})
	if err != nil {
		return JoinResult{}, fmt.Errorf("claiming join session request: %s", err)
	}
	if existing != nil {
		return interpretExistingJoinClaim(existing, incomingPayload)
	}

	actor, err := m.joinRepo.FindActor(ctx, tx, lockedSession.ID, string(userUUID))
	if err != nil {
		return JoinResult{}, err
	}

	var actorID uint
	var participant *internalrepo.Participant
	if actor != nil {
		actorID = actor.ID
		participant, err = m.joinRepo.FindParticipant(ctx, tx, actorID)
		if err != nil {
			return JoinResult{}, err
		}
	} else {
		actorID, err = m.joinRepo.CreateActor(ctx, tx, lockedSession.ID, string(userUUID))
		if err != nil {
			return JoinResult{}, err
		}
	}

	if participant != nil && participant.Active {
		// A *different* idempotency token than any previously used for
		// this user's admission, evaluated against current state, while
		// already an active Participant: a new command, rejected - not
		// silently replayed or treated as success. The rejection is itself
		// the token's completed logical outcome, so it still commits
		// together with the just-created claim.
		if err := idempotency.Complete(ctx, tx, requestID, &lockedSession.ID, outcomeAlreadyJoined, ""); err != nil {
			return JoinResult{}, fmt.Errorf("completing join session request: %s", err)
		}
		return JoinResult{Outcome: JoinOutcomeAlreadyJoined}, nil
	}

	activeCount, err := m.joinRepo.CountActiveParticipants(ctx, tx, lockedSession.ID)
	if err != nil {
		return JoinResult{}, err
	}
	if playersMax > 0 && activeCount >= playersMax {
		if err := idempotency.Complete(ctx, tx, requestID, &lockedSession.ID, outcomeLobbyFull, ""); err != nil {
			return JoinResult{}, fmt.Errorf("completing join session request: %s", err)
		}
		return JoinResult{Outcome: JoinOutcomeLobbyFull}, nil
	}

	if participant == nil {
		if err := m.joinRepo.CreateParticipant(ctx, tx, actorID, string(displayName), now); err != nil {
			return JoinResult{}, err
		}
	} else {
		if err := m.joinRepo.ActivateParticipant(ctx, tx, participant.ID, string(displayName), now); err != nil {
			return JoinResult{}, err
		}
	}

	result := JoinResult{Outcome: JoinOutcomeJoined, SessionUUID: SessionUUID(lockedSession.UUID), DisplayName: displayName}
	responseBytes, err := json.Marshal(result)
	if err != nil {
		return JoinResult{}, fmt.Errorf("marshaling join response payload: %s", err)
	}
	if err := idempotency.Complete(ctx, tx, requestID, &lockedSession.ID, outcomeJoined, string(responseBytes)); err != nil {
		return JoinResult{}, fmt.Errorf("completing join session request: %s", err)
	}
	return result, nil
}

// interpretExistingJoinClaim decides what an already-claimed JOIN identity
// means for the incoming request: replay or conflict. A replayed decline is
// returned as the same outcome value it was originally recorded as, never
// reconstructed as an error.
func interpretExistingJoinClaim(existing *idempotency.Request, incoming joinRequestPayload) (JoinResult, error) {
	if existing.Status != idempotency.StatusCompleted {
		return JoinResult{}, session.ErrIdempotencyInFlight
	}

	var stored joinRequestPayload
	if err := json.Unmarshal([]byte(existing.RequestPayload), &stored); err != nil {
		return JoinResult{}, fmt.Errorf("decoding stored join request payload: %s", err)
	}
	if stored != incoming {
		return JoinResult{}, session.ErrIdempotencyConflict
	}

	switch existing.Outcome {
	case outcomeAlreadyJoined:
		return JoinResult{Outcome: JoinOutcomeAlreadyJoined}, nil
	case outcomeLobbyFull:
		return JoinResult{Outcome: JoinOutcomeLobbyFull}, nil
	}

	if existing.ResponsePayload == nil {
		return JoinResult{}, fmt.Errorf("completed join idempotency record missing response payload")
	}
	var result JoinResult
	if err := json.Unmarshal([]byte(*existing.ResponsePayload), &result); err != nil {
		return JoinResult{}, fmt.Errorf("decoding stored join response payload: %s", err)
	}
	return result, nil
}
