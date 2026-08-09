package program_test

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/v1/program"
)

func TestLinearLayoutDirection_IsValid(t *testing.T) {
	for _, d := range []program.LinearLayoutDirection{program.LinearLayoutDirectionRow, program.LinearLayoutDirectionColumn} {
		if !d.IsValid() {
			t.Errorf("expected %q to be valid", d)
		}
	}

	if program.LinearLayoutDirection("unknown").IsValid() {
		t.Errorf("expected %q to be invalid", "unknown")
	}
}

func TestUIEventType_IsValid(t *testing.T) {
	events := []program.UIEventType{
		program.UIEventTypeClick,
		program.UIEventTypeDoubleClick,
		program.UIEventTypePointerEnter,
		program.UIEventTypePointerLeave,
	}
	for _, e := range events {
		if !e.IsValid() {
			t.Errorf("expected %q to be valid", e)
		}
	}

	if program.UIEventType("unknown").IsValid() {
		t.Errorf("expected %q to be invalid", "unknown")
	}
}
