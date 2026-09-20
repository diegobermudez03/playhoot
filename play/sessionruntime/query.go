package sessionruntime

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/play"
)

// sessionRow is the minimal sessions columns needed to resolve which
// RuntimeTurn a just-committed call just made current.
type sessionRow struct {
	ID            uint
	CurrentTurnID *uint
}

// openedInteractionRow is one session_interactions row this Turn opened
// (opened_by_turn_id = the current Turn), joined to its recipient's
// caller-facing identity.
type openedInteractionRow struct {
	UUID               string
	UserUUID           string
	InteractionPayload []byte
}

// closedInteractionRow is one session_interactions row this Turn closed
// (closed_by_turn_id = the current Turn), joined to its recipient's
// caller-facing identity.
type closedInteractionRow struct {
	UUID     string
	UserUUID string
}

// eventsForCommittedTurn resolves sessionUUID's current_turn_id and
// translates every interaction that Turn opened/closed into play.Events.
// Called only after a manager call has already durably committed and
// reported success - never speculatively.
func (sr *SessionRuntime) eventsForCommittedTurn(ctx context.Context, sessionUUID string) ([]play.Event, error) {
	var row sessionRow
	result := sr.db.WithContext(ctx).Raw(`SELECT id, current_turn_id FROM sessions WHERE uuid = ?`, sessionUUID).Scan(&row)
	if result.Error != nil {
		return nil, fmt.Errorf("resolving session for event fan-out: %s", result.Error)
	}
	if result.RowsAffected == 0 || row.CurrentTurnID == nil {
		return nil, nil
	}
	return sr.eventsForTurn(ctx, row.ID, *row.CurrentTurnID)
}

// eventsForTurn translates every session_interactions row turnID opened or
// closed - the durable record of that Turn's OpenQuestionOutput/
// CloseQuestionOutput Outputs (interaction_capture.go's own capture,
// read back here rather than through any change to Manager) - into
// play.Events, one per recipient.
//
// A row turnID both opened and closed within the very same Turn (opened,
// then immediately closed by an authored transition later in the same
// Turn) is reported as both an Opened and a Closed Event, in that order -
// Coordinator's best-effort delivery makes redundant/rapidly-superseded
// Events harmless, and this keeps translation a direct, unconditional
// mirror of the persisted rows rather than requiring speculative
// suppression logic.
func (sr *SessionRuntime) eventsForTurn(ctx context.Context, sessionID, turnID uint) ([]play.Event, error) {
	var opened []openedInteractionRow
	if err := sr.db.WithContext(ctx).Raw(`
		SELECT si.uuid AS uuid, sa.user_uuid AS user_uuid, si.interaction_payload AS interaction_payload
		FROM session_interactions si
		JOIN session_actors sa ON sa.id = si.session_actor_id
		WHERE si.session_id = ? AND si.opened_by_turn_id = ?
	`, sessionID, turnID).Scan(&opened).Error; err != nil {
		return nil, fmt.Errorf("reading opened interactions for event fan-out: %s", err)
	}

	var closed []closedInteractionRow
	if err := sr.db.WithContext(ctx).Raw(`
		SELECT si.uuid AS uuid, sa.user_uuid AS user_uuid
		FROM session_interactions si
		JOIN session_actors sa ON sa.id = si.session_actor_id
		WHERE si.session_id = ? AND si.closed_by_turn_id = ?
	`, sessionID, turnID).Scan(&closed).Error; err != nil {
		return nil, fmt.Errorf("reading closed interactions for event fan-out: %s", err)
	}

	events := make([]play.Event, 0, len(opened)+len(closed))
	for _, row := range opened {
		question, arguments, err := decodeInteractionPayload(row.InteractionPayload)
		if err != nil {
			return nil, err
		}
		events = append(events, play.Event{
			Recipient:     play.UserUUID(row.UserUUID),
			Kind:          play.EventKindInteractionOpened,
			InteractionID: play.InteractionUUID(row.UUID),
			Question:      question,
			Arguments:     arguments,
		})
	}
	for _, row := range closed {
		events = append(events, play.Event{
			Recipient:     play.UserUUID(row.UserUUID),
			Kind:          play.EventKindInteractionClosed,
			InteractionID: play.InteractionUUID(row.UUID),
		})
	}
	return events, nil
}

// interactionPayloadWire is session_interactions.interaction_payload's
// durable JSON shape, redeclared here since the type that originally wrote
// it is unexported and unreachable from this package - this package reads
// it back purely through the stable persisted column shape instead.
type interactionPayloadWire struct {
	Question  string          `json:"question"`
	Arguments json.RawMessage `json:"arguments"`
}

// decodeInteractionPayload decodes session_interactions.interaction_payload
// into the Question text and a generic, JSON-shaped Arguments map -
// engineservice.EncodeValue's own RecordValue wrapping, translated field by
// field so play's exported Event never carries an engine.Value.
func decodeInteractionPayload(data []byte) (question string, arguments map[string]any, err error) {
	var wire interactionPayloadWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return "", nil, fmt.Errorf("decoding interaction payload: %s", err)
	}
	value, err := engineservice.DecodeValue(wire.Arguments)
	if err != nil {
		return "", nil, fmt.Errorf("decoding interaction arguments: %s", err)
	}
	record, ok := value.(engine.RecordValue)
	if !ok {
		return "", nil, fmt.Errorf("decoding interaction arguments: expected a record value, got %T", value)
	}
	fields := make(map[string]any, len(record.Fields))
	for _, f := range record.Fields {
		fields[f.Name] = valueToAny(f.Value)
	}
	return wire.Question, fields, nil
}

// valueToAny converts an engine.Value into a plain, JSON-marshalable Go
// value, so play's Event.Arguments never carries an engine.Value - the one
// place this translation needs to happen, since Arguments is the only
// piece of an opened interaction with engine-defined internal structure;
// every other field exposed to the client is already a plain string or
// identifier with no internal engine addressing information in it.
func valueToAny(v engine.Value) any {
	switch val := v.(type) {
	case engine.UnitValue:
		return nil
	case engine.BoolValue:
		return val.Value
	case engine.NumberValue:
		return val.Value
	case engine.StringValue:
		return val.Value
	case engine.UserValue:
		return string(val.ID)
	case engine.EnumValue:
		return val.ValueName
	case engine.RecordValue:
		fields := make(map[string]any, len(val.Fields))
		for _, f := range val.Fields {
			fields[f.Name] = valueToAny(f.Value)
		}
		return fields
	case engine.UnionValue:
		fields := make(map[string]any, len(val.Fields)+1)
		fields["variant"] = val.VariantName
		for _, f := range val.Fields {
			fields[f.Name] = valueToAny(f.Value)
		}
		return fields
	case engine.NewTypeValue:
		return valueToAny(val.Underlying)
	case engine.OptionalValue:
		if val.Value == nil {
			return nil
		}
		return valueToAny(val.Value)
	case engine.ListValue:
		elements := make([]any, len(val.Elements))
		for i, e := range val.Elements {
			elements[i] = valueToAny(e)
		}
		return elements
	case engine.MapValue:
		entries := make([]map[string]any, len(val.Entries))
		for i, e := range val.Entries {
			entries[i] = map[string]any{"key": valueToAny(e.Key), "value": valueToAny(e.Value)}
		}
		return entries
	default:
		return nil
	}
}
