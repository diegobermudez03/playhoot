package platform

import (
	"encoding/json"
	"fmt"
)

// Platform Command kinds.
const (
	CommandKindSendEvent       = "SEND_EVENT"
	CommandKindScheduleTimer   = "SCHEDULE_TIMER"
	CommandKindCancelTimer     = "CANCEL_TIMER"
	CommandKindSessionComplete = "SESSION_COMPLETE"
	CommandKindSessionFail     = "SESSION_FAIL"
)

// maxPayloadBytes bounds a single game-defined payload/data field. A
// provisional platform-wide limit until resource limits are designed on
// their own terms; kept here rather than left unenforced so a script
// cannot make an unbounded claim in the meantime.
const maxPayloadBytes = 64 * 1024

// Command is one closed, Playhoot-owned effect an execute call requested.
// Only this package defines Command kinds; authored code can only request
// from this closed set, never invent a new one.
type Command interface {
	commandKind() string
}

// SendEvent asks Playhoot to deliver a transient, game-defined occurrence to
// one or more participants, best-effort. Name and Payload are entirely
// game-defined and opaque to Playhoot; only Recipients is validated.
type SendEvent struct {
	Kind       string          `json:"kind"`
	Recipients []ActorRef      `json:"recipients"`
	Name       string          `json:"name"`
	Payload    json.RawMessage `json:"payload"`
}

func (*SendEvent) commandKind() string { return CommandKindSendEvent }

// ScheduleTimer asks Playhoot to durably schedule Timer to fire after
// DelayMS, echoing Data back on TimerExpired unchanged. Timer is an opaque
// identifier the script chooses and later uses to CancelTimer or recognize
// which TimerExpired fired.
type ScheduleTimer struct {
	Kind    string          `json:"kind"`
	Timer   string          `json:"timer"`
	DelayMS int64           `json:"delay_ms"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (*ScheduleTimer) commandKind() string { return CommandKindScheduleTimer }

// CancelTimer asks Playhoot to cancel a previously scheduled, not-yet-fired
// timer identified by Timer. Cancelling a timer that already fired or was
// never scheduled is not an error.
type CancelTimer struct {
	Kind  string `json:"kind"`
	Timer string `json:"timer"`
}

func (*CancelTimer) commandKind() string { return CommandKindCancelTimer }

// SessionComplete asks Playhoot to transition the Session to its terminal,
// successfully-completed state.
type SessionComplete struct {
	Kind string `json:"kind"`
}

func (*SessionComplete) commandKind() string { return CommandKindSessionComplete }

// SessionFail asks Playhoot to transition the Session to its terminal,
// failed state, recording Reason as the authored explanation.
type SessionFail struct {
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
}

func (*SessionFail) commandKind() string { return CommandKindSessionFail }

// ValidationError indicates a Command failed platform-level validation: an
// unrecognized kind, a missing/malformed required field, an oversized
// payload, or a reference to an actor identifier the caller never supplied.
// Session Runtime treats it exactly like a rejected script - an
// authored-script defect, never an infrastructure failure.
type ValidationError struct {
	Reason string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("platform: command validation failed: %s", e.Reason)
}

// KnownActors is the set of actor identifiers a caller already established
// before parsing an execution's output. ParseCommand rejects any recipient/
// actor reference outside this set, rather than trusting a command's own
// claim of who it addresses.
type KnownActors map[ActorRef]struct{}

// NewKnownActors builds a KnownActors set from actors.
func NewKnownActors(actors ...ActorRef) KnownActors {
	m := make(KnownActors, len(actors))
	for _, a := range actors {
		m[a] = struct{}{}
	}
	return m
}

func (k KnownActors) contains(a ActorRef) bool {
	_, ok := k[a]
	return ok
}

type commandDecoder func(raw json.RawMessage, known KnownActors) (Command, error)

// commandDecoders is the platform's own closed registry: one entry per
// supported Command kind. Adding a new kind means adding its type plus a
// registerCommand call for it in its own file - this map and ParseCommand
// never change to support it.
var commandDecoders = map[string]commandDecoder{}

func registerCommand(kind string, decode commandDecoder) {
	commandDecoders[kind] = decode
}

func init() {
	registerCommand(CommandKindSendEvent, decodeSendEvent)
	registerCommand(CommandKindScheduleTimer, decodeScheduleTimer)
	registerCommand(CommandKindCancelTimer, decodeCancelTimer)
	registerCommand(CommandKindSessionComplete, decodeSessionComplete)
	registerCommand(CommandKindSessionFail, decodeSessionFail)
}

// ParseCommand validates and decodes one element of ExecutionOutput.
// RequestedCommands. known is the set of actor identifiers this call is
// authorized to address; a Command naming any other actor is rejected.
func ParseCommand(raw json.RawMessage, known KnownActors) (Command, error) {
	var envelope struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, &ValidationError{Reason: "command is not a valid JSON object"}
	}
	decode, ok := commandDecoders[envelope.Kind]
	if !ok {
		return nil, &ValidationError{Reason: fmt.Sprintf("unrecognized command kind %q", envelope.Kind)}
	}
	return decode(raw, known)
}

func decodeSendEvent(raw json.RawMessage, known KnownActors) (Command, error) {
	var c SendEvent
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, &ValidationError{Reason: "SEND_EVENT is malformed"}
	}
	if len(c.Recipients) == 0 {
		return nil, &ValidationError{Reason: "SEND_EVENT requires at least one recipient"}
	}
	for _, r := range c.Recipients {
		if !known.contains(r) {
			return nil, &ValidationError{Reason: fmt.Sprintf("SEND_EVENT recipient %q is not a known actor", r)}
		}
	}
	if c.Name == "" {
		return nil, &ValidationError{Reason: "SEND_EVENT requires a name"}
	}
	if len(c.Payload) > maxPayloadBytes {
		return nil, &ValidationError{Reason: "SEND_EVENT payload exceeds the platform size limit"}
	}
	return &c, nil
}

func decodeScheduleTimer(raw json.RawMessage, _ KnownActors) (Command, error) {
	var c ScheduleTimer
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, &ValidationError{Reason: "SCHEDULE_TIMER is malformed"}
	}
	if c.Timer == "" {
		return nil, &ValidationError{Reason: "SCHEDULE_TIMER requires a timer identifier"}
	}
	if c.DelayMS < 0 {
		return nil, &ValidationError{Reason: "SCHEDULE_TIMER delay_ms must not be negative"}
	}
	if len(c.Data) > maxPayloadBytes {
		return nil, &ValidationError{Reason: "SCHEDULE_TIMER data exceeds the platform size limit"}
	}
	return &c, nil
}

func decodeCancelTimer(raw json.RawMessage, _ KnownActors) (Command, error) {
	var c CancelTimer
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, &ValidationError{Reason: "CANCEL_TIMER is malformed"}
	}
	if c.Timer == "" {
		return nil, &ValidationError{Reason: "CANCEL_TIMER requires a timer identifier"}
	}
	return &c, nil
}

func decodeSessionComplete(raw json.RawMessage, _ KnownActors) (Command, error) {
	var c SessionComplete
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, &ValidationError{Reason: "SESSION_COMPLETE is malformed"}
	}
	return &c, nil
}

func decodeSessionFail(raw json.RawMessage, _ KnownActors) (Command, error) {
	var c SessionFail
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, &ValidationError{Reason: "SESSION_FAIL is malformed"}
	}
	if c.Reason == "" {
		return nil, &ValidationError{Reason: "SESSION_FAIL requires a reason"}
	}
	return &c, nil
}
