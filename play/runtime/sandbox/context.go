package sandbox

// ContextSpec is the game a new execution context runs.
type ContextSpec struct {
	GameVersionUUID string

	// Script is the game version's JavaScript source.
	Script []byte
}

// StartInput is what a game's start() receives: who plays, by ParticipantID
// only, and the seed of the only randomness the script may observe.
type StartInput struct {
	Participants []string
	RandomSeed   uint64
}

// Snapshot is a running game's confirmed state, as the Runtime keeps it, to
// restore a context at without running start() again.
type Snapshot struct {
	// State is the authoritative game state, as JSON.
	State []byte

	// PRNGState is the opaque state of the script's randomness.
	PRNGState []byte

	ContextRevision int64
}

// ScriptOutcome is whether authored code completed an execution.
type ScriptOutcome string

const (
	ScriptSucceeded ScriptOutcome = "succeeded"

	// ScriptFailed means the authored code threw, exceeded its budget or
	// returned an invalid value; the context must be discarded.
	ScriptFailed ScriptOutcome = "script_failed"
)

// StartResult is the outcome of a game's start(). State, PRNGState and
// ContextRevision are set only when it succeeded, ScriptError only when not.
type StartResult struct {
	Outcome ScriptOutcome

	// State is the initial authoritative game state, as JSON.
	State []byte

	// PRNGState is the opaque state of the script's randomness after start.
	PRNGState []byte

	// ContextRevision advances with every successful execution.
	ContextRevision int64

	ScriptError string
}

// ProjectResult is one participant's view of the current state, as the
// game's project() returned it. View is set only when it succeeded,
// ScriptError only when not.
type ProjectResult struct {
	Outcome     ScriptOutcome
	View        []byte
	ScriptError string
}

// RestoreResult is the outcome of restoring a context at a Snapshot.
// ScriptError is set only when the script failed to load.
type RestoreResult struct {
	Outcome     ScriptOutcome
	ScriptError string
}
