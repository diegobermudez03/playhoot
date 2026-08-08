package program_test

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/internal/game/program"
)

func TestUnaryOperator_IsValid(t *testing.T) {
	for _, op := range []program.UnaryOperator{program.UnaryOperatorNot, program.UnaryOperatorNegate} {
		if !op.IsValid() {
			t.Errorf("expected %q to be valid", op)
		}
	}

	if program.UnaryOperator("unknown").IsValid() {
		t.Errorf("expected %q to be invalid", "unknown")
	}
}

func TestBinaryOperator_IsValid(t *testing.T) {
	ops := []program.BinaryOperator{
		program.BinaryOperatorAdd, program.BinaryOperatorSubtract, program.BinaryOperatorMultiply,
		program.BinaryOperatorDivide, program.BinaryOperatorModulo,
		program.BinaryOperatorEqual, program.BinaryOperatorNotEqual, program.BinaryOperatorLess,
		program.BinaryOperatorLessOrEqual, program.BinaryOperatorGreater, program.BinaryOperatorGreaterOrEqual,
		program.BinaryOperatorAnd, program.BinaryOperatorOr,
		program.BinaryOperatorIn, program.BinaryOperatorNotIn,
	}
	for _, op := range ops {
		if !op.IsValid() {
			t.Errorf("expected %q to be valid", op)
		}
	}

	if program.BinaryOperator("unknown").IsValid() {
		t.Errorf("expected %q to be invalid", "unknown")
	}
}
