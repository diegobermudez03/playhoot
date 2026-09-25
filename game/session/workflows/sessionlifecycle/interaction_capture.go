package sessionlifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/game/session"
	"gorm.io/gorm"
)

// interactionCaptureRepoAPI is the narrow persistence contract
// captureInteractions needs - satisfied by both startRepoAPI and
// answerInteractionRepoAPI, so every RuntimeTurn-producing step captures
// interactions the same way.
type interactionCaptureRepoAPI interface {
	CreateInteraction(ctx context.Context, tx *gorm.DB, sessionID uint, sessionActorID uint, kind string, engineInteractionID uint64, interactionPayload []byte, openedByTurnID uint) (uint, error)
	CloseActiveInteraction(ctx context.Context, tx *gorm.DB, sessionID uint, engineInteractionID uint64, sessionActorID uint, closedByTurnID uint) error
}

// captureInteractions durably records every engine.OpenQuestionOutput/
// CloseQuestionOutput produced by a just-processed RuntimeTurn as
// session_interactions rows opened/closed by turnID. Every step that
// persists a RuntimeTurn must call this so a question/ask-group instance a
// Turn opens is never left unrecorded.
//
// outputs is the flat, ordered Output list engineservice.StartTurn/
// AdvanceTurn already return for one Turn - this package has no reason to
// know, and never needs to know, how many internal engine Steps produced
// them.
//
// Each Output already carries its own engine-assigned InteractionID and Kind
// (QUESTION vs ASK_GROUP) directly - no compiled Program lookup is needed to
// classify what was opened.
func captureInteractions(ctx context.Context, tx *gorm.DB, repo interactionCaptureRepoAPI, sessionID uint, turnID uint, outputs []engine.Output) error {
	for _, output := range outputs {
		switch o := output.(type) {
		case engine.OpenQuestionOutput:
			actorID, err := parseActorID(o.Recipient)
			if err != nil {
				return fmt.Errorf("parsing interaction recipient: %s", err)
			}
			kind, err := interactionKindLabel(o.Kind)
			if err != nil {
				return err
			}
			payload, err := encodeInteractionPayload(o.Question, o.Arguments)
			if err != nil {
				return fmt.Errorf("encoding interaction payload: %s", err)
			}
			if _, err := repo.CreateInteraction(ctx, tx, sessionID, actorID, kind, uint64(o.InteractionID), payload, turnID); err != nil {
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
			if err := repo.CloseActiveInteraction(ctx, tx, sessionID, uint64(o.InteractionID), actorID, turnID); err != nil {
				return err
			}
		}
	}
	return nil
}

// interactionKindLabel translates an engine.InteractionKind into the
// persisted session_interactions.kind label - a plain, closed translation,
// since InteractionKind is the engine's own exhaustive classification of
// what it just opened.
func interactionKindLabel(kind engine.InteractionKind) (string, error) {
	switch kind {
	case engine.InteractionKindQuestion:
		return session.InteractionKindQuestion, nil
	case engine.InteractionKindAskGroup:
		return session.InteractionKindAskGroup, nil
	default:
		return "", fmt.Errorf("translating interaction kind: unknown engine.InteractionKind %d", kind)
	}
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
