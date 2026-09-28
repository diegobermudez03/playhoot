// Package executor is Session Runtime's own caller-side port onto the
// separately deployed JavaScript Executor (game/session/jsexecutor). Session
// Runtime depends only on the Executor interface here, never on gRPC status
// codes, connection management, or anything about how the Executor itself
// runs authored scripts.
package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ExecutionContext carries the only sources of non-determinism authored
// JavaScript may observe.
type ExecutionContext struct {
	LogicalTime time.Time
	RandomSeed  uint64
	ActingActor string
}

// ResolvedScript is already-resolved authored script content. This package
// only sends it to the Executor; fetching, caching, and versioning script
// artifacts is the caller's responsibility.
type ResolvedScript struct {
	Source string
}

// ExecutionInput is Execute's full set of explicit inputs.
type ExecutionInput struct {
	Script        ResolvedScript
	PreviousState json.RawMessage
	Event         json.RawMessage
	Context       ExecutionContext
}

// ExecutionOutput is the script's new authoritative state plus zero or more
// requested commands, on success.
type ExecutionOutput struct {
	NewState          json.RawMessage
	RequestedCommands []json.RawMessage
}

// Executor runs one authored-script execution against previousState/event/
// context. The production implementation reaches a separately deployed
// service over the network; a caller must not assume anything about how it
// does so.
type Executor interface {
	Execute(ctx context.Context, in ExecutionInput) (ExecutionOutput, error)
}

// ScriptRejectedError indicates the authored script itself declined the
// input: it threw an exception, or returned a value that does not match its
// required shape. It is a business-level outcome, distinct from
// ExecutorError, which represents a failure to run the script at all.
type ScriptRejectedError struct {
	Reason string
}

func (e *ScriptRejectedError) Error() string {
	return fmt.Sprintf("executor: script rejected input: %s", e.Reason)
}

// ExecutorError indicates an infrastructure-level failure: the Executor was
// unreachable, the call failed, or the caller's deadline was exceeded before
// a result arrived. It is distinct from ScriptRejectedError, which
// represents the script's own outcome.
type ExecutorError struct {
	Reason string
	Cause  error
}

func (e *ExecutorError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("executor: request failed: %s: %v", e.Reason, e.Cause)
	}
	return fmt.Sprintf("executor: request failed: %s", e.Reason)
}

func (e *ExecutorError) Unwrap() error { return e.Cause }
