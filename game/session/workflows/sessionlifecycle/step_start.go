package sessionlifecycle

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/game/session/internal/idempotency"
	"github.com/diegobermudez03/playhoot/game/session/internal/sessionlock"
	"github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/clientoutputs"
	"github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/completion"
	"github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/expiration"
	"github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/interactions"
	internalrepo "github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/replay"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/utils"
	"gorm.io/gorm"
)

// startSourceKind is the source_kind label persisted on Start's own
// RuntimeTurn. There is no exhaustive enum of source_kind values yet, so
// this is a plain string.
const startSourceKind = "SESSION_START"

// Start outcome labels persisted to session_requests.outcome for a
// deterministic post-claim decline that must survive as a replayable
// outcome. outcomeStarted is also recorded so a same-token replay can tell
// success from a decline.
const (
	outcomeStarted           = "STARTED"
	outcomeNotHost           = "NOT_HOST"
	outcomeNotEnoughPlayers  = "NOT_ENOUGH_PLAYERS"
	outcomeRuntimeInitFailed = "RUNTIME_INIT_FAILED"
	outcomeLobbyExpired      = "LOBBY_EXPIRED"
)

// startRepoAPI is Start's own narrow persistence contract. The shared
// sessionlock/idempotency mechanism packages are called directly by this
// step instead of through repository forwarding methods.
type startRepoAPI interface {
	expiration.Store
	interactions.CaptureRepo
	FindActor(ctx context.Context, tx *gorm.DB, sessionID uint, userUUID string) (*internalrepo.Actor, error)
	ListActiveParticipantsForRoster(ctx context.Context, tx *gorm.DB, sessionID uint) ([]internalrepo.RosterParticipant, error)
	SetSessionRunning(ctx context.Context, tx *gorm.DB, sessionID uint, startedAt time.Time) error
	CreateRuntimeTurn(ctx context.Context, tx *gorm.DB, sessionID uint, sequence uint64, sourceKind string, sourceInteractionID *uint, actorID *uint) (uint, error)
	CreateRuntimeStart(ctx context.Context, tx *gorm.DB, sessionID uint, seed uint64, rootParameters []byte) error
	SetCurrentTurn(ctx context.Context, tx *gorm.DB, sessionID uint, currentTurnID uint) error
	CloseAllActiveInteractionsForSession(ctx context.Context, tx *gorm.DB, sessionID uint, reason string) error
}

// startRequestPayload is START's meaningful-field idempotency payload.
type startRequestPayload struct {
	SessionUUID string `json:"session_uuid"`
	UserUUID    string `json:"user_uuid"`
}

// Start transitions a LOBBY Session to RUNNING: it verifies host authority,
// builds the `players` root roster from active Participants, initializes
// and executes the Game Language engine's first RuntimeTurn against the
// Session's pinned immutable Game Definition, and atomically commits
// phase=RUNNING together with the committed Turn and JoinCode revocation.
func (m *Manager) Start(ctx context.Context, sessionUUID SessionUUID, userUUID UserUUID, idempotencyKey IdempotencyKey) (StartResult, error) {
	defer logging.Step(ctx, "SessionLifecycle.Start").Close()
	logging.LogFields(ctx,
		logging.Field("session_uuid", string(sessionUUID)),
		logging.Field("user_uuid", string(userUUID)),
	)

	if idempotencyKey == "" {
		return StartResult{}, session.ErrIdempotencyKeyRequired
	}

	return utils.RunInDBTransaction(ctx, m, func(ctx context.Context, tx *gorm.DB) (StartResult, error) {
		return m.startSessionInTx(ctx, tx, sessionUUID, userUUID, idempotencyKey)
	})
}

