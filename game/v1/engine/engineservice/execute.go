package engineservice

import (
	"github.com/diegobermudez03/playhoot/game/v1/engine"
)

// execContext holds the mutable candidate state one Step call executes
// a transition's Operations against. Every mutation goes through
// pathUpdate, which reconstructs only the path from the mutated root to
// the written value — the untouched rest of global/local is shared with
// the original engine.Snapshot, never mutated in place, which is what
// lets Step leave that original Snapshot valid and unchanged whenever
// it returns an error.
type execContext struct {
	program engine.Program
	global  engine.RecordValue
	local   engine.RecordValue
}

// execBlock executes block's operations in order against ctx, threading
// scope forward exactly as compileBlock threaded it at compile time, so
// a LetOperation's binding is visible to later operations in this same
// Block and, for a transition's top-level Block, to its Control.
func execBlock(ctx *execContext, block engine.Block, scope engine.Scope) (engine.Scope, error) {
	for _, op := range block.Operations {
		var err error
		scope, err = execOperation(ctx, op, scope)
		if err != nil {
			return scope, err
		}
	}
	return scope, nil
}

func execOperation(ctx *execContext, op engine.Operation, scope engine.Scope) (engine.Scope, error) {
	switch o := op.(type) {
	case engine.LetOperation:
		v, err := Evaluate(ctx.program, o.Value, scope)
		if err != nil {
			return scope, err
		}
		return extendScope(scope, o.Name, v), nil

	case engine.SetOperation:
		v, err := Evaluate(ctx.program, o.Value, scope)
		if err != nil {
			return scope, err
		}
		return scope, ctx.assign(o.Target, scope, func(engine.Value) (engine.Value, error) { return v, nil })

	case engine.ListAppendOperation:
		v, err := Evaluate(ctx.program, o.Value, scope)
		if err != nil {
			return scope, err
		}
		return scope, ctx.assign(o.Target, scope, func(cur engine.Value) (engine.Value, error) {
			lv := cur.(engine.ListValue)
			elements := make([]engine.Value, len(lv.Elements)+1)
			copy(elements, lv.Elements)
			elements[len(lv.Elements)] = v
			return engine.ListValue{ElementType: lv.ElementType, Elements: elements}, nil
		})

	case engine.ListInsertOperation:
		idx, err := Evaluate(ctx.program, o.Index, scope)
		if err != nil {
			return scope, err
		}
		v, err := Evaluate(ctx.program, o.Value, scope)
		if err != nil {
			return scope, err
		}
		return scope, ctx.assign(o.Target, scope, func(cur engine.Value) (engine.Value, error) {
			lv := cur.(engine.ListValue)
			i, ok := intIndex(idx.(engine.NumberValue).Value)
			if !ok || i < 0 || i > len(lv.Elements) {
				return nil, newExecutionError(ExecutionErrorIndexOutOfRange, "engineservice: list insert index out of range")
			}
			elements := make([]engine.Value, 0, len(lv.Elements)+1)
			elements = append(elements, lv.Elements[:i]...)
			elements = append(elements, v)
			elements = append(elements, lv.Elements[i:]...)
			return engine.ListValue{ElementType: lv.ElementType, Elements: elements}, nil
		})

	case engine.ListRemoveAtOperation:
		idx, err := Evaluate(ctx.program, o.Index, scope)
		if err != nil {
			return scope, err
		}
		return scope, ctx.assign(o.Target, scope, func(cur engine.Value) (engine.Value, error) {
			lv := cur.(engine.ListValue)
			i, ok := intIndex(idx.(engine.NumberValue).Value)
			if !ok || i < 0 || i >= len(lv.Elements) {
				return nil, newExecutionError(ExecutionErrorIndexOutOfRange, "engineservice: list remove index out of range")
			}
			elements := make([]engine.Value, 0, len(lv.Elements)-1)
			elements = append(elements, lv.Elements[:i]...)
			elements = append(elements, lv.Elements[i+1:]...)
			return engine.ListValue{ElementType: lv.ElementType, Elements: elements}, nil
		})

	case engine.MapPutOperation:
		key, err := Evaluate(ctx.program, o.Key, scope)
		if err != nil {
			return scope, err
		}
		v, err := Evaluate(ctx.program, o.Value, scope)
		if err != nil {
			return scope, err
		}
		return scope, ctx.assign(o.Target, scope, func(cur engine.Value) (engine.Value, error) {
			mv := cur.(engine.MapValue)
			entries := make([]engine.MapEntry, 0, len(mv.Entries)+1)
			replaced := false
			for _, e := range mv.Entries {
				if !replaced && e.Key.Equal(key) {
					entries = append(entries, engine.MapEntry{Key: key, Value: v})
					replaced = true
					continue
				}
				entries = append(entries, e)
			}
			if !replaced {
				entries = append(entries, engine.MapEntry{Key: key, Value: v})
			}
			return engine.MapValue{KeyType: mv.KeyType, ValueType: mv.ValueType, Entries: entries}, nil
		})

	case engine.MapDeleteOperation:
		key, err := Evaluate(ctx.program, o.Key, scope)
		if err != nil {
			return scope, err
		}
		return scope, ctx.assign(o.Target, scope, func(cur engine.Value) (engine.Value, error) {
			mv := cur.(engine.MapValue)
			entries := make([]engine.MapEntry, 0, len(mv.Entries))
			for _, e := range mv.Entries {
				if !e.Key.Equal(key) {
					entries = append(entries, e)
				}
			}
			return engine.MapValue{KeyType: mv.KeyType, ValueType: mv.ValueType, Entries: entries}, nil
		})

	case engine.IfOperation:
		cond, err := Evaluate(ctx.program, o.Condition, scope)
		if err != nil {
			return scope, err
		}
		if cond.(engine.BoolValue).Value {
			_, err = execBlock(ctx, o.Then, scope)
		} else {
			_, err = execBlock(ctx, o.Else, scope)
		}
		return scope, err

	case engine.ForEachOperation:
		coll, err := Evaluate(ctx.program, o.Collection, scope)
		if err != nil {
			return scope, err
		}
		for i, item := range coll.(engine.ListValue).Elements {
			if _, err := execBlock(ctx, o.Body, bindItem(scope, o.ItemName, o.IndexName, item, i)); err != nil {
				return scope, err
			}
		}
		return scope, nil

	case engine.MatchOperation:
		v, err := Evaluate(ctx.program, o.Value, scope)
		if err != nil {
			return scope, err
		}
		for _, cs := range o.Cases {
			caseScope, matched := matchPattern(cs.Pattern, v, scope)
			if matched {
				_, err := execBlock(ctx, cs.Body, caseScope)
				return scope, err
			}
		}
		return scope, newExecutionError(ExecutionErrorNoMatchingCase, "engineservice: no match case matched the value in a match operation")

	default:
		return scope, newExecutionError(ExecutionErrorUnknown, "engineservice: cannot execute operation of type %T", op)
	}
}

