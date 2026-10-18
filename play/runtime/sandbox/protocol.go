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
)

// workerOperation is what a workerRequest asks the context worker to do.
type workerOperation string

const (
	// workerOperationLoad creates the context's runtime and evaluates the
	// game's script; it is always the first request.
	workerOperationLoad    workerOperation = "load"
	workerOperationStart   workerOperation = "start"
	workerOperationProject workerOperation = "project"

	// workerOperationRestore sets a loaded context at a running game's
	// confirmed state instead of starting it.
	workerOperationRestore workerOperation = "restore"
)

// workerRequest is one line the host writes to a context worker's stdin.
type workerRequest struct {
	Operation workerOperation `json:"operation"`

	// Script and ScratchDir are set only for workerOperationLoad. The host
	// owns the scratch directory, so a killed worker never leaves it behind.
	Script     string `json:"script,omitempty"`
	ScratchDir string `json:"scratchDir,omitempty"`

	// Participants and RandomSeed are set only for workerOperationStart.
	Participants []string `json:"participants,omitempty"`
	RandomSeed   uint64   `json:"randomSeed,omitempty"`

	// Participant is set only for workerOperationProject.
	Participant string `json:"participant,omitempty"`

	// State, PRNGState and ContextRevision are set only for
	// workerOperationRestore.
	State           json.RawMessage `json:"state,omitempty"`
	PRNGState       []byte          `json:"prngState,omitempty"`
	ContextRevision int64           `json:"contextRevision,omitempty"`
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
}
