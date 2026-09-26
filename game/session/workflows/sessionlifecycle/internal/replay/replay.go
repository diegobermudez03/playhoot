// Package replay reconstructs the durable signal log an already-running
// Session's current authoritative runtime state derives from, shared by
// sessionlifecycle's AnswerInteraction step (and any future step needing
// current runtime state before driving a new signal). It never
// reconstructs an engine.Snapshot itself; that mechanism is entirely owned
// by engineservice.
package replay

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	internalrepo "github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/monitoring"
	"gorm.io/gorm"
)

// AnswerInteractionSourceKind is the source_kind label persisted on the
// RuntimeTurn an accepted interaction response causes.
const AnswerInteractionSourceKind = "INTERACTION_RESPONSE"

// TimerExpiredSourceKind is the source_kind label persisted on the
// RuntimeTurn a timer obligation's expiration causes (Manager.ExpireTimer) -
// covers both the ordinary TimerSlot and KeyedTimerSlot shapes, discriminated
// by the obligation's own EngineKey.
const TimerExpiredSourceKind = "TIMER_EXPIRED"

// UserIntentSourceKind is the source_kind label persisted on the
// RuntimeTurn a submitted user intent causes (Manager.SubmitUserIntent).
// Its durable content lives in session_cause_events (cause_kind
// "USER_INTENT"), referenced by the Turn's source_cause_event_id.
const UserIntentSourceKind = "USER_INTENT"

// SessionCancelledSourceKind is the source_kind label persisted on the
// RuntimeTurn a host's manual cancellation causes when the authored game
// itself has a transition matching SessionCancelled (Manager.CancelSession).
// Its durable content lives in session_cause_events (cause_kind
// "SESSION_CANCELLED"), referenced by the Turn's source_cause_event_id. A
// cancellation the engine rejects outright never reaches this - no
// RuntimeTurn is created for it (see CancelSession's own Approved Design).
const SessionCancelledSourceKind = "SESSION_CANCELLED"

// Repo is the narrow persistence contract LoadPriorSignals needs.
type Repo interface {
	GetRuntimeStart(ctx context.Context, tx *gorm.DB, sessionID uint) (*internalrepo.RuntimeStart, error)
	ListRuntimeTurns(ctx context.Context, tx *gorm.DB, sessionID uint) ([]internalrepo.RuntimeTurnRecord, error)
	GetInteractionByID(ctx context.Context, tx *gorm.DB, interactionID uint) (*internalrepo.Interaction, error)
	GetTimerObligationByID(ctx context.Context, tx *gorm.DB, timerObligationID uint) (*internalrepo.TimerObligation, error)
	GetCauseEventByID(ctx context.Context, tx *gorm.DB, causeEventID uint) (*internalrepo.CauseEvent, error)
}

// LoadPriorSignals loads sessionID's Start record and every already-
// committed session_runtime_turns row, in order, and returns the
// engine.InitializationInput Start requires plus the ordered engine.Signal
// each subsequent turn drove - exactly what engineservice.AdvanceTurn needs
// to internally reconstruct current runtime state and process a new signal.
func LoadPriorSignals(ctx context.Context, tx *gorm.DB, repo Repo, sessionID uint) (engine.InitializationInput, []engine.Signal, error) {
	start, err := repo.GetRuntimeStart(ctx, tx, sessionID)
	if err != nil {
		return engine.InitializationInput{}, nil, err
	}
	if start == nil {
		monitoring.Alert(ctx, fmt.Sprintf("session %d has no runtime start record", sessionID))
		return engine.InitializationInput{}, nil, fmt.Errorf("loading session %d prior signals: no runtime start record", sessionID)
	}
	rootParameters, err := DecodeRootParameters(start.RootParameters)
	if err != nil {
		monitoring.Alert(ctx, fmt.Sprintf("session %d has an undecodable runtime start record: %s", sessionID, err))
		return engine.InitializationInput{}, nil, fmt.Errorf("loading session %d prior signals: decoding root parameters: %s", sessionID, err)
	}
	input := engine.InitializationInput{RootParameters: rootParameters, Seed: start.Seed}

	turns, err := repo.ListRuntimeTurns(ctx, tx, sessionID)
	if err != nil {
		return engine.InitializationInput{}, nil, err
	}
	if len(turns) == 0 {
		monitoring.Alert(ctx, fmt.Sprintf("session %d has no committed runtime turns", sessionID))
		return engine.InitializationInput{}, nil, fmt.Errorf("loading session %d prior signals: no committed runtime turns", sessionID)
	}

	// turns[0] is Start's own turn - its driving signal is the engine's own
	// synthesized WorkflowStarted, which engineservice.AdvanceTurn already
	// reproduces internally and never needs supplied. Only turns after it
	// have an externally-driven signal to reconstruct.
	priorSignals := make([]engine.Signal, 0, len(turns)-1)
	for _, turn := range turns[1:] {
		signal, err := loadReplaySignal(ctx, tx, repo, turn)
		if err != nil {
			return engine.InitializationInput{}, nil, err
		}
		priorSignals = append(priorSignals, signal)
	}
	return input, priorSignals, nil
}

