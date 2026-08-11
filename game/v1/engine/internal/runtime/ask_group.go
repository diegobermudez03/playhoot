package runtime

import (
	"github.com/diegobermudez03/playhoot/game/v1/engine"
)

// execOpenAskGroup evaluates o.Recipients, o.Arguments, and o.Completion,
// then occupies the named ask-group slot in ctx's candidate instance
// state, producing one OpenQuestionOutput per recipient. Opening an
// already occupied slot, a Recipients list with a duplicate identity, or
// an invalid quorum fails the entire transition atomically — see
// program.OpenAskGroupOperation. A policy already satisfied by zero
// recipients (AskGroupAllResponsesPolicy's documented empty-list case)
// completes the group immediately, within this same operation.
func (ctx *execContext) execOpenAskGroup(o engine.OpenAskGroupOperation, scope engine.Scope) error {
	idx, ok := ctx.findAskGroupSlot(o.Slot)
	if !ok {
		return newExecutionError(ExecutionErrorUnknown, "engineservice: ask-group slot %q not found", o.Slot)
	}
	if ctx.askGroupSlots[idx].Pending != nil {
		return newExecutionError(ExecutionErrorSlotOccupied, "engineservice: ask-group slot %q is already occupied", o.Slot)
	}
	if err := ctx.checkActiveSlotLimit(); err != nil {
		return err
	}

	recipientsV, err := Evaluate(ctx.program, o.Recipients, scope)
	if err != nil {
		return err
	}
	elements := recipientsV.(engine.ListValue).Elements
	recipients := make([]engine.UserID, len(elements))
	seen := make(map[engine.UserID]bool, len(elements))
	for i, el := range elements {
		id := el.(engine.UserValue).ID
		if seen[id] {
			return newExecutionError(ExecutionErrorDuplicateRecipient,
				"engineservice: ask-group slot %q recipients contains a duplicate identity", o.Slot)
		}
		seen[id] = true
		recipients[i] = id
	}

	args, err := evalCallArguments(ctx, o.Arguments, scope)
	if err != nil {
		return err
	}

	kind, quorum, err := ctx.resolveAskGroupCompletion(o.Completion, len(recipients), scope)
	if err != nil {
		return err
	}

	pending := &engine.PendingAskGroup{
		Recipients:     recipients,
		Arguments:      args,
		CompletionKind: kind,
		QuorumCount:    quorum,
	}
	if askGroupPolicySatisfied(*pending) {
		pending.Completed = true
	}
	ctx.askGroupSlots[idx] = engine.AskGroupSlotInstance{Name: o.Slot, Pending: pending}

	slotDecl, _ := ctx.askGroupSlotDeclaration(o.Slot)
	for _, r := range recipients {
		ctx.outputs = append(ctx.outputs, engine.OpenQuestionOutput{Slot: o.Slot, Recipient: r, Question: slotDecl.Question, Arguments: args})
	}
	return nil
}

// resolveAskGroupCompletion evaluates policy into its durable runtime
// form: an engine.AskGroupCompletionKind and, for AskGroupQuorumPolicy,
// the evaluated quorum count. It fails if a quorum count does not
// evaluate to a positive integer no greater than recipientCount, or if
// an AskGroupFirstResponsePolicy is given zero recipients — both per
// program's documented rules.
func (ctx *execContext) resolveAskGroupCompletion(policy engine.AskGroupCompletionPolicy, recipientCount int, scope engine.Scope) (engine.AskGroupCompletionKind, int, error) {
	switch p := policy.(type) {
	case engine.AskGroupAllResponsesPolicy:
		return engine.AskGroupCompletionAllResponses, 0, nil

	case engine.AskGroupFirstResponsePolicy:
		if recipientCount == 0 {
			return 0, 0, newExecutionError(ExecutionErrorInvalidQuorum,
				"engineservice: a first-response ask group cannot be opened with no recipients")
		}
		return engine.AskGroupCompletionFirstResponse, 0, nil

	case engine.AskGroupQuorumPolicy:
		v, err := Evaluate(ctx.program, p.Count, scope)
		if err != nil {
			return 0, 0, err
		}
		count, ok := intIndex(v.(engine.NumberValue).Value)
		if !ok || count <= 0 || count > recipientCount {
			return 0, 0, newExecutionError(ExecutionErrorInvalidQuorum,
				"engineservice: quorum count %v is invalid for %d recipients", v.(engine.NumberValue).Value, recipientCount)
		}
		return engine.AskGroupCompletionQuorum, count, nil

	default:
		return 0, 0, newExecutionError(ExecutionErrorUnknown, "engineservice: unsupported ask-group completion policy %T", policy)
	}
}

