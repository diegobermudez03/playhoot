package sessionlifecycle

import (
	"encoding/json"
	"testing"

	"github.com/diegobermudez03/playhoot/session"
	"github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/platform"
	"github.com/stretchr/testify/require"
)

func TestCollectOutboundEvents(t *testing.T) {
	t.Run("no_commands_returns_nil", func(t *testing.T) {
		require.Nil(t, collectOutboundEvents(1, nil))
	})

	t.Run("commands_with_no_send_event_returns_nil", func(t *testing.T) {
		commands := []platform.Command{
			&platform.ScheduleTimer{Timer: "t1", DelayMS: 1000},
			&platform.SessionComplete{},
		}
		require.Nil(t, collectOutboundEvents(1, commands))
	})

	t.Run("multiple_send_events_preserved_in_order_with_independent_recipients_ids_and_shared_revision", func(t *testing.T) {
		payload1 := json.RawMessage(`{"a":1}`)
		payload2 := json.RawMessage(`{"b":2}`)
		commands := []platform.Command{
			&platform.ScheduleTimer{Timer: "t1", DelayMS: 1000},
			&platform.SendEvent{Recipients: []platform.ActorRef{"1", "2"}, Name: "confetti", Payload: payload1},
			&platform.SendEvent{Recipients: []platform.ActorRef{"3"}, Name: "beep", Payload: payload2},
		}

		events := collectOutboundEvents(5, commands)

		require.Equal(t, []session.OutboundEvent{
			{ID: "5:1", Recipients: []session.ActorRef{"1", "2"}, Name: "confetti", Payload: payload1, Revision: 5},
			{ID: "5:2", Recipients: []session.ActorRef{"3"}, Name: "beep", Payload: payload2, Revision: 5},
		}, events)
		require.NotEqual(t, events[0].ID, events[1].ID, "two events from the same call must have distinct ids")
	})

	t.Run("same_input_produces_byte_identical_ids_across_repeated_calls", func(t *testing.T) {
		commands := []platform.Command{
			&platform.SendEvent{Recipients: []platform.ActorRef{"1"}, Name: "ping", Payload: json.RawMessage(`{}`)},
		}

		first := collectOutboundEvents(7, commands)
		second := collectOutboundEvents(7, commands)

		require.Equal(t, first, second, "the same (sequence, commands) input must deterministically produce the same id, not a fresh one per call")
	})
}
