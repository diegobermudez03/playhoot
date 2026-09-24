package codec

import (
	"encoding/json"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
)

type workflowInstanceWire struct {
	Workflow      string             `json:"workflow"`
	State         string             `json:"state"`
	Parameters    []fieldValueWire   `json:"parameters,omitempty"`
	LocalState    json.RawMessage    `json:"local_state"`
	Outcome       json.RawMessage    `json:"outcome,omitempty"`
	QuestionSlots []questionSlotWire `json:"question_slots,omitempty"`
	AskGroupSlots []askGroupSlotWire `json:"ask_group_slots,omitempty"`
	TimerSlots    []timerSlotWire    `json:"timer_slots,omitempty"`
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

type workflowOutcomeWire struct {
	Kind   int             `json:"kind"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
	Reason string          `json:"reason,omitempty"`
}

// EncodeWorkflowInstance encodes instance — the one workflow instance a
// Session runs for its entire lifetime.
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

	return json.Marshal(workflowInstanceWire{
		Workflow:      instance.Workflow,
		State:         instance.State,
		Parameters:    params,
		LocalState:    localState,
		Outcome:       outcome,
		QuestionSlots: questionSlots,
		AskGroupSlots: askGroupSlots,
		TimerSlots:    timerSlots,
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

	return engine.WorkflowInstance{
		Workflow:      w.Workflow,
		State:         w.State,
		Parameters:    nilIfEmpty(params),
		LocalState:    localRecord,
		Outcome:       outcome,
		QuestionSlots: nilIfEmpty(questionSlots),
		AskGroupSlots: nilIfEmpty(askGroupSlots),
		TimerSlots:    timerSlots,
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
