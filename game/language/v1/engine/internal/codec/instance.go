package codec

import (
	"encoding/json"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
)

type workflowInstanceWire struct {
	Workflow           string                  `json:"workflow"`
	State              string                  `json:"state"`
	Parameters         []fieldValueWire        `json:"parameters,omitempty"`
	LocalState         json.RawMessage         `json:"local_state"`
	Outcome            json.RawMessage         `json:"outcome,omitempty"`
	QuestionSlots      []questionSlotWire      `json:"question_slots,omitempty"`
	AskGroupSlots      []askGroupSlotWire      `json:"ask_group_slots,omitempty"`
	TimerSlots         []timerSlotWire         `json:"timer_slots,omitempty"`
	KeyedQuestionSlots []keyedQuestionSlotWire `json:"keyed_question_slots,omitempty"`
	KeyedAskGroupSlots []keyedAskGroupSlotWire `json:"keyed_ask_group_slots,omitempty"`
	KeyedTimerSlots    []keyedTimerSlotWire    `json:"keyed_timer_slots,omitempty"`
}

type questionSlotWire struct {
	Name    string          `json:"name"`
	Pending json.RawMessage `json:"pending,omitempty"`
}

type pendingQuestionWire struct {
	Recipient     string           `json:"recipient"`
	Arguments     []fieldValueWire `json:"arguments,omitempty"`
	InteractionID uint64           `json:"interaction_id"`
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
	InteractionID  uint64                 `json:"interaction_id"`
}

type askGroupResponseWire struct {
	Respondent string          `json:"respondent"`
	Answer     json.RawMessage `json:"answer"`
}

type timerSlotWire struct {
	Name    string `json:"name"`
	Pending bool   `json:"pending,omitempty"`
}

type keyedQuestionSlotWire struct {
	Name    string                     `json:"name"`
	Pending []keyedPendingQuestionWire `json:"pending,omitempty"`
}

type keyedPendingQuestionWire struct {
	Key           json.RawMessage  `json:"key"`
	Recipient     string           `json:"recipient"`
	Arguments     []fieldValueWire `json:"arguments,omitempty"`
	InteractionID uint64           `json:"interaction_id"`
}

type keyedAskGroupSlotWire struct {
	Name    string                     `json:"name"`
	Pending []keyedPendingAskGroupWire `json:"pending,omitempty"`
}

type keyedPendingAskGroupWire struct {
	Key            json.RawMessage        `json:"key"`
	Recipients     []string               `json:"recipients,omitempty"`
	Arguments      []fieldValueWire       `json:"arguments,omitempty"`
	Responses      []askGroupResponseWire `json:"responses,omitempty"`
	Completed      bool                   `json:"completed,omitempty"`
	CompletionKind int                    `json:"completion_kind"`
	QuorumCount    int                    `json:"quorum_count,omitempty"`
	InteractionID  uint64                 `json:"interaction_id"`
}

type keyedTimerSlotWire struct {
	Name    string                  `json:"name"`
	Pending []keyedPendingTimerWire `json:"pending,omitempty"`
}

type keyedPendingTimerWire struct {
	Key json.RawMessage `json:"key"`
}

