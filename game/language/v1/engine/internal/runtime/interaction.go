package runtime

import (
	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
)

// resolvedInteraction is the result of resolving an engine.InteractionID
// to the underlying occurrence it addresses — slot name, optional key
// (nil unless keyed), and which family it belongs to. Step uses this to
// dispatch to and reuse exactly the same Slot(+Key)-addressed logic that
// existed before InteractionID did, instead of duplicating it.
type resolvedInteraction struct {
	slot     string
	key      engine.Value
	keyed    bool
	askGroup bool
}

// resolveInteractionID searches every Question/Ask Group pending
// occurrence (ordinary and keyed) on instance for the one whose
// InteractionID equals id — a linear scan, consistent with this
// package's existing keyed-slot Value.Equal-searched precedent, since a
// workflow instance's simultaneously-pending-interaction count is
// expected to stay small. Returns false if id addresses nothing
// currently pending (unknown, stale, or already answered/closed).
func resolveInteractionID(instance engine.WorkflowInstance, id engine.InteractionID) (resolvedInteraction, bool) {
	for _, s := range instance.QuestionSlots {
		if s.Pending != nil && s.Pending.InteractionID == id {
			return resolvedInteraction{slot: s.Name}, true
		}
	}
	for _, s := range instance.KeyedQuestionSlots {
		for _, p := range s.Pending {
			if p.InteractionID == id {
				return resolvedInteraction{slot: s.Name, key: p.Key, keyed: true}, true
			}
		}
	}
	for _, s := range instance.AskGroupSlots {
		if s.Pending != nil && s.Pending.InteractionID == id {
			return resolvedInteraction{slot: s.Name, askGroup: true}, true
		}
	}
	for _, s := range instance.KeyedAskGroupSlots {
		for _, p := range s.Pending {
			if p.InteractionID == id {
				return resolvedInteraction{slot: s.Name, key: p.Key, keyed: true, askGroup: true}, true
			}
		}
	}
	return resolvedInteraction{}, false
}
