package engine

// Trace explains one executed transition: which signal was matched,
// which transition and guard result led to it, which operations ran,
// and which WorkflowControl result was applied.
//
// A Trace is produced only as part of a Commit and describes exactly one
// Step call. It is intended to support debugging, explanation, and
// replay verification (see LOGICAL_CONTRACT.md's determinism
// guarantees), not to drive further execution.
//
// This is currently an empty placeholder: this version does not yet
// record any real information about the executed transition. Concrete
// fields are added once transition selection and execution are
// implemented.
type Trace struct{}
