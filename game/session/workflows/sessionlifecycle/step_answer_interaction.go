package sessionlifecycle

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/game/session/internal/sessionlock"
	internalrepo "github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/utils"
	"gorm.io/gorm"
)

// answerInteractionSourceKind is the source_kind label persisted on the
// RuntimeTurn an accepted interaction response causes.
const answerInteractionSourceKind = "INTERACTION_RESPONSE"

// answerInteractionRepoAPI is AnswerInteraction's own narrow persistence
// contract. The shared sessionlock mechanism package is called directly by
// this step instead of through a repository forwarding method.
type answerInteractionRepoAPI interface {
	interactionCaptureRepoAPI
	ResolveSessionForInteraction(ctx context.Context, interactionUUID string) (*uint, error)
	FindInteractionByUUID(ctx context.Context, tx *gorm.DB, interactionUUID string) (*internalrepo.Interaction, error)
	FindActor(ctx context.Context, tx *gorm.DB, sessionID uint, userUUID string) (*internalrepo.Actor, error)
	GetRuntimeTurn(ctx context.Context, tx *gorm.DB, turnID uint) (*internalrepo.RuntimeTurn, error)
	GetRuntimeStart(ctx context.Context, tx *gorm.DB, sessionID uint) (*internalrepo.RuntimeStart, error)
	ListRuntimeTurns(ctx context.Context, tx *gorm.DB, sessionID uint) ([]internalrepo.RuntimeTurnRecord, error)
	GetInteractionByID(ctx context.Context, tx *gorm.DB, interactionID uint) (*internalrepo.Interaction, error)
	CreateRuntimeTurn(ctx context.Context, tx *gorm.DB, sessionID uint, sequence uint64, sourceKind string, sourceInteractionID *uint, actorID *uint) (uint, error)
	SetCurrentTurn(ctx context.Context, tx *gorm.DB, sessionID uint, currentTurnID uint) error
	CloseAnsweredInteraction(ctx context.Context, tx *gorm.DB, interactionID uint, responsePayload []byte, closedByTurnID uint) error
	SetSessionTerminal(ctx context.Context, tx *gorm.DB, sessionID uint, terminalAt time.Time, terminalReason string) error
	CloseAllActiveInteractionsForSession(ctx context.Context, tx *gorm.DB, sessionID uint, reason string) error
}

// AnswerInteraction submits answer as the caller's response to the
// still-ACTIVE interaction interactionUUID, against a RUNNING Session. At
// most one mutation against a given Session executes at a time; this call
// waits its turn and then always reloads current authoritative state before
// applying the response.
//
// answer is a typed engine.Value; decoding a transport-level payload into
// one is the caller's responsibility.
//
// No idempotency key is required: the interaction's own persisted state is
// the dedup identity. A retried, semantically equivalent response replays
// AnswerInteractionOutcomeAnswered without a second engine effect; a
// conflicting different response against an already-resolved interaction is
// rejected as AnswerInteractionOutcomeConflict.
func (m *Manager) AnswerInteraction(ctx context.Context, interactionUUID InteractionUUID, userUUID UserUUID, answer engine.Value) (AnswerInteractionResult, error) {
	defer logging.Step(ctx, "SessionLifecycle.AnswerInteraction").Close()
	logging.LogFields(ctx,
		logging.Field("interaction_uuid", string(interactionUUID)),
		logging.Field("user_uuid", string(userUUID)),
	)

	sessionID, err := m.answerInteractionRepo.ResolveSessionForInteraction(ctx, string(interactionUUID))
	if err != nil {
		return AnswerInteractionResult{}, err
	}
	if sessionID == nil {
		return AnswerInteractionResult{}, session.ErrInteractionNotFound
	}

	return utils.RunInDBTransaction(ctx, m, func(ctx context.Context, tx *gorm.DB) (AnswerInteractionResult, error) {
		return m.answerInteractionInTx(ctx, tx, *sessionID, interactionUUID, userUUID, answer)
	})
}

