package engineservice

import (
	"github.com/diegobermudez03/playhoot/game/v1/engine"
)

// execBeginTaskGroup evaluates o.Completion and changes the named
// task-group slot from empty to building in ctx's candidate instance
// state. Beginning an already occupied slot fails the entire transition
// atomically — see program.BeginTaskGroupOperation. Unlike
// OpenAskGroupOperation, a quorum's bound against the eventual task
// count is not checked here — the group's final task count is not yet
// known; see execSealTaskGroup.
func (ctx *execContext) execBeginTaskGroup(o engine.BeginTaskGroupOperation, scope engine.Scope) error {
	idx, ok := ctx.findTaskGroupSlot(o.Slot)
	if !ok {
		return newExecutionError(ExecutionErrorUnknown, "engineservice: task-group slot %q not found", o.Slot)
	}
	if ctx.taskGroupSlots[idx].Group != nil {
		return newExecutionError(ExecutionErrorSlotOccupied, "engineservice: task-group slot %q is already occupied", o.Slot)
	}

	kind, quorum, err := ctx.resolveTaskGroupCompletionKind(o.Completion, scope)
	if err != nil {
		return err
	}

	ctx.taskGroupSlots[idx] = engine.TaskGroupSlotInstance{
		Name: o.Slot,
		Group: &engine.TaskGroupState{
			Phase:          engine.TaskGroupPhaseBuilding,
			CompletionKind: kind,
			QuorumCount:    quorum,
		},
	}
	return nil
}

// resolveTaskGroupCompletionKind evaluates policy into its durable
// runtime form: an engine.TaskGroupCompletionKind and, for
// TaskGroupQuorumTerminalPolicy, the evaluated quorum count. It only
// validates that a quorum count is structurally a positive integer —
// bounding it against the group's eventual task count is deferred to
// execSealTaskGroup, since the task count is not yet known at Begin
// time.
func (ctx *execContext) resolveTaskGroupCompletionKind(policy engine.TaskGroupCompletionPolicy, scope engine.Scope) (engine.TaskGroupCompletionKind, int, error) {
	switch p := policy.(type) {
	case engine.TaskGroupAllTerminalPolicy:
		return engine.TaskGroupCompletionAllTerminal, 0, nil

	case engine.TaskGroupFirstTerminalPolicy:
		return engine.TaskGroupCompletionFirstTerminal, 0, nil

	case engine.TaskGroupQuorumTerminalPolicy:
		v, err := Evaluate(ctx.program, p.Count, scope)
		if err != nil {
			return 0, 0, err
		}
		count, ok := intIndex(v.(engine.NumberValue).Value)
		if !ok || count <= 0 {
			return 0, 0, newExecutionError(ExecutionErrorInvalidQuorum,
				"engineservice: quorum count must be a positive integer, got %v", v.(engine.NumberValue).Value)
		}
		return engine.TaskGroupCompletionQuorumTerminal, count, nil

	default:
		return 0, 0, newExecutionError(ExecutionErrorUnknown, "engineservice: unsupported task-group completion policy %T", policy)
	}
}

