// Package replay reconstructs the durable signal log an already-running
// Session's current authoritative runtime state derives from, shared by
// sessionlifecycle's AnswerInteraction step (and any future step needing
// current runtime state before driving a new signal). It never
// reconstructs an engine.Snapshot itself; that mechanism is entirely owned
// by engineservice.
package replay

import (
	"context"
	"fmt"
	"strconv"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	internalrepo "github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/monitoring"
	"gorm.io/gorm"
)

// AnswerInteractionSourceKind is the source_kind label persisted on the
// RuntimeTurn an accepted interaction response causes - the one durable
// cause LoadPriorSignals currently knows how to reconstruct a signal from.
const AnswerInteractionSourceKind = "INTERACTION_RESPONSE"

// Repo is the narrow persistence contract LoadPriorSignals needs.
type Repo interface {
	GetRuntimeStart(ctx context.Context, tx *gorm.DB, sessionID uint) (*internalrepo.RuntimeStart, error)
	ListRuntimeTurns(ctx context.Context, tx *gorm.DB, sessionID uint) ([]internalrepo.RuntimeTurnRecord, error)
	GetInteractionByID(ctx context.Context, tx *gorm.DB, interactionID uint) (*internalrepo.Interaction, error)
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
