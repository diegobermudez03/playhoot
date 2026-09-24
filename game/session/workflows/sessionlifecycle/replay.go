package sessionlifecycle

import (
	"context"
	"fmt"
	"strconv"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	internalrepo "github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/runtimeturn"
	"github.com/diegobermudez03/playhoot/monitoring"
	"gorm.io/gorm"
)

// reconstructCurrentSnapshot deterministically rebuilds sessionID's current
// authoritative engine.Snapshot purely from durable state - Start's
// persisted Seed/RootParameters and the ordered session_runtime_turns
// replay-input log, replayed against compiledProgram. It never reads or
// relies on a persisted Snapshot of any kind, and holds no cache of its own:
// every call replays from scratch, so this always reflects durable state
// even after total process loss. compiledProgram is the caller's
// already-compiled pinned Definition, so this never recompiles it a second
// time.
func (m *Manager) reconstructCurrentSnapshot(ctx context.Context, tx *gorm.DB, compiledProgram engine.Program, sessionID uint) (engine.Snapshot, error) {
	start, err := m.answerInteractionRepo.GetRuntimeStart(ctx, tx, sessionID)
	if err != nil {
		return engine.Snapshot{}, err
	}
	if start == nil {
		monitoring.Alert(ctx, fmt.Sprintf("session %d has no runtime start record", sessionID))
		return engine.Snapshot{}, fmt.Errorf("reconstructing session %d snapshot: no runtime start record", sessionID)
	}
	rootParameters, err := decodeRootParameters(start.RootParameters)
	if err != nil {
		monitoring.Alert(ctx, fmt.Sprintf("session %d has an undecodable runtime start record: %s", sessionID, err))
		return engine.Snapshot{}, fmt.Errorf("reconstructing session %d snapshot: decoding root parameters: %s", sessionID, err)
	}

	turns, err := m.answerInteractionRepo.ListRuntimeTurns(ctx, tx, sessionID)
	if err != nil {
		return engine.Snapshot{}, err
	}
	if len(turns) == 0 {
		monitoring.Alert(ctx, fmt.Sprintf("session %d has no committed runtime turns", sessionID))
		return engine.Snapshot{}, fmt.Errorf("reconstructing session %d snapshot: no committed runtime turns", sessionID)
	}

	snapshot, startSignal, err := engineservice.NewSnapshot(compiledProgram, engine.InitializationInput{
		RootParameters: rootParameters,
		Seed:           start.Seed,
	})
	if err != nil {
		monitoring.Alert(ctx, fmt.Sprintf("session %d: replaying start initialization diverged from its original commit: %s", sessionID, err))
		return engine.Snapshot{}, fmt.Errorf("reconstructing session %d snapshot: replaying start initialization: %s", sessionID, err)
	}
	current, err := replayTurn(compiledProgram, snapshot, startSignal, turns[0])
	if err != nil {
		monitoring.Alert(ctx, err.Error())
		return engine.Snapshot{}, err
	}

	for _, turn := range turns[1:] {
		signal, err := m.loadReplaySignal(ctx, tx, turn)
		if err != nil {
			return engine.Snapshot{}, err
		}
		current, err = replayTurn(compiledProgram, current, signal, turn)
		if err != nil {
			monitoring.Alert(ctx, err.Error())
			return engine.Snapshot{}, err
		}
	}
	return current, nil
}

// replayTurn drains one already-committed turn's replay-input signal against
// current and reports a data-integrity error if replaying it does not
// reproduce the same successful commit live execution already made - the
// pinned Definition and every durable input are immutable, so a replay of an
// already-committed cause is expected to always succeed identically.
func replayTurn(compiledProgram engine.Program, current engine.Snapshot, signal engine.Signal, turn internalrepo.RuntimeTurnRecord) (engine.Snapshot, error) {
	drainResult := runtimeturn.Drain(compiledProgram, current, signal)
	if drainResult.Err != nil {
		return engine.Snapshot{}, fmt.Errorf("reconstructing runtime turn %d (sequence %d): replay diverged from its original commit: %s", turn.ID, turn.Sequence, drainResult.Err)
	}
	return drainResult.Snapshot, nil
}

// loadReplaySignal loads turn's own durable cause and rebuilds the
// engine.Signal it drove. Only the cause kinds Start/AnswerInteraction can
// actually produce are supported today; a cause with no case here has no
// durable representation to reconstruct from yet.
func (m *Manager) loadReplaySignal(ctx context.Context, tx *gorm.DB, turn internalrepo.RuntimeTurnRecord) (engine.Signal, error) {
	switch turn.SourceKind {
	case answerInteractionSourceKind:
		if turn.SourceInteractionID == nil || turn.ActorID == nil {
			err := fmt.Errorf("reconstructing runtime turn %d: %s turn missing source_interaction_id/actor_id", turn.ID, answerInteractionSourceKind)
			monitoring.Alert(ctx, err.Error())
			return engine.Signal{}, err
		}
		interaction, err := m.answerInteractionRepo.GetInteractionByID(ctx, tx, *turn.SourceInteractionID)
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
// fields and its respondent's actor id - the same construction AnswerInteraction
// itself performs for the response it is currently processing.
func buildAnswerSignal(interaction *internalrepo.Interaction, actorID uint) (engine.Signal, error) {
	signalKind, err := answerSignalKind(interaction.Kind)
	if err != nil {
		return engine.Signal{}, err
	}
	answer, err := engineservice.DecodeValue(interaction.ResponsePayload)
	if err != nil {
		return engine.Signal{}, fmt.Errorf("decoding interaction response: %s", err)
	}
	return engine.Signal{
		Kind:       signalKind,
		Slot:       interaction.EngineSlot,
		Respondent: engine.UserID(strconv.FormatUint(uint64(actorID), 10)),
		Answer:     answer,
	}, nil
}

// encodeRootParameters encodes rootParameters (an
// engine.InitializationInput.RootParameters value) as
// session_runtime_starts.root_parameters, reusing the engine's own
// FieldValue-list encoding by wrapping it in a nameless RecordValue - the
// same technique interaction_capture.go already uses for an open question's
// Arguments - rather than maintaining a second encoding for the same shape.
func encodeRootParameters(rootParameters map[string]engine.Value) ([]byte, error) {
	fields := make([]engine.FieldValue, 0, len(rootParameters))
	for name, value := range rootParameters {
		fields = append(fields, engine.FieldValue{Name: name, Value: value})
	}
	return engineservice.EncodeValue(engine.RecordValue{Fields: fields})
}

// decodeRootParameters decodes session_runtime_starts.root_parameters back
// into an engine.InitializationInput.RootParameters value, the counterpart
// to encodeRootParameters.
func decodeRootParameters(data []byte) (map[string]engine.Value, error) {
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
