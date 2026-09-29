// Package platform defines the closed, Playhoot-owned vocabulary that
// crosses the sandboxed JavaScript execution boundary: the platform Events
// that drive an execute call and the platform Commands its output may
// request. Every kind is presentation-agnostic - no member of this
// vocabulary describes a screen, a visual effect, a question, or any other
// rendering concept. A game's own action/data names and payloads are opaque
// to this package; only the envelope around them is validated here.
package platform

import (
	"encoding/json"
	"fmt"
)

// Platform Event kinds.
const (
	EventKindSessionStarted          = "SESSION_STARTED"
	EventKindPlayerEvent             = "PLAYER_EVENT"
	EventKindParticipantDisconnected = "PARTICIPANT_DISCONNECTED"
	EventKindParticipantReconnected  = "PARTICIPANT_RECONNECTED"
	EventKindParticipantLeft         = "PARTICIPANT_LEFT"
	EventKindTimerExpired            = "TIMER_EXPIRED"
	EventKindSessionCancelled        = "SESSION_CANCELLED"
)

// ActorRef identifies a Session participant by the identifier Playhoot
// itself assigned. Authored code never invents or reinterprets one; it only
// ever echoes back an ActorRef a platform Event already supplied.
type ActorRef string

// Event is one closed, Playhoot-certified cause driving an execute call.
// Only this package defines Event kinds; authored code never constructs
// one.
type Event interface {
	eventKind() string
}

// EncodeEvent produces the wire JSON to send as the Executor's
// ExecutionInput.Event.
func EncodeEvent(e Event) (json.RawMessage, error) {
	return json.Marshal(e)
}

type eventDecoder func(raw json.RawMessage, known KnownActors) (Event, error)

// eventDecoders is the platform's own closed registry for decoding a
// previously encoded Event back into its typed value - symmetric to
// commandDecoders. Adding a new kind means adding its type plus a
// registerEvent call for it, never editing this map or ParseEvent.
var eventDecoders = map[string]eventDecoder{}

func registerEvent(kind string, decode eventDecoder) {
	eventDecoders[kind] = decode
}

func init() {
	registerEvent(EventKindSessionStarted, decodeSessionStarted)
	registerEvent(EventKindPlayerEvent, decodePlayerEvent)
	registerEvent(EventKindParticipantDisconnected, decodeParticipantDisconnected)
	registerEvent(EventKindParticipantReconnected, decodeParticipantReconnected)
	registerEvent(EventKindParticipantLeft, decodeParticipantLeft)
	registerEvent(EventKindTimerExpired, decodeTimerExpired)
	registerEvent(EventKindSessionCancelled, decodeSessionCancelled)
}

// ParseEvent validates and decodes a previously encoded Event back into its
// typed value. known is the set of actor identifiers this call recognizes;
// an Event naming any other actor is rejected.
func ParseEvent(raw json.RawMessage, known KnownActors) (Event, error) {
	var envelope struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, &ValidationError{Reason: "event is not a valid JSON object"}
	}
	decode, ok := eventDecoders[envelope.Kind]
	if !ok {
		return nil, &ValidationError{Reason: fmt.Sprintf("unrecognized event kind %q", envelope.Kind)}
	}
	return decode(raw, known)
}

func decodeSessionStarted(raw json.RawMessage, known KnownActors) (Event, error) {
	var e SessionStarted
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, &ValidationError{Reason: "SESSION_STARTED is malformed"}
	}
	if len(e.RootParameters) > maxPayloadBytes {
		return nil, &ValidationError{Reason: "SESSION_STARTED root_parameters exceeds the platform size limit"}
	}
	for _, p := range e.Players {
		if !known.contains(p) {
			return nil, &ValidationError{Reason: fmt.Sprintf("SESSION_STARTED player %q is not a known actor", p)}
		}
	}
	return &e, nil
}

func decodePlayerEvent(raw json.RawMessage, known KnownActors) (Event, error) {
	var e PlayerEvent
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, &ValidationError{Reason: "PLAYER_EVENT is malformed"}
	}
	if !known.contains(e.Actor) {
		return nil, &ValidationError{Reason: fmt.Sprintf("PLAYER_EVENT actor %q is not a known actor", e.Actor)}
	}
	if e.Name == "" {
		return nil, &ValidationError{Reason: "PLAYER_EVENT requires a name"}
	}
	if len(e.Payload) > maxPayloadBytes {
		return nil, &ValidationError{Reason: "PLAYER_EVENT payload exceeds the platform size limit"}
	}
	return &e, nil
}

func decodeParticipantDisconnected(raw json.RawMessage, known KnownActors) (Event, error) {
	var e ParticipantDisconnected
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, &ValidationError{Reason: "PARTICIPANT_DISCONNECTED is malformed"}
	}
	if !known.contains(e.Participant) {
		return nil, &ValidationError{Reason: fmt.Sprintf("PARTICIPANT_DISCONNECTED participant %q is not a known actor", e.Participant)}
	}
	return &e, nil
}

func decodeParticipantReconnected(raw json.RawMessage, known KnownActors) (Event, error) {
	var e ParticipantReconnected
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, &ValidationError{Reason: "PARTICIPANT_RECONNECTED is malformed"}
	}
	if !known.contains(e.Participant) {
		return nil, &ValidationError{Reason: fmt.Sprintf("PARTICIPANT_RECONNECTED participant %q is not a known actor", e.Participant)}
	}
	return &e, nil
}

