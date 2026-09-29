package sessionlifecycle

import (
	"errors"
	"testing"

	"github.com/diegobermudez03/playhoot/session/internal/executor"
	"github.com/stretchr/testify/require"
)

func TestClassifyExecutionError(t *testing.T) {
	t.Run("extracts the stable code and message from an executor.ExecutorError", func(t *testing.T) {
		err := &executor.ExecutorError{Reason: "unreachable"}
		code, message := classifyExecutionError(err)
		require.Equal(t, RuntimeFailureErrorCodeExecutorError, code)
		require.Equal(t, err.Error(), message)
	})

	t.Run("any other error shape is classified the same way", func(t *testing.T) {
		err := errors.New("some other failure")
		code, message := classifyExecutionError(err)
		require.Equal(t, RuntimeFailureErrorCodeExecutorError, code)
		require.Equal(t, "some other failure", message)
	})
}
