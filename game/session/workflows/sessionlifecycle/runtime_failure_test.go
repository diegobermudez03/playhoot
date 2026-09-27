package sessionlifecycle

import (
	"errors"
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/stretchr/testify/require"
)

func TestMapExecutionErrorCode(t *testing.T) {
	t.Run("maps every known engine code to a distinct stable string", func(t *testing.T) {
		codes := []engineservice.ExecutionErrorCode{
			engineservice.ExecutionErrorUnknown,
			engineservice.ExecutionErrorUndefinedReference,
			engineservice.ExecutionErrorDivisionByZero,
			engineservice.ExecutionErrorIndexOutOfRange,
			engineservice.ExecutionErrorKeyNotFound,
			engineservice.ExecutionErrorNoMatchingCase,
			engineservice.ExecutionErrorInvalidInitialState,
			engineservice.ExecutionErrorInvariantViolation,
			engineservice.ExecutionErrorSnapshotProgramMismatch,
			engineservice.ExecutionErrorBudgetExceeded,
			engineservice.ExecutionErrorLoopLimitExceeded,
			engineservice.ExecutionErrorInvalidRandomRange,
			engineservice.ExecutionErrorEmptyRandomCollection,
			engineservice.ExecutionErrorSlotOccupied,
			engineservice.ExecutionErrorInvalidTimerDelay,
			engineservice.ExecutionErrorDuplicateRecipient,
			engineservice.ExecutionErrorInvalidQuorum,
			engineservice.ExecutionErrorAskGroupNotJoined,
			engineservice.ExecutionErrorPresentationSlotOccupied,
			engineservice.ExecutionErrorActiveSlotLimitExceeded,
			engineservice.ExecutionErrorStepChainExceeded,
		}
		seen := make(map[string]bool, len(codes))
		for _, code := range codes {
			mapped := mapExecutionErrorCode(code)
			require.NotEmpty(t, mapped)
			require.NotEqual(t, RuntimeFailureErrorCodeUnrecognized, mapped)
			require.False(t, seen[mapped], "code %v mapped to a non-distinct string %q", code, mapped)
			seen[mapped] = true
		}
	})

	t.Run("ExecutionErrorStepChainExceeded maps to GAME-ADR-0019's own named example", func(t *testing.T) {
		require.Equal(t, RuntimeFailureErrorCodeStepLimitExceeded, mapExecutionErrorCode(engineservice.ExecutionErrorStepChainExceeded))
		require.Equal(t, "runtime_turn_step_limit_exceeded", mapExecutionErrorCode(engineservice.ExecutionErrorStepChainExceeded))
	})

	t.Run("an unrecognized future code falls back defensively", func(t *testing.T) {
		require.Equal(t, RuntimeFailureErrorCodeUnrecognized, mapExecutionErrorCode(engineservice.ExecutionErrorCode(-1)))
	})
}

func TestClassifyExecutionError(t *testing.T) {
	t.Run("extracts code and message from an engineservice.ExecutionError", func(t *testing.T) {
		err := &engineservice.ExecutionError{Code: engineservice.ExecutionErrorDivisionByZero, Message: "division by zero"}
		code, message := classifyExecutionError(err)
		require.Equal(t, RuntimeFailureErrorCodeDivisionByZero, code)
		require.Equal(t, "division by zero", message)
	})

	t.Run("falls back to unrecognized for a non-ExecutionError", func(t *testing.T) {
		err := errors.New("some other failure")
		code, message := classifyExecutionError(err)
		require.Equal(t, RuntimeFailureErrorCodeUnrecognized, code)
		require.Equal(t, "some other failure", message)
	})
}