// assign applies fn to the value currently stored at target — reading
// it, then replacing it with fn's result — mutating ctx.global or
// ctx.local as target's root names. Every intermediate value on the
// path is a fresh copy; every value off the path is shared with the
// original, unmutated engine.Snapshot.
func (ctx *execContext) assign(target engine.AssignmentTarget, scope engine.Scope, fn func(engine.Value) (engine.Value, error)) error {
	root, path, err := ctx.flattenTarget(target, scope)
	if err != nil {
		return err
	}
	switch root {
	case "global":
		updated, err := applyPath(ctx.global, path, fn)
		if err != nil {
			return err
		}
		ctx.global = updated.(engine.RecordValue)
	case "local":
		updated, err := applyPath(ctx.local, path, fn)
		if err != nil {
			return err
		}
		ctx.local = updated.(engine.RecordValue)
	default:
		return newExecutionError(ExecutionErrorUnknown, "engineservice: unknown assignment root %q", root)
	}
	return nil
}

// pathStep is one step of a flattened engine.AssignmentTarget: either a
// record field access (field non-empty) or a list/map access at an
// already-evaluated index/key.
type pathStep struct {
	field string
	index engine.Value
}

// flattenTarget walks target from its outermost accessor down to its
// NameTarget root, evaluating every IndexTarget's Index along the way,
// and returns the root name together with the path from root to target
// in root-to-leaf order.
func (ctx *execContext) flattenTarget(target engine.AssignmentTarget, scope engine.Scope) (string, []pathStep, error) {
	switch t := target.(type) {
	case engine.NameTarget:
		return t.Name, nil, nil
	case engine.FieldTarget:
		root, path, err := ctx.flattenTarget(t.Target, scope)
		if err != nil {
			return "", nil, err
		}
		return root, append(path, pathStep{field: t.Field}), nil
	case engine.IndexTarget:
		root, path, err := ctx.flattenTarget(t.Target, scope)
		if err != nil {
			return "", nil, err
		}
		idx, err := Evaluate(ctx.program, t.Index, scope)
		if err != nil {
			return "", nil, err
		}
		return root, append(path, pathStep{index: idx}), nil
	default:
		return "", nil, newExecutionError(ExecutionErrorUnknown, "engineservice: unsupported assignment target %T", target)
	}
}

