package completion

import (
	"testing"

	"github.com/diegobermudez03/playhoot/session"
	"github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/platform"
	"github.com/stretchr/testify/require"
)

func TestDetect(t *testing.T) {
	t.Run("completed", func(t *testing.T) {
		commands := []platform.Command{
			&platform.SendEvent{Recipients: []platform.ActorRef{"1"}, Name: "celebrate"},
			&platform.SessionComplete{},
		}
		reason, terminated := Detect(commands)
		require.True(t, terminated)
		require.Equal(t, session.TerminalReasonGameCompleted, reason)
	})

	t.Run("failed", func(t *testing.T) {
		commands := []platform.Command{
			&platform.SessionFail{Reason: "oops"},
		}
		reason, terminated := Detect(commands)
		require.True(t, terminated)
		require.Equal(t, session.TerminalReasonGameFailed, reason)
	})

	t.Run("no_terminal_command_is_not_terminated", func(t *testing.T) {
		commands := []platform.Command{
			&platform.SendEvent{Recipients: []platform.ActorRef{"1"}, Name: "celebrate"},
			&platform.ScheduleTimer{Timer: "t1", DelayMS: 1000},
		}
		reason, terminated := Detect(commands)
		require.False(t, terminated)
		require.Empty(t, reason)
	})

	t.Run("no_commands_is_not_terminated", func(t *testing.T) {
		reason, terminated := Detect(nil)
		require.False(t, terminated)
		require.Empty(t, reason)
	})
}