// startSessionInTx is Start's per-transaction business logic, split out
// from the public Start so it can be exercised directly by
// mocked-collaborator unit tests without needing a real DB transaction -
// sessionlock/idempotency mechanism calls further down this same path
// require a real Postgres connection to run their SQL, which is instead
// proven by this package's repository-integration/concurrency tests.
func (m *Manager) startSessionInTx(ctx context.Context, tx *gorm.DB, sessionUUID SessionUUID, userUUID UserUUID, idempotencyKey IdempotencyKey) (StartResult, error) {
	incomingPayload := startRequestPayload{SessionUUID: string(sessionUUID), UserUUID: string(userUUID)}

	lockedSession, err := sessionlock.LockByUUID(ctx, tx, string(sessionUUID))
	if err != nil {
		return StartResult{}, err
	}
	if lockedSession == nil {
		return StartResult{}, session.ErrSessionNotFound
	}

	now := time.Now().UTC()
	if _, err := expiration.MaterializeIfDue(ctx, tx, m.startRepo, lockedSession, now); err != nil {
		return StartResult{}, err
	}

	// Claimed before any phase-based decision: unlike Join/Leave, Start's
	// own action can itself drive the Session to TERMINAL or RUNNING, so a
	// same-token retry must replay that already-recorded outcome via
	// interpretExistingStartClaim below rather than re-derive an answer from
	// current phase as if this were a fresh command.
	payloadBytes, err := json.Marshal(incomingPayload)
	if err != nil {
		return StartResult{}, fmt.Errorf("marshaling start request payload: %s", err)
	}
	requestID, existing, err := idempotency.Claim(ctx, tx, idempotency.ClaimInput{
		Operation:      operationStart,
		UserUUID:       string(userUUID),
		IdempotencyKey: string(idempotencyKey),
		SessionID:      &lockedSession.ID,
		RequestPayload: string(payloadBytes),
	})
	if err != nil {
		return StartResult{}, fmt.Errorf("claiming start session request: %s", err)
	}
	if existing != nil {
		return interpretExistingStartClaim(existing, incomingPayload)
	}

	// No prior claim exists for this token, so it is evaluated as a fresh
	// command against current state.
	//
	// If the Session is already RUNNING, a different token's Start already
	// won; reporting Started here too is harmless and consistent, since the
	// operation is naturally idempotent regardless of who actually caused
	// it. Completing this claim lets a later retry under this same token
	// replay the same answer.
	//
	// If the Session is TERMINAL, the outcome reflects its actual
	// terminal_reason instead of always assuming lobby expiration, so a
	// Session terminalized by Start's own fatal path is reported as
	// RuntimeInitFailed rather than mislabeled as a lobby timeout.
	if lockedSession.Phase == session.PhaseRunning {
		result := StartResult{Outcome: StartOutcomeStarted, SessionUUID: SessionUUID(lockedSession.UUID)}
		responseBytes, err := json.Marshal(result)
		if err != nil {
			return StartResult{}, fmt.Errorf("marshaling start response payload: %s", err)
		}
		if err := idempotency.Complete(ctx, tx, requestID, &lockedSession.ID, outcomeStarted, string(responseBytes)); err != nil {
			return StartResult{}, fmt.Errorf("completing start session request: %s", err)
		}
		return result, nil
	}
	if lockedSession.Phase != session.PhaseLobby {
		outcome, resultOutcome := outcomeLobbyExpired, StartOutcomeLobbyExpired
		if lockedSession.TerminalReason != nil && *lockedSession.TerminalReason != session.TerminalReasonLobbyExpired {
			outcome, resultOutcome = outcomeRuntimeInitFailed, StartOutcomeRuntimeInitFailed
		}
		if err := idempotency.Complete(ctx, tx, requestID, &lockedSession.ID, outcome, ""); err != nil {
			return StartResult{}, fmt.Errorf("completing start session request: %s", err)
		}
		return StartResult{Outcome: resultOutcome}, nil
	}

	// A missing actor is indistinguishable from "not the host" for this
	// purpose, since only sessions.host_actor_id grants Start authority.
	actor, err := m.startRepo.FindActor(ctx, tx, lockedSession.ID, string(userUUID))
	if err != nil {
		return StartResult{}, err
	}
	if actor == nil || lockedSession.HostActorID == nil || actor.ID != *lockedSession.HostActorID {
		if err := idempotency.Complete(ctx, tx, requestID, &lockedSession.ID, outcomeNotHost, ""); err != nil {
			return StartResult{}, fmt.Errorf("completing start session request: %s", err)
		}
		return StartResult{Outcome: StartOutcomeNotHost}, nil
	}

	// The pinned Definition/Version UUID is read directly, never the Game's
	// current version. Start's SessionUUID already identifies the Session
	// directly, so this read has no unlocked pre-lookup step - it happens
	// once the Session row is known to exist under lock.
	definition, err := m.pinnedGameReader.GetGameDefinition(ctx, lockedSession.GameDefinitionUUID)
	if err != nil {
		return StartResult{}, err
	}
	if definition == nil {
		monitoring.Alert(ctx, "session pinned game definition is missing")
		return StartResult{}, session.ErrPinnedDefinitionMissing
	}

	compiledProgram, diagnostics := engineservice.Compile(*definition)
	if diagnostics.HasErrors() {
		// The pinned Definition already compiled successfully at Create -
		// an unexpected recompile failure now is a data-integrity problem,
		// not a deterministic authored-game failure.
		monitoring.Alert(ctx, fmt.Sprintf(
			"pinned game definition failed to recompile at session start: session_uuid=%s game_definition_uuid=%s",
			sessionUUID, lockedSession.GameDefinitionUUID,
		))
		return m.terminalizeStartFatal(ctx, tx, lockedSession, requestID, now, session.TerminalReasonRuntimeStateInvalid)
	}

	// The roster is built strictly from Participants active at the
	// serialized Start moment, ordered by joined_at ascending, ties broken
	// by internal actor id.
	roster, err := m.startRepo.ListActiveParticipantsForRoster(ctx, tx, lockedSession.ID)
	if err != nil {
		return StartResult{}, err
	}
	if len(roster) < definition.Players.Min || (definition.Players.Max > 0 && len(roster) > definition.Players.Max) {
		// The players.max case is defensive - Join already enforces it and
		// is not expected to ever trigger here in practice - grouped under
		// the same ordinary LOBBY-phase decline as players.min.
		if err := idempotency.Complete(ctx, tx, requestID, &lockedSession.ID, outcomeNotEnoughPlayers, ""); err != nil {
			return StartResult{}, fmt.Errorf("completing start session request: %s", err)
		}
		return StartResult{Outcome: StartOutcomeNotEnoughPlayers}, nil
	}

	players := make([]engine.Value, len(roster))
	for i, p := range roster {
		// Each engine.UserValue.ID is derived from the Participant's
		// internal session_actors.id - an opaque string representation,
		// only ever compared for equality (engine.UserID's own contract).
		// The caller's own public user identifier never enters
		// engine/runtime state.
		players[i] = engine.UserValue{ID: engine.UserID(strconv.FormatUint(uint64(p.ActorID), 10))}
	}

	seed := drawSeed()
	rootParameters := map[string]engine.Value{
		"players": engine.ListValue{ElementType: engine.UserType{}, Elements: players},
	}
	outputs, err := engineservice.StartTurn(compiledProgram, engine.InitializationInput{
		RootParameters: rootParameters,
		Seed:           seed,
	}, engine.DefaultLimits())
	if err != nil {
		// Everything downstream of a successful compile that fails is
		// treated as RUNTIME_EXECUTION_FAILED.
		return m.terminalizeStartFatal(ctx, tx, lockedSession, requestID, now, session.TerminalReasonRuntimeExecutionFailed)
	}

	encodedRootParameters, err := replay.EncodeRootParameters(rootParameters)
	if err != nil {
		return StartResult{}, fmt.Errorf("encoding start root parameters: %s", err)
	}
	turnID, err := m.startRepo.CreateRuntimeTurn(ctx, tx, lockedSession.ID, 1, startSourceKind, nil, nil)
	if err != nil {
		return StartResult{}, err
	}
	if err := m.startRepo.CreateRuntimeStart(ctx, tx, lockedSession.ID, seed, encodedRootParameters); err != nil {
		return StartResult{}, err
	}
	if err := interactions.Capture(ctx, tx, m.startRepo, lockedSession.ID, turnID, outputs); err != nil {
		return StartResult{}, err
	}
	if err := m.startRepo.SetCurrentTurn(ctx, tx, lockedSession.ID, turnID); err != nil {
		return StartResult{}, err
	}
	if err := m.startRepo.SetSessionRunning(ctx, tx, lockedSession.ID, now); err != nil {
		return StartResult{}, err
	}

	// A Definition may complete/fail/cancel its own root instance on its
	// very first transition - the Session genuinely started (Turn 1
	// committed, started_at set above) and immediately ended, rather than
	// never having started at all.
	terminalReason, terminated := completion.Detect(outputs)
	if terminated {
		if err := m.startRepo.SetSessionTerminal(ctx, tx, lockedSession.ID, now, terminalReason); err != nil {
			return StartResult{}, err
		}
		if err := m.startRepo.CloseAllActiveInteractionsForSession(ctx, tx, lockedSession.ID, session.InteractionClosureReasonSessionTerminated); err != nil {
			return StartResult{}, err
		}
	}

	if err := m.startRepo.RevokeActiveJoinCode(ctx, tx, lockedSession.ID, now); err != nil {
		return StartResult{}, err
	}

	mappedOutputs, err := m.mapOutputs(ctx, tx, lockedSession.ID, clientoutputs.ClientFacing(outputs))
	if err != nil {
		return StartResult{}, fmt.Errorf("mapping client-facing outputs: %s", err)
	}

	result := StartResult{Outcome: StartOutcomeStarted, SessionUUID: SessionUUID(lockedSession.UUID), Outputs: mappedOutputs, TerminalReason: terminalReason}
	responseBytes, err := json.Marshal(result)
	if err != nil {
		return StartResult{}, fmt.Errorf("marshaling start response payload: %s", err)
	}
	if err := idempotency.Complete(ctx, tx, requestID, &lockedSession.ID, outcomeStarted, string(responseBytes)); err != nil {
		return StartResult{}, fmt.Errorf("completing start session request: %s", err)
	}
	return result, nil
}