func decodeParticipantLeft(raw json.RawMessage, known KnownActors) (Event, error) {
	var e ParticipantLeft
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, &ValidationError{Reason: "PARTICIPANT_LEFT is malformed"}
	}
	if !known.contains(e.Participant) {
		return nil, &ValidationError{Reason: fmt.Sprintf("PARTICIPANT_LEFT participant %q is not a known actor", e.Participant)}
	}
	return &e, nil
}

func decodeTimerExpired(raw json.RawMessage, _ KnownActors) (Event, error) {
	var e TimerExpired
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, &ValidationError{Reason: "TIMER_EXPIRED is malformed"}
	}
	if e.Timer == "" {
		return nil, &ValidationError{Reason: "TIMER_EXPIRED requires a timer identifier"}
	}
	if len(e.Data) > maxPayloadBytes {
		return nil, &ValidationError{Reason: "TIMER_EXPIRED data exceeds the platform size limit"}
	}
	return &e, nil
}

func decodeSessionCancelled(raw json.RawMessage, _ KnownActors) (Event, error) {
	var e SessionCancelled
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, &ValidationError{Reason: "SESSION_CANCELLED is malformed"}
	}
	return &e, nil
}

// SessionStarted is the platform fact that begins a Session's Runtime
// lifecycle: the authored script's very first invocation, with no prior
// state.
type SessionStarted struct {
	Kind           string          `json:"kind"`
	RootParameters json.RawMessage `json:"root_parameters"`
	Players        []ActorRef      `json:"players"`
}

// NewSessionStarted builds the platform's own initial Event for a Session,
// carrying the root parameters and player roster established at Start.
func NewSessionStarted(rootParameters json.RawMessage, players []ActorRef) *SessionStarted {
	return &SessionStarted{Kind: EventKindSessionStarted, RootParameters: rootParameters, Players: players}
}

func (*SessionStarted) eventKind() string { return EventKindSessionStarted }

// PlayerEvent is the only generic game-defined action a player may
// initiate. Name and Payload are entirely game-defined and opaque to
// Playhoot; Actor is Playhoot-established and never a client's own claim.
type PlayerEvent struct {
	Kind    string          `json:"kind"`
	Actor   ActorRef        `json:"actor"`
	Name    string          `json:"name"`
	Payload json.RawMessage `json:"payload"`
}

// NewPlayerEvent builds a game-defined action attributed to actor, whose
// identity is already established by the caller, never by the action's own
// content.
func NewPlayerEvent(actor ActorRef, name string, payload json.RawMessage) *PlayerEvent {
	return &PlayerEvent{Kind: EventKindPlayerEvent, Actor: actor, Name: name, Payload: payload}
}

func (*PlayerEvent) eventKind() string { return EventKindPlayerEvent }

// ParticipantDisconnected is the platform-certified fact that participant
// lost its live connection. It is never modeled as a PlayerEvent: a
// disconnect is something that happened to a participant, not an action a
// participant requested.
type ParticipantDisconnected struct {
	Kind        string   `json:"kind"`
	Participant ActorRef `json:"participant"`
}

// NewParticipantDisconnected builds the disconnect fact for participant.
func NewParticipantDisconnected(participant ActorRef) *ParticipantDisconnected {
	return &ParticipantDisconnected{Kind: EventKindParticipantDisconnected, Participant: participant}
}

func (*ParticipantDisconnected) eventKind() string { return EventKindParticipantDisconnected }

// ParticipantReconnected is the platform-certified fact that participant
// re-established its live connection.
type ParticipantReconnected struct {
	Kind        string   `json:"kind"`
	Participant ActorRef `json:"participant"`
}

// NewParticipantReconnected builds the reconnect fact for participant.
func NewParticipantReconnected(participant ActorRef) *ParticipantReconnected {
	return &ParticipantReconnected{Kind: EventKindParticipantReconnected, Participant: participant}
}

func (*ParticipantReconnected) eventKind() string { return EventKindParticipantReconnected }

// ParticipantLeft is the platform-certified fact that participant
// deliberately left the Session, distinct from a disconnect it may never
// recover from.
type ParticipantLeft struct {
	Kind        string   `json:"kind"`
	Participant ActorRef `json:"participant"`
}

// NewParticipantLeft builds the leave fact for participant.
func NewParticipantLeft(participant ActorRef) *ParticipantLeft {
	return &ParticipantLeft{Kind: EventKindParticipantLeft, Participant: participant}
}

func (*ParticipantLeft) eventKind() string { return EventKindParticipantLeft }

// TimerExpired is the platform-certified fact that a previously scheduled
// timer fired. Timer is the same opaque identifier the script chose when it
// requested ScheduleTimer; Data carries back whatever opaque payload the
// script attached to that request, if any.
type TimerExpired struct {
	Kind  string          `json:"kind"`
	Timer string          `json:"timer"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// NewTimerExpired builds the expiration fact for timer, echoing back
// data exactly as scheduled.
func NewTimerExpired(timer string, data json.RawMessage) *TimerExpired {
	return &TimerExpired{Kind: EventKindTimerExpired, Timer: timer, Data: data}
}

func (*TimerExpired) eventKind() string { return EventKindTimerExpired }

// SessionCancelled is the platform-certified fact that the Session's host
// cancelled it. The authored script may still accept or reject it like any
// other Event; the Session terminalizes either way, since the platform's
// own cancellation invariant does not depend on the game reacting to it.
type SessionCancelled struct {
	Kind string `json:"kind"`
}

// NewSessionCancelled builds the cancellation fact.
func NewSessionCancelled() *SessionCancelled {
	return &SessionCancelled{Kind: EventKindSessionCancelled}
}

func (*SessionCancelled) eventKind() string { return EventKindSessionCancelled }
