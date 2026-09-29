package sessionlifecycle

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/session"
	"github.com/diegobermudez03/playhoot/session/internal/executor"
	"github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/completion"
	"github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/platform"
	internalrepo "github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
	"gorm.io/gorm"
)

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

// startRepoAPI is Start's own narrow persistence contract. LockSessionByUUID/
// ClaimSessionRequest/CompleteSessionRequest are this workflow's own
// locking/idempotency-claim mechanics, not a domain-wide protocol.
type startRepoAPI interface {
	LockSessionByUUID(ctx context.Context, tx *gorm.DB, sessionUUID string) (*internalrepo.Session, error)
	FindActor(ctx context.Context, tx *gorm.DB, sessionID uint, userUUID string) (*internalrepo.Actor, error)
	ListActiveParticipantsForRoster(ctx context.Context, tx *gorm.DB, sessionID uint) ([]internalrepo.RosterParticipant, error)
	GetGameVersionArtifact(ctx context.Context, tx *gorm.DB, definitionUUID string) (*internalrepo.GameVersionArtifact, error)
	SetSessionRunning(ctx context.Context, tx *gorm.DB, sessionID uint, startedAt time.Time, activityExpiresAt time.Time) error
	CreateRuntimeTurn(ctx context.Context, tx *gorm.DB, sessionID uint, sequence uint64, sourceKind string, sourceTimerObligationID *uint, sourceCauseEventID *uint, actorID *uint, newState json.RawMessage) (uint, error)
	SetCurrentTurn(ctx context.Context, tx *gorm.DB, sessionID uint, currentTurnID uint) error
	CreateRuntimeFailure(ctx context.Context, tx *gorm.DB, sessionID uint, failureKind string, errorCode string, errorMessage string, baseTurnID *uint, attemptedSequence uint64, sourceKind string, sourceTimerObligationID *uint, actorID *uint, diagnosticPayload []byte) error
	SetSessionTerminal(ctx context.Context, tx *gorm.DB, sessionID uint, terminalAt time.Time, terminalReason string) error
	RevokeActiveJoinCode(ctx context.Context, tx *gorm.DB, sessionID uint, revokedAt time.Time) error
	CancelAllActiveTimerObligationsForSession(ctx context.Context, tx *gorm.DB, sessionID uint, reason string) error
	ClaimSessionRequest(ctx context.Context, tx *gorm.DB, input internalrepo.ClaimSessionRequestInput) (requestID uint, existing *internalrepo.Request, err error)
	CompleteSessionRequest(ctx context.Context, tx *gorm.DB, requestID uint, sessionID *uint, outcome string, responsePayload string) error
}

// startRequestPayload is START's meaningful-field idempotency payload.
type startRequestPayload struct {
	SessionUUID string `json:"session_uuid"`
	UserUUID    string `json:"user_uuid"`
}

// Start transitions a LOBBY Session to RUNNING: it verifies host authority,
// builds the `players` roster from active Participants, and drives the
// authored backend script's own first execution - a SESSION_STARTED Event
// against no prior state (PreviousState nil) - atomically committing
// phase=RUNNING together with the committed Turn and JoinCode revocation.
// Start is an ordinary Execute call like every other RUNNING-phase step;
// there is no structurally distinct entry point for it - the authored
// script constructs its own initial state as its reaction to
// SESSION_STARTED.
func (m *Manager) Start(ctx context.Context, sessionUUID session.SessionUUID, userUUID session.UserUUID, idempotencyKey session.IdempotencyKey) (session.StartResult, error) {
	defer logging.Step(ctx, "SessionLifecycle.Start").Close()
	logging.LogFields(ctx,
		logging.Field("session_uuid", string(sessionUUID)),
		logging.Field("user_uuid", string(userUUID)),
	)

	if idempotencyKey == "" {
		return session.StartResult{}, session.ErrIdempotencyKeyRequired
	}

	return utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (session.StartResult, error) {
		return m.startSessionInTx(ctx, tx, sessionUUID, userUUID, idempotencyKey)
	})
}

