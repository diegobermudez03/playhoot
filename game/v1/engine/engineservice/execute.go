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
// it returns an error. random is likewise a local candidate: it only
// ever advances here, in execOperation's DrawRandomOperation case, and
// is only written back to the committed Snapshot on success — see
// engine.RandomState's doc comment for why a failed step must not
// advance it.
type execContext struct {
	program  engine.Program
	workflow engine.Workflow
	global   engine.RecordValue
	local    engine.RecordValue
	random   engine.RandomState
	limits   engine.Limits
	opCount  int

	// path addresses, within the current game instance's child-workflow
	// tree, the WorkflowInstance this execContext is executing a
	// transition for — the same path Step resolved via
	// engine.Signal.Path. execSpawnChildWorkflow appends the spawned
	// slot's name to this to address the new child's own WorkflowStarted
	// signal.
	path []string

	// questionSlots, timerSlots, childSlots, and askGroupSlots are
	// candidate copies of the current instance's slot instances — copied
	// once, up front, from engine.Snapshot so every mutation (an
	// operation's, or Step's own slot-clearing on an accepted answer,
	// timer expiration, child-outcome join, or ask-group-completion
	// join) is applied to this copy and never to the original
	// Snapshot's slices.
	questionSlots []engine.QuestionSlotInstance
	timerSlots    []engine.TimerSlotInstance
	childSlots    []engine.ChildWorkflowSlotInstance
	askGroupSlots []engine.AskGroupSlotInstance

	// outputs accumulates every declarative engine.Output produced so
	// far. Per LOGICAL_CONTRACT.md, the engine only ever describes what
	// should happen; ctx.outputs becomes engine.Commit.Outputs only if
	// the whole step succeeds, and is discarded otherwise.
	outputs []engine.Output

	// internalSignals accumulates every engine.Signal this step causes
	// but does not itself apply — currently, only the WorkflowStarted
	// signal a SpawnChildWorkflowOperation causes for its new child.
	// Per LOGICAL_CONTRACT.md, these become engine.Commit.InternalSignals
	// only if the whole step succeeds, for a later Step call to apply.
	internalSignals []engine.Signal
}

// findQuestionSlot returns the index of the question slot named name in
// ctx.questionSlots, if any.
func (ctx *execContext) findQuestionSlot(name string) (int, bool) {
	for i, s := range ctx.questionSlots {
		if s.Name == name {
			return i, true
		}
	}
	return 0, false
}

// findTimerSlot returns the index of the timer slot named name in
// ctx.timerSlots, if any.
func (ctx *execContext) findTimerSlot(name string) (int, bool) {
	for i, s := range ctx.timerSlots {
		if s.Name == name {
			return i, true
		}
	}
	return 0, false
}

// questionSlotDeclaration returns the compiled engine.QuestionSlot named
// name on ctx.workflow, if any — used to recover the Question name a
// slot was declared against, for OpenQuestionOutput.
func (ctx *execContext) questionSlotDeclaration(name string) (engine.QuestionSlot, bool) {
	for _, s := range ctx.workflow.QuestionSlots {
		if s.Name == name {
			return s, true
		}
	}
	return engine.QuestionSlot{}, false
}

// findChildSlot returns the index of the child slot named name in
// ctx.childSlots, if any.
func (ctx *execContext) findChildSlot(name string) (int, bool) {
	for i, s := range ctx.childSlots {
		if s.Name == name {
			return i, true
		}
	}
	return 0, false
}

// childSlotDeclaration returns the compiled engine.ChildWorkflowSlot
// named name on ctx.workflow, if any — used to recover the workflow
// type a slot was declared against, for execSpawnChildWorkflow.
func (ctx *execContext) childSlotDeclaration(name string) (engine.ChildWorkflowSlot, bool) {
	for _, s := range ctx.workflow.ChildSlots {
		if s.Name == name {
			return s, true
		}
	}
	return engine.ChildWorkflowSlot{}, false
}

