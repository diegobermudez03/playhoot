package engine

// AssignmentTarget is the compiled representation of one
// program.AssignmentTarget: a writable location a SetOperation or a
// list/map mutation operation may modify. The compiler guarantees the
// root of every AssignmentTarget is a NameTarget naming "global" or
// "local" — the language's only two mutable roots.
//
// AssignmentTarget is a closed interface, mirroring program's own
// closed-interface pattern.
type AssignmentTarget interface {
	isAssignmentTarget()
}

// NameTarget identifies the mutable root "global" or "local".
type NameTarget struct {
	Name string
}

func (NameTarget) isAssignmentTarget() {}

// FieldTarget accesses the named field Field below Target. The compiler
// guarantees Target's resolved type is a record declaring Field.
type FieldTarget struct {
	Target AssignmentTarget
	Field  string
}

func (FieldTarget) isAssignmentTarget() {}

// IndexTarget accesses the list element or map entry below Target at
// the position or key produced by Index.
type IndexTarget struct {
	Target AssignmentTarget
	Index  Expression
}

func (IndexTarget) isAssignmentTarget() {}

// Block is the compiled representation of one program.Block: an ordered
// sequence of synchronous operations.
type Block struct {
	Operations []Operation
}

// Operation is the compiled representation of one program.Operation.
//
// Operation is a closed interface, mirroring program's own
// closed-interface pattern.
type Operation interface {
	isOperation()
}

// LetOperation introduces the immutable lexical binding Name, visible to
// later operations in the same Block (and this Block's owning
// Transition's Control, if this is the transition's top-level Block),
// bound to Value.
type LetOperation struct {
	Name  string
	Value Expression
}

func (LetOperation) isOperation() {}

// SetOperation replaces the value stored at Target with the result of
// Value.
type SetOperation struct {
	Target AssignmentTarget
	Value  Expression
}

func (SetOperation) isOperation() {}

// ListAppendOperation appends the result of Value to the end of the
// list stored at Target.
type ListAppendOperation struct {
	Target AssignmentTarget
	Value  Expression
}

func (ListAppendOperation) isOperation() {}

// ListInsertOperation inserts the result of Value into the list stored
// at Target at the position produced by Index.
type ListInsertOperation struct {
	Target AssignmentTarget
	Index  Expression
	Value  Expression
}

func (ListInsertOperation) isOperation() {}

// ListRemoveAtOperation removes the element at the position produced by
// Index from the list stored at Target.
type ListRemoveAtOperation struct {
	Target AssignmentTarget
	Index  Expression
}

func (ListRemoveAtOperation) isOperation() {}

// MapPutOperation inserts a new entry, or replaces the value of an
// existing entry, for Key in the map stored at Target.
type MapPutOperation struct {
	Target AssignmentTarget
	Key    Expression
	Value  Expression
}

func (MapPutOperation) isOperation() {}

// MapDeleteOperation removes the entry for Key from the map stored at
// Target, if one exists.
type MapDeleteOperation struct {
	Target AssignmentTarget
	Key    Expression
}

func (MapDeleteOperation) isOperation() {}

// IfOperation executes exactly one of Then or Else depending on
// Condition.
type IfOperation struct {
	Condition Expression
	Then      Block
	Else      Block
}

func (IfOperation) isOperation() {}

// ForEachOperation iterates, in order, over a snapshot of the finite
// list produced by evaluating Collection, executing Body once per
// element with ItemName (and, if non-empty, IndexName) bound.
type ForEachOperation struct {
	Collection Expression
	ItemName   string
	IndexName  string
	Body       Block
}

func (ForEachOperation) isOperation() {}

// MatchOperation executes the Body of the first case in Cases whose
// Pattern matches Value — see MatchPattern.
type MatchOperation struct {
	Value Expression
	Cases []MatchOperationCase
}

func (MatchOperation) isOperation() {}

// MatchOperationCase pairs one MatchPattern with the Body executed when
// that pattern is selected. Pattern's lexical bindings, if any, are in
// scope only within Body.
type MatchOperationCase struct {
	Pattern MatchPattern
	Body    Block
}

// DrawRandomOperation draws one value from Generator and introduces it
// as the immutable lexical binding Name, behaving lexically like the
// binding LetOperation introduces. Drawing advances the enclosing
// engineservice.Step call's candidate RandomState; per
// program.DrawRandomOperation's documented atomicity, that advancement
// is rolled back, along with every other candidate change, if the step
// fails for any reason.
type DrawRandomOperation struct {
	Name      string
	Generator RandomGenerator
}

