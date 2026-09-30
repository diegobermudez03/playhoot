package executor

import "context"

// Fake is an in-memory Executor for tests that need to control Execute's/
// Project's outcome deterministically without a real Executor process. A
// nil ExecuteFunc/ProjectFunc returns a zero output and no error.
type Fake struct {
	ExecuteFunc func(ctx context.Context, in ExecutionInput) (ExecutionOutput, error)
	ProjectFunc func(ctx context.Context, in ProjectionInput) (ProjectionOutput, error)
}

func (f *Fake) Execute(ctx context.Context, in ExecutionInput) (ExecutionOutput, error) {
	if f.ExecuteFunc == nil {
		return ExecutionOutput{}, nil
	}
	return f.ExecuteFunc(ctx, in)
}

func (f *Fake) Project(ctx context.Context, in ProjectionInput) (ProjectionOutput, error) {
	if f.ProjectFunc == nil {
		return ProjectionOutput{}, nil
	}
	return f.ProjectFunc(ctx, in)
}
