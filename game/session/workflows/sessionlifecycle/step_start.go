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
	internalrepo "github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/utils"
	"gorm.io/gorm"
)

// maxStepsPerRuntimeTurn is the V1 Session-level RuntimeTurn Step-chain
// bound (GAME-ADR-0019): every actual engine.Step call belonging to the
// same RuntimeTurn counts - the initial signal plus every subsequent Step
// caused by draining a prior Step's InternalSignals - applying equally to
// Start's own initialization chain. A code-level constant, not authored by
// individual games and not a durable per-Session setting.
const maxStepsPerRuntimeTurn = 20

// runtimeTurnSnapshotFormatVersion is session_runtime_turns.
// snapshot_format_version's starting value (WORK-0003's Implementation
// Freedom).
const runtimeTurnSnapshotFormatVersion = 1

// startSourceKind is the source_kind label persisted on Start's own
// RuntimeTurn (the exhaustive source_kind enum remains undecided
// repository-wide - SESSION_RUNTIME_PERSISTENCE_MODEL.md's Not Yet
// Decided list).
const startSourceKind = "SESSION_START"

// Start outcome labels persisted to session_requests.outcome for a
// deterministic post-claim decline that must survive as a replayable
// outcome (WORK-0001's Transaction Ownership; WORK-0003's Blocker 3 for
// outcomeRuntimeInitFailed specifically). outcomeStarted is also recorded
// so a same-token replay can tell success from a decline.
const (
	outcomeStarted           = "STARTED"
	outcomeNotHost           = "NOT_HOST"
	outcomeNotEnoughPlayers  = "NOT_ENOUGH_PLAYERS"
	outcomeRuntimeInitFailed = "RUNTIME_INIT_FAILED"
)