// loadReplaySignal loads turn's own durable cause and rebuilds the
// engine.Signal it drove. Only the cause kinds a step can actually produce
// are supported today; a cause with no case here has no durable
// representation to reconstruct from yet.
func loadReplaySignal(ctx context.Context, tx *gorm.DB, repo Repo, turn internalrepo.RuntimeTurnRecord) (engine.Signal, error) {
	switch turn.SourceKind {
	case AnswerInteractionSourceKind:
		if turn.SourceInteractionID == nil || turn.ActorID == nil {
			err := fmt.Errorf("reconstructing runtime turn %d: %s turn missing source_interaction_id/actor_id", turn.ID, AnswerInteractionSourceKind)
			monitoring.Alert(ctx, err.Error())
			return engine.Signal{}, err
		}
		interaction, err := repo.GetInteractionByID(ctx, tx, *turn.SourceInteractionID)
		if err != nil {
			return engine.Signal{}, err
		}
		if interaction == nil {
			err := fmt.Errorf("reconstructing runtime turn %d: source interaction %d not found", turn.ID, *turn.SourceInteractionID)
			monitoring.Alert(ctx, err.Error())
			return engine.Signal{}, err
		}
		signal, err := buildAnswerSignal(interaction, *turn.ActorID)
		if err != nil {
			wrapped := fmt.Errorf("reconstructing runtime turn %d: %s", turn.ID, err)
			monitoring.Alert(ctx, wrapped.Error())
			return engine.Signal{}, wrapped
		}
		return signal, nil
	case TimerExpiredSourceKind:
		if turn.SourceTimerObligationID == nil {
			err := fmt.Errorf("reconstructing runtime turn %d: %s turn missing source_timer_obligation_id", turn.ID, TimerExpiredSourceKind)
			monitoring.Alert(ctx, err.Error())
			return engine.Signal{}, err
		}
		obligation, err := repo.GetTimerObligationByID(ctx, tx, *turn.SourceTimerObligationID)
		if err != nil {
			return engine.Signal{}, err
		}
		if obligation == nil {
			err := fmt.Errorf("reconstructing runtime turn %d: source timer obligation %d not found", turn.ID, *turn.SourceTimerObligationID)
			monitoring.Alert(ctx, err.Error())
			return engine.Signal{}, err
		}
		signal, err := buildTimerExpiredSignal(obligation)
		if err != nil {
			wrapped := fmt.Errorf("reconstructing runtime turn %d: %s", turn.ID, err)
			monitoring.Alert(ctx, wrapped.Error())
			return engine.Signal{}, wrapped
		}
		return signal, nil
	case UserIntentSourceKind:
		if turn.SourceCauseEventID == nil || turn.ActorID == nil {
			err := fmt.Errorf("reconstructing runtime turn %d: %s turn missing source_cause_event_id/actor_id", turn.ID, UserIntentSourceKind)
			monitoring.Alert(ctx, err.Error())
			return engine.Signal{}, err
		}
		causeEvent, err := repo.GetCauseEventByID(ctx, tx, *turn.SourceCauseEventID)
		if err != nil {
			return engine.Signal{}, err
		}
		if causeEvent == nil {
			err := fmt.Errorf("reconstructing runtime turn %d: source cause event %d not found", turn.ID, *turn.SourceCauseEventID)
			monitoring.Alert(ctx, err.Error())
			return engine.Signal{}, err
		}
		signal, err := buildUserIntentSignal(causeEvent, *turn.ActorID)
		if err != nil {
			wrapped := fmt.Errorf("reconstructing runtime turn %d: %s", turn.ID, err)
			monitoring.Alert(ctx, wrapped.Error())
			return engine.Signal{}, wrapped
		}
		return signal, nil
	case SessionCancelledSourceKind:
		if turn.SourceCauseEventID == nil {
			err := fmt.Errorf("reconstructing runtime turn %d: %s turn missing source_cause_event_id", turn.ID, SessionCancelledSourceKind)
			monitoring.Alert(ctx, err.Error())
			return engine.Signal{}, err
		}
		causeEvent, err := repo.GetCauseEventByID(ctx, tx, *turn.SourceCauseEventID)
		if err != nil {
			return engine.Signal{}, err
		}
		if causeEvent == nil {
			err := fmt.Errorf("reconstructing runtime turn %d: source cause event %d not found", turn.ID, *turn.SourceCauseEventID)
			monitoring.Alert(ctx, err.Error())
			return engine.Signal{}, err
		}
		return buildSessionCancelledSignal(), nil
	default:
		err := fmt.Errorf("reconstructing runtime turn %d: unsupported source_kind %q for replay", turn.ID, turn.SourceKind)
		monitoring.Alert(ctx, err.Error())
		return engine.Signal{}, err
	}
}

