package completion

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/stretchr/testify/require"
)

func TestDetect(t *testing.T) {
	t.Run("completed", func(t *testing.T) {
		outputs := []engine.Output{
			engine.OpenQuestionOutput{InteractionID: 1, Recipient: "1"},
			engine.RunCompletedOutput{Outcome: engine.RunOutcome{Kind: engine.RunOutcomeCompleted, Result: engine.NumberValue{Value: 1}}},
		}
		reason, terminated := Detect(outputs)
		require.True(t, terminated)
		require.Equal(t, session.TerminalReasonGameCompleted, reason)
	})

	t.Run("failed", func(t *testing.T) {
		outputs := []engine.Output{
			engine.RunCompletedOutput{Outcome: engine.RunOutcome{Kind: engine.RunOutcomeFailed, Error: "oops"}},
		}
		reason, terminated := Detect(outputs)
		require.True(t, terminated)
		require.Equal(t, session.TerminalReasonGameFailed, reason)
	})

	t.Run("cancelled", func(t *testing.T) {
		outputs := []engine.Output{
			engine.RunCompletedOutput{Outcome: engine.RunOutcome{Kind: engine.RunOutcomeCancelled, Reason: "abandoned"}},
		}
		reason, terminated := Detect(outputs)
		require.True(t, terminated)
		require.Equal(t, session.TerminalReasonGameCancelled, reason)
	})

	t.Run("no_run_completed_output_is_not_terminated", func(t *testing.T) {
		outputs := []engine.Output{
			engine.OpenQuestionOutput{InteractionID: 1, Recipient: "1"},
			engine.EmitEffectOutput{Effect: "Celebrate"},
		}
		reason, terminated := Detect(outputs)
		require.False(t, terminated)
		require.Empty(t, reason)
	})

	t.Run("no_outputs_is_not_terminated", func(t *testing.T) {
		reason, terminated := Detect(nil)
		require.False(t, terminated)
		require.Empty(t, reason)
	})
}