// applyPath reconstructs current with fn applied to the value found by
// following path, copying only the values along that path.
func applyPath(current engine.Value, path []pathStep, fn func(engine.Value) (engine.Value, error)) (engine.Value, error) {
	if len(path) == 0 {
		return fn(current)
	}
	step := path[0]

	if step.field != "" {
		rv, ok := current.(engine.RecordValue)
		if !ok {
			return nil, newExecutionError(ExecutionErrorUnknown, "engineservice: cannot access field %q on a non-record value", step.field)
		}
		fv, ok := rv.FieldByName(step.field)
		if !ok {
			return nil, newExecutionError(ExecutionErrorUnknown, "engineservice: record %q has no field named %q", rv.TypeName, step.field)
		}
		updated, err := applyPath(fv.Value, path[1:], fn)
		if err != nil {
			return nil, err
		}
		newFields := make([]engine.FieldValue, len(rv.Fields))
		copy(newFields, rv.Fields)
		for i, f := range newFields {
			if f.Name == step.field {
				newFields[i] = engine.FieldValue{Name: step.field, Value: updated}
				break
			}
		}
		return engine.RecordValue{TypeName: rv.TypeName, Fields: newFields}, nil
	}

	switch cv := current.(type) {
	case engine.ListValue:
		i, ok := intIndex(step.index.(engine.NumberValue).Value)
		if !ok || i < 0 || i >= len(cv.Elements) {
			return nil, newExecutionError(ExecutionErrorIndexOutOfRange, "engineservice: list index out of range")
		}
		updated, err := applyPath(cv.Elements[i], path[1:], fn)
		if err != nil {
			return nil, err
		}
		newElements := make([]engine.Value, len(cv.Elements))
		copy(newElements, cv.Elements)
		newElements[i] = updated
		return engine.ListValue{ElementType: cv.ElementType, Elements: newElements}, nil

	case engine.MapValue:
		idx := -1
		for i, e := range cv.Entries {
			if e.Key.Equal(step.index) {
				idx = i
				break
			}
		}
		if idx == -1 {
			return nil, newExecutionError(ExecutionErrorKeyNotFound, "engineservice: map has no entry for the given key")
		}
		updated, err := applyPath(cv.Entries[idx].Value, path[1:], fn)
		if err != nil {
			return nil, err
		}
		newEntries := make([]engine.MapEntry, len(cv.Entries))
		copy(newEntries, cv.Entries)
		newEntries[idx] = engine.MapEntry{Key: cv.Entries[idx].Key, Value: updated}
		return engine.MapValue{KeyType: cv.KeyType, ValueType: cv.ValueType, Entries: newEntries}, nil

	default:
		return nil, newExecutionError(ExecutionErrorUnknown, "engineservice: cannot index a value of type %T", current)
	}
}

func intIndex(n float64) (int, bool) {
	i := int(n)
	return i, float64(i) == n
}

// controlOutcome is applyControl's result: either a state transition
// (Goto sets Changed and State; Stay leaves Changed false) or a
// terminal engine.WorkflowOutcome.
type controlOutcome struct {
	changed bool
	state   string
	outcome *engine.WorkflowOutcome
}

// applyControl evaluates control against scope, recursing through
// ConditionalControl and MatchControl to find the single selected
// terminal outcome.
func applyControl(p engine.Program, control engine.WorkflowControl, scope engine.Scope) (controlOutcome, error) {
	switch c := control.(type) {
	case engine.GotoControl:
		return controlOutcome{changed: true, state: c.State}, nil

	case engine.StayControl:
		return controlOutcome{}, nil

	case engine.CompleteControl:
		v, err := Evaluate(p, c.Result, scope)
		if err != nil {
			return controlOutcome{}, err
		}
		return controlOutcome{outcome: &engine.WorkflowOutcome{Kind: engine.WorkflowOutcomeCompleted, Result: v}}, nil

	case engine.FailControl:
		v, err := Evaluate(p, c.Error, scope)
		if err != nil {
			return controlOutcome{}, err
		}
		return controlOutcome{outcome: &engine.WorkflowOutcome{Kind: engine.WorkflowOutcomeFailed, Error: v.(engine.StringValue).Value}}, nil

	case engine.CancelControl:
		v, err := Evaluate(p, c.Reason, scope)
		if err != nil {
			return controlOutcome{}, err
		}
		return controlOutcome{outcome: &engine.WorkflowOutcome{Kind: engine.WorkflowOutcomeCancelled, Reason: v.(engine.StringValue).Value}}, nil

	case engine.ConditionalControl:
		v, err := Evaluate(p, c.Condition, scope)
		if err != nil {
			return controlOutcome{}, err
		}
		if v.(engine.BoolValue).Value {
			return applyControl(p, c.Then, scope)
		}
		return applyControl(p, c.Else, scope)

	case engine.MatchControl:
		v, err := Evaluate(p, c.Value, scope)
		if err != nil {
			return controlOutcome{}, err
		}
		for _, cs := range c.Cases {
			caseScope, matched := matchPattern(cs.Pattern, v, scope)
			if matched {
				return applyControl(p, cs.Control, caseScope)
			}
		}
		return controlOutcome{}, newExecutionError(ExecutionErrorNoMatchingCase, "engineservice: no match case matched the value in a workflow control")

	default:
		return controlOutcome{}, newExecutionError(ExecutionErrorUnknown, "engineservice: cannot apply workflow control of type %T", control)
	}
}