// buildAnswerSignal deterministically rebuilds the engine.Signal an accepted
// interaction response drove, purely from that interaction's own durable
// fields and its respondent's actor id - the same construction
// AnswerInteraction itself performs for the response it is currently
// processing.
func buildAnswerSignal(interaction *internalrepo.Interaction, actorID uint) (engine.Signal, error) {
	answer, err := engineservice.DecodeValue(interaction.ResponsePayload)
	if err != nil {
		return engine.Signal{}, fmt.Errorf("decoding interaction response: %s", err)
	}
	return engine.Signal{
		Kind:          engine.SignalKindInteractionAnswered,
		InteractionID: engine.InteractionID(interaction.EngineInteractionID),
		Respondent:    engine.UserID(strconv.FormatUint(uint64(actorID), 10)),
		Answer:        answer,
	}, nil
}

// buildTimerExpiredSignal deterministically rebuilds the engine.Signal a
// timer obligation's expiration drove, purely from that obligation's own
// durable fields - the ordinary SignalKindTimerExpired when EngineKey is
// nil, or SignalKindKeyedTimerExpired with the decoded authored key
// otherwise.
func buildTimerExpiredSignal(obligation *internalrepo.TimerObligation) (engine.Signal, error) {
	if obligation.EngineKey == nil {
		return engine.Signal{Kind: engine.SignalKindTimerExpired, Slot: obligation.EngineSlot}, nil
	}
	key, err := engineservice.DecodeValue(obligation.EngineKey)
	if err != nil {
		return engine.Signal{}, fmt.Errorf("decoding timer obligation key: %s", err)
	}
	return engine.Signal{Kind: engine.SignalKindKeyedTimerExpired, Slot: obligation.EngineSlot, Key: key}, nil
}

// buildUserIntentSignal deterministically rebuilds the engine.Signal a
// submitted user intent drove, purely from that cause event's own durable
// payload and its actor id - the same construction Manager.SubmitUserIntent
// itself performs for the intent it is currently processing.
func buildUserIntentSignal(causeEvent *internalrepo.CauseEvent, actorID uint) (engine.Signal, error) {
	intentName, arguments, err := DecodeUserIntentPayload(causeEvent.Payload)
	if err != nil {
		return engine.Signal{}, fmt.Errorf("decoding user intent payload: %s", err)
	}
	fields := make(map[string]engine.Value, len(arguments.Fields))
	for _, f := range arguments.Fields {
		fields[f.Name] = f.Value
	}
	return engine.Signal{
		Kind:   engine.SignalKindIntent,
		Intent: intentName,
		Actor:  engine.UserID(strconv.FormatUint(uint64(actorID), 10)),
		Fields: fields,
	}, nil
}

