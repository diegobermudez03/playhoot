// Package clientoutputs selects the subset of a committed RuntimeTurn's
// engine Outputs a live delivery mechanism eventually fans out to
// connected clients, shared by sessionlifecycle's Start/AnswerInteraction
// steps.
package clientoutputs

import "github.com/diegobermudez03/playhoot/game/language/v1/engine"

// ClientFacing returns the subset of outputs a live delivery mechanism
// eventually fans out to connected clients - every EmitEffectOutput and
// every ActivatePresentationOutput/UpdatePresentationOutput/
// RemovePresentationOutput - in the same relative order engine execution
// produced them. Every other Output kind is either already durably
// captured elsewhere (a pending question, via the interactions package) or
// not yet this workflow's concern (workflow completion, timers).
func ClientFacing(outputs []engine.Output) []engine.Output {
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