type runOutcomeWire struct {
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
		outcome, err = encodeRunOutcome(pathField(path, "outcome"), *instance.Outcome)
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

	keyedQuestionSlots, err := encodeKeyedQuestionSlots(path, instance.KeyedQuestionSlots)
	if err != nil {
		return nil, err
	}
	keyedAskGroupSlots, err := encodeKeyedAskGroupSlots(path, instance.KeyedAskGroupSlots)
	if err != nil {
		return nil, err
	}
	keyedTimerSlots, err := encodeKeyedTimerSlots(path, instance.KeyedTimerSlots)
	if err != nil {
		return nil, err
	}

	return json.Marshal(workflowInstanceWire{
		Workflow:           instance.Workflow,
		State:              instance.State,
		Parameters:         params,
		LocalState:         localState,
		Outcome:            outcome,
		QuestionSlots:      questionSlots,
		AskGroupSlots:      askGroupSlots,
		TimerSlots:         timerSlots,
		KeyedQuestionSlots: keyedQuestionSlots,
		KeyedAskGroupSlots: keyedAskGroupSlots,
		KeyedTimerSlots:    keyedTimerSlots,
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

	var outcome *engine.RunOutcome
	if !isEmptyOrNull(w.Outcome) {
		o, err := decodeRunOutcome(pathField(path, "outcome"), w.Outcome)
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

	keyedQuestionSlots, err := decodeKeyedQuestionSlots(path, w.KeyedQuestionSlots)
	if err != nil {
		return engine.WorkflowInstance{}, err
	}
	keyedAskGroupSlots, err := decodeKeyedAskGroupSlots(path, w.KeyedAskGroupSlots)
	if err != nil {
		return engine.WorkflowInstance{}, err
	}
	keyedTimerSlots, err := decodeKeyedTimerSlots(path, w.KeyedTimerSlots)
	if err != nil {
		return engine.WorkflowInstance{}, err
	}

	return engine.WorkflowInstance{
		Workflow:           w.Workflow,
		State:              w.State,
		Parameters:         nilIfEmpty(params),
		LocalState:         localRecord,
		Outcome:            outcome,
		QuestionSlots:      nilIfEmpty(questionSlots),
		AskGroupSlots:      nilIfEmpty(askGroupSlots),
		TimerSlots:         timerSlots,
		KeyedQuestionSlots: nilIfEmpty(keyedQuestionSlots),
		KeyedAskGroupSlots: nilIfEmpty(keyedAskGroupSlots),
		KeyedTimerSlots:    nilIfEmpty(keyedTimerSlots),
	}, nil
}

func encodeRunOutcome(path string, o engine.RunOutcome) (json.RawMessage, error) {
	result, err := EncodeValue(pathField(path, "result"), o.Result)
	if err != nil {
		return nil, err
	}
	return json.Marshal(runOutcomeWire{Kind: int(o.Kind), Result: result, Error: o.Error, Reason: o.Reason})
}

func decodeRunOutcome(path string, data json.RawMessage) (engine.RunOutcome, error) {
	var w runOutcomeWire
	if err := strictDecodeInto(path, data, &w); err != nil {
		return engine.RunOutcome{}, err
	}
	result, err := DecodeValue(pathField(path, "result"), w.Result)
	if err != nil {
		return engine.RunOutcome{}, err
	}
	return engine.RunOutcome{Kind: engine.RunOutcomeKind(w.Kind), Result: result, Error: w.Error, Reason: w.Reason}, nil
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
			raw, err := json.Marshal(pendingQuestionWire{Recipient: string(s.Pending.Recipient), Arguments: args, InteractionID: uint64(s.Pending.InteractionID)})
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
			pending = &engine.PendingQuestion{Recipient: engine.UserID(pw.Recipient), Arguments: args, InteractionID: engine.InteractionID(pw.InteractionID)}
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
		InteractionID: uint64(p.InteractionID),
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
		InteractionID: engine.InteractionID(w.InteractionID),
	}, nil
}

func encodeKeyedQuestionSlots(path string, slots []engine.KeyedQuestionSlotInstance) ([]keyedQuestionSlotWire, error) {
	result := make([]keyedQuestionSlotWire, len(slots))
	for i, s := range slots {
		spath := pathIndex(pathField(path, "keyed_question_slots"), i)
		pending := make([]keyedPendingQuestionWire, len(s.Pending))
		for j, p := range s.Pending {
			ppath := pathIndex(spath, j)
			key, err := EncodeValue(pathField(ppath, "key"), p.Key)
			if err != nil {
				return nil, err
			}
			args, err := encodeFieldValues(ppath, p.Arguments)
			if err != nil {
				return nil, err
			}
			pending[j] = keyedPendingQuestionWire{Key: key, Recipient: string(p.Recipient), Arguments: args, InteractionID: uint64(p.InteractionID)}
		}
		result[i] = keyedQuestionSlotWire{Name: s.Name, Pending: pending}
	}
	return result, nil
}

func decodeKeyedQuestionSlots(path string, wire []keyedQuestionSlotWire) ([]engine.KeyedQuestionSlotInstance, error) {
	result := make([]engine.KeyedQuestionSlotInstance, len(wire))
	for i, s := range wire {
		spath := pathIndex(pathField(path, "keyed_question_slots"), i)
		pending := make([]engine.KeyedPendingQuestion, len(s.Pending))
		for j, p := range s.Pending {
			ppath := pathIndex(spath, j)
			key, err := DecodeValue(pathField(ppath, "key"), p.Key)
			if err != nil {
				return nil, err
			}
			args, err := decodeFieldValues(ppath, p.Arguments)
			if err != nil {
				return nil, err
			}
			pending[j] = engine.KeyedPendingQuestion{Key: key, Recipient: engine.UserID(p.Recipient), Arguments: args, InteractionID: engine.InteractionID(p.InteractionID)}
		}
		result[i] = engine.KeyedQuestionSlotInstance{Name: s.Name, Pending: pending}
	}
	return result, nil
}

func encodeKeyedAskGroupSlots(path string, slots []engine.KeyedAskGroupSlotInstance) ([]keyedAskGroupSlotWire, error) {
	result := make([]keyedAskGroupSlotWire, len(slots))
	for i, s := range slots {
		spath := pathIndex(pathField(path, "keyed_ask_group_slots"), i)
		pending := make([]keyedPendingAskGroupWire, len(s.Pending))
		for j, p := range s.Pending {
			ppath := pathIndex(spath, j)
			key, err := EncodeValue(pathField(ppath, "key"), p.Key)
			if err != nil {
				return nil, err
			}
			raw, err := encodePendingAskGroup(ppath, p.PendingAskGroup)
			if err != nil {
				return nil, err
			}
			var base pendingAskGroupWire
			if err := json.Unmarshal(raw, &base); err != nil {
				return nil, err
			}
			pending[j] = keyedPendingAskGroupWire{
				Key: key, Recipients: base.Recipients, Arguments: base.Arguments, Responses: base.Responses,
				Completed: base.Completed, CompletionKind: base.CompletionKind, QuorumCount: base.QuorumCount,
				InteractionID: base.InteractionID,
			}
		}
		result[i] = keyedAskGroupSlotWire{Name: s.Name, Pending: pending}
	}
	return result, nil
}

func decodeKeyedAskGroupSlots(path string, wire []keyedAskGroupSlotWire) ([]engine.KeyedAskGroupSlotInstance, error) {
	result := make([]engine.KeyedAskGroupSlotInstance, len(wire))
	for i, s := range wire {
		spath := pathIndex(pathField(path, "keyed_ask_group_slots"), i)
		pending := make([]engine.KeyedPendingAskGroup, len(s.Pending))
		for j, p := range s.Pending {
			ppath := pathIndex(spath, j)
			key, err := DecodeValue(pathField(ppath, "key"), p.Key)
			if err != nil {
				return nil, err
			}
			raw, err := json.Marshal(pendingAskGroupWire{
				Recipients: p.Recipients, Arguments: p.Arguments, Responses: p.Responses,
				Completed: p.Completed, CompletionKind: p.CompletionKind, QuorumCount: p.QuorumCount,
				InteractionID: p.InteractionID,
			})
			if err != nil {
				return nil, err
			}
			base, err := decodePendingAskGroup(ppath, raw)
			if err != nil {
				return nil, err
			}
			pending[j] = engine.KeyedPendingAskGroup{Key: key, PendingAskGroup: base}
		}
		result[i] = engine.KeyedAskGroupSlotInstance{Name: s.Name, Pending: pending}
	}
	return result, nil
}

func encodeKeyedTimerSlots(path string, slots []engine.KeyedTimerSlotInstance) ([]keyedTimerSlotWire, error) {
	result := make([]keyedTimerSlotWire, len(slots))
	for i, s := range slots {
		spath := pathIndex(pathField(path, "keyed_timer_slots"), i)
		pending := make([]keyedPendingTimerWire, len(s.Pending))
		for j, p := range s.Pending {
			key, err := EncodeValue(pathField(pathIndex(spath, j), "key"), p.Key)
			if err != nil {
				return nil, err
			}
			pending[j] = keyedPendingTimerWire{Key: key}
		}
		result[i] = keyedTimerSlotWire{Name: s.Name, Pending: pending}
	}
	return result, nil
}

func decodeKeyedTimerSlots(path string, wire []keyedTimerSlotWire) ([]engine.KeyedTimerSlotInstance, error) {
	result := make([]engine.KeyedTimerSlotInstance, len(wire))
	for i, s := range wire {
		spath := pathIndex(pathField(path, "keyed_timer_slots"), i)
		pending := make([]engine.KeyedPendingTimer, len(s.Pending))
		for j, p := range s.Pending {
			key, err := DecodeValue(pathField(pathIndex(spath, j), "key"), p.Key)
			if err != nil {
				return nil, err
			}
			pending[j] = engine.KeyedPendingTimer{Key: key}
		}
		result[i] = engine.KeyedTimerSlotInstance{Name: s.Name, Pending: pending}
	}
	return result, nil
}