// startSessionInTx is Start's per-transaction business logic, split out
// from the public Start so it can be exercised directly by
// mocked-collaborator unit tests without needing a real DB transaction -
// sessionlock/idempotency mechanism calls further down this same path
// require a real Postgres connection to run their SQL, which is instead
// proven by this package's repository-integration/concurrency tests.
func (m *Manager) startSessionInTx(ctx context.Context, tx *gorm.DB, sessionUUID session.SessionUUID, userUUID session.UserUUID, idempotencyKey session.IdempotencyKey) (session.StartResult, error) {
	incomingPayload := startRequestPayload{SessionUUID: string(sessionUUID), UserUUID: string(userUUID)}

	lockedSession, err := m.startRepo.LockSessionByUUID(ctx, tx, string(sessionUUID))
	if err != nil {
		return session.StartResult{}, err
	}
	if lockedSession == nil {
		return session.StartResult{}, session.ErrSessionNotFound
	}

	now := time.Now().UTC()
	if _, err := m.lobbyExpirer.MaterializeIfDue(ctx, tx, lockedSession, now); err != nil {
		return session.StartResult{}, err
	}

	// Claimed before any phase-based decision: unlike Join/Leave, Start's
	// own action can itself drive the Session to TERMINAL or RUNNING, so a
	// same-token retry must replay that already-recorded outcome via
	// interpretExistingStartClaim below rather than re-derive an answer from
	// current phase as if this were a fresh command.
	payloadBytes, err := json.Marshal(incomingPayload)
	if err != nil {
		return session.StartResult{}, fmt.Errorf("marshaling start request payload: %s", err)
	}
	requestID, existing, err := m.startRepo.ClaimSessionRequest(ctx, tx, internalrepo.ClaimSessionRequestInput{
		Operation:      operationStart,
		UserUUID:       string(userUUID),
		IdempotencyKey: string(idempotencyKey),
		SessionID:      &lockedSession.ID,
		RequestPayload: string(payloadBytes),
	})
	if err != nil {
		return session.StartResult{}, fmt.Errorf("claiming start session request: %s", err)
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
		result := session.StartResult{Outcome: session.StartOutcomeStarted, SessionUUID: session.SessionUUID(lockedSession.UUID)}
		responseBytes, err := json.Marshal(result)
		if err != nil {
			return session.StartResult{}, fmt.Errorf("marshaling start response payload: %s", err)
		}
		if err := m.startRepo.CompleteSessionRequest(ctx, tx, requestID, &lockedSession.ID, outcomeStarted, string(responseBytes)); err != nil {
			return session.StartResult{}, fmt.Errorf("completing start session request: %s", err)
		}
		return result, nil
	}
	if lockedSession.Phase != session.PhaseLobby {
		outcome, resultOutcome := outcomeLobbyExpired, session.StartOutcomeLobbyExpired
		if lockedSession.TerminalReason != nil && *lockedSession.TerminalReason != session.TerminalReasonLobbyExpired {
			outcome, resultOutcome = outcomeRuntimeInitFailed, session.StartOutcomeRuntimeInitFailed
		}
		if err := m.startRepo.CompleteSessionRequest(ctx, tx, requestID, &lockedSession.ID, outcome, ""); err != nil {
			return session.StartResult{}, fmt.Errorf("completing start session request: %s", err)
		}
		return session.StartResult{Outcome: resultOutcome}, nil
	}

	// A missing actor is indistinguishable from "not the host" for this
	// purpose, since only sessions.host_actor_id grants Start authority.
	actor, err := m.startRepo.FindActor(ctx, tx, lockedSession.ID, string(userUUID))
	if err != nil {
		return session.StartResult{}, err
	}
	if actor == nil || lockedSession.HostActorID == nil || actor.ID != *lockedSession.HostActorID {
		if err := m.startRepo.CompleteSessionRequest(ctx, tx, requestID, &lockedSession.ID, outcomeNotHost, ""); err != nil {
			return session.StartResult{}, fmt.Errorf("completing start session request: %s", err)
		}
		return session.StartResult{Outcome: session.StartOutcomeNotHost}, nil
	}

	// Unlike Join, this has no unlocked pre-lookup step: Start's SessionUUID
	// already identifies the Session directly, so this read happens once the
	// Session row is known to exist under lock.
	artifact, err := m.startRepo.GetGameVersionArtifact(ctx, tx, lockedSession.GameDefinitionUUID)
	if err != nil {
		return session.StartResult{}, err
	}
	if artifact == nil {
		monitoring.Alert(ctx, "session pinned game version artifact is missing")
		return session.StartResult{}, session.ErrPinnedDefinitionMissing
	}

	// The roster is built strictly from Participants active at the
	// serialized Start moment, ordered by joined_at ascending, ties broken
	// by internal actor id.
	roster, err := m.startRepo.ListActiveParticipantsForRoster(ctx, tx, lockedSession.ID)
	if err != nil {
		return session.StartResult{}, err
	}
	if len(roster) < artifact.ParticipantMin || (artifact.ParticipantMax != nil && len(roster) > *artifact.ParticipantMax) {
		// The participant-max case is defensive - Join already enforces it
		// and is not expected to ever trigger here in practice - grouped
		// under the same ordinary LOBBY-phase decline as participant-min.
		if err := m.startRepo.CompleteSessionRequest(ctx, tx, requestID, &lockedSession.ID, outcomeNotEnoughPlayers, ""); err != nil {
			return session.StartResult{}, fmt.Errorf("completing start session request: %s", err)
		}
		return session.StartResult{Outcome: session.StartOutcomeNotEnoughPlayers}, nil
	}
	known := knownActorsFromRoster(roster)

	players := make([]platform.ActorRef, len(roster))
	for i, p := range roster {
		players[i] = actorRefForActorID(p.ActorID)
	}

	// RootParameters has no source yet: nothing in the current platform
	// surface lets a host supply custom Session-start parameters at
	// Create/Start time (a future need, not yet designed) - nil is honest,
	// not a placeholder for a feature this WORK silently invented. The
	// authored script's own reaction to SESSION_STARTED still receives the
	// full Players roster to construct whatever initial state it needs.
	event := platform.NewSessionStarted(nil, players)
	encodedEvent, err := platform.EncodeEvent(event)
	if err != nil {
		return session.StartResult{}, fmt.Errorf("encoding session started event: %s", err)
	}

	// Unlike every other RUNNING-phase step, Start has no ordinary "decline,
	// stay as you were" path for a rejected Event: a fresh Session's first
	// execution either succeeds or the Session never starts at all, so
	// *executor.ScriptRejectedError is treated exactly like an
	// *executor.ExecutorError here - both terminalize via the same fatal
	// path, mirroring this call site's pre-existing behavior under the
	// retired engine (StartTurn's own error already had no decline branch
	// either).
	output, err := m.executor.Execute(ctx, executor.ExecutionInput{
		Script:        executor.ResolvedScript{Source: artifact.BackendScript},
		PreviousState: nil,
		Event:         encodedEvent,
		Context:       executor.ExecutionContext{LogicalTime: now, RandomSeed: drawSeed()},
	})
	if err != nil {
		errorCode, errorMessage := classifyExecutionError(err)
		return m.terminalizeStartFatal(ctx, tx, lockedSession, requestID, now, session.TerminalReasonRuntimeExecutionFailed,
			RuntimeFailureKindExecution, errorCode, errorMessage)
	}

	commands, err := parseCommands(output.RequestedCommands, known)
	if err != nil {
		errorCode, errorMessage := classifyExecutionError(err)
		return m.terminalizeStartFatal(ctx, tx, lockedSession, requestID, now, session.TerminalReasonRuntimeExecutionFailed,
			RuntimeFailureKindExecution, errorCode, errorMessage)
	}

	turnID, err := m.startRepo.CreateRuntimeTurn(ctx, tx, lockedSession.ID, 1, sessionStartSourceKind, nil, nil, nil, output.NewState)
	if err != nil {
		return session.StartResult{}, err
	}
	if err := m.startRepo.SetCurrentTurn(ctx, tx, lockedSession.ID, turnID); err != nil {
		return session.StartResult{}, err
	}
	if err := m.startRepo.SetSessionRunning(ctx, tx, lockedSession.ID, now, now.Add(m.activityTTL)); err != nil {
		return session.StartResult{}, err
	}

	// The authored script may complete/fail its own instance on its very
	// first execution - the Session genuinely started (Turn 1 committed,
	// started_at set above) and immediately ended, rather than never having
	// started at all.
	terminalReason, terminated := completion.Detect(commands)
	if terminated {
		if err := m.startRepo.SetSessionTerminal(ctx, tx, lockedSession.ID, now, terminalReason); err != nil {
			return session.StartResult{}, err
		}
		if err := m.startRepo.CancelAllActiveTimerObligationsForSession(ctx, tx, lockedSession.ID, session.TimerObligationClosureReasonSessionTerminated); err != nil {
			return session.StartResult{}, err
		}
	}

	if err := m.startRepo.RevokeActiveJoinCode(ctx, tx, lockedSession.ID, now); err != nil {
		return session.StartResult{}, err
	}

	result := session.StartResult{Outcome: session.StartOutcomeStarted, SessionUUID: session.SessionUUID(lockedSession.UUID), TerminalReason: terminalReason}
	responseBytes, err := json.Marshal(result)
	if err != nil {
		return session.StartResult{}, fmt.Errorf("marshaling start response payload: %s", err)
	}
	if err := m.startRepo.CompleteSessionRequest(ctx, tx, requestID, &lockedSession.ID, outcomeStarted, string(responseBytes)); err != nil {
		return session.StartResult{}, fmt.Errorf("completing start session request: %s", err)
	}
	return result, nil
}

