package executor

import "context"

// Fake is an in-memory Executor for tests that need to control Execute's
// outcome deterministically without a real Executor process. A nil
// ExecuteFunc returns a zero ExecutionOutput and no error.
type Fake struct {
	ExecuteFunc func(ctx context.Context, in ExecutionInput) (ExecutionOutput, error)
}

func (f *Fake) Execute(ctx context.Context, in ExecutionInput) (ExecutionOutput, error) {
	if f.ExecuteFunc == nil {
		return ExecutionOutput{}, nil
	}
	return f.ExecuteFunc(ctx, in)
}
