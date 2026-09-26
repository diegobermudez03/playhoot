package sessionlifecycle

import (
	"context"
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/stretchr/testify/require"
)

// TestManagerSubmitUserIntent covers SubmitUserIntent's pre-transaction
// validation - the only part mockable without a real DB transaction: the
// lock, idempotency claim, actor/argument validation, and engine execution
// further down this path call the shared sessionlock/idempotency mechanism
// packages and the real Game Language engine directly, which require a
// real Postgres connection to exercise meaningfully. That business logic is
// proven instead by this package's TestManagerSubmitUserIntent_Integration*
// tests against a real disposable database.
func TestManagerSubmitUserIntent(t *testing.T) {
	m := &Manager{}

	_, err := m.SubmitUserIntent(context.Background(), "session-uuid", "user-uuid", "Guess", nil, "")
	require.ErrorIs(t, err, session.ErrIdempotencyKeyRequired)
}

func TestDecodeUserIntentArguments(t *testing.T) {
	t.Run("empty_defaults_to_empty_record", func(t *testing.T) {
		value, err := decodeUserIntentArguments(nil)
		require.NoError(t, err)
		require.Equal(t, engine.RecordValue{}, value)
	})

	t.Run("decodes_a_valid_record", func(t *testing.T) {
		encoded, err := engineservice.EncodeValue(engine.RecordValue{Fields: []engine.FieldValue{{Name: "value", Value: engine.NumberValue{Value: 42}}}})
		require.NoError(t, err)

		value, err := decodeUserIntentArguments(encoded)
		require.NoError(t, err)
		record, ok := value.(engine.RecordValue)
		require.True(t, ok)
		field, ok := record.FieldByName("value")
		require.True(t, ok)
		require.Equal(t, engine.NumberValue{Value: 42}, field.Value)
	})

	t.Run("malformed_json_is_an_error", func(t *testing.T) {
		_, err := decodeUserIntentArguments([]byte("{not json"))
		require.Error(t, err)
	})
}

func TestValidateUserIntentArguments(t *testing.T) {
	numberType := program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}
	definition := program.Definition{
		UserIntents: []program.UserIntentDeclaration{
			{Name: "Guess", Parameters: []program.FieldDeclaration{{Name: "value", Type: numberType}}},
			{Name: "NoOp"},
			{Name: "Named", Parameters: []program.FieldDeclaration{{Name: "thing", Type: program.NamedTypeReference{Name: "Thing"}}}},
		},
	}

	record := func(fields ...engine.FieldValue) engine.Value {
		return engine.RecordValue{Fields: fields}
	}

	tests := map[string]struct {
		intent    string
		arguments engine.Value
		wantOK    bool
	}{
		"unknown_intent_is_rejected": {
			intent:    "NotDeclared",
			arguments: record(),
			wantOK:    false,
		},
		"correct_arguments_are_accepted": {
			intent:    "Guess",
			arguments: record(engine.FieldValue{Name: "value", Value: engine.NumberValue{Value: 1}}),
			wantOK:    true,
		},
		"missing_declared_field_is_rejected": {
			intent:    "Guess",
			arguments: record(),
			wantOK:    false,
		},
		"wrong_builtin_type_is_rejected": {
			intent:    "Guess",
			arguments: record(engine.FieldValue{Name: "value", Value: engine.StringValue{Value: "nope"}}),
			wantOK:    false,
		},
		"non_record_arguments_are_rejected": {
			intent:    "Guess",
			arguments: engine.NumberValue{Value: 1},
			wantOK:    false,
		},
		"zero_parameter_intent_accepts_empty_record": {
			intent:    "NoOp",
			arguments: record(),
			wantOK:    true,
		},
		"named_type_parameter_present_is_accepted_without_deep_validation": {
			// Recorded, accepted residual limitation (see this WORK's
			// "Argument Validation Boundary"): a named-typed parameter is
			// only checked for field presence, not deep type conformance,
			// since resolving it requires compiler-internal named-type
			// resolution this package does not have access to.
			intent:    "Named",
			arguments: record(engine.FieldValue{Name: "thing", Value: engine.StringValue{Value: "anything at all"}}),
			wantOK:    true,
		},
		"named_type_parameter_missing_is_still_rejected": {
			intent:    "Named",
			arguments: record(),
			wantOK:    false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, ok := validateUserIntentArguments(definition, tt.intent, tt.arguments)
			require.Equal(t, tt.wantOK, ok)
		})
	}
}

func TestBuiltinEngineType(t *testing.T) {
	tests := map[string]struct {
		ref    program.TypeReference
		want   engine.Type
		wantOK bool
	}{
		"bool":       {program.BuiltinTypeReference{Type: program.BuiltinTypeBool}, engine.BoolType{}, true},
		"number":     {program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}, engine.NumberType{}, true},
		"string":     {program.BuiltinTypeReference{Type: program.BuiltinTypeString}, engine.StringType{}, true},
		"user":       {program.BuiltinTypeReference{Type: program.BuiltinTypeUser}, engine.UserType{}, true},
		"unit":       {program.BuiltinTypeReference{Type: program.BuiltinTypeUnit}, nil, false},
		"named_type": {program.NamedTypeReference{Name: "Thing"}, nil, false},
		"list_type":  {program.ListTypeReference{Element: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}}, nil, false},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := builtinEngineType(tt.ref)
			require.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				require.Equal(t, tt.want, got)
			}
		})
	}
}