// execFinalizeAskGroup forces the named, currently collecting ask-group
// slot to complete using only its accepted responses so far, producing
// one CloseQuestionOutput per recipient left without an accepted
// answer. Finalizing a slot already completed-awaiting-join is an
// idempotent no-op — see program.FinalizeAskGroupOperation's documented
// answer-versus-deadline race resolution. Finalizing an empty slot is
// an execution error.
func (ctx *execContext) execFinalizeAskGroup(o engine.FinalizeAskGroupOperation) error {
	idx, ok := ctx.findAskGroupSlot(o.Slot)
	if !ok {
		return newExecutionError(ExecutionErrorUnknown, "engineservice: ask-group slot %q not found", o.Slot)
	}
	pending := ctx.askGroupSlots[idx].Pending
	if pending == nil {
		return newExecutionError(ExecutionErrorUnknown, "engineservice: ask-group slot %q is empty", o.Slot)
	}
	if pending.Completed {
		return nil
	}

	updated := *pending
	updated.Completed = true
	ctx.askGroupSlots[idx] = engine.AskGroupSlotInstance{Name: o.Slot, Pending: &updated}
	for _, r := range missingAskGroupRecipients(updated) {
		ctx.outputs = append(ctx.outputs, engine.CloseQuestionOutput{Slot: o.Slot, Recipient: r})
	}
	return nil
}

// execCancelAskGroup abandons the named, currently collecting ask-group
// slot: every accepted response so far is discarded, the slot is
// cleared, and one CloseQuestionOutput is produced per recipient who
// never had an accepted answer — no completion signal is ever produced.
// Cancelling an already empty slot is an idempotent no-op. Cancelling a
// slot that holds a terminal outcome still awaiting join is an
// execution error — see program.CancelAskGroupOperation.
func (ctx *execContext) execCancelAskGroup(o engine.CancelAskGroupOperation) error {
	idx, ok := ctx.findAskGroupSlot(o.Slot)
	if !ok {
		return newExecutionError(ExecutionErrorUnknown, "engineservice: ask-group slot %q not found", o.Slot)
	}
	pending := ctx.askGroupSlots[idx].Pending
	if pending == nil {
		return nil
	}
	if pending.Completed {
		return newExecutionError(ExecutionErrorAskGroupNotJoined,
			"engineservice: ask-group slot %q holds a terminal outcome that must be joined before it can be cancelled", o.Slot)
	}

	for _, r := range missingAskGroupRecipients(*pending) {
		ctx.outputs = append(ctx.outputs, engine.CloseQuestionOutput{Slot: o.Slot, Recipient: r})
	}
	ctx.askGroupSlots[idx] = engine.AskGroupSlotInstance{Name: o.Slot}
	return nil
}

// askGroupPolicySatisfied reports whether pending's CompletionKind is
// satisfied by its current Responses — see engine.AskGroupCompletionKind.
func askGroupPolicySatisfied(pending engine.PendingAskGroup) bool {
	switch pending.CompletionKind {
	case engine.AskGroupCompletionFirstResponse:
		return len(pending.Responses) >= 1
	case engine.AskGroupCompletionQuorum:
		return len(pending.Responses) >= pending.QuorumCount
	default:
		return len(pending.Responses) >= len(pending.Recipients)
	}
}