// terminalizeStartFatal performs Start's pre-first-Turn fatal path:
// atomically terminalizes the Session directly from LOBBY, persists the
// session_runtime_failures diagnostic record (base_turn_id NULL,
// attempted_sequence 1 - no Turn has ever committed yet), revokes its
// JoinCode, and records the fatal outcome as the START idempotency claim's
// completed - and replayable - outcome, so a Start that already fatally
// terminalized the Session is never re-attempted on a same-token retry.
// started_at is left at its canonical NULL - the Session never actually
// ran. No session_runtime_turns row is written.
func (m *Manager) terminalizeStartFatal(ctx context.Context, tx *gorm.DB, lockedSession *internalrepo.Session, requestID uint, terminalAt time.Time, terminalReason string, failureKind string, errorCode string, errorMessage string) (session.StartResult, error) {
	if err := m.materializeRuntimeFailure(ctx, tx, m.startRepo, lockedSession, terminalAt, terminalReason,
		failureKind, errorCode, errorMessage, nil, 1, sessionStartSourceKind, nil, nil); err != nil {
		return session.StartResult{}, err
	}
	if err := m.startRepo.RevokeActiveJoinCode(ctx, tx, lockedSession.ID, terminalAt); err != nil {
		return session.StartResult{}, err
	}
	if err := m.startRepo.CompleteSessionRequest(ctx, tx, requestID, &lockedSession.ID, outcomeRuntimeInitFailed, ""); err != nil {
		return session.StartResult{}, fmt.Errorf("completing start session request: %s", err)
	}
	return session.StartResult{Outcome: session.StartOutcomeRuntimeInitFailed}, nil
}

