// Package sandbox runs author-written JavaScript inside a QuickJS-on-
// WebAssembly sandbox, hosted in a process separate from the caller's own,
// so untrusted or generated game rules cannot read the host's files,
// network, or process state. See LOGICAL_CONTRACT.md in this package for the
// full contract.
package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ExecutionContext carries the only sources of non-determinism authored
// JavaScript may observe. The sandbox neutralizes the language's own
// ambient clock/randomness (Date, Math.random), so every such value a
// script can obtain traces back to a field here.
type ExecutionContext struct {
	LogicalTime time.Time
	RandomSeed  uint64
	ActingActor string
}

// ResolvedScript is already-resolved authored script content. This package
// only executes it; fetching, caching, and versioning script artifacts is
// the caller's responsibility.
type ResolvedScript struct {
	Source string
}

// RawCommand is an opaque, ordered command value. This package passes it
// through unexamined; validating a command's business meaning or vocabulary
// is the caller's responsibility.
type RawCommand = json.RawMessage

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
	RequestedCommands []RawCommand
}

// ScriptRejectedError indicates the authored script itself declined the
// input: it threw an exception, or returned a value that does not match the
// required {newState, requestedCommands} shape. It is a business-level
// outcome, never a Go panic or a crash of the calling process.
type ScriptRejectedError struct {
	Reason string
}

func (e *ScriptRejectedError) Error() string {
	return fmt.Sprintf("sandbox: script rejected input: %s", e.Reason)
}

// WorkerExecutionError indicates an infrastructure-level execution failure:
// the worker process crashed, the IPC exchange failed, or the caller's
// deadline was exceeded before the worker responded. It is distinct from
// ScriptRejectedError, which represents the script's own outcome.
type WorkerExecutionError struct {
	Reason string
	Cause  error
}

func (e *WorkerExecutionError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("sandbox: worker execution failed: %s: %v", e.Reason, e.Cause)
	}
	return fmt.Sprintf("sandbox: worker execution failed: %s", e.Reason)
}

func (e *WorkerExecutionError) Unwrap() error { return e.Cause }

// currentWorkerArg selects which re-exec sentinel Execute spawns itself
// with. It is a package variable, not a constant, only so tests can point it
// at a deliberately misbehaving worker mode to prove worker-crash isolation
// without changing Execute's own production behavior.
var currentWorkerArg = workerModeArg

// Execute runs one authored-script execution against previousState/event/
// context inside a QuickJS-on-WebAssembly sandbox, hosted in a process
// separate from the caller's own. Execute is stateless as a library — no
// invocation depends on or mutates state left behind by an unrelated prior
// invocation — and is safe for concurrent use across different callers; it
// does not itself serialize concurrent calls, so a caller that needs at most
// one execution in flight for a given logical unit of work must enforce
// that itself.
func Execute(ctx context.Context, in ExecutionInput) (ExecutionOutput, error) {
	exePath, err := os.Executable()
	if err != nil {
		return ExecutionOutput{}, &WorkerExecutionError{Reason: "resolving own executable path", Cause: err}
	}

	// Owned here, not by the worker: a killed worker process never runs its
	// own deferred cleanup, so a directory the worker created itself would
	// be left on disk forever after every caller-deadline kill. This
	// process is never the one killed, so its own cleanup always runs.
	scratchDir, err := os.MkdirTemp("", "playhoot-jsexecutor-*")
	if err != nil {
		return ExecutionOutput{}, &WorkerExecutionError{Reason: "creating sandbox scratch directory", Cause: err}
	}
	defer os.RemoveAll(scratchDir)

	req := workerRequest{
		Script:        in.Script.Source,
		PreviousState: in.PreviousState,
		Event:         in.Event,
		Context: workerContext{
			LogicalTime: in.Context.LogicalTime.UTC().Format(time.RFC3339Nano),
			RandomSeed:  strconv.FormatUint(in.Context.RandomSeed, 10),
			ActingActor: in.Context.ActingActor,
		},
		ScratchDir: scratchDir,
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return ExecutionOutput{}, &WorkerExecutionError{Reason: "encoding worker request", Cause: err}
	}

	cmd := exec.CommandContext(ctx, exePath, currentWorkerArg)
	// A child process inherits the parent's full environment by default,
	// which would otherwise hand every authored script's sandbox whatever
	// real secrets/config this application process happens to hold.
	cmd.Env = []string{}
	cmd.Stdin = bytes.NewReader(reqBytes)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if ctx.Err() != nil {
		return ExecutionOutput{}, &WorkerExecutionError{Reason: "execution deadline exceeded", Cause: ctx.Err()}
	}
	if runErr != nil {
		return ExecutionOutput{}, &WorkerExecutionError{
			Reason: fmt.Sprintf("worker process failed (stderr: %q)", strings.TrimSpace(stderr.String())),
			Cause:  runErr,
		}
	}

	var resp workerResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return ExecutionOutput{}, &WorkerExecutionError{
			Reason: fmt.Sprintf("decoding worker response (stdout: %q)", truncate(stdout.String(), 256)),
			Cause:  err,
		}
	}

	if resp.Rejected != nil {
		return ExecutionOutput{}, &ScriptRejectedError{Reason: *resp.Rejected}
	}
	if resp.Fatal != nil {
		return ExecutionOutput{}, &WorkerExecutionError{Reason: *resp.Fatal}
	}

	return ExecutionOutput{
		NewState:          resp.NewState,
		RequestedCommands: resp.RequestedCommands,
	}, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
