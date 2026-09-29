package platform

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/diegobermudez03/playhoot/session/internal/executor"
	"github.com/stretchr/testify/require"
)

func TestEncodeEvent(t *testing.T) {
	tests := map[string]struct {
		event Event
		kind  string
	}{
		"session_started": {
			event: NewSessionStarted(json.RawMessage(`{"seed":1}`), []ActorRef{"actor-1", "actor-2"}),
			kind:  EventKindSessionStarted,
		},
		"player_event": {
			event: NewPlayerEvent("actor-1", "play_card", json.RawMessage(`{"cardId":"7-hearts"}`)),
			kind:  EventKindPlayerEvent,
		},
		"participant_disconnected": {
			event: NewParticipantDisconnected("actor-1"),
			kind:  EventKindParticipantDisconnected,
		},
		"participant_reconnected": {
			event: NewParticipantReconnected("actor-1"),
			kind:  EventKindParticipantReconnected,
		},
		"participant_left": {
			event: NewParticipantLeft("actor-1"),
			kind:  EventKindParticipantLeft,
		},
		"timer_expired": {
			event: NewTimerExpired("round-timer", json.RawMessage(`{"round":2}`)),
			kind:  EventKindTimerExpired,
		},
		"session_cancelled": {
			event: NewSessionCancelled(),
			kind:  EventKindSessionCancelled,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			raw, err := EncodeEvent(tc.event)
			require.NoError(t, err)

			var envelope struct {
				Kind string `json:"kind"`
			}
			require.NoError(t, json.Unmarshal(raw, &envelope))
			require.Equal(t, tc.kind, envelope.Kind)
		})
	}
}

func TestParseCommand_Accepts(t *testing.T) {
	known := NewKnownActors("actor-1", "actor-2")

	tests := map[string]struct {
		raw  string
		kind string
	}{
		"send_event": {
			raw:  `{"kind":"SEND_EVENT","recipients":["actor-1"],"name":"card_played","payload":{"cardId":"7-hearts"}}`,
			kind: CommandKindSendEvent,
		},
		"schedule_timer": {
			raw:  `{"kind":"SCHEDULE_TIMER","timer":"round-timer","delay_ms":30000}`,
			kind: CommandKindScheduleTimer,
		},
		"cancel_timer": {
			raw:  `{"kind":"CANCEL_TIMER","timer":"round-timer"}`,
			kind: CommandKindCancelTimer,
		},
		"session_complete": {
			raw:  `{"kind":"SESSION_COMPLETE"}`,
			kind: CommandKindSessionComplete,
		},
		"session_fail": {
			raw:  `{"kind":"SESSION_FAIL","reason":"invariant violated"}`,
			kind: CommandKindSessionFail,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cmd, err := ParseCommand(json.RawMessage(tc.raw), known)
			require.NoError(t, err)
			require.Equal(t, tc.kind, cmd.commandKind())
		})
	}
}

func TestParseCommand_Rejects(t *testing.T) {
	known := NewKnownActors("actor-1")

	tests := map[string]string{
		"unknown_kind":         `{"kind":"OPEN_INTERACTION"}`,
		"not_json":             `not json at all`,
		"missing_recipients":   `{"kind":"SEND_EVENT","recipients":[],"name":"card_played"}`,
		"unknown_recipient":    `{"kind":"SEND_EVENT","recipients":["actor-99"],"name":"card_played"}`,
		"missing_send_name":    `{"kind":"SEND_EVENT","recipients":["actor-1"],"name":""}`,
		"missing_timer":        `{"kind":"SCHEDULE_TIMER","timer":"","delay_ms":100}`,
		"negative_delay":       `{"kind":"SCHEDULE_TIMER","timer":"t","delay_ms":-1}`,
		"missing_cancel_timer": `{"kind":"CANCEL_TIMER","timer":""}`,
		"missing_fail_reason":  `{"kind":"SESSION_FAIL","reason":""}`,
	}

	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := ParseCommand(json.RawMessage(raw), known)
			require.Error(t, err)
			require.IsType(t, &ValidationError{}, err)
		})
	}
}

