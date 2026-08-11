package runtime

import (
	"github.com/diegobermudez03/playhoot/game/v1/engine"
)

// presentationKey identifies one occupied (PresentationSlot, user) pair
// — per program.PresentationSlotDeclaration, at most one presentation
// may occupy a given slot for a given user at a time.
type presentationKey struct {
	Slot      string
	Recipient engine.UserID
}

// presentationEntry is one active presentation, ready to become an
// ActivatePresentationOutput or UpdatePresentationOutput. Name is the
// stable identity diffPresentations correlates across recomputation —
// a workflow-level or state-level Presentation's own declared Name, or
// a pending question's owning QuestionSlot's Name.
type presentationEntry struct {
	Key   presentationKey
	Name  string
	View  string
	Model engine.Value
}

// deriveActivePresentations computes every presentation active for
// instance right now: every workflow-level Presentation (active for
// the instance's whole lifetime), every state-level Presentation of
// instance's current State (active only while in that state), and one
// entry per occupied QuestionSlot with a non-nil Presentation (active
// only while that question is pending) — see program.PresentationDeclaration
// and program.QuestionPresentationDeclaration. A terminated instance
// (Outcome != nil) has no active presentations at all.
//
// Per program.ProjectionDeclaration's documented "only a successfully
// committed snapshot may ever be projected", callers must only ever
// pass an instance and global state that already reflect a fully
// validated, about-to-commit (or already-committed) result — never a
// speculative or partially applied one.
func deriveActivePresentations(p engine.Program, workflow engine.Workflow, instance engine.WorkflowInstance, global engine.RecordValue) ([]presentationEntry, error) {
	if instance.Outcome != nil {
		return nil, nil
	}

	var entries []presentationEntry
	seen := make(map[presentationKey]bool)

	addOccupant := func(key presentationKey, name, view string, model engine.Value) error {
		if seen[key] {
			return newExecutionError(ExecutionErrorPresentationSlotOccupied,
				"engineservice: presentation slot %q is occupied more than once for user %q", key.Slot, key.Recipient)
		}
		seen[key] = true
		entries = append(entries, presentationEntry{Key: key, Name: name, View: view, Model: model})
		return nil
	}

	addTargeted := func(decls []engine.Presentation) error {
		scope := instanceBaseScope(instance, global)
		for _, decl := range decls {
			targetsV, err := Evaluate(p, decl.Targets, scope)
			if err != nil {
				return err
			}
			argValues, err := evaluateArguments(p, decl.ProjectionArguments, scope)
			if err != nil {
				return err
			}
			projection, ok := p.Projections[decl.Projection]
			if !ok {
				return newExecutionError(ExecutionErrorUnknown, "engineservice: projection %q is not compiled", decl.Projection)
			}
			for _, el := range targetsV.(engine.ListValue).Elements {
				user := el.(engine.UserValue).ID
				model, err := evaluateProjectionModel(p, projection, argValues, global, user)
				if err != nil {
					return err
				}
				if err := addOccupant(presentationKey{Slot: decl.Slot, Recipient: user}, decl.Name, decl.View, model); err != nil {
					return err
				}
			}
		}
		return nil
	}

	if err := addTargeted(workflow.Presentations); err != nil {
		return nil, err
	}
	if state, ok := workflowStateByName(workflow, instance.State); ok {
		if err := addTargeted(state.Presentations); err != nil {
			return nil, err
		}
	}

	for _, slot := range instance.QuestionSlots {
		if slot.Pending == nil {
			continue
		}
		slotDecl, ok := workflowQuestionSlot(workflow, slot.Name)
		if !ok || slotDecl.Presentation == nil {
			continue
		}
		pres := slotDecl.Presentation

		qScope := engine.Scope{Bindings: map[string]engine.Value{
			globalScopeRootName: global,
			"recipient":         engine.UserValue{ID: slot.Pending.Recipient},
		}}
		for _, arg := range slot.Pending.Arguments {
			qScope = extendScope(qScope, arg.Name, arg.Value)
		}
		argValues, err := evaluateArguments(p, pres.ProjectionArguments, qScope)
		if err != nil {
			return nil, err
		}
		projection, ok := p.Projections[pres.Projection]
		if !ok {
			return nil, newExecutionError(ExecutionErrorUnknown, "engineservice: projection %q is not compiled", pres.Projection)
		}
		model, err := evaluateProjectionModel(p, projection, argValues, global, slot.Pending.Recipient)
		if err != nil {
			return nil, err
		}
		if err := addOccupant(presentationKey{Slot: pres.Slot, Recipient: slot.Pending.Recipient}, slot.Name, pres.View, model); err != nil {
			return nil, err
		}
	}

	return entries, nil
}

// evaluateProjectionModel evaluates projection.Body against global, the
// implicit "viewer" binding bound to viewer, and argValues bound to
// projection's declared Parameters — never "local" or any
// transition-specific binding, per program.ProjectionDeclaration's
// documented body scope.
func evaluateProjectionModel(p engine.Program, projection engine.Projection, argValues []engine.FieldValue, global engine.RecordValue, viewer engine.UserID) (engine.Value, error) {
	bindings := make(map[string]engine.Value, len(argValues)+2)
	bindings[globalScopeRootName] = global
	bindings["viewer"] = engine.UserValue{ID: viewer}
	for _, av := range argValues {
		bindings[av.Name] = av.Value
	}
	return Evaluate(p, projection.Body, engine.Scope{Bindings: bindings})
}

// evaluateArguments evaluates args in order into their captured values
// — like evalCallArguments, but usable outside an execContext, since
// presentation derivation runs after a transition's own operations
// have already finished executing.
func evaluateArguments(p engine.Program, args []engine.CallArgument, scope engine.Scope) ([]engine.FieldValue, error) {
	result := make([]engine.FieldValue, len(args))
	for i, a := range args {
		v, err := Evaluate(p, a.Value, scope)
		if err != nil {
			return nil, err
		}
		result[i] = engine.FieldValue{Name: a.Name, Value: v}
	}
	return result, nil
}

// diffPresentations compares before (the active set immediately before
// the current transition ran) against after (the active set once it
// committed) and returns one Output per change, in a deterministic
// order: every entry in after (in derivation order) becomes an
// ActivatePresentationOutput if its Key was not active before, or an
// UpdatePresentationOutput if it was; every entry in before (in
// derivation order) whose Key is no longer in after becomes a
// RemovePresentationOutput. See deriveActivePresentations.
func diffPresentations(before, after []presentationEntry) []engine.Output {
	beforeByKey := make(map[presentationKey]presentationEntry, len(before))
	for _, e := range before {
		beforeByKey[e.Key] = e
	}
	afterByKey := make(map[presentationKey]presentationEntry, len(after))
	for _, e := range after {
		afterByKey[e.Key] = e
	}

	var outputs []engine.Output
	for _, e := range after {
		if _, existed := beforeByKey[e.Key]; existed {
			outputs = append(outputs, engine.UpdatePresentationOutput{Slot: e.Key.Slot, Recipient: e.Key.Recipient, Name: e.Name, Model: e.Model})
		} else {
			outputs = append(outputs, engine.ActivatePresentationOutput{Slot: e.Key.Slot, Recipient: e.Key.Recipient, Name: e.Name, View: e.View, Model: e.Model})
		}
	}
	for _, e := range before {
		if _, stillActive := afterByKey[e.Key]; !stillActive {
			outputs = append(outputs, engine.RemovePresentationOutput{Slot: e.Key.Slot, Recipient: e.Key.Recipient, Name: e.Name})
		}
	}
	return outputs
}
