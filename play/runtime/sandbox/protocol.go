package sandbox

import (
	"encoding/json"
	"time"
)

// workerModeArg is the re-exec sentinel argument that makes this same binary
// act as a context worker instead of starting normally. Every entry point
// (main, and this package's TestMain) must check it before anything else.
const workerModeArg = "__playhoot_jsexecutor_worker__"

// Limits of one context. QuickJS enforces memory and stack itself; the
// execution budget is enforced by the host, which kills a worker that
// exceeds it, since QuickJS cannot interrupt a loop that never yields.
const (
	defaultMemoryLimitBytes  = 64 * 1024 * 1024
	defaultMaxStackSizeBytes = 1 * 1024 * 1024
	defaultExecutionBudget   = 5 * time.Second
	defaultMaxOutputBytes    = 256 * 1024

	// workerGoMemoryLimitBytes softly caps a worker's whole Go heap, which
	// includes its JavaScript runtime's memory.
	workerGoMemoryLimitBytes = 160 * 1024 * 1024
)

// workerOperation is what a workerRequest asks the context worker to do.
type workerOperation string

const (
	// workerOperationLoad creates the context's runtime and evaluates the
	// game's script; it is always the first request.
	workerOperationLoad    workerOperation = "load"
	workerOperationStart   workerOperation = "start"
	workerOperationExecute workerOperation = "execute"
	workerOperationProject workerOperation = "project"

	// workerOperationRestore sets a loaded context at a running game's
	// confirmed state instead of starting it.
	workerOperationRestore workerOperation = "restore"
)

// workerRequest is one line the host writes to a context worker's stdin.
type workerRequest struct {
	Operation workerOperation `json:"operation"`

	// Script is set only for workerOperationLoad.
	Script string `json:"script,omitempty"`

	// Participants and RandomSeed are set only for workerOperationStart.
	Participants []string `json:"participants,omitempty"`
	RandomSeed   uint64   `json:"randomSeed,omitempty"`

	// Participant is set only for workerOperationProject.
	Participant string `json:"participant,omitempty"`

	// Steps are set only for workerOperationExecute.
	Steps []workerStep `json:"steps,omitempty"`

	// SessionTimeMs is set only for workerOperationRestore: the Session time
	// the restored state was reached at.
	SessionTimeMs int64 `json:"sessionTimeMs,omitempty"`

	// State, PRNGState and ContextRevision are set only for
	// workerOperationRestore.
	State           json.RawMessage `json:"state,omitempty"`
	PRNGState       []byte          `json:"prngState,omitempty"`
	ContextRevision int64           `json:"contextRevision,omitempty"`
}

// workerStep is one operation of an execution batch at SessionTime:
// transitions for samples(), one command for command(), update(), or
// playerDisconnected() or playerRejoined() for the participant Disconnected
// or Rejoined names.
type workerStep struct {
	SessionTime  int64              `json:"sessionTime"`
	Transitions  []workerTransition `json:"transitions,omitempty"`
	Command      *workerCommand     `json:"command,omitempty"`
	Update       bool               `json:"update,omitempty"`
	Disconnected string             `json:"disconnected,omitempty"`
	Rejoined     string             `json:"rejoined,omitempty"`
}

// workerCommand is one COMMAND, as command() receives it.
type workerCommand struct {
	Participant string          `json:"participant"`
	Name        string          `json:"name"`
	Payload     json.RawMessage `json:"payload"`
	SessionTime int64           `json:"sessionTime"`
}

// workerTransition is one input transition, as samples() receives it.
type workerTransition struct {
	Participant string          `json:"participant"`
	Inputs      json.RawMessage `json:"inputs"`
	SessionTime int64           `json:"sessionTime"`
}

// workerError is a logical error the step at index Step returned to
// Participant.
type workerError struct {
	Step        int             `json:"step"`
	Participant string          `json:"participant"`
	Error       json.RawMessage `json:"error"`
}

// workerEvent is a logical event the step at index Step emitted, for
// Participants or, when none, for everyone.
type workerEvent struct {
	Step         int             `json:"step"`
	Participants []string        `json:"participants,omitempty"`
	Event        json.RawMessage `json:"event"`
}

// workerResponse is the line a context worker answers each request with. At
// most one of ScriptError (the authored code failed) or Fatal (the worker
// itself failed) is set.
type workerResponse struct {
	ScriptError *string `json:"scriptError,omitempty"`
	Fatal       *string `json:"fatal,omitempty"`

	State           json.RawMessage `json:"state,omitempty"`
	PRNGState       []byte          `json:"prngState,omitempty"`
	ContextRevision int64           `json:"contextRevision,omitempty"`
	View            json.RawMessage `json:"view,omitempty"`

	// CommandResponses answer an execute request's command steps, in order.
	CommandResponses []json.RawMessage `json:"commandResponses,omitempty"`

	// Errors are the logical errors an execute request's steps returned.
	Errors []workerError `json:"errors,omitempty"`

	// Events are the logical events an execute request's steps emitted.
	Events []workerEvent `json:"events,omitempty"`

	// RejoinsAccepted answer an execute request's rejoin steps, in order.
	RejoinsAccepted []bool `json:"rejoinsAccepted,omitempty"`

	// MemoryBytes is all the memory the worker process holds after the call.
	MemoryBytes uint64 `json:"memoryBytes,omitempty"`
}