// findAskGroupSlot returns the index of the ask-group slot named name in
// ctx.askGroupSlots, if any.
func (ctx *execContext) findAskGroupSlot(name string) (int, bool) {
	for i, s := range ctx.askGroupSlots {
		if s.Name == name {
			return i, true
		}
	}
	return 0, false
}

// askGroupSlotDeclaration returns the compiled engine.AskGroupSlot named
// name on ctx.workflow, if any — used to recover the Question name a
// slot was declared against, for OpenQuestionOutput.
func (ctx *execContext) askGroupSlotDeclaration(name string) (engine.AskGroupSlot, bool) {
	for _, s := range ctx.workflow.AskGroupSlots {
		if s.Name == name {
			return s, true
		}
	}
	return engine.AskGroupSlot{}, false
}

// evalCallArguments evaluates args in order into their captured values.
func evalCallArguments(ctx *execContext, args []engine.CallArgument, scope engine.Scope) ([]engine.FieldValue, error) {
	result := make([]engine.FieldValue, len(args))
	for i, a := range args {
		v, err := Evaluate(ctx.program, a.Value, scope)
		if err != nil {
			return nil, err
		}
		result[i] = engine.FieldValue{Name: a.Name, Value: v}
	}
	return result, nil
}

// consumeOp counts one more operation toward ctx.limits.MaxOperations —
// called once per operation execOperation executes, including once per
// ForEachOperation iteration and once per branch taken inside a nested
// Block, so a runaway transition (an enormous or effectively unbounded
// combination of loops and branches) fails deterministically instead of
// hanging.
func (ctx *execContext) consumeOp() error {
	ctx.opCount++
	if ctx.opCount > ctx.limits.MaxOperations {
		return newExecutionError(ExecutionErrorBudgetExceeded,
			"engineservice: exceeded the maximum of %d operations for one step", ctx.limits.MaxOperations)
	}
	return nil
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
	if err := ctx.consumeOp(); err != nil {
		return scope, err
	}

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
		elements := coll.(engine.ListValue).Elements
		if len(elements) > ctx.limits.MaxLoopIterations {
			return scope, newExecutionError(ExecutionErrorLoopLimitExceeded,
				"engineservice: for-each would run %d iterations, exceeding the limit of %d", len(elements), ctx.limits.MaxLoopIterations)
		}
		for i, item := range elements {
			if _, err := execBlock(ctx, o.Body, bindItem(scope, o.ItemName, o.IndexName, item, i)); err != nil {
				return scope, err
			}
		}
		return scope, nil

	case engine.DrawRandomOperation:
		v, err := ctx.drawRandom(o.Generator, scope)
		if err != nil {
			return scope, err
		}
		return extendScope(scope, o.Name, v), nil

	case engine.OpenQuestionOperation:
		return scope, ctx.execOpenQuestion(o, scope)

	case engine.CloseQuestionOperation:
		return scope, ctx.execCloseQuestion(o)

	case engine.ScheduleTimerOperation:
		return scope, ctx.execScheduleTimer(o, scope)

	case engine.CancelTimerOperation:
		return scope, ctx.execCancelTimer(o)

	case engine.EmitEffectOperation:
		return scope, ctx.execEmitEffect(o, scope)

	case engine.SpawnChildWorkflowOperation:
		return scope, ctx.execSpawnChildWorkflow(o, scope)

	case engine.CancelChildWorkflowOperation:
		return scope, ctx.execCancelChildWorkflow(o, scope)

	case engine.OpenAskGroupOperation:
		return scope, ctx.execOpenAskGroup(o, scope)

	case engine.FinalizeAskGroupOperation:
		return scope, ctx.execFinalizeAskGroup(o)

	case engine.CancelAskGroupOperation:
		return scope, ctx.execCancelAskGroup(o)

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

// drawRandom evaluates generator against ctx's candidate RandomState,
// advancing it exactly once per drawn value (once for
// RandomIntegerGenerator and RandomElementGenerator, once per shuffled
// element for RandomShuffleGenerator) and returning the produced Value.
func (ctx *execContext) drawRandom(generator engine.RandomGenerator, scope engine.Scope) (engine.Value, error) {
	switch g := generator.(type) {
	case engine.RandomIntegerGenerator:
		minV, err := Evaluate(ctx.program, g.Minimum, scope)
		if err != nil {
			return nil, err
		}
		maxV, err := Evaluate(ctx.program, g.Maximum, scope)
		if err != nil {
			return nil, err
		}
		minI, minOK := intIndex(minV.(engine.NumberValue).Value)
		maxI, maxOK := intIndex(maxV.(engine.NumberValue).Value)
		if !minOK || !maxOK || minI > maxI {
			return nil, newExecutionError(ExecutionErrorInvalidRandomRange,
				"engineservice: invalid random integer range [%v, %v]", minV.(engine.NumberValue).Value, maxV.(engine.NumberValue).Value)
		}
		span := uint64(maxI-minI) + 1
		state, raw := ctx.random.Next()
		ctx.random = state
		return engine.NumberValue{Value: float64(minI) + float64(raw%span)}, nil

	case engine.RandomElementGenerator:
		collV, err := Evaluate(ctx.program, g.Collection, scope)
		if err != nil {
			return nil, err
		}
		elements := collV.(engine.ListValue).Elements
		if len(elements) == 0 {
			return nil, newExecutionError(ExecutionErrorEmptyRandomCollection,
				"engineservice: cannot draw a random element from an empty collection")
		}
		state, raw := ctx.random.Next()
		ctx.random = state
		return elements[raw%uint64(len(elements))], nil

	case engine.RandomShuffleGenerator:
		collV, err := Evaluate(ctx.program, g.Collection, scope)
		if err != nil {
			return nil, err
		}
		list := collV.(engine.ListValue)
		shuffled := make([]engine.Value, len(list.Elements))
		copy(shuffled, list.Elements)
		for i := len(shuffled) - 1; i > 0; i-- {
			state, raw := ctx.random.Next()
			ctx.random = state
			j := int(raw % uint64(i+1))
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		}
		return engine.ListValue{ElementType: list.ElementType, Elements: shuffled}, nil

	default:
		return nil, newExecutionError(ExecutionErrorUnknown, "engineservice: unsupported random generator %T", generator)
	}
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

// execOpenQuestion evaluates o.Recipient and o.Arguments, then occupies
// the named question slot in ctx's candidate instance state, producing
// an OpenQuestionOutput. Opening an already occupied slot fails the
// entire transition atomically — see program.OpenQuestionOperation.
func (ctx *execContext) execOpenQuestion(o engine.OpenQuestionOperation, scope engine.Scope) error {
	idx, ok := ctx.findQuestionSlot(o.Slot)
	if !ok {
		return newExecutionError(ExecutionErrorUnknown, "engineservice: question slot %q not found", o.Slot)
	}
	if ctx.questionSlots[idx].Pending != nil {
		return newExecutionError(ExecutionErrorSlotOccupied, "engineservice: question slot %q is already occupied", o.Slot)
	}

	recipientV, err := Evaluate(ctx.program, o.Recipient, scope)
	if err != nil {
		return err
	}
	args, err := evalCallArguments(ctx, o.Arguments, scope)
	if err != nil {
		return err
	}

	recipient := recipientV.(engine.UserValue).ID
	ctx.questionSlots[idx] = engine.QuestionSlotInstance{
		Name:    o.Slot,
		Pending: &engine.PendingQuestion{Recipient: recipient, Arguments: args},
	}

	slotDecl, _ := ctx.questionSlotDeclaration(o.Slot)
	ctx.outputs = append(ctx.outputs, engine.OpenQuestionOutput{
		Slot:      o.Slot,
		Recipient: recipient,
		Question:  slotDecl.Question,
		Arguments: args,
	})
	return nil
}

// execCloseQuestion clears the named question slot in ctx's candidate
// instance state, if occupied, producing a CloseQuestionOutput.
// Closing an already empty slot is an idempotent no-op — see
// program.CloseQuestionOperation.
func (ctx *execContext) execCloseQuestion(o engine.CloseQuestionOperation) error {
	idx, ok := ctx.findQuestionSlot(o.Slot)
	if !ok {
		return newExecutionError(ExecutionErrorUnknown, "engineservice: question slot %q not found", o.Slot)
	}
	pending := ctx.questionSlots[idx].Pending
	if pending == nil {
		return nil
	}
	ctx.questionSlots[idx] = engine.QuestionSlotInstance{Name: o.Slot}
	ctx.outputs = append(ctx.outputs, engine.CloseQuestionOutput{Slot: o.Slot, Recipient: pending.Recipient})
	return nil
}

// execScheduleTimer evaluates o.DelayMilliseconds, validates it is a
// finite, non-negative integer, and occupies the named timer slot in
// ctx's candidate instance state, producing a ScheduleTimerOutput.
// Scheduling into an already occupied slot fails the entire transition
// atomically — see program.ScheduleTimerOperation. The engine itself
// never starts a real timer; delivering the resulting
// TimerExpiredSignalSource signal after DelayMilliseconds is an
// application layer's job.
func (ctx *execContext) execScheduleTimer(o engine.ScheduleTimerOperation, scope engine.Scope) error {
	idx, ok := ctx.findTimerSlot(o.Slot)
	if !ok {
		return newExecutionError(ExecutionErrorUnknown, "engineservice: timer slot %q not found", o.Slot)
	}
	if ctx.timerSlots[idx].Pending {
		return newExecutionError(ExecutionErrorSlotOccupied, "engineservice: timer slot %q is already occupied", o.Slot)
	}

	delayV, err := Evaluate(ctx.program, o.DelayMilliseconds, scope)
	if err != nil {
		return err
	}
	delay := delayV.(engine.NumberValue).Value
	if i, ok := intIndex(delay); !ok || i < 0 {
		return newExecutionError(ExecutionErrorInvalidTimerDelay,
			"engineservice: timer delay must be a non-negative integer number of milliseconds, got %v", delay)
	}

	ctx.timerSlots[idx] = engine.TimerSlotInstance{Name: o.Slot, Pending: true}
	ctx.outputs = append(ctx.outputs, engine.ScheduleTimerOutput{Slot: o.Slot, DelayMilliseconds: delay})
	return nil
}

// execCancelTimer clears the named timer slot in ctx's candidate
// instance state, if occupied, producing a CancelTimerOutput.
// Cancelling an already empty slot is an idempotent no-op — see
// program.CancelTimerOperation.
func (ctx *execContext) execCancelTimer(o engine.CancelTimerOperation) error {
	idx, ok := ctx.findTimerSlot(o.Slot)
	if !ok {
		return newExecutionError(ExecutionErrorUnknown, "engineservice: timer slot %q not found", o.Slot)
	}
	if !ctx.timerSlots[idx].Pending {
		return nil
	}
	ctx.timerSlots[idx] = engine.TimerSlotInstance{Name: o.Slot}
	ctx.outputs = append(ctx.outputs, engine.CancelTimerOutput{Slot: o.Slot})
	return nil
}

// execEmitEffect evaluates o.Recipients and o.Arguments and produces an
// EmitEffectOutput. This never mutates any candidate state — an effect
// is presentation-only.
func (ctx *execContext) execEmitEffect(o engine.EmitEffectOperation, scope engine.Scope) error {
	recipientsV, err := Evaluate(ctx.program, o.Recipients, scope)
	if err != nil {
		return err
	}
	elements := recipientsV.(engine.ListValue).Elements
	recipients := make([]engine.UserID, len(elements))
	for i, el := range elements {
		recipients[i] = el.(engine.UserValue).ID
	}

	args, err := evalCallArguments(ctx, o.Arguments, scope)
	if err != nil {
		return err
	}

	ctx.outputs = append(ctx.outputs, engine.EmitEffectOutput{Effect: o.Effect, Recipients: recipients, Arguments: args})
	return nil
}

// execSpawnChildWorkflow evaluates o.Arguments and creates a new child
// workflow instance in the named child slot in ctx's candidate instance
// state. Spawning into an already occupied slot — running or a terminal
// outcome still awaiting join — fails the entire transition atomically
// — see program.SpawnChildWorkflowOperation. This does not itself apply
// the child's WorkflowStarted transition; it queues the corresponding
// engine.Signal, addressed to the new child, as one of this step's
// internalSignals for a later Step call to apply.
func (ctx *execContext) execSpawnChildWorkflow(o engine.SpawnChildWorkflowOperation, scope engine.Scope) error {
	idx, ok := ctx.findChildSlot(o.Slot)
	if !ok {
		return newExecutionError(ExecutionErrorUnknown, "engineservice: child slot %q not found", o.Slot)
	}
	if ctx.childSlots[idx].Child != nil {
		return newExecutionError(ExecutionErrorSlotOccupied, "engineservice: child slot %q is already occupied", o.Slot)
	}

	slotDecl, _ := ctx.childSlotDeclaration(o.Slot)
	childWorkflow, ok := ctx.program.Workflows[slotDecl.Workflow]
	if !ok {
		return newExecutionError(ExecutionErrorUnknown, "engineservice: workflow %q is not compiled", slotDecl.Workflow)
	}

	args, err := evalCallArguments(ctx, o.Arguments, scope)
	if err != nil {
		return err
	}
	child, err := newChildInstance(ctx.program, childWorkflow, args)
	if err != nil {
		return err
	}

	ctx.childSlots[idx] = engine.ChildWorkflowSlotInstance{Name: o.Slot, Child: &child}

	childPath := make([]string, len(ctx.path)+1)
	copy(childPath, ctx.path)
	childPath[len(ctx.path)] = o.Slot
	ctx.internalSignals = append(ctx.internalSignals, engine.Signal{Kind: engine.SignalKindNamed, Path: childPath, Name: "WorkflowStarted"})
	return nil
}

// execCancelChildWorkflow evaluates o.Reason and recursively discards
// the running child workflow instance — together with every descendant
// it owns — in the named child slot in ctx's candidate instance state.
// This is parent-driven cancellation: it never produces a signal, since
// program.CancelChildWorkflowOperation documents that the parent already
// knows it requested the cancellation. Cancelling an already empty slot
// is an idempotent no-op. Cancelling a slot holding a terminal outcome
// still awaiting join fails the entire transition atomically — that
// outcome must be joined through its own child-outcome signal first,
// never silently discarded.
func (ctx *execContext) execCancelChildWorkflow(o engine.CancelChildWorkflowOperation, scope engine.Scope) error {
	idx, ok := ctx.findChildSlot(o.Slot)
	if !ok {
		return newExecutionError(ExecutionErrorUnknown, "engineservice: child slot %q not found", o.Slot)
	}
	child := ctx.childSlots[idx].Child
	if child == nil {
		return nil
	}
	if child.Outcome != nil {
		return newExecutionError(ExecutionErrorChildOutcomeNotJoined,
			"engineservice: child slot %q holds a terminal outcome that must be joined before it can be cancelled", o.Slot)
	}

	if _, err := Evaluate(ctx.program, o.Reason, scope); err != nil {
		return err
	}

	// Dropping the pointer discards the entire subtree at once: every
	// descendant this child owns goes with it, recursively, since
	// nothing else references it.
	ctx.childSlots[idx] = engine.ChildWorkflowSlotInstance{Name: o.Slot}
	return nil
}
