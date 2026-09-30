package sandbox

import "encoding/json"

// workerModeArg is the re-exec sentinel argument that tells this same
// binary to act as a sandbox worker instead of starting normally. It must
// be checked at the very top of every real and test entry point (this
// package's own TestMain, and jsexecutor's main.go) before any other
// initialization.
const workerModeArg = "__playhoot_jsexecutor_worker__"

// Best-effort resource defaults enforced by the QuickJS runtime itself.
// These alone are not a sufficient resource-limit guarantee: a tight loop
// that never yields back to the runtime can outlast them, so the caller's
// own context deadline (enforced by killing the whole worker process) is
// what a runaway execution's termination actually depends on. This
// implementation assumes MaxExecutionTime's unit is milliseconds based on
// the upstream field name alone; that assumption has not been independently
// verified against the library's own behavior.
const (
	defaultMemoryLimitBytes       = 64 * 1024 * 1024
	defaultMaxStackSizeBytes      = 1 * 1024 * 1024
	defaultMaxExecutionTimeMillis = 5000
)

// defaultMaxOutputBytes/defaultMaxCommandCount bound a successful
// execution's own returned shape (NewState plus RequestedCommands) -
// distinct from the resource limits above, which bound the runtime while a
// script is executing. Enforced by Execute itself (the caller process),
// after decoding the worker's response, so no worker-side code needs to
// reason about the caller's limits.
const (
	defaultMaxOutputBytes  = 256 * 1024
	defaultMaxCommandCount = 100
)

type workerContext struct {
	LogicalTime string `json:"logicalTime"`
	RandomSeed  string `json:"randomSeed"`
	ActingActor string `json:"actingActor"`
}

// workerOperation discriminates which of this package's two pure script
// entry points a workerRequest asks for. Both share the same worker
// process/runtime lifecycle (Execution Model in LOGICAL_CONTRACT.md) -
// only which global function is invoked, and which fields of
// workerRequest/workerResponse are meaningful, differ.
type workerOperation string

const (
	workerOperationExecute workerOperation = "execute"
	workerOperationProject workerOperation = "project"
)

type workerRequest struct {
	Operation workerOperation `json:"operation"`
	Script    string          `json:"script"`
	// PreviousState/Event are meaningful only for workerOperationExecute.
	PreviousState json.RawMessage `json:"previousState,omitempty"`
	Event         json.RawMessage `json:"event,omitempty"`
	// State/Viewer are meaningful only for workerOperationProject. State is
	// already the caller-constructed, viewer-scoped ProjectInput - never
	// the full authoritative state.
	State   json.RawMessage `json:"state,omitempty"`
	Viewer  string          `json:"viewer,omitempty"`
	Context workerContext   `json:"context"`
	// ScratchDir is a fresh, empty directory the caller (Execute/Project)
	// created and owns cleaning up. The worker mounts it as the sandbox's
	// filesystem root; the worker creating its own would leave it orphaned
	// on disk whenever the worker process is killed rather than exiting
	// normally, since a killed process never runs its own deferred cleanup.
	ScratchDir string `json:"scratchDir"`
}

// workerResponse is the worker process's single reply. Exactly one of
// NewState (Execute success), ClientState (Project success), Rejected
// (script-level outcome), or Fatal (infrastructure-level failure) is
// populated.
type workerResponse struct {
	NewState          json.RawMessage   `json:"newState,omitempty"`
	RequestedCommands []json.RawMessage `json:"requestedCommands,omitempty"`
	ClientState       json.RawMessage   `json:"clientState,omitempty"`
	Rejected          *string           `json:"rejected,omitempty"`
	Fatal             *string           `json:"fatal,omitempty"`
}

func rawOrNull(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "null"
	}
	return string(raw)
}
