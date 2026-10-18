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

	// SessionTimeMs is the effective Session time the state was reached at.
	SessionTimeMs int64
}

// Transition is one participant's new input state, at the effective Session
// time the Runtime accepted it.
type Transition struct {
	ParticipantID string

	// Inputs is the full input snapshot, as a JSON object.
	Inputs []byte

	SessionTimeMs int64
}

// SamplesInput is what a game's samples() receives: input transitions of
// every participant in the order the Runtime accepted them, and the Session
// time the batch brings the game to.
type SamplesInput struct {
	Transitions   []Transition
	SessionTimeMs int64
}

// ExecutionResult is the outcome of a mutating execution. State, PRNGState
// and ContextRevision are set only when it succeeded, ScriptError only when
// not.
type ExecutionResult struct {
	Outcome         ScriptOutcome
	State           []byte
	PRNGState       []byte
	ContextRevision int64
	ScriptError     string
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
