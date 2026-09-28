package jsengine

import "encoding/json"

// workerModeArg is the re-exec sentinel argument that tells this same
// binary to act as a jsengine worker instead of starting normally. It must
// be checked at the very top of every real and test entry point (see
// game/session's RunWorkerIfRequested and this package's TestMain) before
// any other initialization.
const workerModeArg = "__playhoot_jsengine_worker__"

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

type workerContext struct {
	LogicalTime string `json:"logicalTime"`
	RandomSeed  string `json:"randomSeed"`
	ActingActor string `json:"actingActor"`
}

type workerRequest struct {
	Script        string          `json:"script"`
	PreviousState json.RawMessage `json:"previousState"`
	Event         json.RawMessage `json:"event"`
	Context       workerContext   `json:"context"`
	// ScratchDir is a fresh, empty directory the caller (Execute) created
	// and owns cleaning up. The worker mounts it as the sandbox's
	// filesystem root; the worker creating its own would leave it orphaned
	// on disk whenever the worker process is killed rather than exiting
	// normally, since a killed process never runs its own deferred cleanup.
	ScratchDir string `json:"scratchDir"`
}

// workerResponse is the worker process's single reply. Exactly one of
// NewState (success), Rejected (script-level outcome), or Fatal
// (infrastructure-level failure) is populated.
type workerResponse struct {
	NewState          json.RawMessage   `json:"newState,omitempty"`
	RequestedCommands []json.RawMessage `json:"requestedCommands,omitempty"`
	Rejected          *string           `json:"rejected,omitempty"`
	Fatal             *string           `json:"fatal,omitempty"`
}

func rawOrNull(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "null"
	}
	return string(raw)
}
