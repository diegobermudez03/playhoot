package program_test

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/program"
)

func TestBuiltinType_IsValid(t *testing.T) {
	valid := []program.BuiltinType{
		program.BuiltinTypeUnit,
		program.BuiltinTypeBool,
		program.BuiltinTypeNumber,
		program.BuiltinTypeString,
		program.BuiltinTypeUser,
	}
	for _, bt := range valid {
		if !bt.IsValid() {
			t.Errorf("expected %q to be valid", bt)
		}
	}

	if program.BuiltinType("unknown").IsValid() {
		t.Errorf("expected %q to be invalid", "unknown")
	}
}
