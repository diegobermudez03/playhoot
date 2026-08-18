package codec

import (
	"encoding/json"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
)

type workflowInstanceWire struct {
	Workflow       string              `json:"workflow"`
	State          string              `json:"state"`
	Parameters     []fieldValueWire    `json:"parameters,omitempty"`
	LocalState     json.RawMessage     `json:"local_state"`
	Outcome        json.RawMessage     `json:"outcome,omitempty"`
	QuestionSlots  []questionSlotWire  `json:"question_slots,omitempty"`
	AskGroupSlots  []askGroupSlotWire  `json:"ask_group_slots,omitempty"`
	TimerSlots     []timerSlotWire     `json:"timer_slots,omitempty"`
	ChildSlots     []childSlotWire     `json:"child_slots,omitempty"`
	TaskGroupSlots []taskGroupSlotWire `json:"task_group_slots,omitempty"`
}

type questionSlotWire struct {
	Name    string          `json:"name"`
	Pending json.RawMessage `json:"pending,omitempty"`
}

type pendingQuestionWire struct {
	Recipient string           `json:"recipient"`
	Arguments []fieldValueWire `json:"arguments,omitempty"`
}

type askGroupSlotWire struct {
	Name    string          `json:"name"`
	Pending json.RawMessage `json:"pending,omitempty"`
}

type pendingAskGroupWire struct {
	Recipients     []string               `json:"recipients,omitempty"`
	Arguments      []fieldValueWire       `json:"arguments,omitempty"`
	Responses      []askGroupResponseWire `json:"responses,omitempty"`
	Completed      bool                   `json:"completed,omitempty"`
	CompletionKind int                    `json:"completion_kind"`
	QuorumCount    int                    `json:"quorum_count,omitempty"`
}

type askGroupResponseWire struct {
	Respondent string          `json:"respondent"`
	Answer     json.RawMessage `json:"answer"`
}

type timerSlotWire struct {
	Name    string `json:"name"`
	Pending bool   `json:"pending,omitempty"`
}

type childSlotWire struct {
	Name  string          `json:"name"`
	Child json.RawMessage `json:"child,omitempty"`
}

type taskGroupSlotWire struct {
	Name  string          `json:"name"`
	Group json.RawMessage `json:"group,omitempty"`
}

type taskGroupStateWire struct {
	Tasks          []taskGroupTaskWire `json:"tasks,omitempty"`
	Phase          int                 `json:"phase"`
	CompletionKind int                 `json:"completion_kind"`
	QuorumCount    int                 `json:"quorum_count,omitempty"`
	TerminalOrder  []json.RawMessage   `json:"terminal_order,omitempty"`
}

type taskGroupTaskWire struct {
	Key   json.RawMessage `json:"key"`
	Child json.RawMessage `json:"child"`
}

