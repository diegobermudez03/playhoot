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
		require.Nil(t, collectOutboundEvents(nil))
	})

	t.Run("commands_with_no_send_event_returns_nil", func(t *testing.T) {
		commands := []platform.Command{
			&platform.ScheduleTimer{Timer: "t1", DelayMS: 1000},
			&platform.SessionComplete{},
		}
		require.Nil(t, collectOutboundEvents(commands))
	})

	t.Run("multiple_send_events_preserved_in_order_with_independent_recipients", func(t *testing.T) {
		payload1 := json.RawMessage(`{"a":1}`)
		payload2 := json.RawMessage(`{"b":2}`)
		commands := []platform.Command{
			&platform.ScheduleTimer{Timer: "t1", DelayMS: 1000},
			&platform.SendEvent{Recipients: []platform.ActorRef{"1", "2"}, Name: "confetti", Payload: payload1},
			&platform.SendEvent{Recipients: []platform.ActorRef{"3"}, Name: "beep", Payload: payload2},
		}

		events := collectOutboundEvents(commands)

		require.Equal(t, []session.OutboundEvent{
			{Recipients: []session.ActorRef{"1", "2"}, Name: "confetti", Payload: payload1},
			{Recipients: []session.ActorRef{"3"}, Name: "beep", Payload: payload2},
		}, events)
	})
}