// interpretExistingStartClaim decides what an already-claimed START
// identity means for the incoming request: replay or conflict. A replayed
// decline - including the fatal RuntimeInitFailed outcome - is returned as
// the same outcome value it was originally recorded as, never reconstructed
// as an error and never re-attempted against a Session that has since
// become TERMINAL.
func interpretExistingStartClaim(existing *internalrepo.Request, incoming startRequestPayload) (session.StartResult, error) {
	if existing.Status != internalrepo.RequestStatusCompleted {
		return session.StartResult{}, session.ErrIdempotencyInFlight
	}

	var stored startRequestPayload
	if err := json.Unmarshal([]byte(existing.RequestPayload), &stored); err != nil {
		return session.StartResult{}, fmt.Errorf("decoding stored start request payload: %s", err)
	}
	if stored != incoming {
		return session.StartResult{}, session.ErrIdempotencyConflict
	}

	switch existing.Outcome {
	case outcomeNotHost:
		return session.StartResult{Outcome: session.StartOutcomeNotHost}, nil
	case outcomeNotEnoughPlayers:
		return session.StartResult{Outcome: session.StartOutcomeNotEnoughPlayers}, nil
	case outcomeRuntimeInitFailed:
		return session.StartResult{Outcome: session.StartOutcomeRuntimeInitFailed}, nil
	case outcomeLobbyExpired:
		return session.StartResult{Outcome: session.StartOutcomeLobbyExpired}, nil
	}

	if existing.ResponsePayload == nil {
		return session.StartResult{}, fmt.Errorf("completed start idempotency record missing response payload")
	}
	var result session.StartResult
	if err := json.Unmarshal([]byte(*existing.ResponsePayload), &result); err != nil {
		return session.StartResult{}, fmt.Errorf("decoding stored start response payload: %s", err)
	}
	return result, nil
}

// drawSeed draws ExecutionContext.RandomSeed from a fresh, unpredictable
// source, once per Execute call - the Executor itself never reads OS
// randomness. crypto/rand is a legitimate real-entropy source for this
// per-call draw. Strict deterministic replay across calls is not a
// requirement anything currently depends on, so a fresh seed each call -
// rather than one derived to reproduce a specific prior run - is
// sufficient.
func drawSeed() uint64 {
	var buf [8]byte
	if _, err := cryptorand.Read(buf[:]); err != nil {
		// crypto/rand.Read failing is not a realistic operational condition
		// on any supported platform; falling back to wall-clock time keeps
		// this from hard-failing on it while still drawing a value that is
		// not guessable in advance.
		return uint64(time.Now().UnixNano())
	}
	return binary.LittleEndian.Uint64(buf[:])
}