type workflowOutcomeWire struct {
	Kind   int             `json:"kind"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
	Reason string          `json:"reason,omitempty"`
}

// EncodeWorkflowInstance encodes instance — the recursive edge of a
// Snapshot's child-workflow tree, through ChildSlots and TaskGroupSlots.
func EncodeWorkflowInstance(path string, instance engine.WorkflowInstance) (json.RawMessage, error) {
	params, err := encodeFieldValues(path, instance.Parameters)
	if err != nil {
		return nil, err
	}
	localState, err := EncodeValue(pathField(path, "local_state"), instance.LocalState)
	if err != nil {
		return nil, err
	}

	var outcome json.RawMessage
	if instance.Outcome != nil {
		outcome, err = encodeWorkflowOutcome(pathField(path, "outcome"), *instance.Outcome)
		if err != nil {
			return nil, err
		}
	}

	questionSlots, err := encodeQuestionSlots(path, instance.QuestionSlots)
	if err != nil {
		return nil, err
	}
	askGroupSlots, err := encodeAskGroupSlots(path, instance.AskGroupSlots)
	if err != nil {
		return nil, err
	}
	timerSlots := make([]timerSlotWire, len(instance.TimerSlots))
	for i, s := range instance.TimerSlots {
		timerSlots[i] = timerSlotWire{Name: s.Name, Pending: s.Pending}
	}
	childSlots, err := encodeChildSlots(path, instance.ChildSlots)
	if err != nil {
		return nil, err
	}
	taskGroupSlots, err := encodeTaskGroupSlots(path, instance.TaskGroupSlots)
	if err != nil {
		return nil, err
	}

	return json.Marshal(workflowInstanceWire{
		Workflow:       instance.Workflow,
		State:          instance.State,
		Parameters:     params,
		LocalState:     localState,
		Outcome:        outcome,
		QuestionSlots:  questionSlots,
		AskGroupSlots:  askGroupSlots,
		TimerSlots:     timerSlots,
		ChildSlots:     childSlots,
		TaskGroupSlots: taskGroupSlots,
	})
}

// DecodeWorkflowInstance decodes data into an engine.WorkflowInstance.
func DecodeWorkflowInstance(path string, data json.RawMessage) (engine.WorkflowInstance, error) {
	var w workflowInstanceWire
	if err := strictDecodeInto(path, data, &w); err != nil {
		return engine.WorkflowInstance{}, err
	}

	params, err := decodeFieldValues(path, w.Parameters)
	if err != nil {
		return engine.WorkflowInstance{}, err
	}
	localState, err := DecodeValue(pathField(path, "local_state"), w.LocalState)
	if err != nil {
		return engine.WorkflowInstance{}, err
	}
	localRecord, _ := localState.(engine.RecordValue)

	var outcome *engine.WorkflowOutcome
	if !isEmptyOrNull(w.Outcome) {
		o, err := decodeWorkflowOutcome(pathField(path, "outcome"), w.Outcome)
		if err != nil {
			return engine.WorkflowInstance{}, err
		}
		outcome = &o
	}

	questionSlots, err := decodeQuestionSlots(path, w.QuestionSlots)
	if err != nil {
		return engine.WorkflowInstance{}, err
	}
	askGroupSlots, err := decodeAskGroupSlots(path, w.AskGroupSlots)
	if err != nil {
		return engine.WorkflowInstance{}, err
	}
	var timerSlots []engine.TimerSlotInstance
	for _, s := range w.TimerSlots {
		timerSlots = append(timerSlots, engine.TimerSlotInstance{Name: s.Name, Pending: s.Pending})
	}
	childSlots, err := decodeChildSlots(path, w.ChildSlots)
	if err != nil {
		return engine.WorkflowInstance{}, err
	}
	taskGroupSlots, err := decodeTaskGroupSlots(path, w.TaskGroupSlots)
	if err != nil {
		return engine.WorkflowInstance{}, err
	}

	return engine.WorkflowInstance{
		Workflow:       w.Workflow,
		State:          w.State,
		Parameters:     nilIfEmpty(params),
		LocalState:     localRecord,
		Outcome:        outcome,
		QuestionSlots:  nilIfEmpty(questionSlots),
		AskGroupSlots:  nilIfEmpty(askGroupSlots),
		TimerSlots:     timerSlots,
		ChildSlots:     nilIfEmpty(childSlots),
		TaskGroupSlots: nilIfEmpty(taskGroupSlots),
	}, nil
}

func encodeWorkflowOutcome(path string, o engine.WorkflowOutcome) (json.RawMessage, error) {
	result, err := EncodeValue(pathField(path, "result"), o.Result)
	if err != nil {
		return nil, err
	}
	return json.Marshal(workflowOutcomeWire{Kind: int(o.Kind), Result: result, Error: o.Error, Reason: o.Reason})
}

func decodeWorkflowOutcome(path string, data json.RawMessage) (engine.WorkflowOutcome, error) {
	var w workflowOutcomeWire
	if err := strictDecodeInto(path, data, &w); err != nil {
		return engine.WorkflowOutcome{}, err
	}
	result, err := DecodeValue(pathField(path, "result"), w.Result)
	if err != nil {
		return engine.WorkflowOutcome{}, err
	}
	return engine.WorkflowOutcome{Kind: engine.WorkflowOutcomeKind(w.Kind), Result: result, Error: w.Error, Reason: w.Reason}, nil
}

func encodeQuestionSlots(path string, slots []engine.QuestionSlotInstance) ([]questionSlotWire, error) {
	result := make([]questionSlotWire, len(slots))
	for i, s := range slots {
		spath := pathIndex(pathField(path, "question_slots"), i)
		var pending json.RawMessage
		if s.Pending != nil {
			args, err := encodeFieldValues(spath, s.Pending.Arguments)
			if err != nil {
				return nil, err
			}
			raw, err := json.Marshal(pendingQuestionWire{Recipient: string(s.Pending.Recipient), Arguments: args})
			if err != nil {
				return nil, err
			}
			pending = raw
		}
		result[i] = questionSlotWire{Name: s.Name, Pending: pending}
	}
	return result, nil
}

func decodeQuestionSlots(path string, wire []questionSlotWire) ([]engine.QuestionSlotInstance, error) {
	result := make([]engine.QuestionSlotInstance, len(wire))
	for i, s := range wire {
		spath := pathIndex(pathField(path, "question_slots"), i)
		var pending *engine.PendingQuestion
		if !isEmptyOrNull(s.Pending) {
			var pw pendingQuestionWire
			if err := strictDecodeInto(spath, s.Pending, &pw); err != nil {
				return nil, err
			}
			args, err := decodeFieldValues(spath, pw.Arguments)
			if err != nil {
				return nil, err
			}
			pending = &engine.PendingQuestion{Recipient: engine.UserID(pw.Recipient), Arguments: args}
		}
		result[i] = engine.QuestionSlotInstance{Name: s.Name, Pending: pending}
	}
	return result, nil
}

func encodeAskGroupSlots(path string, slots []engine.AskGroupSlotInstance) ([]askGroupSlotWire, error) {
	result := make([]askGroupSlotWire, len(slots))
	for i, s := range slots {
		spath := pathIndex(pathField(path, "ask_group_slots"), i)
		var pending json.RawMessage
		if s.Pending != nil {
			raw, err := encodePendingAskGroup(spath, *s.Pending)
			if err != nil {
				return nil, err
			}
			pending = raw
		}
		result[i] = askGroupSlotWire{Name: s.Name, Pending: pending}
	}
	return result, nil
}

func encodePendingAskGroup(path string, p engine.PendingAskGroup) (json.RawMessage, error) {
	args, err := encodeFieldValues(path, p.Arguments)
	if err != nil {
		return nil, err
	}
	recipients := make([]string, len(p.Recipients))
	for i, r := range p.Recipients {
		recipients[i] = string(r)
	}
	responses := make([]askGroupResponseWire, len(p.Responses))
	for i, r := range p.Responses {
		answer, err := EncodeValue(pathIndex(pathField(path, "responses"), i), r.Answer)
		if err != nil {
			return nil, err
		}
		responses[i] = askGroupResponseWire{Respondent: string(r.Respondent), Answer: answer}
	}
	return json.Marshal(pendingAskGroupWire{
		Recipients: recipients, Arguments: args, Responses: responses,
		Completed: p.Completed, CompletionKind: int(p.CompletionKind), QuorumCount: p.QuorumCount,
	})
}

func decodeAskGroupSlots(path string, wire []askGroupSlotWire) ([]engine.AskGroupSlotInstance, error) {
	result := make([]engine.AskGroupSlotInstance, len(wire))
	for i, s := range wire {
		spath := pathIndex(pathField(path, "ask_group_slots"), i)
		var pending *engine.PendingAskGroup
		if !isEmptyOrNull(s.Pending) {
			p, err := decodePendingAskGroup(spath, s.Pending)
			if err != nil {
				return nil, err
			}
			pending = &p
		}
		result[i] = engine.AskGroupSlotInstance{Name: s.Name, Pending: pending}
	}
	return result, nil
}

func decodePendingAskGroup(path string, data json.RawMessage) (engine.PendingAskGroup, error) {
	var w pendingAskGroupWire
	if err := strictDecodeInto(path, data, &w); err != nil {
		return engine.PendingAskGroup{}, err
	}
	args, err := decodeFieldValues(path, w.Arguments)
	if err != nil {
		return engine.PendingAskGroup{}, err
	}
	var recipients []engine.UserID
	for _, r := range w.Recipients {
		recipients = append(recipients, engine.UserID(r))
	}
	var responses []engine.AskGroupResponse
	for i, r := range w.Responses {
		answer, err := DecodeValue(pathIndex(pathField(path, "responses"), i), r.Answer)
		if err != nil {
			return engine.PendingAskGroup{}, err
		}
		responses = append(responses, engine.AskGroupResponse{Respondent: engine.UserID(r.Respondent), Answer: answer})
	}
	return engine.PendingAskGroup{
		Recipients: recipients, Arguments: nilIfEmpty(args), Responses: responses,
		Completed: w.Completed, CompletionKind: engine.AskGroupCompletionKind(w.CompletionKind), QuorumCount: w.QuorumCount,
	}, nil
}

func encodeChildSlots(path string, slots []engine.ChildWorkflowSlotInstance) ([]childSlotWire, error) {
	result := make([]childSlotWire, len(slots))
	for i, s := range slots {
		var child json.RawMessage
		if s.Child != nil {
			raw, err := EncodeWorkflowInstance(pathIndex(pathField(path, "child_slots"), i), *s.Child)
			if err != nil {
				return nil, err
			}
			child = raw
		}
		result[i] = childSlotWire{Name: s.Name, Child: child}
	}
	return result, nil
}

func decodeChildSlots(path string, wire []childSlotWire) ([]engine.ChildWorkflowSlotInstance, error) {
	result := make([]engine.ChildWorkflowSlotInstance, len(wire))
	for i, s := range wire {
		var child *engine.WorkflowInstance
		if !isEmptyOrNull(s.Child) {
			c, err := DecodeWorkflowInstance(pathIndex(pathField(path, "child_slots"), i), s.Child)
			if err != nil {
				return nil, err
			}
			child = &c
		}
		result[i] = engine.ChildWorkflowSlotInstance{Name: s.Name, Child: child}
	}
	return result, nil
}

func encodeTaskGroupSlots(path string, slots []engine.TaskGroupSlotInstance) ([]taskGroupSlotWire, error) {
	result := make([]taskGroupSlotWire, len(slots))
	for i, s := range slots {
		spath := pathIndex(pathField(path, "task_group_slots"), i)
		var group json.RawMessage
		if s.Group != nil {
			raw, err := encodeTaskGroupState(spath, *s.Group)
			if err != nil {
				return nil, err
			}
			group = raw
		}
		result[i] = taskGroupSlotWire{Name: s.Name, Group: group}
	}
	return result, nil
}

func encodeTaskGroupState(path string, g engine.TaskGroupState) (json.RawMessage, error) {
	tasks := make([]taskGroupTaskWire, len(g.Tasks))
	for i, t := range g.Tasks {
		tpath := pathIndex(pathField(path, "tasks"), i)
		key, err := EncodeValue(pathField(tpath, "key"), t.Key)
		if err != nil {
			return nil, err
		}
		child, err := EncodeWorkflowInstance(pathField(tpath, "child"), t.Child)
		if err != nil {
			return nil, err
		}
		tasks[i] = taskGroupTaskWire{Key: key, Child: child}
	}
	terminalOrder := make([]json.RawMessage, len(g.TerminalOrder))
	for i, v := range g.TerminalOrder {
		raw, err := EncodeValue(pathIndex(pathField(path, "terminal_order"), i), v)
		if err != nil {
			return nil, err
		}
		terminalOrder[i] = raw
	}
	return json.Marshal(taskGroupStateWire{
		Tasks: tasks, Phase: int(g.Phase), CompletionKind: int(g.CompletionKind),
		QuorumCount: g.QuorumCount, TerminalOrder: terminalOrder,
	})
}

func decodeTaskGroupSlots(path string, wire []taskGroupSlotWire) ([]engine.TaskGroupSlotInstance, error) {
	result := make([]engine.TaskGroupSlotInstance, len(wire))
	for i, s := range wire {
		spath := pathIndex(pathField(path, "task_group_slots"), i)
		var group *engine.TaskGroupState
		if !isEmptyOrNull(s.Group) {
			g, err := decodeTaskGroupState(spath, s.Group)
			if err != nil {
				return nil, err
			}
			group = &g
		}
		result[i] = engine.TaskGroupSlotInstance{Name: s.Name, Group: group}
	}
	return result, nil
}

func decodeTaskGroupState(path string, data json.RawMessage) (engine.TaskGroupState, error) {
	var w taskGroupStateWire
	if err := strictDecodeInto(path, data, &w); err != nil {
		return engine.TaskGroupState{}, err
	}
	tasks := make([]engine.TaskGroupTask, len(w.Tasks))
	for i, t := range w.Tasks {
		tpath := pathIndex(pathField(path, "tasks"), i)
		key, err := DecodeValue(pathField(tpath, "key"), t.Key)
		if err != nil {
			return engine.TaskGroupState{}, err
		}
		child, err := DecodeWorkflowInstance(pathField(tpath, "child"), t.Child)
		if err != nil {
			return engine.TaskGroupState{}, err
		}
		tasks[i] = engine.TaskGroupTask{Key: key, Child: child}
	}
	terminalOrder := make([]engine.Value, len(w.TerminalOrder))
	for i, raw := range w.TerminalOrder {
		v, err := DecodeValue(pathIndex(pathField(path, "terminal_order"), i), raw)
		if err != nil {
			return engine.TaskGroupState{}, err
		}
		terminalOrder[i] = v
	}
	return engine.TaskGroupState{
		Tasks: nilIfEmpty(tasks), Phase: engine.TaskGroupPhase(w.Phase), CompletionKind: engine.TaskGroupCompletionKind(w.CompletionKind),
		QuorumCount: w.QuorumCount, TerminalOrder: nilIfEmpty(terminalOrder),
	}, nil
}