func TestParseCommand_RejectsOversizedPayload(t *testing.T) {
	known := NewKnownActors("actor-1")
	oversized, err := json.Marshal(string(make([]byte, maxPayloadBytes+1)))
	require.NoError(t, err)

	raw := json.RawMessage(`{"kind":"SEND_EVENT","recipients":["actor-1"],"name":"x","payload":` + string(oversized) + `}`)
	_, err = ParseCommand(raw, known)
	require.Error(t, err)
	require.IsType(t, &ValidationError{}, err)
}

// throwawayCommand exercises the registry-extension property: adding a new
// platform Command kind touches only its own registration, never
// ParseCommand or the shared registry map.
type throwawayCommand struct {
	Kind string `json:"kind"`
}

func (*throwawayCommand) commandKind() string { return "THROWAWAY_FOR_TEST" }

func init() {
	registerCommand("THROWAWAY_FOR_TEST", func(raw json.RawMessage, _ KnownActors) (Command, error) {
		var c throwawayCommand
		if err := json.Unmarshal(raw, &c); err != nil {
			return nil, &ValidationError{Reason: "malformed"}
		}
		return &c, nil
	})
}

func TestParseCommand_RegistryExtension(t *testing.T) {
	cmd, err := ParseCommand(json.RawMessage(`{"kind":"THROWAWAY_FOR_TEST"}`), nil)
	require.NoError(t, err)
	require.Equal(t, "THROWAWAY_FOR_TEST", cmd.commandKind())
}

func TestParseEvent_Accepts(t *testing.T) {
	known := NewKnownActors("actor-1", "actor-2")

	tests := map[string]struct {
		raw  string
		kind string
	}{
		"session_started": {
			raw:  `{"kind":"SESSION_STARTED","root_parameters":{"seed":1},"players":["actor-1","actor-2"]}`,
			kind: EventKindSessionStarted,
		},
		"player_event": {
			raw:  `{"kind":"PLAYER_EVENT","actor":"actor-1","name":"play_card","payload":{"cardId":"7-hearts"}}`,
			kind: EventKindPlayerEvent,
		},
		"participant_disconnected": {
			raw:  `{"kind":"PARTICIPANT_DISCONNECTED","participant":"actor-1"}`,
			kind: EventKindParticipantDisconnected,
		},
		"participant_reconnected": {
			raw:  `{"kind":"PARTICIPANT_RECONNECTED","participant":"actor-1"}`,
			kind: EventKindParticipantReconnected,
		},
		"participant_left": {
			raw:  `{"kind":"PARTICIPANT_LEFT","participant":"actor-1"}`,
			kind: EventKindParticipantLeft,
		},
		"timer_expired": {
			raw:  `{"kind":"TIMER_EXPIRED","timer":"round-timer","data":{"round":2}}`,
			kind: EventKindTimerExpired,
		},
		"session_cancelled": {
			raw:  `{"kind":"SESSION_CANCELLED"}`,
			kind: EventKindSessionCancelled,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			event, err := ParseEvent(json.RawMessage(tc.raw), known)
			require.NoError(t, err)
			require.Equal(t, tc.kind, event.eventKind())
		})
	}
}

func TestParseEvent_Rejects(t *testing.T) {
	known := NewKnownActors("actor-1")

	tests := map[string]string{
		"unknown_kind":            `{"kind":"WORKFLOW_STARTED"}`,
		"not_json":                `not json at all`,
		"unknown_player_actor":    `{"kind":"PLAYER_EVENT","actor":"actor-99","name":"play_card"}`,
		"missing_player_name":     `{"kind":"PLAYER_EVENT","actor":"actor-1","name":""}`,
		"unknown_disconnect":      `{"kind":"PARTICIPANT_DISCONNECTED","participant":"actor-99"}`,
		"unknown_reconnect":       `{"kind":"PARTICIPANT_RECONNECTED","participant":"actor-99"}`,
		"unknown_left":            `{"kind":"PARTICIPANT_LEFT","participant":"actor-99"}`,
		"unknown_session_started": `{"kind":"SESSION_STARTED","players":["actor-99"]}`,
		"missing_timer":           `{"kind":"TIMER_EXPIRED","timer":""}`,
	}

	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := ParseEvent(json.RawMessage(raw), known)
			require.Error(t, err)
			require.IsType(t, &ValidationError{}, err)
		})
	}
}