// terminalizeStartFatal performs Start's pre-first-Turn fatal path:
// atomically terminalizes the Session directly from LOBBY, revokes its
// JoinCode, and records the fatal outcome as the START idempotency claim's
// completed - and replayable - outcome, so a Start that already fatally
// terminalized the Session is never re-attempted on a same-token retry.
// started_at is left at its canonical NULL - the Session never actually
// ran. No session_runtime_turns/session_runtime_starts row is written, and
// no runtime-failure diagnostic entity exists yet.
func (m *Manager) terminalizeStartFatal(ctx context.Context, tx *gorm.DB, lockedSession *sessionlock.Session, requestID uint, terminalAt time.Time, terminalReason string) (StartResult, error) {
	if err := m.startRepo.SetSessionTerminal(ctx, tx, lockedSession.ID, terminalAt, terminalReason); err != nil {
		return StartResult{}, err
	}
	if err := m.startRepo.RevokeActiveJoinCode(ctx, tx, lockedSession.ID, terminalAt); err != nil {
		return StartResult{}, err
	}
	if err := idempotency.Complete(ctx, tx, requestID, &lockedSession.ID, outcomeRuntimeInitFailed, ""); err != nil {
		return StartResult{}, fmt.Errorf("completing start session request: %s", err)
	}
	return StartResult{Outcome: StartOutcomeRuntimeInitFailed}, nil
}