// startRepoAPI is Start's own narrow persistence contract (see
// createRepoAPI's doc comment on why this is not shared verbatim with
// Create/Join/Leave despite some overlapping method shapes). The shared
// sessionlock/idempotency mechanism packages are called directly by this
// step instead of through repository forwarding methods
// (`docs/engineering/standards/repositories.md`'s Sharing Rule).
type startRepoAPI interface {
	expirationStore
	FindActor(ctx context.Context, tx *gorm.DB, sessionID uint, userUUID string) (*internalrepo.Actor, error)
	ListActiveParticipantsForRoster(ctx context.Context, tx *gorm.DB, sessionID uint) ([]internalrepo.RosterParticipant, error)
	SetSessionRunning(ctx context.Context, tx *gorm.DB, sessionID uint, startedAt time.Time) error
	CreateRuntimeTurn(ctx context.Context, tx *gorm.DB, sessionID uint, sequence uint64, sourceKind string, snapshotPayload []byte, snapshotFormatVersion int) (uint, error)
	CreateRuntimeStep(ctx context.Context, tx *gorm.DB, runtimeTurnID uint, stepIndex int, commitPayload []byte) error
	SetCurrentTurn(ctx context.Context, tx *gorm.DB, sessionID uint, currentTurnID uint) error
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
// See WORK-0003's Outcome/Approved Design.
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
	if _, err := materializeExpirationIfDue(ctx, tx, m.startRepo, lockedSession, now); err != nil {
		return StartResult{}, err
	}
	if lockedSession.Phase == session.PhaseRunning {
		// A concurrent Start (a different idempotency token racing this
		// same call for lock acquisition - see this package's
		// TestManagerStart_Integration's concurrency case) already won and
		// committed Turn 1: this is not a lobby-expiration decline, it is
		// simply already-true current state, accurately reported as the
		// same outcome a caller who actually won the race would see
		// (StartOutcome's own documented meaning: "the Session is now
		// RUNNING", true regardless of who committed it).
		return StartResult{Outcome: StartOutcomeStarted, SessionUUID: SessionUUID(lockedSession.UUID)}, nil
	}
	if lockedSession.Phase != session.PhaseLobby {
		// A rejection discovered before any idempotency claim is attempted
		// never reaches session_requests at all - there is no token-scoped
		// outcome to record. Any materialization above must still commit
		// even though this attempted Start is rejected (WORK-0001's
		// Transaction Ownership) - it already has, via this same
		// callback's eventual successful return.
		return StartResult{Outcome: StartOutcomeLobbyExpired}, nil
	}

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

	// Resolves (SessionUUID, UserUUID) -> SessionActorID and verifies host
	// authority (GAME-ADR-0004/0005) - a missing actor is indistinguishable
	// from "not the host" for this purpose, since only sessions.host_actor_id
	// grants Start authority.
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

	// Loads the Session's pinned Definition/Version UUID directly - never
	// the Game's current version (GAME-ADR-0001/0004). Unlike Join, Start's
	// SessionUUID already identifies the Session directly, so there is no
	// unlocked pre-lookup step other than this read, made once the Session
	// row is known to exist under lock.
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
		// an unexpected recompile failure now is durable-state invalidity
		// (GAME-ADR-0017's third class), not a deterministic authored-game
		// failure (WORK-0003's Fatal-Path Classification).
		monitoring.Alert(ctx, fmt.Sprintf(
			"pinned game definition failed to recompile at session start: session_uuid=%s game_definition_uuid=%s",
			sessionUUID, lockedSession.GameDefinitionUUID,
		))
		return m.terminalizeStartFatal(ctx, tx, lockedSession, requestID, now, session.TerminalReasonRuntimeStateInvalid)
	}

	// The roster is built strictly from Participants active at the
	// serialized Start moment (game/README.md's Session Runtime Lobby
	// Lifecycle Contract), ordered by joined_at ascending, ties broken by
	// internal actor id (WORK-0003 Blocker 4, human-approved 2026-09-19).
	roster, err := m.startRepo.ListActiveParticipantsForRoster(ctx, tx, lockedSession.ID)
	if err != nil {
		return StartResult{}, err
	}
	if len(roster) < definition.Players.Min || (definition.Players.Max > 0 && len(roster) > definition.Players.Max) {
		// The players.max case is defensive - Join already enforces it and
		// is not expected to ever trigger here in practice - grouped under
		// the same ordinary LOBBY-phase decline as players.min (WORK-0003's
		// Approved Design).
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
		// Identity.UserUUID never enters engine/runtime state
		// (GAME-ADR-0005/0006).
		players[i] = engine.UserValue{ID: engine.UserID(strconv.FormatUint(uint64(p.ActorID), 10))}
	}

	snapshot, startSignal, err := engineservice.NewSnapshot(compiledProgram, engine.InitializationInput{
		RootParameters: map[string]engine.Value{
			"players": engine.ListValue{ElementType: engine.UserType{}, Elements: players},
		},
		Seed: drawSeed(),
	})
	if err != nil {
		// Everything downstream of a successful compile is
		// RUNTIME_EXECUTION_FAILED (WORK-0003's Fatal-Path Classification).
		return m.terminalizeStartFatal(ctx, tx, lockedSession, requestID, now, session.TerminalReasonRuntimeExecutionFailed)
	}

	finalSnapshot, steps, ok := drainRuntimeTurn(compiledProgram, snapshot, startSignal)
	if !ok {
		return m.terminalizeStartFatal(ctx, tx, lockedSession, requestID, now, session.TerminalReasonRuntimeExecutionFailed)
	}

	snapshotPayload, err := engineservice.EncodeSnapshot(finalSnapshot)
	if err != nil {
		return StartResult{}, fmt.Errorf("encoding start snapshot: %s", err)
	}
	turnID, err := m.startRepo.CreateRuntimeTurn(ctx, tx, lockedSession.ID, 1, startSourceKind, snapshotPayload, runtimeTurnSnapshotFormatVersion)
	if err != nil {
		return StartResult{}, err
	}
	for i, step := range steps {
		stepPayload, err := json.Marshal(step)
		if err != nil {
			return StartResult{}, fmt.Errorf("encoding start runtime step %d: %s", i, err)
		}
		if err := m.startRepo.CreateRuntimeStep(ctx, tx, turnID, i, stepPayload); err != nil {
			return StartResult{}, err
		}
	}
	if err := m.startRepo.SetCurrentTurn(ctx, tx, lockedSession.ID, turnID); err != nil {
		return StartResult{}, err
	}
	if err := m.startRepo.SetSessionRunning(ctx, tx, lockedSession.ID, now); err != nil {
		return StartResult{}, err
	}
	if err := m.startRepo.RevokeActiveJoinCode(ctx, tx, lockedSession.ID, now); err != nil {
		return StartResult{}, err
	}

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