// execSpawnTaskGroupChild evaluates o.Key and o.Arguments and adds one
// task to the named, currently building task-group slot in ctx's
// candidate instance state. Spawning into a slot that is not currently
// building, or under a Key already used by another task in the same
// group, fails the entire transition atomically — see
// program.SpawnTaskGroupChildOperation. This does not itself queue the
// new task's WorkflowStarted signal here; see execSealTaskGroup's doc
// comment for why queuing at either point is equivalent.
func (ctx *execContext) execSpawnTaskGroupChild(o engine.SpawnTaskGroupChildOperation, scope engine.Scope) error {
	idx, ok := ctx.findTaskGroupSlot(o.Slot)
	if !ok {
		return newExecutionError(ExecutionErrorUnknown, "engineservice: task-group slot %q not found", o.Slot)
	}
	group := ctx.taskGroupSlots[idx].Group
	if group == nil || group.Phase != engine.TaskGroupPhaseBuilding {
		return newExecutionError(ExecutionErrorSlotOccupied, "engineservice: task-group slot %q is not currently building", o.Slot)
	}

	keyV, err := Evaluate(ctx.program, o.Key, scope)
	if err != nil {
		return err
	}
	if _, exists := findGroupTaskIndex(*group, keyV); exists {
		return newExecutionError(ExecutionErrorDuplicateTaskKey, "engineservice: task-group slot %q already has a task for the given key", o.Slot)
	}

	slotDecl, _ := ctx.taskGroupSlotDeclaration(o.Slot)
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

	updated := *group
	updated.Tasks = append(append([]engine.TaskGroupTask{}, group.Tasks...), engine.TaskGroupTask{Key: keyV, Child: child})
	ctx.taskGroupSlots[idx] = engine.TaskGroupSlotInstance{Name: o.Slot, Group: &updated}

	taskPath := append(append([]engine.PathStep{}, ctx.path...), engine.PathStep{Slot: o.Slot, TaskKey: keyV})
	ctx.internalSignals = append(ctx.internalSignals, engine.Signal{Kind: engine.SignalKindNamed, Path: taskPath, Name: "WorkflowStarted"})
	return nil
}

// execSealTaskGroup closes membership on the named, currently building
// task-group slot in ctx's candidate instance state: it validates the
// group's completion policy against its final task count — a
// first-terminal group with zero tasks, or a quorum exceeding the final
// task count, fails the transition atomically — then moves the group to
// running, immediately completing it if that is already trivially
// satisfied (an all-terminal group with zero tasks). Sealing a slot
// that is empty, running, or completed-awaiting-join is an execution
// error — see program.SealTaskGroupOperation.
func (ctx *execContext) execSealTaskGroup(o engine.SealTaskGroupOperation) error {
	idx, ok := ctx.findTaskGroupSlot(o.Slot)
	if !ok {
		return newExecutionError(ExecutionErrorUnknown, "engineservice: task-group slot %q not found", o.Slot)
	}
	group := ctx.taskGroupSlots[idx].Group
	if group == nil || group.Phase != engine.TaskGroupPhaseBuilding {
		return newExecutionError(ExecutionErrorSlotOccupied, "engineservice: task-group slot %q is not currently building", o.Slot)
	}

	switch group.CompletionKind {
	case engine.TaskGroupCompletionFirstTerminal:
		if len(group.Tasks) == 0 {
			return newExecutionError(ExecutionErrorInvalidQuorum, "engineservice: a first-terminal task group cannot be sealed with no tasks")
		}
	case engine.TaskGroupCompletionQuorumTerminal:
		if group.QuorumCount > len(group.Tasks) {
			return newExecutionError(ExecutionErrorInvalidQuorum,
				"engineservice: quorum count %d exceeds the sealed task count of %d", group.QuorumCount, len(group.Tasks))
		}
	}

	updated := *group
	updated.Phase = engine.TaskGroupPhaseRunning
	if taskGroupPolicySatisfied(updated) {
		updated.Phase = engine.TaskGroupPhaseCompleted
	}
	ctx.taskGroupSlots[idx] = engine.TaskGroupSlotInstance{Name: o.Slot, Group: &updated}
	return nil
}

