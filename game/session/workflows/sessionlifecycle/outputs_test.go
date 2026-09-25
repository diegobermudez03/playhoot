package sessionlifecycle

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/stretchr/testify/require"
)

func TestClientFacingOutputs(t *testing.T) {
	t.Run("selects_only_effect_and_presentation_outputs_in_order", func(t *testing.T) {
		activate := engine.ActivatePresentationOutput{Slot: "hud", Recipient: "1", Name: "Hud", View: "HudView"}
		effect := engine.EmitEffectOutput{Effect: "Celebrate", Recipients: []engine.UserID{"1", "2"}}
		update := engine.UpdatePresentationOutput{Slot: "hud", Recipient: "2", Name: "Hud"}
		remove := engine.RemovePresentationOutput{Slot: "hud", Recipient: "1", Name: "Hud"}
		outputs := []engine.Output{
			engine.OpenQuestionOutput{InteractionID: 1, Recipient: "1"},
			activate,
			effect,
			engine.CloseQuestionOutput{InteractionID: 2, Recipient: "1"},
			update,
			remove,
		}

		got := clientFacingOutputs(outputs)

		require.Equal(t, []engine.Output{activate, effect, update, remove}, got)
	})

	t.Run("returns_nil_when_no_client_facing_output_is_present", func(t *testing.T) {
		outputs := []engine.Output{engine.OpenQuestionOutput{InteractionID: 1, Recipient: "1"}}

		require.Nil(t, clientFacingOutputs(outputs))
	})

	t.Run("returns_nil_for_no_outputs", func(t *testing.T) {
		require.Nil(t, clientFacingOutputs(nil))
	})
}