// missingAskGroupRecipients returns pending.Recipients who have no entry
// in pending.Responses, in original recipient-list order — see
// program.AskGroupCompletedSignalSource's documented "missing" field.
func missingAskGroupRecipients(pending engine.PendingAskGroup) []engine.UserID {
	responded := make(map[engine.UserID]bool, len(pending.Responses))
	for _, r := range pending.Responses {
		responded[r.Respondent] = true
	}
	missing := make([]engine.UserID, 0, len(pending.Recipients)-len(pending.Responses))
	for _, r := range pending.Recipients {
		if !responded[r] {
			missing = append(missing, r)
		}
	}
	return missing
}

// stepAskGroupAnswer implements SignalKindAskGroupAnswered's documented
// behavior: unlike every other Step path, an accepted answer never
// selects or runs a transition — it only records the answer against the
// targeted ask-group slot's PendingAskGroup and re-evaluates its
// completion policy, as one atomic Commit. A rejected answer (stale
// slot, unauthorized or duplicate respondent, or an invalid value)
// returns ErrInputRejected and leaves snapshot unchanged, exactly like
// any other rejected input.
func stepAskGroupAnswer(p engine.Program, snapshot engine.Snapshot, signal engine.Signal) (engine.Commit, error) {
	target, ok := resolveInstance(snapshot.Root, signal.Path)
	if !ok || target.Outcome != nil {
		return engine.Commit{}, ErrSignalRejected
	}
	workflow, ok := p.Workflows[target.Workflow]
	if !ok {
		return engine.Commit{}, newExecutionError(ExecutionErrorUnknown, "engineservice: workflow %q is not compiled", target.Workflow)
	}

	idx, ok := findInstanceAskGroupSlotIndex(target, signal.Slot)
	if !ok {
		return engine.Commit{}, newExecutionError(ExecutionErrorUnknown, "engineservice: workflow %q has no ask-group slot named %q", target.Workflow, signal.Slot)
	}
	pending := target.AskGroupSlots[idx].Pending
	if pending == nil || pending.Completed {
		return engine.Commit{}, ErrInputRejected
	}
	if !containsUserID(pending.Recipients, signal.Respondent) || hasAskGroupResponse(pending.Responses, signal.Respondent) {
		return engine.Commit{}, ErrInputRejected
	}

	slotDecl, ok := workflowAskGroupSlot(workflow, signal.Slot)
	if !ok {
		return engine.Commit{}, newExecutionError(ExecutionErrorUnknown, "engineservice: workflow %q has no ask-group slot named %q", target.Workflow, signal.Slot)
	}
	question, ok := p.Questions[slotDecl.Question]
	if !ok {
		return engine.Commit{}, newExecutionError(ExecutionErrorUnknown, "engineservice: question %q is not compiled", slotDecl.Question)
	}
	if signal.Answer == nil || !signal.Answer.Validate(question.ResponseType) {
		return engine.Commit{}, ErrInputRejected
	}
	if question.Validation != nil {
		bindings := map[string]engine.Value{"respondent": engine.UserValue{ID: signal.Respondent}, "answer": signal.Answer}
		for _, arg := range pending.Arguments {
			bindings[arg.Name] = arg.Value
		}
		v, err := Evaluate(p, question.Validation, engine.Scope{Bindings: bindings})
		if err != nil {
			return engine.Commit{}, err
		}
		if !v.(engine.BoolValue).Value {
			return engine.Commit{}, ErrInputRejected
		}
	}

	updated := *pending
	updated.Responses = append(append([]engine.AskGroupResponse{}, pending.Responses...), engine.AskGroupResponse{Respondent: signal.Respondent, Answer: signal.Answer})

	var outputs []engine.Output
	if askGroupPolicySatisfied(updated) {
		updated.Completed = true
		for _, r := range missingAskGroupRecipients(updated) {
			outputs = append(outputs, engine.CloseQuestionOutput{Slot: signal.Slot, Recipient: r})
		}
	}

	newSlots := append([]engine.AskGroupSlotInstance{}, target.AskGroupSlots...)
	newSlots[idx] = engine.AskGroupSlotInstance{Name: signal.Slot, Pending: &updated}
	newTarget := target
	newTarget.AskGroupSlots = newSlots

	newRoot, err := applyInstancePath(snapshot.Root, signal.Path, newTarget)
	if err != nil {
		return engine.Commit{}, err
	}

	return engine.Commit{
		Snapshot: engine.Snapshot{
			GlobalState: snapshot.GlobalState,
			Root:        newRoot,
			Random:      snapshot.Random,
			Sequence:    snapshot.Sequence + 1,
		},
		Outputs: outputs,
		Trace: engine.Trace{
			Path:        signal.Path,
			Workflow:    target.Workflow,
			StateBefore: target.State,
			StateAfter:  target.State,
			Outputs:     outputs,
		},
		ConsumedSignal: signal,
	}, nil
}