// execFinalizeTaskGroup forces the named, currently running task-group
// slot to complete using only the authored terminal outcomes recorded
// so far. Every task that has not yet reached one is left in place but
// permanently unaddressable and reported as unfinished — see
// resolveInstance's TaskGroupPhaseCompleted guard. Finalizing a slot
// already completed-awaiting-join is an idempotent no-op — see
// program.FinalizeTaskGroupOperation's documented answer-versus-deadline
// race resolution. Finalizing an empty or still-building slot is an
// execution error.
func (ctx *execContext) execFinalizeTaskGroup(o engine.FinalizeTaskGroupOperation) error {
	idx, ok := ctx.findTaskGroupSlot(o.Slot)
	if !ok {
		return newExecutionError(ExecutionErrorUnknown, "engineservice: task-group slot %q not found", o.Slot)
	}
	group := ctx.taskGroupSlots[idx].Group
	if group == nil || group.Phase == engine.TaskGroupPhaseBuilding {
		return newExecutionError(ExecutionErrorUnknown, "engineservice: task-group slot %q is empty or still building", o.Slot)
	}
	if group.Phase == engine.TaskGroupPhaseCompleted {
		return nil
	}

	updated := *group
	updated.Phase = engine.TaskGroupPhaseCompleted
	ctx.taskGroupSlots[idx] = engine.TaskGroupSlotInstance{Name: o.Slot, Group: &updated}
	return nil
}

// execCancelTaskGroup evaluates o.Reason and abandons the named
// task-group slot — building or running — in ctx's candidate instance
// state: every task is discarded and the slot cleared, without
// producing a TaskGroupCompletedSignalSource signal. Cancelling an
// already empty slot is an idempotent no-op. Cancelling a slot that
// holds a terminal outcome still awaiting join fails the entire
// transition atomically — see program.CancelTaskGroupOperation.
func (ctx *execContext) execCancelTaskGroup(o engine.CancelTaskGroupOperation, scope engine.Scope) error {
	idx, ok := ctx.findTaskGroupSlot(o.Slot)
	if !ok {
		return newExecutionError(ExecutionErrorUnknown, "engineservice: task-group slot %q not found", o.Slot)
	}
	group := ctx.taskGroupSlots[idx].Group
	if group == nil {
		return nil
	}
	if group.Phase == engine.TaskGroupPhaseCompleted {
		return newExecutionError(ExecutionErrorTaskGroupNotJoined,
			"engineservice: task-group slot %q holds a terminal outcome that must be joined before it can be cancelled", o.Slot)
	}

	if _, err := Evaluate(ctx.program, o.Reason, scope); err != nil {
		return err
	}

	ctx.taskGroupSlots[idx] = engine.TaskGroupSlotInstance{Name: o.Slot}
	return nil
}

// taskGroupPolicySatisfied reports whether group's CompletionKind is
// satisfied by its current TerminalOrder — see engine.TaskGroupCompletionKind.
func taskGroupPolicySatisfied(group engine.TaskGroupState) bool {
	switch group.CompletionKind {
	case engine.TaskGroupCompletionFirstTerminal:
		return len(group.TerminalOrder) >= 1
	case engine.TaskGroupCompletionQuorumTerminal:
		return len(group.TerminalOrder) >= group.QuorumCount
	default:
		return len(group.TerminalOrder) >= len(group.Tasks)
	}
}

// validateTaskGroupCompletion implements program.TaskGroupCompletedSignalSource's
// acceptance rule: the named task-group slot on instance must currently
// hold a terminal outcome awaiting join. This doubles as the
// "duplicate delivery after joining" check, since an accepted
// completion signal clears its slot atomically with the rest of the
// step that handles it.
func validateTaskGroupCompletion(instance engine.WorkflowInstance, signal engine.Signal) error {
	slot, ok := findInstanceTaskGroupSlot(instance, signal.Slot)
	if !ok || slot.Group == nil || slot.Group.Phase != engine.TaskGroupPhaseCompleted {
		return ErrInputRejected
	}
	return nil
}

