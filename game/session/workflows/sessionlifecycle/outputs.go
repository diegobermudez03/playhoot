package sessionlifecycle

import "github.com/diegobermudez03/playhoot/game/language/v1/engine"

// clientFacingOutputs returns the subset of a committed RuntimeTurn's
// Outputs that a live delivery mechanism eventually fans out to connected
// clients - every EmitEffectOutput and every ActivatePresentationOutput/
// UpdatePresentationOutput/RemovePresentationOutput - in the same relative
// order engine execution produced them. Every other Output kind is either
// already durably captured elsewhere (a pending question, via
// captureInteractions) or not yet this package's concern (workflow
// completion, timers).
func clientFacingOutputs(outputs []engine.Output) []engine.Output {
	var result []engine.Output
	for _, output := range outputs {
		switch output.(type) {
		case engine.EmitEffectOutput,
			engine.ActivatePresentationOutput,
			engine.UpdatePresentationOutput,
			engine.RemovePresentationOutput:
			result = append(result, output)
		}
	}
	return result
}