// interpretExistingStartClaim decides what an already-claimed START
// identity means for the incoming request: replay or conflict. A replayed
// decline - including the fatal RuntimeInitFailed outcome - is returned as
// the same outcome value it was originally recorded as, never reconstructed
// as an error and never re-attempted against a Session that has since
// become TERMINAL.
func interpretExistingStartClaim(existing *idempotency.Request, incoming startRequestPayload) (StartResult, error) {
	if existing.Status != idempotency.StatusCompleted {
		return StartResult{}, session.ErrIdempotencyInFlight
	}

	var stored startRequestPayload
	if err := json.Unmarshal([]byte(existing.RequestPayload), &stored); err != nil {
		return StartResult{}, fmt.Errorf("decoding stored start request payload: %s", err)
	}
	if stored != incoming {
		return StartResult{}, session.ErrIdempotencyConflict
	}

	switch existing.Outcome {
	case outcomeNotHost:
		return StartResult{Outcome: StartOutcomeNotHost}, nil
	case outcomeNotEnoughPlayers:
		return StartResult{Outcome: StartOutcomeNotEnoughPlayers}, nil
	case outcomeRuntimeInitFailed:
		return StartResult{Outcome: StartOutcomeRuntimeInitFailed}, nil
	case outcomeLobbyExpired:
		return StartResult{Outcome: StartOutcomeLobbyExpired}, nil
	}

	if existing.ResponsePayload == nil {
		return StartResult{}, fmt.Errorf("completed start idempotency record missing response payload")
	}
	var result StartResult
	if err := json.Unmarshal([]byte(*existing.ResponsePayload), &result); err != nil {
		return StartResult{}, fmt.Errorf("decoding stored start response payload: %s", err)
	}
	return result, nil
}

// drawSeed draws InitializationInput.Seed from a fresh, unpredictable
// source, once, per InitializationInput.Seed's own documented contract -
// the engine itself never reads OS randomness. crypto/rand is a legitimate
// real-entropy source for this one-time draw (this call site only - it is
// never used inside the deterministic engine simulation itself).
func drawSeed() uint64 {
	var buf [8]byte
	if _, err := cryptorand.Read(buf[:]); err != nil {
		// crypto/rand.Read failing is not a realistic operational condition
		// on any supported platform; falling back to wall-clock time keeps
		// Start from hard-failing on it while still drawing a value that is
		// not guessable in advance.
		return uint64(time.Now().UnixNano())
	}
	return binary.LittleEndian.Uint64(buf[:])
}