// answerInteractionInTx is AnswerInteraction's per-transaction logic,
// isolated so its pre-engine-execution checks (respondent authorization,
// interaction state) can be tested without a real database connection; the
// lock acquisition and engine execution beyond this point require one.
func (m *Manager) answerInteractionInTx(ctx context.Context, tx *gorm.DB, sessionID uint, interactionUUID InteractionUUID, userUUID UserUUID, answer engine.Value) (AnswerInteractionResult, error) {
	lockedSession, err := sessionlock.LockByID(ctx, tx, sessionID)
	if err != nil {
		return AnswerInteractionResult{}, err
	}
	if lockedSession == nil {
		return AnswerInteractionResult{}, session.ErrSessionNotFound
	}

	interaction, err := m.answerInteractionRepo.FindInteractionByUUID(ctx, tx, string(interactionUUID))
	if err != nil {
		return AnswerInteractionResult{}, err
	}
	if interaction == nil {
		return AnswerInteractionResult{}, session.ErrInteractionNotFound
	}

	// A missing actor is indistinguishable from "not this interaction's
	// recipient" for this purpose - rejected the same way the engine itself
	// would reject an unauthorized Respondent, without needing to reach the
	// engine at all.
	actor, err := m.answerInteractionRepo.FindActor(ctx, tx, lockedSession.ID, string(userUUID))
	if err != nil {
		return AnswerInteractionResult{}, err
	}
	if actor == nil || actor.ID != interaction.SessionActorID {
		return AnswerInteractionResult{Outcome: AnswerInteractionOutcomeRejected, SessionUUID: SessionUUID(lockedSession.UUID)}, nil
	}

	responsePayload, err := engineservice.EncodeValue(answer)
	if err != nil {
		return AnswerInteractionResult{}, fmt.Errorf("encoding interaction response: %s", err)
	}

	if interaction.State != session.InteractionStateActive {
		if interaction.State == session.InteractionStateClosed {
			// Compared as decoded engine.Values, via Value's own Equal, not
			// as raw bytes: response_payload round-trips through a JSONB
			// column, which does not preserve object key order, so a
			// stored/freshly-encoded byte comparison would be unreliable.
			storedResponse, err := engineservice.DecodeValue(interaction.ResponsePayload)
			if err != nil {
				return AnswerInteractionResult{}, fmt.Errorf("decoding stored interaction response: %s", err)
			}
			if storedResponse.Equal(answer) {
				return AnswerInteractionResult{Outcome: AnswerInteractionOutcomeAnswered, SessionUUID: SessionUUID(lockedSession.UUID)}, nil
			}
			return AnswerInteractionResult{Outcome: AnswerInteractionOutcomeConflict, SessionUUID: SessionUUID(lockedSession.UUID)}, nil
		}
		// InteractionStateTerminated (or any other non-ACTIVE state): the
		// interaction is no longer answerable, an ordinary stale decline.
		return AnswerInteractionResult{Outcome: AnswerInteractionOutcomeRejected, SessionUUID: SessionUUID(lockedSession.UUID)}, nil
	}

	if lockedSession.CurrentTurnID == nil {
		// An ACTIVE interaction only exists because some committed
		// RuntimeTurn created it, so a locked Session that still owns one
		// but reports no current_turn_id at all is a data-integrity
		// condition, not an ordinary decline.
		monitoring.Alert(ctx, "session has an active interaction but no current_turn_id")
		return AnswerInteractionResult{}, fmt.Errorf("session %d has an active interaction but no current_turn_id", lockedSession.ID)
	}
	currentTurn, err := m.answerInteractionRepo.GetRuntimeTurn(ctx, tx, *lockedSession.CurrentTurnID)
	if err != nil {
		return AnswerInteractionResult{}, err
	}
	if currentTurn == nil {
		monitoring.Alert(ctx, "session current_turn_id does not resolve to a runtime turn")
		return AnswerInteractionResult{}, fmt.Errorf("session %d current_turn_id %d does not resolve to a runtime turn", lockedSession.ID, *lockedSession.CurrentTurnID)
	}

	now := time.Now().UTC()

	// The pinned Definition/Version UUID is read directly, never the Game's
	// current version.
	definition, err := m.pinnedGameReader.GetGameDefinition(ctx, lockedSession.GameDefinitionUUID)
	if err != nil {
		return AnswerInteractionResult{}, err
	}
	if definition == nil {
		monitoring.Alert(ctx, "session pinned game definition is missing")
		return AnswerInteractionResult{}, session.ErrPinnedDefinitionMissing
	}
	compiledProgram, diagnostics := engineservice.Compile(*definition)
	if diagnostics.HasErrors() {
		// The pinned Definition already compiled successfully at Create/at
		// every prior RuntimeTurn - an unexpected recompile failure now is a
		// data-integrity problem, not a deterministic authored-game failure.
		monitoring.Alert(ctx, fmt.Sprintf(
			"pinned game definition failed to recompile answering interaction: session_uuid=%s game_definition_uuid=%s",
			lockedSession.UUID, lockedSession.GameDefinitionUUID,
		))
		return m.terminalizeAnswerInteractionFatal(ctx, tx, lockedSession, now, session.TerminalReasonRuntimeStateInvalid)
	}

	// Current authoritative Runtime state is never loaded from a persisted
	// Snapshot (none exists), and this package never reconstructs one
	// itself - engineservice.AdvanceTurn internally replays every durable
	// cause committed so far, given only the ordered signal log below.
	input, priorSignals, err := m.loadPriorSignals(ctx, tx, lockedSession.ID)
	if err != nil {
		return AnswerInteractionResult{}, fmt.Errorf("reconstructing current runtime state: %s", err)
	}

	signal := engine.Signal{
		Kind:          engine.SignalKindInteractionAnswered,
		InteractionID: engine.InteractionID(interaction.EngineInteractionID),
		Respondent:    engine.UserID(strconv.FormatUint(uint64(actor.ID), 10)),
		Answer:        answer,
	}

	outputs, err := engineservice.AdvanceTurn(compiledProgram, input, priorSignals, signal, engine.DefaultLimits())
	if err != nil {
		if errors.Is(err, engineservice.ErrReplayDivergence) {
			// Every element of priorSignals already succeeded once - it is
			// durable specifically because it did - so failing to replay it
			// identically is a data-integrity condition, never an ordinary
			// decline.
			monitoring.Alert(ctx, fmt.Sprintf("session %d: %s", lockedSession.ID, err))
			return AnswerInteractionResult{}, fmt.Errorf("reconstructing current runtime state: %s", err)
		}
		// A rejection of the response itself is an ordinary declined
		// outcome: no RuntimeTurn, no Snapshot mutation, Session stays
		// RUNNING.
		if errors.Is(err, engineservice.ErrSignalRejected) || errors.Is(err, engineservice.ErrInputRejected) {
			return AnswerInteractionResult{Outcome: AnswerInteractionOutcomeRejected, SessionUUID: SessionUUID(lockedSession.UUID)}, nil
		}
		return m.terminalizeAnswerInteractionFatal(ctx, tx, lockedSession, now, session.TerminalReasonRuntimeExecutionFailed)
	}

	actorID := actor.ID
	turnID, err := m.answerInteractionRepo.CreateRuntimeTurn(ctx, tx, lockedSession.ID, currentTurn.Sequence+1, answerInteractionSourceKind, &interaction.ID, &actorID)
	if err != nil {
		return AnswerInteractionResult{}, err
	}
	// The engine clears an accepted answer's own slot internally, before
	// the transition's own operations run, and produces no Output
	// recording that closure - only an authored CloseQuestionOperation on a
	// *different* slot ever does. The answered interaction is therefore
	// closed directly by its already-known id instead of being discovered
	// through captured Outputs. This runs before captureInteractions so an
	// authored transition that reopens this same (session, InteractionID)
	// key within the same Turn finds it already closed, not still occupying
	// the active-interaction uniqueness constraint.
	if err := m.answerInteractionRepo.CloseAnsweredInteraction(ctx, tx, interaction.ID, responsePayload, turnID); err != nil {
		return AnswerInteractionResult{}, err
	}
	if err := captureInteractions(ctx, tx, m.answerInteractionRepo, lockedSession.ID, turnID, outputs); err != nil {
		return AnswerInteractionResult{}, err
	}
	if err := m.answerInteractionRepo.SetCurrentTurn(ctx, tx, lockedSession.ID, turnID); err != nil {
		return AnswerInteractionResult{}, err
	}

	return AnswerInteractionResult{Outcome: AnswerInteractionOutcomeAnswered, SessionUUID: SessionUUID(lockedSession.UUID)}, nil
}

// terminalizeAnswerInteractionFatal performs AnswerInteraction's fatal path:
// atomically terminalizes the Session and closes every currently-ACTIVE
// session_interactions row for it, so a TERMINAL Session never retains one
// still ACTIVE. No partial RuntimeTurn is ever persisted:
// sessions.current_turn_id remains at the last committed Turn.
func (m *Manager) terminalizeAnswerInteractionFatal(ctx context.Context, tx *gorm.DB, lockedSession *sessionlock.Session, terminalAt time.Time, terminalReason string) (AnswerInteractionResult, error) {
	if err := m.answerInteractionRepo.SetSessionTerminal(ctx, tx, lockedSession.ID, terminalAt, terminalReason); err != nil {
		return AnswerInteractionResult{}, err
	}
	if err := m.answerInteractionRepo.CloseAllActiveInteractionsForSession(ctx, tx, lockedSession.ID, session.InteractionClosureReasonSessionTerminated); err != nil {
		return AnswerInteractionResult{}, err
	}
	return AnswerInteractionResult{Outcome: AnswerInteractionOutcomeRuntimeExecutionFailed, SessionUUID: SessionUUID(lockedSession.UUID)}, nil
}
