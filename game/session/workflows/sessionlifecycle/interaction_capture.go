package sessionlifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/runtimeturn"
	"gorm.io/gorm"
)

// interactionCaptureRepoAPI is the narrow persistence contract
// captureInteractions needs - satisfied by both startRepoAPI and
// answerInteractionRepoAPI, so every RuntimeTurn-producing step captures
// interactions the same way.
type interactionCaptureRepoAPI interface {
	CreateInteraction(ctx context.Context, tx *gorm.DB, sessionID uint, sessionActorID uint, kind string, enginePath []byte, engineSlot string, interactionPayload []byte, openedByTurnID uint) (uint, error)
	CloseActiveInteraction(ctx context.Context, tx *gorm.DB, sessionID uint, enginePath []byte, engineSlot string, sessionActorID uint, closedByTurnID uint) error
}

// captureInteractions durably records every engine.OpenQuestionOutput/
// CloseQuestionOutput produced anywhere within a just-drained RuntimeTurn
// (steps, in Step-call order) as session_interactions rows opened/closed by
// turnID. Every step that persists a RuntimeTurn must call this so a
// question/ask-group instance a Turn opens is never left unrecorded.
//
// compiledProgram resolves each OpenQuestionOutput's Slot to QUESTION vs
// ASK_GROUP by looking up the owning Workflow's QuestionSlots/AskGroupSlots
// - the only place that distinction is available, since OpenQuestionOutput
// itself carries no such discriminator.
func captureInteractions(ctx context.Context, tx *gorm.DB, repo interactionCaptureRepoAPI, compiledProgram engine.Program, sessionID uint, turnID uint, steps []runtimeturn.StepTrace) error {
	for _, step := range steps {
		if len(step.Outputs) == 0 {
			continue
		}
		for _, output := range step.Outputs {
			switch o := output.(type) {
			case engine.OpenQuestionOutput:
				actorID, err := parseActorID(o.Recipient)
				if err != nil {
					return fmt.Errorf("parsing interaction recipient: %s", err)
				}
				kind, err := resolveInteractionKind(compiledProgram, step.Workflow, o.Slot)
				if err != nil {
					return err
				}
				payload, err := encodeInteractionPayload(o.Question, o.Arguments)
				if err != nil {
					return fmt.Errorf("encoding interaction payload: %s", err)
				}
				if _, err := repo.CreateInteraction(ctx, tx, sessionID, actorID, kind, emptyEnginePath, o.Slot, payload, turnID); err != nil {
					return err
				}
			case engine.CloseQuestionOutput:
				// Only ever produced by an authored CloseQuestionOperation
				// explicitly closing a still-pending question without an
				// answer (cleanup, cancellation, abandonment). Answering a
				// question never reaches here: the engine clears an accepted
				// answer's own slot internally, before the transition's own
				// operations run, and produces no Output for that closure.
				actorID, err := parseActorID(o.Recipient)
				if err != nil {
					return fmt.Errorf("parsing interaction recipient: %s", err)
				}
				if err := repo.CloseActiveInteraction(ctx, tx, sessionID, emptyEnginePath, o.Slot, actorID, turnID); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// resolveInteractionKind determines whether slotName on workflowName is a
// QuestionSlot or an AskGroupSlot, read directly off the compiled
// engine.Program's own slot declarations - the only place this distinction
// is available.
func resolveInteractionKind(p engine.Program, workflowName, slotName string) (string, error) {
	workflow, ok := p.Workflows[workflowName]
	if !ok {
		return "", fmt.Errorf("resolving interaction kind: unknown workflow %q", workflowName)
	}
	for _, s := range workflow.QuestionSlots {
		if s.Name == slotName {
			return session.InteractionKindQuestion, nil
		}
	}
	for _, s := range workflow.AskGroupSlots {
		if s.Name == slotName {
			return session.InteractionKindAskGroup, nil
		}
	}
	return "", fmt.Errorf("resolving interaction kind: workflow %q declares no question/ask-group slot %q", workflowName, slotName)
}

// parseActorID recovers the internal SessionActorID an engine.UserID
// encodes as its decimal string form - the only value ever bound to a
// `list<user>` element or a question's Recipient, so it is always a plain
// internal actor id, never a public user identifier.
func parseActorID(recipient engine.UserID) (uint, error) {
	id, err := strconv.ParseUint(string(recipient), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing engine.UserID %q as session actor id: %s", recipient, err)
	}
	return uint(id), nil
}

// emptyEnginePath is session_interactions.engine_path's persisted value for
// every interaction: a Session runs exactly one workflow instance, so there
// is no tree-addressing information left to encode. This is byte-identical
// to what this column has always held for every interaction ever captured.
var emptyEnginePath = []byte("[]")

// interactionPayloadWire is interaction_payload's wire shape. Arguments is
// engine.Value-typed data (each engine.FieldValue's Value), so it is
// captured through engineservice.EncodeValue - reusing the engine's own
// FieldValue-list encoding by wrapping Arguments in a nameless RecordValue,
// rather than maintaining a second encoding for the same shape.
type interactionPayloadWire struct {
	Question  string          `json:"question"`
	Arguments json.RawMessage `json:"arguments"`
}

func encodeInteractionPayload(question string, arguments []engine.FieldValue) ([]byte, error) {
	encodedArguments, err := engineservice.EncodeValue(engine.RecordValue{Fields: arguments})
	if err != nil {
		return nil, fmt.Errorf("encoding interaction arguments: %s", err)
	}
	return json.Marshal(interactionPayloadWire{Question: question, Arguments: encodedArguments})
}