// validateAskGroupCompletion implements program.AskGroupCompletedSignalSource's
// acceptance rule: the named ask-group slot on instance must currently
// hold a terminal outcome awaiting join. This doubles as the
// "duplicate delivery after joining" check, since an accepted
// completion signal clears its slot atomically with the rest of the
// step that handles it.
func validateAskGroupCompletion(instance engine.WorkflowInstance, signal engine.Signal) error {
	slot, ok := findInstanceAskGroupSlot(instance, signal.Slot)
	if !ok || slot.Pending == nil || !slot.Pending.Completed {
		return ErrInputRejected
	}
	return nil
}

// askGroupCompletionFields builds AskGroupCompletedSignalSource's
// "responses", "respondents", and "missing" fields from pending's
// durable data, reading the question's declared ResponseType from
// slotName's declaration on workflow.
func askGroupCompletionFields(p engine.Program, workflow engine.Workflow, pending *engine.PendingAskGroup, slotName string) map[string]engine.Value {
	var responseType engine.Type
	if slotDecl, ok := workflowAskGroupSlot(workflow, slotName); ok {
		if question, ok := p.Questions[slotDecl.Question]; ok {
			responseType = question.ResponseType
		}
	}

	entries := make([]engine.MapEntry, 0, len(pending.Responses))
	respondents := make([]engine.Value, 0, len(pending.Responses))
	for _, r := range pending.Responses {
		entries = append(entries, engine.MapEntry{Key: engine.UserValue{ID: r.Respondent}, Value: r.Answer})
		respondents = append(respondents, engine.UserValue{ID: r.Respondent})
	}
	missingIDs := missingAskGroupRecipients(*pending)
	missing := make([]engine.Value, len(missingIDs))
	for i, m := range missingIDs {
		missing[i] = engine.UserValue{ID: m}
	}

	return map[string]engine.Value{
		"responses":   engine.MapValue{KeyType: engine.UserType{}, ValueType: responseType, Entries: entries},
		"respondents": engine.ListValue{ElementType: engine.UserType{}, Elements: respondents},
		"missing":     engine.ListValue{ElementType: engine.UserType{}, Elements: missing},
	}
}

func findInstanceAskGroupSlot(instance engine.WorkflowInstance, name string) (engine.AskGroupSlotInstance, bool) {
	for _, s := range instance.AskGroupSlots {
		if s.Name == name {
			return s, true
		}
	}
	return engine.AskGroupSlotInstance{}, false
}

func findInstanceAskGroupSlotIndex(instance engine.WorkflowInstance, name string) (int, bool) {
	for i, s := range instance.AskGroupSlots {
		if s.Name == name {
			return i, true
		}
	}
	return 0, false
}

func workflowAskGroupSlot(workflow engine.Workflow, name string) (engine.AskGroupSlot, bool) {
	for _, s := range workflow.AskGroupSlots {
		if s.Name == name {
			return s, true
		}
	}
	return engine.AskGroupSlot{}, false
}

func containsUserID(ids []engine.UserID, id engine.UserID) bool {
	for _, i := range ids {
		if i == id {
			return true
		}
	}
	return false
}

func hasAskGroupResponse(responses []engine.AskGroupResponse, respondent engine.UserID) bool {
	for _, r := range responses {
		if r.Respondent == respondent {
			return true
		}
	}
	return false
}
