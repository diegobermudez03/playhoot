package engine

// Commit is the atomic result of one engineservice.Step call.
//
// Per LOGICAL_CONTRACT.md, a Commit represents, as a single unit:
//
//   - the new Snapshot;
//   - declarative Outputs;
//   - internal Signals that must be processed later (for example, a
//     completion signal that this step causes but does not itself
//     handle);
//   - the transition Trace;
//   - the consumed Signal.
//
// A Commit is only ever produced by a successful Step call. When Step
// returns a non-nil error instead, no Commit is produced, the input
// Snapshot is unchanged, and no Output is considered published.
type Commit struct {
	Snapshot        Snapshot
	Outputs         []Output
	InternalSignals []Signal
	Trace           Trace
	ConsumedSignal  Signal
}