func TestParseEvent_RejectsOversizedPayload(t *testing.T) {
	known := NewKnownActors("actor-1")
	oversized, err := json.Marshal(string(make([]byte, maxPayloadBytes+1)))
	require.NoError(t, err)

	raw := json.RawMessage(`{"kind":"PLAYER_EVENT","actor":"actor-1","name":"x","payload":` + string(oversized) + `}`)
	_, err = ParseEvent(raw, known)
	require.Error(t, err)
	require.IsType(t, &ValidationError{}, err)

	oversizedRoot := json.RawMessage(`{"kind":"SESSION_STARTED","root_parameters":` + string(oversized) + `,"players":[]}`)
	_, err = ParseEvent(oversizedRoot, known)
	require.Error(t, err)
	require.IsType(t, &ValidationError{}, err)
}

// throwawayEvent exercises the registry-extension property on the Event
// side, symmetric to TestParseCommand_RegistryExtension.
type throwawayEvent struct {
	Kind string `json:"kind"`
}

func (*throwawayEvent) eventKind() string { return "THROWAWAY_EVENT_FOR_TEST" }

func init() {
	registerEvent("THROWAWAY_EVENT_FOR_TEST", func(raw json.RawMessage, _ KnownActors) (Event, error) {
		var e throwawayEvent
		if err := json.Unmarshal(raw, &e); err != nil {
			return nil, &ValidationError{Reason: "malformed"}
		}
		return &e, nil
	})
}

func TestParseEvent_RegistryExtension(t *testing.T) {
	event, err := ParseEvent(json.RawMessage(`{"kind":"THROWAWAY_EVENT_FOR_TEST"}`), nil)
	require.NoError(t, err)
	require.Equal(t, "THROWAWAY_EVENT_FOR_TEST", event.eventKind())
}

// TestExecuteRoundTrip exercises the full round trip from an encoded Event
// through Command validation - build, encode, feed through a Fake
// Executor, parse every returned Command - without a real Executor
// process.
func TestExecuteRoundTrip(t *testing.T) {
	known := NewKnownActors("actor-1")

	events := []Event{
		NewSessionStarted(json.RawMessage(`{"seed":1}`), []ActorRef{"actor-1"}),
		NewPlayerEvent("actor-1", "play_card", json.RawMessage(`{"cardId":"7-hearts"}`)),
		NewParticipantDisconnected("actor-1"),
		NewParticipantReconnected("actor-1"),
		NewParticipantLeft("actor-1"),
		NewTimerExpired("round-timer", json.RawMessage(`{"round":2}`)),
		NewSessionCancelled(),
	}

	fabricatedCommands := []string{
		`{"kind":"SEND_EVENT","recipients":["actor-1"],"name":"card_played","payload":{"cardId":"7-hearts"}}`,
		`{"kind":"SCHEDULE_TIMER","timer":"round-timer","delay_ms":30000}`,
		`{"kind":"CANCEL_TIMER","timer":"round-timer"}`,
		`{"kind":"SESSION_COMPLETE"}`,
		`{"kind":"SESSION_FAIL","reason":"invariant violated"}`,
	}

	for _, event := range events {
		encoded, err := EncodeEvent(event)
		require.NoError(t, err)

		fake := &executor.Fake{
			ExecuteFunc: func(_ context.Context, in executor.ExecutionInput) (executor.ExecutionOutput, error) {
				require.Equal(t, json.RawMessage(encoded), in.Event)
				raw := make([]json.RawMessage, len(fabricatedCommands))
				for i, c := range fabricatedCommands {
					raw[i] = json.RawMessage(c)
				}
				return executor.ExecutionOutput{NewState: json.RawMessage(`{}`), RequestedCommands: raw}, nil
			},
		}

		out, err := fake.Execute(context.Background(), executor.ExecutionInput{
			PreviousState: json.RawMessage(`{}`),
			Event:         encoded,
		})
		require.NoError(t, err)

		for _, raw := range out.RequestedCommands {
			_, err := ParseCommand(raw, known)
			require.NoError(t, err)
		}
	}
}