func (DrawRandomOperation) isOperation() {}

// OpenQuestionOperation opens one concrete instance of the question
// associated with the named workflow slot Slot, for Recipient, with
// Arguments captured as that instance's parameters. The compiler
// guarantees Recipient is statically user and Arguments matches the
// slot's question's declared parameters exactly. Opening an already
// occupied slot is an execution error that fails the transition
// atomically — see program.OpenQuestionOperation.
type OpenQuestionOperation struct {
	Slot      string
	Recipient Expression
	Arguments []CallArgument
}

func (OpenQuestionOperation) isOperation() {}

// CloseQuestionOperation closes the pending question instance in the
// named workflow slot Slot, if any, without producing a
// QuestionAnsweredSignalSource signal. Closing an already empty slot is
// an idempotent no-op.
type CloseQuestionOperation struct {
	Slot string
}

func (CloseQuestionOperation) isOperation() {}

// ScheduleTimerOperation schedules a timer in the named workflow timer
// slot Slot, to fire after DelayMilliseconds evaluates. Scheduling into
// an already occupied slot is an execution error that fails the
// transition atomically — see program.ScheduleTimerOperation.
type ScheduleTimerOperation struct {
	Slot              string
	DelayMilliseconds Expression
}

func (ScheduleTimerOperation) isOperation() {}

// CancelTimerOperation cancels the currently pending timer in the named
// workflow timer slot Slot, if any, without producing a
// TimerExpiredSignalSource signal. Cancelling an already empty slot is
// an idempotent no-op.
type CancelTimerOperation struct {
	Slot string
}

func (CancelTimerOperation) isOperation() {}

// OpenKeyedQuestionOperation opens one concrete instance of the question
// associated with the named workflow keyed slot Slot's occurrence Key,
// for Recipient, with Arguments captured as that occurrence's
// parameters, generalizing OpenQuestionOperation to a (slot, key)
// occurrence. The compiler guarantees Key is statically compatible with
// the slot's declared KeyType, Recipient is statically user, and
// Arguments matches the slot's question's declared parameters exactly.
// Opening an already occupied (slot, key) is an execution error that
// fails the transition atomically — see program.OpenKeyedQuestionOperation.
type OpenKeyedQuestionOperation struct {
	Slot      string
	Key       Expression
	Recipient Expression
	Arguments []CallArgument
}

func (OpenKeyedQuestionOperation) isOperation() {}

// CloseKeyedQuestionOperation closes the pending question instance at
// the named workflow keyed slot Slot's occurrence Key, if any, without
// producing a KeyedQuestionAnsweredSignalSource signal, generalizing
// CloseQuestionOperation to a (slot, key) occurrence. Closing an already
// empty (slot, key) is an idempotent no-op.
type CloseKeyedQuestionOperation struct {
	Slot string
	Key  Expression
}

func (CloseKeyedQuestionOperation) isOperation() {}

// ScheduleKeyedTimerOperation schedules a timer at the named workflow
// keyed timer slot Slot's occurrence Key, to fire after
// DelayMilliseconds evaluates, generalizing ScheduleTimerOperation to a
// (slot, key) occurrence. Scheduling into an already occupied (slot,
// key) is an execution error that fails the transition atomically — see
// program.ScheduleKeyedTimerOperation.
type ScheduleKeyedTimerOperation struct {
	Slot              string
	Key               Expression
	DelayMilliseconds Expression
}

func (ScheduleKeyedTimerOperation) isOperation() {}

// CancelKeyedTimerOperation cancels the currently pending timer at the
// named workflow keyed timer slot Slot's occurrence Key, if any, without
// producing a KeyedTimerExpiredSignalSource signal, generalizing
// CancelTimerOperation to a (slot, key) occurrence. Cancelling an
// already empty (slot, key) is an idempotent no-op.
type CancelKeyedTimerOperation struct {
	Slot string
	Key  Expression
}

func (CancelKeyedTimerOperation) isOperation() {}

// EmitEffectOperation emits one instance of the named effect Effect to
// Recipients with the given Arguments. The compiler guarantees
// Recipients is statically list<user> and Arguments matches Effect's
// declared parameters exactly. This never mutates authoritative state;
// it only produces a declarative EmitEffectOutput once the enclosing
// transition commits.
type EmitEffectOperation struct {
	Effect     string
	Recipients Expression
	Arguments  []CallArgument
}

func (EmitEffectOperation) isOperation() {}
