package codec

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/program"
)

// --- keyed workflow-owned slot declarations ---

func TestKeyedQuestionSlotDeclaration_RoundTrip(t *testing.T) {
	cases := []program.KeyedQuestionSlotDeclaration{
		{
			Name:     "quiz",
			Question: "ChooseAnswer",
			KeyType:  program.BuiltinTypeReference{Type: program.BuiltinTypeString},
			Presentation: &program.QuestionPresentationDeclaration{
				Slot:       "primaryInteraction",
				Projection: "QuizProjection",
				View:       "QuizView",
			},
		},
		{Name: "headless", Question: "Confirm", KeyType: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}, Presentation: nil},
	}
	for i, original := range cases {
		raw, err := encodeKeyedQuestionSlotDeclaration("$", original)
		if err != nil {
			t.Fatalf("case %d: encode: %v", i, err)
		}
		decoded, err := decodeKeyedQuestionSlotDeclaration("$", raw)
		if err != nil {
			t.Fatalf("case %d: decode: %v", i, err)
		}
		if !reflect.DeepEqual(original, decoded) {
			t.Fatalf("case %d: round trip mismatch:\n  original = %#v\n  decoded  = %#v", i, original, decoded)
		}
	}
}

func TestDecode_KeyedQuestionSlotDeclaration_Null(t *testing.T) {
	_, err := decodeKeyedQuestionSlotDeclaration("$", json.RawMessage("null"))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

func TestKeyedAskGroupSlotDeclaration_RoundTrip(t *testing.T) {
	original := program.KeyedAskGroupSlotDeclaration{
		Name:     "teamVotes",
		Question: "ChooseCard",
		KeyType:  program.BuiltinTypeReference{Type: program.BuiltinTypeString},
		Presentation: &program.QuestionPresentationDeclaration{
			Slot:       "primaryInteraction",
			Projection: "ChooseCardProjection",
			ProjectionArguments: []program.CallArgument{
				{Name: "options", Value: program.ReferenceExpression{Name: "options"}},
			},
			View: "ChooseCardView",
		},
	}
	raw, err := encodeKeyedAskGroupSlotDeclaration("$", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeKeyedAskGroupSlotDeclaration("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
	}
}

func TestKeyedTimerSlotDeclaration_RoundTrip(t *testing.T) {
	original := program.KeyedTimerSlotDeclaration{Name: "disconnectTimeout", KeyType: program.BuiltinTypeReference{Type: program.BuiltinTypeString}}
	raw, err := encodeKeyedTimerSlotDeclaration("$", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeKeyedTimerSlotDeclaration("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
	}
}

func TestDecode_KeyedTimerSlotDeclaration_Null(t *testing.T) {
	_, err := decodeKeyedTimerSlotDeclaration("$", json.RawMessage("null"))
	if err == nil {
		t.Fatal("expected an error")
	}
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

// --- keyed operations ---

func TestKeyedQuestionOperations_RoundTrip(t *testing.T) {
	openOriginal := program.OpenKeyedQuestionOperation{
		Slot:      "quiz",
		Key:       program.ReferenceExpression{Name: "playerID"},
		Recipient: program.ReferenceExpression{Name: "player"},
		Arguments: []program.CallArgument{{Name: "prompt", Value: program.StringLiteralExpression{Value: "Ready?"}}},
	}
	raw, err := encodeOperation("$", openOriginal)
	if err != nil {
		t.Fatalf("encode open: %v", err)
	}
	decoded, err := decodeOperation("$", raw)
	if err != nil {
		t.Fatalf("decode open: %v", err)
	}
	if !reflect.DeepEqual(program.Operation(openOriginal), decoded) {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", openOriginal, decoded)
	}

	closeOriginal := program.CloseKeyedQuestionOperation{Slot: "quiz", Key: program.ReferenceExpression{Name: "playerID"}}
	raw, err = encodeOperation("$", closeOriginal)
	if err != nil {
		t.Fatalf("encode close: %v", err)
	}
	decoded, err = decodeOperation("$", raw)
	if err != nil {
		t.Fatalf("decode close: %v", err)
	}
	if !reflect.DeepEqual(program.Operation(closeOriginal), decoded) {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", closeOriginal, decoded)
	}
}

func TestKeyedTimerOperations_RoundTrip(t *testing.T) {
	scheduleOriginal := program.ScheduleKeyedTimerOperation{
		Slot:              "disconnectTimeout",
		Key:               program.ReferenceExpression{Name: "playerID"},
		DelayMilliseconds: program.NumberLiteralExpression{Value: "30000"},
	}
	raw, err := encodeOperation("$", scheduleOriginal)
	if err != nil {
		t.Fatalf("encode schedule: %v", err)
	}
	decoded, err := decodeOperation("$", raw)
	if err != nil {
		t.Fatalf("decode schedule: %v", err)
	}
	if !reflect.DeepEqual(program.Operation(scheduleOriginal), decoded) {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", scheduleOriginal, decoded)
	}

	cancelOriginal := program.CancelKeyedTimerOperation{Slot: "disconnectTimeout", Key: program.ReferenceExpression{Name: "playerID"}}
	raw, err = encodeOperation("$", cancelOriginal)
	if err != nil {
		t.Fatalf("encode cancel: %v", err)
	}
	decoded, err = decodeOperation("$", raw)
	if err != nil {
		t.Fatalf("decode cancel: %v", err)
	}
	if !reflect.DeepEqual(program.Operation(cancelOriginal), decoded) {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", cancelOriginal, decoded)
	}
}

func TestKeyedAskGroupOperations_RoundTrip(t *testing.T) {
	openOriginal := program.OpenKeyedAskGroupOperation{
		Slot:       "team",
		Key:        program.StringLiteralExpression{Value: "team_a"},
		Recipients: program.ReferenceExpression{Name: "teamMembers"},
		Arguments:  []program.CallArgument{{Name: "question", Value: program.StringLiteralExpression{Value: "Vote?"}}},
		Completion: program.AskGroupQuorumPolicy{Count: program.NumberLiteralExpression{Value: "2"}},
	}
	raw, err := encodeOperation("$", openOriginal)
	if err != nil {
		t.Fatalf("encode open: %v", err)
	}
	decoded, err := decodeOperation("$", raw)
	if err != nil {
		t.Fatalf("decode open: %v", err)
	}
	if !reflect.DeepEqual(program.Operation(openOriginal), decoded) {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", openOriginal, decoded)
	}

	finalizeOriginal := program.FinalizeKeyedAskGroupOperation{Slot: "team", Key: program.StringLiteralExpression{Value: "team_a"}}
	raw, err = encodeOperation("$", finalizeOriginal)
	if err != nil {
		t.Fatalf("encode finalize: %v", err)
	}
	decoded, err = decodeOperation("$", raw)
	if err != nil {
		t.Fatalf("decode finalize: %v", err)
	}
	if !reflect.DeepEqual(program.Operation(finalizeOriginal), decoded) {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", finalizeOriginal, decoded)
	}

	cancelOriginal := program.CancelKeyedAskGroupOperation{Slot: "team", Key: program.StringLiteralExpression{Value: "team_a"}}
	raw, err = encodeOperation("$", cancelOriginal)
	if err != nil {
		t.Fatalf("encode cancel: %v", err)
	}
	decoded, err = decodeOperation("$", raw)
	if err != nil {
		t.Fatalf("decode cancel: %v", err)
	}
	if !reflect.DeepEqual(program.Operation(cancelOriginal), decoded) {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", cancelOriginal, decoded)
	}
}

// --- keyed signal sources ---

func TestKeyedSignalSources_RoundTrip(t *testing.T) {
	cases := []program.SignalSource{
		program.KeyedQuestionAnsweredSignalSource{Slot: "quiz"},
		program.KeyedTimerExpiredSignalSource{Slot: "disconnectTimeout"},
		program.KeyedAskGroupCompletedSignalSource{Slot: "team"},
	}
	for i, original := range cases {
		raw, err := encodeSignalSource("$", original)
		if err != nil {
			t.Fatalf("case %d: encode: %v", i, err)
		}
		decoded, err := decodeSignalSource("$", raw)
		if err != nil {
			t.Fatalf("case %d: decode: %v", i, err)
		}
		if !reflect.DeepEqual(original, decoded) {
			t.Fatalf("case %d: round trip mismatch:\n  original = %#v\n  decoded  = %#v", i, original, decoded)
		}
	}
}

// --- keyed slots on WorkflowDeclaration ---

func TestWorkflowDeclaration_KeyedSlots_RoundTrip(t *testing.T) {
	original := program.WorkflowDeclaration{
		Name:               "Main",
		ResultType:         program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
		KeyedQuestionSlots: []program.KeyedQuestionSlotDeclaration{{Name: "quiz", Question: "Q", KeyType: program.BuiltinTypeReference{Type: program.BuiltinTypeString}}},
		KeyedAskGroupSlots: []program.KeyedAskGroupSlotDeclaration{{Name: "team", Question: "Q", KeyType: program.BuiltinTypeReference{Type: program.BuiltinTypeString}}},
		KeyedTimerSlots:    []program.KeyedTimerSlotDeclaration{{Name: "deadline", KeyType: program.BuiltinTypeReference{Type: program.BuiltinTypeString}}},
		InitialState:       "S",
		States:             []program.WorkflowStateDeclaration{{Name: "S"}},
	}
	raw, err := encodeWorkflowDeclaration("$", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := decodeWorkflowDeclaration("$", raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("round trip mismatch:\n  original = %#v\n  decoded  = %#v", original, decoded)
	}
}