// taskGroupCompletionFields builds TaskGroupCompletedSignalSource's
// "taskKeys", "terminalKeys", "results", "failures", "cancellations",
// and "unfinished" fields from group's durable data, reading the
// declared KeyType and the referenced child workflow's ResultType from
// slotName's declaration on workflow.
func taskGroupCompletionFields(p engine.Program, workflow engine.Workflow, group *engine.TaskGroupState, slotName string) map[string]engine.Value {
	var keyType, resultType engine.Type
	if slotDecl, ok := workflowTaskGroupSlot(workflow, slotName); ok {
		keyType = slotDecl.KeyType
		if wf, ok := p.Workflows[slotDecl.Workflow]; ok {
			resultType = wf.ResultType
		}
	}

	taskKeys := make([]engine.Value, len(group.Tasks))
	for i, t := range group.Tasks {
		taskKeys[i] = t.Key
	}
	terminalKeys := append([]engine.Value{}, group.TerminalOrder...)

	var resultEntries, failureEntries, cancelEntries []engine.MapEntry
	for _, key := range group.TerminalOrder {
		task, ok := findGroupTask(*group, key)
		if !ok || task.Child.Outcome == nil {
			continue
		}
		switch task.Child.Outcome.Kind {
		case engine.WorkflowOutcomeCompleted:
			resultEntries = append(resultEntries, engine.MapEntry{Key: key, Value: task.Child.Outcome.Result})
		case engine.WorkflowOutcomeFailed:
			failureEntries = append(failureEntries, engine.MapEntry{Key: key, Value: engine.StringValue{Value: task.Child.Outcome.Error}})
		case engine.WorkflowOutcomeCancelled:
			cancelEntries = append(cancelEntries, engine.MapEntry{Key: key, Value: engine.StringValue{Value: task.Child.Outcome.Reason}})
		}
	}

	unfinished := make([]engine.Value, 0)
	for _, t := range group.Tasks {
		if !containsValue(group.TerminalOrder, t.Key) {
			unfinished = append(unfinished, t.Key)
		}
	}

	return map[string]engine.Value{
		"taskKeys":      engine.ListValue{ElementType: keyType, Elements: taskKeys},
		"terminalKeys":  engine.ListValue{ElementType: keyType, Elements: terminalKeys},
		"results":       engine.MapValue{KeyType: keyType, ValueType: resultType, Entries: resultEntries},
		"failures":      engine.MapValue{KeyType: keyType, ValueType: engine.StringType{}, Entries: failureEntries},
		"cancellations": engine.MapValue{KeyType: keyType, ValueType: engine.StringType{}, Entries: cancelEntries},
		"unfinished":    engine.ListValue{ElementType: keyType, Elements: unfinished},
	}
}

func findInstanceTaskGroupSlot(instance engine.WorkflowInstance, name string) (engine.TaskGroupSlotInstance, bool) {
	for _, s := range instance.TaskGroupSlots {
		if s.Name == name {
			return s, true
		}
	}
	return engine.TaskGroupSlotInstance{}, false
}

func findTaskGroupSlotIndex(slots []engine.TaskGroupSlotInstance, name string) (int, bool) {
	for i, s := range slots {
		if s.Name == name {
			return i, true
		}
	}
	return 0, false
}

func workflowTaskGroupSlot(workflow engine.Workflow, name string) (engine.TaskGroupSlot, bool) {
	for _, s := range workflow.TaskGroupSlots {
		if s.Name == name {
			return s, true
		}
	}
	return engine.TaskGroupSlot{}, false
}

// findGroupTask returns the task keyed key in group.Tasks, if any,
// comparing keys with engine.Value.Equal since a task key may be any
// comparable-by-value type, not necessarily a Go-comparable one.
func findGroupTask(group engine.TaskGroupState, key engine.Value) (engine.TaskGroupTask, bool) {
	for _, t := range group.Tasks {
		if t.Key.Equal(key) {
			return t, true
		}
	}
	return engine.TaskGroupTask{}, false
}

func findGroupTaskIndex(group engine.TaskGroupState, key engine.Value) (int, bool) {
	for i, t := range group.Tasks {
		if t.Key.Equal(key) {
			return i, true
		}
	}
	return 0, false
}

func containsValue(values []engine.Value, v engine.Value) bool {
	for _, existing := range values {
		if existing.Equal(v) {
			return true
		}
	}
	return false
}