// buildSessionCancelledSignal deterministically rebuilds the engine.Signal a
// manual session cancellation drove. SessionCancelled carries no Fields (the
// compiler's own namedLifecycleSignals catalog declares it an empty schema),
// so nothing needs decoding from the cause event's payload - the cause
// event's existence and actor_id are its only meaningful durable content.
func buildSessionCancelledSignal() engine.Signal {
	return engine.Signal{Kind: engine.SignalKindNamed, Name: "SessionCancelled"}
}

// userIntentPayload is session_cause_events.payload's shape for a
// UserIntentSourceKind cause event: the submitted intent's name, plus its
// arguments encoded as a nameless engine.RecordValue - the same
// EncodeValue-wrapping technique EncodeRootParameters uses, rather than a
// second encoding for the same "named engine.Values" shape.
type userIntentPayload struct {
	IntentName string `json:"intent_name"`
	Arguments  []byte `json:"arguments"`
}

// EncodeUserIntentPayload encodes intentName and its already-decoded
// arguments (an engine.RecordValue) as session_cause_events.payload.
func EncodeUserIntentPayload(intentName string, arguments engine.RecordValue) ([]byte, error) {
	encodedArguments, err := engineservice.EncodeValue(arguments)
	if err != nil {
		return nil, fmt.Errorf("encoding user intent arguments: %s", err)
	}
	return json.Marshal(userIntentPayload{IntentName: intentName, Arguments: encodedArguments})
}

// DecodeUserIntentPayload decodes session_cause_events.payload back into
// the submitted intent's name and its arguments, the counterpart to
// EncodeUserIntentPayload.
func DecodeUserIntentPayload(data []byte) (string, engine.RecordValue, error) {
	var payload userIntentPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", engine.RecordValue{}, fmt.Errorf("unmarshaling user intent payload: %s", err)
	}
	value, err := engineservice.DecodeValue(payload.Arguments)
	if err != nil {
		return "", engine.RecordValue{}, fmt.Errorf("decoding user intent arguments: %s", err)
	}
	record, ok := value.(engine.RecordValue)
	if !ok {
		return "", engine.RecordValue{}, fmt.Errorf("decoded user intent arguments value is a %s, not a record", value.Kind())
	}
	return payload.IntentName, record, nil
}

// EncodeRootParameters encodes rootParameters (an
// engine.InitializationInput.RootParameters value) as
// session_runtime_starts.root_parameters, reusing the engine's own
// FieldValue-list encoding by wrapping it in a nameless RecordValue - the
// same technique the interactions package already uses for an open
// question's Arguments - rather than maintaining a second encoding for the
// same shape.
func EncodeRootParameters(rootParameters map[string]engine.Value) ([]byte, error) {
	fields := make([]engine.FieldValue, 0, len(rootParameters))
	for name, value := range rootParameters {
		fields = append(fields, engine.FieldValue{Name: name, Value: value})
	}
	return engineservice.EncodeValue(engine.RecordValue{Fields: fields})
}

// DecodeRootParameters decodes session_runtime_starts.root_parameters back
// into an engine.InitializationInput.RootParameters value, the counterpart
// to EncodeRootParameters.
func DecodeRootParameters(data []byte) (map[string]engine.Value, error) {
	value, err := engineservice.DecodeValue(data)
	if err != nil {
		return nil, err
	}
	record, ok := value.(engine.RecordValue)
	if !ok {
		return nil, fmt.Errorf("decoded root parameters value is a %s, not a record", value.Kind())
	}
	rootParameters := make(map[string]engine.Value, len(record.Fields))
	for _, f := range record.Fields {
		rootParameters[f.Name] = f.Value
	}
	return rootParameters, nil
}