// terminalizeStartFatal performs Start's pre-first-Turn fatal path
// (GAME-ADR-0019): atomically terminalizes the Session directly from
// LOBBY, revokes its JoinCode, and records the fatal outcome as the START
// idempotency claim's completed - and replayable - outcome, per
// idempotency.md's Completed Outcomes vs. Transient Failures (WORK-0003
// Blocker 3, human-approved 2026-09-19: a Start that already fatally
// terminalized the Session must never be re-attempted on a same-token
// retry). started_at is left at its canonical NULL - the Session never
// actually ran. No session_runtime_turns/steps/state row is written; no
// session_runtime_failures diagnostic entity exists yet (Slice 6's scope).
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
// identity means for the incoming request: replay or conflict
// (`docs/engineering/standards/idempotency.md`'s Token Semantics). A
// replayed decline - including the fatal RuntimeInitFailed outcome - is
// returned as the same outcome value it was originally recorded as
// (GAME-ADR-0022), never reconstructed as an error and never re-attempted
// against a Session that has since become TERMINAL (WORK-0003 Blocker 3).
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

// runtimeStepTrace is session_runtime_steps.commit_payload's serialized
// shape - technical execution trace only, never authoritative gameplay
// (GAME-ADR-0007). It deliberately carries only engine.Commit.Trace's plain
// diagnostic fields, never engine.Value-typed data, so it can be marshaled
// with encoding/json directly.
type runtimeStepTrace struct {
	Workflow       string `json:"workflow"`
	TransitionName string `json:"transition_name"`
	StateBefore    string `json:"state_before"`
	StateAfter     string `json:"state_after"`
	OperationCount int    `json:"operation_count"`
}

// drainRuntimeTurn executes program's initial Signal chain to quiescence,
// draining engine.Commit.InternalSignals in FIFO order and counting every
// actual Step call (the initial one plus every internal-signal-caused one)
// toward maxStepsPerRuntimeTurn - exactly GAME-ADR-0019's definition,
// applying equally to Start's own initialization chain.
//
// This is Start's own inline RuntimeTurn execution logic - per the human's
// 2026-09-19 rejection of a new shared game/session/internal/runtimeturn
// package (WORK-0003's Approved Design: RuntimeTurn Execution Logic
// subsection) - a pure function over engine/engineservice with no
// persistence/transaction dependency, so it can be unit-tested directly
// (see this package's TestDrainRuntimeTurn) without a real Postgres
// connection, mirroring engineservice's own test style.
//
// On success, ok is true, finalSnapshot is the Turn's final authoritative
// Snapshot, and steps traces every actual Step call in order. On a
// non-rejection ExecutionError from any Step call, an outright rejection
// (engineservice.ErrSignalRejected/ErrInputRejected) of the initial signal
// or any subsequent internal signal, or needing a Step call beyond
// maxStepsPerRuntimeTurn to reach quiescence, ok is false and the original
// snapshot is returned unchanged - the caller decides the Session-lifecycle
// consequence (always RUNTIME_EXECUTION_FAILED for Start's own initial
// chain - see WORK-0003's Fatal-Path Classification).
func drainRuntimeTurn(p engine.Program, snapshot engine.Snapshot, initialSignal engine.Signal) (finalSnapshot engine.Snapshot, steps []runtimeStepTrace, ok bool) {
	pending := []engine.Signal{initialSignal}
	current := snapshot
	for len(pending) > 0 {
		if len(steps) >= maxStepsPerRuntimeTurn {
			return snapshot, nil, false
		}
		signal := pending[0]
		pending = pending[1:]

		commit, err := engineservice.Step(p, current, signal, engine.DefaultLimits())
		if err != nil {
			return snapshot, nil, false
		}
		current = commit.Snapshot
		steps = append(steps, runtimeStepTrace{
			Workflow:       commit.Trace.Workflow,
			TransitionName: commit.Trace.TransitionName,
			StateBefore:    commit.Trace.StateBefore,
			StateAfter:     commit.Trace.StateAfter,
			OperationCount: commit.Trace.OperationCount,
		})
		pending = append(pending, commit.InternalSignals...)
	}
	return current, steps, true
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
