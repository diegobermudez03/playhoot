package engine

// AskGroupCompletionPolicy is the compiled representation of one
// program.AskGroupCompletionPolicy, evaluated when an OpenAskGroupOperation
// executes — see engine.AskGroupCompletionKind for its resolved,
// durable runtime form once a group is open.
//
// AskGroupCompletionPolicy is a closed interface, mirroring program's
// own closed-interface pattern.
type AskGroupCompletionPolicy interface {
	isAskGroupCompletionPolicy()
}

// AskGroupAllResponsesPolicy completes an ask group once every unique
// recipient has submitted one accepted answer.
type AskGroupAllResponsesPolicy struct{}

func (AskGroupAllResponsesPolicy) isAskGroupCompletionPolicy() {}

// AskGroupFirstResponsePolicy completes an ask group as soon as the
// first accepted answer is processed.
type AskGroupFirstResponsePolicy struct{}

func (AskGroupFirstResponsePolicy) isAskGroupCompletionPolicy() {}

// AskGroupQuorumPolicy completes an ask group once Count unique
// recipients have submitted accepted answers. Count is evaluated once
// when the group is opened — see program.AskGroupQuorumPolicy.
type AskGroupQuorumPolicy struct {
	Count Expression
}

func (AskGroupQuorumPolicy) isAskGroupCompletionPolicy() {}

// OpenAskGroupOperation opens one ask-group instance in the named
// workflow-owned slot Slot, sending the slot's question to every user
// produced by Recipients, with Arguments as the question's shared,
// group-wide arguments. The compiler guarantees Recipients is statically
// list<user> and Arguments matches the slot's question's declared
// parameters exactly. Opening an already occupied slot — collecting or
// a terminal outcome still awaiting join — is an execution error that
// fails the transition atomically, as is a Recipients list containing a
// duplicate identity, or a Completion whose evaluated quorum is not a
// positive integer no greater than the number of recipients. See
// program.OpenAskGroupOperation.
type OpenAskGroupOperation struct {
	Slot       string
	Recipients Expression
	Arguments  []CallArgument
	Completion AskGroupCompletionPolicy
}

func (OpenAskGroupOperation) isOperation() {}

// FinalizeAskGroupOperation forces the currently collecting ask group in
// the named slot Slot to complete using only the accepted responses
// received so far, without waiting for its completion policy to be
// otherwise satisfied. Finalizing a slot already completed-awaiting-join
// is an idempotent no-op — the documented resolution for an
// answer-versus-deadline race, see program.FinalizeAskGroupOperation.
// Finalizing an empty slot is an execution error.
type FinalizeAskGroupOperation struct {
	Slot string
}

func (FinalizeAskGroupOperation) isOperation() {}

// CancelAskGroupOperation abandons the currently collecting ask group in
// the named slot Slot without producing an AskGroupCompletedSignalSource
// signal: every accepted response so far is discarded and the slot is
// cleared. Cancelling an already empty slot is an idempotent no-op.
// Cancelling a slot that holds a terminal outcome still awaiting join is
// an execution error — that outcome must be joined through
// AskGroupCompletedSignalSource first, never silently discarded. See
// program.CancelAskGroupOperation.
type CancelAskGroupOperation struct {
	Slot string
}

func (CancelAskGroupOperation) isOperation() {}

// OpenKeyedAskGroupOperation opens one ask-group occurrence at the named
// workflow-owned keyed slot Slot's occurrence Key, sending the slot's
// question to every user produced by Recipients, with Arguments as the
// question's shared, group-wide arguments, generalizing
// OpenAskGroupOperation to a (slot, key) occurrence. The compiler
// guarantees Key is statically compatible with the slot's declared
// KeyType, Recipients is statically list<user>, and Arguments matches
// the slot's question's declared parameters exactly. Opening an already
// occupied (slot, key) — collecting or a terminal outcome still awaiting
// join — is an execution error that fails the transition atomically, as
// is a Recipients list containing a duplicate identity, or a Completion
// whose evaluated quorum is not a positive integer no greater than the
// number of recipients. See program.OpenKeyedAskGroupOperation.
type OpenKeyedAskGroupOperation struct {
	Slot       string
	Key        Expression
	Recipients Expression
	Arguments  []CallArgument
	Completion AskGroupCompletionPolicy
}

func (OpenKeyedAskGroupOperation) isOperation() {}

// FinalizeKeyedAskGroupOperation forces the currently collecting
// ask-group occurrence at the named keyed slot Slot's occurrence Key to
// complete using only the accepted responses received so far,
// generalizing FinalizeAskGroupOperation to a (slot, key) occurrence.
// Finalizing an occurrence already completed-awaiting-join is an
// idempotent no-op; finalizing an empty (slot, key) is an execution
// error. See program.FinalizeKeyedAskGroupOperation.
type FinalizeKeyedAskGroupOperation struct {
	Slot string
	Key  Expression
}

func (FinalizeKeyedAskGroupOperation) isOperation() {}

// CancelKeyedAskGroupOperation abandons the currently collecting
// ask-group occurrence at the named keyed slot Slot's occurrence Key
// without producing a KeyedAskGroupCompletedSignalSource signal,
// generalizing CancelAskGroupOperation to a (slot, key) occurrence.
// Cancelling an already empty (slot, key) is an idempotent no-op.
// Cancelling one that holds a terminal outcome still awaiting join is an
// execution error. See program.CancelKeyedAskGroupOperation.
type CancelKeyedAskGroupOperation struct {
	Slot string
	Key  Expression
}

func (CancelKeyedAskGroupOperation) isOperation() {}
